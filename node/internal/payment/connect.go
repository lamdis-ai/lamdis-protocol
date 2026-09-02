package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Getting money out to the people who earned it.
//
// Everything else in this package moves money *into* escrow and holds it. This
// file is the exit. Without it the exchange is a place where earnings accrue
// correctly in a double-entry ledger and no human is ever paid, which is a
// worse failure than not tracking them at all: the numbers say you are owed
// something and nothing arrives.
//
// The shape is Stripe Connect Express. The exchange never sees a bank account,
// a routing number, or a government ID — the worker enters those on Stripe's
// own hosted page and we hold nothing but an account id. That is deliberate:
// the alternative is being the custodian of identity documents for every
// person who ever mows a lawn through this thing.

// ConnectAccount is what the rail knows about a payee.
type ConnectAccount struct {
	// ID is the rail's account identifier, e.g. acct_1234. Safe to store.
	ID string `json:"id"`
	// PayoutsEnabled is whether money can actually reach them right now.
	// Distinct from existing: an account exists the moment it is created and
	// cannot receive anything for as long as verification takes.
	PayoutsEnabled bool `json:"payouts_enabled"`
	// DetailsSubmitted is whether they finished the hosted form at all.
	DetailsSubmitted bool `json:"details_submitted"`
	// Needs is what the rail is still waiting for, in the rail's own words,
	// so a stalled worker is told the actual reason rather than "pending".
	Needs []string `json:"needs,omitempty"`
	// Disabled explains a hard stop, when the rail has given one.
	Disabled string `json:"disabled,omitempty"`
}

// Ready reports whether a transfer to this account can be expected to land.
func (a ConnectAccount) Ready() bool { return a.PayoutsEnabled }

// CreateAccount opens an Express account for one person.
//
// The email is passed so Stripe can reach them about verification; it is the
// only personal detail that crosses this boundary.
func (s *Stripe) CreateAccount(ctx context.Context, person, email string) (ConnectAccount, error) {
	return s.CreateAccountAs(ctx, person, email, "individual", "")
}

// CreateAccountAs opens an account of a given kind.
//
// Business type was hardcoded to "individual", which meant a company could not
// be paid as a company: the rail asked whoever clicked for their own social
// security number, revenue landed in a personal bank account, and the tax
// document went to an employee rather than the business. That is not friction,
// it is a bookkeeping problem no real contractor will accept.
func (s *Stripe) CreateAccountAs(ctx context.Context, person, email, businessType, legalName string) (ConnectAccount, error) {
	if businessType != "individual" && businessType != "company" {
		businessType = "individual"
	}
	f := url.Values{}
	f.Set("type", "express")
	f.Set("business_type", businessType)
	if email != "" {
		f.Set("email", email)
	}
	if businessType == "company" && legalName != "" {
		// Given so the rail can pre-fill and so the name on the account
		// matches the name the buyer was shown.
		f.Set("company[name]", legalName)
	}
	// Capabilities have to be requested explicitly; an account without
	// transfers accepted can be created successfully and never receive a cent.
	f.Set("capabilities[transfers][requested]", "true")
	f.Set("metadata[lamdis_person]", person)
	// Payouts to the person's bank are left on Stripe's default schedule.
	// Choosing one here would mean holding money longer than the rail needs to.

	obj, err := s.call(ctx, http.MethodPost, "/v1/accounts", f,
		DeriveKey("connect-account", person))
	if err != nil {
		return ConnectAccount{}, err
	}
	return accountFrom(obj), nil
}

// AccountLink returns a one-time URL where the worker completes verification.
//
// The link is short-lived and single-use by design at the rail, so it is
// minted on demand rather than stored. Storing one would mean handing out a
// stale link that fails with no explanation.
func (s *Stripe) AccountLink(ctx context.Context, acct, refreshURL, returnURL string) (string, error) {
	f := url.Values{}
	f.Set("account", acct)
	f.Set("refresh_url", refreshURL)
	f.Set("return_url", returnURL)
	f.Set("type", "account_onboarding")
	// Collect everything now. The alternative — collecting the minimum and
	// asking again later — strands a worker at the moment they try to cash
	// out, which is the worst possible time to discover a missing document.
	f.Set("collection_options[fields]", "eventually_due")

	// Never idempotent: each link is meant to be fresh, and replaying a spent
	// one is exactly the failure this avoids.
	obj, err := s.call(ctx, http.MethodPost, "/v1/account_links", f, "")
	if err != nil {
		return "", err
	}
	u, _ := obj.str("url")
	if u == "" {
		return "", fmt.Errorf("payment: the rail returned no onboarding link")
	}
	return u, nil
}

