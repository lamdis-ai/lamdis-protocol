package exchange

// The gateless way in.
//
// Three gates stood between an agent and its first job: a signed-in human, a
// balance topped up in advance, and an issued key. Each one lost the people who
// were only ever going to spend ten minutes finding out whether this was
// interesting — which is everybody, the first time.
//
// This file removes all three for a first job. Anybody may POST /v1/tasks with
// no credential at all. The job is created but not listed; the reply carries a
// pay link and a token. The pay link authorises the buyer's card for the job's
// ceiling — nothing is taken. When the authorisation lands, the job goes on the
// board funded exactly as a balance-funded job would be, and the card is
// charged once, at the end, for what the job actually paid out. The token is
// how the poster follows the job afterwards: it authenticates as the owner of
// that job and nothing else.
//
// The money posture is the same as agentic checkout, and better than the
// balance path: an uncaptured authorisation is not held by us, a job nobody
// takes needs no refund, and the only capture goes through settle().

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/payment"
)

const guestPrefix = "guest:"

// PendingTTL is how long an unpaid job waits for its authorisation.
const PendingTTL = 24 * time.Hour

var tokenAlphabet = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)

// guestOwner is the principal a card-funded job belongs to. Nothing can sign
// in as it and no key maps to it, so its balance is never spendable.
func guestOwner(job string) string { return guestPrefix + job }

func isGuest(person string) bool { return strings.HasPrefix(person, guestPrefix) }

// guestJob is the job a guest principal belongs to.
func guestJob(person string) string { return strings.TrimPrefix(person, guestPrefix) }

// BuyerToken derives the capability that lets whoever posted a job follow it.
//
// Derived, not stored: the exchange key is the only secret, and the token for
// a job that does not exist is as good as random. The purpose string keeps it
// disjoint from a worker's capability for the same job.
func (s *Server) BuyerToken(job string) string {
	mac := hmac.New(sha256.New, []byte(s.Key))
	mac.Write([]byte("lamdis-buyer-token:" + job))
	return "lbt_" + tokenAlphabet.EncodeToString(mac.Sum(nil))[:26]
}

// tokenFrom reads a buyer token from the request: a bearer, or ?t= for pages
// a browser opens from a link.
func tokenFrom(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("Authorization")); len(v) > 7 && strings.EqualFold(v[:7], "bearer ") {
		return strings.TrimSpace(v[7:])
	}
	return strings.TrimSpace(r.URL.Query().Get("t"))
}

// guestFromToken authenticates a token as the owner of exactly the job in the
// path. A token for job A presented on job B is not a credential.
func (s *Server) guestFromToken(r *http.Request, job string) (string, bool) {
	tok := tokenFrom(r)
	if job == "" || !strings.HasPrefix(tok, "lbt_") {
		return "", false
	}
	if !hmac.Equal([]byte(tok), []byte(s.BuyerToken(job))) {
		return "", false
	}
	return guestOwner(job), true
}

// pendingJob is a job that exists but is not funded yet.
type pendingJob struct {
	L       *api.Listing
	Amount  int64
	Session string
	PayAt   string
	Created time.Time
}

// stagePending parks a guest job until its card is authorised and describes
// what the poster has to do next.
func (s *Server) stagePending(ctx context.Context, l *api.Listing) (map[string]any, error) {
	amount := MaxPayoutFor(l)
	base := strings.TrimSuffix(s.BaseURL, "/")
	p := &pendingJob{L: l, Amount: amount, Created: s.now()}
	if s.Authorize != nil {
		session, payAt, err := s.Authorize(ctx, l.Job, amount, l.Currency,
			base+"/paid/"+l.Job+"?session={CHECKOUT_SESSION_ID}",
			base+"/j/"+l.Job)
		if err != nil {
			return nil, fmt.Errorf("could not open a payment link: %w", err)
		}
		p.Session, p.PayAt = session, payAt
	} else {
		// No rail. The link goes to a page that says so, rather than to a
		// dead end that looks like a bug.
		p.PayAt = base + "/pay/" + l.Job
	}
	s.mu.Lock()
	if s.pending == nil {
		s.pending = map[string]*pendingJob{}
	}
	s.pending[l.Job] = p
	s.mu.Unlock()
	token := s.BuyerToken(l.Job)
	out := map[string]any{
		"job": l.Job, "kind": l.Kind, "status": "awaiting_payment",
		"pay_at": p.PayAt, "token": token,
		"amount_minor": amount, "currency": l.Currency,
		"expires_at": p.Created.Add(PendingTTL).Format(time.RFC3339),
		"status_url": base + "/v1/jobs/" + l.Job,
		"watch":      base + "/my/" + l.Job + "?t=" + token,
		"note": "Send the person the pay link. Their card is authorised for the " +
			"ceiling, not charged; the job goes on the board the moment that " +
			"lands, and the card is charged once, at the end, for exactly what " +
			"was paid out on proof. Keep the token: it is how you follow this job.",
	}
	// A wallet instead of a card: the exact USDC amount that names this job.
	if q := s.usdcQuote(l.Job, amount); q != nil {
		out["pay_usdc"] = q
	}
	return out, nil
}

