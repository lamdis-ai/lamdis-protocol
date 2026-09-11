package exchange

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/chain"
)

// Inline payment against a fake facilitator. No chain, no key, no network.

// fakeFacilitator answers /verify and /settle the way x402 v1 says to, and
// counts what it was asked.
type fakeFacilitator struct {
	mu       sync.Mutex
	verifies int
	settles  int
	// invalid, when set, is the invalidReason /verify answers with.
	invalid string
	// failSettle, when set, is the errorReason /settle answers with.
	failSettle string
	// last is the last request body, for checking what was sent.
	last map[string]any
}

func (f *fakeFacilitator) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var in map[string]any
	json.NewDecoder(r.Body).Decode(&in)
	f.last = in
	payload, _ := in["paymentPayload"].(map[string]any)
	inner, _ := payload["payload"].(map[string]any)
	auth, _ := inner["authorization"].(map[string]any)
	from, _ := auth["from"].(string)
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/verify":
		f.verifies++
		if f.invalid != "" {
			json.NewEncoder(w).Encode(map[string]any{"isValid": false, "invalidReason": f.invalid, "payer": from})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"isValid": true, "payer": from})
	case "/settle":
		f.settles++
		if f.failSettle != "" {
			json.NewEncoder(w).Encode(map[string]any{"success": false, "errorReason": f.failSettle,
				"transaction": "", "network": "base-sepolia", "payer": from})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"success": true, "payer": from,
			"transaction": fmt.Sprintf("0x%064x", f.settles), "network": "base-sepolia"})
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeFacilitator) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.verifies, f.settles
}

// x402Server is an exchange with the rail on and a facilitator to settle through.
func x402Server(t *testing.T) (*Server, *fakeFacilitator) {
	t.Helper()
	s, _ := usdcServer(t)
	f := &fakeFacilitator{}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	s.X402 = NewX402(srv.URL)
	return s, f
}

func x402Body(t *testing.T) []byte {
	t.Helper()
	var m map[string]any
	json.Unmarshal(guestJobBody(t), &m)
	m["x402"] = true
	b, _ := json.Marshal(m)
	return b
}

// paymentHeader signs nothing; the facilitator is fake. It carries what a
// real client would: the value the 402 asked for, to its payTo, with a nonce.
func paymentHeader(value, to, nonce string) string {
	b, _ := json.Marshal(map[string]any{
		"x402Version": 1, "scheme": "exact", "network": "base-sepolia",
		"payload": map[string]any{
			"signature": "0xsigned",
			"authorization": map[string]any{
				"from": payerAddr, "to": to, "value": value,
				"validAfter": "0", "validBefore": "9999999999", "nonce": nonce,
			},
		},
	})
	return base64.StdEncoding.EncodeToString(b)
}

