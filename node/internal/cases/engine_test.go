package cases

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/agent"
)

const sampleLetter = `--- page 1 ---
Acme Health Plan
Notice date: March 3, 2027
Member: Jordan Rivera   Member ID: XJ4471-02
Reference number: PA-88213
We reviewed the request for Zepbound (tirzepatide) 5 mg.
This request is denied because it is not medically necessary. The member has not tried and failed two formulary alternatives.
You have the right to appeal this decision within 180 days of receiving this notice.
Send appeals to: Acme Health Plan Appeals, PO Box 1200, Dayton, OH 45401. Fax: 1-800-555-0142.`

type fakeModel struct{ calls []string }

func (f *fakeModel) Complete(ctx context.Context, msgs []agent.Message, tools []agent.ToolSpec) (agent.Message, agent.Usage, error) {
	u := agent.Usage{Prompt: 100, Completion: 50}
	if len(tools) == 0 {
		f.calls = append(f.calls, "transcribe")
		return agent.Message{Role: "assistant", Content: sampleLetter}, u, nil
	}
	name := tools[0].Name
	f.calls = append(f.calls, name)
	var args any
	switch name {
	case "submit_reading":
		args = map[string]any{
			"facts": map[string]any{
				"insurer":     map[string]any{"value": "Acme Health Plan", "quote": "Acme Health Plan", "doc": "f1"},
				"patient":     map[string]any{"value": "Jordan Rivera", "quote": "Member: Jordan Rivera", "doc": "f1"},
				"service":     map[string]any{"value": "Zepbound 5 mg", "quote": "request for Zepbound (tirzepatide) 5 mg", "doc": "f1"},
				"notice_date": map[string]any{"value": "March 3, 2027", "date": "2027-03-03", "quote": "Notice date: March 3, 2027", "doc": "f1"},
				"reason":      map[string]any{"value": "not medically necessary", "quote": "This request is denied because it is not medically necessary.", "doc": "f1"},
				// A quote that is not in the letter must not be believed.
				"member_id": map[string]any{"value": "ZZ999", "quote": "Member ID: ZZ999", "doc": "f1"},
			},
			"plan":   map[string]any{"type": "employer", "sure": false, "why": "no plan type stated"},
			"denial": map[string]any{"category": "step_therapy", "plain": "They want you to try two other drugs first.", "level": "first", "urgent": false, "service_kind": "drug"},
			"odds":   map[string]any{"level": "strong", "headline": "Worth appealing.", "reasons": []any{map[string]any{"text": "Step therapy", "quote": "tried and failed two formulary alternatives", "doc": "f1"}}, "alternatives": []any{}},
		}
	case "submit_letter":
		args = map[string]any{
			"subject": "Appeal of denial PA-88213",
			"paragraphs": []any{
				[]any{
					map[string]any{"text": "I am appealing the denial of Zepbound.", "kind": "fact", "cites": []any{map[string]any{"doc": "f1", "quote": "We reviewed the request for Zepbound (tirzepatide) 5 mg."}}},
					map[string]any{"text": "I tried metformin for a year.", "kind": "fact", "cites": []any{map[string]any{"doc": "f1", "quote": "tried metformin for a year"}}},
					map[string]any{"text": "Please send me the criteria you used.", "kind": "request", "cites": []any{}},
				},
			},
			"enclosures": []any{"Denial letter"},
		}
	}
	raw, _ := json.Marshal(args)
	m := agent.Message{Role: "assistant"}
	tc := agent.ToolCall{ID: "1", Type: "function"}
	tc.Function.Name = name
	tc.Function.Arguments = string(raw)
	m.ToolCalls = []agent.ToolCall{tc}
	return m, u, nil
}

