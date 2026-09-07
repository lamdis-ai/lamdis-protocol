package exchange

import (
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/ledger"
)

// What your agents did with your money.
//
// Everything an agent can do had a route; the person whose balance it spends
// had none. They funded an account, handed out a key, and lost sight of it —
// no list of what was bought, no way to see the evidence they paid for, no
// running total. The only interface on the whole exchange belonged to the
// people earning, not the person spending.
//
// This is the oversight surface. It answers three questions in one response:
// what is left, what went out, and what came back.

// SpendServer reports an account's outgoing side to the person who owns it.
type SpendServer struct {
	Server  *Server
	Workers *api.Workers
}

func (ss *SpendServer) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/spend", ss.handleSpend)
}

// spendRow is one job this account paid for.
type spendRow struct {
	Job            string    `json:"job"`
	Kind           string    `json:"kind"`
	Title          string    `json:"title"`
	Where          string    `json:"where,omitempty"`
	Posted         time.Time `json:"posted"`
	Currency       string    `json:"currency"`
	CommittedMinor int64     `json:"committed_minor"`
	Status         string    `json:"status"`
	Submissions    int       `json:"submissions"`
	Accepted       int       `json:"accepted"`
	// Evidence is where the person can look at what came back. Present only
	// when something did: a link to an empty page is worse than no link.
	Evidence string `json:"evidence,omitempty"`
	Receipt  string `json:"receipt,omitempty"`
	// Agent names the key that spent this, so a runaway agent is identifiable
	// rather than merely visible in aggregate.
	Agent string `json:"agent,omitempty"`

	// Worker is who is doing it, and Standing is their record here.
	//
	// Somebody is going to a buyer's address. Publishing nothing about them
	// was defensible only while nobody could be paid; now that they can, the
	// buyer gets what the exchange actually knows — a stable handle and a
	// completion record — and nothing it does not, because inventing
	// reassurance is worse than admitting the limit.
	Worker string `json:"worker,omitempty"`
	// Supplier is the business behind the person, with its verified
	// credentials. Absent when somebody is working for themselves.
	Supplier map[string]any `json:"supplier,omitempty"`

	// Review is what is waiting on the buyer: how much is about to be sent,
	// when, and what they can do about it. A review window nobody is told
	// about protects nobody.
	Review map[string]any `json:"review,omitempty"`

	// Stages is progress through a long job, piece by piece, so a buyer with a
	// three-day pour is not staring at "somebody is working on it" for three
	// days.
	Stages      []map[string]any `json:"stages,omitempty"`
	StagesDone  int              `json:"stages_done,omitempty"`
	StagesTotal int              `json:"stages_total,omitempty"`
	Completed   int              `json:"worker_completed,omitempty"`
	Abandoned   int              `json:"worker_abandoned,omitempty"`
}

func (ss *SpendServer) handleSpend(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	worker, err := ss.Workers.Authenticate(r, body, ss.Server.now())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "sign in first")
		return
	}
	s := ss.Server

	var rows []spendRow
	var committed, returned int64
	for _, l := range s.Board.All() {
		if l.Owner != worker.ID {
			continue
		}
		// A sandbox job committed nothing and returned nothing. Listing it
		// here would put money that does not exist into a buyer's record of
		// what they have spent.
		if l.Sandbox {
			continue
		}
		subs := s.Submissions(l.Job)
		accepted := 0
		for _, sub := range subs {
			if sub.Verified {
				accepted++
			}
		}
		row := spendRow{
			Job: l.Job, Kind: l.Kind, Title: l.Title, Where: l.Where,
			Posted: l.Posted, Currency: l.Currency,
			CommittedMinor: l.PayMinor + l.BonusMinor + l.ExpenseCapMinor,
			Submissions:    len(subs), Accepted: accepted,
			Status: spendStatus(l, subs, accepted, s.now()),
		}
		if len(subs) > 0 {
			row.Evidence = "/v1/jobs/" + l.Job + "/evidence"
			row.Receipt = "/v1/jobs/" + l.Job + "/receipt"
		}
		if holders := s.Board.HoldersOf(l.Job); len(holders) > 0 {
			row.Worker = shortHandle(holders[0])
			row.Completed, row.Abandoned, _, _ = s.Board.Standing(holders[0])
			// When a business is doing the work, the buyer should see the
			// business: its name, its checked licences, and the cover it
			// carries. That is what they are actually choosing on, and a
			// contractor who pays for all three had nowhere to show it.
			if s.Suppliers != nil {
				if sup, ok := s.Suppliers.SupplierFor(holders[0]); ok {
					row.Supplier = sup.Public(s.now())
				}
			}
		}
		row.Review = s.reviewStateFor(l.Job)
		// Progress through a long job, so a buyer with a three-day pour is not
		// staring at "somebody is working on it" for three days.
		if l.Staged() {
			var names []string
			doneCount := 0
			for i, st := range l.Stages {
				mark := map[string]any{"name": st.Name, "pay_minor": st.PayMinor}
				for _, sub := range subs {
					if sub.Stage == i && sub.Verified {
						mark["done"] = true
						doneCount++
						break
					}
				}
				row.Stages = append(row.Stages, mark)
				names = append(names, st.Name)
			}
			row.StagesDone = doneCount
			row.StagesTotal = len(l.Stages)
			_ = names
		}
		committed += row.CommittedMinor
		if accepted > 0 {
			returned += row.CommittedMinor
		}
		rows = append(rows, row)
	}
	// Newest first: the question is almost always "what just happened".
	sort.Slice(rows, func(i, j int) bool { return rows[i].Posted.After(rows[j].Posted) })
	if rows == nil {
		rows = []spendRow{}
	}

	// Lead with what needs them, rather than making them find it in a list.
	needing := 0
	for _, r := range rows {
		if r.Review != nil && r.Review["awaiting_release_minor"] != nil {
			needing++
		}
	}
	out := map[string]any{
		"jobs": rows, "currency": "USD",
		"committed_minor": committed,
		"awaiting_review": needing,
	}
	if s.Ledger != nil {
		bal, _ := s.Ledger.Balance(r.Context(), ledger.BalanceOf(worker.ID), "USD")
		held, _ := s.Ledger.Balance(r.Context(), ledger.EscrowOf(worker.ID), "USD")
		out["balance_minor"] = bal
		out["held_minor"] = held
	}
	writeJSONResponse(w, out)
}

// spendStatus is the sentence a person actually wants: not the internal phase,
// but whether their money is coming back, going out, or stuck.
func spendStatus(l *api.Listing, subs []api.Submission, accepted int, now time.Time) string {
	switch {
	case accepted > 0:
		return "done"
	case len(subs) > 0:
		return "checking what came back"
	case l.Pricing == api.PriceBids && l.Awarded == "":
		return "collecting offers"
	case l.Expires.Before(now):
		// The money is not lost; it goes back when the sweeper runs. Saying
		// "expired" alone reads like it was spent on nothing.
		return "nobody took it — refunding"
	case l.Taken > 0:
		return "somebody is working on it"
	default:
		return "waiting for somebody to take it"
	}
}

// shortHandle is a stable, non-identifying name for a worker.
//
// Enough for a buyer to say "the same person as last time" and for support to
// find them; not their email, which is not the buyer's to have.
func shortHandle(worker string) string {
	if len(worker) <= 8 {
		return worker
	}
	return worker[:8]
}
