package api

// The thing you can do before you install anything.
//
// A landing page that opens with a shell command asks for a decision before
// it has earned one. This endpoint lets somebody type a question on the site
// and get a real answer from a real model, grounded in a small record that is
// already there, so the point of the product lands in about ten seconds.
//
// It is deliberately the most boring possible surface: one model call, no
// tools, no web, no writes that outlive the hour, and three independent
// limits. The one that actually holds is the credit cap on the key itself,
// which never refills; the rest only make it tedious to get there.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/agent"
)

// Try serves the public demo.
type Try struct {
	// Model answers. Nil turns the demo off and says so politely.
	Model     agent.Model
	ModelName string
	// Origin is the site allowed to call this, e.g. https://lamdis.ai.
	Origin string

	PerVisitorPerDay int
	PerVisitorPerMin int
	GlobalPerDay     int
	MaxChars         int

	Now  func() time.Time
	Logf func(string, ...any)

	mu       sync.Mutex
	sessions map[string]*trySession
	visitors map[string]*visitorRate
	day      string
	global   int
}

type tryEntry struct {
	Who   string `json:"who"`
	Text  string `json:"text"`
	Agent bool   `json:"agent"`
	Asked bool   `json:"asked"`
	When  string `json:"when"`
}

type trySession struct {
	entries []tryEntry
	seen    time.Time
	asked   int
}

type visitorRate struct {
	day    int
	minute int
	minAt  time.Time
}

func (t *Try) now() time.Time {
	if t.Now != nil {
		return t.Now()
	}
	return time.Now()
}

func (t *Try) logf(f string, a ...any) {
	if t.Logf != nil {
		t.Logf(f, a...)
	}
}

func (t *Try) defaults() {
	if t.PerVisitorPerDay == 0 {
		t.PerVisitorPerDay = 8
	}
	if t.PerVisitorPerMin == 0 {
		t.PerVisitorPerMin = 3
	}
	if t.GlobalPerDay == 0 {
		t.GlobalPerDay = 400
	}
	if t.MaxChars == 0 {
		t.MaxChars = 400
	}
	if t.sessions == nil {
		t.sessions = map[string]*trySession{}
	}
	if t.visitors == nil {
		t.visitors = map[string]*visitorRate{}
	}
}

// seeded is the record the demo starts from. It is small, ordinary, and
// contains one thing anybody would forget, one number nobody added up, and
// one promise that may no longer hold, because that is what makes a record
// worth keeping rather than a pile of messages.
func seeded(now time.Time) []tryEntry {
	d := func(days int) string { return now.AddDate(0, 0, -days).Format("Jan 2") }
	return []tryEntry{
		{Who: "you", When: d(34), Text: "Quote from Marek Systems to replace the billing platform: £14,200 a year, four weeks to migrate, can start 6 October. Includes the connectors."},
		{Who: "you", When: d(31), Text: "Second quote, Dalton: £11,800 a year. Six weeks though, and they do not do the data migration, so that is somebody else on top."},
		{Who: "you", When: d(29), Text: "Told the board we would be off the old system by mid-November."},
		{Who: "you", When: d(12), Text: "Migration contractor quoted £1,900 for the whole export. Two days once they have the dump."},
		{Who: "you", When: d(3), Text: "Marek emailed: still 6 October, but now wants 40% up front instead of the 25% in the quote."},
	}
}

// visitor is a coarse identity for rate limiting: good enough to make abuse
// tedious, and never trusted as the thing that bounds cost.
func visitorKey(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		// The entry a trusted proxy appended is the rightmost one.
		return strings.TrimSpace(parts[len(parts)-1])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// allow applies the three limits and reports what is left.
func (t *Try) allow(key string) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.defaults()
	now := t.now()
	day := now.UTC().Format("2006-01-02")
	if t.day != day {
		t.day, t.global, t.visitors = day, 0, map[string]*visitorRate{}
	}
	if t.global >= t.GlobalPerDay {
		return 0, fmt.Errorf("the demo has answered as many questions as it can today. Install it and it is yours, with no limit but your own.")
	}
	v := t.visitors[key]
	if v == nil {
		v = &visitorRate{minAt: now}
		t.visitors[key] = v
	}
	if now.Sub(v.minAt) >= time.Minute {
		v.minute, v.minAt = 0, now
	}
	if v.minute >= t.PerVisitorPerMin {
		return t.PerVisitorPerDay - v.day, fmt.Errorf("one moment, that is a lot of questions at once. Try again in a minute.")
	}
	if v.day >= t.PerVisitorPerDay {
		return 0, fmt.Errorf("that is all the demo will answer for one visitor. Install it and ask as much as you like.")
	}
	v.day++
	v.minute++
	t.global++
	return t.PerVisitorPerDay - v.day, nil
}

