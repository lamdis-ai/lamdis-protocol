package exchange

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/ledger"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/payment"
)

// The exit.
//
// Money entered this system through top-ups, moved through escrow, and settled
// into a payable balance. Until this file existed there was no way for any of
// it to reach a person: the rail hook was declared and never assigned, so
// every worker was told "payouts are not switched on yet" forever while the
// board advertised getting paid.
//
// Two things are kept deliberately separate. Connecting an account is the
// worker's act, done on the rail's own hosted pages so the exchange never
// holds a bank account or an identity document. Sending money is the
// exchange's act, done on a threshold so a fixed per-transfer fee does not eat
// a five dollar errand.

// PayoutAccounts remembers which rail account belongs to which person.
//
// The mapping is the only thing worth storing: everything else about the
// account — whether it can receive money, what is still missing — is the
// rail's answer and goes stale the moment we cache it, so it is asked for
// rather than remembered.
type PayoutAccounts struct {
	mu   sync.Mutex
	by   map[string]string
	path string
	// hydrated records that the whole mapping has been rebuilt from the rail.
	//
	// Recovery used to run per person, on the critical path of somebody
	// clicking a button: a full scan of the rail's accounts, sixteen seconds,
	// every time. It is the same scan whoever asks for it, so it happens once
	// and everybody reads the result.
	hydrated  bool
	hydrating sync.Mutex
}

// NewPayoutAccounts loads the mapping, or starts empty.
//
// It persists to disk because losing it would orphan every connected account:
// the worker would be sent through verification again while their first
// account sat on the rail, verified and unreachable.
func NewPayoutAccounts(dir string) *PayoutAccounts {
	p := &PayoutAccounts{by: map[string]string{}}
	if dir == "" {
		return p
	}
	p.path = filepath.Join(dir, "payout-accounts.json")
	if b, err := os.ReadFile(p.path); err == nil {
		json.Unmarshal(b, &p.by)
	}
	return p
}

// Hydrate rebuilds the whole mapping from the rail, once.
//
// Our copy lives on storage a redeploy can wipe; the rail's does not. Rather
// than rediscovering one person at a time — which made every new worker pay
// for the scan — the map is reconstructed in a single pass and reused.
//
// Safe to call repeatedly and from several requests at once: the first caller
// does the work and the rest wait for it.
func (p *PayoutAccounts) Hydrate(ctx context.Context, rail PayoutRail) {
	lister, ok := rail.(interface {
		EachAccount(context.Context, func(person, acct string)) error
	})
	if !ok {
		return
	}
	p.hydrating.Lock()
	defer p.hydrating.Unlock()
	if p.done() {
		return
	}
	found := map[string]string{}
	if err := lister.EachAccount(ctx, func(person, acct string) {
		if person != "" {
			found[person] = acct
		}
	}); err != nil {
		// Leave it unhydrated so a later request tries again. Marking it done
		// on a failed scan would strand everybody whose mapping was lost.
		log.Printf("payout: could not rebuild the account mapping: %v", err)
		return
	}
	p.mu.Lock()
	for person, acct := range found {
		if _, have := p.by[person]; !have {
			p.by[person] = acct
		}
	}
	p.hydrated = true
	err := p.saveLocked()
	p.mu.Unlock()
	if err != nil {
		log.Printf("payout: rebuilt mapping but could not save it: %v", err)
	}
}

func (p *PayoutAccounts) done() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.hydrated
}

// Get returns the rail account for a person, if they have one.
func (p *PayoutAccounts) Get(person string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	a, ok := p.by[person]
	return a, ok
}

// Put records a new account, refusing to overwrite an existing one.
//
// Repointing a person at a different account is how earned money ends up in
// somebody else's bank, so it is not something this method can do by accident.
func (p *PayoutAccounts) Put(person, acct string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if prev, ok := p.by[person]; ok && prev != acct {
		return fmt.Errorf("payout: %s already has a payout account", person)
	}
	p.by[person] = acct
	return p.saveLocked()
}

func (p *PayoutAccounts) saveLocked() error {
	if p.path == "" {
		return nil
	}
	b, err := json.MarshalIndent(p.by, "", "  ")
	if err != nil {
		return err
	}
	// Written to a temp file and renamed so a crash mid-write cannot leave a
	// truncated map that silently loses people's accounts.
	tmp := p.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p.path)
}

