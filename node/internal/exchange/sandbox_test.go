package exchange

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/account"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/ledger"
)

// sandboxJob posts one against the sandbox with no credential at all and
// returns the job id, the buyer token and the whole reply.
func sandboxJob(t *testing.T, e *e2e, body string) (job, token string, out map[string]any) {
	t.Helper()
	code, b := e.do("POST", "/v1/tasks", []byte(body),
		map[string]string{"Content-Type": "application/json"})
	if code != 200 {
		t.Fatalf("posting a sandbox job: %d %s", code, b)
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	job, _ = out["job"].(string)
	token, _ = out["token"].(string)
	if job == "" || token == "" {
		t.Fatalf("a sandbox job came back without a job or a token: %s", b)
	}
	return job, token, out
}

// waitForSubmission polls the buyer's own status route until the simulated
// operator has finished, the way a developer integrating against this would.
func waitForSubmission(t *testing.T, e *e2e, job, token string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		code, b := e.do("GET", "/v1/jobs/"+job, nil,
			map[string]string{"Authorization": "Bearer " + token})
		if code != 200 {
			t.Fatalf("job status: %d %s", code, b)
		}
		var out map[string]any
		json.Unmarshal(b, &out)
		if n, _ := out["submissions"].(float64); n >= 1 {
			return out
		}
		if time.Now().After(deadline) {
			t.Fatalf("the sandbox never finished: %s", b)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// The whole point, end to end and over HTTP: one call with no credential, and
// a few seconds later a completed job with a receipt.
//
// Before the sandbox existed this was impossible anywhere on earth, because no
// operator had registered anywhere, so every developer's first request was
// answered with "nobody can do this" and they left.
func TestASandboxJobWalksTheWholeLoopWithNoCredential(t *testing.T) {
	e := newE2E(t, "PLACEHOLDER")
	e.srv.SandboxStep = time.Millisecond

	job, token, out := sandboxJob(t, e, `{
		"sandbox": true, "kind": "do",
		"predicate": "The bins are back behind the side gate",
		"instructions": "Wheel both bins through the side gate and latch it.",
		"fee_minor": 1200}`)
	if out["sandbox"] != true {
		t.Fatalf("the reply that creates a sandbox job does not say so: %v", out)
	}
	if esc, _ := out["escrowed"].(float64); esc != 0 {
		t.Fatalf("a sandbox job reports %v escrowed", out["escrowed"])
	}

	st := waitForSubmission(t, e, job, token)
	if st["sandbox"] != true {
		t.Fatalf("job status does not say it is a sandbox job: %v", st)
	}
	if taken, _ := st["taken"].(float64); taken != 1 {
		t.Fatalf("nobody took the sandbox job: %v", st)
	}
	results, _ := st["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("expected one result: %v", st)
	}
	first, _ := results[0].(map[string]any)
	if first["verified"] != true {
		t.Fatalf("the sandbox submission was not accepted: %v", first)
	}

	// The evidence exists and is a real file the developer can look at.
	code, b := e.do("GET", "/v1/jobs/"+job+"/evidence", nil,
		map[string]string{"Authorization": "Bearer " + token})
	if code != 200 {
		t.Fatalf("evidence: %d %s", code, b)
	}
	if !strings.Contains(string(b), "sha256") {
		t.Fatalf("no evidence came back: %s", b)
	}
}

// The receipt is the artefact that leaves this server and is shown to somebody
// who does not trust it. If it can be mistaken for a real one, nothing else
// about the sandbox being labelled matters.
func TestASandboxReceiptSaysTheEvidenceIsSynthetic(t *testing.T) {
	e := newE2E(t, "PLACEHOLDER")
	e.srv.SandboxStep = time.Millisecond
	job, token, _ := sandboxJob(t, e, `{
		"sandbox": true, "kind": "observe",
		"predicate": "a FOR LEASE sign is displayed", "fee_minor": 500}`)
	waitForSubmission(t, e, job, token)

	code, b := e.do("GET", "/v1/jobs/"+job+"/receipt", nil,
		map[string]string{"Authorization": "Bearer " + token})
	if code != 200 {
		t.Fatalf("receipt: %d %s", code, b)
	}
	var r map[string]any
	json.Unmarshal(b, &r)
	if r["sandbox"] != true {
		t.Fatalf("the receipt is not labelled a sandbox receipt: %s", b)
	}
	if r["evidence_synthetic"] != true {
		t.Fatalf("the receipt does not flag the evidence as synthetic: %s", b)
	}
	if paid, _ := r["paid_minor"].(float64); paid != 0 {
		t.Fatalf("the receipt reports %v paid", r["paid_minor"])
	}
	// In words, not only in a boolean: a receipt is read by people.
	note, _ := r["note"].(string)
	for _, want := range []string{"SANDBOX", "generated", "nothing was paid"} {
		if !strings.Contains(note, want) {
			t.Fatalf("the receipt does not say %q in words: %q", want, note)
		}
	}
	// Anchoring is a public claim that a receipt existed unchanged. Spending
	// that on synthetic ones would make the whole chain worth less.
	if _, ok := r["anchor"]; ok {
		t.Fatalf("a sandbox receipt was anchored: %s", b)
	}
}

// A sandbox job must be invisible to everybody except whoever created it.
// An operator who travelled to one would have been sent to an address by a
// developer who was testing.
func TestASandboxJobNeverReachesThePublicBoard(t *testing.T) {
	e := newE2E(t, "PLACEHOLDER")
	e.srv.SandboxStep = time.Hour // never runs; the listing is what matters here

	job, token, _ := sandboxJob(t, e, `{
		"sandbox": true, "kind": "do",
		"predicate": "The bins are back behind the side gate",
		"instructions": "Wheel both bins through the side gate and latch it.",
		"fee_minor": 1200}`)

	code, b := e.do("GET", "/v1/board", nil, nil)
	if code != 200 {
		t.Fatalf("board: %d %s", code, b)
	}
	if strings.Contains(string(b), job) {
		t.Fatalf("a sandbox job is on the public board: %s", b)
	}
	if got := e.srv.Board.Listings(); len(got) != 0 {
		t.Fatalf("a sandbox job is open work: %d listing(s)", len(got))
	}
	// The one credential that may see it, sees it.
	code, b = e.do("GET", "/v1/jobs/"+job, nil,
		map[string]string{"Authorization": "Bearer " + token})
	if code != 200 || !strings.Contains(string(b), `"sandbox":true`) {
		t.Fatalf("the job's own poster cannot read it, or it is unlabelled: %d %s", code, b)
	}
	// And nobody else can, even holding another job's token.
	other := e.srv.BuyerToken("do-somebody-elses")
	if code, _ := e.do("GET", "/v1/jobs/"+job, nil,
		map[string]string{"Authorization": "Bearer " + other}); code == 200 {
		t.Fatal("another job's token read a sandbox job")
	}
}

// The guarantee the whole design rests on: a sandbox job writes no ledger row.
// Not a small one, not a sandbox-only one. None.
func TestASandboxJobTouchesNoMoneyAndALiveJobIsUnaffected(t *testing.T) {
	ctx := context.Background()
	e := newE2E(t, "PLACEHOLDER")
	e.srv.SandboxStep = time.Millisecond

	// A real job on the same server, funded and worked the ordinary way.
	const buyer, worker = "buyer-1", "worker-1"
	if _, err := e.srv.Ledger.Topup(ctx, "t1", buyer, 50000, "USD", ""); err != nil {
		t.Fatal(err)
	}
	live := &api.Listing{
		Job: "obs_live", Kind: api.KindObserve,
		Title:    "a FOR LEASE sign is displayed at 742 Evergreen Rd",
		PayMinor: 500, Currency: "USD", Slots: 1,
		Expires: time.Now().Add(time.Hour),
	}
	if _, err := e.srv.Ledger.Hold(ctx, "h1", live.Job, buyer,
		MaxPayoutFor(live), "USD"); err != nil {
		t.Fatal(err)
	}
	if err := e.srv.Board.Post(live); err != nil {
		t.Fatal(err)
	}
	e.srv.mu.Lock()
	e.srv.buyers[live.Job] = buyer
	e.srv.mu.Unlock()

	before, _ := e.srv.Ledger.Balance(ctx, ledger.BalanceOf(buyer), "USD")

	job, token, _ := sandboxJob(t, e, `{
		"sandbox": true, "kind": "observe",
		"predicate": "a FOR LEASE sign is displayed", "fee_minor": 500,
		"bonus_minor": 1800}`)
	waitForSubmission(t, e, job, token)

	// Nothing was held, nothing was captured, nobody was credited.
	if held, _ := e.srv.Ledger.Held(ctx, job, "USD"); held != 0 {
		t.Fatalf("a sandbox job holds %d in escrow", held)
	}
	if pay, _ := e.srv.Ledger.Balance(ctx,
		ledger.PayableOf(SandboxOperator), "USD"); pay != 0 {
		t.Fatalf("the simulated operator was credited %d", pay)
	}
	if bal, _ := e.srv.Ledger.Balance(ctx, ledger.BalanceOf(buyer), "USD"); bal != before {
		t.Fatalf("a real balance moved during a sandbox run: %d then %d", before, bal)
	}
	// No holdback, so nothing can ever enter the payout queue for it.
	if got := e.srv.Holdbacks.ForJob(job); len(got) != 0 {
		t.Fatalf("a sandbox job created %d holdback(s)", len(got))
	}
	if got := e.srv.Holdbacks.Available(SandboxOperator, time.Now().Add(72*time.Hour)); got != 0 {
		t.Fatalf("the simulated operator is owed %d", got)
	}
	if err := e.srv.Ledger.Audit(ctx); err != nil {
		t.Fatalf("the ledger no longer balances: %v", err)
	}

	// And the live job beside it still works exactly as it did: taken,
	// evidenced against a real challenge code, verified and paid.
	secret, _, err := e.srv.Board.Claim(live.Job, worker)
	if err != nil {
		t.Fatal(err)
	}
	cap, _ := e.srv.Caps.Lookup(secret)
	e.srv.Verify = (&SubmissionVerifier{
		Vision: &scriptedVision{text: api.ChallengeFor(live.Job, cap)}}).Verify
	img := photo(t, 3)
	up := "/v1/work/obs_live/evidence"
	if c, b := e.do("POST", up, img, capHdr(live.Job, secret, "POST", up, img)); c != 200 {
		t.Fatalf("upload: %d %s", c, b)
	}
	fin := "/v1/work/obs_live/submit"
	if c, b := e.do("POST", fin, nil, capHdr(live.Job, secret, "POST", fin, nil)); c != 200 {
		t.Fatalf("submit: %d %s", c, b)
	}
	if pay, _ := e.srv.Ledger.Balance(ctx, ledger.PayableOf(worker), "USD"); pay != net(500) {
		t.Fatalf("the live job credited %d, want %d", pay, net(500))
	}
	if err := e.srv.Ledger.Audit(ctx); err != nil {
		t.Fatal(err)
	}
}

// The settled price band is published to buyers as history. A developer
// looping the sandbox must not be able to move it.
func TestSandboxRunsDoNotMoveThePriceBand(t *testing.T) {
	e := newE2E(t, "PLACEHOLDER")
	e.srv.SandboxStep = time.Millisecond
	for i := 0; i < 6; i++ {
		job, token, _ := sandboxJob(t, e, `{
			"sandbox": true, "kind": "observe",
			"predicate": "a FOR LEASE sign is displayed", "fee_minor": 500}`)
		waitForSubmission(t, e, job, token)
	}
	if band := e.srv.priceBandFor(QuoteRequest{Kind: api.KindObserve}); band != nil {
		t.Fatalf("six sandbox runs produced a price band: %+v", band)
	}
}

// Terms the simulated operator cannot honour are refused with a reason, rather
// than accepted into a job that would then sit forever — which would teach a
// developer that the loop does not close.
func TestTheSandboxRefusesWhatItCannotWalk(t *testing.T) {
	e := newE2E(t, "PLACEHOLDER")
	for _, tc := range []struct{ name, body, want string }{
		{"bids", `{"sandbox":true,"kind":"do","predicate":"clear the gutter",
			"instructions":"clear it","pricing":"bids","max_bid_minor":9000,
			"fee_minor":9000}`, "bidders"},
		{"slots", `{"sandbox":true,"kind":"observe","predicate":"is the sign up",
			"fee_minor":500,"slots":3}`, "one seat"},
		{"project", `{"sandbox":true,"kind":"observe","predicate":"is the sign up",
			"fee_minor":500,"project_id":"p1"}`, "real budget"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, b := e.do("POST", "/v1/tasks", []byte(tc.body),
				map[string]string{"Content-Type": "application/json"})
			if code == 200 {
				t.Fatalf("accepted: %s", b)
			}
			if !strings.Contains(string(b), tc.want) {
				t.Fatalf("refused without saying why: %s", b)
			}
		})
	}
}

// The evidence must be generated, and it must be obviously generated. A
// developer who opens the file should see that nobody photographed anything.
func TestSandboxEvidenceIsADrawnFileAndNotAPhotograph(t *testing.T) {
	img, err := sandboxFrame("do-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(img) < 3 || img[0] != 0xFF || img[1] != 0xD8 || img[2] != 0xFF {
		t.Fatal("the sandbox frame is not a JPEG a viewer can open")
	}
	other, _ := sandboxFrame("do-2")
	if string(img) == string(other) {
		t.Fatal("every sandbox job is handed the same image")
	}
	// Nothing about a drawn frame can carry a challenge code a describer read,
	// and the submission says so rather than borrowing the word a real reading
	// would have used.
	sub := sandboxVerify(api.Submission{}, api.KindObserve)
	if !sub.Verified || !strings.Contains(sub.Why, "SANDBOX") {
		t.Fatalf("the submission does not say what it is: %+v", sub)
	}
}

// An agent key carries the budget its person set. A developer testing an
// integration must not exhaust that budget on jobs that never cost a cent.
func TestSandboxJobsDoNotSpendAnAgentKeysAllowance(t *testing.T) {
	ctx := context.Background()
	e := newE2E(t, "PLACEHOLDER")
	e.srv.SandboxStep = time.Millisecond

	_, pid := verifiedPerson(t, e.srv)
	if err := e.srv.Accounts.CreateAccount(ctx, pid, "buyer@example.com"); err != nil {
		t.Fatal(err)
	}
	secret, key, err := e.srv.Accounts.Issue(ctx, pid, "test",
		account.Limits{MaxPerOutcomeMinor: 5000, MaxTotalMinor: 5000, MaxOpen: 5}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	code, b := e.do("POST", "/v1/tasks", []byte(`{
		"sandbox": true, "kind": "observe",
		"predicate": "a FOR LEASE sign is displayed", "fee_minor": 4000}`),
		map[string]string{"Content-Type": "application/json", "X-Lamdis-Key": secret})
	if code != 200 {
		t.Fatalf("posting a sandbox job with a key: %d %s", code, b)
	}
	var out map[string]any
	json.Unmarshal(b, &out)
	if out["sandbox"] != true {
		t.Fatalf("the reply is not labelled: %s", b)
	}
	total, open, err := e.srv.Accounts.Committed(ctx, key.ID)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || open != 0 {
		t.Fatalf("a sandbox job committed %d against the key (%d open)", total, open)
	}
}

// The listing is written to disk with the rest of the state. If the flag did
// not survive that, a restart would turn a sandbox job into a real one.
func TestTheSandboxFlagSurvivesBeingWrittenToDisk(t *testing.T) {
	dir := t.TempDir()
	l := &api.Listing{
		Job: "do-1", Kind: api.KindDo, Title: "the bins are back",
		Instructions: "wheel them", Sandbox: true, Currency: "USD", Slots: 1,
		Expires: time.Now().Add(time.Hour),
	}
	b := api.NewBoard(api.NewCapabilities())
	b.Persist(dir)
	if err := b.Post(l); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "board.json"))
	if err != nil {
		t.Skipf("the board is not file-backed here: %v", err)
	}
	if !strings.Contains(string(raw), `"sandbox": true`) {
		t.Fatalf("the sandbox flag was not persisted: %s", raw)
	}
}
