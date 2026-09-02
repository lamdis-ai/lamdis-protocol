package exchange

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/account"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
)

func (s *Server) registerBook(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/vendors", s.withBuyer(s.handleListVendors))
	mux.HandleFunc("PUT /v1/vendors", s.withBuyer(s.handleApproveVendor))
	mux.HandleFunc("DELETE /v1/vendors/{supplier}", s.withBuyer(s.handleRevokeVendor))
	mux.HandleFunc("GET /v1/sites", s.withBuyer(s.handleListSites))
	mux.HandleFunc("PUT /v1/sites", s.withBuyer(s.handlePutSite))
	mux.HandleFunc("POST /v1/tasks/sweep", s.withBuyer(s.handleSiteSweep))
}

func (s *Server) handleListVendors(w http.ResponseWriter, r *http.Request, key *account.Key, person string, _ []byte) {
	writeJSONResponse(w, map[string]any{"vendors": s.Book.Vendors(person)})
}

func (s *Server) handleApproveVendor(w http.ResponseWriter, r *http.Request, key *account.Key, person string, body []byte) {
	var in api.Vendor
	if err := json.Unmarshal(body, &in); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request")
		return
	}
	v, err := s.Book.Approve(person, in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSONResponse(w, map[string]any{
		"vendor": v,
		"note": "work directed to this vendor never reaches the open board and " +
			"runs no auction. Rates are yours, agreed elsewhere; the exchange " +
			"carries them and does not interpret them.",
	})
}

func (s *Server) handleRevokeVendor(w http.ResponseWriter, r *http.Request, key *account.Key, person string, _ []byte) {
	if err := s.Book.Revoke(person, r.PathValue("supplier")); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSONResponse(w, map[string]any{
		"revoked": true,
		"note": "work already running is untouched. Withdrawing approval stops " +
			"new work reaching them, not work somebody is standing at an " +
			"address doing.",
	})
}

func (s *Server) handleListSites(w http.ResponseWriter, r *http.Request, key *account.Key, person string, _ []byte) {
	writeJSONResponse(w, map[string]any{"sites": s.Book.Sites(person)})
}

func (s *Server) handlePutSite(w http.ResponseWriter, r *http.Request, key *account.Key, person string, body []byte) {
	var in api.Site
	if err := json.Unmarshal(body, &in); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request")
		return
	}
	site, err := s.Book.PutSite(person, in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSONResponse(w, map[string]any{"site": site})
}

