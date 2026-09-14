// Package perm computes effective permissions by folding a thread's control
// lane in the protocol's total order. The fold is deterministic on every
// replica and fails closed: deny by default, deny/revoke beats grant, and
// only person-signed entries by stewards change anything.
package perm

import (
	"encoding/json"
	"time"

	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
)

// Scope is one of the four fixed v1 scopes.
type Scope string

const (
	ScopeContribute Scope = "contribute"
	ScopeRead       Scope = "read"
	ScopeSummary    Scope = "summary"
	ScopeSearch     Scope = "search"
)

func ValidScope(s Scope) bool {
	switch s {
	case ScopeContribute, ScopeRead, ScopeSummary, ScopeSearch:
		return true
	}
	return false
}

// ScopeSet is a set of scopes held by a principal.
type ScopeSet map[Scope]bool

func (ss ScopeSet) Has(s Scope) bool { return ss[s] }

// Lanes returns the lanes a holder of this scope set may replicate/read.
// Control is visible to every member at any scope (replicas must be able to
// verify grants). Search alone replicates nothing beyond control.
func (ss ScopeSet) Lanes() []protolog.Lane {
	if len(ss) == 0 {
		return nil
	}
	lanes := []protolog.Lane{protolog.LaneControl}
	if ss.Has(ScopeRead) {
		return append(lanes, protolog.LaneSummary, protolog.LaneContent)
	}
	if ss.Has(ScopeSummary) {
		return append(lanes, protolog.LaneSummary)
	}
	return lanes
}

// grantBody is the body schema of core.grant / core.deny / core.revoke.
// Grant: {principal, scopes, ttl_seconds?}. Deny/Revoke: {principal}.
type grantBody struct {
	Principal  string   `json:"principal"`
	Scopes     []string `json:"scopes,omitempty"`
	TTLSeconds int64    `json:"ttl_seconds,omitempty"`
	Request    string   `json:"request,omitempty"` // entry id of core.access_request
}

type membershipBody struct {
	Member string `json:"member"`
	Role   string `json:"role"` // "steward" | "member" | "removed"
}

type delegationBody struct {
	Agent     string `json:"agent"`
	ExpiresAt string `json:"expires_at,omitempty"`
	Revoked   bool   `json:"revoked,omitempty"`
}

// AccessRequest is a pending, unanswered core.access_request.
type AccessRequest struct {
	EntryID   string
	Principal string
	Scopes    []string
	Reason    string
	TS        string
	lamport   uint64
}

// State is the folded permission state of one thread.
type State struct {
	Thread   string
	Stewards map[string]bool
	// Discoverable threads advertise their existence (id + title only) to
	// any authenticated peer, so access can be requested. Default: hidden.
	Discoverable bool
	Title        string
	// requests tracks the latest access request per principal.
	requests map[string]*AccessRequest
	// grants maps principal → granted scopes (already net of deny/revoke).
	grants map[string]ScopeSet
	// expiry maps principal → grant expiry time (zero = no expiry).
	expiry map[string]time.Time
	// delegations maps agent principal → person principal.
	delegations map[string]string
	// deniedAt maps principal → lamport of the latest deny/revoke, so a
	// concurrent grant (same lamport, later in tiebreak order) cannot win:
	// deny beats grant under concurrency.
	deniedAt map[string]uint64
	// contribAt maps principal → lamport of the earliest grant that included
	// contribute. Together with deniedAt it bounds the window in which the
	// principal's authored entries are ingestible (MayContribute).
	contribAt map[string]uint64
}

