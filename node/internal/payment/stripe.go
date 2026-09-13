package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Stripe is a payment rail backed by Stripe Connect, using separate charges
// and transfers.
//
// The mapping to the Adapter interface is not arbitrary, and it is worth
// stating because the obvious mapping is worse:
//
//	Hold     PaymentIntent with capture_method=manual. The buyer's card is
//	         authorized for MaxPayout(terms) and nothing is charged. This is
//	         what makes a contingent payout pre-fundable.
//	Capture  Partial capture of that authorization for the amount the terms
//	         evaluated to, then a Transfer of the net to the provider.
//	Release  Cancels an authorization that was never captured.
//	Refund   Reverses money that was already captured.
//
// The alternative — charge the full amount up front and refund the remainder —
// was measured against the live test API and is strictly worse: Stripe returns
// no processing fee on a refund, but does return a proportional fee on the
// uncaptured part of an authorization. On a $5.00 outcome settling at $1.50
// that is 45c of fees versus 34c.
//
// A partial capture implicitly releases the remainder, so Release after a
// capture is a no-op rather than an error. Stripe permits exactly one capture
// per authorization, which is why the terms must be fully evaluated before
// Capture is called.
type Stripe struct {
	// Secret is the API key. Test keys begin sk_test_; a live key is refused
	// unless AllowLive is set, because every accident in this package is
	// somebody's real money.
	Secret    string
	AllowLive bool

	// Keys maps our idempotency keys to the rail objects they created. Stripe
	// has no lookup-by-idempotency-key endpoint, so an adapter that wants to
	// answer Status after a lost response must remember this itself.
	Keys KeyStore

	HTTP *http.Client
	Base string
	Now  func() time.Time
}

// KeyStore remembers which rail object an idempotency key produced. The
// in-memory implementation is enough for a single process; a durable one is
// what makes reconciliation survive a restart.
type KeyStore interface {
	Get(k Key) (ref, kind string, ok bool)
	Put(k Key, ref, kind string) error
}

// MemoryKeys is a KeyStore for tests and single-process runs.
type MemoryKeys struct {
	mu sync.Mutex
	m  map[Key][2]string
}

func NewMemoryKeys() *MemoryKeys { return &MemoryKeys{m: map[Key][2]string{}} }

func (s *MemoryKeys) Get(k Key) (string, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[k]
	return v[0], v[1], ok
}

func (s *MemoryKeys) Put(k Key, ref, kind string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[k] = [2]string{ref, kind}
	return nil
}

// NewStripe builds the rail from the environment.
func NewStripe() (*Stripe, error) {
	secret := strings.TrimSpace(os.Getenv("STRIPE_SECRET_KEY"))
	if secret == "" {
		return nil, fmt.Errorf("payment: STRIPE_SECRET_KEY is not set")
	}
	s := &Stripe{
		Secret:    secret,
		AllowLive: os.Getenv("LAMDIS_STRIPE_ALLOW_LIVE") == "1",
		Keys:      NewMemoryKeys(),
		HTTP:      &http.Client{Timeout: 30 * time.Second},
		Base:      "https://api.stripe.com",
		Now:       time.Now,
	}
	if err := s.checkKey(); err != nil {
		return nil, err
	}
	return s, nil
}

// checkKey refuses a live key that was not explicitly asked for. This is the
// same reflex as the budget guard: the failure it prevents is not recoverable
// by noticing it afterwards.
func (s *Stripe) checkKey() error {
	if s.Live() && !s.AllowLive {
		return fmt.Errorf("payment: refusing a live Stripe key; " +
			"set LAMDIS_STRIPE_ALLOW_LIVE=1 to move real money")
	}
	if !strings.HasPrefix(s.Secret, "sk_") && !strings.HasPrefix(s.Secret, "rk_") {
		return fmt.Errorf("payment: STRIPE_SECRET_KEY does not look like a Stripe secret key")
	}
	return nil
}

func (s *Stripe) Rail() string { return "stripe" }

