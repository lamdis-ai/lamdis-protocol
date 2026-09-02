package api

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Multi-part work, from the side that does it.
//
// A homeowner's agent posts three things at one address: repair the front
// driveway, pave a new drive to the back, pour a slab for the barn. A paving
// contractor is the right supplier for all three and should want all three.
//
// What the board showed them was three unrelated listings. Nothing said the
// jobs shared a property, a customer, or a week. That is not a display problem,
// it is a pricing one: mobilisation — getting a crew, a paver and a roller to a
// site — is most of the cost of a small job, and three jobs at one address is
// one mobilisation. Priced as strangers, the contractor either bids three
// mobilisations and loses on all of them, or bids one and is ruined if they win
// only two.
//
// Three things follow, and this file holds them:
//
//   - a project's shape is published, minus its budget, so an operator can see
//     that jobs belong together before deciding what to charge;
//   - jobs may depend on other jobs, so an order that really exists can be
//     stated rather than hoped for;
//   - a bid may cover several jobs at once and be all-or-nothing, so the right
//     commercial answer — "all three, one visit, $14,200" — is expressible.
//
// What stays absent is the budget. The envelope is the buyer's negotiating
// position and publishing it would turn every project into a bid at the
// ceiling.

// ProjectBrief is a project as an operator may see it.
//
// Built by redaction's opposite rule to Listing.Public: fields are named in
// rather than cleared out, so a field added to Project later is invisible here
// until somebody decides otherwise.
type ProjectBrief struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Jobs is how many pieces the whole scope has.
	Jobs int `json:"jobs"`
	// Position is where this one falls in the sequence, 1-based.
	Position int `json:"position,omitempty"`
	// Sites is how many distinct locations the scope touches. One means every
	// piece is at the same place, which is the fact that changes the price.
	Sites int `json:"sites"`
	// OneVisit says the whole scope shares a single site.
	OneVisit bool `json:"one_visit,omitempty"`
	// Open is how many pieces nobody has taken yet.
	Open int `json:"open"`
	// BidsAsOne reports that the buyer will accept a single offer covering
	// every piece.
	BidsAsOne bool `json:"bids_as_one,omitempty"`
	// BudgetMinor is deliberately absent, and so is Owner.
}

// Blocked reports which of this job's dependencies are not yet satisfied.
//
// Ordering is a real constraint on physical work, not a nicety. You do not pave
// the back drive while a concrete truck still needs to cross that ground, and a
// slab needs to cure before anything drives on it. Without somewhere to say so,
// two operators book the same ground for the same day and one of them wastes a
// mobilisation.
func (b *Board) Blocked(job string) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	l, ok := b.listings[job]
	if !ok {
		return nil
	}
	var waiting []string
	for _, dep := range l.DependsOn {
		d, ok := b.listings[dep]
		if !ok {
			// A dependency on a job that does not exist blocks nothing. The
			// alternative is a listing nobody can ever take because of a typo.
			continue
		}
		if !d.Accepted {
			waiting = append(waiting, dep)
		}
	}
	return waiting
}

// blockedLocked names the jobs standing in this one's way. Caller holds the lock.
func (b *Board) blockedLocked(l *Listing) []string {
	var waiting []string
	for _, dep := range l.DependsOn {
		if d, ok := b.listings[dep]; ok && !d.Accepted {
			waiting = append(waiting, d.Job)
		}
	}
	return waiting
}

