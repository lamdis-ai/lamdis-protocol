package exchange

import (
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/account"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
)

// Asking before committing.
//
// There was no way to learn what work would cost, or whether anybody within
// range could do it, without posting the job and escrowing the money. So an
// agent's only instrument for finding out was spending its person's funds: it
// posted into a void and discovered by waiting.
//
// That is survivable for a twelve dollar errand and hopeless for the thing
// this exchange is supposed to be for. Somebody who says "do something with
// the front yard, under six thousand" needs their agent to work out what is
// possible before any of the six thousand moves, and an agent cannot plan
// against an exchange that only answers questions in money.
//
// A quote answers three things and holds nothing: is there anybody, what has
// work like this cost, and would you refuse it.

// QuoteRequest is a job an agent is considering.
type QuoteRequest struct {
	Kind   string      `json:"kind"`
	Skills []api.Skill `json:"skills,omitempty"`
	Lat    float64     `json:"lat,omitempty"`
	Lon    float64     `json:"lon,omitempty"`
	Slots  int         `json:"slots,omitempty"`
	Tier   string      `json:"tier,omitempty"`
	// The words, so screening can answer before anything is posted rather than
	// refusing a job the agent has already told somebody it placed.
	Predicate    string `json:"predicate,omitempty"`
	Detail       string `json:"detail,omitempty"`
	Instructions string `json:"instructions,omitempty"`
	// Sandbox asks the question of the fulfilment sandbox rather than of the
	// world. It is feasible by construction there, which is the point: an
	// integration can be finished today at an address where nothing can
	// actually be dispatched. See internal/exchange/sandbox.go.
	Sandbox bool `json:"sandbox,omitempty"`
}

// Quote is what the exchange can say without being paid.
type Quote struct {
	// Reachable buckets how many operators could take this. Bucketed rather
	// than exact: a precise count of who is where, queryable for free, is a
	// map of this marketplace's supply for anyone who wants to compete with it
	// or target the people in it.
	Reachable string `json:"reachable"`
	// Feasible is false when nobody at all could take the work as described.
	Feasible bool   `json:"feasible"`
	Why      string `json:"why,omitempty"`

	// Settled is what work of this shape has actually been paid here, when
	// enough of it has. Not an estimate and not advice: this exchange has no
	// idea what a driveway costs, and the number varies by yard, by region, by
	// season and by what is under the old surface. It is history, offered as
	// history, and absent when there is too little of it to mean anything.
	//
	// The bidding round is the price discovery mechanism. This is context for
	// setting a ceiling, not a substitute for asking.
	Settled *PriceBand `json:"settled_here,omitempty"`

	// Refused reports that this job would not be listed, and why — answered
	// now rather than after the agent has committed to a plan around it.
	Refused     bool   `json:"refused,omitempty"`
	RefusedWhy  string `json:"refused_why,omitempty"`
	NeedsReview bool   `json:"needs_review,omitempty"`

	// Advice is what would make the job more likely to be taken.
	Advice []string `json:"advice,omitempty"`

	// Sandbox says this answer is about the sandbox and not about the world.
	// Present on every sandbox reply and absent from every live one, so an
	// agent cannot read a feasible answer without also reading what it was
	// feasible in.
	Sandbox bool `json:"sandbox,omitempty"`
	// Recorded says this request was filed against the demand register: coarse
	// point, kind, skills, timestamp, nothing else. Only ever set on a live
	// answer that found nothing.
	Recorded bool `json:"recorded,omitempty"`
}

// PriceBand is what similar work has actually been paid here.
//
// Deliberately not called an estimate. The exchange publishes what it has seen
// and nothing more: a figure invented with authority is worse than no figure,
// and the sealed-bid round exists precisely because the people doing the work
// are the ones who know.
type PriceBand struct {
	LowMinor    int64  `json:"low_minor"`
	MedianMinor int64  `json:"median_minor"`
	HighMinor   int64  `json:"high_minor"`
	Currency    string `json:"currency"`
	BasedOn     int    `json:"based_on"`
}

