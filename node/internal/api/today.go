package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/agent"
	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
)

// Today is the home screen: what needs the person, what their agent did
// while they were away, and what it will do next. It is one pass over every
// thread so the page can open on a single request instead of one per thread.
//
// Nothing here is new state. Decisions, runs and rhythms are all already in
// the record; this only gathers them in one place, which is the whole point
// of a home screen.

type todayDecision struct {
	Thread  string   `json:"thread"`
	Title   string   `json:"title"`
	ID      string   `json:"id"`
	Text    string   `json:"text"`
	Options []string `json:"options,omitempty"`
	TS      string   `json:"ts"`
	// Connect is set when this is a Connect card rather than a question.
	Connect *todayConnect `json:"connect,omitempty"`
}

type todayConnect struct {
	Service string `json:"service"`
	URL     string `json:"url"`
}

type todayRun struct {
	Thread   string          `json:"thread"`
	Title    string          `json:"title"`
	ID       string          `json:"id"`
	TS       string          `json:"ts"`
	Text     string          `json:"text"`
	Data     json.RawMessage `json:"data"`
	Followed string          `json:"followed,omitempty"` // what the agent wrote as a result
}

type todaySchedule struct {
	Thread string `json:"thread"`
	Title  string `json:"title"`
	Index  int    `json:"index"` // position in the brief's rhythms; -1 for "every"
	Name   string `json:"name,omitempty"`
	At     string `json:"at,omitempty"`
	Zone   string `json:"zone,omitempty"`
	Every  string `json:"every,omitempty"`
	Prompt string `json:"prompt,omitempty"`
	Paused bool   `json:"paused,omitempty"`
}

type todayWatch struct {
	Thread string `json:"thread"`
	Title  string `json:"title"`
	On     string `json:"on"` // others | all
}

// How far back "while you were away" reaches.
const todayWindow = 36 * time.Hour

func (a *App) handleToday(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ids, err := a.Store.Threads(ctx)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	since := a.now().Add(-todayWindow)
	out := struct {
		Decisions []todayDecision `json:"decisions"`
		Runs      []todayRun      `json:"runs"`
		Schedules []todaySchedule `json:"schedules"`
		Watching  []todayWatch    `json:"watching"`
		Agent     string          `json:"agent"`
	}{Decisions: []todayDecision{}, Runs: []todayRun{}, Schedules: []todaySchedule{}, Watching: []todayWatch{},
		Agent: a.displayName(a.AgentSelf)}

	for _, id := range ids {
		title, entries, err := a.entriesFor(ctx, id, []protolog.Lane{protolog.LaneSummary, protolog.LaneContent})
		if err != nil {
			continue
		}
		if title == "" {
			title = "(untitled)"
		}
		for i, e := range entries {
			switch e.Kind {
			case agent.KindDecision:
				if !e.Resolved {
					out.Decisions = append(out.Decisions, todayDecision{Thread: id, Title: title, ID: e.ID, Text: e.Text, Options: e.Options, TS: e.TS})
				}
			case agent.KindConnect:
				if !e.Resolved {
					var b struct {
						Service string `json:"service"`
						URL     string `json:"url"`
					}
					json.Unmarshal(e.Data, &b)
					out.Decisions = append(out.Decisions, todayDecision{Thread: id, Title: title, ID: e.ID, Text: e.Text, TS: e.TS,
						Connect: &todayConnect{Service: b.Service, URL: b.URL}})
				}
			case agent.KindRun:
				t, err := time.Parse(time.RFC3339, e.TS)
				if err != nil || t.Before(since) {
					continue
				}
				run := todayRun{Thread: id, Title: title, ID: e.ID, TS: e.TS, Text: e.Text, Data: e.Data}
				// A run is recorded after what it wrote, so the entry just
				// before it by the agent is its result.
				for j := i - 1; j >= 0 && j >= i-4; j-- {
					p := entries[j]
					if p.Author == a.AgentSelf && p.Kind != agent.KindRun && p.Text != "" {
						run.Followed = p.Text
						break
					}
				}
				out.Runs = append(out.Runs, run)
			}
		}
		if tl, err := a.Store.Thread(ctx, id); err == nil {
			if b, has := agent.LoadBrief(tl, a.Self); has {
				for k, rh := range b.Rhythms {
					out.Schedules = append(out.Schedules, todaySchedule{Thread: id, Title: title, Index: k, Name: rh.Name,
						At: rh.At, Zone: rh.Zone, Prompt: rh.Prompt, Paused: rh.Paused})
				}
				if b.Every != "" {
					out.Schedules = append(out.Schedules, todaySchedule{Thread: id, Title: title, Index: -1, Every: b.Every, Prompt: b.Text})
				}
				if b.OnNewEntry == "others" || b.OnNewEntry == "all" {
					out.Watching = append(out.Watching, todayWatch{Thread: id, Title: title, On: b.OnNewEntry})
				}
			}
		}
	}
	sort.Slice(out.Decisions, func(i, j int) bool { return out.Decisions[i].TS > out.Decisions[j].TS })
	sort.Slice(out.Runs, func(i, j int) bool { return out.Runs[i].TS > out.Runs[j].TS })
	if len(out.Runs) > 40 {
		out.Runs = out.Runs[:40]
	}
	sort.SliceStable(out.Schedules, func(i, j int) bool { return out.Schedules[i].At < out.Schedules[j].At })
	writeJSON(w, out)
}