// scopeOf collects a project's listings in the order they have to happen.
// Caller holds the lock.
//
// Dependency order, not posting order. A contractor reading "piece 1 of 3"
// takes it to mean "this is the one that goes first", and posting order does
// not mean that — three jobs posted in the same request share a timestamp and
// fall back to sorting by job id, so the sequence a buyer carefully expressed
// came out alphabetical. Pieces that nothing blocks come first; ties keep
// posting order so the buyer's own ordering survives where it is the only
// signal available.
func (b *Board) scopeOf(projectID string) []*Listing {
	var all []*Listing
	for _, l := range b.listings {
		if l.ProjectID == projectID {
			all = append(all, l)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Posted.Equal(all[j].Posted) {
			return all[i].Job < all[j].Job
		}
		return all[i].Posted.Before(all[j].Posted)
	})

	member := map[string]bool{}
	for _, l := range all {
		member[l.Job] = true
	}
	placed := map[string]bool{}
	out := make([]*Listing, 0, len(all))
	// Kahn's algorithm, taking ready pieces in the stable order above so the
	// result is deterministic. Bounded by len(all) passes, so a dependency
	// cycle terminates instead of hanging: whatever is left over is appended
	// in stable order rather than dropped, because a listing that vanishes
	// from the scope because of a bad edge is far worse than one out of order.
	for pass := 0; pass < len(all) && len(out) < len(all); pass++ {
		progressed := false
		for _, l := range all {
			if placed[l.Job] {
				continue
			}
			ready := true
			for _, dep := range l.DependsOn {
				// Only dependencies inside this project can order it. An edge
				// pointing outside is a constraint on claiming, not on where
				// the piece sits in this list.
				if member[dep] && !placed[dep] {
					ready = false
					break
				}
			}
			if ready {
				out = append(out, l)
				placed[l.Job] = true
				progressed = true
			}
		}
		if !progressed {
			break
		}
	}
	for _, l := range all {
		if !placed[l.Job] {
			out = append(out, l)
		}
	}
	return out
}

// BriefFor describes the project a job belongs to, from the board's own view.
//
// Deliberately computed from listings rather than read from the project store:
// the board already knows which listings carry which ProjectID, and reaching
// into the buyer-side store from here is how a budget ends up on a public page.
func (b *Board) BriefFor(job string) *ProjectBrief {
	b.mu.Lock()
	defer b.mu.Unlock()
	l, ok := b.listings[job]
	if !ok || l.ProjectID == "" {
		return nil
	}
	return b.briefLocked(l.ProjectID, job)
}

func (b *Board) briefLocked(projectID, job string) *ProjectBrief {
	scope := b.scopeOf(projectID)
	if len(scope) == 0 {
		return nil
	}
	br := &ProjectBrief{ID: projectID, Jobs: len(scope)}
	sites := map[string]bool{}
	now := b.now()
	for i, l := range scope {
		if l.Job == job {
			br.Position = i + 1
		}
		key := l.SiteID
		if key == "" {
			key = l.Area
		}
		sites[key] = true
		if l.Open(now) && l.Awarded == "" {
			br.Open++
		}
		if br.Title == "" {
			br.Title = l.ProjectTitle
		}
		if l.BidsAsOne {
			br.BidsAsOne = true
		}
	}
	br.Sites = len(sites)
	br.OneVisit = len(sites) == 1
	return br
}

// ProjectScope is the whole of a project as an operator may see it: the brief,
// plus every piece as a public listing, in the order they must happen.
type ProjectScope struct {
	Project *ProjectBrief `json:"project"`
	Jobs    []*ScopeJob   `json:"jobs"`
}

// ScopeJob is one piece, with what an operator needs to sequence it.
type ScopeJob struct {
	*Listing
	// BlockedBy lists pieces that must finish first.
	BlockedBy []string `json:"blocked_by,omitempty"`
	// Claimable says nothing stands in the way of taking this one now.
	Claimable bool `json:"claimable"`
}

// Scope returns a project as an operator may see it, or nil if no listing
// carries that project.
func (b *Board) Scope(projectID string) *ProjectScope {
	b.mu.Lock()
	defer b.mu.Unlock()
	scope := b.scopeOf(projectID)
	if len(scope) == 0 {
		return nil
	}
	out := &ProjectScope{Project: b.briefLocked(projectID, "")}
	now := b.now()
	for _, l := range scope {
		sj := &ScopeJob{Listing: l.Public()}
		sj.Listing.Project = b.briefLocked(projectID, l.Job)
		sj.BlockedBy = b.blockedLocked(l)
		sj.Claimable = len(sj.BlockedBy) == 0 && l.Open(now) && l.Awarded == ""
		out.Jobs = append(out.Jobs, sj)
	}
	return out
}

