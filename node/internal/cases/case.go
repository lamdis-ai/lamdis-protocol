// Package cases is the engine behind Lamdis's insurance appeals: one case per
// denial, holding the letter and everything else about it, the deadlines the
// rules give, an honest read of the odds, and an appeal whose every factual
// sentence is checked against the documents before anybody sends it.
//
// The model reads and drafts; this package decides. Deadlines come from the
// regulations in rules.json, not from the model. A quote counts only when it
// is found word for word in the text that was read from the document it
// claims to come from. Anything the engine cannot back up is shown as such
// and left out of what gets printed.
package cases

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Status is where a case is.
const (
	StatusReading      = "reading"
	StatusNeedsAnswers = "needs_answers"
	StatusReady        = "ready"
	StatusDrafting     = "drafting"
	StatusDrafted      = "drafted"
	StatusSent         = "sent"
	StatusDecided      = "decided"
	StatusError        = "error"
)

// Doc kinds.
const (
	KindDenial       = "denial"
	KindPolicy       = "policy"
	KindDoctorLetter = "doctor_letter"
	KindRecords      = "records"
	KindInsurerReply = "insurer_reply"
	KindOther        = "other"
)

// ValidKind reports whether k is a document kind people may choose.
func ValidKind(k string) bool {
	switch k {
	case KindDenial, KindPolicy, KindDoctorLetter, KindRecords, KindInsurerReply, KindOther:
		return true
	}
	return false
}

// KindLabel is how a document kind reads to a person.
func KindLabel(k string) string {
	switch k {
	case KindDenial:
		return "Denial letter"
	case KindPolicy:
		return "Plan document"
	case KindDoctorLetter:
		return "Doctor's letter"
	case KindRecords:
		return "Medical records"
	case KindInsurerReply:
		return "Letter from the insurer"
	}
	return "Document"
}

// Doc is one file on a case.
type Doc struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Type    string    `json:"type"`
	Kind    string    `json:"kind"`
	AddedBy string    `json:"added_by"`
	Added   time.Time `json:"added"`
	URL     string    `json:"url"`
	Read    bool      `json:"read"`
}

// Fact is one thing read from a document, with the words that say it.
type Fact struct {
	Value    string `json:"value"`
	Date     string `json:"date,omitempty"` // YYYY-MM-DD when the fact is a date
	Quote    string `json:"quote,omitempty"`
	Doc      string `json:"doc,omitempty"`
	Verified bool   `json:"verified"`
}

// Facts are the things a denial letter usually says.
type Facts struct {
	Insurer        *Fact `json:"insurer,omitempty"`
	PlanName       *Fact `json:"plan_name,omitempty"`
	MemberID       *Fact `json:"member_id,omitempty"`
	ClaimNumber    *Fact `json:"claim_number,omitempty"`
	Patient        *Fact `json:"patient,omitempty"`
	Service        *Fact `json:"service,omitempty"`
	ServiceDate    *Fact `json:"service_date,omitempty"`
	NoticeDate     *Fact `json:"notice_date,omitempty"`
	Reason         *Fact `json:"reason,omitempty"`
	StatedDeadline *Fact `json:"stated_deadline,omitempty"`
	AppealAddress  *Fact `json:"appeal_address,omitempty"`
	AppealFax      *Fact `json:"appeal_fax,omitempty"`
	AppealPhone    *Fact `json:"appeal_phone,omitempty"`
	Provider       *Fact `json:"provider,omitempty"`
}

// each visits every fact that is present, by its JSON name.
func (f *Facts) each(fn func(name string, x *Fact)) {
	for _, p := range []struct {
		n string
		x *Fact
	}{{"insurer", f.Insurer}, {"plan_name", f.PlanName}, {"member_id", f.MemberID}, {"claim_number", f.ClaimNumber},
		{"patient", f.Patient}, {"service", f.Service}, {"service_date", f.ServiceDate}, {"notice_date", f.NoticeDate},
		{"reason", f.Reason}, {"stated_deadline", f.StatedDeadline}, {"appeal_address", f.AppealAddress},
		{"appeal_fax", f.AppealFax}, {"appeal_phone", f.AppealPhone}, {"provider", f.Provider}} {
		if p.x != nil {
			fn(p.n, p.x)
		}
	}
}

