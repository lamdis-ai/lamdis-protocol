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

// A find-out job is answered with a table, paid like an observation, and
// receipted as what it is: a signed claim with no photograph behind it.
func TestAReportOnlySubmissionIsAcceptedPaidAndReceiptedHonestly(t *testing.T) {
	ctx := context.Background()
	e := newE2E(t, "NOTUSED")
	const buyer, worker = "buyer-1", "worker-1"
	if _, err := e.srv.Ledger.Topup(ctx, "t1", buyer, 50000, "USD", ""); err != nil {
		t.Fatal(err)
	}
	l := &api.Listing{
		Job: "find-1", Kind: api.KindObserve, Title: "Quotes for a new water heater",
		Instructions: "Call three installers", Where: "812 Marlow Street",
		PayMinor: 900, Currency: "USD", Slots: 1, Tier: "V2",
		Expires: time.Now().Add(time.Hour),
		Report: []api.ReportField{
			{Name: "provider", Label: "Who", Kind: api.FieldText, Required: true, Repeats: true},
			{Name: "price", Label: "Quoted price", Kind: api.FieldMoney, Repeats: true},
		},
	}
	if _, err := e.srv.Ledger.Hold(ctx, "h1", l.Job, buyer, MaxPayoutFor(l), "USD"); err != nil {
		t.Fatal(err)
	}
	if err := e.srv.Board.Post(l); err != nil {
		t.Fatal(err)
	}
	e.srv.mu.Lock()
	e.srv.buyers[l.Job] = buyer
	e.srv.mu.Unlock()

	secret, _, err := e.srv.Board.Claim("find-1", worker)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"report": []map[string]string{
		{"provider": "Acme", "price": "1450.00"},
		{"provider": "Northside", "price": "1300.00"},
	}})
	fin := "/v1/work/find-1/submit"
	code, out := e.do("POST", fin, body, capHdr("find-1", secret, "POST", fin, body))
	if code != 200 {
		t.Fatalf("submit: %d %s", code, out)
	}
	var res map[string]any
	json.Unmarshal(out, &res)
	if res["verified"] != true || res["reached"] != "V0" || res["status"] != "accepted" {
		t.Fatalf("the worker is told %s", out)
	}
	if pay, _ := e.srv.Ledger.Balance(ctx, ledger.PayableOf(worker), "USD"); pay != net(900) {
		t.Fatalf("credited %d, want %d", pay, net(900))
	}
	if held, _ := e.srv.Ledger.Held(ctx, "find-1", "USD"); held != 0 {
		t.Fatalf("%d is stuck in escrow", held)
	}

	// The receipt must not repeat the photographic assurances.
	subs := e.srv.Submissions("find-1")
	if len(subs) != 1 || len(subs[0].Report) != 2 {
		t.Fatalf("stored %+v", subs)
	}
	got, _ := e.srv.Board.Get("find-1")
	block := verificationBlock(got, subs)
	if block["tier_reached"] != "V0" {
		t.Fatalf("receipt claims tier %v for a typed table", block["tier_reached"])
	}
	for _, line := range block["established"].([]string) {
		if strings.Contains(line, "code") || strings.Contains(line, "generated") {
			t.Errorf("receipt asserts a photographic check that never ran: %q", line)
		}
	}
	joined := strings.Join(block["limits"].([]string), " ")
	if !strings.Contains(joined, "no challenge code") {
		t.Errorf("receipt does not say there was no code: %s", joined)
	}
	if err := e.srv.Ledger.Audit(ctx); err != nil {
		t.Fatal(err)
	}
}