// PayoutRail is the part of the payment adapter this file needs. Narrow on
// purpose: nothing here should be able to take a payment.
type PayoutRail interface {
	CreateAccount(ctx context.Context, person, email string) (payment.ConnectAccount, error)
	CreateAccountAs(ctx context.Context, person, email, businessType, legalName string) (payment.ConnectAccount, error)
	AccountLink(ctx context.Context, acct, refreshURL, returnURL string) (string, error)
	Account(ctx context.Context, acct string) (payment.ConnectAccount, error)
	PayOut(ctx context.Context, key payment.Key, acct string, amountMinor int64, currency, note string) (payment.Result, error)
}

// payoutAccountFor answers the console's question: can this person be paid?
func (s *Server) payoutAccountFor(worker string) api.PayoutState {
	if s.Rail == nil {
		return api.PayoutState{Unavailable: true}
	}
	acct, ok := s.PayoutAccounts.Get(worker)
	if !ok {
		return api.PayoutState{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	a, err := s.Rail.Account(ctx, acct)
	if err != nil {
		// The rail being unreachable is not the worker's problem to solve, and
		// telling them to "add a payout account" when they already have one is
		// the kind of wrong instruction that makes people give up.
		return api.PayoutState{Connected: true, Needs: []string{"checking with the payment provider"}}
	}
	return api.PayoutState{
		Connected: true,
		Ready:     a.Ready(),
		Needs:     humanizeNeeds(a.Needs, a.Disabled),
	}
}

// humanizeNeeds turns the rail's field names into things a person can act on.
//
// "individual.verification.document" is precise and means nothing to somebody
// standing in a kitchen wondering why they have not been paid.
func humanizeNeeds(needs []string, disabled string) []string {
	if disabled != "" {
		switch disabled {
		case "requirements.past_due", "requirements.pending_verification":
			// Falls through to the field list, which is more specific.
		default:
			return []string{"the payment provider has paused this account"}
		}
	}
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, n := range needs {
		// Three is the most a sentence can carry. A fresh account comes back
		// with ten or more requirements, and listing them all produces a
		// paragraph that reads as an obstacle course — when in fact the
		// provider collects the whole set on one page. Name enough to show
		// what kind of thing is wanted, then stop.
		if len(out) >= 3 {
			add("a few other details")
			break
		}
		switch {
		case contains(n, "external_account"):
			add("a bank account to pay into")
		case contains(n, "verification.document"):
			add("a photo of your ID")
		case contains(n, "id_number"), contains(n, "ssn_last_4"):
			add("your tax identification number")
		case contains(n, "dob"):
			add("your date of birth")
		case contains(n, "address"):
			add("your address")
		case contains(n, "tos_acceptance"):
			add("accepting the payment provider's terms")
		case contains(n, "email"), contains(n, "phone"):
			add("contact details")
		case contains(n, "name"):
			add("your legal name")
		case contains(n, "business_profile"), contains(n, "mcc"), contains(n, "url"):
			add("what kind of work you do")
		default:
			add("a few other details")
		}
	}
	return out
}

func contains(s, sub string) bool {
	return len(sub) <= len(s) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

// payOut sends what a person is owed, and records it in the ledger.
//
// Ledger first would double-pay on a rail failure; rail first would lose the
// record if the process died between the two. The rail call is idempotent on a
// key derived from the person and the amount, so the recovery path is to call
// again — which pays once.
func (s *Server) payOut(ctx context.Context, person string, amountMinor int64, currency string) (string, error) {
	if s.Rail == nil {
		return "", fmt.Errorf("payouts are not available on this exchange")
	}
	acct, ok := s.PayoutAccounts.Get(person)
	if !ok {
		return "", fmt.Errorf("no payout account connected")
	}
	// The key must be a function of ledger state, never of the clock.
	//
	// It was the hour, which defeated the very protection the comment below
	// claims: if the transfer succeeded and the ledger write failed, the
	// balance still showed as owed, and the next hourly sweep derived a
	// *different* key for the same money — so the rail treated it as a new
	// transfer and paid twice.
	//
	// Cumulative credited is stable across retries (a failed ledger write does
	// not change it) and moves after every recorded payout, so a genuine
	// second payout of the same amount still gets its own key.
	var credited int64
	if s.Ledger != nil {
		var err error
		credited, err = s.Ledger.Credited(ctx, ledger.PayableOf(person), currency)
		if err != nil {
			return "", fmt.Errorf("could not read your earnings history")
		}
	}
	key := payment.DeriveKey("payout",
		fmt.Sprintf("%s:%d:%s:%d", person, amountMinor, currency, credited))

	res, err := s.Rail.PayOut(ctx, key, acct, amountMinor, currency, "Lamdis earnings")
	if err != nil {
		return "", err
	}
	s.notifyPaid(person, amountMinor, currency, acct, res.Ref)
	if s.Ledger != nil {
		if _, err := s.Ledger.Payout(ctx, string(key), person, amountMinor, currency, res.Ref); err != nil {
			// The money left. Failing to record it would let it leave again.
			log.Printf("payout: PAID %s %d %s (ref %s) but ledger write failed: %v",
				person, amountMinor, currency, res.Ref, err)
			return res.Ref, fmt.Errorf("paid, but the record failed; support has been notified")
		}
	}
	return res.Ref, nil
}

// SweepPayouts pays everyone who is over the threshold and able to receive it.
//
// Run on a timer. A worker should not have to ask for money they have already
// earned, and making them press a button is a way of keeping balances that
// nobody presses the button for.
func (s *Server) SweepPayouts(ctx context.Context) (paid int, total int64) {
	if s.Rail == nil || s.Ledger == nil {
		return 0, 0
	}
	now := s.now()
	for _, person := range s.PayoutAccounts.People() {
		owed, err := s.Ledger.Balance(ctx, ledger.PayableOf(person), "USD")
		if err != nil || owed <= 0 {
			continue
		}
		// The ledger says what is owed; the holdbacks say what is safe to
		// send. Sweeping on the balance alone is what let money reach a bank
		// within the hour, before the buyer had seen a photograph — while the
		// withdraw route told them a dispute window protected exactly this.
		if s.Holdbacks != nil {
			clear := s.Holdbacks.Available(person, now)
			if clear < owed {
				owed = clear
			}
		}
		if owed < PayoutThresholdMinor {
			continue
		}
		acct, ok := s.PayoutAccounts.Get(person)
		if !ok {
			continue
		}
		a, err := s.Rail.Account(ctx, acct)
		if err != nil || !a.Ready() {
			continue
		}
		if _, err := s.payOut(ctx, person, owed, "USD"); err != nil {
			log.Printf("payout: %s owed %d: %v", person, owed, err)
			continue
		}
		if s.Holdbacks != nil {
			s.Holdbacks.MarkPaid(person, now)
		}
		paid++
		total += owed
	}
	return paid, total
}

// People lists everyone with a connected account.
func (p *PayoutAccounts) People() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.by))
	for k := range p.by {
		out = append(out, k)
	}
	return out
}

