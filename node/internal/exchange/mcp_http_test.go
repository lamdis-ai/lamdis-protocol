package exchange

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The one property that makes a shared endpoint safe: the credential comes
// from the request, never from the server.
//
// Getting this wrong looks almost identical in code — one Exchange built at
// startup instead of one per request — and the consequence is that every
// caller spends whoever's key the server happened to hold.
func TestEachCallerGetsTheirOwnCredential(t *testing.T) {
	s := &Server{BaseURL: "https://exchange.example"}

	for _, tc := range []struct{ header, value, want string }{
		{"Authorization", "Bearer lam_alice", "lam_alice"},
		{"Authorization", "bearer lam_bob", "lam_bob"},
		{"X-Lamdis-Key", "lam_carol", "lam_carol"},
		{"Authorization", "  Bearer   lam_dave  ", "lam_dave"},
	} {
		r := httptest.NewRequest("POST", "/mcp", nil)
		r.Header.Set(tc.header, tc.value)
		if got := agentKeyFrom(r); got != tc.want {
			t.Errorf("%s: %q -> %q, want %q", tc.header, tc.value, got, tc.want)
		}
	}

	// Two requests, two different keys, two different servers.
	a := httptest.NewRequest("POST", "/mcp", nil)
	a.Header.Set("Authorization", "Bearer lam_alice")
	b := httptest.NewRequest("POST", "/mcp", nil)
	b.Header.Set("Authorization", "Bearer lam_bob")
	if s.mcpServerFor(a) == s.mcpServerFor(b) {
		t.Fatal("two callers were handed the same server instance")
	}
}

// An unauthenticated caller is not refused any more: they reach the guest
// tools, which can read, check feasibility, and post a job that comes back
// with a pay link. The gate that used to stand here lost everybody who was
// only going to spend ten minutes finding out whether this was interesting.
func TestTheEndpointWelcomesWithoutAKey(t *testing.T) {
	s := &Server{BaseURL: "https://exchange.example"}
	reached := false
	h := s.requireAgentKey(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/mcp", nil))
	if !reached || w.Code == http.StatusUnauthorized {
		t.Fatalf("an unauthenticated request was refused (%d); it should reach the guest tools", w.Code)
	}

	// And what it gets is the guest surface, not the account surface: no
	// balance, no projects, no vendors — those presume somebody to bill.
	srv := s.mcpServerFor(httptest.NewRequest("POST", "/mcp", nil))
	if srv == nil {
		t.Fatal("no server for a guest")
	}
}

// A bad key is still refused, plainly, with the next step.
func TestABadKeyIsRefusedWithTheNextStep(t *testing.T) {
	s := &Server{BaseURL: "https://exchange.lamdis.ai"}
	h := s.requireAgentKey(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("a bad credential reached the tools")
	}))
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", mcpPath, nil)
	r.Header.Set("Authorization", "Bearer lam_nonsense")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", w.Code)
	}
	for _, want := range []string{"/console", "/signin"} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("the refusal does not mention %q: %s", want, w.Body.String())
		}
	}
}
