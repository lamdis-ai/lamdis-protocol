package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A recorded review must free the seat it was assigned and be paid.
//
// Before this, AssignReview took a seat and set a lease that nothing ever
// cleared: Board.Done was reachable only from the evidence path. Every review
// therefore lapsed forty-five minutes after it was recorded, counted as an
// abandonment, put the reviewer in cooldown, and after three reviews left
// their allowance at one for good. Nothing credited the fee either.
func TestARecordedReviewFreesTheSeatAndPaysOnce(t *testing.T) {
	now := time.Now()
	caps := NewCapabilities()
	b := NewBoard(caps)
	b.Now = func() time.Time { return now }
	if err := b.Post(&Listing{
		Job: "panel-1", Parent: "obs-1", Kind: KindReview, Title: "Is the sign up?",
		PayMinor: 150, BonusMinor: 100, Currency: "USD", Slots: 3,
		Expires: now.Add(2 * time.Hour), Posted: now,
	}); err != nil {
		t.Fatal(err)
	}
	reviews := NewReviewStore()
	reviews.Now = b.Now
	reviews.Add(&ReviewPanel{
		Job: "panel-1", Parent: "obs-1", Question: "Is the sign up?",
		Reviewers: 3, Agreement: 2, FeeMinor: 150, BonusMinor: 100, Currency: "USD",
		Expires: now.Add(2 * time.Hour),
	})

	var settled []string
	rs := &ReviewServer{
		Caps: caps, Reviews: reviews, Secrets: b.Secrets, Board: b,
		Now: b.Now,
		Settled: func(job, worker string, r Review) (int64, error) {
			settled = append(settled, job+":"+worker)
			if r.Worker != worker {
				t.Errorf("the review carries worker %q, settled for %q", r.Worker, worker)
			}
			return 150, nil
		},
	}
	mux := http.NewServeMux()
	rs.Register(mux)

	secret, _, err := b.AssignReview("reviewer")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, allowance, _ := b.Standing("reviewer"); allowance != 1 {
		t.Fatalf("a new account holds %d at once, expected 1", allowance)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, capRequest(t, "POST", "/v1/claims/panel-1/review", "panel-1", secret,
		map[string]any{"finding": true, "confident": true,
			"reason": "The sign is clearly mounted above the door and legible."}))
	if w.Code != http.StatusOK {
		t.Fatalf("review: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		Paid    int64 `json:"paid_minor"`
		Payable bool  `json:"payable"`
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	if out.Paid != 150 || !out.Payable {
		t.Fatalf("the page is told %+v; the fee was credited", out)
	}
	if len(settled) != 1 || settled[0] != "panel-1:reviewer" {
		t.Fatalf("settlement calls: %v", settled)
	}

	// The seat is free: they can take another right away.
	done, abandoned, _, cool := b.Standing("reviewer")
	if done != 1 || abandoned != 0 || !cool.IsZero() {
		t.Fatalf("after one review: completed %d, abandoned %d, cooldown %v",
			done, abandoned, cool)
	}
	// And it stays free when the lease would have lapsed: finishing is not
	// abandoning.
	now = now.Add(3 * time.Hour)
	b.ExpireLapsedClaims()
	done, abandoned, allowance, cool := b.Standing("reviewer")
	if abandoned != 0 || !cool.IsZero() {
		t.Fatalf("a finished review lapsed into an abandonment: abandoned %d, cooldown %v",
			abandoned, cool)
	}
	if done != 1 || allowance < 1 {
		t.Fatalf("standing after a review: completed %d, allowance %d", done, allowance)
	}
}

// The demonstration panel is a real page and a real flow with nothing behind
// it. It must free the seat and count for nothing — in either direction.
func TestThePracticePanelCountsForNothing(t *testing.T) {
	now := time.Now()
	caps := NewCapabilities()
	b := NewBoard(caps)
	b.Now = func() time.Time { return now }
	if err := b.Post(&Listing{
		Job: "panel-demo", Kind: KindReview, Title: "Is a pig visible?",
		Currency: "USD", Slots: 3, Expires: now.Add(2 * time.Hour), Posted: now,
	}); err != nil {
		t.Fatal(err)
	}
	reviews := NewReviewStore()
	reviews.Now = b.Now
	reviews.Add(&ReviewPanel{
		Job: "panel-demo", Question: "Is a pig visible?", Reviewers: 3, Agreement: 2,
		Currency: "USD", Practice: true, Expires: now.Add(2 * time.Hour),
	})
	paid := false
	rs := &ReviewServer{
		Caps: caps, Reviews: reviews, Secrets: b.Secrets, Board: b, Now: b.Now,
		Settled: func(string, string, Review) (int64, error) { paid = true; return 1, nil },
	}
	mux := http.NewServeMux()
	rs.Register(mux)
	secret, _, err := b.AssignReview("learner")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, capRequest(t, "POST", "/v1/claims/panel-demo/review", "panel-demo", secret,
		map[string]any{"finding": false, "confident": true,
			"reason": "That is a drawing of a house, and there is no pig in it."}))
	if w.Code != http.StatusOK {
		t.Fatalf("review: %d %s", w.Code, w.Body.String())
	}
	if paid {
		t.Fatal("the practice panel tried to pay somebody")
	}
	var out struct {
		Practice bool  `json:"practice"`
		Paid     int64 `json:"paid_minor"`
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	if !out.Practice || out.Paid != 0 {
		t.Fatalf("the page is told %+v", out)
	}
	now = now.Add(3 * time.Hour)
	b.ExpireLapsedClaims()
	done, abandoned, _, cool := b.Standing("learner")
	if done != 0 || abandoned != 0 || !cool.IsZero() {
		t.Fatalf("a practice review changed standing: completed %d, abandoned %d, cooldown %v",
			done, abandoned, cool)
	}
}

// Practice jobs must not buy the allowance and the ceiling real work earns.
//
// Two photographs of paper on a kitchen table used to count as two clean
// completions: allowance two, and a third of the way to the "proven" tier.
func TestPracticeWorkDoesNotCountTowardStanding(t *testing.T) {
	now := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	b := NewBoard(NewCapabilities())
	b.Now = func() time.Time { return now }
	for i, id := range []string{"practice-1", "practice-2", "practice-3"} {
		if err := b.Post(&Listing{
			Job: id, Kind: KindObserve, Title: "Practice: photograph anything",
			Practice: true, Currency: "USD", Slots: 50,
			Expires: now.Add(24 * time.Hour), Posted: now,
		}); err != nil {
			t.Fatal(err)
		}
		if _, _, err := b.Claim(id, "newcomer"); err != nil {
			t.Fatalf("practice %d: %v", i, err)
		}
		b.Done(id, "newcomer")
	}
	done, _, allowance, _ := b.Standing("newcomer")
	if done != 0 {
		t.Fatalf("three practice runs counted as %d completions", done)
	}
	if allowance != 1 {
		t.Fatalf("three practice runs raised the allowance to %d", allowance)
	}
	if st := b.StandingFor("newcomer"); st.Settled != 0 {
		t.Fatalf("the assurance record shows %d settled jobs after practice only", st.Settled)
	}

	// Real work still counts.
	if err := b.Post(&Listing{
		Job: "real-1", Kind: KindObserve, Title: "Is the sign up", PayMinor: 500,
		Currency: "USD", Slots: 1, Expires: now.Add(24 * time.Hour), Posted: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.Claim("real-1", "newcomer"); err != nil {
		t.Fatal(err)
	}
	b.Done("real-1", "newcomer")
	if done, _, _, _ := b.Standing("newcomer"); done != 1 {
		t.Fatalf("real work counted as %d", done)
	}
}

// A buyer may not lease a job for a month.
func TestLeasesAreCapped(t *testing.T) {
	now := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	b := NewBoard(NewCapabilities())
	b.Now = func() time.Time { return now }
	err := b.Post(&Listing{
		Job: "long", Kind: KindObserve, Title: "watch the paint dry", PayMinor: 500,
		Currency: "USD", Slots: 1, WorkHours: 720, Expires: now.Add(60 * 24 * time.Hour),
	})
	if err == nil {
		t.Fatal("a 720-hour lease was accepted")
	}
	if got := err.Error(); !contains(got, "72") || !contains(got, "work_hours") {
		t.Fatalf("the refusal does not say what the limit is: %v", err)
	}
	if err := b.Post(&Listing{
		Job: "ok", Kind: KindObserve, Title: "three days", PayMinor: 500,
		Currency: "USD", Slots: 1, WorkHours: MaxWorkHours, Expires: now.Add(7 * 24 * time.Hour),
	}); err != nil {
		t.Fatalf("a %d-hour lease was refused: %v", MaxWorkHours, err)
	}
	// The backstop for a listing that reached the board another way.
	over := &Listing{WorkHours: 1000}
	if got := over.LeaseFor(time.Hour); got != time.Duration(MaxWorkHours)*time.Hour {
		t.Fatalf("LeaseFor let %v through", got)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
