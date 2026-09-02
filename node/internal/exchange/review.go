package exchange

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/account"
)

// The buyer's say in whether the work was any good.
//
// Verification answers "was somebody there, and does the picture show what was
// asked". It cannot answer "was the gutter cleared properly", and the exchange
// says so plainly. That left the buyer paying automatically against a standard
// that was not the one they cared about, with no way to pause and nothing to
// press.
//
// Two controls, deliberately: release, which most buyers will use and which
// pays a good worker faster, and hold, which stops the money and asks a person
// to look. Refunds are not automatic — a buyer who could claw money back at
// will is a different kind of unfair, and the worker has already spent the
// afternoon.

func (s *Server) registerReview(mux *http.ServeMux) {
	// Either credential.
	//
	// The agent spends the money, but judging whether the gutter was actually
	// cleared is the person's call and they make it from the console, signed
	// in as themselves. Requiring an agent key here would have put the one
	// human decision in the system behind a machine credential.
	mux.HandleFunc("POST /v1/jobs/{job}/release", s.withBuyer(s.handleRelease))
	mux.HandleFunc("POST /v1/jobs/{job}/hold", s.withBuyer(s.handleHold))
}

// withBuyer accepts any credential that already spends this account's money.
//
// Three of them, and the third was missing. An agent key is software acting
// for somebody. A signed-in person is the human whose money it is. A signed
// principal is an integration holding its own keypair — which POST /v1/tasks
// has always accepted, and which every route added around it refused.
//
// That inconsistency made the whole buy-side surface unreachable by the buyer
// it was built for: a company could post a job with its own key and could not
// name the vendor to send it to, add the site it happens at, or ask what
// anything costs. Worse, the only way through was a personal email code, so a
// procurement manager's inbox became the root of trust for four hundred
// stores.
//
// This widens nothing. A signed principal could already spend on this account
// through /v1/tasks; refusing it on the routes that describe *how* to spend was
// a gap, not a control.
func (s *Server) withBuyer(
	next func(w http.ResponseWriter, r *http.Request, key *account.Key, person string, body []byte),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			writeError(w, http.StatusBadRequest, "could not read the request")
			return
		}
		if key, person, ok := s.agents.AuthenticateAgent(r); ok {
			next(w, r, key, person.ID, body)
			return
		}
		// A job token: the poster of a job that has no account behind it.
		// It authenticates as the owner of the job in the path and nothing
		// else, which is all a token for one job should be able to do.
		if person, ok := s.guestFromToken(r, r.PathValue("job")); ok {
			next(w, r, nil, person, body)
			return
		}
		if worker, err := s.Workers.Authenticate(r, body, s.now()); err == nil && worker.Verified {
			next(w, r, nil, worker.ID, body)
			return
		}
		// An integration signing with its own key, exactly as it would to post
		// a job. The principal is the account.
		if principal, err := api.AuthenticatePrincipal(r, body, s.now()); err == nil {
			next(w, r, nil, principal, body)
			return
		}
		writeError(w, http.StatusUnauthorized,
			"sign the request with your principal key, sign in, or present an "+
				"agent key issued from your account")
	}
}

func (s *Server) handleRelease(w http.ResponseWriter, r *http.Request, key *account.Key, person string, _ []byte) {
	job := r.PathValue("job")
	if _, ok := s.ownedBy(w, job, person); !ok {
		return
	}
	if s.Holdbacks == nil {
		writeError(w, http.StatusServiceUnavailable, "nothing is being held")
		return
	}
	n := s.Holdbacks.Release(job, s.now())
	s.notifyReleased(job)
	writeJSONResponse(w, map[string]any{
		"job": job, "released": n,
		"status": "the work is accepted and payment is clear to send",
	})
}

func (s *Server) handleHold(w http.ResponseWriter, r *http.Request, key *account.Key, person string, body []byte) {
	job := r.PathValue("job")
	if _, ok := s.ownedBy(w, job, person); !ok {
		return
	}
	var in struct {
		Ground string `json:"ground"`
		Reason string `json:"reason"`
	}
	json.Unmarshal(body, &in)
	// A ground, not a mood.
	//
	// This route used to take any sentence at all and freeze the worker's
	// money on the strength of it, forever. The deliverable was agreed before
	// the work started so that neither side could move it afterwards, and an
	// objection has to be about that rather than about how the buyer feels on
	// the day.
	if !ValidGround(in.Ground) {
		writeJSONResponse(w, map[string]any{
			"error": "an objection has to name what is wrong with the work " +
				"against what was agreed",
			"grounds": Grounds(),
			"note": "if the deliverable was met and you simply want something " +
				"more, that is a new job rather than a reason not to pay for " +
				"this one",
		})
		return
	}
	if strings.TrimSpace(in.Reason) == "" {
		writeError(w, http.StatusBadRequest,
			"say what is wrong in your own words as well; a panel has to read it")
		return
	}
	if s.Holdbacks == nil {
		writeError(w, http.StatusServiceUnavailable, "nothing is being held")
		return
	}
	until := s.now().Add(DisputeWindow)
	reason := GroundLabel(in.Ground) + ": " + in.Reason
	n := s.Holdbacks.Hold(job, reason, until)
	s.notifyHeld(job, reason, until)
	if n == 0 {
		// Either nothing settled yet, or it has already been paid. Those are
		// different situations and telling them apart matters.
		writeJSONResponse(w, map[string]any{
			"job": job, "held": 0,
			"status": "nothing on this job is still holdable",
			"note": "if payment has already been sent, write to support@lamdis.ai — " +
				"a hold cannot recall a transfer that has left",
		})
		return
	}
	writeJSONResponse(w, map[string]any{
		"job": job, "held": n,
		"status":     "payment is frozen while this is decided",
		"decided_by": until.Format(time.RFC3339),
		"decided_by_whom": "a panel of people who are shown what was agreed and " +
			"what was submitted, and who are not you and not us",
		"if_not_decided": "if it is not decided by then the money goes to the " +
			"worker. An objection nobody carries through is not a finding, and " +
			"they should not fund the wait",
		"note": "write to support@lamdis.ai with the job id if you do not hear back",
	})
}

// reviewStateFor describes where a job stands for its buyer: whether anything
// is waiting on them, and how long they have.
func (s *Server) reviewStateFor(job string) map[string]any {
	if s.Holdbacks == nil {
		return nil
	}
	entries := s.Holdbacks.ForJob(job)
	if len(entries) == 0 {
		return nil
	}
	now := s.now()
	out := map[string]any{}
	var pending, held, paid int64
	var soonest time.Time
	for _, e := range entries {
		switch {
		case e.Paid:
			paid += e.AmountMinor
		case e.Held:
			held += e.AmountMinor
		default:
			pending += e.AmountMinor
			if soonest.IsZero() || e.ReleaseAt.Before(soonest) {
				soonest = e.ReleaseAt
			}
		}
	}
	if pending > 0 {
		out["awaiting_release_minor"] = pending
		out["releases_at"] = soonest.Format(time.RFC3339)
		out["you_can"] = []string{"release", "hold"}
		// The number people actually want: how long they have left.
		if mins := int(soonest.Sub(now).Minutes()); mins > 0 {
			out["hours_left"] = float64(mins) / 60
		}
	}
	if held > 0 {
		out["held_minor"] = held
	}
	if paid > 0 {
		out["paid_minor"] = paid
	}
	return out
}
