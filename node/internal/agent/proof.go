package agent

import (
	"context"
	"time"
)

// A coding task's proof state.
//
// The model saying it is finished is a claim. What the harness can show is
// a level: how much evidence stands behind the change as it is now. Levels
// are ordered and each needs the one below it. Evidence is bound to the
// change's fingerprint, so for a given change the level only rises; when
// the change itself moves, the proof starts again for the new change, and
// the history keeps every change's best level.
//
// The level decides three things: what the report may claim, when the task
// is done (the gate asks for checks and review before it accepts), and
// when an until-stuck run stops (a stretch of turns in which neither the
// level nor the change advanced).

type proofLevel int

const (
	levelOpen         proofLevel = iota // nothing yet
	levelExplored                       // relevant code has been read
	levelChanged                        // a change exists
	levelParses                         // every changed file parses
	levelNoRegression                   // the checks show no failure the change introduced
	levelTested                         // and a test run on this change passed
	levelReviewed                       // and a reviewer found nothing blocking in this diff
)

var levelNames = []string{"open", "explored", "changed", "parses", "no regression", "tested", "reviewed"}

func (l proofLevel) String() string { return levelNames[l] }

// proofStep is one advance, kept in the run record.
type proofStep struct {
	Change string `json:"change"` // fingerprint of the change the evidence is about
	Level  int    `json:"level"`
	Name   string `json:"name"`
	At     string `json:"at"`
}

// level is the evidence that stands behind the current change.
func (h *harness) level(ctx context.Context) (proofLevel, string) {
	files := h.changed(ctx)
	if len(files) == 0 {
		if len(h.read) > 0 {
			return levelExplored, ""
		}
		return levelOpen, ""
	}
	fp := fingerprint(files)
	for _, f := range files {
		if ok, _ := parses(ctx, f); !ok {
			return levelChanged, fp
		}
	}
	if h.verifiedAt != fp || len(h.failing()) > 0 || len(h.checks) == 0 {
		return levelParses, fp
	}
	tested := false
	for _, c := range h.checks {
		if c.Kind == "test" && c.Status == "passed" {
			tested = true
		}
	}
	if !tested {
		return levelNoRegression, fp
	}
	if h.review == nil || h.reviewedAt != fp || len(h.review.blocking()) > 0 {
		return levelTested, fp
	}
	return levelReviewed, fp
}

// advance records the current level when it is higher than anything this
// change has reached, and reports the level.
func (h *harness) advance(ctx context.Context) proofLevel {
	l, fp := h.level(ctx)
	best := levelOpen
	for _, s := range h.proof {
		if s.Change == fp && proofLevel(s.Level) > best {
			best = proofLevel(s.Level)
		}
	}
	if l > best || (len(h.proof) == 0 && l > levelOpen) {
		h.proof = append(h.proof, proofStep{Change: fp, Level: int(l), Name: l.String(), At: time.Now().UTC().Format(time.RFC3339)})
	}
	return l
}