// A bid on the whole scope.
//
// The unit of a project bid is a set of lines, not one number, because escrow
// and settlement are per job all the way down. Asking the contractor to split
// their own total is not busywork: it is where they decide which piece carries
// the mobilisation, and it is what lets the buyer award the whole thing without
// anybody guessing what each part was worth.

// ProjectBid is one supplier's offer on several jobs at once.
type ProjectBid struct {
	ID      string `json:"id"`
	Project string `json:"project"`
	Worker  string `json:"worker"`
	// Lines is what they would charge for each piece. Every job in the project
	// must appear: a partial offer is a different product, and the operator can
	// always bid on the pieces individually instead.
	Lines []BidLine `json:"lines"`
	// TotalMinor is the sum of the lines, carried so a buyer comparing offers
	// reads one number without adding up.
	TotalMinor int64  `json:"total_minor"`
	Currency   string `json:"currency"`
	// Note is how they would run it — the sequence, the crew, the week.
	Note string `json:"note,omitempty"`
	// Assumptions answer whatever the pieces said they do not know.
	Assumptions []Assumption `json:"assumptions,omitempty"`
	// AllOrNothing means award every line or none.
	//
	// Defaults true, and that default is the point of the feature. A contractor
	// who prices three jobs as one mobilisation and wins two of them has priced
	// two jobs wrong. Letting a buyer cherry-pick the cheap lines out of a
	// bundle recreates exactly the problem bundling exists to solve.
	AllOrNothing  bool      `json:"all_or_nothing"`
	AvailableFrom time.Time `json:"available_from,omitempty"`
	Placed        time.Time `json:"placed"`
	Won           bool      `json:"won,omitempty"`
}

// BidLine is what one piece of the scope would cost.
type BidLine struct {
	Job         string `json:"job"`
	AmountMinor int64  `json:"amount_minor"`
	// Note is why this piece costs what it does — "includes mobilisation for
	// all three", which is the sentence that makes a bundle legible.
	Note string `json:"note,omitempty"`
}

