package api

// The node's web application.
//
// The portal next door is an approval inbox: it answers "who is asking for
// access, and do I allow it". This is the thing people actually spend time in
// — threads, what is in them, and asking questions of them — because a
// protocol nobody can look at is a protocol nobody adopts.
//
// Two audiences, one page. The owner arrives with the local token and sees
// everything. A counterparty arrives with a share link and sees exactly the
// lanes that link names, which is the whole point of lanes: you can hand
// somebody "migration on track, two blockers" without handing them "vendor
// stuck on clause 7".

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/agent"
	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/perm"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/store"
)

// App serves the node's web interface.
type App struct {
	Store store.Store
	Key   ed25519.PrivateKey
	Self  string
	// Token authenticates the owner. It is the same local bearer the portal
	// uses: whoever can read the node's data directory is the owner, and no
	// peer signature grants access to this surface.
	Token string
	Names func(principal string) string
	// Ask answers a question against thread context. Nil when no model is
	// configured, which is a state the page explains rather than hides.
	Ask func(ctx context.Context, question string, context []string) (string, error)
	// Model is shown in the interface so nobody has to guess what answered.
	Model string
	// DataDir holds peers.json, shares.json and the owner's display name —
	// the same files the CLI reads, so both surfaces agree on who is who.
	DataDir string
	Now     func() time.Time
	// Runner is the built-in agent; Scheduler makes it autonomous. AgentSelf
	// is the agent's own principal, which acts for Self under a delegation.
	Runner    *agent.Runner
	Scheduler *agent.Scheduler
	AgentSelf string
	// Auth, when set, replaces the local-token check entirely: whoever calls
	// has already been identified upstream. The hosted front door uses this
	// after verifying an account token and routing to that person's node.
	Auth func(*http.Request) bool
	// SharePrefix is where share links live. "/s" when you run your own
	// node; hosted nodes add the account so one address serves many people.
	SharePrefix string
	// ExposeApp lets the interface answer requests from other machines.
	// Off by default: exposing the node so peers can sync must not also
	// put your own threads on the network behind a single bearer token.
	ExposeApp bool
	// NoCommands refuses tool servers that run a local command. A hosted
	// node sets it: running a stranger's command is not a connection.
	NoCommands bool

	agentRevoked bool
}

// fromThisMachine reports whether a request came over the loopback
// interface. A token is a secret; the loopback check is the second lock.
func fromThisMachine(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (a *App) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

// sharePrefix is where this node's share links live.
func (a *App) sharePrefix() string {
	if a.SharePrefix != "" {
		return strings.TrimRight(a.SharePrefix, "/")
	}
	return "/s"
}

func (a *App) name(principal string) string {
	if a.Names != nil {
		if n := a.Names(principal); n != "" {
			return n
		}
	}
	if len(principal) > 20 {
		return principal[:20] + "…"
	}
	return principal
}

// Register mounts the application.
func (a *App) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /app", a.page)
	mux.HandleFunc("GET /app/", a.page)
	mux.HandleFunc("GET /app/api/threads", a.owner(a.handleThreads))
	mux.HandleFunc("GET /app/api/thread/{id}", a.owner(a.handleThread))
	mux.HandleFunc("POST /app/api/post", a.owner(a.handlePost))
	mux.HandleFunc("POST /app/api/chat", a.owner(a.handleChat))
	mux.HandleFunc("GET /app/api/thread/{id}/brief", a.owner(a.handleBriefGet))
	mux.HandleFunc("POST /app/api/thread/{id}/brief", a.owner(a.handleBriefSet))
	mux.HandleFunc("POST /app/api/thread/{id}/run", a.owner(a.handleRunNow))
	mux.HandleFunc("POST /app/api/decision", a.owner(a.handleDecision))
	mux.HandleFunc("GET /app/api/agent", a.owner(a.handleAgent))
	mux.HandleFunc("POST /app/api/agent/revoke", a.owner(a.handleAgentRevoke))
	mux.HandleFunc("POST /app/api/agent/config", a.owner(a.handleAgentConfig))
	mux.HandleFunc("POST /app/api/thread/{id}/delete", a.owner(a.handleDeleteThread))
	mux.HandleFunc("GET /app/api/models", a.owner(a.handleModels))
	mux.HandleFunc("GET /app/api/thread/{id}/links", a.owner(a.handleLinks))
	mux.HandleFunc("GET /app/api/tools", a.owner(a.handleToolsGet))
	mux.HandleFunc("POST /app/api/tools", a.owner(a.handleToolsSet))
	mux.HandleFunc("POST /app/api/tools/probe", a.owner(a.handleToolsProbe))
	mux.HandleFunc("POST /app/api/tools/remove", a.owner(a.handleToolsRemove))
	mux.HandleFunc("POST /app/api/summarize", a.owner(a.handleSummarize))
	mux.HandleFunc("POST /app/api/share", a.owner(a.handleShare))
	mux.HandleFunc("GET /app/api/me", a.owner(a.handleMe))
	mux.HandleFunc("POST /app/api/me", a.owner(a.handleSetName))
	mux.HandleFunc("POST /app/api/threads", a.owner(a.handleCreateThread))
	mux.HandleFunc("GET /app/api/thread/{id}/access", a.owner(a.handleAccess))
	mux.HandleFunc("POST /app/api/thread/{id}/grant", a.owner(a.handleGrant))
	mux.HandleFunc("POST /app/api/thread/{id}/revoke", a.owner(a.handleRevoke))
	mux.HandleFunc("POST /app/api/thread/{id}/decide", a.owner(a.handleDecide))
	mux.HandleFunc("POST /app/api/share/revoke", a.owner(a.handleRevokeLink))
	mux.HandleFunc("POST /app/api/peers", a.owner(a.handleAddPeer))
	// The shared view. Deliberately a different path with its own auth: a
	// capability must never be able to reach an owner route by accident.
	mux.HandleFunc("GET /s/{cap}", a.sharedPage)
	mux.HandleFunc("GET /s/{cap}/api/thread", a.sharedThread)
}