// Account reports the current state of a payee.
func (s *Stripe) Account(ctx context.Context, acct string) (ConnectAccount, error) {
	obj, err := s.call(ctx, http.MethodGet, "/v1/accounts/"+url.PathEscape(acct), nil, "")
	if err != nil {
		return ConnectAccount{}, err
	}
	return accountFrom(obj), nil
}

// PayOut sends earned money to a payee's connected account.
//
// This is a transfer from the platform balance, not a charge: the money was
// already collected when the buyer funded their balance. The idempotency key
// is derived from the caller's key, so a retry after a lost response pays
// once.
func (s *Stripe) PayOut(ctx context.Context, key Key, acct string, amountMinor int64, currency, note string) (Result, error) {
	if amountMinor <= 0 {
		return Result{}, fmt.Errorf("payment: a payout must be positive")
	}
	if acct == "" {
		return Result{}, fmt.Errorf("payment: no connected account for this payee")
	}
	f := url.Values{}
	f.Set("amount", strconv.FormatInt(amountMinor, 10))
	f.Set("currency", strings.ToLower(currency))
	f.Set("destination", acct)
	if note != "" {
		f.Set("description", note)
	}
	f.Set("metadata[lamdis_payout]", string(key))

	obj, err := s.call(ctx, http.MethodPost, "/v1/transfers", f, key)
	if err != nil {
		return Result{}, err
	}
	return s.result(key, "transfer", obj)
}

func accountFrom(obj *object) ConnectAccount {
	a := ConnectAccount{}
	a.ID, _ = obj.str("id")
	a.PayoutsEnabled, _ = obj.boolean("payouts_enabled")
	a.DetailsSubmitted, _ = obj.boolean("details_submitted")
	a.Needs = obj.requirements()
	a.Disabled = obj.disabledReason()
	return a
}

// The account and account_link objects carry fields the shared object struct
// does not model. Rather than widen that struct with Connect-only fields —
// which would make every charge carry dead weight — they are read out of the
// raw body here.

func (o *object) str(field string) (string, bool) {
	if o == nil || len(o.Raw) == 0 {
		return "", false
	}
	var m map[string]any
	if err := json.Unmarshal(o.Raw, &m); err != nil {
		return "", false
	}
	s, ok := m[field].(string)
	return s, ok
}

func (o *object) boolean(field string) (bool, bool) {
	if o == nil || len(o.Raw) == 0 {
		return false, false
	}
	var m map[string]any
	if err := json.Unmarshal(o.Raw, &m); err != nil {
		return false, false
	}
	b, ok := m[field].(bool)
	return b, ok
}

// requirements lists what the rail still wants, preferring the things that are
// blocking right now over the things that will block later.
func (o *object) requirements() []string {
	if o == nil || len(o.Raw) == 0 {
		return nil
	}
	var m struct {
		Requirements struct {
			CurrentlyDue  []string `json:"currently_due"`
			PastDue       []string `json:"past_due"`
			EventuallyDue []string `json:"eventually_due"`
		} `json:"requirements"`
	}
	if err := json.Unmarshal(o.Raw, &m); err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, group := range [][]string{
		m.Requirements.PastDue,
		m.Requirements.CurrentlyDue,
		m.Requirements.EventuallyDue,
	} {
		for _, r := range group {
			if !seen[r] {
				seen[r] = true
				out = append(out, r)
			}
		}
	}
	return out
}

func (o *object) disabledReason() string {
	if o == nil || len(o.Raw) == 0 {
		return ""
	}
	var m struct {
		Requirements struct {
			DisabledReason string `json:"disabled_reason"`
		} `json:"requirements"`
	}
	if err := json.Unmarshal(o.Raw, &m); err != nil {
		return ""
	}
	return m.Requirements.DisabledReason
}