func (s *Server) registerQuote(mux *http.ServeMux) {
	// Public. A quote reads nothing that belongs to anybody and moves no
	// money; it is the question an agent has to be able to ask before it
	// promises a person anything, and the guest tools ask it with no
	// credential at all.
	mux.HandleFunc("POST /v1/quote", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			writeError(w, http.StatusBadRequest, "could not read the request")
			return
		}
		s.handleQuote(w, r, nil, "", body)
	})
}

func (s *Server) handleQuote(w http.ResponseWriter, r *http.Request, key *account.Key, person string, body []byte) {
	var in QuoteRequest
	if err := json.Unmarshal(body, &in); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request")
		return
	}
	if in.Kind == "" {
		in.Kind = api.KindDo
	}
	if in.Slots < 1 {
		in.Slots = 1
	}
	q := Quote{}

	// Would we even carry it. Cheapest question, asked first, and asked of a
	// sandbox job too: what the exchange refuses to list, it refuses to
	// pretend to list.
	if ref := api.Screen(in.Predicate, in.Detail, in.Instructions); ref != nil {
		q.Refused, q.RefusedWhy, q.NeedsReview = true, ref.Why, ref.Review
	}

	// The sandbox answers for itself. There is exactly one operator there and
	// it is not a person, so the honest bucket is a word that cannot be
	// confused with real supply: "simulated" is not on the none/a few/several/
	// plenty ladder on purpose.
	if in.Sandbox {
		q.Sandbox = true
		q.Reachable = "simulated"
		q.Feasible = !q.Refused
		q.Why = "this is the sandbox. Supply here is a simulated operator that " +
			"takes the job, submits generated evidence and settles it in " +
			"seconds. Nobody is dispatched, nothing is photographed and no " +
			"money moves. It says nothing about whether this work can be done " +
			"at this address — ask again without sandbox for that."
		writeJSONResponse(w, q)
		return
	}

	// Who could take it.
	reach, advice := s.reachFor(in)
	q.Reachable = bucket(reach)
	q.Feasible = reach > 0 && !q.Refused
	q.Advice = advice
	if reach == 0 {
		q.Why = "nobody within range is set up for this work right now. That is " +
			"not permanent — operators change what they take — but posting it " +
			"today would most likely sit unclaimed."
		// The honest answer, and then somewhere to go with it.
		//
		// "No" on its own is where every developer evaluating this exchange
		// stopped, because coverage is thin everywhere and "none" was the
		// first and last thing they were told. Neither of these sentences
		// softens the no: one says the integration can be finished today
		// against the sandbox, and the other says the question was written
		// down where it is used to decide whom to recruit.
		q.Advice = append(q.Advice,
			"the fulfilment sandbox is feasible everywhere and settles in "+
				"seconds, so the integration can be finished today: ask again "+
				"with sandbox true, then post with sandbox true. Nothing about "+
				"it is real and every reply says so.")
		if !q.Refused && s.Demand != nil {
			q.Recorded = s.Demand.Record(callerIP(r), Demand{
				LatE7: api.E7(in.Lat), LonE7: api.E7(in.Lon),
				Kind: in.Kind, Skills: in.Skills,
			})
		}
		if q.Recorded {
			q.Advice = append(q.Advice,
				"this request has been recorded against the demand register — "+
					"the coarse area, the kind and the skills, nothing else — so "+
					"supply is recruited where it is being asked for. It is "+
					"public at GET /v1/demand.")
		}
	}

	// What it has cost before.
	if band := s.priceBandFor(in); band != nil {
		q.Settled = band
	} else {
		q.Advice = append(q.Advice,
			"no history for work of this shape here yet, so there is no price to "+
				"quote. Post it for bids with a ceiling you are willing to pay — "+
				"the people who do the work know what it costs and this exchange "+
				"does not.")
	}
	writeJSONResponse(w, q)
}

