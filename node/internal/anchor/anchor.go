package anchor

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Status of a receipt hash or a batch.
const (
	// StatusPending: recorded, and either waiting for the next batch or
	// submitted and waiting for Bitcoin.
	StatusPending = "pending"
	// StatusAnchored: a Bitcoin block commits to it.
	StatusAnchored = "anchored"
)

// Options configure an Anchorer.
type Options struct {
	// Calendars are the OpenTimestamps servers to submit roots to. Empty
	// means DefaultCalendars.
	Calendars []string
	// Every is how often unanchored hashes are batched. Zero means an hour.
	Every time.Duration
	// HTTP is the client used to reach calendars; nil means a 15s timeout.
	HTTP *http.Client
	// Now is the clock, replaceable for tests.
	Now func() time.Time
	// Logf receives one line per batch event. Nil means the standard logger.
	Logf func(format string, args ...any)
}

// entry is one line of the append-only receipt log.
type entry struct {
	SHA256 string `json:"sha256"`
	Job    string `json:"job"`
	// Fingerprint is the receipt minus its issue time. A receipt whose
	// fingerprint is already on file is re-served with its original issue
	// time rather than re-issued, which is what keeps its hash stable.
	Fingerprint string `json:"fingerprint"`
	IssuedAt    string `json:"issued_at"`
	RecordedAt  string `json:"recorded_at"`
}