// Checkout starts a hosted payment page for adding funds to a balance.
//
// Hosted rather than an inline form, for the same reason payouts use hosted
// onboarding: the exchange never touches a card number, so the worst outcome
// of a bug here is a broken link rather than a card breach.
func (s *Stripe) Checkout(ctx context.Context, person string, amountMinor int64, currency, successURL, cancelURL string) (ref, payAt string, err error) {
	if amountMinor <= 0 {
		return "", "", fmt.Errorf("payment: a deposit must be positive")
	}
	f := url.Values{}
	f.Set("mode", "payment")
	f.Set("success_url", successURL)
	f.Set("cancel_url", cancelURL)
	f.Set("client_reference_id", person)
	f.Set("line_items[0][quantity]", "1")
	f.Set("line_items[0][price_data][currency]", strings.ToLower(currency))
	f.Set("line_items[0][price_data][unit_amount]", strconv.FormatInt(amountMinor, 10))
	f.Set("line_items[0][price_data][product_data][name]", "Lamdis Exchange balance")
	f.Set("line_items[0][price_data][product_data][description]",
		"Funds your agents spend on work in the world")
	f.Set("metadata[lamdis_person]", person)
	// Payment methods are left to the rail's defaults so a bank debit can be
	// offered where the fees are lower, rather than forcing cards everywhere.

	obj, err := s.call(ctx, http.MethodPost, "/v1/checkout/sessions", f,
		DeriveKey("checkout", fmt.Sprintf("%s:%d:%s", person, amountMinor, currency)))
	if err != nil {
		return "", "", err
	}
	payAt, _ = obj.str("url")
	if payAt == "" {
		return "", "", fmt.Errorf("payment: the rail returned no checkout link")
	}
	return obj.ID, payAt, nil
}

// CheckoutPaid reports whether a checkout session actually completed.
//
// Polled rather than taken on trust from the browser's return: a person who
// lands on the success page has not necessarily paid, and crediting a balance
// on a redirect is how you give money away.
func (s *Stripe) CheckoutPaid(ctx context.Context, session string) (paid bool, amountMinor int64, person string, err error) {
	obj, err := s.call(ctx, http.MethodGet,
		"/v1/checkout/sessions/"+url.PathEscape(session), nil, "")
	if err != nil {
		return false, 0, "", err
	}
	status, _ := obj.str("payment_status")
	person, _ = obj.str("client_reference_id")
	amountMinor = obj.amountTotal()
	return status == "paid", amountMinor, person, nil
}

func (o *object) amountTotal() int64 {
	if o == nil || len(o.Raw) == 0 {
		return 0
	}
	var m struct {
		AmountTotal int64 `json:"amount_total"`
	}
	if err := json.Unmarshal(o.Raw, &m); err != nil {
		return 0
	}
	return m.AmountTotal
}

// FindAccountFor recovers a payee's account from the rail itself.
//
// The person-to-account mapping is ours to keep, and on a deployment whose
// storage does not survive a restart, ours to lose. Losing it is not a
// cosmetic failure: the next call would open a *second* account for somebody
// who already finished verification, stranding the first one and asking them
// to prove who they are all over again.
//
// The rail already knows. Every account we open is stamped with the person it
// belongs to, so the mapping can always be rebuilt from the authoritative
// side. This is the recovery path, consulted before creating anything.
func (s *Stripe) FindAccountFor(ctx context.Context, person string) (ConnectAccount, bool, error) {
	if person == "" {
		return ConnectAccount{}, false, nil
	}
	starting := ""
	// Bounded rather than unbounded: ten pages is ten thousand accounts, and a
	// deployment past that needs a real index, not a longer loop.
	for page := 0; page < 10; page++ {
		f := url.Values{}
		f.Set("limit", "100")
		if starting != "" {
			f.Set("starting_after", starting)
		}
		obj, err := s.call(ctx, http.MethodGet, "/v1/accounts?"+f.Encode(), nil, "")
		if err != nil {
			return ConnectAccount{}, false, err
		}
		accounts, more, last := obj.accountPage()
		for _, raw := range accounts {
			if raw.person == person {
				got, err := s.Account(ctx, raw.id)
				return got, true, err
			}
		}
		if !more || last == "" {
			break
		}
		starting = last
	}
	return ConnectAccount{}, false, nil
}

type rawAccount struct {
	id     string
	person string
}

func (o *object) accountPage() (accounts []rawAccount, hasMore bool, last string) {
	if o == nil || len(o.Raw) == 0 {
		return nil, false, ""
	}
	var m struct {
		HasMore bool `json:"has_more"`
		Data    []struct {
			ID       string            `json:"id"`
			Metadata map[string]string `json:"metadata"`
		} `json:"data"`
	}
	if err := json.Unmarshal(o.Raw, &m); err != nil {
		return nil, false, ""
	}
	for _, d := range m.Data {
		accounts = append(accounts, rawAccount{id: d.ID, person: d.Metadata["lamdis_person"]})
		last = d.ID
	}
	return accounts, m.HasMore, last
}

