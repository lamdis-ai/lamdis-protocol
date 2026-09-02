package exchange

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/ledger"
)

// Settlement is the step the marketplace was missing entirely.
//
// A worker could sign in, take a job, submit evidence that passed every check,
// and be credited nothing — the escrow simply sat there. Everything else was
// theatre without this: the console showed zero because there was nothing to
// show, and no amount of verification meant anything if it never moved money.
//
// The two job kinds settle differently, and the difference is the same one
// that governs how they are priced. An observation pays a fee for admissible
// evidence whichever way the answer turns out, so that honest evidence of "no"
// is worth as much as "yes" and nobody learns to tell buyers what they want to
// hear. A do-job pays on completion, because there the worker controls the
// answer and paying regardless would be paying for nothing.

// FeeBP is the exchange's cut of what a worker earns, in basis points.
//
// This is the one a worker feels, applied at settlement. Options.FeeBP is a
// different number for the quoting engine, and the two being separately named
// "FeeBP" is a trap: quoting one while charging the other would understate or
// overstate every payout. If they are ever meant to be the same figure, make
// them the same constant.
// Zero, deliberately, and not forever.
//
// Two and a half percent of nothing is nothing. What the fee actually bought
// was a reason for the first operators to look at this and decide it was not
// worth the trouble — and the first operators are the entire product. There is
// no supply to take a cut from until somebody has built it, and charging for
// the privilege of building it is the wrong way round.
//
// It goes back up when there is enough work going through here that we have
// earned it, and that is said out loud on the site rather than left as a
// surprise. Whoever raises it should change this constant and that sentence in
// the same commit.
const FeeBP = 0

// settle moves money for one accepted or rejected submission.
//
// It is idempotent through the ledger: the key is derived from the job and the
// worker, so a retry after a crash credits once. Nothing here writes to the
// ledger twice for the same submission even if verification runs again.
func (s *Server) settle(ctx context.Context, job string, sub api.Submission, worker string) error {
	if s.Ledger == nil {
		return nil
	}
	l, ok := s.Board.Get(job)
	if !ok {
		return fmt.Errorf("settle: no such job %s", job)
	}
	// A job bought through agentic checkout is paid from an authorisation on
	// the buyer's card, not from a balance. Taking it here means the money
	// moves at the same moment and on the same condition as every other
	// settlement, rather than on a path of its own that could drift.
	defer s.SettleACP(ctx, job, sub.Verified)

	key := "settle:" + job + ":" + sub.Holder
	// Already settled. The ledger would deduplicate the credit anyway, but the
	// escrow check below would fail first and turn a harmless retry into an
	// error the caller has to interpret.
	if done, err := s.Ledger.Applied(ctx, key); err != nil {
		return err
	} else if done {
		return s.releaseIfDone(ctx, l)
	}

	held, err := s.Ledger.Held(ctx, job, l.Currency)
	if err != nil {
		return err
	}
	if held <= 0 {
		return fmt.Errorf("settle: nothing is held for %s", job)
	}

	gross := earnedFor(l, sub)
	// An attempt is a wasted trip, and a wasted trip happens once.
	//
	// The staged path returned the job's attempt fee for every stage, and an
	// attempted submission still proves presence — so it advanced the stage
	// and the next one could be attempted too. Four stages paid four attempt
	// fees for one visit that achieved nothing.
	if sub.Attempted && s.alreadyAttempted(job, sub) {
		gross = 0
		sub.Why = "the attempt fee for this job has already been paid"
	}
	if gross > held {
		// Never pay out more than was committed, whatever the terms say. The
		// escrow is the ceiling on what this job can cost.
		gross = held
	}
	if gross > 0 {
		fee := gross * FeeBP / 10000
		if _, err := s.Ledger.Capture(ctx, key, job, worker, gross, fee, l.Currency); err != nil {
			return err
		}
		// Earned, but not yet safe to send. The buyer has a window to look at
		// what came back; a refund cannot claw money out of somebody's bank.
		if s.Holdbacks != nil {
			s.Holdbacks.Add(job, worker, gross-fee, l.Currency, s.now(), s.disputeWindow())
		}
		s.notifySettled(l, sub, worker, gross-fee)
	} else {
		s.notifyRejected(sub, sub.Why, false)
	}

	// Whatever this submission did not earn goes back, but only once the job
	// can no longer be worked: an unfilled seat may still be taken by somebody
	// else, and releasing early would leave the next worker unfunded.
	return s.releaseIfDone(ctx, l)
}

