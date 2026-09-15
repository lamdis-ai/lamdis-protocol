package sync

import (
	"context"
	"crypto/ed25519"
	"path/filepath"
	"testing"
	"time"

	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/perm"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/store"
)

// Two nodes, no HTTP: the transport calls the other node's Server directly
// as a fixed principal, which is exactly what the HTTP layer does after
// verifying a signature.
type local struct {
	s         *Server
	principal string
}

func (l local) List(ctx context.Context) ([]string, error) { return l.s.List(ctx, l.principal) }
func (l local) Pull(ctx context.Context, req PullRequest) (*PullResponse, error) {
	return l.s.Pull(ctx, l.principal, req)
}
func (l local) Push(ctx context.Context, req PushRequest) (*PushResponse, error) {
	return l.s.Push(ctx, l.principal, req)
}

type node struct {
	st   store.Store
	key  ed25519.PrivateKey
	pid  string
	akey ed25519.PrivateKey // delegated agent key
	apid string
}

func newNode(t *testing.T) *node {
	t.Helper()
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "lamdis.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	_, priv, _ := protolog.GenerateKeypair()
	pid, _ := protolog.PrincipalID(priv.Public().(ed25519.PublicKey))
	_, apriv, _ := protolog.GenerateKeypair()
	apid, _ := protolog.PrincipalID(apriv.Public().(ed25519.PublicKey))
	return &node{st: st, key: priv, pid: pid, akey: apriv, apid: apid}
}

func (n *node) append(t *testing.T, thread string, key ed25519.PrivateKey, d protolog.Draft) *protolog.Entry {
	t.Helper()
	tl, err := n.st.Thread(context.Background(), thread)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := protolog.NewAuthor(tl, key)
	e, err := a.Append(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := n.st.AppendEntries(context.Background(), []*protolog.Entry{e}); err != nil {
		t.Fatal(err)
	}
	return e
}

func (n *node) kinds(t *testing.T, thread string) map[string]int {
	t.Helper()
	tl, err := n.st.Thread(context.Background(), thread)
	if err != nil {
		return nil
	}
	out := map[string]int{}
	for _, e := range tl.Entries() {
		out[e.Kind]++
	}
	return out
}

// A collaborator's agent, delegated by the collaborator, must be able to
// write into a thread the collaborator does not steward, and the steward's
// node must accept those writes on push because the delegation travels.
func TestDelegatedAgentWritesReachTheSteward(t *testing.T) {
	ctx := context.Background()
	a, b := newNode(t), newNode(t)

	// A creates a thread and grants B contribute+read.
	_, genesis, err := protolog.NewThreadWith(a.key, "Deal", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.st.AppendEntries(ctx, []*protolog.Entry{genesis}); err != nil {
		t.Fatal(err)
	}
	thread := genesis.ID
	a.append(t, thread, a.key, protolog.Draft{Kind: protolog.KindGrant, Lane: protolog.LaneControl,
		Body: map[string]any{"principal": b.pid, "scopes": []string{"contribute", "read"}}})
	a.append(t, thread, a.key, protolog.Draft{Kind: protolog.KindMessage, Lane: protolog.LaneContent,
		Body: map[string]any{"text": "Acme wants 7%."}})

	// B pulls the thread.
	bToA := &Client{Store: b.st, Peer: local{&Server{Store: a.st}, b.pid}, Self: b.pid, SelfKeys: map[string]bool{b.apid: true}}
	if _, err := bToA.SyncAll(ctx); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if b.kinds(t, thread)[protolog.KindMessage] != 1 {
		t.Fatal("B did not receive A's message")
	}

	// B delegates its agent (control lane, B is not a steward) and the agent
	// writes on B's behalf.
	b.append(t, thread, b.key, protolog.Draft{Kind: protolog.KindDelegation, Lane: protolog.LaneControl,
		Body: map[string]any{"agent": b.apid}})
	b.append(t, thread, b.akey, protolog.Draft{Kind: "agent.note", Lane: protolog.LaneContent, OnBehalfOf: b.pid,
		Body: map[string]any{"text": "7% contradicts the 5% ceiling in our facts thread.", "chain": 1}})

	// Sync again: pushBack must offer the delegation and the note; A must
	// accept both.
	if _, err := bToA.SyncAll(ctx); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	got := a.kinds(t, thread)
	if got[protolog.KindDelegation] != 1 {
		t.Fatalf("A did not receive B's delegation: %v", got)
	}
	if got["agent.note"] != 1 {
		t.Fatalf("A did not accept the agent's note: %v", got)
	}
	tl, _ := a.st.Thread(ctx, thread)
	st := perm.Fold(thread, tl.Entries())
	if !st.ActsFor(b.apid, b.pid) {
		t.Fatal("A's fold does not recognise B's agent")
	}
	for _, e := range tl.Entries() {
		if e.Kind == "agent.note" && (e.Author != b.apid || e.OnBehalfOf != b.pid) {
			t.Fatalf("note not attributed to the agent for B: %+v", e)
		}
	}
}

// A non-steward's grant must still be refused on push, delegation or not.
func TestNonStewardGrantIsRefusedOnPush(t *testing.T) {
	ctx := context.Background()
	a, b := newNode(t), newNode(t)
	_, genesis, _ := protolog.NewThreadWith(a.key, "Deal", false, nil)
	a.st.AppendEntries(ctx, []*protolog.Entry{genesis})
	thread := genesis.ID
	a.append(t, thread, a.key, protolog.Draft{Kind: protolog.KindGrant, Lane: protolog.LaneControl,
		Body: map[string]any{"principal": b.pid, "scopes": []string{"contribute", "read"}}})

	bToA := &Client{Store: b.st, Peer: local{&Server{Store: a.st}, b.pid}, Self: b.pid}
	if _, err := bToA.SyncAll(ctx); err != nil {
		t.Fatal(err)
	}
	// B tries to grant a stranger.
	_, stranger, _ := protolog.GenerateKeypair()
	spid, _ := protolog.PrincipalID(stranger.Public().(ed25519.PublicKey))
	b.append(t, thread, b.key, protolog.Draft{Kind: protolog.KindGrant, Lane: protolog.LaneControl,
		Body: map[string]any{"principal": spid, "scopes": []string{"read"}}})
	if _, err := bToA.SyncAll(ctx); err != nil {
		t.Fatalf("sync should not fail outright: %v", err)
	}
	if a.kinds(t, thread)[protolog.KindGrant] != 1 {
		t.Fatal("A accepted a grant from a non-steward")
	}
	tl, _ := a.st.Thread(ctx, thread)
	if len(perm.Fold(thread, tl.Entries()).EffectiveScopes(spid, time.Now())) != 0 {
		t.Fatal("stranger gained access")
	}
}
