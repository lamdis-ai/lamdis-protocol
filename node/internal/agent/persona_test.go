package agent

import (
	"context"
	"encoding/json"
	"testing"
)

func TestAMentionPicksTheTeamMember(t *testing.T) {
	c := Config{Agents: []Persona{{ID: "p1", Name: "Scout"}, {ID: "p2", Name: "Haggler"}}}
	for text, want := range map[string]string{
		"@Scout find tilers near Ortonville": "p1",
		"hey @haggler, counter at 25%":        "p2",
		"@Haggler then @Scout":                "p2",
		"email scout@example.com":             "",
		"@Scouting is not a name":             "",
		"no one in particular":                "",
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
