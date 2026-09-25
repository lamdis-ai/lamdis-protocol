package agent

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

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
	// Rhythms are times of day this thread's agent stops and thinks, each
	// with its own question. Reacting to what arrives is not the same as
	// standing back and looking at the whole thing, and a person does both
	// at different hours; this is how you say when and about what.
	Rhythms []Rhythm `json:"rhythms,omitempty"`
	// Autonomy overrides the agent's setting here: "" (same as everywhere),
	// "ask", or "auto".
	Autonomy string `json:"autonomy,omitempty"`
	// Agents are the team members who work in this channel besides the
	// main agent, by persona id.
	Agents []string `json:"agents,omitempty"`

	ID string `json:"-"`
	TS string `json:"-"`
}

// Rhythm is one time of day the agent thinks rather than reacts.
type Rhythm struct {
	Name string `json:"name"`
	At   string `json:"at"`             // "07:30", in Zone
	Zone string `json:"zone,omitempty"` // IANA name; empty means this machine's time
	// Paused keeps a rhythm written down without letting it fire, so turning
	// one off for a holiday does not mean retyping the question afterwards.
	Paused bool `json:"paused,omitempty"`
	// Persona is who does this review; empty means the main agent.
	Persona string `json:"persona,omitempty"`
	Prompt  string `json:"prompt"`
}

// Due reports whether this rhythm should run now, given the date string of
// the last time it did. It fires once a day, only after its hour, and only
// within a window: a rhythm you slept through is a rhythm you missed, not
// something to spring on somebody at midnight.
func (r Rhythm) Due(now time.Time, lastRun string) (bool, string) {
	hh, mm, ok := parseClock(r.At)
	if !ok || r.Paused {
		return false, ""
	}
	loc := time.Local
	if r.Zone != "" {
		if l, err := time.LoadLocation(r.Zone); err == nil {
			loc = l
		}
	}
	local := now.In(loc)
	today := local.Format("2006-01-02")
	if lastRun == today {
		return false, today
	}
	at := time.Date(local.Year(), local.Month(), local.Day(), hh, mm, 0, 0, loc)
	if local.Before(at) {
		return false, today
	}
	if local.Sub(at) > rhythmWindow {
		return false, today
	}
	return true, today
}

const rhythmWindow = 4 * time.Hour

// ParseClock reads "07:30".
func ParseClock(s string) (int, int, bool) { return parseClock(s) }

func parseClock(s string) (int, int, bool) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, 0, false
	}
	h, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	m, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, false
	}
	return h, m, true
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
