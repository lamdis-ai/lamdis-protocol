package api

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testHost(t *testing.T) (*Host, *rsa.PrivateKey, map[string]any, http.Handler) {
	t.Helper()
	c, key, claims := newCognito(t)
	h := &Host{Root: t.TempDir(), Cognito: c, Model: "test-model", MaxAccounts: 2,
		Logf: func(f string, a ...any) {}}
	if err := h.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Close)
	return h, key, claims, h.Handler()
}

func as(t *testing.T, key *rsa.PrivateKey, claims map[string]any, sub, email string) string {
	t.Helper()
	c := map[string]any{}
	for k, v := range claims {
		c[k] = v
	}
	c["sub"], c["email"] = sub, email
	return signedToken(t, key, "k1", c, "RS256")
}

func call(h http.Handler, method, path, token string, body string) *httptest.ResponseRecorder {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// Two people on one host are two nodes. Nothing either writes is visible to
// the other, and neither can reach the other's account by asking.
func TestHostKeepsAccountsApart(t *testing.T) {
	h, key, claims, handler := testHost(t)

	sam := as(t, key, claims, "sam-subject", "sam@example.com")
	kim := as(t, key, claims, "kim-subject", "kim@example.com")

	// Sam writes a thread.
	w := call(handler, "POST", "/app/api/threads", sam, `{"title":"Sam's deal"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("sam could not create a thread: %d %s", w.Code, w.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	json.Unmarshal(w.Body.Bytes(), &created)
	if created.ID == "" {
		t.Fatal("no thread id")
	}

	// Kim sees an empty node, not Sam's.
	w = call(handler, "GET", "/app/api/threads", kim, "")
	if w.Code != http.StatusOK {
		t.Fatalf("kim: %d %s", w.Code, w.Body.String())
	}
	var list struct {
		Self    string `json:"self"`
		Threads []struct {
			Title string `json:"title"`
		} `json:"threads"`
	}
	json.Unmarshal(w.Body.Bytes(), &list)
	for _, th := range list.Threads {
		if th.Title == "Sam's deal" {
			t.Fatal("kim can see sam's thread")
		}
	}

	// And cannot open it by id.
	w = call(handler, "GET", "/app/api/thread/"+created.ID, kim, "")
	if w.Code == http.StatusOK {
		t.Fatalf("kim opened sam's thread: %s", w.Body.String())
	}
	w = call(handler, "GET", "/app/api/thread/"+created.ID, sam, "")
	if w.Code != http.StatusOK {
		t.Fatalf("sam cannot open his own thread: %d", w.Code)
	}

	// Two people, two principals, two directories.
	samSelf := list.Self
	w = call(handler, "GET", "/app/api/threads", sam, "")
	json.Unmarshal(w.Body.Bytes(), &list)
	if list.Self == samSelf {
		t.Fatal("both accounts share one identity")
	}
	if h.Count() != 2 {
		t.Fatalf("expected two accounts, got %d", h.Count())
	}
}

// An account is an ordinary node directory, which is what lets somebody
// take it and run it themselves.
func TestHostedAccountIsAPortableNode(t *testing.T) {
	h, key, claims, handler := testHost(t)
	if w := call(handler, "GET", "/app/api/me", as(t, key, claims, "sam-subject", "sam@example.com"), ""); w.Code != http.StatusOK {
		t.Fatalf("me: %d %s", w.Code, w.Body.String())
	}
	entries, _ := os.ReadDir(h.Root)
	if len(entries) != 1 {
		t.Fatalf("expected one account directory, got %d", len(entries))
	}
	dir := filepath.Join(h.Root, entries[0].Name())
	for _, f := range []string{"person.key", "lamdis.db", "agent.key", "email"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("a hosted account is missing %s: %v", f, err)
		}
	}
	if info, err := os.Stat(filepath.Join(dir, "person.key")); err == nil && info.Mode().Perm() != 0o600 {
		t.Fatalf("person key is %v, want 0600", info.Mode().Perm())
	}
	// A new account lands in an empty thread, ready to be typed into.
	w := call(handler, "GET", "/app/api/threads", as(t, key, claims, "sam-subject", "sam@example.com"), "")
	if !strings.Contains(w.Body.String(), `"title":"Notes"`) {
		t.Fatalf("no thread to start in: %s", w.Body.String())
	}
}

func TestHostRefusesWithoutAGoodToken(t *testing.T) {
	_, key, claims, handler := testHost(t)
	if w := call(handler, "GET", "/app/api/threads", "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", w.Code)
	}
	if w := call(handler, "GET", "/app/api/threads", "not-a-token", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("garbage token: %d", w.Code)
	}
	// An address nobody proved they own is not an identity.
	c := map[string]any{}
	for k, v := range claims {
		c[k] = v
	}
	c["sub"], c["email"], c["email_verified"] = "nope", "nope@example.com", false
	if w := call(handler, "GET", "/app/api/threads", signedToken(t, key, "k1", c, "RS256"), ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("unverified email was let in: %d", w.Code)
	}
	// An expired token is no token.
	c["email_verified"] = true
	c["exp"] = time.Now().Add(-time.Hour).Unix()
	if w := call(handler, "GET", "/app/api/threads", signedToken(t, key, "k1", c, "RS256"), ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("expired token was let in: %d", w.Code)
	}
}

// Hosting costs money, so the number of accounts is a number you choose.
func TestHostStopsAtItsAccountLimit(t *testing.T) {
	_, key, claims, handler := testHost(t) // MaxAccounts: 2
	for _, who := range []string{"one", "two"} {
		if w := call(handler, "GET", "/app/api/me", as(t, key, claims, who, who+"@example.com"), ""); w.Code != http.StatusOK {
			t.Fatalf("%s: %d", who, w.Code)
		}
	}
	w := call(handler, "GET", "/app/api/me", as(t, key, claims, "three", "three@example.com"), "")
	if w.Code == http.StatusOK {
		t.Fatal("the account limit was ignored")
	}
	if !strings.Contains(w.Body.String(), "support@lamdis.ai") {
		t.Fatalf("a full host should say what to do: %s", w.Body.String())
	}
}

// The page itself is public; only the data behind it needs a session.
func TestHostServesThePageToAnyone(t *testing.T) {
	_, _, _, handler := testHost(t)
	for _, p := range []string{"/app", "/app/app.js", "/healthz"} {
		if w := call(handler, "GET", p, "", ""); w.Code != http.StatusOK {
			t.Fatalf("%s: %d", p, w.Code)
		}
	}
	if w := call(handler, "GET", "/", "", ""); w.Code != http.StatusFound {
		t.Fatalf("/ should send people to the app: %d", w.Code)
	}
}

// The first visit asks nothing. Somebody arrives, gets a node of their own,
// and can write in it before anybody has asked who they are.
func TestAnyoneCanStartWithoutSigningUp(t *testing.T) {
	h, key, claims, handler := testHost(t)
	h.Guests = true

	w := call(handler, "POST", "/app/api/start", "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}
	var started struct {
		Token string `json:"token"`
		Guest bool   `json:"guest"`
	}
	json.Unmarshal(w.Body.Bytes(), &started)
	if started.Token == "" || !started.Guest {
		t.Fatalf("no visitor token: %s", w.Body.String())
	}

	// That token works straight away, on a node with something to type into.
	w = call(handler, "GET", "/app/api/threads", started.Token, "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"title":"Notes"`) {
		t.Fatalf("a visitor could not open their own node: %d %s", w.Code, w.Body.String())
	}
	guestThread := firstThread(t, handler, started.Token)
	w = call(handler, "POST", "/app/api/post", started.Token,
		`{"thread":"`+guestThread+`","text":"Roof quote came in at 4,800.","lane":"content"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("a visitor could not write: %d %s", w.Code, w.Body.String())
	}

	// A forged token is nothing.
	bad := started.Token[:len(started.Token)-3] + "aaa"
	if w := call(handler, "GET", "/app/api/threads", bad, ""); w.Code == http.StatusOK {
		t.Fatal("a tampered visitor token was accepted")
	}

	// Signing in keeps the node rather than starting another one.
	r := httptest.NewRequest("GET", "/app/api/threads", nil)
	r.Header.Set("Authorization", "Bearer "+as(t, key, claims, "sam-subject", "sam@example.com"))
	r.Header.Set("X-Lamdis-Guest", started.Token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), guestThread) {
		t.Fatalf("signing in lost the node the visitor was using: %d %s", rec.Code, rec.Body.String())
	}
	if h.Count() != 1 {
		t.Fatalf("signing in made a second node: %d accounts", h.Count())
	}
	// And on the next visit, without the visitor token, it is still theirs,
	// with what they wrote still in it.
	if got := firstThread(t, handler, as(t, key, claims, "sam-subject", "sam@example.com")); got != guestThread {
		t.Fatalf("the attached node was not found again: %s vs %s", got, guestThread)
	}
	w = call(handler, "GET", "/app/api/thread/"+guestThread, as(t, key, claims, "sam-subject", "sam@example.com"), "")
	if !strings.Contains(w.Body.String(), "Roof quote") {
		t.Fatalf("what the visitor wrote did not survive signing in: %s", w.Body.String())
	}
}

func firstThread(t *testing.T, h http.Handler, token string) string {
	t.Helper()
	w := call(h, "GET", "/app/api/threads", token, "")
	var out struct {
		Threads []struct {
			ID string `json:"id"`
		} `json:"threads"`
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	if len(out.Threads) == 0 {
		t.Fatal("no threads")
	}
	return out.Threads[0].ID
}

func TestGuestsCanBeTurnedOff(t *testing.T) {
	_, _, _, handler := testHost(t) // Guests defaults to false
	if w := call(handler, "POST", "/app/api/start", "", ""); w.Code != http.StatusForbidden {
		t.Fatalf("a host with guests off still handed out nodes: %d", w.Code)
	}
}