// earnedFor is what one submission is worth under this job's terms.
func earnedFor(l *api.Listing, sub api.Submission) int64 {
	return workFor(l, sub) + expensesFor(l, sub)
}

// expensesFor is what the worker gets back for money they laid out.
//
// Bounded by the cap the buyer escrowed, and paid only alongside work that was
// actually accepted or attempted — otherwise the expense line is a way to be
// paid for submitting nothing.
func expensesFor(l *api.Listing, sub api.Submission) int64 {
	if sub.ExpenseMinor <= 0 || l.ExpenseCapMinor <= 0 {
		return 0
	}
	// An attempt has to be evidenced too, or the expense line pays for a claim
	// that somebody drove somewhere.
	if !sub.Verified {
		return 0
	}
	if sub.ExpenseMinor > l.ExpenseCapMinor {
		return l.ExpenseCapMinor
	}
	return sub.ExpenseMinor
}

func workFor(l *api.Listing, sub api.Submission) int64 {
	// A written answer to a job that asked for one is paid like an
	// observation: the fee for an admissible answer, whatever it says. There
	// is nothing to adjudicate a photograph against, and no bonus, because
	// nothing independent of the worker established the finding.
	if len(l.Report) > 0 && sub.ReportOnly() {
		if !sub.Verified {
			return 0
		}
		return l.PayMinor
	}
	switch l.Kind {
	case api.KindDo:
		// A staged job pays for the piece that was just evidenced, not for the
		// whole thing. Waiting until the last stage would leave a crew three
		// days and forty tons of asphalt out of pocket on work already done
		// and already accepted.
		if l.Staged() {
			st, ok := l.StageAt(sub.Stage)
			if !ok {
				return 0
			}
			if sub.Attempted {
				if !sub.Verified {
					return 0
				}
				return l.AttemptMinor
			}
			// A materials stage is reimbursement against evidence of purchase.
			// It still has to be evidenced.
			//
			// Skipping adjudication here made it the most exploitable line in
			// the system: Verified means only that a code was legible, so a
			// four-thousand-dollar materials stage paid for a photograph of a
			// code card on a kitchen table. What "materials" changes is *what*
			// must be shown — a receipt rather than finished work — and every
			// stage is already judged against its own deliverable, so nothing
			// needs to be bypassed to get that.
			if sub.Verified && sub.Finding {
				return st.PayMinor
			}
			return 0
		}
		// Order matters, and the obvious order is an exploit.
		//
		// Verification asks whether the evidence is admissible — the challenge
		// code is legible, the location matches. An attempt passes all of that
		// by design: the worker really was at the locked gate, and really did
		// photograph it with the code in frame. Checking Verified first would
		// therefore pay the full completion fee to anybody who turned up,
		// photographed the front of the property, and ticked "could not
		// finish".
		//
		// So a declared attempt earns the attempt fee even when the evidence
		// is perfect, because what it evidences is having been there rather
		// than having done the work.
		if sub.Attempted {
			if !sub.Verified {
				// Claiming a wasted trip is not the same as proving one.
				return 0
			}
			return l.AttemptMinor
		}
		// Admissible evidence establishes that somebody was there. The finding
		// establishes that the thing asked for is now true. A do-job needs
		// both: paying the completion fee on presence alone was the exchange
		// selling verified outcomes and settling on verified attendance.
		if sub.Verified && sub.Finding {
			return l.PayMinor
		}
		return 0
	default:
		// An observation pays for admissible evidence regardless of the
		// finding, plus a bonus when the predicate holds. Splitting it this
		// way is what stops a market of strangers converging on "yes".
		if !sub.Verified {
			return 0
		}
		if sub.Finding {
			return l.PayMinor + l.BonusMinor
		}
		return l.PayMinor
	}
}

