// Package sync replicates threads between nodes. The protocol is two verbs
// over any transport: List (which threads can I, principal P, see?) and
// Pull (given my version vector for a thread, stream what I'm missing —
// filtered to the lanes my scopes allow). Serving is always evaluated *as a
// principal*; a peer holding only summary scope is never sent a content
// entry, so out-of-scope data never reaches its disk.
package sync

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"

	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/perm"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/store"
)

// Head is one chain position in wire form.
type Head struct {
	Author string        `json:"author"`
	Lane   protolog.Lane `json:"lane"`
	Seq    uint64        `json:"seq"`
}

// PullRequest asks for everything after the caller's heads in one thread.
type PullRequest struct {
	Thread string `json:"thread"`
	Have   []Head `json:"have"`
}

// PullResponse carries missing entries in per-chain seq order, plus the
// server's own heads (scoped to the caller's lanes) so the caller can
// compute what the server is missing and push it back.
type PullResponse struct {
	Entries []*protolog.Entry `json:"entries"`
	Heads   []Head            `json:"heads"`
}

// PushRequest offers entries authored by the caller (or its delegated keys).
type PushRequest struct {
	Thread  string            `json:"thread"`
	Entries []*protolog.Entry `json:"entries"`
}

// PushResponse reports how many entries the server admitted.
type PushResponse struct {
	Accepted int `json:"accepted"`
}

// Transport is what a client needs from a remote peer.
type Transport interface {
	List(ctx context.Context) ([]string, error)
	Pull(ctx context.Context, req PullRequest) (*PullResponse, error)
	Push(ctx context.Context, req PushRequest) (*PushResponse, error)
}

