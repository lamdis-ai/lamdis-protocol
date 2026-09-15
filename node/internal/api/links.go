package api

// Which threads are tied to which.
//
// A thread is the unit of sharing, not the unit of thinking, so the ties
// between them are where most of the meaning lives: a note that points at
// another thread, an agent that had to read three of them to answer one
// question, a conclusion it carried from one to another. All of that is
// already in the record; this just adds it up and says it out loud.

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/agent"
	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/perm"
)

var reRef = regexp.MustCompile(`\[\[([^\]]{1,120})\]\]`)

type linkOut struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Count int    `json:"count"`
	Why   string `json:"why"`
}

// handleLinks reports what this thread is connected to, and how.
func (a *App) handleLinks(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()
	ids, err := a.Store.Threads(ctx)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	titles := map[string]string{} // lower title -> id
	names := map[string]string{}  // id -> title
	logs := map[string][]*protolog.Entry{}
	for _, t := range ids {
		tl, err := a.Store.Thread(ctx, t)
		if err != nil {
			continue
		}
		st := perm.Fold(t, tl.Entries())
		title := st.Title
		if title == "" {
			title = "(untitled)"
		}
		names[t] = title
		titles[strings.ToLower(title)] = t
		logs[t] = tl.Entries()
	}
	if _, ok := names[id]; !ok {
		http.Error(w, "no such thread", http.StatusNotFound)
		return
	}
	me := strings.ToLower(names[id])

	out := map[string]int{}  // threads this one points at
	in := map[string]int{}   // threads that point here
	read := map[string]int{} // threads the agent used while working here
	via := map[string]string{}

	// References written in entries, both directions.
	for t, entries := range logs {
		for _, e := range entries {
			if e.Lane == protolog.LaneControl {
				continue
			}
			txt := bodyTextOf(e)
			for _, m := range reRef.FindAllStringSubmatch(txt, -1) {
				name := strings.ToLower(strings.TrimSpace(m[1]))
				target := titles[name]
				switch {
				case t == id && target != "" && target != id:
					out[target]++
				case t != id && name == me:
					in[t]++
				}
			}
			// A note the agent carried here from somewhere else.
			if t == id && e.Kind == agent.KindNote {
				var b struct {
					From string `json:"from_thread"`
				}
				if json.Unmarshal(e.Body, &b) == nil && b.From != "" && b.From != id && names[b.From] != "" {
					in[b.From]++
					via[b.From] = "your agent brought a note from there"
				}
			}
		}
	}

	// Threads the agent actually opened while answering in this one.
	for _, e := range logs[id] {
		if e.Kind != agent.KindRun {
			continue
		}
		var b struct {
			Threads []string `json:"threads_read"`
		}
		if json.Unmarshal(e.Body, &b) != nil {
			continue
		}
		for _, t := range b.Threads {
			if t != id && names[t] != "" {
				read[t]++
			}
		}
	}

	list := func(m map[string]int, why string) []linkOut {
		res := []linkOut{}
		for t, n := range m {
			w := why
			if v, ok := via[t]; ok {
				w = v
			}
			res = append(res, linkOut{ID: t, Title: names[t], Count: n, Why: w})
		}
		sort.Slice(res, func(i, j int) bool {
			if res[i].Count != res[j].Count {
				return res[i].Count > res[j].Count
			}
			return res[i].Title < res[j].Title
		})
		return res
	}
	o := list(out, "this thread points there")
	i := list(in, "points at this thread")
	rd := list(read, "your agent read it to answer here")
	writeJSON(w, map[string]any{"id": id, "title": names[id],
		"out": o, "in": i, "read": rd, "total": len(o) + len(i) + len(rd)})
}

func bodyTextOf(e *protolog.Entry) string {
	var b struct {
		Text string `json:"text"`
	}
	json.Unmarshal(e.Body, &b)
	return b.Text
}
