package exchange

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
)

// The failure these cover: a restart lost every open listing, every seat, every
// capability secret and every photograph, while the escrow behind them stayed
// in the ledger. The money was orphaned and the worker holding a link had
// nowhere to submit.

func exchangeAt(t *testing.T, dir string) *Server {
	t.Helper()
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	s, err := Open(key, "https://example.test", Options{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// capture collects what the exchange said while fn ran, so a test can assert
// on a reconciliation line the way an operator would read it.
func capture(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	fn()
	return buf.String()
}

func TestASecondServerFindsTheWorkTheFirstOneHeld(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	s := exchangeAt(t, dir)
	if _, err := s.Ledger.Topup(ctx, "t", "buyer", 100000, "USD", ""); err != nil {
		t.Fatal(err)
	}

	l := &api.Listing{
		Job: "obs_1", Kind: api.KindObserve, Title: "is the sign up",
		Where: "12 Example Street", Owner: "buyer",
		PayMinor: 500, BonusMinor: 1800, Currency: "USD", Slots: 1,
		Expires: time.Now().Add(2 * time.Hour),
	}
	if _, err := s.Ledger.Hold(ctx, "h:obs_1", "obs_1", "buyer",
		MaxPayoutFor(l), l.Currency); err != nil {
		t.Fatal(err)
	}
	if err := s.Board.Post(l); err != nil {
		t.Fatal(err)
	}
	s.setBuyer(l.Job, "buyer")

	secret, _, err := s.Board.Claim("obs_1", "worker")
	if err != nil {
		t.Fatalf("claiming: %v", err)
	}

	img := []byte("not really a jpeg, but the bytes are the bytes")
	sum := sha256.Sum256(img)
	sha := hex.EncodeToString(sum[:])
	if err := s.storeArtifact("obs_1", api.Artifact{
		SHA256: sha, Mime: "image/jpeg", Kind: "image", Bytes: len(img),
	}, img); err != nil {
		t.Fatal(err)
	}
	// Verification is not what is under test, and a nil verifier is the
	// documented "stored but not payable" state.
	s.Verify = nil
	if _, err := s.acceptEvidence(api.Submission{
		Job: "obs_1", Holder: holderOfSecret(secret), At: time.Now(),
		Artifacts: []api.Artifact{{SHA256: sha, Mime: "image/jpeg"}},
	}); err != nil {
		t.Fatal(err)
	}

	// A whole new process over the same disk.
	s2 := exchangeAt(t, dir)

	got, ok := s2.Board.Get("obs_1")
	if !ok {
		t.Fatal("the listing did not survive the restart")
	}
	if got.Where != "12 Example Street" {
		t.Errorf("the address came back as %q", got.Where)
	}
	if got.Owner != "buyer" {
		// Owner is json:"-" on Listing and has to be carried deliberately.
		// Without it the job cannot be settled or refunded.
		t.Errorf("the listing came back owned by %q", got.Owner)
	}
	if got.Taken != 1 {
		t.Errorf("the seat was not restored: taken = %d", got.Taken)
	}
	if s2.Board.HasOpenSeat("obs_1") {
		t.Error("the job is offering a seat somebody is already holding")
	}
	if held := s2.Board.HeldBy("worker"); len(held) != 1 || held[0].Job != "obs_1" {
		t.Errorf("the worker's holding did not survive: %+v", held)
	}

	// The link in the worker's URL fragment still works: the secret is a
	// candidate for the job, and the capability behind it is still known.
	found := false
	for _, cand := range s2.secretsFor("obs_1") {
		if cand == secret {
			found = true
		}
	}
	if !found {
		t.Error("the capability secret did not survive the restart")
	}
	if _, ok := s2.Caps.Lookup(secret); !ok {
		t.Error("the capability itself did not survive, so the link is dead anyway")
	}

	subs := s2.Submissions("obs_1")
	if len(subs) != 1 {
		t.Fatalf("submissions came back as %d", len(subs))
	}
	if subs[0].Artifacts[0].SHA256 != sha {
		t.Error("the submission lost the artifact it points at")
	}
	back, ok := s2.blobFor(sha)
	if !ok || !bytes.Equal(back, img) {
		t.Error("the evidence bytes did not survive the restart")
	}
	if mime := s2.mimeFor(sha); mime != "image/jpeg" {
		t.Errorf("the artifact came back as %q", mime)
	}
	if s2.buyers["obs_1"] != "buyer" {
		t.Errorf("the funder came back as %q, so a refund has nowhere to go",
			s2.buyers["obs_1"])
	}

	// Evidence is on the disk, not in a JSON blob and not in the process.
	if _, err := os.Stat(filepath.Join(dir, "blobs", sha)); err != nil {
		t.Errorf("the evidence is not in the blob store: %v", err)
	}
}

// holderOfSecret mirrors what the board records against a capability.
func holderOfSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func TestReconciliationReportsAndKeeps(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	s := exchangeAt(t, dir)
	if _, err := s.Ledger.Topup(ctx, "t", "buyer", 100000, "USD", ""); err != nil {
		t.Fatal(err)
	}

	l := &api.Listing{
		Job: "obs_short", Kind: api.KindObserve, Title: "is the sign up",
		PayMinor: 500, Currency: "USD", Slots: 1,
		Expires: time.Now().Add(2 * time.Hour),
	}
	need := MaxPayoutFor(l)
	if _, err := s.Ledger.Hold(ctx, "h:obs_short", "obs_short", "buyer", need, "USD"); err != nil {
		t.Fatal(err)
	}
	if err := s.Board.Post(l); err != nil {
		t.Fatal(err)
	}
	// The escrow goes back to the buyer behind the listing's back, which is
	// the shape of a job whose money is no longer there.
	if _, err := s.Ledger.Release(ctx, "r:obs_short", "obs_short", "buyer", need, "USD"); err != nil {
		t.Fatal(err)
	}
	// And money committed to a job that is on no board at all.
	if _, err := s.Ledger.Hold(ctx, "h:ghost", "ghost", "buyer", 4200, "USD"); err != nil {
		t.Fatal(err)
	}

	var s2 *Server
	out := capture(t, func() { s2 = exchangeAt(t, dir) })

	if !strings.Contains(out, "escrow holds 0") {
		t.Errorf("an unfunded listing was not reported:\n%s", out)
	}
	if !strings.Contains(out, "ghost") {
		t.Errorf("orphaned escrow was not reported:\n%s", out)
	}
	// The whole point: nothing was deleted to tidy the discrepancy away.
	if _, ok := s2.Board.Get("obs_short"); !ok {
		t.Error("reconciliation deleted a listing whose escrow looked short")
	}
	if held, _ := s2.Ledger.Held(ctx, "ghost", "USD"); held != 4200 {
		t.Errorf("reconciliation moved orphaned money: %d", held)
	}
}

func TestExpiredWorkIsReportedNotDropped(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	s := exchangeAt(t, dir)
	if _, err := s.Ledger.Topup(ctx, "t", "buyer", 100000, "USD", ""); err != nil {
		t.Fatal(err)
	}
	l := &api.Listing{
		Job: "obs_old", Kind: api.KindObserve, Title: "is the sign up",
		PayMinor: 500, Currency: "USD", Slots: 1,
		Expires: time.Now().Add(30 * time.Millisecond),
	}
	if _, err := s.Ledger.Hold(ctx, "h:obs_old", "obs_old", "buyer",
		MaxPayoutFor(l), "USD"); err != nil {
		t.Fatal(err)
	}
	if err := s.Board.Post(l); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)

	var s2 *Server
	out := capture(t, func() { s2 = exchangeAt(t, dir) })
	if !strings.Contains(out, "obs_old expired") {
		t.Errorf("an expired listing was not reported:\n%s", out)
	}
	if _, ok := s2.Board.Get("obs_old"); !ok {
		t.Error("an expired listing was dropped, and its escrow with it")
	}
}

func TestNoDataDirWritesNothing(t *testing.T) {
	dir := t.TempDir()
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	s, err := Open(key, "https://example.test", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if s.store != nil {
		t.Fatal("an exchange with no -data built a store")
	}
	ctx := context.Background()
	l := &api.Listing{
		Job: "obs_mem", Kind: api.KindObserve, Title: "is the sign up",
		PayMinor: 500, Currency: "USD", Slots: 1,
		Expires: time.Now().Add(time.Hour),
	}
	if _, err := s.Ledger.Topup(ctx, "t", "buyer", 100000, "USD", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Ledger.Hold(ctx, "h:obs_mem", "obs_mem", "buyer",
		MaxPayoutFor(l), "USD"); err != nil {
		t.Fatal(err)
	}
	if err := s.Board.Post(l); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Board.Claim("obs_mem", "worker"); err != nil {
		t.Fatal(err)
	}
	s.putBlob("aa", "image/jpeg", []byte("bytes"))
	if _, ok := s.blobFor("aa"); !ok {
		t.Error("an in-memory exchange lost the bytes it was just handed")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("an exchange with no -data wrote %d file(s)", len(entries))
	}
}

func TestATruncatedFileStillBoots(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"board.json":        `{"listings":{"obs_1":{"job":"obs_1","kin`,
		"submissions.json":  `{"obs_1":[{"job":`,
		"job-secrets.json":  `{"obs_1":["ABC`,
		"job-buyers.json":   `{"obs_1":`,
		"capabilities.json": `{"by_id":{`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var s *Server
	out := capture(t, func() { s = exchangeAt(t, dir) })
	if s == nil {
		t.Fatal("a half-written file stopped the exchange booting")
	}
	if !strings.Contains(out, "unreadable") {
		t.Errorf("the exchange did not say what it could not read:\n%s", out)
	}
	if got := s.Board.Listings(); len(got) != 0 {
		t.Errorf("a truncated board came back with %d listing(s)", len(got))
	}
	// And it is still usable: a broken file must not leave the board read-only.
	ctx := context.Background()
	fresh := &api.Listing{
		Job: "obs_new", Kind: api.KindObserve, Title: "is the sign up",
		PayMinor: 500, Currency: "USD", Slots: 1,
		Expires: time.Now().Add(time.Hour),
	}
	if _, err := s.Ledger.Topup(ctx, "t", "buyer", 100000, "USD", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Ledger.Hold(ctx, "h:obs_new", "obs_new", "buyer",
		MaxPayoutFor(fresh), "USD"); err != nil {
		t.Fatal(err)
	}
	if err := s.Board.Post(fresh); err != nil {
		t.Fatal(err)
	}
	if _, ok := exchangeAt(t, dir).Board.Get("obs_new"); !ok {
		t.Error("work posted after a bad load did not persist")
	}
}

// An operator's capacity is what makes work reach them. Losing it on a deploy
// is silent: nothing errors, they simply stop matching anything, and the
// bootstrap loop clusters on these so it sees an empty city and posts nothing.
func TestCapacitiesSurviveARestart(t *testing.T) {
	dir := t.TempDir()
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))

	first, err := Open(key, "https://example.test", Options{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	first.Capacities.Set("worker-1", api.Capacity{
		Accepting: true, RangeMiles: 18, MaxConcurrent: 3,
		LatE7: api.E7(42.3314), LonE7: api.E7(-83.0458),
	})

	second, err := Open(key, "https://example.test", Options{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	got := second.Capacities.Get("worker-1")
	if !got.Accepting || got.RangeMiles != 18 || got.LatE7 != api.E7(42.3314) {
		t.Fatalf("capacity did not survive: %+v", got)
	}
	if !got.Positioned() {
		t.Error("the operator came back unpositioned, so nothing would reach them")
	}
}