// ask posts the x402 job and returns the 402.
func ask(t *testing.T, h http.Handler) (map[string]any, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/tasks", bytes.NewReader(x402Body(t))))
	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("x402 post: %d %s, want 402", w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	accepts, _ := out["accepts"].([]any)
	if len(accepts) != 1 {
		t.Fatalf("accepts: %v", out)
	}
	req, _ := accepts[0].(map[string]any)
	return out, req
}

// pay retries the same request with the payment.
func pay(t *testing.T, h http.Handler, header string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/v1/tasks", bytes.NewReader(x402Body(t)))
	r.Header.Set("X-PAYMENT", header)
	h.ServeHTTP(w, r)
	return w
}

func TestX402AnswersFourOhTwoWithTheRequirements(t *testing.T) {
	s, f := x402Server(t)
	h := s.Handler()

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/tasks", bytes.NewReader(x402Body(t))))
	if w.Code != http.StatusPaymentRequired || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("%d %s %s", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
	var out map[string]any
	json.Unmarshal(w.Body.Bytes(), &out)
	if out["x402Version"] != float64(1) || out["error"] == "" || out["status"] != "awaiting_payment" {
		t.Fatalf("body: %v", out)
	}
	job := out["job"].(string)
	req := out["accepts"].([]any)[0].(map[string]any)
	amount := int64(out["amount_minor"].(float64))
	want := map[string]any{
		"scheme": "exact", "network": "base-sepolia",
		"maxAmountRequired": strconv.FormatInt(chain.UnitsFor(job, amount), 10),
		"resource":          "https://example.test/v1/tasks",
		"payTo":             exchangeAddr, "asset": chain.USDCBaseSepolia,
		"mimeType": "application/json",
	}
	for k, v := range want {
		if req[k] != v {
			t.Errorf("accepts[0].%s = %v, want %v", k, req[k], v)
		}
	}
	if req["maxTimeoutSeconds"] != float64(x402Timeout) || req["description"] == "" {
		t.Errorf("timeout/description: %v", req)
	}
	extra, _ := req["extra"].(map[string]any)
	if extra["name"] != "USDC" || extra["version"] != "2" {
		t.Errorf("extra: %v", extra)
	}
	if amount != 2300 || req["maxAmountRequired"].(string)[:5] != "23000" {
		t.Errorf("the ceiling is %d cents, asked %v", amount, req["maxAmountRequired"])
	}
	if out["token"] != s.BuyerToken(job) {
		t.Error("no token")
	}
	if boardHas(t, h, job) {
		t.Error("a priced, unpaid job is on the board")
	}
	if _, ok := s.pendingFor(job); !ok {
		t.Error("the job is not parked")
	}
	if v, st := f.counts(); v+st != 0 {
		t.Error("the facilitator was asked before any payment arrived")
	}
}

func TestX402VerifiesSettlesAndListsInOneRoundTrip(t *testing.T) {
	s, f := x402Server(t)
	h := s.Handler()
	ctx := context.Background()

	out, req := ask(t, h)
	job := out["job"].(string)
	w := pay(t, h, paymentHeader(req["maxAmountRequired"].(string), exchangeAddr, "0xabc1"))
	if w.Code != http.StatusOK {
		t.Fatalf("paid retry: %d %s", w.Code, w.Body.String())
	}
	var listed map[string]any
	json.Unmarshal(w.Body.Bytes(), &listed)
	if listed["job"] != job || listed["status"] != "listed" || listed["token"] != s.BuyerToken(job) {
		t.Fatalf("reply: %v", listed)
	}
	raw, err := base64.StdEncoding.DecodeString(w.Header().Get("X-PAYMENT-RESPONSE"))
	if err != nil {
		t.Fatalf("X-PAYMENT-RESPONSE is not base64: %v", err)
	}
	var pr map[string]any
	json.Unmarshal(raw, &pr)
	tx, _ := pr["transaction"].(string)
	if pr["success"] != true || len(tx) != 66 || pr["network"] != "base-sepolia" || pr["payer"] != payerAddr {
		t.Fatalf("X-PAYMENT-RESPONSE: %v", pr)
	}
	if !boardHas(t, h, job) {
		t.Fatal("the job is not on the board")
	}
	l, _ := s.Board.Get(job)
	if l.Funding == nil || l.Funding.Kind != "x402" || l.Funding.Intent != tx ||
		l.Funding.Payer != payerAddr || l.Funding.AuthorizedMinor != 2300 {
		t.Fatalf("funding: %+v", l.Funding)
	}
	if held, _ := s.Ledger.Held(ctx, job, "USD"); held != 2300 {
		t.Errorf("escrow holds %d, want 2300", held)
	}
	if _, ok := s.pendingFor(job); ok {
		t.Error("still parked after listing")
	}
	if v, st := f.counts(); v != 1 || st != 1 {
		t.Errorf("facilitator asked verify %d settle %d, want 1 and 1", v, st)
	}
	// What the facilitator was handed is the requirements the 402 quoted.
	pr2, _ := f.last["paymentRequirements"].(map[string]any)
	if pr2["maxAmountRequired"] != req["maxAmountRequired"] || pr2["payTo"] != exchangeAddr {
		t.Errorf("facilitator got %v", pr2)
	}
	// And the same transfer seen by the watcher later changes nothing.
	if err := s.FundFromChain(ctx, job, tx, 2300, payerAddr); err != nil {
		t.Errorf("the watcher seeing the settle tx: %v", err)
	}
	if held, _ := s.Ledger.Held(ctx, job, "USD"); held != 2300 {
		t.Errorf("escrow holds %d after the watcher, want 2300", held)
	}
}

func TestX402ARejectedPaymentIsPricedAgainAndNothingLists(t *testing.T) {
	s, f := x402Server(t)
	f.invalid = "invalid_signature"
	h := s.Handler()

	out, req := ask(t, h)
	job := out["job"].(string)
	w := pay(t, h, paymentHeader(req["maxAmountRequired"].(string), exchangeAddr, "0xabc2"))
	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("rejected payment: %d %s, want 402", w.Code, w.Body.String())
	}
	var again map[string]any
	json.Unmarshal(w.Body.Bytes(), &again)
	if again["error"] != "invalid_signature" || again["job"] != job {
		t.Fatalf("second 402: %v", again)
	}
	if w.Header().Get("X-PAYMENT-RESPONSE") != "" {
		t.Error("a settlement header on a rejected payment")
	}
	if boardHas(t, h, job) {
		t.Error("listed on a rejected payment")
	}
	if v, st := f.counts(); v != 1 || st != 0 {
		t.Errorf("verify %d settle %d; settle must not be called", v, st)
	}
	if _, ok := s.pendingFor(job); !ok {
		t.Error("the job was dropped; it should wait for a good payment")
	}
}

func TestX402AFailedSettlementListsNothing(t *testing.T) {
	s, f := x402Server(t)
	f.failSettle = "insufficient_funds"
	h := s.Handler()

	out, req := ask(t, h)
	job := out["job"].(string)
	w := pay(t, h, paymentHeader(req["maxAmountRequired"].(string), exchangeAddr, "0xabc3"))
	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("failed settle: %d %s, want 402", w.Code, w.Body.String())
	}
	var again map[string]any
	json.Unmarshal(w.Body.Bytes(), &again)
	if again["error"] != "not listed: insufficient_funds" {
		t.Fatalf("second 402: %v", again["error"])
	}
	if boardHas(t, h, job) {
		t.Error("listed on a failed settlement")
	}
	if held, _ := s.Ledger.Held(context.Background(), job, "USD"); held != 0 {
		t.Errorf("escrow holds %d after a failed settlement", held)
	}
	if v, st := f.counts(); v != 1 || st != 1 {
		t.Errorf("verify %d settle %d", v, st)
	}
	// The same nonce may try again: a failed settlement is not a spent one.
	f.failSettle = ""
	if w := pay(t, h, paymentHeader(req["maxAmountRequired"].(string), exchangeAddr, "0xabc3")); w.Code != http.StatusOK {
		t.Fatalf("retry after a failed settlement: %d %s", w.Code, w.Body.String())
	}
	if !boardHas(t, h, job) {
		t.Error("not listed after the retry")
	}
}

