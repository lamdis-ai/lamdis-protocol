package agent

import (
	"context"
	"strings"
	"testing"

	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
)

// seeing is a Model that keeps every message it was shown.
type seeing struct {
	script
	shown []string
}

func (s *seeing) Complete(ctx context.Context, msgs []Message, tools []ToolSpec) (Message, Usage, error) {
	for _, m := range msgs {
		s.shown = append(s.shown, m.Content)
	}
	return s.script.Complete(ctx, msgs, tools)
}

func (s *seeing) saw(x string) bool {
	for _, m := range s.shown {
		if strings.Contains(m, x) {
			return true
		}
	}
	return false
}

func (f *fixture) thread2(t *testing.T, title string) string {
	t.Helper()
	_, g, err := protolog.NewThreadWith(f.person, title, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.st.AppendEntries(context.Background(), []*protolog.Entry{g}); err != nil {
		t.Fatal(err)
	}
	return g.ID
}

func (f *fixture) postIn(t *testing.T, thread, kind string, body map[string]any) *protolog.Entry {
	t.Helper()
	was := f.thread
	f.thread = thread
	defer func() { f.thread = was }()
	return f.post(t, kind, body, nil)
}

// Two vendors quote for one job, each in their own channel inside a project.
// Whatever the agent in one vendor's channel does, the other's bid must not
// reach it; the project, which only the owner sees, reads both.
func TestAChannelInAProjectCannotSeeItsSiblings(t *testing.T) {
	m := &seeing{script: script{turns: []Message{}}}
	f := setup(t, m)
	acme := f.thread
	rival := f.thread2(t, "Quote from Rival Tiling")
	proj := f.thread2(t, "Kitchen reno")
	f.postIn(t, acme, KindNote, map[string]any{"text": "Acme quotes 11,800."})
	f.postIn(t, rival, KindNote, map[string]any{"text": "Rival quotes 9,400 and can start Monday."})
	f.postIn(t, proj, KindProject, map[string]any{"text": "project"})
	f.postIn(t, proj, KindProjectMember, map[string]any{"thread": acme, "op": "add"})
	f.postIn(t, proj, KindProjectMember, map[string]any{"thread": rival, "op": "add"})

	s := ReadStructure(context.Background(), f.st, f.pid)
	if !s.IsProject[proj] || s.Parent[acme] != proj || s.Parent[rival] != proj || len(s.Children[proj]) != 2 {
		t.Fatalf("structure: %+v", s)
	}

	// In Acme's channel the agent tries every way it has to look sideways.
	m.turns = []Message{
		call("list_threads", map[string]any{}),
		call("read_thread", map[string]any{"thread": rival}),
		call("search_context", map[string]any{"query": "Rival quotes"}),
		call("post_note", map[string]any{"thread": rival, "text": "hello"}),
		say("done"),
	}
	q := f.postIn(t, acme, KindQuestion, map[string]any{"text": "Is Acme the cheapest?"})
	f.r.Run(context.Background(), Trigger{Kind: TriggerChat, Thread: acme, Entry: q.ID})
	if m.calls != 5 {
		t.Fatalf("the agent only got %d turns; the walls were not exercised", m.calls)
	}
	if !m.saw("only one you can see") || !m.saw("can read only this channel") {
		t.Fatal("the tools did not answer as walled")
	}
	for _, leak := range []string{"9,400", "Rival Tiling", "Kitchen reno", rival, proj} {
		if m.saw(leak) {
			t.Fatalf("the walled channel's agent saw %q", leak)
		}
	}
	tl, _ := f.st.Thread(context.Background(), rival)
	for _, e := range tl.Entries() {
		if e.Author == f.r.Agent {
			t.Fatal("the agent wrote into a sibling channel from a walled one")
		}
	}

	// In the project it sees both.
	m.shown, m.calls = nil, 0
	m.turns = []Message{say("Rival is cheaper.")}
	pq := f.postIn(t, proj, KindQuestion, map[string]any{"text": "Who is cheapest?"})
	f.r.Run(context.Background(), Trigger{Kind: TriggerChat, Thread: proj, Entry: pq.ID})
	if !m.saw("9,400") || !m.saw("11,800") {
		t.Fatal("the project's agent did not see its channels")
	}

	// Taking a channel out of the project takes the wall down with it.
	f.postIn(t, proj, KindProjectMember, map[string]any{"thread": rival, "op": "remove"})
	if ReadStructure(context.Background(), f.st, f.pid).Walled(rival) {
		t.Fatal("a removed channel is still walled")
	}
}

// Only the owner arranges a project: membership written by anyone else is
// ignored, so a participant cannot pull a channel into view.
func TestOnlyTheOwnerArrangesAProject(t *testing.T) {
	f := setup(t, &script{})
	proj := f.thread2(t, "P")
	f.postIn(t, proj, KindProject, map[string]any{"text": "project"})
	s := ReadStructure(context.Background(), f.st, "someone-else")
	if s.IsProject[proj] {
		t.Fatal("a project was recognised for someone who did not make it")
	}
}

// Full auto takes away asking and confirming; the default keeps both.
func TestFullAutoStopsAsking(t *testing.T) {
	if FullAuto(Config{}, Brief{}) || !FullAuto(Config{Autonomy: "auto"}, Brief{}) {
		t.Fatal("the agent-wide setting is not honoured")
	}
	if FullAuto(Config{Autonomy: "auto"}, Brief{Autonomy: "ask"}) || !FullAuto(Config{}, Brief{Autonomy: "auto"}) {
		t.Fatal("a channel's own setting does not win")
	}
	m := &script{turns: []Message{say("ok")}}
	f := setup(t, m)
	cfg, _ := LoadConfig(f.dir)
	cfg.Autonomy = "auto"
	SaveConfig(f.dir, cfg)
	q := f.post(t, KindQuestion, map[string]any{"text": "go"}, nil)
	f.r.Run(context.Background(), Trigger{Kind: TriggerChat, Thread: f.thread, Entry: q.ID})
	for _, tool := range m.tools[0] {
		if tool.Name == "ask_person" {
			t.Fatal("full auto still offered ask_person")
		}
	}
}