// Fold computes permission state from a thread's entries. Pass entries in
// any superset of the control lane; non-control entries are ignored. The
// entries slice must already be in total order (ThreadLog.Entries is).
func Fold(threadID string, entries []*protolog.Entry) *State {
	st := &State{
		Thread:      threadID,
		Stewards:    map[string]bool{},
		requests:    map[string]*AccessRequest{},
		grants:      map[string]ScopeSet{},
		expiry:      map[string]time.Time{},
		delegations: map[string]string{},
		deniedAt:    map[string]uint64{},
		contribAt:   map[string]uint64{},
	}
	for _, e := range entries {
		if e.Lane != protolog.LaneControl || e.Thread != threadID {
			continue
		}
		st.apply(e)
	}
	return st
}

// personSigned reports whether e is signed directly by a person: not on
// behalf of anyone, and not by a key that has been delegated as an agent.
// This is the protocol's "only humans grant" rule.
func (st *State) personSigned(e *protolog.Entry) bool {
	if e.OnBehalfOf != "" {
		return false
	}
	_, isAgent := st.delegations[e.Author]
	return !isAgent
}

func (st *State) apply(e *protolog.Entry) {
	switch e.Kind {
	case protolog.KindThread:
		var body struct {
			Stewards     []string `json:"stewards"`
			Title        string   `json:"title"`
			Discoverable bool     `json:"discoverable"`
		}
		if json.Unmarshal(e.Body, &body) == nil {
			for _, s := range body.Stewards {
				st.Stewards[s] = true
			}
			st.Title = body.Title
			st.Discoverable = body.Discoverable
		}
		if len(st.Stewards) == 0 {
			st.Stewards[e.Author] = true // creator is always a steward
		}

	case protolog.KindMembership:
		if !st.Stewards[e.Author] || !st.personSigned(e) {
			return
		}
		var body membershipBody
		if json.Unmarshal(e.Body, &body) != nil || body.Member == "" {
			return
		}
		switch body.Role {
		case "steward":
			st.Stewards[body.Member] = true
		case "removed":
			// A steward cannot remove the last steward (fail closed).
			if st.Stewards[body.Member] && len(st.Stewards) == 1 {
				return
			}
			delete(st.Stewards, body.Member)
			delete(st.grants, body.Member)
			delete(st.expiry, body.Member)
		}

	case protolog.KindDelegation:
		// A person binds (or revokes) an agent key to themselves. Only the
		// person signs their own delegations.
		if e.OnBehalfOf != "" {
			return
		}
		var body delegationBody
		if json.Unmarshal(e.Body, &body) != nil || body.Agent == "" {
			return
		}
		if body.Revoked {
			if st.delegations[body.Agent] == e.Author {
				delete(st.delegations, body.Agent)
			}
			return
		}
		st.delegations[body.Agent] = e.Author

	case protolog.KindAccessRequest:
		// Anyone may file a request for themselves (or their person, when a
		// delegated agent asks on their behalf).
		var body struct {
			Scopes []string `json:"scopes"`
			Reason string   `json:"reason"`
		}
		if json.Unmarshal(e.Body, &body) != nil {
			return
		}
		principal := e.Author
		if e.OnBehalfOf != "" {
			principal = e.OnBehalfOf
		}
		st.requests[principal] = &AccessRequest{
			EntryID: e.ID, Principal: principal, Scopes: body.Scopes,
			Reason: body.Reason, TS: e.TS, lamport: e.Lamport,
		}

	case protolog.KindGrant:
		if !st.Stewards[e.Author] || !st.personSigned(e) {
			return
		}
		var body grantBody
		if json.Unmarshal(e.Body, &body) != nil || body.Principal == "" {
			return
		}
		if e.Lamport <= st.deniedAt[body.Principal] {
			return // deny beats grant under concurrency
		}
		ss := ScopeSet{}
		for _, s := range body.Scopes {
			if ValidScope(Scope(s)) {
				ss[Scope(s)] = true
			}
		}
		if len(ss) == 0 {
			return
		}
		if r, ok := st.requests[body.Principal]; ok && r.lamport <= e.Lamport {
			delete(st.requests, body.Principal)
		}
		st.grants[body.Principal] = ss
		if ss.Has(ScopeContribute) {
			if at, ok := st.contribAt[body.Principal]; !ok || e.Lamport < at {
				st.contribAt[body.Principal] = e.Lamport
			}
		}
		if body.TTLSeconds > 0 {
			if ts, err := time.Parse(time.RFC3339, e.TS); err == nil {
				st.expiry[body.Principal] = ts.Add(time.Duration(body.TTLSeconds) * time.Second)
			}
		} else {
			delete(st.expiry, body.Principal)
		}

	case protolog.KindDeny, protolog.KindRevoke:
		if !st.Stewards[e.Author] || !st.personSigned(e) {
			return
		}
		var body grantBody
		if json.Unmarshal(e.Body, &body) != nil || body.Principal == "" {
			return
		}
		delete(st.grants, body.Principal)
		delete(st.expiry, body.Principal)
		if r, ok := st.requests[body.Principal]; ok && r.lamport <= e.Lamport {
			delete(st.requests, body.Principal)
		}
		if e.Lamport > st.deniedAt[body.Principal] {
			st.deniedAt[body.Principal] = e.Lamport
		}
	}
}

