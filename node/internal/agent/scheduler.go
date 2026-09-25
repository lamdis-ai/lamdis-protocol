package agent

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/perm"
)

// The scheduler is what makes the agent autonomous. It watches every thread
// for entries it has not seen, fires runs the brief asks for, keeps
// schedules, and pulls from peers so remote entries arrive at all.
//
// It finds new entries by comparing chain heads, not by hooking appends: the
// MCP server is a separate process writing the same database, and a hook
// would never see it.

type Scheduler struct {
	Runner *Runner
	State  *State
	// SyncEvery overrides how often it pulls from peers. A node somebody is
	// talking to across the world wants seconds; one that is only keeping
	// itself current is fine with minutes.
	SyncEvery time.Duration
	// Sync pulls from and pushes to every peer; nil when the node has none.
	Sync func(ctx context.Context) error
	// Logf reports what happened; stderr in serve.
	Logf func(format string, args ...any)

	wake chan struct{}
	// projPending: news in a project's channel that the project's own agent
	// should hear about, keyed by the channel, debounced like any other.
	projPending map[string]projNews
	// chimePending: a person wrote in a channel where team members chime in.
	chimePending map[string]projNews
}

type projNews struct {
	project, entry string
	at             time.Time
}

const (
	pollEvery = 15 * time.Second
	debounce  = 20 * time.Second
	maxChain  = 3
)

// Push sends what is here to the peers now, for when somebody is waiting
// on the other end of it.
func (s *Scheduler) Push(ctx context.Context) {
	if s.Sync != nil {
		s.Sync(ctx)
	}
}

// Wake asks for an immediate poll, e.g. after the app writes an entry.
func (s *Scheduler) Wake() {
	if s.wake == nil {
		return
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Scheduler) logf(f string, a ...any) {
	if s.Logf != nil {
		s.Logf(f, a...)
	}
}

// Start runs until ctx ends.
func (s *Scheduler) Start(ctx context.Context) {
	s.wake = make(chan struct{}, 1)
	cfg, _ := LoadConfig(s.Runner.DataDir)
	syncEvery, err := time.ParseDuration(cfg.SyncEvery)
	if err != nil || syncEvery < 30*time.Second {
		syncEvery = 2 * time.Minute
	}
	if s.SyncEvery > 0 {
		syncEvery = s.SyncEvery
	}
	var lastSync time.Time
	// First pass records where every chain is, without firing: a node
	// that just started must not replay history as if it were news.
	s.poll(ctx, true)
	t := time.NewTicker(pollEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-s.wake:
		}
		if s.Sync != nil && time.Since(lastSync) >= syncEvery {
			lastSync = time.Now()
			err := s.Sync(ctx)
			s.State.Update(time.Now(), func(st *State) {
				st.LastSync = time.Now().UTC().Format(time.RFC3339)
				st.LastSyncError = ""
				if err != nil {
					st.LastSyncError = err.Error()
				}
			})
		}
		s.poll(ctx, false)
	}
}