// pendingFor returns a parked job.
func (s *Server) pendingFor(job string) (*pendingJob, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pending[job]
	return p, ok
}

// FundFromCard turns an authorisation into escrow and lists the job.
//
// Idempotent on the session: a webhook and a browser return both call this,
// in either order, and the job is listed once. The ledger sees a top-up of the
// authorised amount to the job's own guest principal and an immediate hold of
// all of it, so everything downstream — settle, holdbacks, receipts, the
// worker's payout — is exactly the balance-funded path.
func (s *Server) FundFromCard(ctx context.Context, job, session, intent string, amountMinor int64, email string) error {
	p, ok := s.pendingFor(job)
	if !ok {
		if _, live := s.Board.Get(job); live {
			return nil
		}
		return fmt.Errorf("no job is waiting for payment under %s", job)
	}
	if p.Session != "" && session != "" && p.Session != session {
		return fmt.Errorf("that payment belongs to a different checkout")
	}
	if amountMinor > 0 && amountMinor < p.Amount {
		return fmt.Errorf("the authorisation covers %d of the %d this job could pay out", amountMinor, p.Amount)
	}
	owner := guestOwner(job)
	l := p.L
	l.Owner = owner
	l.Funding = &api.Funding{Kind: "card", Intent: intent, Session: session,
		AuthorizedMinor: p.Amount, Email: email}
	if s.Ledger != nil {
		key := "card-auth:" + job
		if done, err := s.Ledger.Applied(ctx, key); err != nil {
			return err
		} else if !done {
			if _, err := s.Ledger.Topup(ctx, key, owner, p.Amount, l.Currency, intent); err != nil {
				return err
			}
		}
		if done, _ := s.Ledger.Applied(ctx, "hold-"+job); !done {
			if _, err := s.Ledger.Hold(ctx, "hold-"+job, job, owner, p.Amount, l.Currency); err != nil {
				return err
			}
		}
	}
	if err := s.Board.Post(l); err != nil {
		// Authorised but unpostable: let the card go rather than sit on it.
		if s.Charges != nil && intent != "" {
			_, _ = s.Charges.Release(ctx, payment.Request{
				Key: payment.DeriveKey("card-release", job), HoldRef: intent, Outcome: job})
		}
		return err
	}
	s.mu.Lock()
	s.buyers[job] = owner
	s.saveBuyersLocked()
	delete(s.pending, job)
	s.mu.Unlock()
	s.tell("guest-live:"+job, owner, "Your job is on the board",
		"Your card is authorised and the job is live. Nothing is charged until "+
			"the outcome is proven.\n\nFollow it here:\n"+
			strings.TrimSuffix(s.BaseURL, "/")+"/my/"+job+"?t="+s.BuyerToken(job)+
			"\n\nThat link is the only key to this job. Keep it.")
	return nil
}

// settleCard closes the card side once a job can no longer be worked.
//
// The ledger has already released the remainder into the guest principal's
// balance; that balance is not real money, so it is withdrawn again here, and
// the card is captured for the difference between what was authorised and what
// came back — which is exactly what the job paid out. Nothing paid, nothing
// captured.
func (s *Server) settleCard(ctx context.Context, l *api.Listing, remainder int64) {
	f := l.Funding
	if f != nil && f.Kind == "usdc" {
		s.settleChain(ctx, l, remainder)
		return
	}
	if f == nil || f.Kind != "card" || f.Settled {
		return
	}
	f.Settled = true
	if remainder > 0 && s.Ledger != nil {
		if _, err := s.Ledger.Withdraw(ctx, "card-void:"+l.Job, guestOwner(l.Job),
			remainder, l.Currency, f.Intent); err != nil {
			log.Printf("card       could not void the remainder for %s: %v", l.Job, err)
		}
	}
	paid := f.AuthorizedMinor - remainder
	if s.Charges == nil || f.Intent == "" {
		return
	}
	if paid > 0 {
		if _, err := s.Charges.Capture(ctx, payment.Request{
			Key: payment.DeriveKey("card-capture", l.Job), HoldRef: f.Intent,
			AmountMinor: paid, Currency: l.Currency, Outcome: l.Job,
		}); err != nil {
			// Left authorised: it lapses on its own and costs the buyer
			// nothing, which is the safe direction to fail in.
			log.Printf("card       capture failed for %s: %v", l.Job, err)
		}
		return
	}
	if _, err := s.Charges.Release(ctx, payment.Request{
		Key: payment.DeriveKey("card-release", l.Job), HoldRef: f.Intent, Outcome: l.Job,
		AmountMinor: f.AuthorizedMinor, Currency: l.Currency,
	}); err != nil {
		log.Printf("card       release failed for %s: %v", l.Job, err)
	}
}

