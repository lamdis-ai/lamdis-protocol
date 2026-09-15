package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The cap is the whole safety property, so the request that creates a key
// must carry a limit and must never carry a reset interval: a limit that
// refills is not a cap, it is a subscription to being drained.
func TestMintedKeysAreCappedAndNeverRefill(t *testing.T) {
	var got map[string]any
	var auth, method, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, method, path = r.Header.Get("Authorization"), r.Method, r.URL.Path
		json.NewDecoder(r.Body).Decode(&got)
		json.NewEncoder(w).Encode(map[string]any{
			"key":  "sk-or-v1-minted",
			"data": map[string]any{"hash": "abc123", "name": "sam@example.com", "limit": 2.0, "usage": 0.0, "limit_remaining": 2.0},
		})
	}))
	defer srv.Close()

	p := &Provisioner{ManagementKey: "mgmt-secret", BaseURL: srv.URL}
	k, err := p.Mint(context.Background(), "sam@example.com", 2)
	if err != nil {
		t.Fatal(err)
	}
	if method != "POST" || path != "/keys" {
		t.Fatalf("wrong call: %s %s", method, path)
	}
	if auth != "Bearer mgmt-secret" {
		t.Fatalf("management key not sent: %q", auth)
	}
	if got["limit"] != 2.0 {
		t.Fatalf("no cap was requested: %+v", got)
	}
	if _, refills := got["limit_reset"]; refills {
		t.Fatalf("a refilling limit was requested, which defeats the cap: %+v", got)
	}
	if k.Secret != "sk-or-v1-minted" || k.Hash != "abc123" || k.Limit != 2 {
		t.Fatalf("minted key came back wrong: %+v", k)
	}
}

func TestMintRefusesAnUncappedKey(t *testing.T) {
	p := &Provisioner{ManagementKey: "mgmt", BaseURL: "http://127.0.0.1:1"}
	if _, err := p.Mint(context.Background(), "someone", 0); err == nil {
		t.Fatal("an uncapped key was allowed")
	}
}

func TestProvisionerNeedsAManagementKey(t *testing.T) {
	p := &Provisioner{BaseURL: "http://127.0.0.1:1"}
	_, err := p.List(context.Background())
	if err == nil || !strings.Contains(err.Error(), "management key") {
		t.Fatalf("unhelpful error: %v", err)
	}
}

func TestListReportsWhatIsLeft(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
			{"hash": "h1", "name": "sam", "limit": 2.0, "usage": 0.75, "limit_remaining": 1.25},
			{"hash": "h2", "name": "drained", "limit": 1.0, "usage": 1.0, "limit_remaining": 0.0},
		}})
	}))
	defer srv.Close()
	p := &Provisioner{ManagementKey: "mgmt", BaseURL: srv.URL}
	keys, err := p.List(context.Background())
	if err != nil || len(keys) != 2 {
		t.Fatalf("list: %v %+v", err, keys)
	}
	if keys[0].Left() != 1.25 || keys[0].Spent() != 0.75 {
		t.Fatalf("usage wrong: %+v", keys[0])
	}
	if keys[1].Left() != 0 {
		t.Fatalf("a spent-out key should have nothing left: %+v", keys[1])
	}
}

func TestRevokeCallsDelete(t *testing.T) {
	var method, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.WriteHeader(200)
	}))
	defer srv.Close()
	p := &Provisioner{ManagementKey: "mgmt", BaseURL: srv.URL}
	if err := p.Revoke(context.Background(), "abc123"); err != nil {
		t.Fatal(err)
	}
	if method != "DELETE" || path != "/keys/abc123" {
		t.Fatalf("wrong call: %s %s", method, path)
	}
	if err := p.Revoke(context.Background(), ""); err == nil {
		t.Fatal("revoking nothing should be an error")
	}
}

// The provider's own words reach the operator, because "HTTP 401" does not
// tell anyone that their management key is wrong.
func TestProviderErrorsAreReadable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "Invalid management key"}})
	}))
	defer srv.Close()
	p := &Provisioner{ManagementKey: "wrong", BaseURL: srv.URL}
	_, err := p.Mint(context.Background(), "sam", 2)
	if err == nil || !strings.Contains(err.Error(), "Invalid management key") {
		t.Fatalf("error not surfaced: %v", err)
	}
}

// A ceiling you cannot check is not a ceiling, so minting fails closed.
func TestCeilingStopsMintingAndFailsClosed(t *testing.T) {
	existing := []map[string]any{
		{"hash": "h1", "name": "a", "limit": 2.0, "usage": 0.0},
		{"hash": "h2", "name": "b", "limit": 46.0, "usage": 12.0},
	}
	fail := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			if fail {
				w.WriteHeader(500)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"data": existing})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"key": "sk-or-new",
			"data": map[string]any{"hash": "h3", "name": "c", "limit": 2.0}})
	}))
	defer srv.Close()
	p := &Provisioner{ManagementKey: "mgmt", BaseURL: srv.URL}
	ctx := context.Background()

	committed, n, err := p.Committed(ctx)
	if err != nil || committed != 48 || n != 2 {
		t.Fatalf("committed: %v %v %v", committed, n, err)
	}
	// $48 committed, a $2 key fits under $50; a $3 one does not.
	if _, err := p.MintWithin(ctx, "c", 3, 50); err == nil {
		t.Fatal("minting past the ceiling was allowed")
	} else if !strings.Contains(err.Error(), "ceiling") {
		t.Fatalf("unhelpful ceiling error: %v", err)
	}
	if _, err := p.MintWithin(ctx, "c", 2, 50); err != nil {
		t.Fatalf("a key that fits was refused: %v", err)
	}
	// If the existing keys cannot be counted, nothing is minted.
	fail = true
	if _, err := p.MintWithin(ctx, "c", 1, 50); err == nil {
		t.Fatal("minted without being able to check the ceiling")
	}
	// A disabled key no longer counts against the ceiling.
	fail = false
	existing[1]["disabled"] = true
	if committed, _, _ = p.Committed(ctx); committed != 2 {
		t.Fatalf("a disabled key still counted: $%.2f", committed)
	}
}
