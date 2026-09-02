package anchor

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func leaf(i int) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("receipt-%d", i)))
	return hex.EncodeToString(h[:])
}

// Every leaf of every tree size must fold back to the root along its path,
// and a wrong leaf, sibling or side must not.
func TestMerkleRootAndInclusionPathRoundTrip(t *testing.T) {
	for n := 1; n <= 9; n++ {
		leaves := make([]string, n)
		for i := range leaves {
			leaves[i] = leaf(i)
		}
		root, err := merkleRoot(leaves)
		if err != nil {
			t.Fatal(err)
		}
		if n == 1 && root != leaves[0] {
			t.Fatalf("a one-leaf tree's root should be the leaf itself")
		}
		for i := range leaves {
			path, err := merklePath(leaves, i)
			if err != nil {
				t.Fatal(err)
			}
			if !VerifyPath(leaves[i], path, root) {
				t.Errorf("n=%d leaf %d: path does not reach the root", n, i)
			}
			if VerifyPath(leaf(100+i), path, root) {
				t.Errorf("n=%d leaf %d: a different leaf verified", n, i)
			}
			if len(path) > 0 {
				bad := append([]Step{}, path...)
				if bad[0].Side == "left" {
					bad[0].Side = "right"
				} else {
					bad[0].Side = "left"
				}
				if VerifyPath(leaves[i], bad, root) {
					t.Errorf("n=%d leaf %d: a swapped side verified", n, i)
				}
			}
		}
	}
}

// The proof format has to survive being written and read back unchanged, or
// what we hand out is not what the calendar gave us.
func TestOTSFileRoundTrip(t *testing.T) {
	digest := sha256.Sum256([]byte("root"))
	nonce := []byte{1, 2, 3, 4}
	inner := &Timestamp{}
	inner.Msg, _ = Op{Tag: opAppend, Arg: nonce}.Apply(digest[:])
	committed, _ := Op{Tag: opSHA256}.Apply(inner.Msg)
	inner.Ops = []Branch{{Op: Op{Tag: opSHA256}, Next: &Timestamp{
		Msg:          committed,
		Attestations: []Attestation{pendingAttestation("https://cal.example"), bitcoinAttestation(812345)},
	}}}
	ts := &Timestamp{Msg: digest[:], Ops: []Branch{{Op: Op{Tag: opAppend, Arg: nonce}, Next: inner}}}

	file, err := SerializeFile(digest[:], ts)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(file, otsMagic) {
		t.Fatal("file does not start with the OpenTimestamps magic")
	}
	gotDigest, back, err := ParseFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotDigest, digest[:]) {
		t.Fatal("digest did not survive")
	}
	again, err := SerializeFile(digest[:], back)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(file, again) {
		t.Fatal("serialize → parse → serialize is not stable")
	}
	if h, ok := back.Complete(); !ok || h != 812345 {
		t.Fatalf("bitcoin attestation lost: %d %v", h, ok)
	}
	if n := len(back.pendings()); n != 1 {
		t.Fatalf("expected one pending node, got %d", n)
	}
	if uri, ok := back.pendings()[0].Attestations[0].Pending(); !ok || uri != "https://cal.example" {
		// Sorted on write: pending's tag 0x83 sorts after bitcoin's 0x05.
		if uri, ok := back.pendings()[0].Attestations[1].Pending(); !ok || uri != "https://cal.example" {
			t.Fatal("pending attestation lost its URI")
		}
	}
	// Truncated input must fail rather than yield a half proof.
	if _, _, err := ParseFile(file[:len(file)-3]); err == nil {
		t.Fatal("a truncated proof parsed")
	}
}

// fakeCalendar behaves like a pool: it takes digests, answers with a pending
// timestamp naming itself, and once "mined" answers /timestamp with a
// bitcoin attestation.
type fakeCalendar struct {
	srv *httptest.Server
	mu  sync.Mutex
	// commitments the calendar has seen, and whether they are in a block.
	seen  map[string]bool
	mined bool
	down  bool
	posts int
}

