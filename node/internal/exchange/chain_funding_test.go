package exchange

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/chain"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/ledger"
)

// The stablecoin rail, end to end, against a fake chain. No network.

const (
	exchangeAddr = "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed"
	payerAddr    = "0xfB6916095ca1df60bB79Ce92cE3Ea74c37c5d359"
	workerAddr   = "0xdbF03B407c01E7cD3CBea99509d93f8DDDC8C6FB"
)

// fakeChain answers eth_blockNumber and eth_getLogs from canned transfers.
type fakeChain struct {
	mu   sync.Mutex
	head uint64
	logs []map[string]any
}

func topic(addr string) string {
	return "0x" + strings.Repeat("0", 24) + strings.ToLower(strings.TrimPrefix(addr, "0x"))
}

func (f *fakeChain) transfer(block uint64, tx, from string, units int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs = append(f.logs, map[string]any{
		"address":         chain.USDCBaseSepolia,
		"topics":          []string{chain.TransferTopic, topic(from), topic(exchangeAddr)},
		"data":            fmt.Sprintf("0x%064x", units),
		"blockNumber":     fmt.Sprintf("0x%x", block),
		"transactionHash": tx,
		"logIndex":        "0x0",
	})
}

func (f *fakeChain) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID     int    `json:"id"`
		Method string `json:"method"`
		Params []any  `json:"params"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	f.mu.Lock()
	defer f.mu.Unlock()
	var result any
	switch req.Method {
	case "eth_blockNumber":
		result = fmt.Sprintf("0x%x", f.head)
	case "eth_getLogs":
		filter := req.Params[0].(map[string]any)
		var from, to uint64
		fmt.Sscanf(filter["fromBlock"].(string), "0x%x", &from)
		fmt.Sscanf(filter["toBlock"].(string), "0x%x", &to)
		out := []map[string]any{}
		for _, l := range f.logs {
			var b uint64
			fmt.Sscanf(l["blockNumber"].(string), "0x%x", &b)
			if b >= from && b <= to {
				out = append(out, l)
			}
		}
		result = out
	}
	json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
}

// usdcServer is an exchange with the stablecoin rail on, watching a fake chain.
func usdcServer(t *testing.T) (*Server, *fakeChain) {
	t.Helper()
	s := consoleServer(t)
	f := &fakeChain{head: 1000}
	rpc := httptest.NewServer(f)
	t.Cleanup(rpc.Close)
	rail, err := NewUSDCRail(chain.BaseSepolia, rpc.URL, exchangeAddr, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.USDC = rail
	// Take bearings so the next scan reads new blocks.
	if _, err := s.ScanChain(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s, f
}

func TestTheRailIsOffUnlessConfigured(t *testing.T) {
	s := consoleServer(t)
	if s.USDC != nil {
		t.Fatal("the USDC rail is on with nothing configured")
	}
	out := postAsNobody(t, s.Handler())
	if _, has := out["pay_usdc"]; has {
		t.Error("a USDC quote was given with the rail off")
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/v1/rails", nil))
	var rails map[string]map[string]any
	json.Unmarshal(w.Body.Bytes(), &rails)
	if w.Code != http.StatusOK || rails["usdc"]["on"] != false {
		t.Errorf("/v1/rails: %d %s", w.Code, w.Body.String())
	}
	t.Setenv("LAMDIS_USDC_ADDRESS", exchangeAddr)
	if r, err := NewUSDCRailFromEnv(""); r != nil || err != nil {
		t.Error("an address with no RPC switched the rail on")
	}
}

func TestATransferOfTheExactAmountFundsTheJobOnceConfirmed(t *testing.T) {
	s, f := usdcServer(t)
	h := s.Handler()
	ctx := context.Background()

	out := postAsNobody(t, h)
	job := out["job"].(string)
	amount := int64(out["amount_minor"].(float64))
	q, _ := out["pay_usdc"].(map[string]any)
	if q == nil {
		t.Fatalf("no pay_usdc in the reply: %v", out)
	}
	units := chain.UnitsFor(job, amount)
	if q["address"] != exchangeAddr || q["chain"] != "base-sepolia" ||
		q["contract"] != chain.USDCBaseSepolia || q["amount_usdc"] != chain.Format(units) {
		t.Fatalf("quote: %v", q)
	}
	if !strings.HasPrefix(q["amount_usdc"].(string), "23.000") || q["amount_usdc"] == "23.000000" {
		t.Errorf("the amount should be the ceiling plus dust: %v", q["amount_usdc"])
	}

	// Somebody else pays a similar job the wrong amount: nothing lists.
	f.transfer(1001, "0x"+strings.Repeat("1", 64), payerAddr, amount*chain.UnitsPerCent)
	// The right amount lands, unconfirmed.
	f.transfer(1002, "0x"+strings.Repeat("a", 64), payerAddr, units)
	f.head = 1005
	if n, err := s.ScanChain(ctx); err != nil || n != 0 {
		t.Fatalf("unconfirmed: %d %v", n, err)
	}
	if boardHas(t, h, job) {
		t.Fatal("listed before the transfer was confirmed")
	}
	f.head = 1002 + chain.Confirmations
	if n, err := s.ScanChain(ctx); err != nil || n != 1 {
		t.Fatalf("confirmed: %d %v", n, err)
	}
	if !boardHas(t, h, job) {
		t.Fatal("a paid job is not on the board")
	}
	held, err := s.Ledger.Held(ctx, job, "USD")
	if err != nil || held != amount {
		t.Fatalf("escrow %d (%v), want %d", held, err, amount)
	}
	l, _ := s.Board.Get(job)
	if l.Funding == nil || l.Funding.Kind != "usdc" || l.Funding.Intent != "0x"+strings.Repeat("a", 64) ||
		l.Funding.Payer != payerAddr {
		t.Fatalf("funding not recorded: %+v", l.Funding)
	}
	if un := s.USDC.Watcher.Unmatched(); len(un) != 1 || un[0].Units != amount*chain.UnitsPerCent {
		t.Errorf("the wrong-amount transfer should be held for a person: %+v", un)
	}

	// Idempotent on the hash: another scan, and a direct repeat, move nothing.
	f.head += 50
	if _, err := s.ScanChain(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.FundFromChain(ctx, job, l.Funding.Intent, amount, payerAddr); err != nil {
		t.Fatal(err)
	}
	if bal, _ := s.Ledger.Balance(ctx, ledger.BalanceOf(l.Owner), "USD"); bal != 0 {
		t.Errorf("guest balance %d: the transfer was credited more than once", bal)
	}

	// The job pays out 800 of its ceiling; the rest is owed back to the
	// sender, through the queue, and nothing is left in the guest principal.
	if _, err := s.Ledger.Capture(ctx, "settle:"+job+":w", job, "worker-1", 800, 0, "USD"); err != nil {
		t.Fatal(err)
	}
	if err := s.releaseIfDoneNow(ctx, l); err != nil {
		t.Fatal(err)
	}
	if bal, _ := s.Ledger.Balance(ctx, ledger.BalanceOf(l.Owner), "USD"); bal != 0 {
		t.Errorf("guest balance %d after settlement; want 0", bal)
	}
	items := s.USDC.Queue.List()
	if len(items) != 1 || items[0].Address != payerAddr || items[0].AmountMinor != amount-800 ||
		items[0].Reason != "refund:"+job {
		t.Fatalf("refund queue: %+v", items)
	}
	if err := s.releaseIfDoneNow(ctx, l); err != nil {
		t.Fatal(err)
	}
	if len(s.USDC.Queue.List()) != 1 {
		t.Error("a second settlement queued a second refund")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v1/rails", nil))
	var rails map[string]map[string]any
	json.Unmarshal(w.Body.Bytes(), &rails)
	u := rails["usdc"]
	if u["on"] != true || u["address"] != exchangeAddr || u["queue_length"].(float64) != 1 ||
		u["last_scanned_block"].(float64) != float64(f.head-chain.Confirmations) ||
		u["confirmations"].(float64) != chain.Confirmations {
		t.Errorf("/v1/rails: %v", u)
	}
}

// earn gives a worker a payable balance the way settlement would.
func earn(t *testing.T, s *Server, worker string, amount int64) {
	t.Helper()
	ctx := context.Background()
	key := fmt.Sprintf("earn-%s-%d-%d", worker, amount, time.Now().UnixNano())
	if _, err := s.Ledger.Topup(ctx, key+"-t", "buyer-x", amount, "USD", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Ledger.Hold(ctx, key+"-h", key, "buyer-x", amount, "USD"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Ledger.Capture(ctx, key+"-c", key, worker, amount, 0, "USD"); err != nil {
		t.Fatal(err)
	}
}

func TestEarningsRouteToTheQueueWhenAnAddressIsSet(t *testing.T) {
	s, _ := usdcServer(t)
	s.Holdbacks = nil
	h := s.Handler()
	ctx := context.Background()
	priv, worker := verifiedPerson(t, s)

	// A wrong checksum is refused; a right one is stored in checksum form.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, signedReq(t, priv, "PUT", "/v1/payout/usdc",
		map[string]any{"address": strings.TrimSuffix(workerAddr, "B") + "b"}))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("a bad checksum was accepted: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, signedReq(t, priv, "PUT", "/v1/payout/usdc",
		map[string]any{"address": strings.ToLower(workerAddr)}))
	if w.Code != http.StatusOK {
		t.Fatalf("set address: %d %s", w.Code, w.Body.String())
	}
	if got, _ := s.USDC.Addresses.Get(worker); got != workerAddr {
		t.Fatalf("stored %q", got)
	}
	// Nobody signed in cannot set one.
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("PUT", "/v1/payout/usdc", bytes.NewReader([]byte(`{"address":"`+workerAddr+`"}`))))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous PUT: %d", w.Code)
	}

	earn(t, s, worker, 5000)
	// No card rail at all, and the worker is still paid: into the queue.
	if paid, total := s.SweepPayouts(ctx); paid != 1 || total != 5000 {
		t.Fatalf("sweep: %d %d", paid, total)
	}
	items := s.USDC.Queue.List()
	if len(items) != 1 || items[0].Person != worker || items[0].Address != workerAddr ||
		items[0].AmountMinor != 5000 || items[0].Reason != "earnings" || items[0].SentTx != "" {
		t.Fatalf("queue: %+v", items)
	}
	if owed, _ := s.Ledger.Balance(ctx, ledger.PayableOf(worker), "USD"); owed != 0 {
		t.Errorf("still owed %d after queueing", owed)
	}
	if ref, _ := s.Ledger.Ref(ctx, items[0].LedgerKey); ref != "usdc-queued:"+items[0].ID {
		t.Errorf("ledger ref %q", ref)
	}
	// Sweeping again queues nothing more.
	if paid, _ := s.SweepPayouts(ctx); paid != 0 {
		t.Error("swept the same earnings twice")
	}

	// The status the console reads.
	w = httptest.NewRecorder()
	h.ServeHTTP(w, signedReq(t, priv, "GET", "/v1/payout", nil))
	var st map[string]any
	json.Unmarshal(w.Body.Bytes(), &st)
	if st["usdc_cap_minor"].(float64) != USDCLifetimeCapMinor || st["usdc_paid_minor"].(float64) != 5000 ||
		st["usdc_address"] != workerAddr {
		t.Errorf("/v1/payout: %v", st)
	}

	// The queue is for a signed principal; a person's session is not enough.
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v1/payout/usdc/queue", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous queue read: %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, signedReq(t, s.Key, "GET", "/v1/payout/usdc/queue", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "sent by a person") {
		t.Fatalf("queue: %d %s", w.Code, w.Body.String())
	}
	// A person sends it and says so.
	tx := "0x" + strings.Repeat("b", 64)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, signedReq(t, s.Key, "POST", "/v1/payout/usdc/queue/"+items[0].ID+"/sent", map[string]any{"tx": tx}))
	if w.Code != http.StatusOK {
		t.Fatalf("mark sent: %d %s", w.Code, w.Body.String())
	}
	if got := s.USDC.Queue.List()[0]; got.SentTx != tx {
		t.Errorf("not marked: %+v", got)
	}
	if ref, _ := s.Ledger.Ref(ctx, items[0].LedgerKey); ref != "usdc-sent:"+tx {
		t.Errorf("ledger ref after send %q", ref)
	}
	if s.USDC.Queue.Unsent() != 0 {
		t.Error("still counted as unsent")
	}
	// Survives a restart.
	again := NewUSDCQueue(strings.TrimSuffix(s.USDC.Queue.path, "/usdc-queue.json"))
	if got := again.List(); len(got) != 1 || got[0].SentTx != tx {
		t.Errorf("queue not persisted: %+v", got)
	}
}

func TestTheAddressCapHoldsUntilAPayoutAccountIsConnected(t *testing.T) {
	s, _ := usdcServer(t)
	s.Holdbacks = nil
	ctx := context.Background()
	_, worker := verifiedPerson(t, s)
	if err := s.USDC.Addresses.Put(worker, workerAddr); err != nil {
		t.Fatal(err)
	}
	earn(t, s, worker, USDCLifetimeCapMinor-1000)
	if paid, _ := s.SweepPayouts(ctx); paid != 1 {
		t.Fatal("under the cap was not paid")
	}
	earn(t, s, worker, 2000)
	if paid, _ := s.SweepPayouts(ctx); paid != 0 {
		t.Fatal("paid over the lifetime cap with no identity")
	}
	if owed, _ := s.Ledger.Balance(ctx, ledger.PayableOf(worker), "USD"); owed != 2000 {
		t.Errorf("the held earnings should still be owed: %d", owed)
	}
	// Connect goes through: the cap lifts and the address still wins.
	s.Rail = stubRail{}
	if err := s.PayoutAccounts.Put(worker, "acct_stub"); err != nil {
		t.Fatal(err)
	}
	if paid, total := s.SweepPayouts(ctx); paid != 1 || total != 2000 {
		t.Fatalf("after connect: %d %d", paid, total)
	}
	items := s.USDC.Queue.List()
	if len(items) != 2 || items[1].AmountMinor != 2000 {
		t.Errorf("queue: %+v", items)
	}
	if s.USDC.Queue.PaidTo(worker) != USDCLifetimeCapMinor+1000 {
		t.Errorf("paid %d", s.USDC.Queue.PaidTo(worker))
	}
}

func TestUSDCRoutesAreMounted(t *testing.T) {
	s := consoleServer(t)
	h := s.Handler()
	for _, rt := range []struct{ method, path string }{
		{"GET", "/v1/payout/usdc"}, {"PUT", "/v1/payout/usdc"},
		{"GET", "/v1/payout/usdc/queue"}, {"POST", "/v1/payout/usdc/queue/q1/sent"},
		{"GET", "/v1/rails"},
	} {
		req, _ := http.NewRequest(rt.method, "https://example.test"+rt.path, nil)
		if _, pattern := h.Handler(req); pattern == "" {
			t.Errorf("%s %s is not mounted", rt.method, rt.path)
		}
	}
}
