package agent

import (
	"context"
	"encoding/json"
	"strings"
)

// Turn-taking: when a message is written to a channel with several agents
// in it, decide who answers before anyone does, so they do not talk over
// each other. Mentions decide it outright; otherwise one small call reads
// the message and the agents' roles and picks, usually one, at most two, in
// order. Each then answers in turn and sees what the one before said.

// Speaker is someone who might answer: "" is the main agent.
type Speaker struct {
	ID, Name, About string
}

// Route returns who should answer, in order.
func (r *Runner) Route(ctx context.Context, cfg Config, text string, speakers []Speaker, direct bool) []string {
	if len(speakers) == 0 {
		return nil
	}
	// Named in the message: exactly those, in the order named.
	low := strings.ToLower(text)
	var named []string
	type hit struct {
		id string
		at int
	}
	var hits []hit
	for _, s := range speakers {
		n := strings.ToLower(strings.TrimSpace(s.Name))
		if n == "" {
			continue
		}
		if i := strings.Index(low, "@"+n); i >= 0 {
			hits = append(hits, hit{s.ID, i})
		}
	}
	for len(hits) > 0 {
		best := 0
		for i := range hits {
			if hits[i].at < hits[best].at {
				best = i
			}
		}
		named = append(named, hits[best].id)
		hits = append(hits[:best], hits[best+1:]...)
	}
	if len(named) > 0 {
		return named
	}
	fallback := []string{}
	if direct || len(speakers) == 1 {
		fallback = []string{speakers[0].ID}
	}
	if len(speakers) == 1 {
		return fallback
	}
	model, _ := r.ModelFor(cfg)
	if model == nil {
		return fallback
	}
	var roster strings.Builder
	for _, s := range speakers {
		label := s.Name
		if s.ID == "" {
			label += " (the main agent)"
		}
		roster.WriteString("- " + label + ": " + trunc(s.About, 160) + "\n")
	}
	rule := "Pick who should answer: usually one agent; a second only if it adds something clearly different that the first would not; nobody if the message is plainly meant for another person."
	if direct {
		rule = "Nobody but the agents is in this channel, so at least one must answer: usually the main agent; add one other only if its role clearly adds something different."
	}
	msgs := []Message{
		{Role: "system", Content: "You decide who speaks next in a group chat of AI agents working for one person. Reply with only a JSON array of agent names, in speaking order, e.g. [\"Juniper\"] or [] ."},
		{Role: "user", Content: "Agents here:\n" + roster.String() + "\nNew message:\n" + trunc(text, 1500) + "\n\n" + rule},
	}
	m, usage, err := model.Complete(ctx, msgs, nil)
	r.charge(usage)
	if err != nil {
		return fallback
	}
	raw := strings.TrimSpace(m.Content)
	if i, j := strings.Index(raw, "["), strings.LastIndex(raw, "]"); i >= 0 && j > i {
		raw = raw[i : j+1]
	}
	var names []string
	if json.Unmarshal([]byte(raw), &names) != nil {
		return fallback
	}
	var out []string
	seen := map[string]bool{}
	for _, n := range names {
		for _, s := range speakers {
			if strings.EqualFold(strings.TrimSpace(n), s.Name) && !seen[s.ID] {
				out = append(out, s.ID)
				seen[s.ID] = true
			}
		}
		if len(out) == 2 {
			break
		}
	}
	if len(out) == 0 && direct {
		return fallback
	}
	return out
}

// charge counts tokens spent outside a run (routing) against the day.
func (r *Runner) charge(u Usage) {
	if r.State == nil {
		return
	}
	r.State.Update(r.now(), func(s *State) { s.Tokens += u.Prompt + u.Completion })
}