func newFakeCalendar(t *testing.T) *fakeCalendar {
	f := &fakeCalendar{seen: map[string]bool{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeCalendar) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		http.Error(w, "gone fishing", http.StatusBadGateway)
		return
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/digest":
		f.posts++
		digest, _ := io.ReadAll(r.Body)
		if len(digest) != sha256.Size {
			http.Error(w, "bad digest", http.StatusBadRequest)
			return
		}
		nonce := make([]byte, 16)
		rand.Read(nonce)
		withNonce, _ := Op{Tag: opAppend, Arg: nonce}.Apply(digest)
		commitment, _ := Op{Tag: opSHA256}.Apply(withNonce)
		f.seen[hex.EncodeToString(commitment)] = true
		ts := &Timestamp{Msg: digest, Ops: []Branch{{
			Op: Op{Tag: opAppend, Arg: nonce},
			Next: &Timestamp{Msg: withNonce, Ops: []Branch{{
				Op:   Op{Tag: opSHA256},
				Next: &Timestamp{Msg: commitment, Attestations: []Attestation{pendingAttestation(f.srv.URL)}},
			}}},
		}}}
		raw, _ := ts.Serialize()
		w.Write(raw)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/timestamp/"):
		hexC := strings.TrimPrefix(r.URL.Path, "/timestamp/")
		if !f.seen[hexC] || !f.mined {
			http.Error(w, "Pending confirmation in Bitcoin blockchain", http.StatusNotFound)
			return
		}
		commitment, _ := hex.DecodeString(hexC)
		// A real calendar's answer is a path through its Merkle tree to the
		// transaction; two ops stand in for that here.
		mid, _ := Op{Tag: opPrepend, Arg: []byte("block")}.Apply(commitment)
		top, _ := Op{Tag: opSHA256}.Apply(mid)
		ts := &Timestamp{Msg: commitment, Ops: []Branch{{
			Op: Op{Tag: opPrepend, Arg: []byte("block")},
			Next: &Timestamp{Msg: mid, Ops: []Branch{{
				Op:   Op{Tag: opSHA256},
				Next: &Timestamp{Msg: top, Attestations: []Attestation{bitcoinAttestation(900001)}},
			}}},
		}}}
		raw, _ := ts.Serialize()
		w.Write(raw)
	default:
		http.NotFound(w, r)
	}
}