// PayoutServer is the worker-facing surface: connect an account, see where it
// stands.
type PayoutServer struct {
	Server  *Server
	Workers *api.Workers
	BaseURL string
}

func (ps *PayoutServer) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/payout/connect", ps.handleConnect)
	mux.HandleFunc("GET /v1/payout", ps.handleStatus)
	mux.HandleFunc("GET /payout/done", ps.handleReturn)
	mux.HandleFunc("POST /v1/balance/confirm", ps.handleConfirmDeposit)
	mux.HandleFunc("POST /v1/payout/now", ps.handlePayoutNow)
}

func (ps *PayoutServer) worker(r *http.Request) (*api.Worker, []byte, bool) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	w, err := ps.Workers.Authenticate(r, body, ps.Server.now())
	if err != nil {
		return nil, body, false
	}
	return w, body, true
}

func (ps *PayoutServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	worker, _, ok := ps.worker(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "sign in first")
		return
	}
	st := ps.Server.payoutAccountFor(worker.ID)
	owed := int64(0)
	if ps.Server.Ledger != nil {
		owed, _ = ps.Server.Ledger.Balance(r.Context(), ledger.PayableOf(worker.ID), "USD")
	}
	out := map[string]any{
		"payout": st, "owed_minor": owed, "currency": "USD",
		"threshold_minor": PayoutThresholdMinor,
		"tax":             ps.Server.TaxStatusFor(r.Context(), worker.ID),
	}
	if ps.Server.Holdbacks != nil {
		now := ps.Server.now()
		out["clear_minor"] = ps.Server.Holdbacks.Available(worker.ID, now)
		if pending := ps.Server.Holdbacks.Pending(worker.ID, now); len(pending) > 0 {
			var waiting []map[string]any
			for _, h := range pending {
				row := map[string]any{
					"job": h.Job, "amount_minor": h.AmountMinor,
				}
				if h.Held {
					row["status"] = "the buyer has raised a problem"
					row["reason"] = h.Reason
				} else {
					row["status"] = "waiting out the buyer's review window"
					row["clears"] = h.ReleaseAt.Format(time.RFC3339)
				}
				waiting = append(waiting, row)
			}
			out["waiting"] = waiting
		}
	}
	writeJSONResponse(w, out)
}

