package api

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"
)

// The board is the marketplace, and until this file existed the marketplace
// was a process.
//
// A restart threw away every open listing, every seat somebody was holding and
// every capability secret that had been issued — while the escrow those
// listings were funded from stayed in the ledger, which is durable. The result
// is the worst arrangement of the two: the money is still committed, and the
// job it was committed to no longer exists. A worker part-way through an
// errand, holding a link in a URL fragment, had nowhere to submit and no way
// to be paid; the buyer's money sat in an escrow account naming a job nobody
// could find.
//
// So the board writes itself down. The format is a single JSON snapshot,
// rewritten atomically on mutation, matching holdbacks.json and
// payout-accounts.json rather than inventing a third convention. It is not a
// database: at this volume — hundreds of listings, not millions — a whole-file
// rewrite costs less than a millisecond, and the thing that must never happen
// is a half-written board, which temp-file-and-rename rules out. If the board
// ever outgrows that, the seam is saveLocked and nothing above it changes.
//
// Reads never write. Snapshotting on every board view would rewrite the file
// on every page load; the only read path that mutates is a lapsed lease, and
// that saves because it genuinely changed something.

// storedListing carries a listing through JSON without losing the four fields
// Listing deliberately keeps off the wire.
//
// Owner, Funding, MaxBidMinor and Challenge are tagged `json:"-"` because
// publishing them would hand an anonymous caller the buyer's identity, their
// card authorisation, the ceiling every bid would then land on, and the code
// that is supposed to prove somebody actually went. All four are also exactly
// what a restart must not lose — a listing that comes back without its Owner
// cannot be settled or refunded — so the snapshot names them itself.
type storedListing struct {
	*Listing
	MaxBidMinor int64    `json:"max_bid_minor,omitempty"`
	Owner       string   `json:"owner,omitempty"`
	Funding     *Funding `json:"funding,omitempty"`
	Challenge   string   `json:"challenge,omitempty"`
}

// boardFile is the whole of the board's mutable state.
//
// Every map in Board appears here. A field added to Board and forgotten here
// is a field that silently resets on the next deploy, which is the bug this
// file exists to end, so the two lists are kept in the same order.
type boardFile struct {
	Listings    map[string]*storedListing          `json:"listings,omitempty"`
	Claims      map[string]int                     `json:"claims,omitempty"`
	Bids        map[string][]*Bid                  `json:"bids,omitempty"`
	ProjectBids map[string][]*ProjectBid           `json:"project_bids,omitempty"`
	Leases      map[string]map[string]time.Time    `json:"leases,omitempty"`
	Rejected    map[string]int                     `json:"rejected,omitempty"`
	Abandoned   map[string]int                     `json:"abandoned,omitempty"`
	CoolUntil   map[string]time.Time               `json:"cool_until,omitempty"`
	Seats       map[string]map[string]bool         `json:"seats,omitempty"`
	Completed   map[string]int                     `json:"completed,omitempty"`
	Worked      map[string]map[string]bool         `json:"worked,omitempty"`
	Done        map[string]map[string]map[int]bool `json:"done,omitempty"`
	Owner       map[string]string                  `json:"owner,omitempty"`
	Secrets     map[string][]string                `json:"secrets,omitempty"`
}

// Persist points the board at a directory and loads whatever is already
// there.
//
// An empty directory leaves the board purely in memory, which is what a test
// and `lamdis exchange` with no -data both expect: nothing is read, nothing is
// written, and behaviour is identical to before this file existed.
func (b *Board) Persist(dir string) {
	if dir == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.path = filepath.Join(dir, "board.json")
	b.loadLocked()
}

// loadLocked reads the snapshot back, and refuses to die on a bad one.
//
// A truncated or hand-edited file must not stop the exchange booting. An
// exchange that will not start is worse than an exchange that starts with an
// empty board: the first strands everybody, the second at least serves the
// ledger, the payouts and the console while somebody looks at the file.
func (b *Board) loadLocked() {
	raw, err := os.ReadFile(b.path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("board      could not read %s: %v — starting empty", b.path, err)
		}
		return
	}
	var f boardFile
	if err := json.Unmarshal(raw, &f); err != nil {
		log.Printf("board      %s is unreadable (%v) — starting with an empty board; "+
			"the file is left in place for inspection", b.path, err)
		return
	}
	for job, sl := range f.Listings {
		if sl == nil || sl.Listing == nil {
			log.Printf("board      listing %s could not be read back and was skipped", job)
			continue
		}
		l := sl.Listing
		l.MaxBidMinor, l.Owner, l.Funding, l.Challenge =
			sl.MaxBidMinor, sl.Owner, sl.Funding, sl.Challenge
		b.listings[job] = l
	}
	mergeInts(b.claims, f.Claims)
	mergeInts(b.rejected, f.Rejected)
	mergeInts(b.abandoned, f.Abandoned)
	mergeInts(b.completed, f.Completed)
	for k, v := range f.CoolUntil {
		b.coolUntil[k] = v
	}
	for k, v := range f.Owner {
		b.owner[k] = v
	}
	for k, v := range f.Secrets {
		b.secrets[k] = v
	}
	for k, v := range f.Leases {
		b.leases[k] = v
	}
	for k, v := range f.Seats {
		b.seats[k] = v
	}
	for k, v := range f.Worked {
		b.worked[k] = v
	}
	for k, v := range f.Done {
		b.done[k] = v
	}
	if f.Bids != nil {
		b.bids = f.Bids
	}
	if f.ProjectBids != nil {
		b.projectBids = f.ProjectBids
	}
	if b.bids == nil {
		b.bids = map[string][]*Bid{}
	}
	if b.projectBids == nil {
		b.projectBids = map[string][]*ProjectBid{}
	}
	log.Printf("board      restored %d listing(s), %d seat map(s), %d secret set(s) from %s",
		len(b.listings), len(b.seats), len(b.secrets), b.path)
}

