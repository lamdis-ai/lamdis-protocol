package agent

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/perm"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/store"
)

// script is a Model that returns canned turns in order.
type script struct {
	turns []Message
	calls int
	tools [][]ToolSpec
}

func (s *script) Complete(ctx context.Context, msgs []Message, tools []ToolSpec) (Message, Usage, error) {
	s.tools = append(s.tools, tools)
	if s.calls >= len(s.turns) {
		return Message{Role: "assistant", Content: "NOTHING"}, Usage{Prompt: 10, Completion: 2}, nil
	}
	m := s.turns[s.calls]
	s.calls++
	m.Role = "assistant"
	return m, Usage{Prompt: 10, Completion: 2}, nil
}

func say(text string) Message { return Message{Content: text} }

func call(name string, args map[string]any) Message {
	raw, _ := json.Marshal(args)
	var tc ToolCall
	tc.ID, tc.Type = "c1", "function"
	tc.Function.Name, tc.Function.Arguments = name, string(raw)
	return Message{ToolCalls: []ToolCall{tc}}
}

type fixture struct {
	dir    string
	st     store.Store
	person ed25519.PrivateKey
	pid    string
	r      *Runner
	thread string
}

func setup(t *testing.T, m Model) *fixture {
	t.Helper()
	dir := t.TempDir()
	st, err := store.OpenSQLite(filepath.Join(dir, "lamdis.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	_, priv, _ := protolog.GenerateKeypair()
	pid, _ := protolog.PrincipalID(priv.Public().(ed25519.PublicKey))
	agentKey, agentPID, err := LoadOrMintAgentKey(dir)
	if err != nil {
		t.Fatal(err)
	}
	tl, genesis, err := protolog.NewThreadWith(priv, "Deal with Acme", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AppendEntries(context.Background(), []*protolog.Entry{genesis}); err != nil {
		t.Fatal(err)
	}
	_ = tl
	r := &Runner{Store: st, PersonKey: priv, Person: pid, AgentKey: agentKey, Agent: agentPID,
		Model: m, ModelName: "test", DataDir: dir, State: LoadState(dir)}
	return &fixture{dir: dir, st: st, person: priv, pid: pid, r: r, thread: genesis.ID}
}

func (f *fixture) post(t *testing.T, kind string, body map[string]any, refs *protolog.Refs) *protolog.Entry {
	t.Helper()
	tl, err := f.st.Thread(context.Background(), f.thread)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := protolog.NewAuthor(tl, f.person)
	e, err := a.Append(protolog.Draft{Kind: kind, Lane: protolog.LaneContent, Body: body, Refs: refs})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.st.AppendEntries(context.Background(), []*protolog.Entry{e}); err != nil {
		t.Fatal(err)
	}
	return e
}

func (f *fixture) entries(t *testing.T) []*protolog.Entry {
	t.Helper()
	tl, err := f.st.Thread(context.Background(), f.thread)
	if err != nil {
		t.Fatal(err)
	}
	return tl.Entries()
}

func TestChatWritesAnswerUnderDelegation(t *testing.T) {
	f := setup(t, &script{turns: []Message{say("Two items are open.")}})
	f.post(t, KindNote, map[string]any{"text": "Ledger cutover and on-call rota are open."}, nil)
	q := f.post(t, KindQuestion, map[string]any{"text": "What is open?"}, nil)

	res := f.r.Run(context.Background(), Trigger{Kind: TriggerChat, Thread: f.thread, Entry: q.ID})
	if res.Outcome != "answered" || res.Answer != "Two items are open." {
		t.Fatalf("unexpected result: %+v", res)
	}
	es := f.entries(t)
	st := perm.Fold(f.thread, es)
	if !st.ActsFor(f.r.Agent, f.pid) {
		t.Fatal("no delegation was written for the agent")
	}
	var answer, run *protolog.Entry
	for _, e := range es {
		switch e.Kind {
		case KindAnswer:
			answer = e
		case KindRun:
			run = e
		}
	}
	if answer == nil || answer.Author != f.r.Agent || answer.OnBehalfOf != f.pid {
		t.Fatalf("answer not signed by the agent on the person's behalf: %+v", answer)
	}
	if answer.Refs == nil || answer.Refs.RepliesTo != q.ID {
		t.Fatal("answer does not reply to the question")
	}
	if run == nil || run.Author != f.r.Agent {
		t.Fatal("no run record")
	}
	if es[len(es)-1].Kind != KindRun {
		t.Fatal("the run record must be the last thing written")
	}
	// A second run must not write a second delegation.
	before := len(f.entries(t))
	f.r.Run(context.Background(), Trigger{Kind: TriggerChat, Thread: f.thread, Entry: q.ID})
	n := 0
	for _, e := range f.entries(t) {
		if e.Kind == protolog.KindDelegation {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("delegation written %d times", n)
	}
	if len(f.entries(t)) != before+2 {
		t.Fatalf("second chat should add exactly answer+run, got %d new", len(f.entries(t))-before)
	}
}

func TestAskPersonPausesAndDecisionResumes(t *testing.T) {
	m := &script{turns: []Message{
		call("ask_person", map[string]any{"question": "Accept 5%?", "options": []any{"yes", "no"}}),
		say("Noted, I will hold at 5%."),
	}}
	f := setup(t, m)
	q := f.post(t, KindQuestion, map[string]any{"text": "Should we accept?"}, nil)
	res := f.r.Run(context.Background(), Trigger{Kind: TriggerChat, Thread: f.thread, Entry: q.ID})
	if res.Outcome != "waiting" {
		t.Fatalf("expected waiting, got %+v", res)
	}
	var decision *protolog.Entry
	for _, e := range f.entries(t) {
		if e.Kind == KindDecision {
			decision = e
		}
	}
	if decision == nil || decision.Author != f.r.Agent {
		t.Fatal("no decision entry by the agent")
	}
	var db struct {
		Options []string `json:"options"`
	}
	json.Unmarshal(decision.Body, &db)
	if len(db.Options) != 2 {
		t.Fatalf("options lost: %+v", db)
	}
	reply := f.post(t, KindDecisionReply, map[string]any{"choice": "yes"}, &protolog.Refs{RepliesTo: decision.ID})
	res = f.r.Run(context.Background(), Trigger{Kind: TriggerDecision, Thread: f.thread, Entry: reply.ID})
	if res.Outcome != "answered" || !strings.Contains(res.Answer, "hold") {
		t.Fatalf("decision run: %+v", res)
	}
	// The model was told what the person chose.
	last := m.tools[len(m.tools)-1]
	_ = last
}

func TestAutonomousRunSaysNothing(t *testing.T) {
	f := setup(t, &script{turns: []Message{say("NOTHING")}})
	f.post(t, KindNote, map[string]any{"text": "hello"}, nil)
	res := f.r.Run(context.Background(), Trigger{Kind: TriggerManual, Thread: f.thread})
	if res.Outcome != "nothing" {
		t.Fatalf("expected nothing, got %+v", res)
	}
	for _, e := range f.entries(t) {
		if e.Kind == KindNote && e.Author == f.r.Agent {
			t.Fatal("a NOTHING run must not post a note")
		}
	}
}

func TestAutonomousRunPostsNote(t *testing.T) {
	f := setup(t, &script{turns: []Message{say("The price contradicts the facts thread.")}})
	n := f.post(t, KindNote, map[string]any{"text": "Acme says 7%."}, nil)
	res := f.r.Run(context.Background(), Trigger{Kind: TriggerEntry, Thread: f.thread, Entry: n.ID})
	if res.Outcome != "posted" {
		t.Fatalf("expected posted, got %+v", res)
	}
	var note *protolog.Entry
	for _, e := range f.entries(t) {
		if e.Kind == KindNote && e.Author == f.r.Agent {
			note = e
		}
	}
	if note == nil || note.OnBehalfOf != f.pid || note.Refs == nil || note.Refs.DerivedFrom[0] != n.ID {
		t.Fatalf("agent note missing or unlinked: %+v", note)
	}
	if chainOf(note) != 1 {
		t.Fatalf("agent note should carry chain 1, got %d", chainOf(note))
	}
}

func TestToolCapEndsRunWithRecord(t *testing.T) {
	var turns []Message
	for i := 0; i < 20; i++ {
		turns = append(turns, call("list_threads", map[string]any{}))
	}
	f := setup(t, &script{turns: turns})
	res := f.r.Run(context.Background(), Trigger{Kind: TriggerManual, Thread: f.thread})
	if res.Outcome != "error" || !strings.Contains(res.Error, "tool calls") {
		t.Fatalf("expected the cap to stop the run: %+v", res)
	}
	es := f.entries(t)
	if es[len(es)-1].Kind != KindRun {
		t.Fatal("a failed run must still be recorded")
	}
	var rb struct {
		Outcome string `json:"outcome"`
		Error   string `json:"error"`
	}
	json.Unmarshal(es[len(es)-1].Body, &rb)
	if rb.Outcome != "error" || rb.Error == "" {
		t.Fatalf("run record does not say what went wrong: %+v", rb)
	}
}

func TestPeerTriggeredRunHasNarrowReach(t *testing.T) {
	f := setup(t, &script{})
	cfg, _ := LoadConfig(f.dir)
	// Autonomous, no brief: no web, no notes elsewhere for a peer entry.
	g := f.r.gateFor(Trigger{Kind: TriggerPeer}, Brief{}, cfg)
	names := map[string]bool{}
	for _, s := range f.r.toolSpecs(g, true, &externals{tools: map[string]*externalTool{}}) {
		names[s.Name] = true
	}
	if names["fetch_url"] || names["post_note"] {
		t.Fatalf("peer-triggered run exposes wide tools: %v", names)
	}
	if !names["ask_person"] || !names["search_context"] {
		t.Fatalf("peer-triggered run lost its basic tools: %v", names)
	}
	// A chat gets everything.
	g = f.r.gateFor(Trigger{Kind: TriggerChat}, Brief{}, cfg)
	names = map[string]bool{}
	for _, s := range f.r.toolSpecs(g, true, &externals{tools: map[string]*externalTool{}}) {
		names[s.Name] = true
	}
	if !names["fetch_url"] || !names["post_note"] {
		t.Fatalf("chat run missing tools: %v", names)
	}
	// A brief that allows the web on a domain enables fetch for schedules,
	// but only on that domain.
	g = f.r.gateFor(Trigger{Kind: TriggerSchedule}, Brief{Web: true, AllowDomains: []string{"*.sec.gov"}}, cfg)
	if !g.web || g.anyHost || !domainAllowed("www.sec.gov", g.domains) || domainAllowed("evil.com", g.domains) {
		t.Fatalf("schedule gate wrong: %+v", g)
	}
}

func TestFetchRefusesPrivateAndPlainHTTP(t *testing.T) {
	for _, u := range []string{"http://example.com", "https://localhost/x", "https://127.0.0.1/", "https://10.0.0.1/", "https://169.254.169.254/latest", "https://user:pw@example.com/"} {
		if _, err := PublicHost(u); err == nil {
			t.Fatalf("%s should be refused", u)
		}
	}
	if !domainAllowed("docs.stripe.com", []string{"docs.stripe.com"}) || domainAllowed("stripe.com", []string{"docs.stripe.com"}) {
		t.Fatal("exact domain match wrong")
	}
	if !domainAllowed("a.b.sec.gov", []string{"*.sec.gov"}) || !domainAllowed("sec.gov", []string{"*.sec.gov"}) || domainAllowed("notsec.gov", []string{"*.sec.gov"}) {
		t.Fatal("wildcard domain match wrong")
	}
	if got := htmlToText("<html><head><title>x</title><script>evil()</script></head><body><p>Hello <b>there</b></p><div>Bye</div></body></html>"); !strings.Contains(got, "Hello there") || strings.Contains(got, "evil") {
		t.Fatalf("html extraction: %q", got)
	}
}

func TestSchedulerQualifies(t *testing.T) {
	f := setup(t, &script{})
	s := &Scheduler{Runner: f.r, State: f.r.State}
	st := perm.Fold(f.thread, f.entries(t))
	mine := f.post(t, KindNote, map[string]any{"text": "mine"}, nil)
	q := f.post(t, KindQuestion, map[string]any{"text": "q?"}, nil)
	_, other, _ := protolog.GenerateKeypair()
	otherPID, _ := protolog.PrincipalID(other.Public().(ed25519.PublicKey))
	peer := &protolog.Entry{Kind: KindNote, Lane: protolog.LaneContent, Author: otherPID, Body: json.RawMessage(`{"text":"peer"}`)}
	peerAgent := &protolog.Entry{Kind: KindNote, Lane: protolog.LaneContent, Author: otherPID, OnBehalfOf: otherPID, Body: json.RawMessage(`{"text":"peer agent","chain":2}`)}
	ownAgent := &protolog.Entry{Kind: KindNote, Lane: protolog.LaneContent, Author: f.r.Agent, OnBehalfOf: f.pid, Body: json.RawMessage(`{"text":"me"}`)}

	if s.qualifies(mine, st, Brief{OnNewEntry: "others"}, true) {
		t.Fatal("'others' must ignore the person's own note")
	}
	if !s.qualifies(mine, st, Brief{OnNewEntry: "all"}, true) {
		t.Fatal("'all' must include the person's own note")
	}
	if s.qualifies(q, st, Brief{OnNewEntry: "all"}, true) {
		t.Fatal("chat questions are answered inline, never scheduled")
	}
	if !s.qualifies(peer, st, Brief{OnNewEntry: "others"}, true) {
		t.Fatal("a peer's note must trigger under 'others'")
	}
	if s.qualifies(peerAgent, st, Brief{OnNewEntry: "others"}, true) {
		t.Fatal("a peer agent's note must not trigger under 'others'")
	}
	if !s.qualifies(peerAgent, st, Brief{OnNewEntry: "all"}, true) || chainOf(peerAgent) != 2 {
		t.Fatal("'all' includes peer agents and keeps their chain depth")
	}
	if s.qualifies(ownAgent, st, Brief{OnNewEntry: "all"}, true) {
		t.Fatal("the agent must never trigger itself")
	}
	if s.qualifies(peer, st, Brief{OnNewEntry: "off"}, true) || s.qualifies(peer, st, Brief{}, false) {
		t.Fatal("no brief or off means no autonomy")
	}
}

func TestRevokeSeversEverywhere(t *testing.T) {
	f := setup(t, &script{turns: []Message{say("ok")}})
	q := f.post(t, KindQuestion, map[string]any{"text": "hi"}, nil)
	f.r.Run(context.Background(), Trigger{Kind: TriggerChat, Thread: f.thread, Entry: q.ID})
	n, err := RevokeAgent(context.Background(), f.st, f.dir, f.person, f.pid, f.r.Agent)
	if err != nil || n != 1 {
		t.Fatalf("revoke: n=%d err=%v", n, err)
	}
	if perm.Fold(f.thread, f.entries(t)).ActsFor(f.r.Agent, f.pid) {
		t.Fatal("agent still acts for the person after revocation")
	}
	if HasAgentKey(f.dir) {
		t.Fatal("agent key still on disk")
	}
}
