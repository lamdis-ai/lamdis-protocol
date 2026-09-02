package exchange

import (
	"strings"
	"testing"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
)

// An observation of an address pays on admissibility alone, so the house
// number is the only thing saying the photograph is of that address. It is
// required even when the exchange derived it from the address itself.
func TestAnObservationOfAnAddressNeedsTheHouseNumber(t *testing.T) {
	img := testJPEG(t)
	kindOf := func(kind string) func(string) (string, string, bool) {
		return func(string) (string, string, bool) { return "is the sign up", kind, true }
	}
	derived := &api.SiteMark{Text: "812", Derived: true, Note: "the property number"}

	// Code legible, number nowhere: refused, and told which number.
	v := &SubmissionVerifier{Vision: &fakeVision{text: []string{"MRCPFJ"}}, Predicate: kindOf(api.KindObserve)}
	s := sub("MRCPFJ")
	s.SiteMark = derived
	got, err := v.Verify(withImage(s, img))
	if err == nil || got.Verified {
		t.Fatalf("an observation with no house number in frame was accepted: %+v", got)
	}
	if !strings.Contains(err.Error(), "812") {
		t.Errorf("the refusal does not name the number: %v", err)
	}

	// Code and number legible: accepted, and the mark recorded as seen.
	v = &SubmissionVerifier{Vision: &fakeVision{text: []string{"MRCPFJ", "812 MARLOW ST"}}, Predicate: kindOf(api.KindObserve)}
	s = sub("MRCPFJ")
	s.SiteMark = derived
	got, err = v.Verify(withImage(s, img))
	if err != nil || !got.Verified || !got.MarkSeen {
		t.Fatalf("an observation showing the number was refused: %v %+v", err, got)
	}

	// A do-job still has an adjudicated finding between presence and pay, so
	// a derived mark stays soft there: recorded as unseen, not refused.
	v = &SubmissionVerifier{Vision: &fakeVision{text: []string{"MRCPFJ"}}, Predicate: kindOf(api.KindDo)}
	s = sub("MRCPFJ")
	s.SiteMark = derived
	got, err = v.Verify(withImage(s, img))
	if err != nil || !got.Verified || got.MarkSeen {
		t.Fatalf("a do-job with a derived mark missing: %v %+v", err, got)
	}
}