func TestX402AReplayedPaymentFundsOnce(t *testing.T) {
	s, f := x402Server(t)
	h := s.Handler()
	ctx := context.Background()

	out, req := ask(t, h)
	job := out["job"].(string)
	header := paymentHeader(req["maxAmountRequired"].(string), exchangeAddr, "0xABC4")
	first := pay(t, h, header)
	if first.Code != http.StatusOK {
		t.Fatalf("first: %d %s", first.Code, first.Body.String())
	}
	second := pay(t, h, header)
	if second.Code != http.StatusOK || second.Body.String() != first.Body.String() ||
		second.Header().Get("X-PAYMENT-RESPONSE") != first.Header().Get("X-PAYMENT-RESPONSE") {
		t.Fatalf("replay: %d %s", second.Code, second.Body.String())
	}
	// Case in the nonce is not a different payment.
	third := pay(t, h, paymentHeader(req["maxAmountRequired"].(string), exchangeAddr, "0xabc4"))
	if third.Code != http.StatusOK || third.Body.String() != first.Body.String() {
		t.Fatalf("replay with a re-cased nonce: %d %s", third.Code, third.Body.String())
	}
	if v, st := f.counts(); v != 1 || st != 1 {
		t.Errorf("verify %d settle %d after replays, want 1 and 1", v, st)
	}
	if held, _ := s.Ledger.Held(ctx, job, "USD"); held != 2300 {
		t.Errorf("escrow holds %d, want exactly one funding", held)
	}
	// A different nonce for an amount nobody is waiting for prices a new job
	// rather than touching the listed one.
	w := pay(t, h, paymentHeader(req["maxAmountRequired"].(string), exchangeAddr, "0xabc5"))
	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("a second payment for a listed job: %d %s", w.Code, w.Body.String())
	}
	var again map[string]any
	json.Unmarshal(w.Body.Bytes(), &again)
	if again["job"] == job {
		t.Error("the listed job was priced again")
	}
	if v, st := f.counts(); v != 1 || st != 1 {
		t.Errorf("the facilitator was asked for a payment matching no job: verify %d settle %d", v, st)
	}
}

