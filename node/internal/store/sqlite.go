package store

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/ncruces/go-sqlite3/driver"
	"github.com/ncruces/go-sqlite3/ext/fts5"

	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
)

const rrfK = 60 // standard reciprocal-rank-fusion constant

// SQLite is the embedded default driver: one file, FTS5, no cgo (WASM
// build). Thread logs are cached in memory but never trusted blindly: a
// serve daemon and CLI commands may write the same database from different
// processes, so every cache read revalidates against the thread's max rowid
// and rehydrates when another writer has appended.
type SQLite struct {
	db *sql.DB

	mu   sync.Mutex
	logs map[string]*cachedLog
	dim  int // vector dimension; 0 until first vector arrives
}

type cachedLog struct {
	log    *protolog.ThreadLog
	maxRid int64
}

func OpenSQLite(path string) (*SQLite, error) {
	return openSQLite(path, "WAL", 5000)
}

// OpenSQLiteShared opens a database that more than one machine may open at
// once, such as a hosted account on a network filesystem while a deploy has
// the old and new servers both running. WAL mode relies on shared memory,
// which does not cross machines, so two writers can corrupt each other; the
// rollback journal uses file locks, which a network filesystem does enforce.
// Opening an existing WAL database this way folds the log back in and
// switches it.
func OpenSQLiteShared(path string) (*SQLite, error) {
	return openSQLite(path, "DELETE", 20000)
}