// reachFor counts operators who could take this, and says what is narrowing it.
func (s *Server) reachFor(in QuoteRequest) (int, []string) {
	if s.Capacities == nil {
		return 0, nil
	}
	var reach, blockedBySkill, blockedByRange, notAccepting int
	for worker, cap := range s.Capacities.All() {
		if !cap.Accepting {
			notAccepting++
			continue
		}
		if !cap.Takes(in.Kind) {
			continue
		}
		if !api.MeetsSkills(in.Skills, cap.Skills) {
			blockedBySkill++
			continue
		}
		if api.HasPosition(api.E7(in.Lat), api.E7(in.Lon)) && cap.Positioned() &&
			!api.InRange(api.E7(in.Lat), api.E7(in.Lon), cap.LatE7, cap.LonE7, cap.RangeMiles) {
			blockedByRange++
			continue
		}
		// A licensed trade needs a licence somebody checked, so somebody who
		// merely ticked the box is not reachable supply.
		if s.Suppliers != nil {
			unlicensed := false
			for _, sk := range in.Skills {
				if api.Licensed(sk) && !s.Suppliers.HoldsLicence(worker, sk, s.now()) {
					unlicensed = true
					break
				}
			}
			if unlicensed {
				blockedBySkill++
				continue
			}
		}
		reach++
	}

	// Say what is narrowing it, so the agent can change the plan rather than
	// guess at why nothing happened.
	var advice []string
	if blockedByRange > 0 && reach == 0 {
		advice = append(advice,
			"operators exist for this work but none within travelling distance; "+
				"paying more or allowing longer may widen it")
	}
	if blockedBySkill > 0 && reach == 0 {
		advice = append(advice,
			"the qualifications asked for are the binding constraint here")
	}
	if notAccepting > 0 && reach == 0 {
		advice = append(advice,
			"some operators who could do this have paused taking work")
	}
	return reach, advice
}

// bucket reports supply coarsely.
//
// Exact counts would let anybody map this marketplace's supply for free, which
// is useful to a competitor and to somebody deciding where a sybil fleet would
// go unnoticed. An agent planning a job needs to know whether the answer is
// none, few or plenty; it does not need the number.
func bucket(n int) string {
	switch {
	case n == 0:
		return "none"
	case n < 3:
		return "a few"
	case n < 10:
		return "several"
	default:
		return "plenty"
	}
}

// priceBandFor is what comparable work has actually settled at.
//
// Built from settled jobs rather than from listed prices: what a buyer hoped
// to pay is not evidence of what the work costs. Absent when there is too
// little history, because a median of two is a number that misleads with more
// authority than no number at all.
func (s *Server) priceBandFor(in QuoteRequest) *PriceBand {
	const enough = 5
	var paid []int64
	currency := "USD"
	for _, l := range s.Board.All() {
		if l.Kind != in.Kind {
			continue
		}
		// Sandbox and practice jobs settle at a figure nobody paid. Left in,
		// a developer running the sandbox in a loop would move the public
		// price band for everybody, which is the sort of number that misleads
		// with authority — exactly what this function refuses to produce.
		if l.Sandbox || l.Practice {
			continue
		}
		if !api.MeetsSkills(in.Skills, l.Skills) && !api.MeetsSkills(l.Skills, in.Skills) {
			continue
		}
		for _, sub := range s.Submissions(l.Job) {
			if !sub.Verified || !sub.Finding {
				continue
			}
			if got := earnedFor(l, sub); got > 0 {
				paid = append(paid, got)
				currency = l.Currency
			}
		}
	}
	if len(paid) < enough {
		return nil
	}
	sort.Slice(paid, func(i, j int) bool { return paid[i] < paid[j] })
	return &PriceBand{
		LowMinor:    paid[len(paid)/10],
		MedianMinor: paid[len(paid)/2],
		HighMinor:   paid[len(paid)*9/10],
		Currency:    currency,
		BasedOn:     len(paid),
	}
}

var _ = time.Now