func openTest(t *testing.T, dir string, cal *fakeCalendar) *Anchorer {
	t.Helper()
	a, err := Open(dir, Options{
		Calendars: []string{cal.srv.URL},
		Every:     time.Hour,
		Logf:      t.Logf,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}

// Submit, wait, upgrade: the ordinary life of a batch.
func TestSubmitThenUpgradeAgainstAFakeCalendar(t *testing.T) {
	cal := newFakeCalendar(t)
	a := openTest(t, t.TempDir(), cal)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		a.Record("job-"+fmt.Sprint(i), "fp", "2026-09-02T00:00:00Z", leaf(i))
	}
	a.Record("job-0", "fp", "2026-09-02T00:00:00Z", leaf(0)) // duplicate: ignored
	if a.Pending() != 3 {
		t.Fatalf("pending = %d, want 3", a.Pending())
	}

	b, err := a.Batch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if b == nil || len(b.Leaves) != 3 || b.SubmittedAt == nil || len(b.OTS) == 0 {
		t.Fatalf("batch not submitted: %+v", b)
	}
	if again, _ := a.Batch(ctx); again != nil {
		t.Fatal("a second batch was built with nothing new to anchor")
	}

	p, ok := a.Proof(leaf(1))
	if !ok || p.Status != StatusPending || p.MerkleRoot != b.Root {
		t.Fatalf("proof before mining: %+v", p)
	}
	if !VerifyPath(p.ReceiptSHA256, p.InclusionPath, p.MerkleRoot) {
		t.Fatal("inclusion path does not reach the root")
	}
	if _, ts, err := ParseFile(p.OTSProof); err != nil {
		t.Fatal(err)
	} else if _, done := ts.Complete(); done {
		t.Fatal("proof claims completion before the calendar mined anything")
	}

	// Nothing in a block yet: an upgrade changes nothing.
	if n := a.Upgrade(ctx); n != 0 {
		t.Fatalf("upgraded %d batches before mining", n)
	}

	cal.mu.Lock()
	cal.mined = true
	cal.mu.Unlock()
	if n := a.Upgrade(ctx); n != 1 {
		t.Fatalf("upgraded %d batches after mining, want 1", n)
	}
	p, _ = a.Proof(leaf(1))
	if p.Status != StatusAnchored || p.BitcoinHeight != 900001 || p.AnchoredAt == nil {
		t.Fatalf("proof after mining: %+v", p)
	}
	_, ts, err := ParseFile(p.OTSProof)
	if err != nil {
		t.Fatal(err)
	}
	if h, done := ts.Complete(); !done || h != 900001 {
		t.Fatalf("upgraded proof is not complete: %d %v", h, done)
	}
	if len(ts.pendings()) != 0 {
		t.Fatal("the pending attestation was not dropped after upgrading")
	}
	if got := a.Batches(10); len(got) != 1 || got[0].Status() != StatusAnchored || got[0].Leaves != nil {
		t.Fatalf("listing: %+v", got)
	}
}

// A batch built while the calendars are down, and a batch submitted before a
// restart, both have to come back and finish.
func TestPendingSurvivesRestartAndUnreachableCalendars(t *testing.T) {
	cal := newFakeCalendar(t)
	dir := t.TempDir()
	ctx := context.Background()

	a := openTest(t, dir, cal)
	a.Record("j1", "fp", "2026-09-02T00:00:00Z", leaf(1))
	cal.mu.Lock()
	cal.down = true
	cal.mu.Unlock()
	b, err := a.Batch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if b.SubmittedAt != nil || b.LastError == "" {
		t.Fatalf("a batch was marked submitted with the calendar down: %+v", b)
	}
	p, _ := a.Proof(leaf(1))
	if p.Status != StatusPending || p.MerkleRoot != b.Root || !strings.Contains(p.Note, "retried") {
		t.Fatalf("proof with calendar down: %+v", p)
	}
	a.Close()

	// Restart. The batch and the log must both be there.
	a2 := openTest(t, dir, cal)
	if a2.Pending() != 0 {
		t.Fatal("the receipt was forgotten to be batched after restart")
	}
	if got := a2.Batches(10); len(got) != 1 || got[0].Root != b.Root || got[0].SubmittedAt != nil {
		t.Fatalf("batches after restart: %+v", got)
	}
	if at, ok := a2.IssuedAt("j1", "fp"); !ok || at != "2026-09-02T00:00:00Z" {
		t.Fatal("the pinned issue time did not survive the restart")
	}

	// Calendar back: the retry submits, and the proof appears.
	cal.mu.Lock()
	cal.down = false
	cal.mu.Unlock()
	a2.Upgrade(ctx)
	p, _ = a2.Proof(leaf(1))
	if p.SubmittedAt == nil || len(p.OTSProof) == 0 || p.Status != StatusPending {
		t.Fatalf("after retry: %+v", p)
	}
	a2.Close()

	// Restart again, mine, upgrade.
	a3 := openTest(t, dir, cal)
	cal.mu.Lock()
	cal.mined = true
	cal.mu.Unlock()
	if n := a3.Upgrade(ctx); n != 1 {
		t.Fatalf("upgraded %d after restart, want 1", n)
	}
	p, _ = a3.Proof(leaf(1))
	if p.Status != StatusAnchored || !VerifyPath(p.ReceiptSHA256, p.InclusionPath, p.MerkleRoot) {
		t.Fatalf("final proof: %+v", p)
	}
	// And a fourth open sees it anchored without asking anybody.
	a3.Close()
	cal.mu.Lock()
	cal.down = true
	cal.mu.Unlock()
	a4 := openTest(t, dir, cal)
	if p, _ := a4.Proof(leaf(1)); p.Status != StatusAnchored {
		t.Fatal("anchored status was not persisted")
	}
}

func TestOpenNeedsADirectory(t *testing.T) {
	if _, err := Open("", Options{}); err == nil {
		t.Fatal("opened with no directory")
	}
}
