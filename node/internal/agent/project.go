package agent

import (
	"context"
	"encoding/json"

	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/store"
)

// Projects: a channel that holds other channels.
//
// Three vendors quoting for one job each get their own channel, shared with
// that vendor and nobody else. The project above them is the owner's alone:
// its agent reads every channel in it and can coordinate across them, while
// the agent inside a vendor's channel reads only that channel, so nothing one
// bidder wrote can reach another.
//
// Membership is written only in the project, never in the channels it holds.
// A vendor's copy of their channel carries no sign that a project, or any
// other bidder, exists.

const (
	KindProject       = "project.is"
	KindProjectMember = "project.member" // {"thread": id, "op": "add"|"remove"}
)

// Structure is how this node's channels nest.
type Structure struct {
	IsProject map[string]bool
	Parent    map[string]string   // channel -> its project
	Children  map[string][]string // project -> its channels, in the order added
}

// Walled reports whether the agent in this channel must keep to it.
func (s Structure) Walled(thread string) bool { return s.Parent[thread] != "" }

// ReadStructure folds every project's membership entries. Only entries the
// person wrote count: a project is theirs to arrange.
func ReadStructure(ctx context.Context, st store.Store, person string) Structure {
	s := Structure{IsProject: map[string]bool{}, Parent: map[string]string{}, Children: map[string][]string{}}
	ids, err := st.Threads(ctx)
	if err != nil {
		return s
	}
	exists := map[string]bool{}
	for _, id := range ids {
		exists[id] = true
	}
	for _, id := range ids {
		tl, err := st.Thread(ctx, id)
		if err != nil {
			continue
		}
		var order []string
		in := map[string]bool{}
		for _, e := range tl.Entries() {
			if e.Author != person || e.Lane == protolog.LaneControl {
				continue
			}
			switch e.Kind {
			case KindProject:
				s.IsProject[id] = true
			case KindProjectMember:
				var b struct {
					Thread string `json:"thread"`
					Op     string `json:"op"`
				}
				if json.Unmarshal(e.Body, &b) != nil || b.Thread == "" || b.Thread == id {
					continue
				}
				if b.Op == "remove" {
					in[b.Thread] = false
					continue
				}
				if !in[b.Thread] {
					order = append(order, b.Thread)
				}
				in[b.Thread] = true
			}
		}
		if !s.IsProject[id] {
			continue
		}
		for _, c := range order {
			// A channel belongs to one project: the latest one to claim it.
			if in[c] && exists[c] && !s.IsProject[c] {
				if old := s.Parent[c]; old != "" && old != id {
					s.Children[old] = without(s.Children[old], c)
				}
				s.Parent[c] = id
				s.Children[id] = append(without(s.Children[id], c), c)
			}
		}
	}
	return s
}

func without(xs []string, x string) []string {
	out := xs[:0:0]
	for _, v := range xs {
		if v != x {
			out = append(out, v)
		}
	}
	return out
}