// Plan is what kind of coverage this is, which decides the rules.
type Plan struct {
	Type  string `json:"type"`
	Label string `json:"label"`
	Sure  bool   `json:"sure"`
	Why   string `json:"why,omitempty"`
}

// Denial is what the insurer said no to, and why, in plain words.
type Denial struct {
	Category string `json:"category"`
	Label    string `json:"label"`
	Plain    string `json:"plain"`
	Level    string `json:"level"` // first | second | final
	Urgent   bool   `json:"urgent"`
	Service  string `json:"service_kind,omitempty"` // preservice | postservice | drug
	Short    string `json:"short,omitempty"`        // what was denied, in a few words
}

// Option is one answer to a choice question.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Question is something the person has to tell us.
type Question struct {
	ID      string   `json:"id"`
	Text    string   `json:"text"`
	Why     string   `json:"why,omitempty"`
	Kind    string   `json:"kind"` // choice | date | text
	Options []Option `json:"options,omitempty"`
	// Optional questions never hold a case back from being ready.
	Optional bool `json:"optional,omitempty"`
}

// Source is where a rule comes from.
type Source struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Deadline is a date somebody has to meet.
type Deadline struct {
	ID         string  `json:"id"`
	Label      string  `json:"label"`
	Date       string  `json:"date"`
	DaysLeft   int     `json:"days_left"`
	Basis      string  `json:"basis"`
	Kind       string  `json:"kind"` // appeal | decision | external | other
	Source     *Source `json:"source,omitempty"`
	FromLetter bool    `json:"from_letter"`
	Hours      int     `json:"hours,omitempty"` // set for clocks counted in hours
}

// Reason backs up the odds.
type Reason struct {
	Text  string `json:"text"`
	Quote string `json:"quote,omitempty"`
	Doc   string `json:"doc,omitempty"`
}

// Odds is an honest read of the chances.
type Odds struct {
	Level        string   `json:"level"` // strong | possible | long_shot | unclear
	Headline     string   `json:"headline"`
	Reasons      []Reason `json:"reasons"`
	Alternatives []string `json:"alternatives"`
}

// Step is one thing to do.
type Step struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Done   bool   `json:"done"`
	Action string `json:"action"`
}

// Cite is the evidence for a sentence.
type Cite struct {
	Doc   string `json:"doc"` // a file id, "rule:<id>", or "answer:<question id>"
	Title string `json:"title"`
	Quote string `json:"quote"`
	URL   string `json:"url,omitempty"`
	Found bool   `json:"found"`
}

// Sentence statuses.
const (
	SentVerified    = "verified"    // every quote was found
	SentPlain       = "plain"       // asks for or says nothing that needs checking
	SentUnsupported = "unsupported" // a claim we could not find in the documents
	SentConfirmed   = "confirmed"   // unsupported, but the person says it is true
	SentRemoved     = "removed"
)

// Sentence is one sentence of a letter.
type Sentence struct {
	ID     string `json:"id"`
	Para   int    `json:"para"`
	Text   string `json:"text"`
	Kind   string `json:"kind"` // fact | argument | request | courtesy
	Status string `json:"status"`
	Cites  []Cite `json:"cites"`
}

// Checked counts the factual sentences and how many were backed up.
type Checked struct {
	Facts    int `json:"facts"`
	Verified int `json:"verified"`
	Flagged  int `json:"flagged"`
}

// Letter is a drafted appeal.
type Letter struct {
	Status     string     `json:"status"` // none | drafting | drafted | error
	Error      string     `json:"error,omitempty"`
	Generated  time.Time  `json:"generated,omitempty"`
	To         string     `json:"to"`
	Subject    string     `json:"subject"`
	Sentences  []Sentence `json:"sentences"`
	Enclosures []string   `json:"enclosures"`
	Checked    Checked    `json:"checked"`
}

