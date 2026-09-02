package exchange

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/ledger"
)

// A mailbox that remembers, so a test can read what would have been sent.
type fakeMail struct {
	mu   sync.Mutex
	sent []sentMail
	fail bool
}

type sentMail struct{ to, subject, body string }

func (f *fakeMail) Send(_ context.Context, to, subject, body string) error {
	if f.fail {
		return fmt.Errorf("mail: down")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, sentMail{to, subject, body})
	return nil
}

// mailed waits for the background sends and returns them.
func mailed(s *Server, f *fakeMail) []sentMail {
	s.notify.wg.Wait()
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sentMail(nil), f.sent...)
}

func to(mails []sentMail, addr string) []sentMail {
	var out []sentMail
	for _, m := range mails {
		if m.to == addr {
			out = append(out, m)
		}
	}
	return out
}

// srvWithMail is an exchange where both sides can be reached.
func srvWithMail(t *testing.T) (*Server, context.Context, *fakeMail) {
	t.Helper()
	s, ctx := srvWithMoney(t)
	f := &fakeMail{}
	s.Mail = f
	s.Watches.Set("worker", "worker@example.com", false)
	s.Watches.Set("buyer", "buyer@example.com", false)
	return s, ctx, f
}

func observeJob() *api.Listing {
	return &api.Listing{
		Job: "obs_1", Kind: api.KindObserve, Title: "is the sign up",
		PayMinor: 500, BonusMinor: 1800, Currency: "USD", Slots: 1,
		Expires: time.Now().Add(time.Hour), Owner: "buyer",
	}
}

// Settlement is the moment both sides have been waiting for, and until now
// neither was told.
func TestSettlementTellsBothSidesOnce(t *testing.T) {
	s, ctx, f := srvWithMail(t)
	l := observeJob()
	fund(t, s, ctx, l)

	sub := api.Submission{Job: "obs_1", Holder: "cap1", Verified: true, Finding: true}
	if err := s.settle(ctx, "obs_1", sub, "worker"); err != nil {
		t.Fatal(err)
	}
	got := mailed(s, f)
	if len(got) != 2 {
		t.Fatalf("settlement sent %d emails, want one to each side: %+v", len(got), got)
	}
	w := to(got, "worker@example.com")
	if len(w) != 1 {
		t.Fatalf("the worker got %d emails", len(w))
	}
	for _, want := range []string{"$23.00", "credited", "held until", "https://example.test/console"} {
		if !strings.Contains(w[0].body, want) {
			t.Errorf("the worker's email lacks %q:\n%s", want, w[0].body)
		}
	}
	b := to(got, "buyer@example.com")
	if len(b) != 1 {
		t.Fatalf("the buyer got %d emails", len(b))
	}
	for _, want := range []string{
		"passed verification", "came back yes", "$23.00", "release",
		"https://example.test/v1/jobs/obs_1/receipt", "https://example.test/console",
	} {
		if !strings.Contains(b[0].body, want) {
			t.Errorf("the buyer's email lacks %q:\n%s", want, b[0].body)
		}
	}
	for _, m := range got {
		if strings.Contains(m.body, "!") || strings.Contains(m.subject, "!") {
			t.Errorf("an exclamation mark crept in: %q", m.subject)
		}
	}

	// A retry of the same settlement says nothing new.
	if err := s.settle(ctx, "obs_1", sub, "worker"); err != nil {
		t.Fatal(err)
	}
	if again := mailed(s, f); len(again) != 2 {
		t.Fatalf("a retried settlement sent more email: %d", len(again))
	}
}