// EachAccount walks every account this platform has opened.
//
// One pass, for rebuilding a whole person-to-account mapping at once.
// FindAccountFor answers the same question for a single person, which is the
// same amount of work — so anything asking about more than one person should
// come through here instead.
func (s *Stripe) EachAccount(ctx context.Context, visit func(person, acct string)) error {
	starting := ""
	for page := 0; page < 20; page++ {
		f := url.Values{}
		f.Set("limit", "100")
		if starting != "" {
			f.Set("starting_after", starting)
		}
		obj, err := s.call(ctx, http.MethodGet, "/v1/accounts?"+f.Encode(), nil, "")
		if err != nil {
			return err
		}
		accounts, more, last := obj.accountPage()
		for _, a := range accounts {
			visit(a.person, a.id)
		}
		if !more || last == "" {
			return nil
		}
		starting = last
	}
	return nil
}

// AuthorizeCheckout opens a hosted checkout that authorises a card for a job's
// ceiling without capturing it.
//
// This is the gateless way in. The buyer has no account and no balance; their
// card stands behind the job instead, and is charged once, later, for what the
// job actually paid out. Stripe holds an uncaptured authorisation for up to
// seven days, which is longer than any job on the board lives.
func (s *Stripe) AuthorizeCheckout(ctx context.Context, job string, amountMinor int64, currency, successURL, cancelURL string) (session, payAt string, err error) {
	if amountMinor <= 0 {
		return "", "", fmt.Errorf("payment: an authorisation must be positive")
	}
	f := url.Values{}
	f.Set("mode", "payment")
	f.Set("success_url", successURL)
	f.Set("cancel_url", cancelURL)
	f.Set("client_reference_id", job)
	f.Set("payment_intent_data[capture_method]", "manual")
	f.Set("payment_intent_data[description]", "lamdis job "+job)
	f.Set("payment_intent_data[metadata][lamdis_job]", job)
	f.Set("metadata[lamdis_job]", job)
	f.Set("line_items[0][quantity]", "1")
	f.Set("line_items[0][price_data][currency]", strings.ToLower(currency))
	f.Set("line_items[0][price_data][unit_amount]", strconv.FormatInt(amountMinor, 10))
	f.Set("line_items[0][price_data][product_data][name]", "Lamdis job "+job)
	f.Set("line_items[0][price_data][product_data][description]",
		"Authorised now. Charged only for what is proven done, and only then.")
	// Cards only: an authorisation has to be reversible and partial-capturable,
	// which bank debits are not.
	f.Set("payment_method_types[0]", "card")

	obj, err := s.call(ctx, http.MethodPost, "/v1/checkout/sessions", f,
		DeriveKey("authorize", fmt.Sprintf("%s:%d:%s", job, amountMinor, currency)))
	if err != nil {
		return "", "", err
	}
	payAt, _ = obj.str("url")
	if payAt == "" {
		return "", "", fmt.Errorf("payment: the rail returned no checkout link")
	}
	return obj.ID, payAt, nil
}

// CheckoutAuthorization reports whether a checkout opened by AuthorizeCheckout
// completed, and what it authorised.
//
// A manual-capture session reports payment_status "unpaid" until capture, so
// "paid" is the wrong question here; completion of the session is the fact
// that the card was authorised.
func (s *Stripe) CheckoutAuthorization(ctx context.Context, session string) (ok bool, intent string, amountMinor int64, email string, err error) {
	obj, err := s.call(ctx, http.MethodGet,
		"/v1/checkout/sessions/"+url.PathEscape(session), nil, "")
	if err != nil {
		return false, "", 0, "", err
	}
	var m struct {
		Status          string `json:"status"`
		PaymentIntent   string `json:"payment_intent"`
		AmountTotal     int64  `json:"amount_total"`
		CustomerDetails struct {
			Email string `json:"email"`
		} `json:"customer_details"`
	}
	if len(obj.Raw) > 0 {
		_ = json.Unmarshal(obj.Raw, &m)
	}
	return m.Status == "complete" && m.PaymentIntent != "", m.PaymentIntent, m.AmountTotal, m.CustomerDetails.Email, nil
}
