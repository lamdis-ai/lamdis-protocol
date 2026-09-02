package api

import (
	"encoding/json"
	"net/http"
	"time"
)

// The supply side's view of a project.
//
// Every project route was withBuyer — five of five — so a supplier could not
// read a project, could not price one, and could not confirm one existed. The
// only question they could be asked was "what is your price for this one?",
// three times, about three jobs they had no way of knowing were the same
// property.
//
// These routes are the other half. They authenticate as a worker, they never
// return the budget or the owner, and they let one offer cover a whole scope.

func (s *WorkerServer) registerScope(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/scope/{project}", s.handleScope)
	mux.HandleFunc("POST /v1/scope/{project}/bid", s.handleScopeBid)
	mux.HandleFunc("POST /v1/workers/plan/{job}", s.handlePlan)
	mux.HandleFunc("GET /v1/board/{job}", s.handleReadJob)
	// The same job as a page, for a link to land on. See job_html.go.
	mux.HandleFunc("GET /j/{job}", s.handleJobPage)
}

// handleScope returns a project as a supplier may see it.
//
// Signed in, but no ownership check: this is the supply side reading published
// work. What keeps it safe is Public() on every listing plus a brief that names
// no budget — the same redaction the open board already relies on.
func (s *WorkerServer) handleScope(w http.ResponseWriter, r *http.Request) {
	body, _ := readBody(r)
	if _, err := s.Workers.Authenticate(r, body, s.now()); err != nil {
		refuse(w)
		return
	}
	scope := s.Board.Scope(r.PathValue("project"))
	if scope == nil {
		writeWork(w, http.StatusNotFound, map[string]string{"error": "no such project"})
		return
	}
	writeWork(w, http.StatusOK, scope)
}

// handleScopeBid takes one offer covering several jobs.
func (s *WorkerServer) handleScopeBid(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		refuse(w)
		return
	}
	worker, err := s.Workers.Authenticate(r, body, s.now())
	if err != nil {
		refuse(w)
		return
	}
	var in struct {
		Lines       []BidLine    `json:"lines"`
		Note        string       `json:"note,omitempty"`
		Assumptions []Assumption `json:"assumptions,omitempty"`
		// AllOrNothing defaults true when omitted, which is why it is a
		// pointer. A contractor who priced one mobilisation across three jobs
		// and did not think about this field wants all three or none, and the
		// default has to be the answer that does not ruin them.
		AllOrNothing  *bool  `json:"all_or_nothing,omitempty"`
		AvailableFrom string `json:"available_from,omitempty"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		writeWork(w, http.StatusBadRequest, map[string]string{"error": "malformed request"})
		return
	}
	if len(in.Lines) == 0 {
		writeWork(w, http.StatusBadRequest,
			map[string]string{"error": "price each piece of the scope"})
		return
	}
	allOrNothing := true
	if in.AllOrNothing != nil {
		allOrNothing = *in.AllOrNothing
	}
	var from time.Time
	if in.AvailableFrom != "" {
		from, _ = time.Parse("2006-01-02", in.AvailableFrom)
	}
	bid, err := s.Board.PlaceProjectBid(r.PathValue("project"), worker.ID,
		in.Lines, "", in.Note, from, allOrNothing, in.Assumptions...)
	if err != nil {
		writeWork(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeWork(w, http.StatusOK, map[string]any{
		"placed": true, "total_minor": bid.TotalMinor, "currency": bid.Currency,
		"all_or_nothing": bid.AllOrNothing,
		"note": "one offer, awarded together or not at all; " +
			"you can revise it until bidding closes",
		"payable": worker.Payable(),
	})
}

// handlePlan takes a supplier's breakdown of a job they won.
func (s *WorkerServer) handlePlan(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		refuse(w)
		return
	}
	worker, err := s.Workers.Authenticate(r, body, s.now())
	if err != nil {
		refuse(w)
		return
	}
	var in struct {
		Stages []Stage `json:"stages"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		writeWork(w, http.StatusBadRequest, map[string]string{"error": "malformed request"})
		return
	}
	job := r.PathValue("job")
	if err := s.Board.ProposeStages(job, worker.ID, in.Stages); err != nil {
		writeWork(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeWork(w, http.StatusOK, map[string]any{
		"proposed": true, "job": job, "stages": len(in.Stages),
		"note": "the buyer sees your plan now; work can start once they accept it",
	})
}

// handleReadJob returns one job as an operator may see it.
//
// The board lists what is open; this answers a question about one job in
// particular, which is what somebody has after a dispatch offer arrives naming
// a job id. Without it a pushed offer was a dead end: the agent knew the id and
// had no way to look it up, so the only route to detail was scanning the whole
// board and hoping it was still on it.
//
// Public, like the board it reads from. It used to require a sign-in, which
// made no sense next to GET /v1/board handing the same listing to anybody: a
// dispatch email or a shared link landed on a 401 for the one person it was
// written for. Public() is the redaction that keeps the open board safe, and
// it is the same redaction here. Signing in adds one thing — how far away the
// job is — because that is a fact about the reader, not the listing.
func (s *WorkerServer) handleReadJob(w http.ResponseWriter, r *http.Request) {
	l, ok := s.Board.Get(r.PathValue("job"))
	if !ok || l.Directed() {
		// Directed work belongs to the vendor it was sent to and is not on the
		// open board, so it is not something to look up either.
		writeWork(w, http.StatusNotFound, map[string]string{"error": "no such job"})
		return
	}
	pub := l.Public()
	pub.BlockedBy = s.Board.Blocked(l.Job)
	if l.ProjectID != "" {
		pub.Project = s.Board.BriefFor(l.Job)
	}
	if s.Workers != nil && s.Board.Capacities != nil && HasPosition(l.LatE7, l.LonE7) {
		body, _ := readBody(r)
		if worker, err := s.Workers.Authenticate(r, body, s.now()); err == nil {
			if cap := s.Board.Capacities.Get(worker.ID); cap.Positioned() {
				pub.DistanceMiles = round1(MilesBetween(l.LatE7, l.LonE7, cap.LatE7, cap.LonE7))
			}
		}
	}
	writeWork(w, http.StatusOK, pub)
}