func openSQLite(path, journal string, busyMS int) (*SQLite, error) {
	// The journal mode is set after opening rather than in the address: if
	// another process still has the file open, switching fails, and that
	// must not stop the database opening (nor leave a half-open handle that
	// then locks every later attempt). It switches on a later open instead.
	db, err := driver.Open(
		"file:"+path+"?_pragma=busy_timeout("+strconv.Itoa(busyMS)+")&_pragma=foreign_keys(1)",
		fts5.Register)
	if err != nil {
		return nil, err
	}
	var mode string
	if err := db.QueryRow("PRAGMA journal_mode=" + journal).Scan(&mode); err != nil {
		log.Printf("store: %s: could not set journal mode %s yet: %v", path, journal, err)
	} else if !strings.EqualFold(mode, journal) {
		log.Printf("store: %s: journal mode is %s, wanted %s; it switches when nothing else has it open", path, mode, journal)
	}
	s := &SQLite{db: db, logs: map[string]*cachedLog{}}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLite) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS entries (
  rid     INTEGER PRIMARY KEY,
  id      TEXT NOT NULL UNIQUE,
  thread  TEXT NOT NULL,
  kind    TEXT NOT NULL,
  lane    TEXT NOT NULL,
  author  TEXT NOT NULL,
  seq     INTEGER NOT NULL,
  lamport INTEGER NOT NULL,
  hash    TEXT NOT NULL,
  text    TEXT,
  raw     BLOB NOT NULL,
  UNIQUE(thread, author, lane, seq)
);
CREATE INDEX IF NOT EXISTS idx_entries_order ON entries(thread, lamport, author, id);
CREATE INDEX IF NOT EXISTS idx_entries_chain ON entries(thread, author, lane, seq);
CREATE VIRTUAL TABLE IF NOT EXISTS entries_fts USING fts5(text, content='entries', content_rowid='rid', tokenize='porter unicode61');
CREATE TABLE IF NOT EXISTS meta (k TEXT PRIMARY KEY, v TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS vectors (rid INTEGER PRIMARY KEY REFERENCES entries(rid), vec BLOB NOT NULL);
`)
	if err != nil {
		return err
	}
	var dim string
	if err := s.db.QueryRow(`SELECT v FROM meta WHERE k = 'vec_dim'`).Scan(&dim); err == nil {
		fmt.Sscanf(dim, "%d", &s.dim)
	}
	return nil
}

func (s *SQLite) Close() error { return s.db.Close() }

// DeleteThread drops a thread from this node. The FTS index is external
// content, so it is rebuilt rather than patched row by row.
func (s *SQLite) DeleteThread(ctx context.Context, threadID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for _, q := range []string{
		`DELETE FROM vectors WHERE rid IN (SELECT rid FROM entries WHERE thread = ?)`,
		`DELETE FROM entries WHERE thread = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, threadID); err != nil {
			tx.Rollback()
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO entries_fts(entries_fts) VALUES('rebuild')`); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	delete(s.logs, threadID)
	return nil
}

// threadLog returns the in-memory log for a thread, rehydrating from the
// database whenever another process (or connection) has appended since the
// cache was built. The log package owns every append invariant; the store
// never re-implements them.
func (s *SQLite) threadLog(ctx context.Context, threadID string) (*protolog.ThreadLog, error) {
	var dbMax sql.NullInt64
	if err := s.db.QueryRowContext(ctx,
		`SELECT max(rid) FROM entries WHERE thread = ?`, threadID).Scan(&dbMax); err != nil {
		return nil, err
	}
	if c, ok := s.logs[threadID]; ok && c.maxRid == dbMax.Int64 {
		return c.log, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT raw FROM entries WHERE thread = ? ORDER BY author, lane, seq`, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	l := protolog.NewThreadLog(threadID)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var e protolog.Entry
		if err := json.Unmarshal(raw, &e); err != nil {
			return nil, err
		}
		if err := l.Append(&e); err != nil {
			return nil, fmt.Errorf("stored thread %s is corrupt: %w", threadID, err)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	s.logs[threadID] = &cachedLog{log: l, maxRid: dbMax.Int64}
	return l, nil
}

func (s *SQLite) AppendEntries(ctx context.Context, entries []*protolog.Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range entries {
		l, err := s.threadLog(ctx, e.Thread)
		if err != nil {
			return err
		}
		// Admit to the in-memory log first: validates chain position, or is a
		// no-op if the caller authored into our cached log (same pointer).
		if err := l.Append(e); err != nil {
			return err
		}
		// Persistence is decided by the database, never the cache — a caller
		// appending through a cached ThreadLog must still reach disk here.
		var one int
		switch err := s.db.QueryRowContext(ctx, `SELECT 1 FROM entries WHERE id = ?`, e.ID).Scan(&one); err {
		case nil:
			continue // already persisted (true redelivery)
		case sql.ErrNoRows:
		default:
			return err
		}
		h, err := e.Hash()
		if err != nil {
			return err
		}
		raw, err := json.Marshal(e)
		if err != nil {
			return err
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO entries (id, thread, kind, lane, author, seq, lamport, hash, text, raw)
			 VALUES (?,?,?,?,?,?,?,?,?,?)`,
			e.ID, e.Thread, e.Kind, string(e.Lane), e.Author, e.Seq, e.Lamport, h, indexableText(e), raw)
		if err != nil {
			tx.Rollback()
			return err
		}
		rid, err := res.LastInsertId()
		if err != nil {
			tx.Rollback()
			return err
		}
		if txt := indexableText(e); txt != "" {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO entries_fts (rowid, text) VALUES (?, ?)`, rid, txt); err != nil {
				tx.Rollback()
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		if c, ok := s.logs[e.Thread]; ok && rid > c.maxRid {
			c.maxRid = rid // keep the cache current for our own writes
		}
	}
	return nil
}

// indexableText extracts body.text when it is a non-empty string.
func indexableText(e *protolog.Entry) string {
	var body struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(e.Body, &body); err != nil {
		return ""
	}
	return body.Text
}

func (s *SQLite) Thread(ctx context.Context, threadID string) (*protolog.ThreadLog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.threadLog(ctx, threadID)
	if err != nil {
		return nil, err
	}
	if l.Len() == 0 {
		return nil, fmt.Errorf("thread %s not found", threadID)
	}
	return l, nil
}

func (s *SQLite) Threads(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT thread FROM entries ORDER BY thread`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *SQLite) Entries(ctx context.Context, threadID string, lanes []protolog.Lane) ([]*protolog.Entry, error) {
	if len(lanes) == 0 {
		return nil, fmt.Errorf("entries: at least one lane is required (deny by default)")
	}
	q := `SELECT raw FROM entries WHERE thread = ? AND lane IN (` + placeholders(len(lanes)) + `)
	      ORDER BY lamport, author, id`
	args := []any{threadID}
	for _, l := range lanes {
		args = append(args, string(l))
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*protolog.Entry
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var e protolog.Entry
		if err := json.Unmarshal(raw, &e); err != nil {
			return nil, err
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}

// ensureDim fixes the store's vector dimension on first use. Vectors live as
// float32 BLOBs scanned in Go: at node scale (≤ a few 100k entries) a
// brute-force cosine scan is comfortably under the search budget, keeps the
// binary cgo-free, and hides behind Store so hubs get pgvector ANN instead.
func (s *SQLite) ensureDim(ctx context.Context, dim int) error {
	if s.dim == dim {
		return nil
	}
	if s.dim != 0 {
		return fmt.Errorf("vector dimension %d does not match store dimension %d (reindex required)", dim, s.dim)
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO meta (k, v) VALUES ('vec_dim', ?) ON CONFLICT(k) DO UPDATE SET v = excluded.v`,
		fmt.Sprint(dim)); err != nil {
		return err
	}
	s.dim = dim
	return nil
}

func serializeF32(vec []float32) []byte {
	out := make([]byte, 4*len(vec))
	for i, f := range vec {
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(f))
	}
	return out
}

