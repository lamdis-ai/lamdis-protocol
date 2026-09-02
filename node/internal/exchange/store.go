package exchange

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/ledger"
)

// What the exchange knew and could not remember.
//
// The ledger, the holdbacks, the payout accounts and the anchors were all on
// the durable disk. Four maps on Server were not: the evidence bytes, the
// submissions those bytes belong to, the capability secrets that let a worker
// reach their job at all, and the record of which buyer funded which job.
//
// Every one of them is load-bearing. Lose the buyer map and a refund has
// nowhere to go. Lose the secrets and a worker holding a link in a URL
// fragment is told their capability failed verification. Lose the submissions
// and work that was done and verified is simply not there — while the ledger,
// which did survive, still shows the escrow. Lose the blobs and a receipt
// points at a hash of nothing.
//
// Two formats, for one reason. Evidence is megabytes of photograph and video;
// it goes to files under blobs/, content-addressed, written once and never
// rewritten. Everything else is small, mutually consistent state, so it goes
// to whole-file JSON snapshots written atomically — the same shape as
// holdbacks.json and payout-accounts.json next to it. SQLite would buy
// transactions across the four, and the four are not transactionally related:
// a submission is durable before it is settled, and settlement is the
// ledger's job, which already has a database. Adding a second one to store
// four maps would be more machinery than the problem has.

// stateStore is the exchange's own corner of the data directory.
type stateStore struct {
	blobDir string
	subs    string
	secrets string
	buyers  string
}

// openState prepares the directory. An empty dir means no store at all, which
// is what a test and `lamdis exchange` with no -data rely on: nothing is
// written and nothing is read.
func openState(dir string) (*stateStore, error) {
	if dir == "" {
		return nil, nil
	}
	st := &stateStore{
		blobDir: filepath.Join(dir, "blobs"),
		subs:    filepath.Join(dir, "submissions.json"),
		secrets: filepath.Join(dir, "job-secrets.json"),
		buyers:  filepath.Join(dir, "job-buyers.json"),
	}
	if err := os.MkdirAll(st.blobDir, 0o750); err != nil {
		return nil, fmt.Errorf("preparing the evidence store: %w", err)
	}
	return st, nil
}

// blobPath names the file for one content hash.
//
// The hash is checked rather than trusted: it reaches this function from an
// upload, and a "sha" containing a slash or a pair of dots would otherwise
// write outside the store.
func (st *stateStore) blobPath(sha string) (string, error) {
	if sha == "" || len(sha) > 128 {
		return "", fmt.Errorf("evidence: implausible content hash")
	}
	for _, r := range sha {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return "", fmt.Errorf("evidence: content hash is not hex")
		}
	}
	return filepath.Join(st.blobDir, sha), nil
}

// writeBlob stores the bytes and, beside them, what they are.
//
// The mime lives in a sidecar rather than a shared index so an upload never
// rewrites a file that grows with the exchange. Content-addressed, so writing
// the same evidence twice is idempotent and re-writing it is harmless.
func (st *stateStore) writeBlob(sha, mime string, data []byte) error {
	p, err := st.blobPath(sha)
	if err != nil {
		return err
	}
	if err := api.WriteFileAtomic(p, data); err != nil {
		return err
	}
	if mime != "" {
		if err := api.WriteFileAtomic(p+".mime", []byte(mime)); err != nil {
			return err
		}
	}
	return nil
}

func (st *stateStore) readBlob(sha string) ([]byte, error) {
	p, err := st.blobPath(sha)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(p)
}

// mimes reads back what each stored artifact is. Cheap: the sidecars are a few
// bytes each and there is one per artifact.
func (st *stateStore) mimes() map[string]string {
	out := map[string]string{}
	entries, err := os.ReadDir(st.blobDir)
	if err != nil {
		log.Printf("evidence   could not list %s: %v", st.blobDir, err)
		return out
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".mime") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(st.blobDir, name))
		if err != nil {
			continue
		}
		out[strings.TrimSuffix(name, ".mime")] = strings.TrimSpace(string(b))
	}
	return out
}