func mergeInts(dst, src map[string]int) {
	for k, v := range src {
		dst[k] = v
	}
}

// saveLocked writes the board down. The caller holds b.mu.
//
// Failures are logged and swallowed: losing a write is bad, and taking down a
// live claim because a disk was briefly full is worse. The next mutation
// rewrites the whole file, so a single missed write heals itself.
func (b *Board) saveLocked() {
	if b.path == "" {
		return
	}
	f := boardFile{
		Claims: b.claims, Bids: b.bids, ProjectBids: b.projectBids,
		Leases: b.leases, Rejected: b.rejected, Abandoned: b.abandoned,
		CoolUntil: b.coolUntil, Seats: b.seats, Completed: b.completed,
		Worked: b.worked, Done: b.done, Owner: b.owner, Secrets: b.secrets,
	}
	if len(b.listings) > 0 {
		f.Listings = make(map[string]*storedListing, len(b.listings))
		for job, l := range b.listings {
			f.Listings[job] = &storedListing{
				Listing: l, MaxBidMinor: l.MaxBidMinor, Owner: l.Owner,
				Funding: l.Funding, Challenge: l.Challenge,
			}
		}
	}
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		log.Printf("board      could not encode the board: %v", err)
		return
	}
	if err := WriteFileAtomic(b.path, raw); err != nil {
		log.Printf("board      could not write %s: %v", b.path, err)
	}
}

// capabilityFile is the capability registry on disk.
//
// The secrets in Board.secrets are only half of a working link: authentication
// computes the HMAC from the secret and then looks the capability up by
// sha256(secret) to find its job, its actions and its expiry. Persisting the
// secrets without this map would restore a link that authenticates and is then
// refused as expired, which is the same dead link with a more confusing
// message.
type capabilityFile struct {
	ByID map[string]*Capability `json:"by_id,omitempty"`
}

// Persist points the capability registry at a directory and loads it.
func (cs *Capabilities) Persist(dir string) {
	if dir == "" {
		return
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.path = filepath.Join(dir, "capabilities.json")
	raw, err := os.ReadFile(cs.path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("caps       could not read %s: %v — starting empty", cs.path, err)
		}
		return
	}
	var f capabilityFile
	if err := json.Unmarshal(raw, &f); err != nil {
		log.Printf("caps       %s is unreadable (%v) — every outstanding link will "+
			"need reissuing; the file is left in place", cs.path, err)
		return
	}
	now := cs.now()
	live := 0
	for id, c := range f.ByID {
		if c == nil {
			continue
		}
		if now.After(c.Expires) {
			// Dropped rather than kept: an expired capability authenticates
			// nothing, and carrying it forward only grows the file.
			continue
		}
		cs.byID[id] = c
		live++
	}
	if n := len(f.ByID) - live; n > 0 {
		log.Printf("caps       restored %d live link(s); %d had expired", live, n)
	} else {
		log.Printf("caps       restored %d live link(s)", live)
	}
}

func (cs *Capabilities) saveLocked() {
	if cs.path == "" {
		return
	}
	raw, err := json.MarshalIndent(capabilityFile{ByID: cs.byID}, "", "  ")
	if err != nil {
		log.Printf("caps       could not encode the capability registry: %v", err)
		return
	}
	if err := WriteFileAtomic(cs.path, raw); err != nil {
		log.Printf("caps       could not write %s: %v", cs.path, err)
	}
}

// WriteFileAtomic writes bytes so a reader never sees half of them.
//
// Temp file in the same directory, then rename, which is atomic within a
// filesystem. The mode matches the other stores on the data disk: these files
// name buyers, workers and the secrets that authorise work, and nothing on the
// host but this process has any business reading them.
func WriteFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// Capacities are what an operator told the exchange they can do, and where.
//
// Losing them on a deploy is quieter than losing the board and nearly as bad:
// nothing errors, work simply stops reaching people. An operator who set their
// range on Monday is unpositioned on Tuesday, matches nothing, receives no
// dispatch, and has no way to know why — and the bootstrap loop, which decides
// where to post by clustering these, sees an empty city and posts nothing.
// Neither failure announces itself, which is exactly why this has to be
// written down rather than rebuilt from whoever happens to visit the console.
func (cs *Capacities) Persist(dir string) {
	if dir == "" {
		return
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.path = filepath.Join(dir, "capacities.json")
	raw, err := os.ReadFile(cs.path)
	if err != nil {
		return
	}
	var by map[string]Capacity
	if err := json.Unmarshal(raw, &by); err != nil {
		log.Printf("capacity   %s is unreadable, starting empty: %v", cs.path, err)
		return
	}
	for worker, c := range by {
		cs.by[worker] = c
	}
	log.Printf("capacity   restored %d operator settings", len(cs.by))
}

// saveLocked writes the capacities down. The caller holds cs.mu.
func (cs *Capacities) saveLocked() {
	if cs.path == "" {
		return
	}
	raw, err := json.MarshalIndent(cs.by, "", "  ")
	if err != nil {
		log.Printf("capacity   could not encode: %v", err)
		return
	}
	if err := WriteFileAtomic(cs.path, raw); err != nil {
		log.Printf("capacity   could not write %s: %v", cs.path, err)
	}
}