// releaseIfDone returns the remaining escrow to the buyer once a job is
// finished with — every seat used, or the listing expired.
//
// Money left in an escrow nobody can claim is money taken from a buyer for
// nothing, and before this it stayed there forever.
func (s *Server) releaseIfDone(ctx context.Context, l *api.Listing) error {
	if !l.Finished(s.now()) {
		return nil
	}
	held, err := s.Ledger.Held(ctx, l.Job, l.Currency)
	if err != nil {
		return err
	}
	if held <= 0 {
		// Everything escrowed was paid out. A card behind it is charged in
		// full now; a balance behind it has nothing to return.
		s.settleCard(ctx, l, 0)
		return nil
	}
	buyer, ok := s.buyerOf(l.Job)
	if !ok {
		return fmt.Errorf("settle: no buyer recorded for %s", l.Job)
	}
	if _, err = s.Ledger.Release(ctx, "release:"+l.Job, l.Job, buyer, held, l.Currency); err != nil {
		return err
	}
	// A card-funded job: the released remainder is not real money, and the
	// card is charged now for what the job actually paid out.
	s.settleCard(ctx, l, held)
	return nil
}

// buyerOf recalls who funded a job, which is who a refund belongs to.
func (s *Server) buyerOf(job string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.buyers[job]
	return b, ok
}

// Sweep returns the escrow of every job that has run out of time.
//
// Without it an expired job holds its buyer's money forever: nothing else in
// the system ever looks at a listing again once it stops being claimable.
func (s *Server) Sweep(ctx context.Context) (released int, err error) {
	// Jobs posted without an account whose card never came.
	s.sweepPending()
	for _, l := range s.Board.All() {
		if !l.Finished(s.now()) {
			continue
		}
		held, herr := s.Ledger.Held(ctx, l.Job, l.Currency)
		if herr != nil || held <= 0 {
			continue
		}
		if rerr := s.releaseIfDone(ctx, l); rerr != nil {
			err = rerr
			continue
		}
		s.notifyExpired(l, held)
		released++
	}
	return released, err
}

// StartSweeper returns escrow from finished jobs on a timer.
//
// The service had no background work of any kind before this, which is why an
// expired job held its buyer's money forever: nothing ever looked at a listing
// again once it stopped being claimable. It stops with the context so a
// shutdown does not leave a goroutine writing to a closing ledger.
func (s *Server) StartSweeper(ctx context.Context, every time.Duration) {
	if s.Ledger == nil {
		return
	}
	if every <= 0 {
		every = 5 * time.Minute
	}
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				// Seats first: a lapsed claim that goes back on the board is
				// a job that can still be done, which is better for everyone
				// than an escrow refunded because nobody was allowed to try.
				if freed := s.Board.ExpireLapsedClaims(); freed > 0 {
					log.Printf("sweep: returned %d unused seats to the board", freed)
				}
				if n, err := s.Sweep(ctx); err != nil {
					log.Printf("sweep: released %d, then failed: %v", n, err)
				} else if n > 0 {
					log.Printf("sweep: returned the escrow of %d finished jobs", n)
				}
			}
		}
	}()
}

var _ = ledger.PayableOf

// disputeWindow is how long a buyer has to object before earnings can be sent.
//
// Long enough that somebody who was at work when the job finished still gets a
// look; short enough that a worker is not financing the buyer's convenience.
// A buyer who is happy can release immediately, and most will.
func (s *Server) disputeWindow() time.Duration {
	if s.DisputeWindow > 0 {
		return s.DisputeWindow
	}
	return 24 * time.Hour
}

// alreadyAttempted reports whether this holder has been paid an attempt fee on
// this job before.
func (s *Server) alreadyAttempted(job string, current api.Submission) bool {
	for _, prior := range s.Submissions(job) {
		if prior.Holder != current.Holder || !prior.Attempted {
			continue
		}
		// The submission being settled is already in the list; the earlier one
		// is the one that matters.
		if prior.Stage != current.Stage && prior.Verified {
			return true
		}
	}
	return false
}
