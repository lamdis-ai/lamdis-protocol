package cases

import (
	"testing"
	"time"
)

func TestDeadlinesEveryPlan(t *testing.T) {
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	notice := &Fact{Value: "Oct 30, 2026", Date: "2026-10-30", Verified: true}
	cases := []struct {
		plan, service, level, outcome, sent, decided string
		wantKind, wantDate                           string
	}{
		{"employer", "drug", "first", "", "", "", "appeal", "2027-04-28"},           // 180 days
		{"marketplace", "postservice", "first", "", "", "", "appeal", "2027-04-28"}, // 180 days
		{"medicare_advantage", "preservice", "first", "", "", "", "appeal", "2027-01-03"}, // 5 + 60
		{"medicare_part_d", "drug", "first", "", "", "", "appeal", "2027-01-03"},
		{"original_medicare", "postservice", "first", "", "", "", "appeal", "2027-03-04"}, // 5 + 120
		{"medicaid_managed_care", "preservice", "first", "", "", "", "appeal", "2026-12-29"}, // 60
		{"medicaid", "preservice", "first", "", "", "", "appeal", "2027-01-28"},             // 90
		{"fehb", "postservice", "first", "", "", "", "appeal", "2027-04-30"},                // 6 months
		{"tricare", "postservice", "first", "", "", "", "appeal", "2027-01-28"},             // 90
		// Final internal denial on Oct 30: four months has no Feb 30, so March 1.
		{"employer", "preservice", "final", "", "", "", "external", "2027-03-01"},
		// Sent Nov 1, pre-service: the plan has 30 days.
		{"employer", "preservice", "first", "", "2026-11-01", "", "decision", "2026-12-01"},
		{"employer", "postservice", "first", "", "2026-11-01", "", "decision", "2026-12-31"},
		// Lost at the plan: Medicaid managed care goes to a state fair hearing.
		{"medicaid_managed_care", "preservice", "first", "lost", "2026-11-01", "2026-11-10", "external", "2027-02-08"},
	}
	for _, tc := range cases {
		c := &Case{Plan: Plan{Type: tc.plan, Sure: true}, Answers: map[string]string{"urgent": "no"},
			Denial: Denial{Service: tc.service, Level: tc.level}, Facts: Facts{NoticeDate: notice},
			Sent: tc.sent, Decided: tc.decided, Outcome: Outcome{Result: tc.outcome}}
		ds := Deadlines(c, now)
		var got *Deadline
		for i := range ds {
			if ds[i].Kind == tc.wantKind {
				got = &ds[i]
				break
			}
		}
		if got == nil {
			t.Errorf("%s/%s: no %s deadline in %+v", tc.plan, tc.level, tc.wantKind, ds)
			continue
		}
		if got.Date != tc.wantDate {
			t.Errorf("%s/%s/%s: %s = %s (%s), want %s", tc.plan, tc.service, tc.level, tc.wantKind, got.Date, got.Basis, tc.wantDate)
		}
		if got.Source == nil || got.Source.URL == "" {
			t.Errorf("%s: deadline without a source", tc.plan)
		}
	}
	// Medicare Advantage: a no at the plan goes to the independent reviewer
	// by itself, so nothing is due from the person.
	c := &Case{Plan: Plan{Type: "medicare_advantage", Sure: true}, Answers: map[string]string{}, Denial: Denial{Level: "first"},
		Facts: Facts{NoticeDate: notice}, Sent: "2026-11-01", Decided: "2026-11-20", Outcome: Outcome{Result: "lost"}}
	for _, d := range Deadlines(c, now) {
		if d.Kind == "external" {
			t.Errorf("MA should not ask the person to file with the IRE: %+v", d)
		}
	}
}

func TestNoQuestionsBeforeFirstRead(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	c := &Case{Status: StatusReading, Answers: map[string]string{}, StepsDone: map[string]bool{}}
	Refresh(c, now)
	if len(c.Questions) != 0 || c.Denial.Label != "" {
		t.Fatalf("asked before reading: %+v, label %q", c.Questions, c.Denial.Label)
	}
	c.Status = StatusReady
	Refresh(c, now)
	if len(c.Questions) == 0 {
		t.Fatalf("after reading, an unsure plan and no notice date should be asked")
	}
}
