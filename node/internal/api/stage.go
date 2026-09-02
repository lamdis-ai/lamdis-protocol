package api

import (
	"fmt"
	"strings"
	"time"
)

// Work that takes longer than one visit.
//
// Everything here assumed one person, one trip, one photograph, one payment.
// That holds for checking whether a sign is up and breaks completely on a
// driveway: prep, base, binder, surface, over three days, with forty tons of
// asphalt paid for on the first morning. Under the single-moment model a
// paving crew claimed a job, lost the lease forty-five minutes later while
// standing on it, could submit evidence exactly once, and financed the buyer's
// materials until the whole thing settled.
//
// A staged job is the same contract cut into pieces that can each be shown and
// paid for. It is not a new kind of work — a job with no stages behaves
// exactly as before.

// Stage is one payable piece of a longer job.
type Stage struct {
	// Name is what the crew and the buyer both call it: "base course".
	Name string `json:"name"`
	// Deliverable is what proves this stage specifically. Judged against this
	// rather than against the job's headline predicate, because "the driveway
	// is paved" is not true yet when the base is done and saying it is would
	// make the evidence a lie.
	Deliverable string `json:"deliverable"`
	// PayMinor is what this stage earns when its evidence is accepted.
	PayMinor int64 `json:"pay_minor"`
	// Materials marks a stage that is reimbursement rather than labour — the
	// asphalt, the paint, the parts. Paid on evidence of purchase so nobody
	// carries the buyer's costs for the length of the job.
	Materials bool `json:"materials,omitempty"`
}

// Staged reports whether this job is cut into pieces.
func (l *Listing) Staged() bool { return len(l.Stages) > 0 }

// StageAt returns a stage by position.
func (l *Listing) StageAt(i int) (Stage, bool) {
	if i < 0 || i >= len(l.Stages) {
		return Stage{}, false
	}
	return l.Stages[i], true
}

// StageIndex finds a stage by name, case-insensitively.
func (l *Listing) StageIndex(name string) (int, bool) {
	want := strings.ToLower(strings.TrimSpace(name))
	for i, s := range l.Stages {
		if strings.ToLower(s.Name) == want {
			return i, true
		}
	}
	return -1, false
}

// StagePay is the total across every stage.
func (l *Listing) StagePay() int64 {
	var total int64
	for _, s := range l.Stages {
		total += s.PayMinor
	}
	return total
}

// ValidateStages checks a staged job is coherent before anybody is asked to do
// it.
//
// The sum has to match the headline price or the two disagree about what the
// job costs, and a buyer reading one figure while a crew is paid against
// another is the sort of disagreement that ends in a dispute nobody can
// resolve from the record.
func (l *Listing) ValidateStages() error {
	if !l.Staged() {
		return nil
	}
	if len(l.Stages) > MaxStages {
		return fmt.Errorf("board: %d stages is more than a job can be cut into (limit %d)",
			len(l.Stages), MaxStages)
	}
	seen := map[string]bool{}
	for i, s := range l.Stages {
		if strings.TrimSpace(s.Name) == "" {
			return fmt.Errorf("board: stage %d has no name", i+1)
		}
		if seen[strings.ToLower(s.Name)] {
			return fmt.Errorf("board: two stages are both called %q", s.Name)
		}
		seen[strings.ToLower(s.Name)] = true
		if strings.TrimSpace(s.Deliverable) == "" {
			return fmt.Errorf("board: stage %q does not say what would prove it", s.Name)
		}
		if s.PayMinor < 0 {
			return fmt.Errorf("board: stage %q pays a negative amount", s.Name)
		}
	}
	if got := l.StagePay(); got != l.PayMinor {
		return fmt.Errorf(
			"board: the stages add up to %d but the job is listed at %d; "+
				"they have to agree on what the work costs", got, l.PayMinor)
	}
	return nil
}

// MaxStages bounds how finely a job may be cut. Enough for a real trade job,
// few enough that a board entry stays readable.
const MaxStages = 12

// MaxWorkHours is the longest a buyer may let one person hold a job.
//
// Three days covers a real multi-visit errand. Anything longer is either
// staged work — where each stage carries its own lease — or a job that will
// sit dead in a griefer's hands for as long as the buyer allowed.
const MaxWorkHours = 72

// LeaseFor is how long somebody may hold this job before it is treated as
// abandoned.
//
// Forty-five minutes is right for walking to an address and photographing a
// sign, and catastrophic for anything longer: a paving crew lost the job they
// were standing on, was marked as having abandoned it, and had the whole
// company put in cooldown. A buyer who knows the work takes three days says
// so, and the lease matches the work rather than the other way round.
func (l *Listing) LeaseFor(fallback time.Duration) time.Duration {
	if l.WorkHours > 0 {
		hours := l.WorkHours
		if hours > MaxWorkHours {
			// Post refuses a listing over the cap; this is the backstop for a
			// listing that reached the board another way.
			hours = MaxWorkHours
		}
		return time.Duration(hours) * time.Hour
	}
	if l.Staged() {
		// A staged job is multi-visit by construction. Assuming otherwise
		// would reintroduce exactly the failure stages exist to fix.
		return 48 * time.Hour
	}
	return fallback
}