func (s *Stripe) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Live reports whether this rail is pointed at real money.
//
// A restricted key (rk_live_) moves exactly as much real money as a standard
// one within its permissions; treating only sk_live_ as live let a restricted
// key past the guard and printed "test mode" over real charges.
func (s *Stripe) Live() bool {
	return strings.HasPrefix(s.Secret, "sk_live_") || strings.HasPrefix(s.Secret, "rk_live_")
}

// object is the subset of a Stripe response we act on. Everything else is kept
// verbatim in Result.Raw so the ledger can record what the rail actually said.
type object struct {
	ID             string          `json:"id"`
	Object         string          `json:"object"`
	Status         string          `json:"status"`
	Amount         int64           `json:"amount"`
	AmountCaptured int64           `json:"amount_captured"`
	LatestCharge   string          `json:"latest_charge"`
	Livemode       bool            `json:"livemode"`
	Error          *stripeError    `json:"error"`
	Raw            json.RawMessage `json:"-"`
}

type stripeError struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// call posts a form to Stripe under an idempotency key.
//
// Retries reuse the same key deliberately: that is the entire reason the key is
// a pure function of signed content. A retried request is either deduplicated
// by Stripe or was never received.
func (s *Stripe) call(ctx context.Context, method, path string, form url.Values, key Key) (*object, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 500 * time.Millisecond):
			}
		}
		obj, retry, err := s.once(ctx, method, path, form, key)
		if err == nil {
			return obj, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, lastErr
}

func (s *Stripe) once(ctx context.Context, method, path string, form url.Values, key Key) (*object, bool, error) {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, s.Base+path, body)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", "Bearer "+s.Secret)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", string(key))
	}

	resp, err := s.HTTP.Do(req)
	if err != nil {
		// The request may or may not have been applied. Saying "failed" here
		// is how a payment system pays twice.
		return nil, true, fmt.Errorf("%w: %v", ErrUnknown, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, fmt.Errorf("%w: reading response: %v", ErrUnknown, err)
	}

	var obj object
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, resp.StatusCode >= 500, fmt.Errorf("payment: unparseable Stripe response: %v", err)
	}
	obj.Raw = json.RawMessage(raw)

	switch {
	case resp.StatusCode < 300:
		return &obj, false, nil
	case resp.StatusCode == 429 || resp.StatusCode >= 500:
		return nil, true, fmt.Errorf("payment: stripe %d: %s", resp.StatusCode, obj.errMessage())
	default:
		// 4xx is a decision, not a hiccup. Retrying cannot change it.
		return nil, false, fmt.Errorf("payment: stripe %d: %s", resp.StatusCode, obj.errMessage())
	}
}

func (o *object) errMessage() string {
	if o != nil && o.Error != nil {
		return o.Error.Message
	}
	return "unknown error"
}

// result turns a rail object into a Result and remembers the key mapping.
func (s *Stripe) result(k Key, kind string, obj *object) (Result, error) {
	state := StateSucceeded
	switch obj.Status {
	case "requires_capture", "succeeded", "paid", "":
		state = StateSucceeded
	case "processing", "requires_action", "requires_confirmation", "pending":
		state = StatePending
	case "canceled":
		state = StateSucceeded // a deliberate cancel is a successful release
	case "requires_payment_method":
		state = StateFailed
	}
	if s.Keys != nil && obj.ID != "" {
		if err := s.Keys.Put(k, obj.ID, kind); err != nil {
			return Result{}, err
		}
	}
	return Result{
		Ref:        obj.ID,
		State:      state,
		Raw:        obj.Raw,
		ObservedAt: s.now(),
	}, nil
}

func metadataFor(r Request) url.Values {
	f := url.Values{}
	f.Set("metadata[lamdis_outcome]", r.Outcome)
	f.Set("metadata[lamdis_instruction]", r.Instruction)
	f.Set("metadata[lamdis_key]", string(r.Key))
	return f
}

