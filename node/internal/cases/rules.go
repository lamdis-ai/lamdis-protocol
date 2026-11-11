package cases

import (
	"embed"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// rules.json holds the regulations, each quoted word for word from eCFR, so
// a deadline is always the rule's and never the model's. rules_fallback.json
// is only there so the package builds before rules.json exists.
//
//go:embed *.json
var rulesFS embed.FS

// Rule is one regulation that sets a deadline or gives a right.
type Rule struct {
	ID      string   `json:"id"`
	Plans   []string `json:"plans"`
	Kind    string   `json:"kind"` // appeal | decision | external | right
	Label   string   `json:"label"`
	Days    *int     `json:"days"`
	Unit    string   `json:"unit"`
	From    string   `json:"from"`
	Track   string   `json:"track"`
	Service string   `json:"service"`
	Basis   string   `json:"basis"`
	Cite    string   `json:"cite"`
	URL     string   `json:"url"`
	Quote   string   `json:"quote"`
	Notes   string   `json:"notes"`
	// PresumedReceipt is how many days after the date on a notice the rule
	// presumes it arrived (Medicare counts from receipt, presumed 5 days).
	PresumedReceipt int `json:"presumed_receipt_days"`
}

// nextLevel reports whether a rule belongs to a level beyond the plan's own
// appeal: external review, OPM, a QIC or IRE, a hearing, or an exception.
func (r Rule) nextLevel() bool {
	for _, w := range []string{"external", "opm", "qic", "ire_", "hearing", "exception", "alj", "formal_review"} {
		if strings.Contains(r.ID, w) {
			return true
		}
	}
	return false
}

// judge reports whether a rule is a hearing before a judge, which comes
// after the independent level this engine tracks.
func (r Rule) judge() bool { return strings.Contains(r.ID, "alj") || strings.Contains(r.ID, "hearing_request") && !strings.Contains(r.ID, "fair_hearing") }

func (r Rule) appliesTo(plan string) bool {
	for _, p := range r.Plans {
		if p == plan {
			return true
		}
	}
	return false
}

// Rules is every rule, loaded once.
var Rules = loadRules()

func loadRules() []Rule {
	for _, name := range []string{"rules.json", "rules_fallback.json"} {
		raw, err := rulesFS.ReadFile(name)
		if err != nil {
			continue
		}
		var f struct {
			Rules []Rule `json:"rules"`
		}
		if json.Unmarshal(raw, &f) == nil && len(f.Rules) > 0 {
			return f.Rules
		}
	}
	return nil
}

// RuleByID finds a rule.
func RuleByID(id string) *Rule {
	for i := range Rules {
		if Rules[i].ID == id {
			return &Rules[i]
		}
	}
	return nil
}

// Plan types and how they read.
var planLabels = map[string]string{
	"employer":              "Plan through a job (yours or a family member's)",
	"government_employer":   "Plan through a state or local government job",
	"marketplace":           "Plan you bought yourself (HealthCare.gov, a state marketplace, or directly)",
	"medicare_advantage":    "Medicare Advantage plan",
	"medicare_part_d":       "Medicare drug plan (Part D)",
	"original_medicare":     "Original Medicare",
	"medicaid_managed_care": "Medicaid health plan",
	"medicaid":              "Medicaid",
	"fehb":                  "Federal employee plan (FEHB)",
	"tricare":               "TRICARE",
	"unknown":               "Not sure yet",
}

// PlanLabel is how a plan type reads.
func PlanLabel(t string) string {
	if l, ok := planLabels[t]; ok {
		return l
	}
	return planLabels["unknown"]
}

// ValidPlan reports whether t is a plan type the rules know.
func ValidPlan(t string) bool { _, ok := planLabels[t]; return ok }

var categoryLabels = map[string]string{
	"medical_necessity": "Not medically necessary",
	"experimental":      "Experimental or investigational",
	"out_of_network":    "Out of network",
	"no_prior_auth":     "No prior authorization",
	"exclusion":         "Not covered by the plan",
	"step_therapy":      "Try other treatments first (step therapy)",
	"coding":            "Billing or coding problem",
	"eligibility":       "Coverage or eligibility problem",
	"other":             "Other reason",
}

// CategoryLabel is how a denial category reads.
func CategoryLabel(c string) string {
	if l, ok := categoryLabels[c]; ok {
		return l
	}
	return categoryLabels["other"]
}

func parseDay(s string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(s))
	return t, err == nil
}

