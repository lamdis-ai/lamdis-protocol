package exchange

import (
	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/verify"
)

// What the receipt says about a written report.
//
// The verification block was written for photographs: a code tied to a time,
// metadata tied to a place, a describer that read the frame. A report has none
// of that. Leaving the block as it was would have a receipt asserting "the
// evidence carries a code issued privately for this job" over a table somebody
// typed, which is the one thing a receipt must never do.
func annotateReportReceipt(out map[string]any, subs []api.Submission) {
	var reports, withPhotos int
	for _, sub := range subs {
		if len(sub.Report) == 0 {
			continue
		}
		reports++
		if len(sub.Artifacts) > 0 {
			withPhotos++
		}
	}
	if reports == 0 {
		return
	}
	limits, _ := out["limits"].([]string)
	if withPhotos == reports {
		limits = append(limits,
			"the written answers are the worker's own; the photograph ties "+
				"the visit to a time, not the answers to the truth")
		out["limits"] = limits
		out["report"] = map[string]any{"submissions": reports, "tier_reached": out["tier_requested"]}
		return
	}
	// At least one answer arrived with no photograph behind it. Say what that
	// establishes, which is very little, and replace the photographic claims
	// entirely rather than listing them beside a caveat.
	out["tier_reached"] = string(verify.TierV0)
	out["confidence_ceiling"] = verify.TierV0.Ceiling(false)
	out["ceiling_because"] = "a written report carries no photograph, so nothing " +
		"ties it to a time or a place. It is a signed claim by the person who " +
		"held the job: tier V0"
	out["established"] = []string{
		"the answers were submitted by the holder of this job's capability, " +
			"which was issued when they took the job",
	}
	limits = append(limits,
		"no challenge code was photographed, so nothing establishes when the "+
			"answers were gathered",
		"no image was read, so nothing independent of the worker corroborates "+
			"the answers")
	out["limits"] = limits
	out["report"] = map[string]any{"submissions": reports, "tier_reached": string(verify.TierV0)}
}
