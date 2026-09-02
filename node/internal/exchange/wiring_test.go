package exchange

import (
	"crypto/ed25519"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The test that was missing.
//
// Every other test in this repository builds the component it is testing and
// hands it its dependencies, so all of them passed while the running service
// had none of them. A ledger that is never attached still has a green ledger
// suite; an agent surface that never mounts still has passing agent tests.
//
// This asserts the thing no unit test can: that a server built the way the
// binary builds it is actually able to do the job.
func TestServerBuiltForProductionHasItsSubsystems(t *testing.T) {
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	srv, err := NewServer(key, "https://example.test")
	if err != nil {
		t.Fatal(err)
	}

	// Whatever else is optional, these three are not. Without a ledger there
	// is no escrow and every job lists unfunded; without accounts the agent
	// surface never mounts and every MCP tool is a 404; without a verifier
	// nothing is ever checked and no submission can ever be paid.
	missing := []string{}
	if srv.Ledger == nil {
		missing = append(missing, "Ledger (no escrow, no balances, nothing to pay from)")
	}
	if srv.Accounts == nil {
		missing = append(missing, "Accounts (the agent and MCP surface does not mount)")
	}
	if srv.Verify == nil {
		missing = append(missing, "Verify (no submission can ever become payable)")
	}
	if len(missing) > 0 {
		for _, m := range missing {
			t.Errorf("a production server has no %s", m)
		}
	}
}

// The routes an agent needs must actually be mounted, not merely written.
//
// Checked by asking the real mux, with the real method, because a route
// registered inside a nil-guarded block compiles perfectly and serves 404.
func TestAgentRoutesAreMounted(t *testing.T) {
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	srv, err := NewServer(key, "https://example.test")
	if err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()

	// Every path an MCP tool calls. A tool that reaches a 404 does not exist,
	// however well it is described.
	for _, tc := range []struct{ method, path string }{
		{"POST", "/v1/tasks"},
		{"GET", "/v1/jobs/j1"},
		{"GET", "/v1/jobs/j1/bids"},
		{"POST", "/v1/jobs/j1/award"},
		{"GET", "/v1/jobs/j1/receipt"},
		{"GET", "/v1/agent/balance"},
	} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		// 401 is fine — the route exists and wants credentials. 404 and 405
		// mean nothing is listening.
		if w.Code == http.StatusNotFound || w.Code == http.StatusMethodNotAllowed {
			t.Errorf("%s %s is not mounted (%d); the tool that calls it is broken",
				tc.method, tc.path, w.Code)
		}
	}
}

// The exchange host is the product, not a second sales page. Serving the pitch
// here meant somebody who clicked "open the exchange" arrived back at the
// pitch and thought the link was broken.
func TestExchangeRootGoesToTheBoard(t *testing.T) {
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	srv, err := NewServer(key, "https://example.test")
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, r)

	if w.Code != http.StatusFound {
		t.Fatalf("the exchange root returned %d, not a redirect", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/board" {
		t.Fatalf("the root sends people to %q", loc)
	}
}

// One fee, not two. Order.Defaults used to fill in 500 basis points while
// settlement charged FeeBP, so every quote stated a cut nobody was taking.
func TestOrderDefaultsQuoteTheFeeSettlementCharges(t *testing.T) {
	if got := (Order{}).Defaults().FeeBP; got != FeeBP {
		t.Fatalf("default order fee is %d bp, settlement charges %d bp", got, FeeBP)
	}
}
