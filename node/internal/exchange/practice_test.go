package exchange

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/ledger"
)

// A practice run says what it is. It used to report "payment is still
// settling" over an escrow of nothing, and count as a real completion.
func TestAPracticeRunSaysSoAndCountsForNothing(t *testing.T) {
	ctx := context.Background()
	e := newE2E(t, "PLACEHOLDER")
	l := &api.Listing{
		Job: "practice-1", Kind: api.KindObserve,
		Title:    "Practice: photograph anything with the code in frame",
		Practice: true, Currency: "USD", Slots: 50, Expires: time.Now().Add(24 * time.Hour),
	}
	if err := e.srv.Board.Post(l); err != nil {
		t.Fatal(err)
	}
	secret, _, err := e.srv.Board.Claim("practice-1", "newcomer")
	if err != nil {
		t.Fatal(err)
	}
	cap, _ := e.srv.Caps.Lookup(secret)
	e.srv.Verify = (&SubmissionVerifier{Vision: &scriptedVision{text: api.ChallengeFor("practice-1", cap)}}).Verify

	img := photo(t, 7)
	up := "/v1/work/practice-1/evidence"
	if c, b := e.do("POST", up, img, capHdr("practice-1", secret, "POST", up, img)); c != 200 {
		t.Fatalf("upload: %d %s", c, b)
	}
	fin := "/v1/work/practice-1/submit"
	c, b := e.do("POST", fin, nil, capHdr("practice-1", secret, "POST", fin, nil))
	if c != 200 {
		t.Fatalf("submit: %d %s", c, b)
	}
	var res map[string]any
	json.Unmarshal(b, &res)
	if res["practice"] != true || res["status"] != "practice run recorded" {
		t.Fatalf("the worker is told %s", b)
	}
	if _, ok := res["amount_minor"]; ok {
		t.Fatalf("a practice run reports an amount: %s", b)
	}
	if why, _ := res["why"].(string); strings.Contains(why, "settling") {
		t.Fatalf("a practice run claims payment is settling: %s", b)
	}
	if note, _ := res["note"].(string); !strings.Contains(note, "not count") {
		t.Fatalf("the note does not say it is unrecorded: %s", b)
	}
	if pay, _ := e.srv.Ledger.Balance(ctx, ledger.PayableOf("newcomer"), "USD"); pay != 0 {
		t.Fatalf("a practice run credited %d", pay)
	}
	done, abandoned, allowance, _ := e.srv.Board.Standing("newcomer")
	if done != 0 || abandoned != 0 || allowance != 1 {
		t.Fatalf("practice changed standing: completed %d, abandoned %d, allowance %d",
			done, abandoned, allowance)
	}
	if st := e.srv.Board.StandingFor("newcomer"); st.Settled != 0 {
		t.Fatalf("the assurance record counts %d settled after a practice run", st.Settled)
	}
}
