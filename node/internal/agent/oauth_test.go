package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// oauthServer stands in for a server that wants people to sign in: it
// refuses an unauthenticated call, points at its own metadata, registers
// whoever asks, and issues tokens.
func oauthServer(t *testing.T) (*httptest.Server, *int) {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	refreshes := 0

	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("WWW-Authenticate",
				`Bearer resource_metadata="`+srv.URL+`/.well-known/oauth-protected-resource"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	})
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"resource":              srv.URL + "/mcp",
			"authorization_servers": []string{srv.URL},
			"scopes_supported":      []string{"read", "write"},
		})
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 srv.URL,
			"authorization_endpoint": srv.URL + "/authorize",
			"token_endpoint":         srv.URL + "/token",
			"registration_endpoint":  srv.URL + "/register",
		})
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		if in["token_endpoint_auth_method"] != "none" {
			t.Errorf("a public client must not claim a secret: %v", in["token_endpoint_auth_method"])
		}
		json.NewEncoder(w).Encode(map[string]any{"client_id": "client-123"})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			if r.Form.Get("code_verifier") == "" {
				t.Error("no proof the caller started the flow")
			}
			if r.Form.Get("resource") == "" {
				t.Error("the token was not bound to a resource")
			}
			json.NewEncoder(w).Encode(map[string]any{
				"access_token": "access-1", "refresh_token": "refresh-1", "expires_in": 3600})
		case "refresh_token":
			refreshes++
			json.NewEncoder(w).Encode(map[string]any{"access_token": "access-2", "expires_in": 3600})
		default:
			w.WriteHeader(400)
		}
	})
	return srv, &refreshes
}

// The whole point: a person connects a server that needs authentication
// without ever seeing, typing or storing a secret.
func TestSigningInToAServerWithoutTypingASecret(t *testing.T) {
	srv, refreshes := oauthServer(t)
	ctx := context.Background()

	cfg, err := DiscoverOAuth(ctx, srv.URL+"/mcp", true)
	if err != nil {
		t.Fatalf("discovery: %v", err)
	}
	if cfg == nil {
		t.Fatal("the server asked for authentication and discovery said it did not")
	}
	if cfg.TokenURL != srv.URL+"/token" || cfg.AuthURL != srv.URL+"/authorize" {
		t.Fatalf("wrong endpoints: %+v", cfg)
	}
	if cfg.Resource != srv.URL+"/mcp" {
		t.Fatalf("resource not picked up: %q", cfg.Resource)
	}

	redirect := "https://app.lamdis.ai/app/api/tools/auth/done"
	if err := cfg.Register(ctx, redirect, "Lamdis"); err != nil {
		t.Fatalf("registration: %v", err)
	}
	if cfg.ClientID != "client-123" {
		t.Fatalf("client id: %q", cfg.ClientID)
	}

	v := Verifier()
	u, err := url.Parse(cfg.Authorize(redirect, v, "state-1"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
		t.Fatalf("the request was not bound to this attempt: %v", q)
	}
	if q.Get("code_challenge") == v {
		t.Fatal("the verifier itself was sent instead of its hash")
	}
	if q.Get("redirect_uri") != redirect || q.Get("state") != "state-1" {
		t.Fatalf("authorize query wrong: %v", q)
	}

	if err := cfg.ExchangeCode(ctx, "code-1", v, redirect); err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if cfg.Access != "access-1" || cfg.Refresh != "refresh-1" || !cfg.Connected() {
		t.Fatalf("nothing was granted: %+v", cfg)
	}

	// A live token is left alone; an expired one is renewed quietly.
	if err := cfg.EnsureFresh(ctx); err != nil || *refreshes != 0 {
		t.Fatalf("a good token was refreshed anyway: %v %d", err, *refreshes)
	}
	cfg.Expiry = time.Now().Add(-time.Minute)
	if err := cfg.EnsureFresh(ctx); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if cfg.Access != "access-2" || *refreshes != 1 {
		t.Fatalf("it did not renew: %s after %d refreshes", cfg.Access, *refreshes)
	}
	// Losing the refresh token is said plainly rather than failing later.
	cfg.Refresh, cfg.Expiry = "", time.Now().Add(-time.Minute)
	if err := cfg.EnsureFresh(ctx); err == nil || !strings.Contains(err.Error(), "connect it again") {
		t.Fatalf("unhelpful expiry: %v", err)
	}
}

// A server that wants nothing should not be dressed up as one that does.
func TestAnOpenServerNeedsNoSignIn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer srv.Close()
	cfg, err := DiscoverOAuth(context.Background(), srv.URL+"/mcp", true)
	if err != nil || cfg != nil {
		t.Fatalf("an open server was treated as protected: %+v %v", cfg, err)
	}
}

// A hosted node must not be talked into fetching its own insides.
func TestDiscoveryRefusesTheHostsOwnMachine(t *testing.T) {
	for _, u := range []string{"http://127.0.0.1:8080/mcp", "http://localhost:9000/mcp"} {
		if _, err := DiscoverOAuth(context.Background(), u, false); err == nil {
			t.Fatalf("%s was reachable from a hosted node", u)
		}
	}
	for _, u := range []string{"https://10.0.0.5/mcp", "https://192.168.1.1/mcp", "http://169.254.169.254/mcp"} {
		if _, err := DiscoverOAuth(context.Background(), u, true); err == nil {
			t.Fatalf("%s was reachable even locally", u)
		}
	}
}