// poll looks at every thread once and fires whatever is due.
func (s *Scheduler) poll(ctx context.Context, first bool) {
	r := s.Runner
	ids, err := r.Store.Threads(ctx)
	if err != nil {
		return
	}
	now := r.now()
	var fire []Trigger
	structure := ReadStructure(ctx, r.Store, r.Person)
	projBriefs := map[string]Brief{}
	projHas := map[string]bool{}
	for pid := range structure.IsProject {
		if ptl, err := r.Store.Thread(ctx, pid); err == nil {
			projBriefs[pid], projHas[pid] = LoadBrief(ptl, r.Person)
		}
	}
	if s.projPending == nil {
		s.projPending = map[string]projNews{}
	}
	if s.chimePending == nil {
		s.chimePending = map[string]projNews{}
	}
	for _, id := range ids {
		tl, err := r.Store.Thread(ctx, id)
		if err != nil {
			continue
		}
		st := perm.Fold(id, tl.Entries())
		brief, has := LoadBrief(tl, r.Person)
		heads := tl.Heads()

		var pending string
		var lastNew time.Time
		var lastRun string
		s.State.Update(now, func(state *State) {
			ts := state.thread(id)
			if !ts.Seen {
				for k, seq := range heads {
					ts.Heads[headKey(k)] = seq
				}
				ts.Seen = true
				return
			}
			var newest *protolog.Entry
			for k, seq := range heads {
				hk := headKey(k)
				seen := ts.Heads[hk]
				if seq <= seen {
					continue
				}
				for _, e := range tl.After(k, seen) {
					if s.qualifies(e, st, brief, has) && (newest == nil || e.Lamport > newest.Lamport) {
						newest = e
					}
					// Team members who chime in hear what people write here.
					if has && len(brief.Chime) > 0 && e.Lane == protolog.LaneContent && e.OnBehalfOf == "" && e.Author != r.Agent && !st.IsAgent(e.Author) {
						switch e.Kind {
						case KindRun, KindBrief, KindDecision, KindDecisionReply, KindConnect, KindConnectReply, KindProject, KindProjectMember:
						default:
							s.chimePending[id] = projNews{project: id, entry: e.ID, at: now}
						}
					}
					// The project above this channel may want to hear it too.
					if p := structure.Parent[id]; p != "" && s.qualifies(e, st, projBriefs[p], projHas[p]) {
						s.projPending[id] = projNews{project: p, entry: e.ID, at: now}
					}
				}
				ts.Heads[hk] = seq
			}
			if newest != nil {
				ts.LastNew = now
				ts.Pending = newest.ID
				if !(newest.Author == r.Person || newest.OnBehalfOf == r.Person) {
					ts.Pending = "peer:" + newest.ID
				}
			}
			pending, lastNew, lastRun = ts.Pending, ts.LastNew, ts.LastRun
		})
		if first {
			continue
		}
		// New entries, debounced so a burst becomes one run.
		if pending != "" && now.Sub(lastNew) >= debounce {
			kind, eid := TriggerEntry, pending
			if strings.HasPrefix(pending, "peer:") {
				kind, eid = TriggerPeer, strings.TrimPrefix(pending, "peer:")
			}
			c := 0
			if e := tl.Get(eid); e != nil {
				c = chainOf(e)
			}
			if c >= maxChain {
				s.logf("agent: %s: not reacting to %s, agent chain is %d deep", st.Title, eid, c)
			} else {
				fire = append(fire, Trigger{Kind: kind, Thread: id, Entry: eid, Chain: c})
			}
			s.State.Update(now, func(state *State) { state.thread(id).Pending = "" })
		}
		// Rhythms: the hours this thread's agent stands back and thinks.
		// Marked as run before the run happens, so a failure is a missed
		// morning rather than a loop.
		if has {
			for _, rh := range brief.Rhythms {
				if rh.At == "" {
					continue
				}
				name := rh.Name
				if name == "" {
					name = rh.At
				}
				var last string
				s.State.Update(now, func(st *State) { last = st.thread(id).Rhythms[name] })
				if due, today := rh.Due(now, last); due {
					s.State.Update(now, func(st *State) { st.thread(id).Rhythms[name] = today })
					fire = append(fire, Trigger{Kind: TriggerReflect, Thread: id, Rhythm: name, Prompt: rh.Prompt, Persona: rh.Persona})
				}
			}
		}

		// Schedule.
		if has && brief.Every != "" {
			if every, err := time.ParseDuration(brief.Every); err == nil && every >= 10*time.Minute {
				last, _ := time.Parse(time.RFC3339, lastRun)
				if now.Sub(last) >= every {
					fire = append(fire, Trigger{Kind: TriggerSchedule, Thread: id})
				}
			}
		}
	}
	if !first {
		for id, n := range s.chimePending {
			if now.Sub(n.at) < debounce {
				continue
			}
			delete(s.chimePending, id)
			tl, err := r.Store.Thread(ctx, id)
			if err != nil {
				continue
			}
			b, has := LoadBrief(tl, r.Person)
			if !has {
				continue
			}
			for i, pid := range b.Chime {
				if i == 3 {
					break // a crowd is not a team
				}
				fire = append(fire, Trigger{Kind: TriggerEntry, Thread: id, Entry: n.entry, Persona: pid})
			}
		}
		for child, n := range s.projPending {
			if now.Sub(n.at) < debounce {
				continue
			}
			delete(s.projPending, child)
			if structure.Parent[child] == n.project {
				fire = append(fire, Trigger{Kind: TriggerEntry, Thread: n.project, Entry: n.entry, From: child})
			}
		}
	}
	for _, t := range fire {
		res := r.Run(ctx, t)
		s.logf("agent: %s run on %s: %s %s", t.Kind, t.Thread, res.Outcome, res.Error)
		s.consumeOwn(ctx, t.Thread)
		// Whoever asked is waiting, so send the answer rather than letting
		// it sit until the next round.
		if s.Sync != nil && res.Outcome != "nothing" {
			if err := s.Sync(ctx); err != nil {
				s.logf("agent: could not send that back yet: %v", err)
			}
		}
	}
}

