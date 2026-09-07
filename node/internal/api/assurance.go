package api

import (
	"fmt"
	"time"
)

// What protects a thousand dollars.
//
// Not the photograph. The photograph is worth about eighty-five cents of
// certainty and no amount of image forensics changes that, because the other
// side gets to pick the image and the tooling to make one improves faster than
// the tooling to catch it. A product whose answer to "somebody faked it" is
// "that is the real risk, and here is our honest confidence interval" has told
// the buyer to carry the loss.
//
// So the loss is designed out instead, on both sides, because both sides are
// being asked to trust a stranger with a month's income:
//
//   - A buyer's exposure is bounded by staging. A thousand-dollar job never
//     rides on one verdict. It is cut into pieces, each evidenced and paid on
//     its own, and a buyer who sees the first piece go wrong stops. The most
//     anybody can steal is one stage, not the job.
//
//   - Fraud is made to cost more than it pays. What somebody may take is
//     earned, not granted: a new account cannot claim a thousand-dollar job at
//     all. Getting to that ceiling means completing real work, at real prices,
//     through an identity the payment provider has checked. A thief has to
//     build a reputation worth more than the theft before the theft is
//     possible, and burns it on the first one.
//
//   - A worker's completed work cannot be frozen at a buyer's discretion.
//     That was the other half and it was worse: a buyer could type any
//     sentence into a hold and the money stopped forever, because nothing in
//     the system ever cleared it. See dispute.go.
//
// None of this claims a photograph is trustworthy. It arranges things so that
// it does not have to be.

// Money at which one verdict is too much to hang on a single photograph.
//
// Below it, a bad verdict costs somebody an afternoon. Above it, it costs them
// a month, and the difference in what the exchange should require is not a
// matter of degree.
// Set where losing it stops being an afternoon and starts being a month.
// Below this a job settles in one go, which keeps ordinary errands simple; the
// value ceiling still governs who may take one. Above it, one photograph is
// not allowed to decide the whole thing.
const StakesMinor = 50000 // $500

// How a job breaks down is the operator's business, not ours.
//
// An earlier version of this file capped a stage at $250, then at a share of
// the contract, and both were wrong for the same reason: a paving company
// knows how paving is sequenced and priced, and an exchange inventing rules
// about their stage sizes is telling a business how to run itself. It also
// does not work — a $12,000 driveway under a $250 cap needs twenty-two stages,
// which is not a safeguard, it is a way of making real trade work unpostable.
//
// What the exchange does insist on is narrower and is about exposure, not
// pricing: above StakesMinor a job may not settle in one lump, so the buyer
// finds out how it is going before all of it is committed and can stop. The
// breakdown itself is proposed by whoever is doing the work and accepted by
// whoever is paying. That is a negotiation between two businesses, and both of
// them know more about it than we do.

// Ceilings a worker earns rather than is given.
//
// The numbers are deliberately steep at the bottom. Somebody who has done
// nothing here can take small work and prove they turn up; a thousand-dollar
// job is not available to them at any price, which removes the entire class of
// attack where a fresh account takes one expensive job and disappears.
const (
	// NewCeilingMinor is what an account with no record may hold at once.
	NewCeilingMinor = 7500 // $75
	// ProvenCeilingMinor follows a few clean completions.
	ProvenCeilingMinor = 30000 // $300
	// EstablishedCeilingMinor follows a real run of them.
	EstablishedCeilingMinor = 120000 // $1,200
	// VettedCeilingMinor is for a business whose licences and cover a person
	// at the exchange has checked. Not self-service, which is what lets it
	// carry this much money.
	// A checked business. Set where a real trade contract fits — a driveway
	// and a slab together is fifteen thousand dollars and is ordinary work for
	// the kind of firm that gets vetted. The natural next step is a limit a
	// person sets per supplier when they vet them, the way a credit line
	// works, rather than one number for every vetted business.
	VettedCeilingMinor = 2500000 // $25,000
	// ShakenCeilingMinor is where somebody lands after abandoning work or
	// having evidence rejected as fabricated. Low enough that rebuilding is
	// slow and visible.
	ShakenCeilingMinor = 2500 // $25
)

// Standing is what the exchange knows about somebody's record.
type Standing struct {
	// Settled is how many jobs they finished and were paid for.
	Settled int
	// Abandoned is how many they took and let lapse.
	Abandoned int
	// Rejected counts evidence refused as fabricated. Distinct from work that
	// simply failed: getting it wrong is not the same as trying it on.
	Rejected int
	// Vetted marks a business a person at the exchange checked.
	Vetted bool
}

// ValueCeiling is the most this account may have riding on unfinished work.
//
// A ceiling on exposure rather than on a single job, because three
// four-hundred-dollar jobs taken at once are the same attack as one job at
// twelve hundred, and a per-job limit would not see it.
func ValueCeiling(s Standing) int64 {
	// One rejection for fabricated evidence is enough. There is no reading of
	// it that is a mistake, and an exchange that lets somebody try it and
	// carry on at full size is telling everybody else what it tolerates.
	if s.Rejected > 0 {
		return ShakenCeilingMinor
	}
	if s.Abandoned >= 3 {
		return ShakenCeilingMinor
	}
	if s.Vetted {
		return VettedCeilingMinor
	}
	switch {
	case s.Settled >= 10 && s.Abandoned == 0:
		return EstablishedCeilingMinor
	case s.Settled >= 10:
		return ProvenCeilingMinor
	case s.Settled >= 3 && s.Abandoned == 0:
		return ProvenCeilingMinor
	default:
		return NewCeilingMinor
	}
}