// handleSiteSweep describes work once and points it at many locations.
//
// The shape a company with four hundred stores actually has: the same check
// everywhere, monthly, each one evidenced separately and all of it reconciling
// to one purchase order. Posting that job by job meant four hundred chances to
// mistype an address and no way to see the sweep as one thing.
func (s *Server) handleSiteSweep(w http.ResponseWriter, r *http.Request, key *account.Key, person string, body []byte) {
	var in struct {
		CreateTaskRequest
		Sites []string `json:"sites"`
		// Title for the project the sweep creates, so it reads like the thing
		// somebody asked for rather than a list of job ids.
		Sweep string `json:"sweep"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request")
		return
	}
	if len(in.Sites) == 0 {
		writeError(w, http.StatusBadRequest, "name the sites to sweep")
		return
	}
	if len(in.Sites) > MaxSweep {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("a sweep covers at most %d sites at once", MaxSweep))
		return
	}

	// One envelope for the whole sweep, so the cost of "every store" is a
	// number rather than an exercise in addition.
	title := in.Sweep
	if title == "" {
		title = in.Predicate
	}
	projectID := fmt.Sprintf("sweep-%d", s.now().UnixNano())
	per := in.FeeMinor + in.BonusMinor + in.ExpenseCapMinor
	if _, err := s.Projects.Open(projectID, person, title,
		per*int64(len(in.Sites)), orDefault(in.Currency, "USD")); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	var made []map[string]any
	var failed []map[string]any
	for _, id := range in.Sites {
		site, ok := s.Book.Site(person, id)
		if !ok || site.Retired {
			failed = append(failed, map[string]any{"site": id, "why": "no such live site"})
			continue
		}
		job, err := s.postOne(r, person, in.CreateTaskRequest, site, projectID)
		if err != nil {
			failed = append(failed, map[string]any{"site": id, "why": err.Error()})
			continue
		}
		made = append(made, map[string]any{"site": id, "job": job})
	}
	if made == nil {
		made = []map[string]any{}
	}
	writeJSONResponse(w, map[string]any{
		"sweep": projectID, "posted": made, "failed": failed,
		"status": s.BaseURL + "/v1/projects/" + projectID,
	})
}

// MaxSweep bounds one fan-out. Large enough for a national estate, small
// enough that a mistake is not four thousand escrows.
const MaxSweep = 500

// postOne places a single job at one site, for a sweep.
//
// Deliberately narrow: a sweep is fixed-price directed or open work repeated
// across locations, so it does not carry the bidding, staging or quote paths.
// Anything more elaborate is posted one job at a time, where the caller can
// see each answer.
func (s *Server) postOne(r *http.Request, principal string, in CreateTaskRequest,
	site *api.Site, projectID string) (string, error) {

	kind := in.Kind
	if kind == "" {
		kind = api.KindObserve
	}
	if in.Slots < 1 {
		in.Slots = 1
	}
	if in.Currency == "" {
		in.Currency = "USD"
	}
	if in.Tier == "" {
		in.Tier = "V2"
	}
	ttl := time.Duration(in.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	job := fmt.Sprintf("%s-%s-%d", kind, site.ID, s.now().UnixNano())

	l := &api.Listing{
		Job: job, Kind: kind,
		Title:  in.Predicate,
		Detail: in.Detail,
		// The site supplies the place, so four hundred stores are four hundred
		// rows in one list rather than four hundred typed addresses.
		Where: site.Where, Area: site.Area,
		LatE7: site.LatE7, LonE7: site.LonE7, RadiusM: site.RadiusM,
		SiteID: site.ID,
		// A site's access notes join the job's own, and are released to the
		// claimant only, exactly like instructions.
		Instructions: joinNonEmpty(in.Instructions, site.Access),
		Deliverable:  in.Deliverable,
		NotBefore:    parseWhen(in.NotBeforeRFC3339),
		NotAfter:     parseWhen(in.NotAfterRFC3339),
		PayMinor:     in.FeeMinor, BonusMinor: in.BonusMinor,
		AttemptMinor: in.AttemptMinor, ExpenseCapMinor: in.ExpenseCapMinor,
		Currency: in.Currency, Slots: in.Slots, Tier: in.Tier,
		Skills:    api.NormalizeSkills(in.Skills),
		WorkHours: in.WorkHours,
		Owner:     principal,
		ProjectID: projectID,
		Reference: in.Reference,
		Expires:   s.now().Add(ttl), Posted: s.now(),
	}
	if in.DirectTo != "" {
		if !s.Book.IsApproved(principal, in.DirectTo) {
			return "", fmt.Errorf("%s is not an approved vendor of yours", in.DirectTo)
		}
		l.DirectedTo = []string{in.DirectTo}
	}
	if in.RequireInsuredToMinor > 0 || in.RequireVetted {
		l.Requires = &api.Requirements{
			InsuredToMinor: in.RequireInsuredToMinor,
			Vetted:         in.RequireVetted,
		}
	}
	if ref := api.Screen(l.Title, l.Detail, l.Instructions, l.Deliverable); ref != nil {
		return "", fmt.Errorf("%s", ref.Why)
	}
	if s.Ledger != nil {
		if _, err := s.Ledger.Hold(r.Context(), "hold-"+job, job, principal,
			MaxPayoutFor(l), l.Currency); err != nil {
			return "", err
		}
	}
	if err := s.Projects.Attach(projectID, job); err != nil {
		return "", err
	}
	if err := s.Board.Post(l); err != nil {
		return "", err
	}
	s.setBuyer(job, principal)
	return job, nil
}

func joinNonEmpty(parts ...string) string {
	var out []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "\n\n")
}
