package exchange

import (
	"encoding/json"
	"net/http"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/account"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
)

// The buyer's half of multi-part work: reading the offers that cover a whole
// scope, and accepting one.

func (s *Server) registerScopeBuyer(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/projects/{project}/bids", s.withBuyer(s.handleListScopeBids))
	mux.HandleFunc("POST /v1/projects/{project}/award", s.withBuyer(s.handleAwardScope))
	mux.HandleFunc("GET /v1/jobs/{job}/plan", s.withBuyer(s.handleReadPlan))
	mux.HandleFunc("POST /v1/jobs/{job}/plan", s.withBuyer(s.handlePlanDecision))
}

// handleListScopeBids shows what has been offered on the whole scope.
func (s *Server) handleListScopeBids(w http.ResponseWriter, r *http.Request, key *account.Key, person string, body []byte) {
	project := r.PathValue("project")
	if _, ok := s.Projects.Get(project, person); !ok {
		// The same 404 a stranger gets. A different answer here would let
		// anybody enumerate whose projects exist.
		writeError(w, http.StatusNotFound, "no such project")
		return
	}
	bids := s.Board.ProjectBids(project)
	out := make([]map[string]any, 0, len(bids))
	for _, b := range bids {
		out = append(out, map[string]any{
			"bid": b.ID, "total_minor": b.TotalMinor, "currency": b.Currency,
			"lines": b.Lines, "note": b.Note,
			"all_or_nothing": b.AllOrNothing,
			"available_from": b.AvailableFrom,
		})
	}
	writeJSONResponse(w, map[string]any{"project": project, "bids": out})
}

// handleAwardScope accepts one offer covering several jobs.
//
// Every line is escrowed before any line is awarded. A bundle that half-lands
// is worse than one that does not land at all: the contractor priced arriving
// once, and awarding two pieces of three hands them work at a price that only
// made sense with the third.
func (s *Server) handleAwardScope(w http.ResponseWriter, r *http.Request, key *account.Key, person string, body []byte) {
	var in struct {
		Bid string `json:"bid"`
	}
	if err := json.Unmarshal(body, &in); err != nil || in.Bid == "" {
		writeError(w, http.StatusBadRequest, "name the offer to accept")
		return
	}
	project := r.PathValue("project")
	if _, ok := s.Projects.Get(project, person); !ok {
		writeError(w, http.StatusNotFound, "no such project")
		return
	}
	var won *api.ProjectBid
	for _, b := range s.Board.ProjectBids(project) {
		if b.ID == in.Bid {
			won = b
			break
		}
	}
	if won == nil {
		writeError(w, http.StatusNotFound, "no such offer on this project")
		return
	}
	// Hold every line first. Holds are idempotent on their key, so a failure
	// part-way leaves money reserved rather than spent, and the sweeper returns
	// whatever never became a job.
	if s.Ledger != nil {
		for _, ln := range won.Lines {
			if _, err := s.Ledger.Hold(r.Context(), "hold-"+ln.Job, ln.Job, person,
				ln.AmountMinor, won.Currency); err != nil {
				writeError(w, http.StatusPaymentRequired, err.Error())
				return
			}
		}
	}
	if _, err := s.Board.AwardProject(project, in.Bid, s.checkFunded); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	for _, ln := range won.Lines {
		s.Reservations.Release(ln.Job)
	}
	s.notifyScopeAwarded(project, won)
	writeJSONResponse(w, map[string]any{
		"project": project, "awarded_to": won.Worker,
		"total_minor": won.TotalMinor, "currency": won.Currency,
		"jobs": len(won.Lines),
		"note": "every piece is awarded to one supplier and escrowed at its line amount",
	})
}

// handlePlanDecision is the buyer accepting or sending back a supplier's
// breakdown of a job.
func (s *Server) handlePlanDecision(w http.ResponseWriter, r *http.Request, key *account.Key, person string, body []byte) {
	var in struct {
		Accept bool   `json:"accept"`
		Why    string `json:"why,omitempty"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request")
		return
	}
	job := r.PathValue("job")
	if _, ok := s.ownedBy(w, job, person); !ok {
		return
	}
	if in.Accept {
		if err := s.Board.AcceptPlan(job); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		l, _ := s.Board.Get(job)
		writeJSONResponse(w, map[string]any{
			"job": job, "plan": "accepted", "stages": len(l.Stages),
			"note": "the crew can start; each stage is paid when its evidence is accepted",
		})
		return
	}
	if in.Why == "" {
		writeError(w, http.StatusBadRequest,
			"say why, so the supplier can send back something you would accept")
		return
	}
	if err := s.Board.RejectPlan(job, in.Why); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSONResponse(w, map[string]any{"job": job, "plan": "returned"})
}

// handleReadPlan shows the buyer what their supplier proposed.
func (s *Server) handleReadPlan(w http.ResponseWriter, r *http.Request, key *account.Key, person string, body []byte) {
	job := r.PathValue("job")
	l, ok := s.ownedBy(w, job, person)
	if !ok {
		return
	}
	writeJSONResponse(w, map[string]any{
		"job": job, "plan_by": l.PlanBy, "plan_state": l.PlanState,
		"proposed_stages": l.ProposedStages, "stages": l.Stages,
		"plan_note": l.PlanNote,
	})
}