// owner gates a route on the local token.
//
// The token may arrive as a bearer, as ?token= on the first visit, or as the
// cookie that first visit sets. Compared in constant time: a token that leaks
// through a timing difference is not a token.
func (a *App) owner(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.Auth != nil {
			if !a.Auth(r) {
				http.Error(w, "not authorised", http.StatusUnauthorized)
				return
			}
			next(w, r)
			return
		}
		if !a.ExposeApp && !fromThisMachine(r) {
			http.Error(w, "this interface answers only on this machine", http.StatusForbidden)
			return
		}
		if a.Token == "" || !a.authed(r) {
			http.Error(w, "not authorised", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (a *App) authed(r *http.Request) bool {
	got := ""
	if v := r.Header.Get("Authorization"); strings.HasPrefix(v, "Bearer ") {
		got = strings.TrimPrefix(v, "Bearer ")
	} else if v := r.URL.Query().Get("token"); v != "" {
		got = v
	} else if c, err := r.Cookie("lamdis_token"); err == nil {
		got = c.Value
	}
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(a.Token)) == 1
}

func (a *App) page(w http.ResponseWriter, r *http.Request) {
	if !a.ExposeApp && !fromThisMachine(r) {
		http.Error(w, "this interface answers only on this machine", http.StatusForbidden)
		return
	}
	if !a.authed(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, appNoToken)
		return
	}
	// Move the token out of the URL so it stops being pasted into chat
	// windows and shoulder-surfed.
	if t := r.URL.Query().Get("token"); t != "" {
		http.SetCookie(w, &http.Cookie{
			Name: "lamdis_token", Value: t, Path: "/",
			HttpOnly: true, SameSite: http.SameSiteLaxMode,
		})
		http.Redirect(w, r, "/app", http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, appHTML(a.Model, a.Ask != nil))
}

type appThread struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Mine    bool   `json:"mine"`
	Entries int    `json:"entries"`
	Last    string `json:"last,omitempty"`
	Shared  int    `json:"shared"`
	Pending int    `json:"pending"`
	// LastShared is when the most recent summary was approved, and SinceShared
	// counts entries written after it. A share link reads the summary lane, so
	// this is exactly "how stale is what they can see".
	LastShared  string `json:"last_shared,omitempty"`
	SinceShared int    `json:"since_shared"`
	EverShared  bool   `json:"ever_shared"`
	// Auto says the agent works here on its own; Waiting counts questions
	// the agent has asked and the person has not answered.
	Auto    bool `json:"auto"`
	Waiting int  `json:"waiting"`
}

func (a *App) handleThreads(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ids, err := a.Store.Threads(ctx)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	out := []appThread{}
	for _, id := range ids {
		tl, err := a.Store.Thread(ctx, id)
		if err != nil {
			continue
		}
		entries := tl.Entries()
		st := perm.Fold(id, entries)
		t := appThread{ID: id, Title: st.Title, Mine: st.Stewards[a.Self], Pending: len(st.PendingRequests())}
		seen := map[string]bool{}
		if b, has := agent.LoadBrief(tl, a.Self); has && (b.OnNewEntry != "off" || b.Every != "") {
			t.Auto = true
		}
		answered := map[string]bool{}
		for _, e := range entries {
			if e.Kind == agent.KindDecisionReply && e.Refs != nil {
				answered[e.Refs.RepliesTo] = true
			}
		}
		for _, e := range entries {
			bookkeeping := e.Kind == agent.KindRun || e.Kind == agent.KindBrief
			if e.Lane != protolog.LaneControl && !bookkeeping {
				t.Entries++
				t.Last = e.TS
			}
			if e.Kind == agent.KindDecision && !answered[e.ID] {
				t.Waiting++
			}
			if e.Lane == protolog.LaneSummary {
				t.LastShared, t.EverShared, t.SinceShared = e.TS, true, 0
			} else if e.Lane == protolog.LaneContent && t.EverShared && !bookkeeping {
				t.SinceShared++
			}
			if e.Kind == protolog.KindGrant {
				var b struct {
					Principal string `json:"principal"`
				}
				if json.Unmarshal(e.Body, &b) == nil && b.Principal != "" && !seen[b.Principal] {
					if len(st.EffectiveScopes(b.Principal, a.now())) > 0 {
						seen[b.Principal] = true
						t.Shared++
					}
				}
			}
		}
		if t.Title == "" {
			t.Title = "(untitled)"
		}
		out = append(out, t)
	}
	writeJSON(w, map[string]any{"self": a.Self, "threads": out})
}

type appEntry struct {
	ID     string `json:"id"`
	Lane   string `json:"lane"`
	Kind   string `json:"kind"`
	Author string `json:"author"`
	Who    string `json:"who"`
	Mine   bool   `json:"mine"`
	TS     string `json:"ts"`
	Text   string `json:"text"`
	Agent  string `json:"agent,omitempty"` // "agent" for the built-in one, else the MCP client name
	// OnBehalf is set when a delegated key wrote this for someone.
	OnBehalf  string          `json:"on_behalf_of,omitempty"`
	RepliesTo string          `json:"replies_to,omitempty"`
	Options   []string        `json:"options,omitempty"`
	Resolved  bool            `json:"resolved,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
}

// entriesFor reads a thread through one principal's eyes.
//
// The owner sees every lane. Anyone else sees only what the fold says they
// may, which is the same computation a peer's node would run — the interface
// does not get a private door.
func (a *App) entriesFor(ctx context.Context, id string, lanes []protolog.Lane) (string, []appEntry, error) {
	tl, err := a.Store.Thread(ctx, id)
	if err != nil {
		return "", nil, err
	}
	st := perm.Fold(id, tl.Entries())
	allow := map[protolog.Lane]bool{}
	for _, l := range lanes {
		allow[l] = true
	}
	answered := map[string]bool{}
	for _, e := range tl.Entries() {
		if e.Kind == agent.KindDecisionReply && e.Refs != nil {
			answered[e.Refs.RepliesTo] = true
		}
	}
	out := []appEntry{}
	for _, e := range tl.Entries() {
		if !allow[e.Lane] {
			continue
		}
		var b struct {
			Text    string   `json:"text"`
			Title   string   `json:"title"`
			Agent   string   `json:"agent"`
			Summary string   `json:"summary"`
			Options []string `json:"options"`
			Choice  string   `json:"choice"`
		}
		json.Unmarshal(e.Body, &b)
		txt := b.Text
		if txt == "" {
			txt = b.Title
		}
		if txt == "" && e.Kind == agent.KindRun {
			txt = b.Summary
		}
		if txt == "" && e.Kind == agent.KindDecisionReply {
			txt = b.Choice
		}
		if txt == "" && e.Lane == protolog.LaneControl {
			txt = "(" + e.Kind + ")"
		}
		ae := appEntry{
			ID: e.ID, Lane: string(e.Lane), Kind: e.Kind, Author: e.Author,
			Who: a.displayName(e.Author), Mine: e.Author == a.Self || e.OnBehalfOf == a.Self,
			TS: e.TS, Text: txt, Agent: b.Agent, OnBehalf: e.OnBehalfOf, Options: b.Options,
		}
		if e.OnBehalfOf != "" {
			ae.Agent = "agent"
			if e.Author != a.AgentSelf {
				// Someone else's agent: name it by the person it acts for.
				ae.Who = a.displayName(e.OnBehalfOf) + "'s agent"
			}
		}
		if e.Lane == protolog.LaneControl {
			ae.Agent = "" // a delegation's "agent" field is a principal, not a label
		}
		if e.Refs != nil {
			ae.RepliesTo = e.Refs.RepliesTo
		}
		switch e.Kind {
		case agent.KindDecision:
			ae.Resolved = answered[e.ID]
		case agent.KindRun, agent.KindBrief, agent.KindDecisionReply:
			ae.Data = e.Body
		}
		out = append(out, ae)
	}
	return st.Title, out, nil
}

func (a *App) handleThread(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	title, entries, err := a.entriesFor(r.Context(), id,
		[]protolog.Lane{protolog.LaneControl, protolog.LaneSummary, protolog.LaneContent})
	if err != nil {
		http.Error(w, "no such thread", http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]any{"id": id, "title": title, "entries": entries})
}

func (a *App) handlePost(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Thread string `json:"thread"`
		Text   string `json:"text"`
		Lane   string `json:"lane"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in) != nil || strings.TrimSpace(in.Text) == "" {
		http.Error(w, "thread and text are required", http.StatusBadRequest)
		return
	}
	lane := protolog.Lane(in.Lane)
	kind := protolog.KindMessage
	switch lane {
	case protolog.LaneSummary:
		kind = protolog.KindSummary
	case protolog.LaneContent, "":
		lane = protolog.LaneContent
	default:
		http.Error(w, "a message goes in the content or summary lane", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	tl, err := a.Store.Thread(ctx, in.Thread)
	if err != nil {
		http.Error(w, "no such thread", http.StatusNotFound)
		return
	}
	author, err := protolog.NewAuthor(tl, a.Key)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	e, err := author.Append(protolog.Draft{Kind: kind, Lane: lane,
		Body: map[string]any{"text": strings.TrimSpace(in.Text)}})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := a.Store.AppendEntries(ctx, []*protolog.Entry{e}); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"id": e.ID, "lane": string(lane)})
}