// Doctor is the doctor's-office link and what came through it.
type Doctor struct {
	Link     string    `json:"link"`
	Invited  time.Time `json:"invited,omitempty"`
	Uploaded bool      `json:"uploaded"`
	LMN      *Letter   `json:"lmn,omitempty"`
}

// Event is one line of the case's history.
type Event struct {
	At   time.Time `json:"at"`
	Text string    `json:"text"`
}

// Outcome is how it ended.
type Outcome struct {
	Result string `json:"result"` // "" | won | partial | lost
	Date   string `json:"date"`
	Note   string `json:"note"`
}

// Case is one denial being fought.
type Case struct {
	ID           string            `json:"id"`
	Created      time.Time         `json:"created"`
	Updated      time.Time         `json:"updated"`
	Title        string            `json:"title"`
	Status       string            `json:"status"`
	Error        string            `json:"error,omitempty"`
	Docs         []Doc             `json:"docs"`
	Facts        Facts             `json:"facts"`
	Plan         Plan              `json:"plan"`
	Denial       Denial            `json:"denial"`
	Questions    []Question        `json:"questions"`
	Answers      map[string]string `json:"answers,omitempty"`
	Deadlines    []Deadline        `json:"deadlines"`
	NextDeadline *Deadline         `json:"next_deadline"`
	Odds         Odds              `json:"odds"`
	Steps        []Step            `json:"steps"`
	StepsDone    map[string]bool   `json:"steps_done,omitempty"`
	Letter       Letter            `json:"letter"`
	Doctor       Doctor            `json:"doctor"`
	Timeline     []Event           `json:"timeline"`
	Outcome      Outcome           `json:"outcome"`
	// Sent is the day the appeal went out; Decided the day the answer came.
	Sent    string `json:"sent_date,omitempty"`
	Decided string `json:"decided_date,omitempty"`
	// Reminded records which reminders went out, by deadline id and offset.
	Reminded map[string]bool `json:"reminded,omitempty"`
	// DoctorCap is kept apart from Doctor so the JSON for the person never
	// carries the raw capability twice.
	DoctorCap string `json:"doctor_cap,omitempty"`
}

// Summary is a case as a list shows it.
type Summary struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Status       string    `json:"status"`
	NextDeadline *Deadline `json:"next_deadline"`
	Outcome      string    `json:"outcome"`
	Updated      time.Time `json:"updated"`
}

// Summary is the case as a list shows it.
func (c *Case) Summary() Summary {
	return Summary{ID: c.ID, Title: c.Title, Status: c.Status, NextDeadline: c.NextDeadline,
		Outcome: c.Outcome.Result, Updated: c.Updated}
}

// Log adds a line to the case's history.
func (c *Case) Log(now time.Time, text string) {
	c.Timeline = append(c.Timeline, Event{At: now.UTC(), Text: text})
}

// DocByID finds a document on the case.
func (c *Case) DocByID(id string) *Doc {
	for i := range c.Docs {
		if c.Docs[i].ID == id {
			return &c.Docs[i]
		}
	}
	return nil
}

// HasDoc reports whether a file is on the case already.
func (c *Case) HasDoc(id string) bool { return c.DocByID(id) != nil }

// NewID makes a case id.
func NewID() string {
	b := make([]byte, 9)
	rand.Read(b)
	return "c_" + hex.EncodeToString(b)
}

