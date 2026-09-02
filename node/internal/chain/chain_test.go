package chain

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// No network. Every test speaks to a fake RPC in-process.

func TestKeccakMatchesTheKnownVectors(t *testing.T) {
	for in, want := range map[string]string{
		"":                                  "c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470",
		"abc":                               "4e03657aea45a94fc7d47ba826c8d667c0d1e6e33a64a036ec44f58fa12d6c45",
		"Transfer(address,address,uint256)": strings.TrimPrefix(TransferTopic, "0x"),
	} {
		h := Keccak256([]byte(in))
		if got := hex.EncodeToString(h[:]); got != want {
			t.Errorf("keccak(%q) = %s, want %s", in, got, want)
		}
	}
	// Longer than one block of the sponge: 200 bytes spans two absorptions.
	h := Keccak256([]byte(strings.Repeat("a", 200)))
	if got := hex.EncodeToString(h[:]); got != "96ea54061def936c4be90b518992fdc6f12f535068a256229aca54267b4d084d" {
		t.Errorf("keccak(a*200) = %s", got)
	}
}

func TestAddressesAreChecksummed(t *testing.T) {
	// The EIP-55 reference vectors.
	for _, a := range []string{
		"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",
		"0xfB6916095ca1df60bB79Ce92cE3Ea74c37c5d359",
		"0xdbF03B407c01E7cD3CBea99509d93f8DDDC8C6FB",
		"0xD1220A0cf47c7B9Be7A2E6BA89F429762e7b9aDb",
	} {
		if Checksum(a) != a {
			t.Errorf("checksum(%s) = %s", a, Checksum(a))
		}
		if err := ValidAddress(a); err != nil {
			t.Errorf("%s: %v", a, err)
		}
		if err := ValidAddress(strings.ToLower(a)); err != nil {
			t.Errorf("lowercase %s should be accepted: %v", a, err)
		}
	}
	// One flipped character in a mixed-case address is refused.
	bad := "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAeD"
	if err := ValidAddress(bad); err == nil {
		t.Error("a wrong checksum was accepted")
	}
	for _, s := range []string{"", "0x12", "5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",
		"0xZZAeb6053F3E94C9b9A09f33669435E7Ef1BeAed"} {
		if err := ValidAddress(s); err == nil {
			t.Errorf("%q was accepted", s)
		}
	}
	if err := ValidAddress(USDCBase); err != nil {
		t.Errorf("the mainnet contract constant fails its own checksum: %v", err)
	}
	if err := ValidAddress(USDCBaseSepolia); err != nil {
		t.Errorf("the sepolia contract constant fails its own checksum: %v", err)
	}
}

func TestDustIsSmallUniqueAndStable(t *testing.T) {
	seen := map[int64]string{}
	for i := 0; i < 200; i++ {
		job := fmt.Sprintf("job-%d", i)
		d := Dust(job)
		if d < 1 || d > 999 {
			t.Fatalf("dust for %s is %d", job, d)
		}
		if Dust(job) != d {
			t.Fatal("dust is not stable")
		}
		seen[d] = job
	}
	if len(seen) < 150 {
		t.Errorf("only %d distinct dust values across 200 jobs", len(seen))
	}
	if got := UnitsFor("j", 2300); got != 2300*UnitsPerCent+Dust("j") {
		t.Errorf("units %d", got)
	}
	if Format(23_000_017) != "23.000017" || Format(5) != "0.000005" {
		t.Errorf("format: %s %s", Format(23_000_017), Format(5))
	}
	if MinorOf(UnitsFor("j", 2300)) != 2300 {
		t.Error("minor of units did not drop the dust")
	}
}

// fakeRPC answers eth_blockNumber and eth_getLogs from canned data.
type fakeRPC struct {
	mu    sync.Mutex
	head  uint64
	logs  []map[string]any
	calls []string
}

func (f *fakeRPC) add(block uint64, tx string, from, to string, units int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs = append(f.logs, map[string]any{
		"address":         strings.ToLower(USDCBase),
		"topics":          []string{TransferTopic, topicFor(from), topicFor(to)},
		"data":            fmt.Sprintf("0x%064x", units),
		"blockNumber":     hexUint(block),
		"transactionHash": tx,
		"logIndex":        "0x0",
	})
}

func (f *fakeRPC) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req rpcRequest
	json.NewDecoder(r.Body).Decode(&req)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, req.Method)
	var result any
	switch req.Method {
	case "eth_blockNumber":
		result = hexUint(f.head)
	case "eth_getLogs":
		filter := req.Params[0].(map[string]any)
		from, _ := parseHexUint(filter["fromBlock"].(string))
		to, _ := parseHexUint(filter["toBlock"].(string))
		topics := filter["topics"].([]any)
		var out []map[string]any
		for _, l := range f.logs {
			b, _ := parseHexUint(l["blockNumber"].(string))
			if b < from || b > to {
				continue
			}
			if want, _ := topics[2].(string); want != "" && l["topics"].([]string)[2] != want {
				continue
			}
			out = append(out, l)
		}
		if out == nil {
			out = []map[string]any{}
		}
		result = out
	default:
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID,
			"error": map[string]any{"code": -32601, "message": "no such method"}})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
}

const (
	us    = "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed"
	payer = "0xfB6916095ca1df60bB79Ce92cE3Ea74c37c5d359"
)

