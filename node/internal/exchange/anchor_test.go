package exchange

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/anchor"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
)

// A stand-in for an OpenTimestamps pool: accepts digests, and once mined,
// answers with a bitcoin attestation. Speaks just enough of the wire format.
func fakePool(t *testing.T) (*httptest.Server, *bool) {
	t.Helper()
	mined := false
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/digest":
			digest, _ := io.ReadAll(r.Body)
			ts := &anchor.Timestamp{Msg: digest}
			commit := sha256.Sum256(digest)
			ts.Ops = []anchor.Branch{{Op: anchor.Op{Tag: 0x08}, Next: &anchor.Timestamp{
				Msg:          commit[:],
				Attestations: []anchor.Attestation{pendingFor(srv.URL)},
			}}}
			raw, _ := ts.Serialize()
			w.Write(raw)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/timestamp/"):
			if !mined {
				http.Error(w, "pending", http.StatusNotFound)
				return
			}
			commit, _ := hex.DecodeString(strings.TrimPrefix(r.URL.Path, "/timestamp/"))
			ts := &anchor.Timestamp{Msg: commit, Attestations: []anchor.Attestation{bitcoinAt(123456)}}
			raw, _ := ts.Serialize()
			w.Write(raw)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &mined
}

// The attestation constructors are unexported; build the bytes by hand.
func pendingFor(uri string) anchor.Attestation {
	payload := append([]byte{byte(len(uri))}, uri...)
	return anchor.Attestation{Tag: [8]byte{0x83, 0xdf, 0xe3, 0x0d, 0x2e, 0xf9, 0x0c, 0x8e}, Payload: payload}
}

func bitcoinAt(height int) anchor.Attestation {
	var payload []byte
	for height >= 0x80 {
		payload = append(payload, byte(height)|0x80)
		height >>= 7
	}
	payload = append(payload, byte(height))
	return anchor.Attestation{Tag: [8]byte{0x05, 0x88, 0x96, 0x0d, 0x73, 0xd7, 0x19, 0x01}, Payload: payload}
}