// --- sharing ------------------------------------------------------------
//
// A share link is a read capability, and it is deliberately NOT a protocol
// grant. A grant names a principal and is folded from the signed control lane;
// a counterparty who has never run a node has no principal to name. So this is
// an application-level bearer: signed by the node key, scoped to one thread and
// one set of lanes, and expiring. It can read and it can never write.
//
// Keeping the two apart matters. If share links wrote grants, the permission
// fold would stop being a statement about principals, and the property that
// makes the protocol checkable by somebody else's node would quietly die.

type shareClaim struct {
	ID     string   `json:"i,omitempty"`
	Thread string   `json:"t"`
	Lanes  []string `json:"l"`
	Exp    int64    `json:"e"`
}

func (a *App) mintShare(c shareClaim) (string, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, a.Key.Seed())
	mac.Write(raw)
	enc := base64.RawURLEncoding
	return enc.EncodeToString(raw) + "." + enc.EncodeToString(mac.Sum(nil)), nil
}

func (a *App) readShare(tok string) (shareClaim, bool) {
	var c shareClaim
	body, sig, ok := strings.Cut(tok, ".")
	if !ok {
		return c, false
	}
	enc := base64.RawURLEncoding
	raw, err := enc.DecodeString(body)
	if err != nil {
		return c, false
	}
	want, err := enc.DecodeString(sig)
	if err != nil {
		return c, false
	}
	mac := hmac.New(sha256.New, a.Key.Seed())
	mac.Write(raw)
	if !hmac.Equal(want, mac.Sum(nil)) {
		return c, false
	}
	if json.Unmarshal(raw, &c) != nil {
		return c, false
	}
	if c.Exp > 0 && a.now().Unix() > c.Exp {
		return c, false
	}
	if c.ID != "" && a.shareRevoked(c.ID) {
		return c, false
	}
	return c, true
}