func (ps *PayoutServer) handleConnect(w http.ResponseWriter, r *http.Request) {
	worker, _, ok := ps.worker(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "sign in first")
		return
	}
	if ps.Server.Rail == nil {
		writeError(w, http.StatusServiceUnavailable,
			"this exchange has no payment rail configured yet")
		return
	}
	acct, have := ps.Server.PayoutAccounts.Get(worker.ID)
	if !have {
		// Our mapping lives on storage a redeploy can wipe; the rail's does
		// not. Rebuild from the rail before opening anything, or somebody who
		// already finished verification gets a second, unverified account and
		// the first is stranded.
		//
		// Hydrated once for everybody rather than scanned per person: this is
		// a slow call, and it used to sit on the critical path of a button.
		ps.Server.PayoutAccounts.Hydrate(r.Context(), ps.Server.Rail)
		acct, have = ps.Server.PayoutAccounts.Get(worker.ID)
	}
	if !have {
		// A company is onboarded as a company, so the rail asks it for an EIN
		// rather than asking an employee for their social security number.
		kind, legal := "individual", ""
		if ps.Server.Suppliers != nil {
			if sup, ok := ps.Server.Suppliers.Get(worker.ID); ok && sup.Kind == api.KindCompany {
				kind, legal = "company", sup.LegalName
			}
		}
		created, err := ps.Server.Rail.CreateAccountAs(r.Context(), worker.ID, worker.Email, kind, legal)
		if err != nil {
			writeError(w, http.StatusBadGateway,
				"the payment provider could not open an account just now")
			return
		}
		if err := ps.Server.PayoutAccounts.Put(worker.ID, created.ID); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		acct = created.ID
	}
	base := strings.TrimSuffix(ps.BaseURL, "/")
	link, err := ps.Server.Rail.AccountLink(r.Context(), acct,
		base+"/console", base+"/payout/done")
	if err != nil {
		writeError(w, http.StatusBadGateway,
			"the payment provider could not start verification just now")
		return
	}
	// The link goes to the client rather than a redirect, so the console can
	// open it and know it was opened.
	writeJSONResponse(w, map[string]any{"url": link})
}

// handleReturn is where the rail sends the worker when they finish.
//
// It asserts nothing about whether they succeeded — the rail returns people
// here whether they completed the form or abandoned it, so claiming success
// would be a lie a quarter of the time. The console asks the rail directly.
func (ps *PayoutServer) handleReturn(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/console?payout=returned", http.StatusSeeOther)
}

// modeOf says whether the rail is moving real money, so the boot banner cannot
// be ambiguous about it.
func modeOf(s *payment.Stripe) string {
	if s.Live() {
		return "LIVE — real money"
	}
	return "test mode"
}

// ConfirmDeposit credits a balance once the rail says the money arrived.
//
// Asked of the rail rather than believed from the browser. A person who lands
// on the success page has not necessarily paid — they may have opened the URL
// directly — and crediting on a redirect is how a balance gets funded for
// free. Idempotent on the session id, so a refresh credits once.
func (ps *PayoutServer) handleConfirmDeposit(w http.ResponseWriter, r *http.Request) {
	worker, _, ok := ps.worker(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "sign in first")
		return
	}
	session := r.URL.Query().Get("session")
	if session == "" {
		writeError(w, http.StatusBadRequest, "which payment?")
		return
	}
	rail, isStripe := ps.Server.Rail.(*payment.Stripe)
	if !isStripe {
		writeError(w, http.StatusServiceUnavailable, "no payment rail")
		return
	}
	paid, amount, person, err := rail.CheckoutPaid(r.Context(), session)
	if err != nil {
		writeError(w, http.StatusBadGateway, "could not check with the payment provider")
		return
	}
	if !paid {
		writeJSONResponse(w, map[string]any{
			"credited": false,
			"status":   "the payment has not cleared yet",
		})
		return
	}
	// The session names who paid. Trusting the caller instead would let anyone
	// credit their own balance with somebody else's session id.
	if person != worker.ID {
		writeError(w, http.StatusForbidden, "that payment belongs to another account")
		return
	}
	if err := ps.Server.CreditDeposit(r.Context(), session, person, amount, "USD", session); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSONResponse(w, map[string]any{
		"credited": true, "amount_minor": amount, "currency": "USD",
	})
}