func day(t time.Time) string { return t.Format("2006-01-02") }

func daysBetween(from, to time.Time) int {
	a := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	b := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	return int(math.Round(b.Sub(a).Hours() / 24))
}

// noticeDate is the date on the denial, from the person if they told us,
// else from the letter.
func noticeDate(c *Case) (time.Time, bool) {
	if v := c.Answers["notice_date"]; v != "" {
		if t, ok := parseDay(v); ok {
			return t, true
		}
	}
	if c.Facts.NoticeDate != nil && c.Facts.NoticeDate.Date != "" {
		return parseDay(c.Facts.NoticeDate.Date)
	}
	return time.Time{}, false
}

// track is standard or expedited.
func track(c *Case) string {
	if c.Answers["urgent"] == "yes" || (c.Answers["urgent"] == "" && c.Denial.Urgent) {
		return "expedited"
	}
	return "standard"
}

func matches(r Rule, plan, trk, service string) bool {
	if !r.appliesTo(plan) {
		return false
	}
	if r.Track != "" && r.Track != "any" && r.Track != trk {
		return false
	}
	if r.Service != "" && r.Service != "any" && service != "" && r.Service != service {
		// A drug denial is still a pre-service request when no separate
		// drug rule exists for this plan.
		if !(service == "drug" && r.Service == "preservice") {
			return false
		}
	}
	return true
}

// clock applies a rule to a starting day.
func clock(r Rule, from time.Time, plan string) (time.Time, int) {
	if r.Days == nil {
		return time.Time{}, 0
	}
	n := *r.Days
	if strings.HasPrefix(r.Unit, "hour") {
		return from.Add(time.Duration(n) * time.Hour), n
	}
	start := from
	// A rule counted from receipt may presume receipt some days after the
	// date on the notice (Medicare: five).
	if r.From == "notice_received" || r.From == "final_denial" {
		if r.PresumedReceipt > 0 {
			start = start.AddDate(0, 0, r.PresumedReceipt)
		} else if plan == "original_medicare" && r.From == "notice_received" {
			start = start.AddDate(0, 0, 5)
		}
	}
	if strings.HasPrefix(r.Unit, "month") {
		return addMonths(start, n), 0
	}
	return start.AddDate(0, 0, n), 0
}

// addMonths counts calendar months the way the regulations do: when the
// later month has no matching day (October 30 plus four months), the date
// is the first day of the month after.
func addMonths(t time.Time, n int) time.Time {
	y, m, d := t.Date()
	target := time.Date(y, m+time.Month(n), 1, 0, 0, 0, 0, t.Location())
	last := target.AddDate(0, 1, -1).Day()
	if d > last {
		return target.AddDate(0, 1, 0)
	}
	return time.Date(target.Year(), target.Month(), d, 0, 0, 0, 0, t.Location())
}

func sourceOf(r Rule) *Source {
	if r.Cite == "" {
		return nil
	}
	return &Source{Title: r.Cite, URL: r.URL}
}

// pick returns the matching rules of a kind for one level (the plan's own
// appeal, or the independent level after it), preferring the most specific
// service and track, and never a hearing before a judge.
func pick(kind, plan, trk, service string, next bool) []Rule {
	find := func(withService bool) []Rule {
		var out []Rule
		for _, r := range Rules {
			if r.Kind != kind || r.Days == nil || r.judge() {
				continue
			}
			if kind == "decision" && r.nextLevel() != next {
				continue
			}
			svc := service
			if !withService {
				svc = ""
			}
			if matches(r, plan, trk, svc) {
				out = append(out, r)
			}
		}
		return out
	}
	out := find(true)
	if len(out) == 0 {
		out = find(false)
	}
	score := func(r Rule) int {
		s := 0
		if r.Service == service {
			s += 2
		}
		if r.Track == trk {
			s++
		}
		return s
	}
	sort.SliceStable(out, func(i, j int) bool { return score(out[i]) > score(out[j]) })
	if len(out) > 0 {
		best := score(out[0])
		var top []Rule
		for _, r := range out {
			if score(r) == best {
				top = append(top, r)
			}
		}
		return top
	}
	return nil
}