// Hold authorizes the buyer's card for the full amount the terms could pay out.
//
// PaymentMethod comes from the outcome's buyer record. The rail never stores
// card details; it receives a payment method id that the buyer's own client
// created.
func (s *Stripe) Hold(ctx context.Context, r Request) (Result, error) {
	if r.Key == "" {
		return Result{}, fmt.Errorf("payment: operation has no idempotency key")
	}
	pm := r.Source
	if pm == "" {
		return Result{}, fmt.Errorf("payment: hold needs a payment method")
	}
	f := metadataFor(r)
	f.Set("amount", strconv.FormatInt(r.AmountMinor, 10))
	f.Set("currency", strings.ToLower(r.Currency))
	f.Set("payment_method", pm)
	f.Set("confirm", "true")
	f.Set("capture_method", "manual")
	f.Set("transfer_group", r.Outcome)
	f.Set("description", "lamdis outcome "+r.Outcome)
	f.Set("automatic_payment_methods[enabled]", "true")
	f.Set("automatic_payment_methods[allow_redirects]", "never")

	obj, err := s.call(ctx, http.MethodPost, "/v1/payment_intents", f, r.Key)
	if err != nil {
		return Result{State: stateForErr(err), ObservedAt: s.now()}, err
	}
	return s.result(r.Key, "payment_intent", obj)
}

// Capture takes the settled amount out of the authorization and forwards the
// provider's net.
//
// Two rail movements, so two derived keys. They must be distinct or Stripe
// would treat the transfer as a replay of the capture and silently return the
// capture's response — money that never moved, reported as moved.
func (s *Stripe) Capture(ctx context.Context, r Request) (Result, error) {
	if r.Key == "" {
		return Result{}, fmt.Errorf("payment: operation has no idempotency key")
	}
	if r.FeeMinor < 0 || r.FeeMinor > r.AmountMinor {
		return Result{}, fmt.Errorf("payment: fee %d is not within the captured %d",
			r.FeeMinor, r.AmountMinor)
	}
	pi, err := s.holdRef(r)
	if err != nil {
		return Result{}, err
	}

	f := metadataFor(r)
	f.Set("amount_to_capture", strconv.FormatInt(r.AmountMinor, 10))
	capKey := DeriveKey("capture", string(r.Key))
	obj, err := s.call(ctx, http.MethodPost, "/v1/payment_intents/"+pi+"/capture", f, capKey)
	if err != nil {
		return Result{State: stateForErr(err), ObservedAt: s.now()}, err
	}
	res, err := s.result(capKey, "capture", obj)
	if err != nil {
		return Result{}, err
	}

	// Nothing to forward: the provider earned nothing, or there is no payee.
	net := r.AmountMinor - r.FeeMinor
	if net <= 0 || r.Destination == "" {
		return res, nil
	}

	tf := url.Values{}
	tf.Set("amount", strconv.FormatInt(net, 10))
	tf.Set("currency", strings.ToLower(r.Currency))
	tf.Set("destination", r.Destination)
	tf.Set("transfer_group", r.Outcome)
	tf.Set("description", "lamdis settlement "+r.Outcome)
	tf.Set("metadata[lamdis_outcome]", r.Outcome)
	tf.Set("metadata[lamdis_instruction]", r.Instruction)
	// Tying the transfer to the charge makes it wait for those funds rather
	// than drawing on whatever else happens to be in the platform balance.
	if obj.LatestCharge != "" {
		tf.Set("source_transaction", obj.LatestCharge)
	}
	trKey := DeriveKey("transfer", string(r.Key))
	tr, err := s.call(ctx, http.MethodPost, "/v1/transfers", tf, trKey)
	if err != nil {
		// The capture succeeded and the transfer did not. The money is on the
		// platform balance and owed to the provider; reconciliation, not a
		// retry of the whole capture, is what resolves this.
		return Result{Ref: res.Ref, State: StateUnknown, Raw: res.Raw, ObservedAt: s.now()},
			fmt.Errorf("payment: captured %s but transfer failed: %w", res.Ref, err)
	}
	if _, err := s.result(trKey, "transfer", tr); err != nil {
		return Result{}, err
	}
	return res, nil
}