// WarmPayoutAccounts rebuilds the person-to-account mapping in the background
// at startup.
//
// The deployment's storage does not survive a restart, so after every deploy
// the mapping is empty and the first person to open payout settings would pay
// for the rebuild while staring at a button. Doing it at boot means nobody
// does.
func (s *Server) WarmPayoutAccounts(ctx context.Context) {
	if s.Rail == nil || s.PayoutAccounts == nil {
		return
	}
	go func() {
		start := s.now()
		s.PayoutAccounts.Hydrate(ctx, s.Rail)
		if n := len(s.PayoutAccounts.People()); n > 0 {
			log.Printf("payout: recovered %d payout account(s) from the rail in %v",
				n, s.now().Sub(start).Round(time.Millisecond))
		}
	}()
}

// StartPayoutSweeper pays out on a timer.
//
// Earned money should not wait on somebody remembering to press a button —
// least of all the person who is owed it.
func (s *Server) StartPayoutSweeper(ctx context.Context, every time.Duration) {
	if s.Rail == nil {
		return
	}
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				// Objections that were never carried through.
				//
				// Runs before the payout sweep, so money freed here goes out on
				// the same pass rather than waiting another hour. A worker owed
				// a month's income should not wait on our scheduling.
				if s.Holdbacks != nil {
					if freed := s.Holdbacks.ExpireHolds(s.now()); len(freed) > 0 {
						log.Printf("disputes   %d objection(s) lapsed undecided; "+
							"earnings released to the worker: %v", len(freed), freed)
						s.notifyHoldsLapsed(freed)
					}
				}
				if n, total := s.SweepPayouts(ctx); n > 0 {
					log.Printf("payout: sent %d payment(s) totalling %d minor units", n, total)
				}
			}
		}
	}()
}

// handlePayoutNow sends what somebody is owed even though it is under the
// threshold, because they asked.
//
// The loudest complaint about the first marketplace of this shape was a worker
// who earned five dollars and could not get it out. Our threshold exists for a
// good reason — a flat transfer fee would eat a small balance — but a
// threshold somebody cannot override is indistinguishable from not being paid.
// The fee is theirs to weigh, not ours to impose.
func (ps *PayoutServer) handlePayoutNow(w http.ResponseWriter, r *http.Request) {
	worker, _, ok := ps.worker(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "sign in first")
		return
	}
	s := ps.Server
	if s.Rail == nil || s.Ledger == nil {
		writeError(w, http.StatusServiceUnavailable,
			"payouts are not switched on for this exchange")
		return
	}
	now := s.now()
	owed, err := s.Ledger.Balance(r.Context(), ledger.PayableOf(worker.ID), "USD")
	if err != nil || owed <= 0 {
		writeJSONResponse(w, map[string]any{
			"sent": false, "status": "there is nothing owed to you right now"})
		return
	}
	clear := owed
	if s.Holdbacks != nil {
		if c := s.Holdbacks.Available(worker.ID, now); c < clear {
			clear = c
		}
	}
	if clear <= 0 {
		// Held money is the buyer's review window, not a threshold, and that
		// one is not the worker's to waive.
		writeJSONResponse(w, map[string]any{
			"sent": false,
			"status": "your earnings are still inside the buyer's review window; " +
				"they clear on their own",
			"waiting": s.Holdbacks.Pending(worker.ID, now),
		})
		return
	}
	ref, err := s.payOut(r.Context(), worker.ID, clear, "USD")
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if s.Holdbacks != nil {
		s.Holdbacks.MarkPaid(worker.ID, now)
	}
	writeJSONResponse(w, map[string]any{
		"sent": true, "amount_minor": clear, "currency": "USD", "reference": ref,
		"note": "sent below the usual threshold because you asked; the " +
			"provider's transfer fee comes out of it",
	})
}
