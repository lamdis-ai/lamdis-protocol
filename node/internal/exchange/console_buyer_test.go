package exchange

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
)

// The console's buyer side, reached with the person's own session.
//
// A buyer could sign in, add funds and issue keys from the console, and then
// could not buy anything from it: POST /v1/tasks took an agent key or a signed
// principal, and every read route on a job took an agent key only. The person
// whose money it is was the one credential the buy side refused.

// verifiedPerson enrols a worker the way a device key does, then marks them
// verified the way an identity provider would. Cognito cannot be stood up in
// a test; a signed-in person and a verified enrolled principal reach the
// exchange through the same Workers.Authenticate, which is what is exercised.
func verifiedPerson(t *testing.T, s *Server) (ed25519.PrivateKey, string) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := protolog.PrincipalID(priv.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.Workers.Enroll(pid)
	if err != nil {
		t.Fatal(err)
	}
	w.Verified = true
	return priv, pid
}

func signedReq(t *testing.T, priv ed25519.PrivateKey, method, path string, body any) *http.Request {
	t.Helper()
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(raw))
	if err := api.Sign(r, priv, raw); err != nil {
		t.Fatal(err)
	}
	return r
}

func consoleServer(t *testing.T) *Server {
	t.Helper()
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	s, err := Open(key, "https://example.test", Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAPersonCanPostAJobFromTheConsole(t *testing.T) {
	s := consoleServer(t)
	h := s.Handler()
	ctx := context.Background()

	funded, buyer := verifiedPerson(t, s)
	if _, err := s.Ledger.Topup(ctx, "t1", buyer, 100000, "USD", ""); err != nil {
		t.Fatal(err)
	}
	job := map[string]any{
		"kind": "observe", "predicate": "Is the sign still up on the corner",
		"deliverable": "A photo of the frontage", "fee_minor": 500,
	}

	w := httptest.NewRecorder()
	h.ServeHTTP(w, signedReq(t, funded, "POST", "/v1/tasks", job))
	if w.Code != http.StatusOK {
		t.Fatalf("a funded, verified person could not post: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		Job string `json:"job"`
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	l, ok := s.Board.Get(out.Job)
	if !ok {
		t.Fatalf("the job %q is not on the board", out.Job)
	}
	if l.Owner != buyer {
		t.Errorf("the listing is owned by %q, not the person who posted it", l.Owner)
	}
	if l.PostedByAgent {
		t.Error("a job a person posted by hand is marked as posted by an agent")
	}

	// Unfunded is refused exactly as it is for an agent: the listing never
	// exists, because a worker who completes unfunded work has been defrauded
	// by the exchange.
	broke, _ := verifiedPerson(t, s)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, signedReq(t, broke, "POST", "/v1/tasks", job))
	if w.Code != http.StatusPaymentRequired {
		t.Errorf("an unfunded person posting got %d, wanted 402: %s", w.Code, w.Body.String())
	}
	if n := len(s.Board.All()); n != 1 {
		t.Errorf("an unfunded post still produced a listing (%d on the board)", n)
	}

	// A browser session that does not check out is told to sign in, not
	// handed the signing-key error meant for integrations.
	raw, _ := json.Marshal(job)
	r := httptest.NewRequest("POST", "/v1/tasks", bytes.NewReader(raw))
	r.Header.Set("Authorization", "Bearer not-a-session")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), "/signin") {
		t.Errorf("a dead session got %d %s; the console expects a 401 naming /signin",
			w.Code, w.Body.String())
	}
}

func TestTheBuyerReadsTheirOwnReceiptWithASession(t *testing.T) {
	s := consoleServer(t)
	h := s.Handler()
	ctx := context.Background()

	owner, buyer := verifiedPerson(t, s)
	if _, err := s.Ledger.Topup(ctx, "t1", buyer, 100000, "USD", ""); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, signedReq(t, owner, "POST", "/v1/tasks", map[string]any{
		"kind": "do", "predicate": "The gutters are clear",
		"instructions": "Clear both runs.", "deliverable": "A photo along each run.",
		"fee_minor": 6000,
	}))
	if w.Code != http.StatusOK {
		t.Fatalf("post: %d %s", w.Code, w.Body.String())
	}
	var posted struct {
		Job string `json:"job"`
	}
	json.Unmarshal(w.Body.Bytes(), &posted)

	// Something came back.
	s.mu.Lock()
	s.submissions[posted.Job] = append(s.submissions[posted.Job], api.Submission{
		Job: posted.Job, Verified: true, At: time.Now(), AttestedBy: "device_key",
		Artifacts: []api.Artifact{{SHA256: "abc123", Kind: "photo", Mime: "image/jpeg"}},
	})
	s.mu.Unlock()

	for _, path := range []string{
		"/v1/jobs/" + posted.Job + "/receipt",
		"/v1/jobs/" + posted.Job + "/evidence",
		"/v1/jobs/" + posted.Job,
	} {
		// The owner, with their session.
		w = httptest.NewRecorder()
		h.ServeHTTP(w, signedReq(t, owner, "GET", path, nil))
		if w.Code != http.StatusOK {
			t.Errorf("the buyer could not read %s with their session: %d %s",
				path, w.Code, w.Body.String())
		}
		// Somebody else, also signed in: not theirs, and not told it exists.
		other, _ := verifiedPerson(t, s)
		w = httptest.NewRecorder()
		h.ServeHTTP(w, signedReq(t, other, "GET", path, nil))
		if w.Code == http.StatusOK {
			t.Errorf("another signed-in account read %s", path)
		}
		if w.Code != http.StatusNotFound && w.Code != http.StatusForbidden {
			t.Errorf("a stranger reading %s got %d, wanted a refusal", path, w.Code)
		}
		// Nobody at all.
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("an anonymous read of %s got %d, wanted 401", path, w.Code)
		}
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, signedReq(t, owner, "GET", "/v1/jobs/"+posted.Job+"/receipt", nil))
	var receipt map[string]any
	json.Unmarshal(w.Body.Bytes(), &receipt)
	v, _ := receipt["verification"].(map[string]any)
	if v == nil || v["confidence_ceiling"] == nil {
		t.Error("the receipt the console renders carries no confidence ceiling")
	}
}

// The registrations, read from the source, so a revert to withAgent on any of
// these is caught even if the handler tests above are one day rewritten.
func TestConsoleReadRoutesAcceptTheSession(t *testing.T) {
	src, err := readFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{
		`"GET /v1/jobs/{job}"`, `"GET /v1/jobs/{job}/receipt"`,
		`"GET /v1/jobs/{job}/evidence"`, `"GET /v1/jobs/{job}/evidence/{sha}"`,
	} {
		i := strings.Index(src, route)
		if i < 0 {
			t.Fatalf("%s is no longer registered", route)
		}
		line := src[i:]
		if j := strings.IndexByte(line, '\n'); j >= 0 {
			line = line[:j]
		}
		if !strings.Contains(line, "withBuyer") {
			t.Errorf("%s is registered %q; the console sends a session token, "+
				"which only withBuyer accepts", route, line)
		}
	}
	// The balance route reads the key and stays agent-only.
	i := strings.Index(src, `"GET /v1/agent/balance"`)
	if i < 0 || !strings.Contains(src[i:i+120], "withAgent") {
		t.Error("/v1/agent/balance dereferences the key and must stay withAgent")
	}
	i = strings.Index(src, `"POST /v1/tasks"`)
	if i < 0 || !strings.Contains(src[i:i+80], "postJobAsAnyone") {
		t.Error("POST /v1/tasks no longer takes a signed-in person")
	}
}