// readJSON loads one snapshot into v, and never stops the exchange booting.
//
// A truncated file — a machine that lost power mid-rename, a disk that filled
// — leaves the exchange with less state, not with no exchange. Reported
// plainly and left in place, because an operator can read a broken file and
// cannot read one this process deleted.
func readJSON(path string, v any) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("exchange   could not read %s: %v — continuing without it", path, err)
		}
		return
	}
	if err := json.Unmarshal(raw, v); err != nil {
		log.Printf("exchange   %s is unreadable (%v) — continuing without it; "+
			"the file is left in place for inspection", path, err)
	}
}

// loadState brings back what the last process knew. Called from Open before
// anything is served.
func (s *Server) loadState() {
	if s.store == nil {
		return
	}
	var (
		subs    map[string][]api.Submission
		secrets map[string][]string
		buyers  map[string]string
	)
	readJSON(s.store.subs, &subs)
	readJSON(s.store.secrets, &secrets)
	readJSON(s.store.buyers, &buyers)

	s.mu.Lock()
	defer s.mu.Unlock()
	for job, v := range subs {
		s.submissions[job] = v
	}
	for job, v := range secrets {
		s.secrets[job] = v
	}
	for job, v := range buyers {
		s.buyers[job] = v
	}
	s.blobMime = s.store.mimes()
	n := 0
	for _, v := range s.submissions {
		n += len(v)
	}
	log.Printf("exchange   restored %d submission(s) on %d job(s), %d link set(s), "+
		"%d funded job(s), %d stored artifact(s)",
		n, len(s.submissions), len(s.secrets), len(s.buyers), len(s.blobMime))
}

// The three snapshot writers. Each is called with s.mu held, so what reaches
// the disk is the map as it stood at the moment of the change.

func (s *Server) saveSubmissionsLocked() {
	if s.store == nil {
		return
	}
	writeJSONFile(s.store.subs, s.submissions)
}

func (s *Server) saveSecretsLocked() {
	if s.store == nil {
		return
	}
	writeJSONFile(s.store.secrets, s.secrets)
}

func (s *Server) saveBuyersLocked() {
	if s.store == nil {
		return
	}
	writeJSONFile(s.store.buyers, s.buyers)
}

func writeJSONFile(path string, v any) {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		log.Printf("exchange   could not encode %s: %v", path, err)
		return
	}
	if err := api.WriteFileAtomic(path, raw); err != nil {
		log.Printf("exchange   could not write %s: %v", path, err)
	}
}

// putBlob records evidence under its content hash.
//
// With a store the bytes go to disk and stay there: holding a megabyte of
// photograph in a map for the life of the process is what the durable disk is
// for. Without one, the old behaviour exactly — the bytes live in memory and
// die with the process.
func (s *Server) putBlob(sha, mime string, data []byte) {
	s.mu.Lock()
	st := s.store
	if st == nil {
		s.blobs[sha] = data
	}
	if mime != "" {
		s.blobMime[sha] = mime
	}
	s.mu.Unlock()
	if st == nil {
		return
	}
	if err := st.writeBlob(sha, mime, data); err != nil {
		// Keep them in memory so this process can still serve the evidence
		// somebody just uploaded. It will not survive a restart, and the log
		// says so rather than the worker discovering it later.
		log.Printf("evidence   could not store %s: %v — keeping it in memory only, "+
			"it will not survive a restart", sha, err)
		s.mu.Lock()
		s.blobs[sha] = data
		s.mu.Unlock()
	}
}

// mimeFor is what the bytes were recorded as, or empty if nothing knows.
func (s *Server) mimeFor(sha string) string {
	s.mu.Lock()
	mime, st := s.blobMime[sha], s.store
	s.mu.Unlock()
	if mime != "" || st == nil {
		return mime
	}
	p, err := st.blobPath(sha)
	if err != nil {
		return ""
	}
	b, err := os.ReadFile(p + ".mime")
	if err != nil {
		return ""
	}
	mime = strings.TrimSpace(string(b))
	if mime != "" {
		s.mu.Lock()
		s.blobMime[sha] = mime
		s.mu.Unlock()
	}
	return mime
}