// CheckExposure reports whether taking this job would put more at stake than
// this account has earned the right to hold.
func CheckExposure(s Standing, alreadyHeldMinor, takingMinor int64) error {
	ceiling := ValueCeiling(s)
	if alreadyHeldMinor+takingMinor <= ceiling {
		return nil
	}
	// Said in terms of what to do about it, because the honest answer is
	// "finish some smaller work first" and hiding that behind a refusal
	// teaches nothing.
	if alreadyHeldMinor > 0 {
		return fmt.Errorf(
			"you can have %s of unfinished work at once and you are already "+
				"holding %s. Finish some of it, or take something smaller",
			minor(ceiling, "USD"), minor(alreadyHeldMinor, "USD"))
	}
	return fmt.Errorf(
		"work at this value opens up once you have a record here. Your limit "+
			"is %s at the moment; it rises as you complete jobs, and a business "+
			"whose licences and cover we have checked starts far higher",
		minor(ceiling, "USD"))
}

// RequireStaging reports whether a job this size may settle on one verdict.
//
// The rule is about the buyer's maximum loss, so it is stated in those terms:
// no single piece of evidence may be worth more than StakesMinor, whoever
// wrote the plan.
func RequireStaging(l *Listing) error {
	if l.PayMinor <= StakesMinor {
		return nil
	}
	if l.PlanBy == PlanBySupplier {
		// The winner writes the plan and it is checked when they propose it.
		// Refusing at posting would make the feature unusable, since the whole
		// point is that the stages are not known yet.
		return nil
	}
	if !l.Staged() {
		return fmt.Errorf(
			"a job worth %s cannot settle on a single photograph. Cut it into "+
				"stages that are each evidenced and paid as they are done, or set "+
				"plan_by to \"supplier\" and let whoever wins it propose the "+
				"breakdown. Nobody should be asked to risk %s on one verdict — "+
				"and that includes you",
			minor(l.PayMinor, l.Currency), minor(l.PayMinor, l.Currency))
	}
	return nil
}

// StandingFor reads an account's record off the board.
func (b *Board) StandingFor(worker string) Standing {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.standingLocked(worker)
}

func (b *Board) standingLocked(worker string) Standing {
	acct := b.accountFor(worker)
	st := Standing{
		Settled:   b.completed[acct],
		Abandoned: b.abandoned[acct],
		Rejected:  b.rejected[acct],
	}
	if b.Suppliers != nil {
		if sup, ok := b.Suppliers.SupplierFor(worker); ok && sup.Vetted {
			st.Vetted = true
		}
	}
	return st
}

// ExposureOf is what this account currently has riding on unfinished work.
func (b *Board) ExposureOf(worker string) int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.exposureLocked(worker)
}

func (b *Board) exposureLocked(worker string) int64 {
	acct := b.accountFor(worker)
	var total int64
	for job, holders := range b.seats {
		l, ok := b.listings[job]
		if !ok {
			continue
		}
		for holder := range holders {
			if b.accountFor(holder) != acct {
				continue
			}
			// A staged job risks the stage in front of them, not the whole
			// contract: the earlier stages are already evidenced and paid.
			total += l.AtRiskMinor()
			break
		}
	}
	return total
}

// AtRiskMinor is what one seat on this job actually puts at stake.
func (l *Listing) AtRiskMinor() int64 {
	// A sandbox job escrows nothing, so there is nothing to be at risk. Left
	// out, the exposure ceiling meant for real money would refuse the
	// simulated operator its own rehearsal after a few runs, and a developer
	// would watch the sandbox stop working for no reason they could see.
	if l.Sandbox {
		return 0
	}
	if !l.Staged() {
		return l.PayMinor
	}
	var largest int64
	for _, s := range l.Stages {
		if s.PayMinor > largest {
			largest = s.PayMinor
		}
	}
	return largest
}

// MarkRejected records evidence refused as fabricated, which is the one thing
// that collapses a ceiling on its own.
func (b *Board) MarkRejected(worker string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.rejected == nil {
		b.rejected = map[string]int{}
	}
	b.rejected[b.accountFor(worker)]++
	b.saveLocked()
}

// unusedAssuranceTime keeps the time import honest if the file is trimmed.
var _ = time.Time{}

// SeedStanding gives an account a record it did not earn.
//
// For tests and for seeding a demonstration board, and deliberately not
// reachable over any route: an account that can grant itself a record is an
// account with no ceiling, which is the whole mechanism undone. It lives here
// rather than in a test file because the demo seeder needs it too, and a
// helper that exists in two copies drifts.
func (b *Board) SeedStanding(worker string, settled, abandoned int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	acct := b.accountFor(worker)
	b.completed[acct] = settled
	b.abandoned[acct] = abandoned
	b.saveLocked()
}

// SeedVetted marks an account as a checked business, for tests and demos.
//
// Same reasoning as SeedStanding: real vetting is a person at the exchange
// reading licences and insurance certificates, and there is deliberately no
// route that reaches it. This exists so a test can describe a vetted
// contractor without reimplementing the supplier store.
func (b *Board) SeedVetted(worker string) error {
	if b.Suppliers == nil {
		b.Suppliers = NewSuppliers()
	}
	if _, err := b.Suppliers.Upsert(worker, Supplier{
		Kind: KindCompany, LegalName: worker,
	}); err != nil {
		return err
	}
	return b.Suppliers.Vet(worker, "seed", "seeded", true)
}