// session finds or starts one visitor's demo thread, and forgets old ones.
func (t *Try) session(id string) (string, *trySession) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.defaults()
	now := t.now()
	for k, s := range t.sessions {
		if now.Sub(s.seen) > time.Hour {
			delete(t.sessions, k)
		}
	}
	if s := t.sessions[id]; s != nil && id != "" {
		s.seen = now
		return id, s
	}
	if len(t.sessions) > 2000 {
		return "", nil
	}
	buf := make([]byte, 12)
	rand.Read(buf)
	id = hex.EncodeToString(buf)
	s := &trySession{entries: seeded(now), seen: now}
	t.sessions[id] = s
	return id, s
}

const trySystem = `You are somebody's own agent, answering from the record they have kept
over the past few weeks. You are shown all of it.

Rules:
1. Answer only from the record. Do not invent numbers, dates or names.
2. Do the arithmetic when it helps, and say which entries you used and when
   they were written.
3. If two entries disagree, or something has been missed, say so plainly.
   That is the most useful thing you can do.
4. If the answer is not in the record, say so in one sentence.
5. Two to four sentences. Plain text, no markdown, no headings, no bullets.

Write like a capable person who has read everything and is telling a friend.`

// Handler mounts the demo.
func (t *Try) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("OPTIONS /v1/try", t.cors)
	mux.HandleFunc("POST /v1/try", t.handle)
	mux.HandleFunc("GET /v1/try", t.start)
	return mux
}

func (t *Try) cors(w http.ResponseWriter, r *http.Request) {
	t.setCORS(w)
	w.WriteHeader(http.StatusNoContent)
}

func (t *Try) setCORS(w http.ResponseWriter) {
	origin := t.Origin
	if origin == "" {
		origin = "*"
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Headers", "content-type")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Vary", "Origin")
}

// start hands out a fresh thread so the page has something to show.
func (t *Try) start(w http.ResponseWriter, r *http.Request) {
	t.setCORS(w)
	id, s := t.session("")
	if s == nil {
		writeJSON(w, map[string]any{"error": "the demo is busy; try again shortly"})
		return
	}
	writeJSON(w, map[string]any{"session": id, "entries": s.entries,
		"on": t.Model != nil, "model": t.ModelName})
}

func (t *Try) handle(w http.ResponseWriter, r *http.Request) {
	t.setCORS(w)
	var in struct {
		Session string `json:"session"`
		Text    string `json:"text"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&in) != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	q := strings.TrimSpace(in.Text)
	if q == "" {
		http.Error(w, "say something", http.StatusBadRequest)
		return
	}
	if len(q) > t.MaxChars {
		q = q[:t.MaxChars]
	}
	if t.Model == nil {
		writeJSON(w, map[string]any{"error": "The live demo is off right now. Install it and it runs on whichever model you choose."})
		return
	}
	left, err := t.allow(visitorKey(r))
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error(), "left": 0})
		return
	}
	id, s := t.session(in.Session)
	if s == nil {
		writeJSON(w, map[string]any{"error": "the demo is busy; try again shortly"})
		return
	}

	var sb strings.Builder
	sb.WriteString("Their record, oldest first:\n\n")
	t.mu.Lock()
	for _, e := range s.entries {
		if e.Agent {
			sb.WriteString(e.When + " (you answered): " + e.Text + "\n")
		} else if !e.Asked {
			sb.WriteString(e.When + ": " + e.Text + "\n")
		}
	}
	t.mu.Unlock()
	sb.WriteString("\nThey ask: " + q)

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	msg, usage, err := t.Model.Complete(ctx, []agent.Message{
		{Role: "system", Content: trySystem},
		{Role: "user", Content: sb.String()},
	}, nil)
	if err != nil {
		t.logf("try: %v", err)
		writeJSON(w, map[string]any{"error": "The model did not answer just then. Try once more."})
		return
	}
	answer := strings.TrimSpace(msg.Content)

	when := t.now().Format("3:04 PM")
	t.mu.Lock()
	s.asked++
	s.entries = append(s.entries,
		tryEntry{Who: "you", Text: q, Asked: true, When: when},
		tryEntry{Who: "your agent", Text: answer, Agent: true, When: when})
	entries := append([]tryEntry(nil), s.entries...)
	t.mu.Unlock()

	writeJSON(w, map[string]any{"session": id, "answer": answer, "entries": entries,
		"left": left, "model": t.ModelName,
		"tokens": map[string]int{"prompt": usage.Prompt, "completion": usage.Completion}})
}
