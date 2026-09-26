package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAMentionPicksTheTeamMember(t *testing.T) {
	c := Config{Agents: []Persona{{ID: "p1", Name: "Scout"}, {ID: "p2", Name: "Haggler"}}}
	for text, want := range map[string]string{
		"@Scout find tilers near Ortonville": "p1",
		"hey @haggler, counter at 25%":       "p2",
		"@Haggler then @Scout":               "p2",
		"email scout@example.com":            "",
		"@Scouting is not a name":            "",
		"no one in particular":               "",
	} {
		if got := PersonaMentioned(c, text); got != want {
			t.Fatalf("%q -> %q, want %q", text, got, want)
		}
	}
}

// A team member answers in its own name and manner, and what it writes says
// who wrote it.
func TestATeamMemberAnswersAsItself(t *testing.T) {
	m := &seeing{script: script{turns: []Message{say("Three tilers found.")}}}
	f := setup(t, m)
	cfg, _ := LoadConfig(f.dir)
	cfg.Agents = []Persona{{ID: "p1", Name: "Scout", About: "A thorough local researcher."}}
	SaveConfig(f.dir, cfg)
	q := f.post(t, KindQuestion, map[string]any{"text": "@Scout find tilers"}, nil)
	res := f.r.Run(context.Background(), Trigger{Kind: TriggerChat, Thread: f.thread, Entry: q.ID, Persona: "p1"})
	if res.Outcome != "answered" {
		t.Fatalf("%+v", res)
	}
	if !m.saw("Your name is Scout") || !m.saw("A thorough local researcher.") {
		t.Fatal("the persona's name and manner did not reach the model")
	}
	var stamped bool
	for _, e := range f.entries(t) {
		if e.Kind == KindAnswer {
			var b struct {
				Persona string `json:"persona"`
			}
			json.Unmarshal(e.Body, &b)
			stamped = b.Persona == "Scout"
		}
	}
	if !stamped {
		t.Fatal("the answer does not say Scout wrote it")
	}
	if f.r.persona != nil {
		t.Fatal("the persona outlived its run")
	}
}

// A team member set to chime in speaks up after a person writes, in its own
// name, and does not answer itself or the agent.
func TestATeamMemberChimesInOnItsOwn(t *testing.T) {
	m := &seeing{script: script{turns: []Message{say("What if Rival's deposit is lost? Ask for staged payments.")}}}
	f := setup(t, m)
	clock := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	f.r.Now = func() time.Time { return clock }
	cfg, _ := LoadConfig(f.dir)
	cfg.Agents = []Persona{{ID: "pd", Name: "Devil", About: "A devil's advocate."}}
	SaveConfig(f.dir, cfg)
	f.post(t, KindBrief, map[string]any{"text": "", "on_new_entry": "off", "agents": []string{"pd"}, "chime": []string{"pd"}, "main_quiet": true}, nil)
	s := &Scheduler{Runner: f.r, State: f.r.State}
	ctx := context.Background()
	s.poll(ctx, true)

	f.post(t, KindNote, map[string]any{"text": "Going with Rival, 50% deposit up front."}, nil)
	s.poll(ctx, false)
	clock = clock.Add(debounce + time.Second)
	s.poll(ctx, false)

	var said bool
	for _, e := range f.entries(t) {
		var b struct {
			Text, Persona string
		}
		json.Unmarshal(e.Body, &b)
		if e.Author == f.r.Agent && b.Persona == "Devil" && strings.Contains(b.Text, "staged payments") {
			said = true
		}
	}
	if !said {
		t.Fatal("the devil's advocate did not chime in")
	}
	if !m.saw("You are Devil, one of the agents in this channel") {
		t.Fatal("it was not told it was chiming in")
	}
	// Its own words do not set it off again.
	calls := m.calls
	s.poll(ctx, false)
	clock = clock.Add(debounce + time.Second)
	s.poll(ctx, false)
	if m.calls != calls {
		t.Fatal("the team member answered itself")
	}
}

func TestTurnTaking(t *testing.T) {
	sp := []Speaker{{ID: "", Name: "Juniper"}, {ID: "pd", Name: "Devil"}, {ID: "ps", Name: "Scout"}}
	f := setup(t, &script{})
	cfg, _ := LoadConfig(f.dir)
	ctx := context.Background()
	if got := f.r.Route(ctx, cfg, "@Scout then @Devil, what do you think?", sp, false); len(got) != 2 || got[0] != "ps" || got[1] != "pd" {
		t.Fatalf("mentions: %v", got)
	}
	f.r.Model = &script{turns: []Message{say(`["Devil", "Scout", "Juniper"]`)}}
	if got := f.r.Route(ctx, cfg, "is this a good idea?", sp, false); len(got) != 2 || got[0] != "pd" {
		t.Fatalf("model pick, capped at two: %v", got)
	}
	f.r.Model = &script{turns: []Message{say(`not json at all`)}}
	if got := f.r.Route(ctx, cfg, "hi", sp, true); len(got) != 1 || got[0] != "" {
		t.Fatalf("alone, a failed route still gets the main agent: %v", got)
	}
	f.r.Model = &script{turns: []Message{say(`[]`)}}
	if got := f.r.Route(ctx, cfg, "Ray, see you Thursday", sp, false); len(got) != 0 {
		t.Fatalf("a message for another person got %v", got)
	}
}