func (s *SQLite) UpsertVector(ctx context.Context, entryID string, vec []float32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureDim(ctx, len(vec)); err != nil {
		return err
	}
	var rid int64
	if err := s.db.QueryRowContext(ctx, `SELECT rid FROM entries WHERE id = ?`, entryID).Scan(&rid); err != nil {
		return fmt.Errorf("upsert vector: entry %s: %w", entryID, err)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO vectors (rid, vec) VALUES (?, ?) ON CONFLICT(rid) DO UPDATE SET vec = excluded.vec`,
		rid, serializeF32(vec))
	return err
}

func (s *SQLite) PendingEmbeds(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id FROM entries
		WHERE text IS NOT NULL AND text != '' AND rid NOT IN (SELECT rid FROM vectors)
		ORDER BY rid LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *SQLite) EntryText(ctx context.Context, entryID string) (string, error) {
	var txt sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT text FROM entries WHERE id = ?`, entryID).Scan(&txt)
	if err != nil {
		return "", err
	}
	return txt.String, nil
}

// Search runs FTS and/or vector retrieval, lane-filtered inside each query
// (never post-filtered), and merges with reciprocal rank fusion.
func (s *SQLite) Search(ctx context.Context, req SearchRequest) ([]Hit, error) {
	if len(req.Lanes) == 0 {
		return nil, fmt.Errorf("search: at least one lane is required (deny by default)")
	}
	k := req.K
	if k <= 0 {
		k = 20
	}
	mode := req.Mode
	if mode == "" {
		mode = ModeHybrid
	}
	type ranked struct {
		hit Hit
		rrf float64
	}
	merged := map[string]*ranked{}
	addList := func(hits []Hit) {
		for i, h := range hits {
			r, ok := merged[h.EntryID]
			if !ok {
				r = &ranked{hit: h}
				merged[h.EntryID] = r
			}
			r.rrf += 1.0 / float64(rrfK+i+1)
			if r.hit.Snippet == "" {
				r.hit.Snippet = h.Snippet
			}
		}
	}

	if mode == ModeFTS || mode == ModeHybrid || (mode == ModeVector && req.QueryVec == nil) {
		hits, err := s.searchFTS(ctx, req, k)
		if err != nil {
			return nil, err
		}
		addList(hits)
	}
	if (mode == ModeVector || mode == ModeHybrid) && req.QueryVec != nil && s.dim > 0 {
		hits, err := s.searchVec(ctx, req, k)
		if err != nil {
			return nil, err
		}
		addList(hits)
	}

	out := make([]Hit, 0, len(merged))
	for _, r := range merged {
		r.hit.Rank = r.rrf
		out = append(out, r.hit)
	}
	sortHits(out)
	if len(out) > k {
		out = out[:k]
	}
	return out, nil
}

func sortHits(hits []Hit) {
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && (hits[j].Rank > hits[j-1].Rank ||
			(hits[j].Rank == hits[j-1].Rank && hits[j].EntryID < hits[j-1].EntryID)); j-- {
			hits[j], hits[j-1] = hits[j-1], hits[j]
		}
	}
}

func (s *SQLite) searchFTS(ctx context.Context, req SearchRequest, k int) ([]Hit, error) {
	where, args := scopeFilter(req)
	q := `SELECT e.id, e.thread, e.kind, e.lane,
	             snippet(entries_fts, 0, '', '', '…', 12)
	      FROM entries_fts f JOIN entries e ON e.rid = f.rowid
	      WHERE entries_fts MATCH ?` + where + ` ORDER BY f.rank LIMIT ?`
	args = append([]any{ftsQuery(req.Query)}, args...)
	args = append(args, k)
	return s.scanHits(ctx, q, args)
}

func (s *SQLite) searchVec(ctx context.Context, req SearchRequest, k int) ([]Hit, error) {
	if len(req.QueryVec) != s.dim {
		return nil, fmt.Errorf("query vector dimension %d does not match store dimension %d", len(req.QueryVec), s.dim)
	}
	// Scope filtering happens in SQL (candidates never include out-of-scope
	// rows); scoring is a cosine scan in Go over the candidate blobs.
	where, args := scopeFilter(req)
	q := `SELECT e.id, e.thread, e.kind, e.lane, substr(coalesce(e.text,''), 1, 160), v.vec
	      FROM vectors v JOIN entries e ON e.rid = v.rid WHERE 1=1` + where
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type scored struct {
		hit Hit
		sim float64
	}
	var cands []scored
	for rows.Next() {
		var h Hit
		var lane string
		var blob []byte
		if err := rows.Scan(&h.EntryID, &h.Thread, &h.Kind, &lane, &h.Snippet, &blob); err != nil {
			return nil, err
		}
		h.Lane = protolog.Lane(lane)
		cands = append(cands, scored{hit: h, sim: cosineF32(req.QueryVec, blob)})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].sim != cands[j].sim {
			return cands[i].sim > cands[j].sim
		}
		return cands[i].hit.EntryID < cands[j].hit.EntryID
	})
	if len(cands) > k {
		cands = cands[:k]
	}
	out := make([]Hit, len(cands))
	for i, c := range cands {
		out[i] = c.hit
	}
	return out, nil
}