func testClient(t *testing.T, f *fakeRPC) *Client {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, USDCBase, us)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestClientReadsTransfersToUs(t *testing.T) {
	f := &fakeRPC{head: 100}
	f.add(50, "0xaa", payer, us, 23_000_017)
	f.add(51, "0xbb", payer, "0xdbF03B407c01E7cD3CBea99509d93f8DDDC8C6FB", 5_000_000)
	c := testClient(t, f)
	ctx := context.Background()
	if n, err := c.BlockNumber(ctx); err != nil || n != 100 {
		t.Fatalf("head %d %v", n, err)
	}
	got, err := c.Transfers(ctx, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Units != 23_000_017 || got[0].From != payer ||
		got[0].To != us || got[0].Block != 50 || got[0].TxHash != "0xaa" {
		t.Fatalf("transfers: %+v", got)
	}
	// A wide range is asked for in pieces the RPC will accept.
	f.calls = nil
	if _, err := c.Transfers(ctx, 1, MaxLogRange*3); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 3 {
		t.Errorf("%d calls for a range of three chunks", len(f.calls))
	}
}

func TestClientRefusesWhatTheFilterShouldHaveExcluded(t *testing.T) {
	c := &Client{Contract: USDCBase, Recipient: us}
	base := rpcLog{Address: USDCBase, Topics: []string{TransferTopic, topicFor(payer), topicFor(us)},
		Data: "0x" + fmt.Sprintf("%064x", 100), BlockNumber: "0x10", TxHash: "0x1", LogIndex: "0x0"}
	if _, ok := c.transferOf(base); !ok {
		t.Fatal("a good log was refused")
	}
	other := base
	other.Address = USDCBaseSepolia
	if _, ok := c.transferOf(other); ok {
		t.Error("a log from another contract was accepted")
	}
	removed := base
	removed.Removed = true
	if _, ok := c.transferOf(removed); ok {
		t.Error("a reorged-out log was accepted")
	}
	huge := base
	huge.Data = "0x" + strings.Repeat("f", 64)
	if _, ok := c.transferOf(huge); ok {
		t.Error("an amount beyond int64 was accepted")
	}
}

func TestWatcherWaitsForConfirmationsAndOffersEachTransferOnce(t *testing.T) {
	f := &fakeRPC{head: 100}
	c := testClient(t, f)
	dir := t.TempDir()
	w := NewWatcher(c, dir, Confirmations)
	ctx := context.Background()
	var offered []string
	handle := func(_ context.Context, tr Transfer) (string, bool, error) {
		offered = append(offered, tr.TxHash)
		if tr.Units == 23_000_017 {
			return "job-1", true, nil
		}
		return "", false, nil
	}
	// First scan only takes its bearings.
	if n, err := w.Scan(ctx, handle); err != nil || n != 0 {
		t.Fatalf("first scan: %d %v", n, err)
	}
	if w.LastBlock() != 100-Confirmations {
		t.Fatalf("last block %d", w.LastBlock())
	}
	// A transfer lands at the head: not yet confirmed.
	f.add(101, "0xaa", payer, us, 23_000_017)
	f.head = 105
	if n, err := w.Scan(ctx, handle); err != nil || n != 0 || len(offered) != 0 {
		t.Fatalf("unconfirmed transfer was offered: %d %v %v", n, err, offered)
	}
	// Twelve blocks later it is.
	f.head = 113
	if n, err := w.Scan(ctx, handle); err != nil || n != 1 {
		t.Fatalf("confirmed scan: %d %v", n, err)
	}
	if len(offered) != 1 {
		t.Fatalf("offered %v", offered)
	}
	// Never again, including after a restart from disk.
	f.head = 200
	if _, err := w.Scan(ctx, handle); err != nil {
		t.Fatal(err)
	}
	w2 := NewWatcher(c, dir, Confirmations)
	if w2.LastBlock() != 200-Confirmations {
		t.Errorf("position not persisted: %d", w2.LastBlock())
	}
	if label, ok := w2.Seen("0xaa:0"); !ok || label != "job-1" {
		t.Errorf("matched tx not persisted: %q %v", label, ok)
	}
	f.head = 300
	if _, err := w2.Scan(ctx, handle); err != nil {
		t.Fatal(err)
	}
	if len(offered) != 1 {
		t.Errorf("a handled transfer was offered again: %v", offered)
	}
	// Money for no job is kept for a person to return.
	f.add(350, "0xcc", payer, us, 7_000_000)
	f.head = 400
	if _, err := w2.Scan(ctx, handle); err != nil {
		t.Fatal(err)
	}
	if un := w2.Unmatched(); len(un) != 1 || un[0].TxHash != "0xcc" {
		t.Errorf("unmatched: %+v", un)
	}
}

func TestWatcherDoesNotAdvancePastAFailedHandler(t *testing.T) {
	f := &fakeRPC{head: 100}
	c := testClient(t, f)
	w := NewWatcher(c, "", Confirmations)
	ctx := context.Background()
	w.Scan(ctx, nil)
	f.add(90, "0xaa", payer, us, 1_000_001)
	f.head = 120
	fail := true
	tries := 0
	handle := func(_ context.Context, tr Transfer) (string, bool, error) {
		tries++
		if fail {
			return "", false, fmt.Errorf("ledger down")
		}
		return "job", true, nil
	}
	if _, err := w.Scan(ctx, handle); err == nil {
		t.Fatal("a failing handler did not fail the scan")
	}
	fail = false
	if n, err := w.Scan(ctx, handle); err != nil || n != 1 || tries != 2 {
		t.Fatalf("retry: n=%d tries=%d %v", n, tries, err)
	}
}