// A refusal has a reason, and the worker has to see it after they have walked
// away from the page.
func TestRejectionCarriesTheReason(t *testing.T) {
	s, ctx, f := srvWithMail(t)
	l := &api.Listing{
		Job: "do_1", Kind: api.KindDo, Title: "put the sign up",
		Instructions: "fix it in the window", PayMinor: 4000, Currency: "USD", Slots: 1,
		Expires: time.Now().Add(time.Hour), Owner: "buyer",
	}
	fund(t, s, ctx, l)
	secret, _, err := s.Board.Claim("do_1", "worker")
	if err != nil {
		t.Fatal(err)
	}
	// A capability names itself by the hash of its secret.
	sum := sha256.Sum256([]byte(secret))
	holder := hex.EncodeToString(sum[:])
	const why = "the code 7K3M9P was not legible in any photo"
	s.Verify = func(sub api.Submission, _ func(string) ([]byte, bool)) (api.Submission, error) {
		return sub, fmt.Errorf("%s", why)
	}
	sub := api.Submission{Job: "do_1", Holder: holder}
	if _, err := s.acceptEvidence(sub); err == nil {
		t.Fatal("the verifier's refusal was swallowed")
	}
	got := mailed(s, f)
	w := to(got, "worker@example.com")
	if len(w) != 1 {
		t.Fatalf("the worker got %d emails: %+v", len(w), got)
	}
	if !strings.Contains(w[0].body, why) {
		t.Errorf("the reason is missing:\n%s", w[0].body)
	}
	if !strings.Contains(w[0].body, "https://example.test/w/do_1#") {
		t.Errorf("no way back to the job:\n%s", w[0].body)
	}
	b := to(got, "buyer@example.com")
	if len(b) != 1 || !strings.Contains(b[0].body, why) {
		t.Errorf("the buyer was not told what failed: %+v", b)
	}

	// The same refusal again is not news; a different one is.
	if _, err := s.acceptEvidence(sub); err == nil {
		t.Fatal("expected a refusal")
	}
	if n := len(mailed(s, f)); n != 2 {
		t.Fatalf("a repeated refusal sent more email: %d", n)
	}
	s.Verify = func(sub api.Submission, _ func(string) ([]byte, bool)) (api.Submission, error) {
		return sub, fmt.Errorf("every photo that recorded a location was taken outside the area for this job")
	}
	s.acceptEvidence(sub)
	if n := len(to(mailed(s, f), "worker@example.com")); n != 2 {
		t.Fatalf("a new reason was not sent: %d", n)
	}
}

// An objection freezes somebody's money. They hear what was said and by when
// it ends.
func TestHoldTellsTheWorkerTheGroundAndTheDeadline(t *testing.T) {
	s, ctx, f := srvWithMail(t)
	l := observeJob()
	fund(t, s, ctx, l)
	sub := api.Submission{Job: "obs_1", Holder: "cap1", Verified: true, Finding: true}
	if err := s.settle(ctx, "obs_1", sub, "worker"); err != nil {
		t.Fatal(err)
	}
	mailed(s, f)

	until := time.Date(2026, 9, 9, 15, 0, 0, 0, time.UTC)
	reason := GroundLabel(GroundNotDone) + ": the sign is still on the ground"
	if n := s.Holdbacks.Hold("obs_1", reason, until); n != 1 {
		t.Fatalf("held %d", n)
	}
	s.notifyHeld("obs_1", reason, until)
	s.notifyHeld("obs_1", reason, until)

	w := to(mailed(s, f), "worker@example.com")
	if len(w) != 2 {
		t.Fatalf("the worker got %d emails, want the settlement and one objection", len(w))
	}
	body := w[1].body
	for _, want := range []string{
		"the work described was not done", "still on the ground",
		"Wed 9 Sep at 3:00pm", "$23.00", "goes to you",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the objection email lacks %q:\n%s", want, body)
		}
	}

	// The objection lapses undecided, and the worker hears that too.
	freed := s.Holdbacks.ExpireHolds(until.Add(time.Hour))
	s.notifyHoldsLapsed(freed)
	w = to(mailed(s, f), "worker@example.com")
	if len(w) != 3 || !strings.Contains(w[2].body, "lapsed in your favour") {
		t.Fatalf("the lapse was not reported: %+v", w)
	}
}

