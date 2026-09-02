package exchange

import (
	"context"
	"fmt"
	"log"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
)

// Paying reviewers.
//
// A panel was listed on the board with a fee, seated through AssignReview,
// and answered through the review page — which then told the reviewer the fee
// "reaches your account with your next payout". Nothing credited it. The only
// path that moved money was the evidence path, and a review is not evidence.
// Somebody who verified three panels in ten minutes earned nothing and, because
// nothing freed the seat either, was in cooldown by lunchtime.
//
// This is the settlement for a review: the same ledger, the same idempotent
// key discipline, the same fee and the same holdback as work.

// settleReview credits one recorded review from the panel's escrow.
//
// Idempotent on job and worker, like settle: a retry credits once. The base
// fee is paid for looking, whichever way the answer went, because paying only
// the majority would buy agreement rather than judgement. The bonus, if the
// panel offered one, is paid once the panel is decided, to everybody who
// answered with the finding and felt able to tell.
func (s *Server) settleReview(job, worker string, r api.Review) (int64, error) {
	if s.Ledger == nil || worker == "" {
		return 0, nil
	}
	p, ok := s.Reviews.Panel(job)
	if !ok {
		return 0, fmt.Errorf("review: no such panel %s", job)
	}
	if p.Practice {
		return 0, nil
	}
	l, ok := s.Board.Get(job)
	if !ok {
		return 0, fmt.Errorf("review: %s is not on the board", job)
	}
	ctx := context.Background()
	key := "review:" + job + ":" + worker
	var paid int64
	if done, err := s.Ledger.Applied(ctx, key); err != nil {
		return 0, err
	} else if !done {
		held, err := s.Ledger.Held(ctx, job, l.Currency)
		if err != nil {
			return 0, err
		}
		gross := p.FeeMinor
		if gross > held {
			// The escrow is the ceiling on what a panel can cost.
			gross = held
		}
		if gross > 0 {
			fee := gross * FeeBP / 10000
			if _, err := s.Ledger.Capture(ctx, key, job, worker, gross, fee, l.Currency); err != nil {
				return 0, err
			}
			if s.Holdbacks != nil {
				s.Holdbacks.Add(job, worker, gross-fee, l.Currency, s.now(), s.disputeWindow())
			}
			paid = gross - fee
		}
	}

	// The bonus and the remainder wait for the whole panel: a seat that is
	// taken and not yet answered is still owed its fee, and releasing the
	// escrow on the strength of Taken >= Slots would leave the last reviewer
	// unpaid.
	t := s.Reviews.Tally(job)
	if !t.Complete {
		return paid, nil
	}
	if p.BonusMinor > 0 && t.Decided {
		for _, rv := range t.Reviews {
			if rv.Worker == "" || !rv.Confident || rv.Finding != t.Finding {
				continue
			}
			bkey := "review-bonus:" + job + ":" + rv.Worker
			if done, _ := s.Ledger.Applied(ctx, bkey); done {
				continue
			}
			held, err := s.Ledger.Held(ctx, job, l.Currency)
			if err != nil || held <= 0 {
				break
			}
			gross := p.BonusMinor
			if gross > held {
				gross = held
			}
			fee := gross * FeeBP / 10000
			if _, err := s.Ledger.Capture(ctx, bkey, job, rv.Worker, gross, fee, l.Currency); err != nil {
				log.Printf("review: bonus for %s on %s: %v", rv.Worker, job, err)
				continue
			}
			if s.Holdbacks != nil {
				s.Holdbacks.Add(job, rv.Worker, gross-fee, l.Currency, s.now(), s.disputeWindow())
			}
			if rv.Worker == worker {
				paid += gross - fee
			}
		}
	}
	// Whatever the panel did not pay goes back to whoever funded it, if that
	// is known. A panel opened by the exchange itself has no buyer to refund.
	if _, ok := s.buyerOf(job); ok {
		if err := s.releaseIfDone(ctx, l); err != nil {
			log.Printf("review: releasing the rest of %s: %v", job, err)
		}
	}
	return paid, nil
}