// Release cancels an authorization nothing was captured from.
//
// After a partial capture Stripe has already released the remainder, so this
// is a no-op rather than an error — the postcondition the caller wants is
// "nothing is still authorized", and that already holds.
func (s *Stripe) Release(ctx context.Context, r Request) (Result, error) {
	pi, err := s.holdRef(r)
	if err != nil {
		return Result{}, err
	}
	if _, _, captured := s.Keys.Get(DeriveKey("capture", string(r.Key))); captured {
		return Result{Ref: pi, State: StateSucceeded, ObservedAt: s.now()}, nil
	}
	f := url.Values{}
	f.Set("cancellation_reason", "abandoned")
	relKey := DeriveKey("release", string(r.Key))
	obj, err := s.call(ctx, http.MethodPost, "/v1/payment_intents/"+pi+"/cancel", f, relKey)
	if err != nil {
		return Result{State: stateForErr(err), ObservedAt: s.now()}, err
	}
	return s.result(relKey, "release", obj)
}

// Refund reverses captured money.
//
// Unlike an uncaptured release, Stripe keeps its processing fee on a refund.
// The exchange eats that; passing it to either party would mean the amount
// returned did not match the amount the terms said to return.
func (s *Stripe) Refund(ctx context.Context, r Request) (Result, error) {
	pi, err := s.holdRef(r)
	if err != nil {
		return Result{}, err
	}
	f := metadataFor(r)
	f.Set("payment_intent", pi)
	f.Set("amount", strconv.FormatInt(r.AmountMinor, 10))
	refKey := DeriveKey("refund", string(r.Key))
	obj, err := s.call(ctx, http.MethodPost, "/v1/refunds", f, refKey)
	if err != nil {
		return Result{State: stateForErr(err), ObservedAt: s.now()}, err
	}
	return s.result(refKey, "refund", obj)
}

// Status answers what became of a key after a lost response.
func (s *Stripe) Status(ctx context.Context, key Key) (Result, error) {
	ref, kind, ok := s.Keys.Get(key)
	if !ok {
		return Result{State: StateUnknown, ObservedAt: s.now()}, ErrUnknown
	}
	path := ""
	switch kind {
	case "payment_intent", "capture", "release":
		path = "/v1/payment_intents/" + ref
	case "transfer":
		path = "/v1/transfers/" + ref
	case "refund":
		path = "/v1/refunds/" + ref
	default:
		return Result{State: StateUnknown, ObservedAt: s.now()}, ErrUnknown
	}
	obj, err := s.call(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return Result{State: StateUnknown, ObservedAt: s.now()}, err
	}
	return Result{
		Ref: obj.ID, State: stateFromStatus(obj.Status),
		Raw: obj.Raw, ObservedAt: s.now(),
	}, nil
}

func stateFromStatus(status string) string {
	switch status {
	case "succeeded", "paid", "requires_capture", "canceled", "":
		return StateSucceeded
	case "processing", "pending", "requires_action", "requires_confirmation":
		return StatePending
	default:
		return StateFailed
	}
}

func stateForErr(err error) string {
	if strings.Contains(err.Error(), ErrUnknown.Error()) {
		return StateUnknown
	}
	return StateFailed
}

// holdRef finds the authorization an operation acts on.
//
// The caller supplies it, because the entry that instructs a settlement is not
// the entry that instructed the hold and their keys therefore differ. The
// KeyStore is consulted only as a fallback for a hold this same process placed.
func (s *Stripe) holdRef(r Request) (string, error) {
	if r.HoldRef != "" {
		return r.HoldRef, nil
	}
	if ref, _, ok := s.Keys.Get(r.Key); ok {
		return ref, nil
	}
	return "", fmt.Errorf("payment: no authorization known for outcome %s", r.Outcome)
}
