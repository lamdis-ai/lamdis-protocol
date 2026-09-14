package agent

import (
	"encoding/json"

	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
)

// Entry kinds this package writes or reads. They live outside core.* so the
// protocol spec is untouched; peers replicate them opaquely.
const (
	KindQuestion      = "chat.question"
	KindAnswer        = "chat.answer"
	KindNote          = "agent.note"
	KindRun           = "agent.run"
	KindDecision      = "agent.decision"
	KindDecisionReply = "agent.decision_reply"
	KindBrief         = "thread.brief"
)

// Brief is a thread's standing instructions: the harness. It is an entry in
// the thread (kind thread.brief, content lane) so it travels with the thread
// and a collaborator can read what your agent has been told to do.
type Brief struct {
	Text string `json:"text"`
	// OnNewEntry: "off", "others" (anyone but you and your agent), "all".
	OnNewEntry string `json:"on_new_entry"`
	// Every is a Go duration such as "6h"; empty means no schedule.
	Every string `json:"every"`
	// Web lets autonomous runs fetch pages from AllowDomains (or the node's).
	Web          bool     `json:"web"`
	AllowDomains []string `json:"allow_domains,omitempty"`
	// Tools are "server.tool" names autonomous runs may call.
	Tools []string `json:"tools,omitempty"`

	ID string `json:"-"`
	TS string `json:"-"`
}

// LoadBrief returns the latest brief the person themselves wrote. Only the
// person's own briefs count: a collaborator with contribute could otherwise
// post one and steer your agent, which is the obvious attack.
func LoadBrief(tl *protolog.ThreadLog, person string) (Brief, bool) {
	var best *protolog.Entry
	for _, e := range tl.Entries() {
		if e.Kind != KindBrief || e.Lane != protolog.LaneContent || e.Author != person || e.OnBehalfOf != "" {
			continue
		}
		if best == nil || e.Lamport > best.Lamport || (e.Lamport == best.Lamport && e.ID > best.ID) {
			best = e
		}
	}
	if best == nil {
		return Brief{OnNewEntry: "off"}, false
	}
	var b Brief
	if json.Unmarshal(best.Body, &b) != nil {
		return Brief{OnNewEntry: "off"}, false
	}
	if b.OnNewEntry == "" {
		b.OnNewEntry = "off"
	}
	b.ID, b.TS = best.ID, best.TS
	return b, true
}