func TestReadDraftAndDeadlines(t *testing.T) {
	now := time.Date(2027, 3, 10, 12, 0, 0, 0, time.UTC)
	fm := &fakeModel{}
	e := &Engine{NewModel: func(int) agent.Model { return fm },
		Open: func(id string) ([]byte, string, string, error) { return []byte{0xff, 0xd8}, "image/jpeg", "page1.jpg", nil },
		Now:  func() time.Time { return now }}
	c := &Case{ID: NewID(), Docs: []Doc{{ID: "f1", Kind: KindDenial, Type: "image/jpeg"}}, Answers: map[string]string{}, StepsDone: map[string]bool{}}
	texts := map[string]string{}
	text := func(id string) string { return texts[id] }
	if err := e.ReadAll(context.Background(), c, text, func(id, t string) { texts[id] = t }); err != nil {
		t.Fatal(err)
	}
	if c.Facts.MemberID == nil || c.Facts.MemberID.Verified {
		t.Fatalf("an invented member ID was believed: %+v", c.Facts.MemberID)
	}
	if !c.Facts.NoticeDate.Verified || c.Facts.NoticeDate.Date != "2027-03-03" {
		t.Fatalf("notice date: %+v", c.Facts.NoticeDate)
	}
	Refresh(c, now)
	// The plan type was not certain, so we must ask before giving a date.
	if c.Status != StatusNeedsAnswers || c.Questions[0].ID != "plan_source" {
		t.Fatalf("status %s questions %+v", c.Status, c.Questions)
	}
	if err := ApplyAnswer(c, "plan_source", "employer"); err != nil {
		t.Fatal(err)
	}
	Refresh(c, now)
	if c.NextDeadline == nil || c.NextDeadline.Kind != "appeal" {
		t.Fatalf("no appeal deadline: %+v", c.Deadlines)
	}
	// 180 days from March 3, 2027.
	if c.NextDeadline.Date != "2027-08-30" {
		t.Fatalf("deadline %s, want 2027-08-30", c.NextDeadline.Date)
	}
	l, err := e.Draft(context.Background(), c, text)
	if err != nil {
		t.Fatal(err)
	}
	if l.Sentences[0].Status != SentVerified || l.Sentences[1].Status != SentUnsupported || l.Sentences[2].Status != SentPlain {
		t.Fatalf("statuses: %s %s %s", l.Sentences[0].Status, l.Sentences[1].Status, l.Sentences[2].Status)
	}
	if l.Checked.Facts != 2 || l.Checked.Verified != 1 || l.Checked.Flagged != 1 {
		t.Fatalf("checked %+v", l.Checked)
	}
	// Once sent, the insurer's clock is what matters.
	c.Sent = "2027-03-20"
	c.Letter = l
	Refresh(c, now)
	if c.NextDeadline == nil || c.NextDeadline.Kind != "decision" {
		t.Fatalf("after sending: %+v", c.Deadlines)
	}
}

func TestMedicareAdvantageDeadline(t *testing.T) {
	now := time.Date(2027, 1, 5, 0, 0, 0, 0, time.UTC)
	c := &Case{Plan: Plan{Type: "medicare_advantage", Sure: true}, Answers: map[string]string{},
		Facts: Facts{NoticeDate: &Fact{Value: "Jan 2", Date: "2027-01-02", Verified: true}}}
	Refresh(c, now)
	if c.NextDeadline == nil {
		t.Fatalf("no deadline")
	}
	// 65 days from January 2, 2027.
	if c.NextDeadline.Date != "2027-03-08" {
		t.Fatalf("MA deadline %s, want 2027-03-08", c.NextDeadline.Date)
	}
	// A letter stating an earlier date wins.
	c.Facts.StatedDeadline = &Fact{Value: "by Feb 1", Date: "2027-02-01", Verified: true}
	Refresh(c, now)
	if c.NextDeadline.Date != "2027-02-01" || !c.NextDeadline.FromLetter {
		t.Fatalf("stated deadline should win: %+v", c.NextDeadline)
	}
}

func TestLoadedCaseHasMaps(t *testing.T) {
	dir := t.TempDir()
	st := For(dir)
	c := &Case{ID: NewID()}
	if err := st.Put(c); err != nil {
		t.Fatal(err)
	}
	// Sending before any step is ticked must not hit a nil map.
	if _, err := st.Update(c.ID, func(c *Case) error {
		c.StepsDone["send"] = true
		c.Answers["story"] = "x"
		c.Reminded["a:b:0"] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
