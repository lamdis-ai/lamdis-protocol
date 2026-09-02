package exchange

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/ledger"
)

// A panel, funded, seated from the board, and answered over HTTP the way the
// page does it. Returns the server and a way to seat and answer.
func panelServer(t *testing.T) (*e2e, context.Context) {
	t.Helper()
	ctx := context.Background()
	e := newE2E(t, "unused")
	if _, err := e.srv.Ledger.Topup(ctx, "t1", "buyer", 100000, "USD", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := e.srv.Ledger.Hold(ctx, "h:panel-1", "panel-1", "buyer", (150+100)*3, "USD"); err != nil {
		t.Fatal(err)
	}
	e.srv.mu.Lock()
	e.srv.buyers["panel-1"] = "buyer"
	e.srv.mu.Unlock()
	if err := e.srv.AddPanel(&api.ReviewPanel{
		Job: "panel-1", Parent: "obs-9", Question: "Is the sign up?",
		Reviewers: 3, Agreement: 2, FeeMinor: 150, BonusMinor: 100, Currency: "USD",
		Expires: time.Now().Add(2 * time.Hour),
	}, testJPEG(t), "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	return e, ctx
}

func (e *e2e) review(t *testing.T, worker string, finding bool) map[string]any {
	t.Helper()
	secret, _, err := e.srv.Board.AssignReview(worker)
	if err != nil {
		t.Fatalf("%s: %v", worker, err)
	}
	body, _ := json.Marshal(map[string]any{
		"finding": finding, "confident": true,
		"reason": "The sign is clearly mounted above the door and legible from the street.",
	})
	p := "/v1/claims/panel-1/review"
	code, out := e.do("POST", p, body, capHdr("panel-1", secret, "POST", p, body))
	if code != 200 {
		t.Fatalf("%s review: %d %s", worker, code, out)
	}
	var res map[string]any
	json.Unmarshal(out, &res)
	return res
}

// The gap the review found: reviewers were promised money and paid nothing,
// and the seat they were assigned lapsed into an abandonment.
func TestAReviewIsPaidOnceAndFreesTheSeat(t *testing.T) {
	e, ctx := panelServer(t)
	res := e.review(t, "w1", true)
	if res["paid_minor"] != float64(net(150)) {
		t.Fatalf("the page is told %v, want %d credited", res["paid_minor"], net(150))
	}
	if pay, _ := e.srv.Ledger.Balance(ctx, ledger.PayableOf("w1"), "USD"); pay != net(150) {
		t.Fatalf("w1 is owed %d, want %d", pay, net(150))
	}
	// Once. A retry of the settlement credits nothing more.
	if _, err := e.srv.settleReview("panel-1", "w1", api.Review{Worker: "w1"}); err != nil {
		t.Fatal(err)
	}
	if pay, _ := e.srv.Ledger.Balance(ctx, ledger.PayableOf("w1"), "USD"); pay != net(150) {
		t.Fatalf("a second settlement credited again: %d", pay)
	}
	// The fee waits out the window like any other earnings.
	if clear := e.srv.Holdbacks.Available("w1", time.Now()); clear != 0 {
		t.Fatalf("%d was clear to send before the window closed", clear)
	}
	if pending := e.srv.Holdbacks.Pending("w1", time.Now()); len(pending) != 1 {
		t.Fatalf("holdbacks: %+v", pending)
	}
	// The seat is free and the record is a completion, not an abandonment.
	done, abandoned, _, cool := e.srv.Board.Standing("w1")
	if done != 1 || abandoned != 0 || !cool.IsZero() {
		t.Fatalf("standing after a review: completed %d, abandoned %d, cooldown %v",
			done, abandoned, cool)
	}
	// Escrow for the two unanswered seats stays put.
	if held, _ := e.srv.Ledger.Held(ctx, "panel-1", "USD"); held != (150+100)*3-150 {
		t.Fatalf("held %d after one of three reviews", held)
	}

	// The panel completes: two agree, one dissents. The bonus goes to the two,
	// and what nobody earned goes back to the buyer.
	e.review(t, "w2", true)
	e.review(t, "w3", false)
	for _, c := range []struct {
		who  string
		want int64
	}{{"w1", net(150) + net(100)}, {"w2", net(150) + net(100)}, {"w3", net(150)}} {
		if pay, _ := e.srv.Ledger.Balance(ctx, ledger.PayableOf(c.who), "USD"); pay != c.want {
			t.Errorf("%s is owed %d, want %d", c.who, pay, c.want)
		}
	}
	if held, _ := e.srv.Ledger.Held(ctx, "panel-1", "USD"); held != 0 {
		t.Fatalf("%d is stuck in the panel's escrow", held)
	}
	bal, _ := e.srv.Ledger.Balance(ctx, ledger.BalanceOf("buyer"), "USD")
	if want := int64(100000 - 3*150 - 2*100); bal != want {
		t.Fatalf("buyer holds %d, want %d", bal, want)
	}
	if err := e.srv.Ledger.Audit(ctx); err != nil {
		t.Fatal(err)
	}
}
