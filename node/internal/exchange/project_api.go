package exchange

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/account"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/ledger"
)

func (s *Server) registerProjects(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/projects", s.withBuyer(s.handleOpenProject))
	mux.HandleFunc("GET /v1/projects", s.withBuyer(s.handleListProjects))
	mux.HandleFunc("GET /v1/projects/{project}", s.withBuyer(s.handleProjectState))
	mux.HandleFunc("POST /v1/projects/{project}/close", s.withBuyer(s.handleCloseProject))
	mux.HandleFunc("POST /v1/jobs/{job}/cancel", s.withBuyer(s.handleCancelJob))
}

func (s *Server) handleOpenProject(w http.ResponseWriter, r *http.Request, key *account.Key, person string, body []byte) {
	var in struct {
		Title       string `json:"title"`
		BudgetMinor int64  `json:"budget_minor"`
		Currency    string `json:"currency"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request")
		return
	}
	if in.Currency == "" {
		in.Currency = "USD"
	}
	id := fmt.Sprintf("proj-%d", s.now().UnixNano())
	pr, err := s.Projects.Open(id, person, in.Title, in.BudgetMinor, in.Currency)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSONResponse(w, map[string]any{
		"project": pr,
		"note": "attach jobs with project_id. Committing more than the budget " +
			"is refused with the amount remaining, rather than failing as an " +
			"escrow error part-way through a plan.",
	})
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request, key *account.Key, person string, _ []byte) {
	writeJSONResponse(w, map[string]any{"projects": s.Projects.List(person)})
}

// handleProjectState is the question an orchestrator asks constantly: what has
// this cost so far, and what is left.
func (s *Server) handleProjectState(w http.ResponseWriter, r *http.Request, key *account.Key, person string, _ []byte) {
	id := r.PathValue("project")
	pr, ok := s.Projects.Get(id, person)
	if !ok {
		writeError(w, http.StatusNotFound, "no such project")
		return
	}
	st := s.projectState(r, pr)
	writeJSONResponse(w, st)
}

func (s *Server) projectState(r *http.Request, pr *api.Project) api.ProjectState {
	st := api.ProjectState{Project: *pr}
	for _, job := range s.Projects.JobsIn(pr.ID) {
		l, ok := s.Board.Get(job)
		if !ok {
			continue
		}
		committed := MaxPayoutFor(l)
		pj := api.ProjectJob{
			Job: job, Kind: l.Kind, Title: l.Title,
			CommittedMinor: committed, Posted: l.Posted,
		}
		subs := s.Submissions(job)
		var earned int64
		for _, sub := range subs {
			earned += earnedFor(l, sub)
		}
		switch {
		case earned > 0:
			pj.Status = "done"
			st.SpentMinor += earned
			st.ReleasedMinor += committed - earned
		case l.Cancelled:
			pj.Status = "cancelled"
			st.ReleasedMinor += committed
		case l.Expires.Before(s.now()):
			pj.Status = "expired, refunding"
			st.ReleasedMinor += committed
		default:
			pj.Status = "live"
			st.CommittedMinor += committed
		}
		st.Jobs = append(st.Jobs, pj)
	}
	if pr.BudgetMinor > 0 {
		st.RemainingMinor = pr.BudgetMinor - st.CommittedMinor - st.SpentMinor
		if st.RemainingMinor < 0 {
			st.RemainingMinor = 0
		}
	}
	if st.Jobs == nil {
		st.Jobs = []api.ProjectJob{}
	}
	return st
}

func (s *Server) handleCloseProject(w http.ResponseWriter, r *http.Request, key *account.Key, person string, _ []byte) {
	if err := s.Projects.Close(r.PathValue("project"), person); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSONResponse(w, map[string]any{"closed": true,
		"note": "jobs already live run to completion"})
}

// handleCancelJob takes work back and releases the money.
//
// Without this, escrow committed to a job stayed committed until it expired —
// even when the survey that job informed had just proved it pointless. An
// agent replanning had to either over-commit early and strand funds, or post
// one job at a time and pay for it in latency. Replanning is the whole point
// of an agent orchestrating, and it was the one thing the money could not
// express.
func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request, key *account.Key, person string, body []byte) {
	job := r.PathValue("job")
	l, ok := s.ownedBy(w, job, person)
	if !ok {
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	json.Unmarshal(body, &in)

	// Work already done is not cancellable. Somebody spent their afternoon on
	// it, and taking the money back because the plan changed is the engage-
	// then-ghost pattern this exchange exists to not be.
	for _, sub := range s.Submissions(job) {
		if sub.Verified {
			writeError(w, http.StatusConflict,
				"somebody has already done this and been accepted; it cannot be "+
					"cancelled. Use hold if there is a problem with the work.")
			return
		}
	}
	held := s.Board.HoldersOf(job)
	if len(held) > 0 {
		// Somebody is mid-job. They keep the attempt fee for having travelled,
		// which is the difference between cancelling a plan and stiffing a
		// person who is standing at the address.
		writeJSONResponse(w, map[string]any{
			"job": job, "cancelled": false,
			"status": fmt.Sprintf("%d person is holding this right now", len(held)),
			"note": "wait for them to finish or hand it back. Cancelling out " +
				"from under somebody at the address is how a marketplace stops " +
				"having anybody at addresses.",
		})
		return
	}
	if err := s.Board.Cancel(job); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	var released int64
	if s.Ledger != nil {
		released = MaxPayoutFor(l)
		if _, err := s.Ledger.Release(r.Context(), "cancel-"+job, job, person,
			released, l.Currency); err != nil {
			// The listing is already withdrawn; the money follows on the sweep.
			released = 0
		} else {
			// Nothing was paid out, so a card behind this job is released
			// in full and never charged.
			s.settleCard(r.Context(), l, released)
		}
	}
	writeJSONResponse(w, map[string]any{
		"job": job, "cancelled": true, "released_minor": released,
		"currency": l.Currency,
	})
}

var _ = ledger.PayableOf
var _ = time.Now