// NewCap makes an unguessable capability for a link.
func NewCap() string {
	b := make([]byte, 18)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ValidID reports whether id could be a case id, so it is safe in a path.
func ValidID(id string) bool {
	if len(id) != 20 || !strings.HasPrefix(id, "c_") {
		return false
	}
	for _, r := range id[2:] {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

// Store keeps one account's cases on disk: a directory per case holding
// case.json and the text read from each document.
type Store struct {
	Dir string // the account's directory
	mu  sync.Mutex
}

var stores sync.Map // account dir -> *Store

// For returns the one Store for an account directory, so every request on
// that account shares one lock.
func For(accountDir string) *Store {
	v, _ := stores.LoadOrStore(accountDir, &Store{Dir: accountDir})
	return v.(*Store)
}

func (s *Store) root() string           { return filepath.Join(s.Dir, "cases") }
func (s *Store) caseDir(id string) string { return filepath.Join(s.root(), id) }

// Has reports whether the account holds any case, which counts as using it.
func Has(accountDir string) bool {
	entries, err := os.ReadDir(filepath.Join(accountDir, "cases"))
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() && ValidID(e.Name()) {
			return true
		}
	}
	return false
}

// Get loads a case.
func (s *Store) Get(id string) (*Case, error) {
	if !ValidID(id) {
		return nil, os.ErrNotExist
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.get(id)
}

func (s *Store) get(id string) (*Case, error) {
	raw, err := os.ReadFile(filepath.Join(s.caseDir(id), "case.json"))
	if err != nil {
		return nil, err
	}
	var c Case
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	// Empty maps are left out of case.json; handlers write into them.
	if c.Answers == nil {
		c.Answers = map[string]string{}
	}
	if c.StepsDone == nil {
		c.StepsDone = map[string]bool{}
	}
	if c.Reminded == nil {
		c.Reminded = map[string]bool{}
	}
	return &c, nil
}

// Put saves a case.
func (s *Store) Put(c *Case) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.put(c)
}

func (s *Store) put(c *Case) error {
	dir := s.caseDir(c.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(c, "", " ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, "case.json.tmp")
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "case.json"))
}

// Update loads, changes and saves a case under the account's lock.
func (s *Store) Update(id string, fn func(c *Case) error) (*Case, error) {
	if !ValidID(id) {
		return nil, os.ErrNotExist
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.get(id)
	if err != nil {
		return nil, err
	}
	if err := fn(c); err != nil {
		return nil, err
	}
	return c, s.put(c)
}

// List returns every case, newest first.
func (s *Store) List() []*Case {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, _ := os.ReadDir(s.root())
	var out []*Case
	for _, e := range entries {
		if !e.IsDir() || !ValidID(e.Name()) {
			continue
		}
		if c, err := s.get(e.Name()); err == nil {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	return out
}

// Delete removes a case and what was read from its documents.
func (s *Store) Delete(id string) error {
	if !ValidID(id) {
		return os.ErrNotExist
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return os.RemoveAll(s.caseDir(id))
}

// PutText keeps the text read from one document.
func (s *Store) PutText(caseID, fileID, text string) error {
	dir := filepath.Join(s.caseDir(caseID), "text")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, safeName(fileID)+".txt"), []byte(text), 0o600)
}

// Text is what was read from one document, or "".
func (s *Store) Text(caseID, fileID string) string {
	raw, err := os.ReadFile(filepath.Join(s.caseDir(caseID), "text", safeName(fileID)+".txt"))
	if err != nil {
		return ""
	}
	return string(raw)
}

func safeName(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, s)
}

// FindByCap finds the case a doctor's-office capability names.
func (s *Store) FindByCap(capability string) (*Case, error) {
	if len(capability) < 20 {
		return nil, os.ErrNotExist
	}
	for _, c := range s.List() {
		if c.DoctorCap != "" && subtleEq(c.DoctorCap, capability) {
			return c, nil
		}
	}
	return nil, os.ErrNotExist
}

func subtleEq(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

// Title builds a short name for a case from what was read.
func Title(c *Case) string {
	var parts []string
	if c.Denial.Short != "" {
		parts = append(parts, shorten(c.Denial.Short, 32))
	} else if c.Facts.Service != nil && c.Facts.Service.Value != "" {
		parts = append(parts, shorten(c.Facts.Service.Value, 40))
	}
	if c.Facts.Insurer != nil && c.Facts.Insurer.Value != "" {
		parts = append(parts, shorten(c.Facts.Insurer.Value, 30))
	}
	if len(parts) == 0 {
		return "Your denial"
	}
	return strings.Join(parts, " · ")
}

func shorten(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

// Errorf is an error worded for a person.
func Errorf(format string, a ...any) error { return fmt.Errorf(format, a...) }