// PendingRequests returns unanswered access requests, oldest first.
func (st *State) PendingRequests() []*AccessRequest {
	var out []*AccessRequest
	for _, r := range st.requests {
		out = append(out, r)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].lamport < out[j-1].lamport; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// EffectiveScopes returns the scopes principal p holds at time now.
// Stewards implicitly hold every scope. An agent key holds the scopes of
// the person it is delegated to (severed instantly when the delegation is
// revoked, per the fold).
func (st *State) EffectiveScopes(p string, now time.Time) ScopeSet {
	if person, ok := st.delegations[p]; ok {
		p = person
	}
	if st.Stewards[p] {
		return ScopeSet{ScopeContribute: true, ScopeRead: true, ScopeSummary: true, ScopeSearch: true}
	}
	ss, ok := st.grants[p]
	if !ok {
		return ScopeSet{}
	}
	if exp, has := st.expiry[p]; has && now.After(exp) {
		return ScopeSet{}
	}
	out := ScopeSet{}
	for s := range ss {
		out[s] = true
	}
	return out
}

// Lanes is the one call sync and read paths use: which lanes may principal
// p replicate from this thread right now. Empty means none (deny).
func (st *State) Lanes(p string, now time.Time) []protolog.Lane {
	return st.EffectiveScopes(p, now).Lanes()
}

// MayContribute reports whether entries authored by principal p at the given
// lamport clock are ingestible: stewards always; granted principals within
// their [grant, revoke) lamport window. The window keeps a collaborator's
// pre-revocation history syncable to fresh replicas without letting them
// author anything new. Caveat (documented in the spec): lamport is
// author-asserted, so a revoked author could backdate; v1 accepts this in
// the honest-node threat model.
func (st *State) MayContribute(p string, lamport uint64) bool {
	if person, ok := st.delegations[p]; ok {
		p = person
	}
	if st.Stewards[p] {
		return true
	}
	grantAt, ok := st.contribAt[p]
	if !ok || lamport < grantAt {
		return false
	}
	if end, denied := st.deniedAt[p]; denied && end >= grantAt && lamport >= end {
		return false
	}
	// Also honor TTL expiry for *currently* held grants: if the principal
	// still holds contribute now the window is open; if the grant expired,
	// only history before the denial stands (expiry has no lamport, so an
	// expired-but-never-revoked grant keeps its window open in v1).
	return true
}

// ActsFor reports whether author is principal itself or an agent key
// currently delegated to principal.
func (st *State) ActsFor(author, principal string) bool {
	if author == principal {
		return true
	}
	return st.delegations[author] == principal
}

// IsAgent reports whether p is currently a delegated agent key in this
// thread, i.e. it acts for someone rather than being a person itself.
func (st *State) IsAgent(p string) bool {
	_, ok := st.delegations[p]
	return ok
}