// Deadlines works out the dates that matter for where the case is now.
func Deadlines(c *Case, now time.Time) []Deadline {
	plan := c.Plan.Type
	trk := track(c)
	service := c.Denial.Service
	var out []Deadline
	add := func(d Deadline, t time.Time) {
		d.Date = day(t)
		d.DaysLeft = daysBetween(now, t)
		out = append(out, d)
	}
	nd, haveNotice := noticeDate(c)
	lost := c.Outcome.Result == "lost"
	switch {
	case c.Sent != "" && c.Outcome.Result == "":
		// The appeal is in: the insurer's clock runs.
		sent, ok := parseDay(c.Sent)
		if !ok {
			break
		}
		for _, r := range pick("decision", plan, trk, service, c.Denial.Level == "final" || lost) {
			t, hours := clock(r, sent, plan)
			basis := r.Basis
			if basis == "" {
				basis = fmt.Sprintf("%d days after they get your appeal", *r.Days)
			}
			add(Deadline{ID: "decision", Label: "Your plan should decide by", Basis: basis + " (counted from the day you sent it)",
				Kind: "decision", Source: sourceOf(r), Hours: hours}, t)
			break
		}
	case lost || c.Denial.Level == "final":
		// A final no: the independent level is next.
		from := nd
		ok := haveNotice
		if lost && c.Decided != "" {
			from, ok = parseDay(c.Decided)
		}
		if ok {
			if plan == "medicare_advantage" {
				// The plan must forward a no to Medicare's independent
				// reviewer itself; nothing is due from the person.
				break
			}
			for _, r := range pick("external", plan, trk, service, true) {
				t, hours := clock(r, from, plan)
				add(Deadline{ID: "external", Label: r.Label, Basis: r.Basis, Kind: "external", Source: sourceOf(r), Hours: hours}, t)
				break
			}
		}
	default:
		if haveNotice {
			var best *Deadline
			var bestT time.Time
			for _, r := range pick("appeal", plan, trk, service, false) {
				t, hours := clock(r, nd, plan)
				d := Deadline{ID: "appeal", Label: r.Label, Basis: r.Basis, Kind: "appeal", Source: sourceOf(r), Hours: hours}
				// Counted from when you got it, which we can't know: the
				// date on the letter is the safe stand-in.
				if r.From == "notice_received" && r.PresumedReceipt == 0 && plan != "original_medicare" && d.Basis != "" {
					d.Basis += ", counted here from the date on the letter to be safe"
				}
				if best == nil || t.Before(bestT) {
					best, bestT = &d, t
				}
			}
			if best != nil {
				add(*best, bestT)
			}
		}
	}
	// What the letter itself says wins when it is earlier.
	if c.Facts.StatedDeadline != nil && c.Facts.StatedDeadline.Date != "" && c.Sent == "" && !lost {
		if t, ok := parseDay(c.Facts.StatedDeadline.Date); ok {
			d := Deadline{ID: "letter", Label: "Deadline your letter gives", Basis: "Printed on your denial letter", Kind: "appeal", FromLetter: true}
			add(d, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out
}

// Next is the first date still ahead, the person's own before the insurer's.
func Next(ds []Deadline) *Deadline {
	for _, kind := range [][]string{{"appeal", "external"}, {"decision"}} {
		for i := range ds {
			if ds[i].DaysLeft < 0 {
				continue
			}
			for _, k := range kind {
				if ds[i].Kind == k {
					d := ds[i]
					return &d
				}
			}
		}
	}
	return nil
}

// Rights are the rules a letter may cite for this plan.
func Rights(plan string) []Rule {
	var out []Rule
	for _, r := range Rules {
		if r.appliesTo(plan) && r.Quote != "" && (r.Kind == "right" || r.Kind == "appeal") && len(r.Quote) < 1500 {
			out = append(out, r)
		}
		if len(out) == 16 {
			break
		}
	}
	return out
}

// Questions are what we still need from the person.
func Questions(c *Case) []Question {
	var qs []Question
	if c.Answers == nil {
		c.Answers = map[string]string{}
	}
	if (!c.Plan.Sure || c.Plan.Type == "" || c.Plan.Type == "unknown") && c.Answers["plan_source"] == "" {
		qs = append(qs, Question{ID: "plan_source", Kind: "choice",
			Text: "Where does this insurance come from?",
			Why:  "The rules and your deadline depend on the kind of plan. Your letter didn't make it certain.",
			Options: []Option{
				{"employer", "A job (mine or a family member's)"},
				{"marketplace", "I bought it myself (HealthCare.gov, a state marketplace, or directly)"},
				{"medicare_advantage", "Medicare Advantage (a Medicare plan from a private company)"},
				{"medicare_part_d", "A Medicare drug plan (Part D)"},
				{"original_medicare", "Original Medicare (the red, white and blue card)"},
				{"medicaid", "Medicaid or CHIP"},
				{"fehb", "A federal employee plan"},
				{"tricare", "TRICARE"},
				{"unknown", "I'm not sure"},
			}})
	}
	if _, ok := noticeDate(c); !ok && c.Answers["notice_date"] == "" {
		qs = append(qs, Question{ID: "notice_date", Kind: "date",
			Text: "What date is printed on the denial letter?",
			Why:  "Your appeal deadline is counted from it. We couldn't read it clearly."})
	}
	if (c.Denial.Service == "preservice" || c.Denial.Service == "drug") && c.Answers["urgent"] == "" && c.Sent == "" {
		qs = append(qs, Question{ID: "urgent", Kind: "choice", Optional: true,
			Text: "Could waiting a month or more for a decision seriously harm your health?",
			Why:  "If so, you can ask for a fast (expedited) appeal, decided within about 72 hours. Your doctor can back this up.",
			Options: []Option{{"yes", "Yes"}, {"no", "No"}}})
	}
	if c.Answers["story"] == "" {
		qs = append(qs, Question{ID: "story", Kind: "text", Optional: true,
			Text: "In a sentence or two: why do you need this, and what have you already tried?",
			Why:  "Your own words make the appeal specific. We only use what you write here."})
	}
	return qs
}

// ApplyAnswer records an answer and what it changes.
func ApplyAnswer(c *Case, id, value string) error {
	value = strings.TrimSpace(value)
	if c.Answers == nil {
		c.Answers = map[string]string{}
	}
	switch id {
	case "plan_source":
		if !ValidPlan(value) {
			return Errorf("pick one of the options")
		}
		c.Answers[id] = value
		t := value
		if t == "medicaid" && c.Facts.Insurer != nil {
			// Most Medicaid is run by a health plan company; a denial
			// from one is a managed-care denial with its own rules.
			low := strings.ToLower(c.Facts.Insurer.Value)
			if !strings.Contains(low, "department") && !strings.Contains(low, "state of") && !strings.Contains(low, "agency") {
				t = "medicaid_managed_care"
			}
		}
		c.Plan = Plan{Type: t, Label: PlanLabel(t), Sure: t != "unknown", Why: "You told us."}
	case "notice_date":
		if _, ok := parseDay(value); !ok {
			return Errorf("that date didn't read as a date")
		}
		c.Answers[id] = value
	case "urgent":
		if value != "yes" && value != "no" {
			return Errorf("answer yes or no")
		}
		c.Answers[id] = value
	case "story":
		if len(value) > 2000 {
			value = value[:2000]
		}
		c.Answers[id] = value
	default:
		return Errorf("unknown question")
	}
	return nil
}

// Steps is the checklist for where the case is.
func Steps(c *Case) []Step {
	plan := c.Plan.Type
	nd := Next(c.Deadlines)
	by := ""
	if nd != nil && nd.Kind == "appeal" {
		by = " before " + humanDate(nd.Date)
	}
	var s []Step
	add := func(st Step) {
		st.Done = st.Done || c.StepsDone[st.ID]
		s = append(s, st)
	}
	fileRight := "You have the right to free copies of everything the plan used to decide, including the medical criteria. Call the number on your letter or ask in writing. It often shows exactly what the reviewer thought was missing."
	if plan == "original_medicare" {
		fileRight = "Ask your doctor's office for the records they sent with the claim, and call 1-800-MEDICARE if the notice is unclear about what was missing."
	}
	add(Step{ID: "get_file", Title: "Ask for your claim file", Detail: fileRight, Action: "none"})
	add(Step{ID: "invite_doctor", Title: "Ask your doctor's office for a letter",
		Detail: "A short letter from your doctor explaining why you need this is the strongest thing an appeal can include. Send them your case link: they can read what was denied, edit a draft letter we prepare for them, and upload it straight to your case.",
		Done:   c.Doctor.Uploaded, Action: "invite_doctor"})
	add(Step{ID: "draft_letter", Title: "Draft your appeal",
		Detail: "We write it from your documents and check every factual sentence against them. Anything we can't back up is flagged for you, not sent.",
		Done:   c.Letter.Status == "drafted", Action: "draft_letter"})
	sendHow := "Send it to the address or fax number on your denial letter. Keep a copy and proof you sent it: a fax confirmation page or certified mail receipt."
	if plan == "medicare_advantage" || plan == "medicare_part_d" {
		sendHow += " If waiting could seriously harm your health, call the plan and ask for a fast (expedited) decision too."
	}
	add(Step{ID: "send", Title: "Send your appeal" + by, Detail: sendHow, Done: c.Sent != "", Action: "send"})
	wait := "Write down the date you sent it. If the decision date passes with no answer, call the plan and ask for it; a missed deadline can count in your favor."
	if plan == "medicare_advantage" {
		wait += " If the plan says no again, it has to send your case to an independent reviewer on its own. You don't have to do anything for that."
	}
	add(Step{ID: "decision", Title: "Get the decision", Detail: wait, Done: c.Outcome.Result != "", Action: "record_decision"})
	if c.Outcome.Result == "lost" || c.Denial.Level == "final" {
		ext := "Ask for an independent external review. Someone outside your insurance company decides, and if they say yes, the plan has to pay. It is free in most states."
		switch plan {
		case "medicare_advantage":
			ext = "The plan must forward your case to Medicare's independent reviewer automatically. If that reviewer also says no, the next step is a hearing with a judge (the letter will say how)."
		case "medicare_part_d":
			ext = "Ask Medicare's independent reviewer (the IRE) to reconsider. The decision letter explains how and by when."
		case "original_medicare":
			ext = "Ask for reconsideration by a Qualified Independent Contractor. The decision notice explains how."
		case "medicaid", "medicaid_managed_care":
			ext = "Ask your state for a fair hearing. You can also ask that your benefits continue while you wait if you ask quickly."
		case "fehb":
			ext = "Ask the Office of Personnel Management (OPM) to review the plan's decision."
		}
		add(Step{ID: "external", Title: "Take it to an independent reviewer", Detail: ext, Action: "external_review"})
	}
	return s
}

func humanDate(d string) string {
	t, ok := parseDay(d)
	if !ok {
		return d
	}
	return t.Format("January 2, 2006")
}

// HumanDate formats a YYYY-MM-DD for people.
func HumanDate(d string) string { return humanDate(d) }

// Refresh recomputes everything that follows from the facts and answers.
func Refresh(c *Case, now time.Time) {
	if c.Plan.Type == "" {
		c.Plan.Type = "unknown"
	}
	if v := c.Answers["plan_source"]; v != "" && c.Plan.Why != "You told us." {
		ApplyAnswer(c, "plan_source", v)
	}
	c.Plan.Label = PlanLabel(c.Plan.Type)
	c.Denial.Label = ""
	if c.Denial.Category != "" {
		c.Denial.Label = CategoryLabel(c.Denial.Category)
	}
	c.Questions = Questions(c)
	if c.Status == StatusReading && c.Denial.Category == "" {
		// Nothing read yet: don't ask what the letter may already say.
		c.Questions = []Question{}
	}
	c.Deadlines = Deadlines(c, now)
	c.NextDeadline = Next(c.Deadlines)
	c.Steps = Steps(c)
	c.Title = Title(c)
	if c.Status == StatusReading || c.Status == StatusDrafting || c.Status == StatusError {
		return
	}
	needs := false
	for _, q := range c.Questions {
		if !q.Optional {
			needs = true
		}
	}
	switch {
	case c.Outcome.Result != "":
		c.Status = StatusDecided
	case c.Sent != "":
		c.Status = StatusSent
	case needs:
		c.Status = StatusNeedsAnswers
	case c.Letter.Status == "drafted":
		c.Status = StatusDrafted
	default:
		c.Status = StatusReady
	}
}