func (a *App) handleShare(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Thread string `json:"thread"`
		Scope  string `json:"scope"` // "summary" or "read"
		Days   int    `json:"days"`
		Label  string `json:"label"` // who it is for, for the owner's own list
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in) != nil || in.Thread == "" {
		http.Error(w, "thread is required", http.StatusBadRequest)
		return
	}
	lanes := []string{string(protolog.LaneSummary)}
	if in.Scope == "read" {
		lanes = append(lanes, string(protolog.LaneContent))
	}
	days := in.Days
	if days <= 0 {
		days = 30
	}
	now := a.now()
	rec := shareRecord{
		ID: newShareID(), Thread: in.Thread, Lanes: lanes,
		Exp: now.AddDate(0, 0, days).Unix(), Created: now.Unix(),
		Label: strings.TrimSpace(in.Label),
	}
	tok, err := a.mintShare(shareClaim{ID: rec.ID, Thread: rec.Thread, Lanes: rec.Lanes, Exp: rec.Exp})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	// Recorded so "who can see this" can list it and the owner can withdraw
	// it. If the record cannot be written the link still works until expiry;
	// say so rather than fail the whole action.
	shares := append(a.loadShares(), rec)
	saveErr := a.saveShares(shares)
	out := map[string]any{"id": rec.ID, "path": a.sharePrefix() + "/" + tok, "lanes": lanes, "days": days}
	if saveErr != nil {
		out["note"] = "the link works but could not be recorded, so it cannot be revoked early"
	}
	writeJSON(w, out)
}