// consumeOwn advances heads past the agent's own writes so a run's outputs
// are never mistaken for news on the next poll.
func (s *Scheduler) consumeOwn(ctx context.Context, thread string) {
	r := s.Runner
	tl, err := r.Store.Thread(ctx, thread)
	if err != nil {
		return
	}
	heads := tl.Heads()
	s.State.Update(r.now(), func(state *State) {
		ts := state.thread(thread)
		for k, seq := range heads {
			if k.Author == r.Agent {
				ts.Heads[headKey(k)] = seq
			}
		}
	})
}

// Consume is the exported form for the app: after it answers a chat or a
// decision inline, the entries involved are not news.
func (s *Scheduler) Consume(ctx context.Context, thread string) {
	r := s.Runner
	tl, err := r.Store.Thread(ctx, thread)
	if err != nil {
		return
	}
	heads := tl.Heads()
	s.State.Update(r.now(), func(state *State) {
		ts := state.thread(thread)
		for k, seq := range heads {
			if k.Author == r.Agent || k.Author == r.Person {
				ts.Heads[headKey(k)] = seq
			}
		}
	})
}

// qualifies decides whether an entry should trigger a run. The agent's own
// writes never do; chats and decisions are answered inline; a brief set to
// "others" ignores the person's own notes and other people's agents.
func (s *Scheduler) qualifies(e *protolog.Entry, st *perm.State, b Brief, has bool) bool {
	r := s.Runner
	if !has || b.OnNewEntry == "" || b.OnNewEntry == "off" {
		return false
	}
	if e.Lane == protolog.LaneControl || e.Author == r.Agent {
		return false
	}
	switch e.Kind {
	case KindQuestion, KindAnswer, KindDecision, KindDecisionReply, KindRun, KindBrief:
		return false
	}
	if b.OnNewEntry == "others" {
		if e.Author == r.Person || e.OnBehalfOf == r.Person {
			return false
		}
		if e.OnBehalfOf != "" || st.IsAgent(e.Author) {
			return false
		}
	}
	return true
}

// chainOf is how many agent hops led to an entry: 0 for a person's own
// words, at least 1 for anything an agent wrote.
func chainOf(e *protolog.Entry) int {
	if e.OnBehalfOf == "" {
		return 0
	}
	var b struct {
		Chain int `json:"chain"`
	}
	if json.Unmarshal(e.Body, &b) != nil || b.Chain < 1 {
		return 1
	}
	return b.Chain
}

func headKey(k protolog.ChainKey) string { return k.Author + "|" + string(k.Lane) }

// Status is what the interface shows about autonomy.
func (s *Scheduler) Status(now time.Time) map[string]any {
	out := s.State.Snapshot(now)
	out["poll_seconds"] = int(pollEvery.Seconds())
	out["sync"] = s.Sync != nil
	return out
}
