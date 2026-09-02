package api

import (
	"fmt"
	"strings"
)

// A report is the answer to a job whose deliverable is information.
//
// "Get three quotes for a water heater" is not answered by a photograph. It is
// answered by a small table, and the listing already said so: Report on a
// Listing names the fields. What was missing was any way to fill them in — the
// submit route demanded a photo with the challenge code, so the one job a
// person could do from a chair could not be delivered at all, and the buyer
// was charged for a photograph of a code card.

// ReportRow is one row of answers, field name to value. A job that collects
// several of something — three quotes — sends several rows.
type ReportRow map[string]string

// Bounds on a report, so the exchange is not used as storage and a required
// field cannot be satisfied by whitespace.
const (
	MaxReportRows     = 50
	MaxReportValueLen = 2000
)

// ValidateReport checks answers against the fields a listing asked for and
// returns the cleaned rows: trimmed, unknown fields dropped, nothing missing
// that was required.
//
// Field names are the listing's, not the worker's, so a value the buyer's
// agent did not ask for cannot ride in beside the ones it did.
func ValidateReport(fields []ReportField, rows []ReportRow) ([]ReportRow, error) {
	if len(fields) == 0 {
		return nil, fmt.Errorf("this job does not take a written report")
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("the report is empty")
	}
	if len(rows) > MaxReportRows {
		return nil, fmt.Errorf("a report may hold at most %d rows", MaxReportRows)
	}
	known := map[string]ReportField{}
	for _, f := range fields {
		known[f.Name] = f
	}
	out := make([]ReportRow, 0, len(rows))
	for i, row := range rows {
		clean := ReportRow{}
		for name, v := range row {
			if _, ok := known[name]; !ok {
				continue
			}
			v = strings.TrimSpace(v)
			if len(v) > MaxReportValueLen {
				return nil, fmt.Errorf("%s is too long; keep it under %d characters",
					name, MaxReportValueLen)
			}
			if v != "" {
				clean[name] = v
			}
		}
		for _, f := range fields {
			if !f.Required {
				continue
			}
			// A field that does not repeat is asked once, on the first row.
			if !f.Repeats && i > 0 {
				continue
			}
			if clean[f.Name] == "" {
				label := f.Label
				if label == "" {
					label = f.Name
				}
				if len(rows) > 1 {
					return nil, fmt.Errorf("row %d is missing %s", i+1, label)
				}
				return nil, fmt.Errorf("%s is required", label)
			}
		}
		if len(clean) == 0 {
			return nil, fmt.Errorf("row %d is empty", i+1)
		}
		out = append(out, clean)
	}
	return out, nil
}

// ReportOnly reports whether a submission is a written answer with no
// photograph behind it.
//
// Such a submission carries no challenge code, so nothing ties it to a time
// or a place: it is a signed claim by the person who held the job, and the
// receipt says exactly that.
func (s Submission) ReportOnly() bool {
	return len(s.Report) > 0 && len(s.Artifacts) == 0
}