// Nothing goes out when mail is off, and a mail failure changes nothing about
// the money.
func TestNoMailMeansNoMailAndNoHarm(t *testing.T) {
	s, ctx := srvWithMoney(t)
	s.Watches.Set("worker", "worker@example.com", false)
	l := observeJob()
	fund(t, s, ctx, l)
	sub := api.Submission{Job: "obs_1", Holder: "cap1", Verified: true, Finding: true}
	if err := s.settle(ctx, "obs_1", sub, "worker"); err != nil {
		t.Fatal(err)
	}
	s.notify.wg.Wait()
	if _, told := s.notify.seen.Load("settled:obs_1:cap1#0"); told {
		t.Fatal("something was recorded as sent with no mailer configured")
	}

	// Mail configured but down: the credit still lands.
	s2, ctx2 := srvWithMoney(t)
	f := &fakeMail{fail: true}
	s2.Mail = f
	s2.Watches.Set("worker", "worker@example.com", false)
	fund(t, s2, ctx2, observeJob())
	if err := s2.settle(ctx2, "obs_1", sub, "worker"); err != nil {
		t.Fatalf("a mail failure broke settlement: %v", err)
	}
	if pay, _ := s2.Ledger.Balance(ctx2, ledger.PayableOf("worker"), "USD"); pay != net(2300) {
		t.Fatalf("worker credited %d", pay)
	}
	if got := mailed(s2, f); len(got) != 0 {
		t.Fatalf("a failing mailer recorded sends: %+v", got)
	}

	// Somebody who asked for quiet gets quiet.
	s3, ctx3 := srvWithMoney(t)
	f3 := &fakeMail{}
	s3.Mail = f3
	s3.Watches.Set("worker", "worker@example.com", true)
	fund(t, s3, ctx3, observeJob())
	if err := s3.settle(ctx3, "obs_1", sub, "worker"); err != nil {
		t.Fatal(err)
	}
	if got := mailed(s3, f3); len(got) != 0 {
		t.Fatalf("somebody who turned alerts off was emailed: %+v", got)
	}
}

// A job nobody took is the end of a conversation the buyer never had.
func TestExpiredUnfilledJobTellsTheBuyer(t *testing.T) {
	s, ctx, f := srvWithMail(t)
	l := observeJob()
	fund(t, s, ctx, l)
	s.Now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if n, err := s.Sweep(ctx); err != nil || n != 1 {
		t.Fatalf("sweep: %d, %v", n, err)
	}
	b := to(mailed(s, f), "buyer@example.com")
	if len(b) != 1 {
		t.Fatalf("the buyer got %d emails", len(b))
	}
	for _, want := range []string{"nobody on it", "is the sign up", "$23.00", "returned to your balance"} {
		if !strings.Contains(b[0].body, want) {
			t.Errorf("the expiry email lacks %q:\n%s", want, b[0].body)
		}
	}
	s.Sweep(ctx)
	if n := len(mailed(s, f)); n != 1 {
		t.Fatalf("a second sweep repeated the expiry: %d", n)
	}
}

// Winning a bid is the moment a worker starts planning their day around it.
func TestAwardTellsTheWinner(t *testing.T) {
	s, ctx, f := srvWithMail(t)
	l := &api.Listing{
		Job: "bid_1", Kind: api.KindDo, Title: "paint the fence",
		Instructions: "two coats, white", PayMinor: 9000, Currency: "USD", Slots: 1,
		Expires: time.Now().Add(48 * time.Hour), Owner: "buyer",
	}
	fund(t, s, ctx, l)
	s.notifyAwarded(l, "worker", 8500, "USD")
	s.notifyAwarded(l, "worker", 8500, "USD")
	w := to(mailed(s, f), "worker@example.com")
	if len(w) != 1 {
		t.Fatalf("the winner got %d emails", len(w))
	}
	for _, want := range []string{"paint the fence", "$85.00", "https://example.test/board"} {
		if !strings.Contains(w[0].body, want) {
			t.Errorf("the award email lacks %q:\n%s", want, w[0].body)
		}
	}
}