func TestX402IsOffWithoutAFacilitator(t *testing.T) {
	// No env: nothing.
	t.Setenv("LAMDIS_X402_FACILITATOR", "")
	if x, err := NewX402FromEnv(); x != nil || err != nil {
		t.Fatalf("NewX402FromEnv with nothing set: %v %v", x, err)
	}
	t.Setenv("LAMDIS_X402_FACILITATOR", "not a url")
	if _, err := NewX402FromEnv(); err == nil {
		t.Fatal("a facilitator that is not a URL was accepted")
	}
	t.Setenv("LAMDIS_X402_FACILITATOR", "https://x402.org/facilitator/")
	x, err := NewX402FromEnv()
	if err != nil || x == nil || x.Facilitator != "https://x402.org/facilitator" {
		t.Fatalf("NewX402FromEnv: %v %v", x, err)
	}

	// Rail on, no facilitator: asking for x402 gets the pay link, not a 402.
	s, _ := usdcServer(t)
	h := s.Handler()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/tasks", bytes.NewReader(x402Body(t))))
	if w.Code != http.StatusOK {
		t.Fatalf("x402 with no facilitator: %d %s, want the pay link", w.Code, w.Body.String())
	}
	var out map[string]any
	json.Unmarshal(w.Body.Bytes(), &out)
	if _, has := out["x402"]; has || out["pay_at"] == "" {
		t.Errorf("reply advertises x402 with it off: %v", out)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v1/rails", nil))
	var rails map[string]map[string]any
	json.Unmarshal(w.Body.Bytes(), &rails)
	if rails["x402"]["on"] != false {
		t.Errorf("/v1/rails: %v", rails["x402"])
	}

	// Facilitator named, rail off: still off, and a payment header is inert.
	off := consoleServer(t)
	off.X402 = NewX402("https://x402.org/facilitator")
	h = off.Handler()
	w = httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/v1/tasks", bytes.NewReader(x402Body(t)))
	r.Header.Set("X-PAYMENT", paymentHeader("1", exchangeAddr, "0x1"))
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("rail off: %d %s", w.Code, w.Body.String())
	}
}

func TestX402LeavesTheCardPathAloneAndAdvertises(t *testing.T) {
	s, f := x402Server(t)
	h := s.Handler()

	// The default request, with everything on, is the pay link it always was.
	out := postAsNobody(t, h)
	if out["status"] != "awaiting_payment" || out["pay_at"] == "" || out["token"] == "" {
		t.Fatalf("card path: %v", out)
	}
	if _, has := out["accepts"]; has {
		t.Error("x402 requirements on a request that did not ask")
	}
	if _, has := out["pay_usdc"]; !has {
		t.Error("pay_usdc missing")
	}
	ad, _ := out["x402"].(map[string]any)
	if ad == nil || ad["supported"] != true || ad["network"] != "base-sepolia" || ad["asset"] != chain.USDCBaseSepolia {
		t.Fatalf("x402 advert: %v", ad)
	}
	if v, st := f.counts(); v+st != 0 {
		t.Error("the facilitator was asked on the card path")
	}
	// A payment header on a request that did not ask for x402 is ignored.
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/v1/tasks", bytes.NewReader(guestJobBody(t)))
	r.Header.Set("X-PAYMENT", paymentHeader("2300000001", exchangeAddr, "0x9"))
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("header without x402 true: %d", w.Code)
	}
	if v, st := f.counts(); v+st != 0 {
		t.Error("the facilitator was asked without x402 true")
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v1/rails", nil))
	var rails map[string]map[string]any
	json.Unmarshal(w.Body.Bytes(), &rails)
	if rails["x402"]["on"] != true || rails["x402"]["network"] != "base-sepolia" ||
		rails["x402"]["facilitator"] != s.X402.Facilitator {
		t.Errorf("/v1/rails: %v", rails["x402"])
	}
}

func TestX402RefusesAPaymentForAnotherAddress(t *testing.T) {
	s, f := x402Server(t)
	h := s.Handler()
	out, req := ask(t, h)
	w := pay(t, h, paymentHeader(req["maxAmountRequired"].(string), workerAddr, "0xabc6"))
	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("payment to the wrong address: %d %s", w.Code, w.Body.String())
	}
	if v, st := f.counts(); v+st != 0 {
		t.Error("the facilitator was asked about a payment to somebody else")
	}
	if boardHas(t, h, out["job"].(string)) {
		t.Error("listed")
	}
	w = pay(t, h, "not base64 at all")
	if w.Code != http.StatusBadRequest {
		t.Errorf("garbage header: %d", w.Code)
	}
}