// A receipt fetched twice is the same document, its hash is recorded once,
// and after the batch reaches Bitcoin the receipt says where it is anchored.
func TestReceiptCarriesItsAnchorOnceAnchored(t *testing.T) {
	s := consoleServer(t)
	pool, mined := fakePool(t)
	a, err := anchor.Open(filepath.Join(t.TempDir(), "anchor"), anchor.Options{
		Calendars: []string{pool.URL}, Logf: t.Logf,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	s.Anchors = a
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
	s.mu.Lock()
	s.submissions[posted.Job] = append(s.submissions[posted.Job], api.Submission{
		Job: posted.Job, Verified: true, At: time.Now(), AttestedBy: "device_key",
		Artifacts: []api.Artifact{{SHA256: "abc123", Kind: "photo", Mime: "image/jpeg"}},
	})
	s.mu.Unlock()

	receipt := func() map[string]any {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, signedReq(t, owner, "GET", "/v1/jobs/"+posted.Job+"/receipt", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("receipt: %d %s", w.Code, w.Body.String())
		}
		var out map[string]any
		json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}

	// The clock moves between fetches; the receipt must not.
	base := time.Now()
	s.Now = func() time.Time { return base }
	first := receipt()
	s.Now = func() time.Time { return base.Add(3 * time.Minute) }
	second := receipt()
	an1, _ := first["anchor"].(map[string]any)
	an2, _ := second["anchor"].(map[string]any)
	if an1 == nil || an2 == nil {
		t.Fatalf("the receipt carries no anchor block: %v", first)
	}
	if an1["receipt_sha256"] != an2["receipt_sha256"] || first["issued_at"] != second["issued_at"] {
		t.Fatalf("an unchanged receipt was re-issued as a new document:\n%v\n%v", an1, an2)
	}
	if an1["status"] != "pending" || an1["merkle_root"] != nil {
		t.Fatalf("a fresh receipt claims more than pending: %v", an1)
	}
	sha, _ := an1["receipt_sha256"].(string)

	// The hash is over the receipt minus signature and anchor, as documented.
	doc := map[string]any{}
	for k, v := range second {
		if k != "signature" && k != "anchor" {
			doc[k] = v
		}
	}
	canon, _ := json.Marshal(doc)
	sum := sha256.Sum256(canon)
	if hex.EncodeToString(sum[:]) != sha {
		t.Fatal("receipt_sha256 is not the hash of the receipt minus signature and anchor")
	}

	// The proof endpoint: same credential, honest about being pending.
	w = httptest.NewRecorder()
	h.ServeHTTP(w, signedReq(t, owner, "GET", "/v1/jobs/"+posted.Job+"/receipt/anchor", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("anchor: %d %s", w.Code, w.Body.String())
	}
	var proof map[string]any
	json.Unmarshal(w.Body.Bytes(), &proof)
	if proof["status"] != "pending" || proof["receipt_sha256"] != sha || proof["merkle_root"] != nil {
		t.Fatalf("proof before batching: %v", proof)
	}
	// A stranger gets nothing, and nobody at all gets 401.
	other, _ := verifiedPerson(t, s)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, signedReq(t, other, "GET", "/v1/jobs/"+posted.Job+"/receipt/anchor", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("another account read the proof: %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v1/jobs/"+posted.Job+"/receipt/anchor", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous read of the proof got %d", w.Code)
	}

	// Batch, mine, upgrade.
	if b, err := a.Batch(ctx); err != nil || b == nil || b.SubmittedAt == nil {
		t.Fatalf("batch: %v %+v", err, b)
	}
	*mined = true
	if n := a.Upgrade(ctx); n != 1 {
		t.Fatalf("upgraded %d, want 1", n)
	}

	third := receipt()
	an3, _ := third["anchor"].(map[string]any)
	if an3["receipt_sha256"] != sha || an3["status"] != "anchored" || an3["merkle_root"] == nil ||
		an3["bitcoin_block"] != float64(123456) {
		t.Fatalf("the receipt does not carry its anchor after anchoring: %v", an3)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, signedReq(t, owner, "GET", "/v1/jobs/"+posted.Job+"/receipt/anchor?sha256="+sha, nil))
	var full struct {
		SHA   string        `json:"receipt_sha256"`
		Root  string        `json:"merkle_root"`
		Path  []anchor.Step `json:"inclusion_path"`
		OTS   string        `json:"ots_proof"`
		Cals  []string      `json:"calendars"`
		How   []string      `json:"how_to_verify"`
		State string        `json:"status"`
	}
	json.Unmarshal(w.Body.Bytes(), &full)
	if full.State != "anchored" || !anchor.VerifyPath(full.SHA, full.Path, full.Root) {
		t.Fatalf("the served inclusion path does not reach the root: %+v", full)
	}
	ots, err := base64.StdEncoding.DecodeString(full.OTS)
	if err != nil {
		t.Fatal(err)
	}
	digest, ts, err := anchor.ParseFile(ots)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(digest) != full.Root {
		t.Fatal("the .ots proof is not over the merkle root")
	}
	if height, ok := ts.Complete(); !ok || height != 123456 {
		t.Fatalf("the served proof is not complete: %d %v", height, ok)
	}
	if len(full.Cals) != 1 || full.Cals[0] != pool.URL {
		t.Errorf("calendars: %v", full.Cals)
	}
	if strings.Join(full.How, " ") == "" || !strings.Contains(strings.Join(full.How, " "), "ots verify") {
		t.Error("how_to_verify does not name the ots tool")
	}

	// The public list of roots: no token needed.
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v1/anchors", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("/v1/anchors: %d", w.Code)
	}
	var list struct {
		Enabled bool `json:"enabled"`
		Batches []struct {
			Root   string `json:"merkle_root"`
			Status string `json:"status"`
		} `json:"batches"`
	}
	json.Unmarshal(w.Body.Bytes(), &list)
	if !list.Enabled || len(list.Batches) != 1 || list.Batches[0].Root != full.Root ||
		list.Batches[0].Status != "anchored" {
		t.Fatalf("public roots: %s", w.Body.String())
	}
}

// With anchoring off, the receipt still works and the routes say so.
func TestReceiptWithoutAnchoringSaysNothingAboutAnchors(t *testing.T) {
	s := consoleServer(t)
	h := s.Handler()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v1/anchors", nil))
	var out map[string]any
	json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != http.StatusOK || out["enabled"] != false {
		t.Fatalf("/v1/anchors with anchoring off: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v1/jobs/j1/receipt/anchor", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("the proof route is not mounted behind the buyer credential: %d", w.Code)
	}
}