// cosineF32 computes cosine similarity between a query vector and a stored
// little-endian float32 blob without allocating a decoded slice.
func cosineF32(q []float32, blob []byte) float64 {
	if len(blob) != 4*len(q) {
		return -1
	}
	var dot, nq, nb float64
	for i := range q {
		b := math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4:]))
		dot += float64(q[i]) * float64(b)
		nq += float64(q[i]) * float64(q[i])
		nb += float64(b) * float64(b)
	}
	if nq == 0 || nb == 0 {
		return -1
	}
	return dot / (math.Sqrt(nq) * math.Sqrt(nb))
}

func (s *SQLite) scanHits(ctx context.Context, q string, args []any) ([]Hit, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Hit
	for rows.Next() {
		var h Hit
		var lane string
		if err := rows.Scan(&h.EntryID, &h.Thread, &h.Kind, &lane, &h.Snippet); err != nil {
			return nil, err
		}
		h.Lane = protolog.Lane(lane)
		out = append(out, h)
	}
	return out, rows.Err()
}

// scopeFilter builds the lane/thread/kind WHERE clauses shared by both
// retrieval paths. Lanes are mandatory (checked by Search).
func scopeFilter(req SearchRequest) (string, []any) {
	var sb strings.Builder
	var args []any
	sb.WriteString(` AND e.lane IN (` + placeholders(len(req.Lanes)) + `)`)
	for _, l := range req.Lanes {
		args = append(args, string(l))
	}
	if len(req.Threads) > 0 {
		sb.WriteString(` AND e.thread IN (` + placeholders(len(req.Threads)) + `)`)
		for _, t := range req.Threads {
			args = append(args, t)
		}
	}
	if len(req.Kinds) > 0 {
		sb.WriteString(` AND e.kind IN (` + placeholders(len(req.Kinds)) + `)`)
		for _, kd := range req.Kinds {
			args = append(args, kd)
		}
	}
	return sb.String(), args
}

// ftsQuery quotes each term so user input is never interpreted as FTS5 syntax.
// ftsStopwords are function words that carry no retrieval signal. Deliberately
// short: only words that are almost never the point of a question. "How" and
// "why" are absent because a thread can be about how or why something happened.
var ftsStopwords = map[string]bool{
	"a": true, "an": true, "and": true, "any": true, "are": true, "as": true,
	"at": true, "be": true, "been": true, "but": true, "by": true, "can": true,
	"did": true, "do": true, "does": true, "for": true, "from": true, "get": true,
	"had": true, "has": true, "have": true, "i": true, "if": true, "in": true,
	"into": true, "is": true, "it": true, "its": true, "me": true, "of": true,
	"on": true, "or": true, "our": true, "should": true, "so": true, "than": true,
	"that": true, "the": true, "their": true, "them": true, "then": true,
	"there": true, "these": true, "they": true, "this": true, "to": true,
	"was": true, "we": true, "were": true, "what": true, "when": true,
	"which": true, "who": true, "will": true, "with": true, "would": true,
	"you": true, "your": true,
}

// ftsQuery turns a natural-language question into an FTS5 MATCH expression.
//
// Bare terms separated by spaces are an implicit AND in FTS5, so every word had
// to appear in the same entry. One stray function word sank the whole query: an
// agent asking "does the exchange hold the money" matched nothing, because no
// entry contained "does". Agents ask in sentences rather than in keywords, so
// the query has to survive being a sentence.
//
// Function words are dropped and the rest are OR-ed. Precision does not suffer
// the way it looks like it should, because ranking already favours entries that
// match more of the terms — it comes from bm25 rather than from demanding every
// word be present.
func ftsQuery(q string) string {
	fields := strings.Fields(q)
	quote := func(f string) string {
		return `"` + strings.ReplaceAll(f, `"`, `""`) + `"`
	}
	terms := make([]string, 0, len(fields))
	for _, f := range fields {
		word := strings.Trim(strings.ToLower(f), `.,;:!?'"()[]`)
		if word == "" || ftsStopwords[word] {
			continue
		}
		terms = append(terms, quote(f))
	}
	// A question made entirely of function words still has to mean something,
	// and an empty MATCH is a syntax error rather than an empty result.
	if len(terms) == 0 {
		for _, f := range fields {
			terms = append(terms, quote(f))
		}
	}
	if len(terms) == 0 {
		return `""`
	}
	return strings.Join(terms, " OR ")
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