func newShareID() string {
	var b [9]byte
	rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func (a *App) sharedPage(w http.ResponseWriter, r *http.Request) {
	c, ok := a.readShare(r.PathValue("cap"))
	if !ok {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, appNotice("This link is not valid",
			"It may have expired, or been altered. Ask whoever sent it for a new one."))
		return
	}
	full := false
	for _, l := range c.Lanes {
		if l == string(protolog.LaneContent) {
			full = true
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Referrer-Policy", "no-referrer")
	fmt.Fprint(w, sharedHTML(full))
}

func (a *App) sharedThread(w http.ResponseWriter, r *http.Request) {
	c, ok := a.readShare(r.PathValue("cap"))
	if !ok {
		http.Error(w, "not valid", http.StatusNotFound)
		return
	}
	lanes := make([]protolog.Lane, 0, len(c.Lanes))
	for _, l := range c.Lanes {
		lanes = append(lanes, protolog.Lane(l))
	}
	title, entries, err := a.entriesFor(r.Context(), c.Thread, lanes)
	if err != nil {
		http.Error(w, "no such thread", http.StatusNotFound)
		return
	}
	// A reader of a share link has no business seeing who else holds one, so
	// only the entries themselves cross the boundary.
	writeJSON(w, map[string]any{
		"title": title, "entries": entries,
		"lanes": c.Lanes, "expires": strconv.FormatInt(c.Exp, 10),
	})
}

// handleSummarize drafts what a stranger should be told about a thread.
//
// This is how the shareable layer gets written without asking a person to
// classify every sentence as they type. Everything they write is private; when
// they decide to share, the draft is proposed, they edit it, and only the
// approved text is published. Without a model configured the person writes
// the summary themselves, which is slower but the same shape.
func (a *App) handleSummarize(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Thread string `json:"thread"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in) != nil || in.Thread == "" {
		http.Error(w, "thread is required", http.StatusBadRequest)
		return
	}
	title, entries, err := a.entriesFor(r.Context(), in.Thread,
		[]protolog.Lane{protolog.LaneSummary, protolog.LaneContent})
	if err != nil {
		http.Error(w, "no such thread", http.StatusNotFound)
		return
	}
	if a.Ask == nil {
		writeJSON(w, map[string]any{"draft": "", "model": ""})
		return
	}
	// If something has already been shared, the draft is an update: what has
	// changed since, not the whole story told again. That is what a person
	// would write, and it is what the other side wants to read.
	var lastSummary appEntry
	lastIdx := -1
	for i, e := range entries {
		if e.Lane == string(protolog.LaneSummary) {
			lastSummary, lastIdx = e, i
		}
	}
	lines := make([]string, 0, len(entries))
	for i, e := range entries {
		if strings.TrimSpace(e.Text) == "" || e.Lane == string(protolog.LaneSummary) ||
			e.Kind == agent.KindRun || e.Kind == agent.KindBrief {
			continue
		}
		if lastIdx >= 0 && i < lastIdx {
			continue
		}
		lines = append(lines, e.TS[:10]+" "+e.Who+": "+e.Text)
	}
	guard := "Leave out anything that reads as a private note, a number someone would not want a " +
		"counterparty to know, or a walk-away position. Plain prose, no headings, under 120 words."
	var q string
	mode := "summary"
	if lastIdx >= 0 {
		mode = "update"
		if len(lines) == 0 {
			writeJSON(w, map[string]any{"draft": "", "mode": "current", "model": a.Model,
				"previous": lastSummary.Text})
			return
		}
		q = "The reader already has this earlier update about \"" + title + "\":\n\n" + lastSummary.Text +
			"\n\nWrite the next update for the same reader, covering only what has happened since. " +
			"Say what changed, what was decided, and what is still open. Do not repeat the earlier update. " + guard
	} else {
		q = "Write a short summary of this thread for someone outside it, titled \"" + title + "\". " +
			"State the current situation, decisions made, and what is still open. " + guard
	}
	draft, err := a.Ask(r.Context(), q, lines)
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"draft": draft, "mode": mode, "model": a.Model, "since": len(lines)})
}
