package api

import (
	"fmt"
	"sort"
	"time"
)

// Some jobs have a price and some have to be asked about.
//
// "Photograph this building" is worth roughly what anyone would charge to walk
// there, so a buyer can name a figure. "Mow my lawn" is not: it depends on the
// lawn. Naming a price for that either overpays every time or sits unclaimed,
// and an agent posting on somebody's behalf has no way to know which.
//
// So a job may be posted open, with a date by which bids close. Workers say
// what they would charge and when they could do it; the person (or their agent,
// under limits they set) picks one. Everything downstream is unchanged — the
// winning bid becomes the price, and the same evidence and settlement rules
// apply.
const (
	// PriceFixed means the buyer named the amount and the first qualified
	// worker takes it.
	PriceFixed = "fixed"
	// PriceBids means workers propose amounts and one is chosen.
	PriceBids = "bids"
)

// Bid is one worker's offer on an open job.
type Bid struct {
	ID     string `json:"id"`
	Job    string `json:"job"`
	Worker string `json:"worker"`
	// AmountMinor is what they would charge, all in.
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
	// Note is how they would approach it, in their words. Buyers pick on this
	// as much as on price, and hiding it would make every job an auction on
	// cost alone.
	Note string `json:"note,omitempty"`
	// Assumptions answer whatever the job said it does not know. See brief.go.
	Assumptions []Assumption `json:"assumptions,omitempty"`
	// AvailableFrom is the earliest they could do it.
	AvailableFrom time.Time `json:"available_from,omitempty"`
	Placed        time.Time `json:"placed"`
	// Won is set once this bid is accepted.
	Won bool `json:"won,omitempty"`
}

// PlaceBid records an offer.
//
// One bid per worker per job, replaced if they revise it: letting somebody
// stack bids would let one worker crowd out every other offer a buyer sees.
func (b *Board) PlaceBid(job, worker string, amountMinor int64, currency, note string, from time.Time, assumptions ...Assumption) (*Bid, error) {
	if amountMinor <= 0 {
		return nil, fmt.Errorf("board: a bid must name an amount")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	l, ok := b.listings[job]
	if !ok {
		return nil, ErrUnavailable
	}
	if l.Pricing != PriceBids {
		return nil, fmt.Errorf("board: this job has a fixed price; take it instead of bidding")
	}
	now := b.now()
	if !l.BidsCloseAt.IsZero() && now.After(l.BidsCloseAt) {
		return nil, fmt.Errorf("board: bidding on this job has closed")
	}
	if l.Awarded != "" {
		return nil, fmt.Errorf("board: this job has already been awarded")
	}
	// A number with nothing behind it is not an offer. A job that says it does
	// not know its own dimensions has to be bid with a stated assumption, or
	// both sides discover on site that they bought different things.
	if err := l.CheckAssumptions(assumptions); err != nil {
		return nil, err
	}
	// Nobody bids on a job they are meant to judge, or judges one they bid on.
	if l.Parent != "" && b.worked[worker][l.Parent] {
		return nil, fmt.Errorf("board: you worked on what this reviews")
	}

	if currency == "" {
		currency = l.Currency
	}
	bid := &Bid{
		ID: fmt.Sprintf("%s:%s", job, worker), Job: job, Worker: worker,
		AmountMinor: amountMinor, Currency: currency, Note: note,
		Assumptions:   append([]Assumption(nil), assumptions...),
		AvailableFrom: from, Placed: now,
	}
	if b.bids == nil {
		b.bids = map[string][]*Bid{}
	}
	for i, existing := range b.bids[job] {
		if existing.Worker == worker {
			b.bids[job][i] = bid // a revision, not a second offer
			b.saveLocked()
			return bid, nil
		}
	}
	b.bids[job] = append(b.bids[job], bid)
	b.saveLocked()
	return bid, nil
}

// Bids returns the offers on a job, cheapest first.
func (b *Board) Bids(job string) []*Bid {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]*Bid, 0, len(b.bids[job]))
	for _, bid := range b.bids[job] {
		cp := *bid
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AmountMinor < out[j].AmountMinor })
	return out
}

// Award accepts a bid and turns it into the job's price.
//
// The escrow check runs again here rather than only at posting, because until
// a bid is accepted nobody knows what the job costs — an open job is funded
// for whatever ceiling the buyer set, and a bid above it cannot be accepted
// however good it looks.
func (b *Board) Award(job, bidID string, funded func(*Listing) error) (*Bid, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	l, ok := b.listings[job]
	if !ok {
		return nil, ErrUnavailable
	}
	if l.Awarded != "" {
		return nil, fmt.Errorf("board: this job has already been awarded")
	}
	var won *Bid
	for _, bid := range b.bids[job] {
		if bid.ID == bidID {
			won = bid
			break
		}
	}
	if won == nil {
		return nil, fmt.Errorf("board: no such bid")
	}
	if l.MaxBidMinor > 0 && won.AmountMinor > l.MaxBidMinor {
		return nil, fmt.Errorf("board: that bid is above the %d ceiling set for this job",
			l.MaxBidMinor)
	}
	// The winning amount becomes the price, so everything downstream — escrow,
	// settlement, the worker's console — reads one number.
	proposed := *l
	proposed.PayMinor = won.AmountMinor
	if funded != nil {
		if err := funded(&proposed); err != nil {
			return nil, fmt.Errorf("board: %w", err)
		}
	}
	l.PayMinor = won.AmountMinor
	l.Awarded = won.Worker
	// What was assumed becomes part of the job. Accepting a bid accepts the
	// figures it was priced on, and the evidence is judged against those —
	// not against the blank the buyer started with.
	l.Agreed = append([]Assumption(nil), won.Assumptions...)
	won.Won = true
	b.saveLocked()
	return won, nil
}
