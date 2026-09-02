package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func reportBoard(t *testing.T) (*Board, *http.ServeMux, *Submission) {
	t.Helper()
	caps := NewCapabilities()
	b := NewBoard(caps)
	var got Submission
	ws := &WorkServer{
		Caps: caps, Board: b,
		Submit: func(sub Submission) (Submission, error) {
			got = sub
			sub.Verified = true
			sub.Reached = "V0"
			return sub, nil
		},
	}
	mux := http.NewServeMux()
	ws.Register(mux)
	if err := b.Post(&Listing{
		Job: "find-1", Kind: KindObserve, Title: "Quotes for a new water heater",
		Instructions: "Call three local installers", Deliverable: "3 results",
		PayMinor: 1500, Currency: "USD", Slots: 1, Expires: time.Now().Add(time.Hour),
		Report: []ReportField{
			{Name: "provider", Label: "Who", Kind: FieldText, Required: true, Repeats: true},
			{Name: "price", Label: "Quoted price", Kind: FieldMoney, Repeats: true},
			{Name: "notes", Label: "Anything else", Kind: FieldText, Repeats: true},
		},
	}); err != nil {
		t.Fatal(err)
	}
	return b, mux, &got
}

// The one job a person can do from a chair used to be undeliverable: the
// submit route demanded a photograph carrying the code, and a table of quotes
// is not a photograph.
func TestAReportJobTakesAReportWithNoPhotograph(t *testing.T) {
	b, mux, got := reportBoard(t)
	secret, _, err := b.Claim("find-1", "caller")
	if err != nil {
		t.Fatal(err)
	}

	// The brief carries the fields, so the page can draw the form.
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, capRequest(t, "GET", "/v1/work/find-1", "find-1", secret, nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"report"`) {
		t.Fatalf("brief: %d %s", w.Code, w.Body.String())
	}

	rows := []map[string]string{
		{"provider": "Acme Plumbing", "price": "1450.00", "notes": "Tuesday"},
		{"provider": "Bob's Heating", "price": "1300.00", "ignored": "not a field"},
		{"provider": "Northside", "notes": "no answer, left a message"},
	}
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, capRequest(t, "POST", "/v1/work/find-1/submit", "find-1", secret,
		map[string]any{"report": rows}))
	if w.Code != http.StatusOK {
		t.Fatalf("submit: %d %s", w.Code, w.Body.String())
	}
	if len(got.Artifacts) != 0 || len(got.Report) != 3 {
		t.Fatalf("stored %d files and %d rows", len(got.Artifacts), len(got.Report))
	}
	if got.Report[1]["ignored"] != "" {
		t.Error("a field the job did not ask for rode in with the answers")
	}
	if got.Report[0]["provider"] != "Acme Plumbing" {
		t.Errorf("row 0: %v", got.Report[0])
	}
	var out map[string]any
	json.Unmarshal(w.Body.Bytes(), &out)
	if out["reached"] != "V0" || out["status"] != "accepted" {
		t.Fatalf("the worker is told %v", out)
	}
	if out["amount_minor"] != float64(1500) {
		t.Fatalf("amount %v, want 1500", out["amount_minor"])
	}
}

// A required field missing is a refusal with the field named, not a
// submission spent.
func TestAReportMissingARequiredFieldIsRefusedAndNotSpent(t *testing.T) {
	b, mux, got := reportBoard(t)
	secret, _, err := b.Claim("find-1", "caller")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, capRequest(t, "POST", "/v1/work/find-1/submit", "find-1", secret,
		map[string]any{"report": []map[string]string{{"price": "12.00"}}}))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "Who") {
		t.Fatalf("missing provider: %d %s", w.Code, w.Body.String())
	}
	if got.Job != "" {
		t.Fatal("a refused report reached the store")
	}
	// Nothing at all is told which of the two it is missing.
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, capRequest(t, "POST", "/v1/work/find-1/submit", "find-1", secret, nil))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "report") {
		t.Fatalf("empty: %d %s", w.Code, w.Body.String())
	}
	// And the worker can still submit properly afterwards.
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, capRequest(t, "POST", "/v1/work/find-1/submit", "find-1", secret,
		map[string]any{"report": []map[string]string{{"provider": "Acme"}}}))
	if w.Code != http.StatusOK {
		t.Fatalf("after fixing it: %d %s", w.Code, w.Body.String())
	}
}

// A photo-only job stays a photo-only job.
func TestAPhotoJobStillNeedsAPhoto(t *testing.T) {
	caps := NewCapabilities()
	b := NewBoard(caps)
	ws := &WorkServer{Caps: caps, Board: b,
		Submit: func(sub Submission) (Submission, error) { return sub, nil }}
	mux := http.NewServeMux()
	ws.Register(mux)
	if err := b.Post(&Listing{
		Job: "obs-1", Kind: KindObserve, Title: "is the sign up", PayMinor: 500,
		Currency: "USD", Slots: 1, Expires: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	secret, _, _ := b.Claim("obs-1", "w")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, capRequest(t, "POST", "/v1/work/obs-1/submit", "obs-1", secret,
		map[string]any{"report": []map[string]string{{"anything": "yes"}}}))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "photo") {
		t.Fatalf("a report was accepted for a job that wanted a photograph: %d %s",
			w.Code, w.Body.String())
	}
}

func TestValidateReportBounds(t *testing.T) {
	fields := []ReportField{{Name: "a", Required: true}}
	if _, err := ValidateReport(fields, nil); err == nil {
		t.Error("an empty report passed")
	}
	if _, err := ValidateReport(nil, []ReportRow{{"a": "x"}}); err == nil {
		t.Error("a job with no fields took a report")
	}
	if _, err := ValidateReport(fields, []ReportRow{{"a": strings.Repeat("x", MaxReportValueLen+1)}}); err == nil {
		t.Error("an oversized value passed")
	}
	if _, err := ValidateReport(fields, []ReportRow{{"a": "   "}}); err == nil {
		t.Error("whitespace satisfied a required field")
	}
	many := make([]ReportRow, MaxReportRows+1)
	for i := range many {
		many[i] = ReportRow{"a": "x"}
	}
	if _, err := ValidateReport(fields, many); err == nil {
		t.Error("too many rows passed")
	}
}

// The work page must render the form and submit without an upload.
func TestWorkPageHandlesReportJobs(t *testing.T) {
	for _, need := range []string{"b.report", "readReport", "reportComplete", "finalize(\"\")"} {
		if !strings.Contains(workPageHTML, need) {
			t.Errorf("the work page has no %s; report jobs cannot be submitted from it", need)
		}
	}
	if !strings.Contains(workPageHTML, "b.practice") {
		t.Error("the work page does not say when a job is a practice run")
	}
}