// Server answers List/Pull against a local store, enforcing permissions.
type Server struct {
	Store store.Store
	Now   func() time.Time
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// visibleLanes folds the thread's permissions and returns the lanes the
// principal may replicate (nil = thread invisible to them).
func (s *Server) visibleLanes(ctx context.Context, threadID, principal string) ([]protolog.Lane, error) {
	tl, err := s.Store.Thread(ctx, threadID)
	if err != nil {
		return nil, err
	}
	st := perm.Fold(threadID, tl.Entries())
	return st.Lanes(principal, s.now()), nil
}

// List returns the ids of threads the principal holds any scope on.
func (s *Server) List(ctx context.Context, principal string) ([]string, error) {
	ids, err := s.Store.Threads(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, id := range ids {
		lanes, err := s.visibleLanes(ctx, id, principal)
		if err != nil {
			return nil, err
		}
		if len(lanes) > 0 {
			out = append(out, id)
		}
	}
	return out, nil
}

// Pull returns entries the principal is missing, restricted to permitted
// lanes. Chains stream in ascending seq so receivers can verify as they go.
func (s *Server) Pull(ctx context.Context, principal string, req PullRequest) (*PullResponse, error) {
	lanes, err := s.visibleLanes(ctx, req.Thread, principal)
	if err != nil {
		return nil, err
	}
	if len(lanes) == 0 {
		// Indistinguishable from a nonexistent thread: no oracle for strangers.
		return nil, fmt.Errorf("thread %s not found", req.Thread)
	}
	permitted := map[protolog.Lane]bool{}
	for _, l := range lanes {
		permitted[l] = true
	}
	have := map[protolog.ChainKey]uint64{}
	for _, h := range req.Have {
		have[protolog.ChainKey{Thread: req.Thread, Author: h.Author, Lane: h.Lane}] = h.Seq
	}
	tl, err := s.Store.Thread(ctx, req.Thread)
	if err != nil {
		return nil, err
	}
	resp := &PullResponse{}
	for key, seq := range tl.Heads() {
		if !permitted[key.Lane] {
			continue
		}
		resp.Heads = append(resp.Heads, Head{Author: key.Author, Lane: key.Lane, Seq: seq})
		for _, e := range tl.After(key, have[key]) {
			resp.Entries = append(resp.Entries, e)
		}
	}
	return resp, nil
}

// Push admits entries offered by principal. Rules, all fail-closed:
// every entry is authored by the caller (or a key delegated to the caller);
// summary/content require the contribute window; control-lane entries are
// accepted only from the thread's stewards — which is what lets a steward
// host a thread on a hub node (including bootstrapping it with the genesis)
// and keep grants/revocations flowing to it on every sync.
func (s *Server) Push(ctx context.Context, principal string, req PushRequest) (*PushResponse, error) {
	ordered := orderForAppend(req.Entries, req.Thread)
	total := len(ordered)
	tl, err := s.Store.Thread(ctx, req.Thread)
	if err != nil {
		// Bootstrap: an unknown thread is admitted only when the batch opens
		// with its genesis, authored by the authenticated principal. Anyone
		// may host their own thread anywhere they're paired.
		if len(ordered) == 0 || ordered[0].ID != req.Thread || ordered[0].Author != principal {
			return nil, fmt.Errorf("thread %s not found", req.Thread)
		}
		if err := s.Store.AppendEntries(ctx, ordered[:1]); err != nil {
			return nil, err
		}
		if tl, err = s.Store.Thread(ctx, req.Thread); err != nil {
			return nil, err
		}
		ordered = ordered[1:]
	} else if lanes, _ := s.visibleLanes(ctx, req.Thread, principal); len(lanes) == 0 {
		return nil, fmt.Errorf("thread %s not found", req.Thread)
	}
	// Two passes: control first, then the rest against the fold that
	// includes it. A person may push their own delegation or access request
	// into a thread they do not steward — that is how a collaborator's agent
	// becomes verifiable here — but never a grant.
	st := perm.Fold(req.Thread, tl.Entries())
	var control, rest []*protolog.Entry
	for _, e := range ordered {
		if e.Thread != req.Thread {
			return nil, fmt.Errorf("entry %s belongs to another thread", e.ID)
		}
		if !st.ActsFor(e.Author, principal) && !(e.Lane == protolog.LaneControl && e.Author == principal) {
			return nil, fmt.Errorf("entry %s: author is not the authenticated principal", e.ID)
		}
		switch e.Lane {
		case protolog.LaneControl:
			selfBinding := e.Author == principal && e.OnBehalfOf == "" &&
				(e.Kind == protolog.KindDelegation || e.Kind == protolog.KindAccessRequest)
			if !st.Stewards[e.Author] && !selfBinding {
				return nil, fmt.Errorf("entry %s: control lane requires stewardship", e.ID)
			}
			control = append(control, e)
		case protolog.LaneSummary, protolog.LaneContent:
			rest = append(rest, e)
		default:
			return nil, fmt.Errorf("entry %s: unknown lane", e.ID)
		}
	}
	if len(control) > 0 {
		if err := s.Store.AppendEntries(ctx, control); err != nil {
			return nil, err
		}
		if tl, err = s.Store.Thread(ctx, req.Thread); err != nil {
			return nil, err
		}
		st = perm.Fold(req.Thread, tl.Entries())
	}
	for _, e := range rest {
		if !st.ActsFor(e.Author, principal) {
			return nil, fmt.Errorf("entry %s: author is not the authenticated principal", e.ID)
		}
		if !st.MayContribute(e.Author, e.Lamport) {
			return nil, fmt.Errorf("entry %s: author lacks contribute on this thread", e.ID)
		}
	}
	if err := s.Store.AppendEntries(ctx, rest); err != nil {
		return nil, err
	}
	return &PushResponse{Accepted: total}, nil
}

// DiscoverableThread is what a hidden-by-default node advertises: existence
// and title, never contents.
type DiscoverableThread struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Discover lists threads that are either already visible to the principal
// or explicitly marked discoverable.
func (s *Server) Discover(ctx context.Context, principal string) ([]DiscoverableThread, error) {
	ids, err := s.Store.Threads(ctx)
	if err != nil {
		return nil, err
	}
	var out []DiscoverableThread
	for _, id := range ids {
		tl, err := s.Store.Thread(ctx, id)
		if err != nil {
			return nil, err
		}
		st := perm.Fold(id, tl.Entries())
		if st.Discoverable || len(st.Lanes(principal, s.now())) > 0 {
			out = append(out, DiscoverableThread{ID: id, Title: st.Title})
		}
	}
	return out, nil
}

// SubmitAccessRequest admits a signed core.access_request from a principal
// with no other rights on the thread — the one control-lane write an
// outsider may make. Everything else about the entry is validated as usual.
func (s *Server) SubmitAccessRequest(ctx context.Context, principal string, e *protolog.Entry) error {
	tl, err := s.Store.Thread(ctx, e.Thread)
	if err != nil {
		return fmt.Errorf("thread not found")
	}
	st := perm.Fold(e.Thread, tl.Entries())
	if !st.Discoverable && len(st.Lanes(principal, s.now())) == 0 {
		return fmt.Errorf("thread not found") // hidden threads give no oracle
	}
	if e.Kind != protolog.KindAccessRequest || e.Lane != protolog.LaneControl {
		return fmt.Errorf("only core.access_request may be submitted")
	}
	if !st.ActsFor(e.Author, principal) {
		return fmt.Errorf("author is not the authenticated principal")
	}
	return s.Store.AppendEntries(ctx, []*protolog.Entry{e})
}

// BuildAccessRequest constructs the signed core.access_request entry an
// outsider submits: first entry of their own control chain in a thread they
// don't hold. Lamport 1 is fine — answering grants always sort later.
func BuildAccessRequest(priv ed25519.PrivateKey, threadID string, scopes []string, reason string, now func() time.Time) (*protolog.Entry, error) {
	if now == nil {
		now = time.Now
	}
	pid, err := protolog.PrincipalID(priv.Public().(ed25519.PublicKey))
	if err != nil {
		return nil, err
	}
	t := now().UTC()
	body, _ := json.Marshal(map[string]any{"scopes": scopes, "reason": reason})
	e := &protolog.Entry{
		V:       protolog.EnvelopeVersion,
		ID:      ulid.MustNew(ulid.Timestamp(t), ulid.DefaultEntropy()).String(),
		Thread:  threadID,
		Kind:    protolog.KindAccessRequest,
		Lane:    protolog.LaneControl,
		Author:  pid,
		Seq:     1,
		Lamport: 1,
		Prev:    protolog.GenesisPrev,
		TS:      t.Format(time.RFC3339),
		Body:    body,
	}
	if err := e.Sign(priv); err != nil {
		return nil, err
	}
	return e, nil
}

// Client syncs threads with one remote peer: pull everything permitted,
// then push back chains authored by us that the peer is missing.
type Client struct {
	Store store.Store
	Peer  Transport
	// Self is the local principal; chains authored by Self (or listed in
	// SelfKeys) are offered back to the peer after each pull.
	Self     string
	SelfKeys map[string]bool // additional local authors (delegated agent keys)
}

func (c *Client) actsAsSelf(author string) bool {
	return author == c.Self || c.SelfKeys[author]
}

// SyncAll lists remote-visible threads and pulls each. Returns per-thread
// entry counts. Unknown threads bootstrap from their control lane.
func (c *Client) SyncAll(ctx context.Context) (map[string]int, error) {
	ids, err := c.Peer.List(ctx)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, id := range ids {
		n, err := c.SyncThread(ctx, id)
		if err != nil {
			return counts, fmt.Errorf("thread %s: %w", id, err)
		}
		counts[id] = n
	}
	return counts, nil
}

// SyncThread pulls one thread, then pushes back our own missing chains.
// Ingestion is defended in depth: the store re-validates every signature
// and chain position, and non-control entries are admitted only if their
// author holds contribute in the thread's fold — a compromised peer cannot
// hand us entries its members were never authorized to write.
func (c *Client) SyncThread(ctx context.Context, threadID string) (int, error) {
	var have []Head
	if tl, err := c.Store.Thread(ctx, threadID); err == nil {
		for key, seq := range tl.Heads() {
			have = append(have, Head{Author: key.Author, Lane: key.Lane, Seq: seq})
		}
	} // unknown thread: empty vector bootstraps from genesis
	resp, err := c.Peer.Pull(ctx, PullRequest{Thread: threadID, Have: have})
	if err != nil {
		return 0, err
	}
	n := 0
	if len(resp.Entries) > 0 {
		ordered := orderForAppend(resp.Entries, threadID)
		// Control entries land first (orderForAppend), so the fold below
		// already reflects any grants that arrived in this same pull.
		var control, rest []*protolog.Entry
		for _, e := range ordered {
			if e.Lane == protolog.LaneControl {
				control = append(control, e)
			} else {
				rest = append(rest, e)
			}
		}
		if len(control) > 0 {
			if err := c.Store.AppendEntries(ctx, control); err != nil {
				return 0, err
			}
		}
		if len(rest) > 0 {
			tl, err := c.Store.Thread(ctx, threadID)
			if err != nil {
				return 0, err
			}
			st := perm.Fold(threadID, tl.Entries())
			for _, e := range rest {
				if !st.MayContribute(e.Author, e.Lamport) {
					return 0, fmt.Errorf("entry %s: author lacks contribute; refusing peer's unauthorized data", e.ID)
				}
			}
			if err := c.Store.AppendEntries(ctx, rest); err != nil {
				return 0, err
			}
		}
		n = len(ordered)
	}
	pushed, err := c.pushBack(ctx, threadID, resp.Heads)
	if err != nil {
		return n, err
	}
	return n + pushed, nil
}

// ShareThread seeds one of our threads onto a peer — the "host this on the
// hub" verb. It pushes every chain we authored, genesis first; afterwards a
// normal sync keeps it current in both directions.
func (c *Client) ShareThread(ctx context.Context, threadID string) (int, error) {
	tl, err := c.Store.Thread(ctx, threadID)
	if err != nil {
		return 0, err
	}
	var peerHeads []Head
	if resp, err := c.Peer.Pull(ctx, PullRequest{Thread: threadID}); err == nil {
		peerHeads = resp.Heads
	} // a 404 just means the peer doesn't hold it yet
	peerSeq := map[protolog.ChainKey]uint64{}
	for _, h := range peerHeads {
		peerSeq[protolog.ChainKey{Thread: threadID, Author: h.Author, Lane: h.Lane}] = h.Seq
	}
	var offer []*protolog.Entry
	for key, seq := range tl.Heads() {
		if !c.actsAsSelf(key.Author) {
			continue
		}
		if seq > peerSeq[key] {
			offer = append(offer, tl.After(key, peerSeq[key])...)
		}
	}
	if len(offer) == 0 {
		return 0, nil
	}
	resp, err := c.Peer.Push(ctx, PushRequest{Thread: threadID, Entries: offer})
	if err != nil {
		return 0, err
	}
	return resp.Accepted, nil
}

// pushBack offers the peer whatever our own summary/content chains hold
// beyond the peer's heads. Only self-authored chains are pushed: everyone
// syncs their own writing; everything else arrives by pull.
func (c *Client) pushBack(ctx context.Context, threadID string, peerHeads []Head) (int, error) {
	if c.Self == "" {
		return 0, nil // pull-only client
	}
	tl, err := c.Store.Thread(ctx, threadID)
	if err != nil {
		return 0, nil // nothing local to push
	}
	peerSeq := map[protolog.ChainKey]uint64{}
	for _, h := range peerHeads {
		peerSeq[protolog.ChainKey{Thread: threadID, Author: h.Author, Lane: h.Lane}] = h.Seq
	}
	st := perm.Fold(threadID, tl.Entries())
	var offer []*protolog.Entry
	for key, seq := range tl.Heads() {
		if !c.actsAsSelf(key.Author) {
			continue
		}
		if seq <= peerSeq[key] {
			continue
		}
		// Our control chain travels too when we steward the thread — that's
		// how grants and revocations reach a hub without it asking. When we
		// do not, only our own delegations and access requests go, so a peer
		// can verify our agent's writes.
		if key.Lane == protolog.LaneControl && !st.Stewards[key.Author] {
			for _, e := range tl.After(key, peerSeq[key]) {
				if e.Kind == protolog.KindDelegation || e.Kind == protolog.KindAccessRequest {
					offer = append(offer, e)
				}
			}
			continue
		}
		offer = append(offer, tl.After(key, peerSeq[key])...)
	}
	if len(offer) == 0 {
		return 0, nil
	}
	resp, err := c.Peer.Push(ctx, PushRequest{Thread: threadID, Entries: offer})
	if err != nil {
		return 0, fmt.Errorf("push: %w", err)
	}
	return resp.Accepted, nil
}

// orderForAppend sorts pulled entries so appends satisfy chain rules:
// genesis first, then per-chain ascending seq, control lanes before others
// (grants must be verifiable before content that relies on them).
func orderForAppend(entries []*protolog.Entry, threadID string) []*protolog.Entry {
	out := append([]*protolog.Entry(nil), entries...)
	laneRank := map[protolog.Lane]int{protolog.LaneControl: 0, protolog.LaneSummary: 1, protolog.LaneContent: 2}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && lessAppend(out[j], out[j-1], threadID, laneRank); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func lessAppend(a, b *protolog.Entry, threadID string, laneRank map[protolog.Lane]int) bool {
	if (a.ID == threadID) != (b.ID == threadID) {
		return a.ID == threadID // genesis first
	}
	if laneRank[a.Lane] != laneRank[b.Lane] {
		return laneRank[a.Lane] < laneRank[b.Lane]
	}
	if a.Author != b.Author {
		return a.Author < b.Author
	}
	return a.Seq < b.Seq
}