// PlaceProjectBid records an offer across a whole project.
func (b *Board) PlaceProjectBid(projectID, worker string, lines []BidLine, currency, note string, from time.Time, allOrNothing bool, assumptions ...Assumption) (*ProjectBid, error) {
	if worker == "" {
		return nil, fmt.Errorf("board: a bid needs a bidder")
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("board: a project bid must price at least one piece")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	scope := b.scopeOf(projectID)
	if len(scope) == 0 {
		return nil, ErrUnavailable
	}
	if !scope[0].BidsAsOne {
		return nil, fmt.Errorf("board: this project is not taking offers on the whole scope; bid on the pieces")
	}

	inScope := map[string]*Listing{}
	for _, l := range scope {
		inScope[l.Job] = l
	}
	now := b.now()
	var total int64
	seen := map[string]bool{}
	for _, ln := range lines {
		l, ok := inScope[ln.Job]
		if !ok {
			return nil, fmt.Errorf("board: %s is not part of this project", ln.Job)
		}
		if seen[ln.Job] {
			return nil, fmt.Errorf("board: %s is priced twice", ln.Job)
		}
		seen[ln.Job] = true
		if ln.AmountMinor <= 0 {
			return nil, fmt.Errorf("board: every piece needs an amount")
		}
		// A ceiling is per job and still binds inside a bundle. Otherwise a
		// bundle is a way to route around the limit the buyer set.
		if l.MaxBidMinor > 0 && ln.AmountMinor > l.MaxBidMinor {
			return nil, fmt.Errorf("board: the amount for %s is above what the buyer set for it", ln.Job)
		}
		if l.Awarded != "" {
			return nil, fmt.Errorf("board: %s has already been awarded", ln.Job)
		}
		if !l.BidsCloseAt.IsZero() && now.After(l.BidsCloseAt) {
			return nil, fmt.Errorf("board: bidding has closed on %s", ln.Job)
		}
		// Every piece's open questions have to be answered, whichever piece
		// they belong to. A bundle is one offer and carries one set.
		if err := l.CheckAssumptions(assumptions); err != nil {
			return nil, err
		}
		total += ln.AmountMinor
	}
	// An all-or-nothing bid that does not cover the scope is not all of
	// anything. Caught here rather than at award, when the buyer has already
	// decided and the contractor has already been told they won.
	if allOrNothing && len(seen) != len(scope) {
		var missing []string
		for _, l := range scope {
			if !seen[l.Job] {
				missing = append(missing, l.Job)
			}
		}
		return nil, fmt.Errorf("board: an all-or-nothing offer has to price the whole scope; missing %s",
			strings.Join(missing, ", "))
	}
	if currency == "" {
		currency = scope[0].Currency
	}
	bid := &ProjectBid{
		ID: projectID + ":" + worker, Project: projectID, Worker: worker,
		Lines: append([]BidLine(nil), lines...), TotalMinor: total,
		Currency: currency, Note: note, AllOrNothing: allOrNothing,
		Assumptions:   append([]Assumption(nil), assumptions...),
		AvailableFrom: from, Placed: now,
	}
	if b.projectBids == nil {
		b.projectBids = map[string][]*ProjectBid{}
	}
	for i, existing := range b.projectBids[projectID] {
		if existing.Worker == worker {
			b.projectBids[projectID][i] = bid // a revision, not a second offer
			b.saveLocked()
			return bid, nil
		}
	}
	b.projectBids[projectID] = append(b.projectBids[projectID], bid)
	b.saveLocked()
	return bid, nil
}

// ProjectBids returns offers on a whole project, cheapest first.
func (b *Board) ProjectBids(projectID string) []*ProjectBid {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]*ProjectBid, 0, len(b.projectBids[projectID]))
	for _, bid := range b.projectBids[projectID] {
		cp := *bid
		cp.Lines = append([]BidLine(nil), bid.Lines...)
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TotalMinor < out[j].TotalMinor })
	return out
}

// AwardProject accepts a bundle, awarding every line at once.
//
// The funded callback runs for every line before any line is awarded. A bundle
// that is half-funded must not half-land: the contractor was told the price of
// arriving once, and awarding two of three pieces hands them a job they priced
// on an assumption that is no longer true.
func (b *Board) AwardProject(projectID, bidID string, funded func(*Listing) error) (*ProjectBid, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	var won *ProjectBid
	for _, bid := range b.projectBids[projectID] {
		if bid.ID == bidID {
			won = bid
			break
		}
	}
	if won == nil {
		return nil, fmt.Errorf("board: no such offer")
	}
	// Check everything first.
	type pending struct {
		l      *Listing
		amount int64
	}
	var todo []pending
	for _, ln := range won.Lines {
		l, ok := b.listings[ln.Job]
		if !ok {
			return nil, ErrUnavailable
		}
		if l.Awarded != "" {
			return nil, fmt.Errorf("board: %s has already been awarded", ln.Job)
		}
		if l.MaxBidMinor > 0 && ln.AmountMinor > l.MaxBidMinor {
			return nil, fmt.Errorf("board: the amount for %s is above the ceiling set for it", ln.Job)
		}
		proposed := *l
		proposed.PayMinor = ln.AmountMinor
		if funded != nil {
			if err := funded(&proposed); err != nil {
				return nil, fmt.Errorf("board: %w", err)
			}
		}
		todo = append(todo, pending{l: l, amount: ln.AmountMinor})
	}
	// Then commit all of it.
	for _, p := range todo {
		p.l.PayMinor = p.amount
		p.l.Awarded = won.Worker
		p.l.Agreed = append([]Assumption(nil), won.Assumptions...)
	}
	won.Won = true
	b.saveLocked()
	return won, nil
}

