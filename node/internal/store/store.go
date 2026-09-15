// Package store persists entries and serves permitted reads and searches.
// Two drivers implement Store: SQLite (embedded, default) and Postgres
// (hubs). The contract test suite in store_test.go runs against every
// driver; protocol invariants themselves live in the log package.
package store

import (
	"context"

	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
)

// SearchMode selects retrieval strategy.
type SearchMode string

const (
	ModeHybrid SearchMode = "hybrid"
	ModeVector SearchMode = "vector"
	ModeFTS    SearchMode = "fts"
)

// SearchRequest is the protocol search verb. Queries are always text —
// embeddings never cross the store boundary inbound.
type SearchRequest struct {
	Query   string
	Threads []string        // empty = all visible threads
	Kinds   []string        // empty = all kinds
	Lanes   []protolog.Lane // empty = all lanes the caller may read
	Mode    SearchMode
	K       int // max results; 0 = default 20
	// QueryVec is the embedding of Query, computed by the node's embedder.
	// nil degrades vector/hybrid modes to FTS. Stores never embed.
	QueryVec []float32
}

// Hit is one search result.
type Hit struct {
	EntryID string
	Thread  string
	Kind    string
	Lane    protolog.Lane
	Snippet string
	Rank    float64
}

// Store is the persistence contract shared by all drivers.
type Store interface {
	// AppendEntries validates each entry against its thread's chains (via the
	// log package) and persists it. Idempotent for identical redelivery.
	AppendEntries(ctx context.Context, entries []*protolog.Entry) error
	// Thread loads a full thread log (all lanes) from storage.
	Thread(ctx context.Context, threadID string) (*protolog.ThreadLog, error)
	// Threads lists all thread ids held by this node.
	Threads(ctx context.Context) ([]string, error)
	// Entries returns entries of a thread restricted to the given lanes, in
	// total order. Lane filtering here is the enforcement point for scopes.
	Entries(ctx context.Context, threadID string, lanes []protolog.Lane) ([]*protolog.Entry, error)
	// Search runs FTS/vector/hybrid retrieval over body.text of entries in
	// the given lanes. Lane filtering is enforced in-query, not post-filter.
	Search(ctx context.Context, req SearchRequest) ([]Hit, error)
	// UpsertVector attaches an embedding to an entry (async embed worker).
	UpsertVector(ctx context.Context, entryID string, vec []float32) error
	// PendingEmbeds lists entry ids with indexable text but no vector yet.
	PendingEmbeds(ctx context.Context, limit int) ([]string, error)
	// EntryText returns the indexable text of an entry ("" if none).
	EntryText(ctx context.Context, entryID string) (string, error)
	// DeleteThread removes a thread and everything in it from this node.
	// Copies on other nodes are untouched: the log is append-only between
	// peers, and deletion is a local decision about local storage.
	DeleteThread(ctx context.Context, threadID string) error
	Close() error
}