// sweepPending drops jobs whose payment never came.
func (s *Server) sweepPending() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for job, p := range s.pending {
		if s.now().Sub(p.Created) > PendingTTL {
			delete(s.pending, job)
			n++
		}
	}
	return n
}

// registerGuest mounts the pages a poster without an account lands on.
func (s *Server) registerGuest(mux *http.ServeMux) {
	mux.HandleFunc("GET /pay/{job}", s.handlePayPage)
	mux.HandleFunc("GET /paid/{job}", s.handlePaidReturn)
	mux.HandleFunc("GET /my/{job}", s.handleMyJob)
	mux.HandleFunc("GET /post", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, api.PostPage())
	})
}

func (s *Server) handlePayPage(w http.ResponseWriter, r *http.Request) {
	job := r.PathValue("job")
	p, ok := s.pendingFor(job)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, api.GuestNotice("No job is waiting for payment",
			"Either it was paid and is on the board, or it waited longer than a day and was dropped."))
		return
	}
	if p.Session != "" && p.PayAt != "" {
		http.Redirect(w, r, p.PayAt, http.StatusFound)
		return
	}
	fmt.Fprint(w, api.GuestNotice("This exchange cannot take card payments yet",
		"The job is saved for a day. When a payment rail is configured this link will open the checkout."))
}

// handlePaidReturn is where the checkout sends the payer back. The rail is
// asked, not the browser: landing here is not proof of anything.
func (s *Server) handlePaidReturn(w http.ResponseWriter, r *http.Request) {
	job := r.PathValue("job")
	session := r.URL.Query().Get("session")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if l, live := s.Board.Get(job); live {
		// Already listed. The token goes only to whoever holds the checkout
		// session that paid for it: job ids are public, and a redirect that
		// handed the token to anybody who typed the URL would make every
		// operator the buyer of every card-funded job.
		if l.Funding == nil || session == "" || l.Funding.Session != session {
			fmt.Fprint(w, api.GuestNotice("This job is already on the board",
				"Use the link from your payment confirmation to follow it."))
			return
		}
		http.Redirect(w, r, "/my/"+job+"?t="+s.BuyerToken(job), http.StatusFound)
		return
	}
	{
		if s.Authorized == nil || session == "" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, api.GuestNotice("Payment not confirmed", "No checkout session to check."))
			return
		}
		ok, intent, amount, email, err := s.Authorized(r.Context(), session)
		if err != nil || !ok {
			fmt.Fprint(w, api.GuestNotice("Payment not confirmed yet",
				"The rail has not confirmed the authorisation. If you completed the checkout, reload in a moment."))
			return
		}
		if err := s.FundFromCard(r.Context(), job, session, intent, amount, email); err != nil {
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, api.GuestNotice("Could not list the job", err.Error()))
			return
		}
	}
	http.Redirect(w, r, "/my/"+job+"?t="+s.BuyerToken(job), http.StatusFound)
}

func (s *Server) handleMyJob(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, api.MyJobPage(r.PathValue("job")))
}

// guestSentinel is the principal handleCreateTask is handed when nobody is
// signed in at all; it becomes the job's own guest principal once the job id
// exists.
const guestSentinel = "guest"

// releaseIfDoneNow settles a job's escrow as though it had just finished,
// for tests that drive the card side without a worker.
func (s *Server) releaseIfDoneNow(ctx context.Context, l *api.Listing) error {
	l.Expires = s.now().Add(-time.Second)
	return s.releaseIfDone(ctx, l)
}

// Anonymous posting is free, so it is rate-limited: each job parks a listing
// and, with a rail, opens a checkout session. Per address per hour, and a
// ceiling on how many unpaid jobs the exchange will park at once.
const (
	guestPerHour   = 12
	guestMaxParked = 500
)

// guestAllowed says whether this caller may park another unpaid job.
func (s *Server) guestAllowed(r *http.Request) error {
	ip := r.Header.Get("X-Forwarded-For")
	if i := strings.IndexByte(ip, ','); i >= 0 {
		ip = ip[:i]
	}
	ip = strings.TrimSpace(ip)
	if ip == "" {
		ip = r.RemoteAddr
		if i := strings.LastIndexByte(ip, ':'); i >= 0 {
			ip = ip[:i]
		}
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pending) >= guestMaxParked {
		return fmt.Errorf("too many unpaid jobs are waiting; try again in a while")
	}
	if s.guestSeen == nil {
		s.guestSeen = map[string][]time.Time{}
	}
	recent := s.guestSeen[ip][:0]
	for _, t := range s.guestSeen[ip] {
		if now.Sub(t) < time.Hour {
			recent = append(recent, t)
		}
	}
	if len(recent) >= guestPerHour {
		s.guestSeen[ip] = recent
		return fmt.Errorf("that is enough unpaid jobs from one place for an hour; pay for one, or sign in")
	}
	s.guestSeen[ip] = append(recent, now)
	return nil
}