// Who decides how a job is cut into stages.
//
// This is a reversal, and it is worth saying why plainly.
//
// Stages were built for exactly this case — a driveway, prep and base and
// binder and surface, three days, forty tons of asphalt paid for on the first
// morning — and the direction was wrong. Stages were taken from the buyer's
// posting request, so the schedule a crew is judged against was authored by the
// person who wanted the work. That holds when a buyer genuinely knows the
// breakdown: a compliance sweep, a materials-then-labour split, anything a
// procurement team has done before.
//
// It does not hold for trade work, which is most of what this is for. A
// homeowner does not know what a binder course is. Their agent does not either,
// and an agent guessing is worse than a homeowner not knowing, because a guess
// is plausible and gets posted. The crew then works to a schedule invented by
// somebody who has never poured concrete, and the exchange judges them against
// it.
//
// The knowledge of how work decomposes lives on the supply side. So: the buyer
// says what outcome they want and what they will pay at most; the supplier who
// wins says how it breaks down and what each piece is worth; the buyer accepts
// the plan before anybody starts. That is how trade work is actually bought.
const (
	// PlanByBuyer is the original behaviour and stays the default. A job
	// posted with stages already on it is unchanged in every respect.
	PlanByBuyer = "buyer"
	// PlanBySupplier means the winning bidder proposes the breakdown.
	PlanBySupplier = "supplier"
)

// Plan states.
const (
	PlanNone     = ""
	PlanProposed = "proposed"
	PlanAccepted = "accepted"
)

// ProposeStages records a supplier's breakdown of a job they hold.
//
// The sum must equal the agreed price. The supplier is deciding the shape of
// the schedule, not reopening the price — that was settled when their bid was
// accepted, and letting a plan change the total would make every award a first
// offer.
func (b *Board) ProposeStages(job, worker string, stages []Stage) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	l, ok := b.listings[job]
	if !ok {
		return ErrUnavailable
	}
	if l.PlanBy != PlanBySupplier {
		return fmt.Errorf("board: this job's stages were set by the buyer")
	}
	if l.Awarded == "" || b.accountFor(worker) != b.accountFor(l.Awarded) {
		// The same answer a stranger gets. Whether this job has been awarded,
		// and to whom, is not a question the board answers for passers-by.
		return ErrUnavailable
	}
	if l.PlanState == PlanAccepted {
		return fmt.Errorf("board: the plan for this job has already been agreed")
	}
	if len(stages) == 0 {
		return fmt.Errorf("board: a plan needs at least one stage")
	}
	// Validated against a copy carrying the proposal, so a bad plan cannot
	// leave the listing in a state where the stages and the price disagree.
	probe := *l
	probe.Stages = stages
	if err := probe.ValidateStages(); err != nil {
		return err
	}
	l.ProposedStages = append([]Stage(nil), stages...)
	l.PlanState = PlanProposed
	b.saveLocked()
	return nil
}

// AcceptPlan is the buyer agreeing to the supplier's breakdown, which is the
// moment it becomes the schedule the work is judged against.
func (b *Board) AcceptPlan(job string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	l, ok := b.listings[job]
	if !ok {
		return ErrUnavailable
	}
	if l.PlanState != PlanProposed {
		return fmt.Errorf("board: there is no plan waiting on this job")
	}
	l.Stages = append([]Stage(nil), l.ProposedStages...)
	l.ProposedStages = nil
	l.PlanState = PlanAccepted
	b.saveLocked()
	return nil
}

// RejectPlan sends a plan back with a reason.
func (b *Board) RejectPlan(job, why string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	l, ok := b.listings[job]
	if !ok {
		return ErrUnavailable
	}
	if l.PlanState != PlanProposed {
		return fmt.Errorf("board: there is no plan waiting on this job")
	}
	l.ProposedStages = nil
	l.PlanState = PlanNone
	l.PlanNote = why
	b.saveLocked()
	return nil
}
