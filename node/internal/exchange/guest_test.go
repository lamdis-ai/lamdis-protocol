package exchange

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/payment"
)

// The gateless path, end to end: nobody signed in, nothing topped up, no key.

func guestJobBody(t *testing.T) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"kind": "observe", "predicate": "the FOR LEASE sign is up",
		"where": "742 Evergreen Rd", "area": "Detroit, MI",
		"lat": 42.33, "lon": -83.04, "radius_m": 150,
		"fee_minor": 2300, "attempt_minor": 500,
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func postAsNobody(t *testing.T, h http.Handler) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/tasks", bytes.NewReader(guestJobBody(t))))
	if w.Code != http.StatusOK {
		t.Fatalf("anonymous post: %d %s", w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func boardHas(t *testing.T, h http.Handler, job string) bool {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v1/board", nil))
	return strings.Contains(w.Body.String(), `"`+job+`"`)
}

func TestNobodyCanPostAJobAndGetsAPayLink(t *testing.T) {
	s := consoleServer(t)
	h := s.Handler()

	out := postAsNobody(t, h)
	if out["status"] != "awaiting_payment" {
		t.Fatalf("status %v, want awaiting_payment: %v", out["status"], out)
	}
	job, _ := out["job"].(string)
	tok, _ := out["token"].(string)
	payAt, _ := out["pay_at"].(string)
	if job == "" || !strings.HasPrefix(tok, "lbt_") || payAt == "" {
		t.Fatalf("reply lacks job/token/pay_at: %v", out)
	}
	// The ceiling is what a balance-funded job would have escrowed: pay plus
	// nothing extra here, because the attempt fee is smaller than the pay.
	if out["amount_minor"].(float64) != 2300 {
		t.Errorf("amount_minor %v, want 2300", out["amount_minor"])
	}
	// Not on the board until the card lands.
	if boardHas(t, h, job) {
		t.Fatal("an unpaid job reached the open board")
	}
	// No rail in a test: the pay link is our own page, which says so.
	if !strings.HasSuffix(payAt, "/pay/"+job) {
		t.Errorf("pay_at %q should be the no-rail page", payAt)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/pay/"+job, nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "cannot take card payments") {
		t.Errorf("the pay page should say plainly that there is no rail: %d %s", w.Code, w.Body.String()[:120])
	}

	// The token reads the job's state; a token for another job does not.
	r := httptest.NewRequest("GET", "/v1/jobs/"+job, nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "awaiting_payment") {
		t.Errorf("token status read: %d %s", w.Code, w.Body.String())
	}
	other := postAsNobody(t, h)
	r = httptest.NewRequest("GET", "/v1/jobs/"+job, nil)
	r.Header.Set("Authorization", "Bearer "+other["token"].(string))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code == http.StatusOK {
		t.Error("a token for job B read job A")
	}
	// The guest page for it renders with nothing but the token.
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/my/"+job+"?t="+tok, nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), job) {
		t.Errorf("/my page: %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/post", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "No account needed") {
		t.Errorf("/post page: %d", w.Code)
	}
}

func TestAnAuthorisedCardListsTheJobAndIsChargedOnlyForWhatWasPaid(t *testing.T) {
	s := consoleServer(t)
	rail := payment.NewMock()
	s.Charges = rail
	h := s.Handler()
	ctx := context.Background()

	out := postAsNobody(t, h)
	job := out["job"].(string)
	amount := int64(out["amount_minor"].(float64))

	// The rail authorised the card (the mock's hold stands in for the intent).
	if _, err := rail.Hold(ctx, payment.Request{Key: "auth", AmountMinor: amount, Currency: "USD", Outcome: job}); err != nil {
		t.Fatal(err)
	}
	if err := s.FundFromCard(ctx, job, "cs_1", "pi_1", amount, "payer@example.test"); err != nil {
		t.Fatal(err)
	}
	// Twice is once.
	if err := s.FundFromCard(ctx, job, "cs_1", "pi_1", amount, "payer@example.test"); err != nil {
		t.Fatal(err)
	}
	if !boardHas(t, h, job) {
		t.Fatal("a paid job is not on the board")
	}
	held, err := s.Ledger.Held(ctx, job, "USD")
	if err != nil || held != amount {
		t.Fatalf("escrow %d (%v), want %d", held, err, amount)
	}
	l, _ := s.Board.Get(job)
	if l.Funding == nil || l.Funding.Intent != "pi_1" || l.Funding.Email != "payer@example.test" {
		t.Fatalf("funding not recorded on the listing: %+v", l.Funding)
	}
	if email, ok := s.emailFor(l.Owner); !ok || email != "payer@example.test" {
		t.Errorf("the payer is not reachable for notifications: %q %v", email, ok)
	}

	// The job paid out 800 of its 2300 ceiling. The card is charged 800.
	if _, err := s.Ledger.Capture(ctx, "settle:"+job+":w", job, "worker-1", 800, 0, "USD"); err != nil {
		t.Fatal(err)
	}
	if err := s.releaseIfDoneNow(ctx, l); err != nil {
		t.Fatal(err)
	}
	if got := rail.HeldFor(job); got != amount-800 {
		t.Errorf("card still holds %d; want %d (captured exactly what was paid)", got, amount-800)
	}
	// The guest principal's balance carries no phantom money afterwards.
	if bal, _ := s.Ledger.Balance(ctx, l.Owner, "USD"); bal != 0 {
		t.Errorf("guest balance %d after settlement; want 0", bal)
	}
	// And settling again captures nothing more.
	if err := s.releaseIfDoneNow(ctx, l); err != nil {
		t.Fatal(err)
	}
	if got := rail.HeldFor(job); got != amount-800 {
		t.Errorf("a second settlement moved the card again: %d", got)
	}
}

func TestACancelledCardJobIsNeverCharged(t *testing.T) {
	s := consoleServer(t)
	rail := payment.NewMock()
	s.Charges = rail
	h := s.Handler()
	ctx := context.Background()

	out := postAsNobody(t, h)
	job, tok := out["job"].(string), out["token"].(string)
	amount := int64(out["amount_minor"].(float64))
	_, _ = rail.Hold(ctx, payment.Request{Key: "auth", AmountMinor: amount, Currency: "USD", Outcome: job})
	if err := s.FundFromCard(ctx, job, "cs_2", "pi_2", amount, ""); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/v1/jobs/"+job+"/cancel", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("cancel with the token: %d %s", w.Code, w.Body.String())
	}
	if got := rail.HeldFor(job); got != 0 {
		t.Errorf("the authorisation was not released on cancel: %d still held", got)
	}
	if bal, _ := s.Ledger.Balance(ctx, guestOwner(job), "USD"); bal != 0 {
		t.Errorf("guest balance %d after cancel; want 0", bal)
	}
}

func TestAnUnpaidJobIsDroppedAfterADay(t *testing.T) {
	s := consoleServer(t)
	h := s.Handler()
	out := postAsNobody(t, h)
	job := out["job"].(string)
	base := time.Now()
	s.Now = func() time.Time { return base.Add(PendingTTL + time.Hour) }
	if _, err := s.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.FundFromCard(context.Background(), job, "cs_3", "pi_3", 2300, ""); err == nil {
		t.Error("a payment for a dropped job was accepted")
	}
}

func TestGuestsCannotUseAccountShapedJobs(t *testing.T) {
	s := consoleServer(t)
	h := s.Handler()
	raw, _ := json.Marshal(map[string]any{
		"kind": "observe", "predicate": "anything", "fee_minor": 100,
		"pricing": "bids", "max_bid_minor": 5000,
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/tasks", bytes.NewReader(raw)))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "account") {
		t.Errorf("open bidding without an account should be refused with a reason: %d %s", w.Code, w.Body.String())
	}
}

// The adversarial review's two findings against this path.

func TestThePaidReturnOnlyHandsTheTokenToTheSessionThatPaid(t *testing.T) {
	s := consoleServer(t)
	h := s.Handler()
	out := postAsNobody(t, h)
	job := out["job"].(string)
	if err := s.FundFromCard(context.Background(), job, "cs_real", "pi_9", 2300, ""); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"", "?session=cs_somebody_elses"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/paid/"+job+q, nil))
		if w.Code == http.StatusFound || strings.Contains(w.Header().Get("Location"), "lbt_") {
			t.Fatalf("/paid/%s%s handed out the buyer token", job, q)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/paid/"+job+"?session=cs_real", nil))
	if w.Code != http.StatusFound || !strings.Contains(w.Header().Get("Location"), "lbt_") {
		t.Fatalf("the session that paid was not sent to its job: %d %s", w.Code, w.Header().Get("Location"))
	}
}

func TestACardFundedJobCannotBeReleasedEarly(t *testing.T) {
	s := consoleServer(t)
	h := s.Handler()
	out := postAsNobody(t, h)
	job, tok := out["job"].(string), out["token"].(string)
	if err := s.FundFromCard(context.Background(), job, "cs_r", "pi_r", 2300, ""); err != nil {
		t.Fatal(err)
	}
	s.Holdbacks.Add(job, "worker-1", 2300, "USD", s.now(), s.disputeWindow())
	r := httptest.NewRequest("POST", "/v1/jobs/"+job+"/release", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"released":0`) {
		t.Fatalf("early release on a card-funded job: %d %s", w.Code, w.Body.String())
	}
}

func TestAnonymousPostingIsRateLimited(t *testing.T) {
	s := consoleServer(t)
	h := s.Handler()
	var last int
	for i := 0; i < guestPerHour+1; i++ {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/v1/tasks", bytes.NewReader(guestJobBody(t)))
		r.Header.Set("X-Forwarded-For", "203.0.113.9")
		h.ServeHTTP(w, r)
		last = w.Code
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("the %dth unpaid job from one address got %d, want 429", guestPerHour+1, last)
	}
}

// The feasibility check is the question an agent must ask before promising
// anything, so it must be askable with no credential at all.
func TestAQuoteNeedsNoCredential(t *testing.T) {
	s := consoleServer(t)
	h := s.Handler()
	raw, _ := json.Marshal(map[string]any{"kind": "observe", "predicate": "the sign is up", "lat": 42.33, "lon": -83.04})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/quote", bytes.NewReader(raw)))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "feasible") {
		t.Fatalf("anonymous quote: %d %s", w.Code, w.Body.String())
	}
}