// Batch is one Merkle tree and what happened to its root.
type Batch struct {
	Seq       int       `json:"seq"`
	Root      string    `json:"root"`
	Leaves    []string  `json:"leaves"`
	CreatedAt time.Time `json:"created_at"`
	// Calendars that accepted the root. Empty until one does.
	Calendars   []string   `json:"calendars,omitempty"`
	SubmittedAt *time.Time `json:"submitted_at,omitempty"`
	// OTS is the detached proof file, once at least one calendar answered.
	OTS           []byte     `json:"ots,omitempty"`
	AnchoredAt    *time.Time `json:"anchored_at,omitempty"`
	BitcoinHeight int        `json:"bitcoin_height,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
}

// Status is pending until a Bitcoin block attests to the root.
func (b *Batch) Status() string {
	if b.AnchoredAt != nil {
		return StatusAnchored
	}
	return StatusPending
}

// Proof is everything a third party needs to tie one receipt to Bitcoin.
type Proof struct {
	ReceiptSHA256 string     `json:"receipt_sha256"`
	Job           string     `json:"job"`
	RecordedAt    string     `json:"recorded_at"`
	Status        string     `json:"status"`
	MerkleRoot    string     `json:"merkle_root,omitempty"`
	InclusionPath []Step     `json:"inclusion_path,omitempty"`
	OTSProof      []byte     `json:"ots_proof,omitempty"`
	Calendars     []string   `json:"calendars,omitempty"`
	SubmittedAt   *time.Time `json:"submitted_at,omitempty"`
	AnchoredAt    *time.Time `json:"anchored_at,omitempty"`
	BitcoinHeight int        `json:"bitcoin_height,omitempty"`
	// Note says, in words, what is still to happen when it is not anchored.
	Note string `json:"note,omitempty"`
}

// Anchorer collects receipt hashes and gets their roots onto Bitcoin.
type Anchorer struct {
	dir       string
	calendars []string
	every     time.Duration
	client    *calendar
	now       func() time.Time
	logf      func(string, ...any)

	mu      sync.Mutex
	entries []entry
	bySHA   map[string]int
	byJob   map[string][]int
	// leafBatch maps a receipt hash to the batch that holds it.
	leafBatch map[string]int
	batches   []*Batch
	logFile   *os.File
}

// Open loads the log and batches under dir, creating it if needed.
func Open(dir string, opt Options) (*Anchorer, error) {
	if dir == "" {
		return nil, errors.New("anchor: a data directory is required")
	}
	if err := os.MkdirAll(filepath.Join(dir, "batches"), 0o750); err != nil {
		return nil, err
	}
	a := &Anchorer{
		dir:       dir,
		calendars: opt.Calendars,
		every:     opt.Every,
		client:    newCalendar(opt.HTTP),
		now:       opt.Now,
		logf:      opt.Logf,
		bySHA:     map[string]int{},
		byJob:     map[string][]int{},
		leafBatch: map[string]int{},
	}
	if len(a.calendars) == 0 {
		a.calendars = DefaultCalendars
	}
	if a.every <= 0 {
		a.every = time.Hour
	}
	if a.now == nil {
		a.now = time.Now
	}
	if a.logf == nil {
		a.logf = log.Printf
	}
	if err := a.loadLog(); err != nil {
		return nil, err
	}
	if err := a.loadBatches(); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "receipts.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return nil, err
	}
	a.logFile = f
	return a, nil
}

// Close releases the log file.
func (a *Anchorer) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.logFile == nil {
		return nil
	}
	err := a.logFile.Close()
	a.logFile = nil
	return err
}

// Calendars is where roots are sent.
func (a *Anchorer) Calendars() []string { return append([]string{}, a.calendars...) }

// Every is the batching interval.
func (a *Anchorer) Every() time.Duration { return a.every }

func (a *Anchorer) loadLog() error {
	f, err := os.Open(filepath.Join(a.dir, "receipts.log"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			// A torn last line from a crash mid-write is the one thing an
			// append-only log cannot promise against. Skip it; the receipt
			// will be recorded again the next time it is served.
			continue
		}
		a.index(e)
	}
	return sc.Err()
}

func (a *Anchorer) index(e entry) {
	if _, dup := a.bySHA[e.SHA256]; dup {
		return
	}
	a.entries = append(a.entries, e)
	i := len(a.entries) - 1
	a.bySHA[e.SHA256] = i
	a.byJob[e.Job] = append(a.byJob[e.Job], i)
}

func (a *Anchorer) loadBatches() error {
	names, err := filepath.Glob(filepath.Join(a.dir, "batches", "*.json"))
	if err != nil {
		return err
	}
	for _, n := range names {
		b, err := os.ReadFile(n)
		if err != nil {
			return err
		}
		var batch Batch
		if err := json.Unmarshal(b, &batch); err != nil {
			return fmt.Errorf("anchor: %s: %w", n, err)
		}
		a.batches = append(a.batches, &batch)
	}
	sort.Slice(a.batches, func(i, j int) bool { return a.batches[i].Seq < a.batches[j].Seq })
	for _, b := range a.batches {
		for _, leaf := range b.Leaves {
			a.leafBatch[leaf] = b.Seq
		}
	}
	return nil
}

func (a *Anchorer) saveBatch(b *Batch) error {
	raw, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(a.dir, "batches", fmt.Sprintf("%08d.json", b.Seq))
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o640); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// IssuedAt returns the issue time already on file for a receipt of this job
// with this fingerprint, so an unchanged receipt is served as it was first
// issued rather than as a new document every time.
func (a *Anchorer) IssuedAt(job, fingerprint string) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	idx := a.byJob[job]
	for i := len(idx) - 1; i >= 0; i-- {
		if e := a.entries[idx[i]]; e.Fingerprint == fingerprint {
			return e.IssuedAt, true
		}
	}
	return "", false
}

// Record notes that a receipt with this hash was served. Recording the same
// hash twice is a no-op. It never touches the network.
func (a *Anchorer) Record(job, fingerprint, issuedAt string, sha256Hex string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, dup := a.bySHA[sha256Hex]; dup {
		return
	}
	e := entry{
		SHA256: sha256Hex, Job: job, Fingerprint: fingerprint,
		IssuedAt: issuedAt, RecordedAt: a.now().UTC().Format(time.RFC3339),
	}
	a.index(e)
	if a.logFile != nil {
		raw, _ := json.Marshal(e)
		if _, err := a.logFile.Write(append(raw, '\n')); err != nil {
			a.logf("anchor: could not append to receipts.log: %v", err)
		}
	}
}

// Latest is the most recently recorded receipt hash for a job.
func (a *Anchorer) Latest(job string) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	idx := a.byJob[job]
	if len(idx) == 0 {
		return "", false
	}
	return a.entries[idx[len(idx)-1]].SHA256, true
}

// Proof assembles what is known about one receipt hash.
func (a *Anchorer) Proof(sha256Hex string) (Proof, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	i, ok := a.bySHA[sha256Hex]
	if !ok {
		return Proof{}, false
	}
	e := a.entries[i]
	p := Proof{
		ReceiptSHA256: e.SHA256, Job: e.Job, RecordedAt: e.RecordedAt,
		Status: StatusPending,
	}
	seq, batched := a.leafBatch[e.SHA256]
	if !batched {
		p.Note = fmt.Sprintf("recorded; it will be included in the next batch (every %s)", a.every)
		return p, true
	}
	b := a.batchLocked(seq)
	if b == nil {
		return p, true
	}
	p.MerkleRoot = b.Root
	for li, leaf := range b.Leaves {
		if leaf == e.SHA256 {
			p.InclusionPath, _ = merklePath(b.Leaves, li)
			break
		}
	}
	p.Status = b.Status()
	p.Calendars = append([]string{}, b.Calendars...)
	p.OTSProof = b.OTS
	p.SubmittedAt, p.AnchoredAt, p.BitcoinHeight = b.SubmittedAt, b.AnchoredAt, b.BitcoinHeight
	switch {
	case b.SubmittedAt == nil:
		p.Note = "batched; the root has not yet been accepted by a calendar and will be retried"
		if b.LastError != "" {
			p.Note += " (last attempt: " + b.LastError + ")"
		}
	case b.AnchoredAt == nil:
		p.Note = "submitted; the calendar commits to Bitcoin within hours, after which the proof upgrades"
	}
	return p, true
}

func (a *Anchorer) batchLocked(seq int) *Batch {
	for _, b := range a.batches {
		if b.Seq == seq {
			return b
		}
	}
	return nil
}

// Batches lists the most recent batches, newest first, without their leaves.
func (a *Anchorer) Batches(limit int) []Batch {
	a.mu.Lock()
	defer a.mu.Unlock()
	if limit <= 0 || limit > len(a.batches) {
		limit = len(a.batches)
	}
	out := make([]Batch, 0, limit)
	for i := len(a.batches) - 1; i >= 0 && len(out) < limit; i-- {
		b := *a.batches[i]
		b.Leaves = nil
		out = append(out, b)
	}
	return out
}

// Pending counts receipts recorded but not yet in any batch.
func (a *Anchorer) Pending() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	n := 0
	for _, e := range a.entries {
		if _, ok := a.leafBatch[e.SHA256]; !ok {
			n++
		}
	}
	return n
}

// Batch builds a tree over every unbatched hash, persists it, and submits
// the root. A calendar being down leaves the batch pending for a retry;
// only an error writing to disk is returned. Nil means nothing to do.
func (a *Anchorer) Batch(ctx context.Context) (*Batch, error) {
	a.mu.Lock()
	var leaves []string
	for _, e := range a.entries {
		if _, ok := a.leafBatch[e.SHA256]; !ok {
			leaves = append(leaves, e.SHA256)
		}
	}
	if len(leaves) == 0 {
		a.mu.Unlock()
		return nil, nil
	}
	root, err := merkleRoot(leaves)
	if err != nil {
		a.mu.Unlock()
		return nil, err
	}
	seq := 1
	if n := len(a.batches); n > 0 {
		seq = a.batches[n-1].Seq + 1
	}
	b := &Batch{Seq: seq, Root: root, Leaves: leaves, CreatedAt: a.now().UTC()}
	if err := a.saveBatch(b); err != nil {
		a.mu.Unlock()
		return nil, err
	}
	a.batches = append(a.batches, b)
	for _, l := range leaves {
		a.leafBatch[l] = seq
	}
	a.mu.Unlock()

	a.submit(ctx, b)
	return b, nil
}

// submit sends a batch's root to every calendar and keeps what comes back.
func (a *Anchorer) submit(ctx context.Context, b *Batch) {
	digest, _ := hex.DecodeString(b.Root)
	var merged *Timestamp
	var accepted []string
	var lastErr string
	for _, cal := range a.calendars {
		t, err := a.client.submit(ctx, cal, digest)
		if err != nil {
			lastErr = err.Error()
			continue
		}
		if merged == nil {
			merged = t
		} else if err := merged.Merge(t); err != nil {
			lastErr = err.Error()
			continue
		}
		accepted = append(accepted, cal)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if merged == nil {
		b.LastError = lastErr
		_ = a.saveBatch(b)
		a.logf("anchor: batch %d root %s (%d receipts) could not reach a calendar: %s; will retry",
			b.Seq, short(b.Root), len(b.Leaves), lastErr)
		return
	}
	ots, err := SerializeFile(digest, merged)
	if err != nil {
		b.LastError = err.Error()
		_ = a.saveBatch(b)
		return
	}
	now := a.now().UTC()
	b.OTS, b.Calendars, b.SubmittedAt, b.LastError = ots, accepted, &now, ""
	_ = a.saveBatch(b)
	a.logf("anchor: batch %d root %s (%d receipts) submitted to %s",
		b.Seq, short(b.Root), len(b.Leaves), strings.Join(accepted, ", "))
}

// Upgrade retries unsubmitted batches and asks calendars whether submitted
// ones have reached Bitcoin. It reports how many batches became anchored.
func (a *Anchorer) Upgrade(ctx context.Context) int {
	a.mu.Lock()
	var retry, waiting []*Batch
	for _, b := range a.batches {
		switch {
		case b.SubmittedAt == nil:
			retry = append(retry, b)
		case b.AnchoredAt == nil:
			waiting = append(waiting, b)
		}
	}
	a.mu.Unlock()

	for _, b := range retry {
		a.submit(ctx, b)
	}
	anchored := 0
	for _, b := range waiting {
		a.mu.Lock()
		digest, t, err := ParseFile(b.OTS)
		a.mu.Unlock()
		if err != nil {
			continue
		}
		if !a.client.upgrade(ctx, t) {
			continue
		}
		ots, err := SerializeFile(digest, t)
		if err != nil {
			continue
		}
		a.mu.Lock()
		b.OTS = ots
		if h, done := t.Complete(); done {
			now := a.now().UTC()
			b.AnchoredAt, b.BitcoinHeight = &now, h
			anchored++
			a.logf("anchor: batch %d root %s anchored in bitcoin block %d",
				b.Seq, short(b.Root), h)
		}
		_ = a.saveBatch(b)
		a.mu.Unlock()
	}
	return anchored
}

// Start runs batching and upgrading on the interval until ctx ends. Network
// work happens here and nowhere else, so serving a receipt never waits on a
// calendar.
func (a *Anchorer) Start(ctx context.Context) {
	go func() {
		t := time.NewTicker(a.every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if _, err := a.Batch(ctx); err != nil {
					a.logf("anchor: batching failed: %v", err)
				}
				a.Upgrade(ctx)
			}
		}
	}()
}

func short(root string) string {
	if len(root) > 12 {
		return root[:12] + "…"
	}
	return root
}