// setSecrets records the capability secrets issued for a job.
func (s *Server) setSecrets(job string, secrets []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secrets[job] = secrets
	s.saveSecretsLocked()
}

// setBuyer remembers who funded a job, which is where a refund has to go.
func (s *Server) setBuyer(job, principal string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buyers[job] = principal
	s.saveBuyersLocked()
}

// reconcile says out loud what came back inconsistent, and changes nothing.
//
// The rule throughout is: report, do not repair. Every discrepancy here is
// somebody's money or somebody's afternoon, and a boot-time sweep that deletes
// a listing whose escrow looks short, or drops a submission whose photograph
// is missing, destroys the evidence a person would need to argue about it. A
// line in the log costs nothing and can be acted on; a silent deletion cannot
// be undone.
func (s *Server) reconcile(ctx context.Context) {
	if s.store == nil {
		return
	}
	now := s.now()
	listings := s.Board.All()
	known := make(map[string]bool, len(listings))
	currencies := map[string]bool{}
	for _, l := range listings {
		known[l.Job] = true
		cur := l.Currency
		if cur == "" {
			cur = "USD"
		}
		currencies[cur] = true

		if l.Cancelled {
			continue
		}
		if !now.Before(l.Expires) {
			log.Printf("reconcile  %s expired at %s and is still on the board — "+
				"kept, so its escrow can be returned rather than stranded",
				l.Job, l.Expires.UTC().Format(time.RFC3339))
		}
		if s.Ledger == nil {
			continue
		}
		need := MaxPayoutFor(l)
		held, err := s.Ledger.Held(ctx, l.Job, cur)
		if err != nil {
			log.Printf("reconcile  could not read the escrow for %s: %v", l.Job, err)
			continue
		}
		if held < need {
			// Not delisted. A job whose escrow has been partly paid out is the
			// normal state of a part-finished panel, and one whose escrow has
			// genuinely gone is a question for a person, not for a boot sweep.
			log.Printf("reconcile  %s could pay out %d %s and escrow holds %d — "+
				"listing kept; check before anyone else claims it",
				l.Job, need, cur, held)
		}
	}

	// The other direction, which is the failure this whole change is about:
	// money committed to a job that no longer exists anywhere.
	if s.Ledger != nil {
		if len(currencies) == 0 {
			currencies["USD"] = true
		}
		for cur := range currencies {
			accounts, err := s.Ledger.Accounts(ctx, cur)
			if err != nil {
				log.Printf("reconcile  could not list %s accounts: %v", cur, err)
				continue
			}
			for acct, bal := range accounts {
				job, ok := strings.CutPrefix(acct, ledger.EscrowOf(""))
				if !ok || bal <= 0 || known[job] {
					continue
				}
				log.Printf("reconcile  escrow holds %d %s for %s, which is on no listing — "+
					"nothing was moved; it needs returning to its buyer by hand",
					bal, cur, job)
			}
		}
	}

	// Evidence a submission points at and the store no longer has.
	s.mu.Lock()
	missing := map[string][]string{}
	for job, subs := range s.submissions {
		for _, sub := range subs {
			for _, a := range sub.Artifacts {
				if a.SHA256 == "" {
					continue
				}
				if _, err := s.store.blobPath(a.SHA256); err != nil {
					missing[job] = append(missing[job], a.SHA256)
					continue
				}
				if _, ok := s.blobs[a.SHA256]; ok {
					continue
				}
				if _, err := s.store.readBlob(a.SHA256); err != nil {
					missing[job] = append(missing[job], a.SHA256)
				}
			}
		}
	}
	s.mu.Unlock()
	for job, shas := range missing {
		// The submission stays. Its hash and its verdict are on the receipt,
		// and those are what a dispute is argued from; the bytes are what a
		// person would like to look at. Losing the second is not a reason to
		// throw away the first.
		log.Printf("reconcile  %s has %d submission artifact(s) whose bytes are gone "+
			"(%s) — the submissions and their verdicts are kept",
			job, len(shas), strings.Join(shas, ", "))
	}
}
