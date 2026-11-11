package api

// Insurance appeals on the hosted node.
//
// Each account keeps its cases in its own directory (see internal/cases), so
// deleting an account deletes its cases, and nothing about one person's
// denial is ever in another person's files. Reading and drafting run in the
// background on the host's own model key, restricted to providers that keep
// nothing, under a daily ceiling of their own.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/agent"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/cases"
)

// DefaultCaseModel reads letters and drafts appeals.
const DefaultCaseModel = "anthropic/claude-sonnet-5.5"

// caseJobs remembers which cases have work running, so nothing runs twice.
var caseJobs sync.Map

func (h *Host) caseModelName() string {
	if h.CaseModel != "" {
		return h.CaseModel
	}
	return DefaultCaseModel
}

func (h *Host) caseKey() string {
	if h.CaseKey != "" {
		return h.CaseKey
	}
	return h.SharedKey
}

// engine is the case engine for one account.
func (h *Host) engine(acct *Account) *cases.Engine {
	return &cases.Engine{
		NewModel: func(maxTokens int) agent.Model {
			m := agent.NewOpenRouter(h.caseKey(), h.caseModelName())
			if m == nil {
				return nil
			}
			m.ZDR = true
			m.MaxTokens = maxTokens
			return m
		},
		Open: func(id string) ([]byte, string, string, error) { return readAccountFile(acct.Dir, id) },
		Spent: func(what string, u agent.Usage) {
			h.CasePool.Add(h.now(), u.Prompt+u.Completion)
			h.logf("cases: %s %s: %d in / %d out", acct.ID, what, u.Prompt, u.Completion)
		},
		Now: h.now,
	}
}

// readAccountFile returns an uploaded file's bytes, type and name.
func readAccountFile(dir, id string) ([]byte, string, string, error) {
	if !isFileID(id) {
		return nil, "", "", os.ErrNotExist
	}
	fd := filesDir(dir)
	raw, err := os.ReadFile(filepath.Join(fd, id+".json"))
	if err != nil {
		return nil, "", "", err
	}
	var f FileRef
	json.Unmarshal(raw, &f)
	ext, ok := allowedTypes[f.Type]
	if !ok {
		return nil, "", "", os.ErrNotExist
	}
	data, err := os.ReadFile(filepath.Join(fd, id+ext))
	return data, f.Type, f.Name, err
}

// saveAccountFile stores bytes as an uploaded file, the way /upload does.
func saveAccountFile(dir, name string, data []byte) (FileRef, error) {
	name = cleanName(name)
	if len(data) == 0 {
		return FileRef{}, fmt.Errorf("That file is empty.")
	}
	if len(data) > maxFileBytes {
		return FileRef{}, fmt.Errorf("Files can be up to 20 MB.")
	}
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	typ := fileType(name, head)
	ext, ok := allowedTypes[typ]
	if !ok || strings.HasPrefix(typ, "application/json") {
		return FileRef{}, fmt.Errorf("Lamdis takes PDFs and photos (JPEG, PNG, WebP).")
	}
	sum := sha256.Sum256(data)
	id := hex.EncodeToString(sum[:16])
	fd := filesDir(dir)
	os.MkdirAll(fd, 0o700)
	if err := os.WriteFile(filepath.Join(fd, id+ext), data, 0o600); err != nil {
		return FileRef{}, fmt.Errorf("could not keep the file")
	}
	ref := FileRef{ID: id, Name: name, Type: typ, Size: int64(len(data))}
	meta, _ := json.Marshal(ref)
	os.WriteFile(filepath.Join(fd, id+".json"), meta, 0o600)
	return ref, nil
}

// caseUsage is one account's case work today.
type caseUsage struct {
	Day     string `json:"day"`
	Reads   int    `json:"reads"`
	Letters int    `json:"letters"`
}

var caseUsageMu sync.Mutex

// allowCase spends one of today's reads or letters for an account, or says
// why not.
func (h *Host) allowCase(dir, what string) error {
	if h.caseKey() == "" {
		return fmt.Errorf("Reading letters is switched off on this server right now.")
	}
	if h.CasePool.Full(h.now()) {
		return fmt.Errorf("Lamdis is busier than usual today. Your case is saved; try again in a few hours.")
	}
	caseUsageMu.Lock()
	defer caseUsageMu.Unlock()
	p := filepath.Join(dir, "case_usage.json")
	var u caseUsage
	if raw, err := os.ReadFile(p); err == nil {
		json.Unmarshal(raw, &u)
	}
	today := h.now().UTC().Format("2006-01-02")
	if u.Day != today {
		u = caseUsage{Day: today}
	}
	reads, letters := envIntOr("LAMDIS_CASE_READS", 12), envIntOr("LAMDIS_CASE_LETTERS", 10)
	switch what {
	case "read":
		if u.Reads >= reads {
			return fmt.Errorf("That's as many pages as we can read for one person today. Your case is saved; add more tomorrow.")
		}
		u.Reads++
	case "letter":
		if u.Letters >= letters {
			return fmt.Errorf("That's as many drafts as we can write for one person today. Your case is saved; try again tomorrow.")
		}
		u.Letters++
	}
	raw, _ := json.Marshal(u)
	os.WriteFile(p, raw, 0o600)
	return nil
}

func envIntOr(k string, d int) int {
	var v int
	if _, err := fmt.Sscanf(strings.TrimSpace(os.Getenv(k)), "%d", &v); err == nil && v > 0 {
		return v
	}
	return d
}

// registerCases mounts the case routes on one account's mux.
func (h *Host) registerCases(acct *Account, mux *http.ServeMux) {
	st := cases.For(acct.Dir)
	fileURL := func(id string) string { return h.fileURL(acct.ID, id) }
	out := func(w http.ResponseWriter, c *cases.Case) {
		h.present(acct, c)
		writeJSON(w, c)
	}
	fail := func(w http.ResponseWriter, code int, msg string) {
		writeStatusJSON(w, code, map[string]any{"error": msg})
	}
	decode := func(r *http.Request, v any) error {
		return json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(v)
	}
	get := func(w http.ResponseWriter, r *http.Request) (*cases.Case, bool) {
		c, err := st.Get(r.PathValue("id"))
		if err != nil {
			fail(w, http.StatusNotFound, "That case isn't here. It may have been deleted.")
			return nil, false
		}
		return c, true
	}
	addDocs := func(c *cases.Case, ids []string, kind, by string) int {
		n := 0
		for _, id := range ids {
			if !isFileID(id) || c.HasDoc(id) {
				continue
			}
			_, typ, name, err := readAccountFile(acct.Dir, id)
			if err != nil {
				continue
			}
			c.Docs = append(c.Docs, cases.Doc{ID: id, Name: name, Type: typ, Kind: kind, AddedBy: by,
				Added: h.now().UTC(), URL: fileURL(id)})
			n++
		}
		return n
	}

	mux.HandleFunc("GET /app/api/cases", func(w http.ResponseWriter, r *http.Request) {
		var list []cases.Summary
		for _, c := range st.List() {
			h.present(acct, c)
			list = append(list, c.Summary())
		}
		if list == nil {
			list = []cases.Summary{}
		}
		writeJSON(w, map[string]any{"cases": list})
	})
	mux.HandleFunc("POST /app/api/cases", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Files []string `json:"files"`
		}
		decode(r, &in)
		if len(in.Files) == 0 {
			fail(w, http.StatusBadRequest, "Add at least one page of the denial letter.")
			return
		}
		if len(in.Files) > 20 {
			fail(w, http.StatusBadRequest, "That's more than 20 pages. Start with the denial letter itself.")
			return
		}
		if err := h.allowCase(acct.Dir, "read"); err != nil {
			fail(w, http.StatusTooManyRequests, err.Error())
			return
		}
		now := h.now().UTC()
		c := &cases.Case{ID: cases.NewID(), Created: now, Updated: now, Status: cases.StatusReading,
			Answers: map[string]string{}, StepsDone: map[string]bool{}, Letter: cases.Letter{Status: "none"},
			Plan: cases.Plan{Type: "unknown"}, Timeline: []cases.Event{}}
		if addDocs(c, in.Files, cases.KindDenial, "you") == 0 {
			fail(w, http.StatusBadRequest, "Those files didn't upload. Try adding them again.")
			return
		}
		c.Log(now, "You added your denial letter.")
		cases.Refresh(c, now)
		c.Status = cases.StatusReading
		if err := st.Put(c); err != nil {
			fail(w, http.StatusInternalServerError, "Could not save the case.")
			return
		}
		h.logf("cases: %s opened %s (%d pages)", acct.ID, c.ID, len(c.Docs))
		h.readCase(acct, c.ID)
		out(w, c)
	})
	mux.HandleFunc("GET /app/api/cases/{id}", func(w http.ResponseWriter, r *http.Request) {
		if c, ok := get(w, r); ok {
			out(w, c)
		}
	})
	mux.HandleFunc("POST /app/api/cases/{id}/docs", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Files []string `json:"files"`
			Kind  string   `json:"kind"`
		}
		decode(r, &in)
		if !cases.ValidKind(in.Kind) {
			in.Kind = cases.KindOther
		}
		if err := h.allowCase(acct.Dir, "read"); err != nil {
			fail(w, http.StatusTooManyRequests, err.Error())
			return
		}
		var added int
		c, err := st.Update(r.PathValue("id"), func(c *cases.Case) error {
			added = addDocs(c, in.Files, in.Kind, "you")
			if added == 0 {
				return fmt.Errorf("those files didn't upload; try adding them again")
			}
			c.Log(h.now(), fmt.Sprintf("You added %s.", strings.ToLower(cases.KindLabel(in.Kind))))
			c.Status = cases.StatusReading
			c.Error = ""
			c.Updated = h.now().UTC()
			return nil
		})
		if err != nil {
			fail(w, http.StatusBadRequest, capitalize(err.Error()))
			return
		}
		h.readCase(acct, c.ID)
		out(w, c)
	})
	mux.HandleFunc("POST /app/api/cases/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		c, ok := get(w, r)
		if !ok {
			return
		}
		if c.Status != cases.StatusError && c.Letter.Status != "error" {
			out(w, c)
			return
		}
		if c.Letter.Status == "error" {
			h.draftLetter(acct, c.ID)
		} else {
			st.Update(c.ID, func(c *cases.Case) error { c.Status, c.Error = cases.StatusReading, ""; return nil })
			h.readCase(acct, c.ID)
		}
		c, _ = st.Get(c.ID)
		out(w, c)
	})
	mux.HandleFunc("POST /app/api/cases/{id}/answers", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Answers map[string]string `json:"answers"`
		}
		decode(r, &in)
		c, err := st.Update(r.PathValue("id"), func(c *cases.Case) error {
			for k, v := range in.Answers {
				if err := cases.ApplyAnswer(c, k, v); err != nil {
					return err
				}
			}
			c.Updated = h.now().UTC()
			cases.Refresh(c, h.now())
			return nil
		})
		if err != nil {
			fail(w, http.StatusBadRequest, capitalize(err.Error()))
			return
		}
		out(w, c)
	})
	mux.HandleFunc("POST /app/api/cases/{id}/letter", func(w http.ResponseWriter, r *http.Request) {
		c, ok := get(w, r)
		if !ok {
			return
		}
		if c.Status == cases.StatusReading {
			fail(w, http.StatusConflict, "We're still reading your pages. Try again in a moment.")
			return
		}
		if err := h.allowCase(acct.Dir, "letter"); err != nil {
			fail(w, http.StatusTooManyRequests, err.Error())
			return
		}
		h.draftLetter(acct, c.ID)
		c, _ = st.Get(c.ID)
		out(w, c)
	})
	mux.HandleFunc("POST /app/api/cases/{id}/letter/sentence", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Sentence string `json:"sentence"`
			Keep     bool   `json:"keep"`
		}
		decode(r, &in)
		c, err := st.Update(r.PathValue("id"), func(c *cases.Case) error {
			for i := range c.Letter.Sentences {
				s := &c.Letter.Sentences[i]
				if s.ID != in.Sentence {
					continue
				}
				switch {
				case !in.Keep:
					s.Status = cases.SentRemoved
				case s.Status == cases.SentUnsupported || s.Status == cases.SentRemoved:
					if s.Status == cases.SentRemoved && allFound(s.Cites) && len(s.Cites) > 0 {
						s.Status = cases.SentVerified
					} else if s.Status == cases.SentRemoved && len(s.Cites) == 0 && s.Kind != "fact" && s.Kind != "argument" {
						s.Status = cases.SentPlain
					} else {
						s.Status = cases.SentConfirmed
					}
				}
				c.Letter.Checked = cases.Count(c.Letter)
				c.Updated = h.now().UTC()
				return nil
			}
			return fmt.Errorf("that sentence isn't in the letter")
		})
		if err != nil {
			fail(w, http.StatusBadRequest, capitalize(err.Error()))
			return
		}
		out(w, c)
	})
	mux.HandleFunc("GET /app/api/cases/{id}/letter.html", func(w http.ResponseWriter, r *http.Request) {
		c, ok := get(w, r)
		if !ok {
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		io.WriteString(w, letterHTML(c, c.Letter, false))
	})
	mux.HandleFunc("POST /app/api/cases/{id}/invite", func(w http.ResponseWriter, r *http.Request) {
		var draft bool
		c, err := st.Update(r.PathValue("id"), func(c *cases.Case) error {
			if c.DoctorCap == "" {
				c.DoctorCap = cases.NewCap()
				c.Doctor.Invited = h.now().UTC()
				c.Log(h.now(), "You made a link for your doctor's office.")
			}
			draft = c.Doctor.LMN == nil
			c.Updated = h.now().UTC()
			return nil
		})
		if err != nil {
			fail(w, http.StatusNotFound, "That case isn't here.")
			return
		}
		if draft {
			h.draftLMN(acct, c.ID)
		}
		h.present(acct, c)
		writeJSON(w, map[string]any{"url": c.Doctor.Link})
	})
	mux.HandleFunc("POST /app/api/cases/{id}/event", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Kind    string `json:"kind"`
			Date    string `json:"date"`
			Outcome string `json:"outcome"`
			Note    string `json:"note"`
		}
		decode(r, &in)
		if in.Date == "" {
			in.Date = h.now().Format("2006-01-02")
		}
		if _, err := time.Parse("2006-01-02", in.Date); err != nil {
			fail(w, http.StatusBadRequest, "That date didn't read as a date.")
			return
		}
		if len(in.Note) > 2000 {
			in.Note = in.Note[:2000]
		}
		c, err := st.Update(r.PathValue("id"), func(c *cases.Case) error {
			switch in.Kind {
			case "sent":
				c.Sent = in.Date
				c.StepsDone["send"] = true
				c.Log(h.now(), "You sent your appeal on "+cases.HumanDate(in.Date)+".")
			case "decision":
				switch in.Outcome {
				case "won", "partial", "lost":
				default:
					return fmt.Errorf("say whether you won, partly won, or lost")
				}
				c.Outcome = cases.Outcome{Result: in.Outcome, Date: in.Date, Note: strings.TrimSpace(in.Note)}
				c.Decided = in.Date
				word := map[string]string{"won": "won", "partial": "partly won", "lost": "were turned down"}[in.Outcome]
				c.Log(h.now(), "You "+word+" on "+cases.HumanDate(in.Date)+".")
				h.logf("cases: %s %s outcome %s", acct.ID, c.ID, in.Outcome)
			case "reopen":
				c.Outcome = cases.Outcome{}
				c.Decided = ""
			case "note":
				if strings.TrimSpace(in.Note) == "" {
					return fmt.Errorf("write a note first")
				}
				c.Log(h.now(), strings.TrimSpace(in.Note))
			default:
				return fmt.Errorf("unknown event")
			}
			c.Updated = h.now().UTC()
			cases.Refresh(c, h.now())
			return nil
		})
		if err != nil {
			fail(w, http.StatusBadRequest, capitalize(err.Error()))
			return
		}
		out(w, c)
	})
	mux.HandleFunc("POST /app/api/cases/{id}/step", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Step string `json:"step"`
			Done bool   `json:"done"`
		}
		decode(r, &in)
		c, err := st.Update(r.PathValue("id"), func(c *cases.Case) error {
			if c.StepsDone == nil {
				c.StepsDone = map[string]bool{}
			}
			c.StepsDone[in.Step] = in.Done
			c.Updated = h.now().UTC()
			cases.Refresh(c, h.now())
			return nil
		})
		if err != nil {
			fail(w, http.StatusNotFound, "That case isn't here.")
			return
		}
		out(w, c)
	})
	mux.HandleFunc("POST /app/api/cases/{id}/delete", func(w http.ResponseWriter, r *http.Request) {
		c, ok := get(w, r)
		if !ok {
			return
		}
		// Files that belong only to this case go with it.
		keep := map[string]bool{}
		for _, other := range st.List() {
			if other.ID == c.ID {
				continue
			}
			for _, d := range other.Docs {
				keep[d.ID] = true
			}
		}
		for _, d := range c.Docs {
			if !keep[d.ID] {
				removeAccountFile(acct.Dir, d.ID)
			}
		}
		st.Delete(c.ID)
		h.logf("cases: %s deleted %s", acct.ID, c.ID)
		writeJSON(w, map[string]any{"ok": true})
	})
}

func allFound(cs []cases.Cite) bool {
	for _, c := range cs {
		if !c.Found {
			return false
		}
	}
	return true
}

func removeAccountFile(dir, id string) {
	if !isFileID(id) {
		return
	}
	fd := filesDir(dir)
	raw, err := os.ReadFile(filepath.Join(fd, id+".json"))
	if err == nil {
		var f FileRef
		json.Unmarshal(raw, &f)
		if ext, ok := allowedTypes[f.Type]; ok {
			os.Remove(filepath.Join(fd, id+ext))
		}
	}
	os.Remove(filepath.Join(fd, id+".json"))
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	s = strings.ToUpper(s[:1]) + s[1:]
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}

// present fills in what depends on today and on this host: days left, the
// doctor's link, a case left mid-read by a restart.
func (h *Host) present(acct *Account, c *cases.Case) {
	now := h.now()
	if (c.Status == cases.StatusReading || c.Letter.Status == "drafting") && now.Sub(c.Updated) > 6*time.Minute {
		if _, running := caseJobs.Load(acct.ID + "/" + c.ID); !running {
			if c.Status == cases.StatusReading {
				c.Status, c.Error = cases.StatusError, "Reading was interrupted. Try again."
			}
			if c.Letter.Status == "drafting" {
				c.Letter.Status, c.Letter.Error = "error", "Drafting was interrupted. Try again."
			}
		}
	}
	for i := range c.Deadlines {
		if t, err := time.Parse("2006-01-02", c.Deadlines[i].Date); err == nil {
			c.Deadlines[i].DaysLeft = daysUntil(now, t)
		}
	}
	c.NextDeadline = cases.Next(c.Deadlines)
	if c.DoctorCap != "" {
		c.Doctor.Link = strings.TrimRight(h.PublicBase, "/") + "/d/" + acct.ID + "/" + c.DoctorCap
	}
	for i := range c.Docs {
		c.Docs[i].URL = h.fileURL(acct.ID, c.Docs[i].ID)
	}
	if c.Questions == nil {
		c.Questions = []cases.Question{}
	}
	if c.Deadlines == nil {
		c.Deadlines = []cases.Deadline{}
	}
	if c.Steps == nil {
		c.Steps = []cases.Step{}
	}
	if c.Timeline == nil {
		c.Timeline = []cases.Event{}
	}
	if c.Letter.Sentences == nil {
		c.Letter.Sentences = []cases.Sentence{}
	}
	if c.Letter.Enclosures == nil {
		c.Letter.Enclosures = []string{}
	}
	if c.Odds.Reasons == nil {
		c.Odds.Reasons = []cases.Reason{}
	}
	if c.Odds.Alternatives == nil {
		c.Odds.Alternatives = []string{}
	}
}

func daysUntil(now, t time.Time) int {
	a := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	b := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return int(b.Sub(a).Hours() / 24)
}

// job runs fn for a case in the background unless something already is.
func (h *Host) job(acct *Account, caseID string, fn func(ctx context.Context)) bool {
	key := acct.ID + "/" + caseID
	if _, busy := caseJobs.LoadOrStore(key, true); busy {
		return false
	}
	go func() {
		defer caseJobs.Delete(key)
		base := h.ctx
		if base == nil {
			base = context.Background()
		}
		ctx, cancel := context.WithTimeout(base, 5*time.Minute)
		defer cancel()
		defer func() {
			if r := recover(); r != nil {
				h.logf("cases: %s %s panicked: %v", acct.ID, caseID, r)
			}
		}()
		fn(ctx)
	}()
	return true
}

// readCase reads whatever on the case is unread, then works out the rest.
func (h *Host) readCase(acct *Account, caseID string) {
	st := cases.For(acct.Dir)
	e := h.engine(acct)
	h.job(acct, caseID, func(ctx context.Context) {
		c, err := st.Get(caseID)
		if err != nil {
			return
		}
		texts := map[string]string{}
		var mu sync.Mutex
		text := func(id string) string {
			mu.Lock()
			t, ok := texts[id]
			mu.Unlock()
			if ok {
				return t
			}
			return st.Text(caseID, id)
		}
		err = e.ReadAll(ctx, c, text, func(id, t string) {
			mu.Lock()
			texts[id] = t
			mu.Unlock()
			st.PutText(caseID, id, t)
		})
		_, uerr := st.Update(caseID, func(saved *cases.Case) error {
			// Keep anything the person did meanwhile; take what was read.
			saved.Facts, saved.Plan, saved.Denial, saved.Odds = c.Facts, c.Plan, c.Denial, c.Odds
			if saved.Answers["plan_source"] != "" {
				cases.ApplyAnswer(saved, "plan_source", saved.Answers["plan_source"])
			}
			for i := range saved.Docs {
				if d := c.DocByID(saved.Docs[i].ID); d != nil && d.Read {
					saved.Docs[i].Read = true
				}
			}
			saved.Updated = h.now().UTC()
			if err != nil && !anyRead(saved) {
				saved.Status, saved.Error = cases.StatusError, readError(err)
				return nil
			}
			if err != nil {
				saved.Log(h.now(), "Some pages could not be read: "+readError(err))
			} else {
				saved.Log(h.now(), "Lamdis read your pages.")
			}
			saved.Status, saved.Error = cases.StatusReady, ""
			cases.Refresh(saved, h.now())
			return nil
		})
		if uerr != nil {
			h.logf("cases: %s %s save after read: %v", acct.ID, caseID, uerr)
		}
		if err != nil {
			h.logf("cases: %s %s read: %v", acct.ID, caseID, err)
		}
	})
}

func anyRead(c *cases.Case) bool {
	for _, d := range c.Docs {
		if d.Read {
			return true
		}
	}
	return false
}

func readError(err error) string {
	var oc agent.OutOfCredit
	if asOutOfCredit(err, &oc) {
		return "Lamdis can't read letters right now. Your case is saved; try again later."
	}
	msg := err.Error()
	if strings.Contains(msg, "deadline exceeded") || strings.Contains(msg, "canceled") {
		return "Reading took too long. Try again."
	}
	return "We couldn't read that. Try again, or add clearer photos."
}

func asOutOfCredit(err error, oc *agent.OutOfCredit) bool {
	_, ok := err.(agent.OutOfCredit)
	return ok
}

// draftLetter writes the appeal in the background.
func (h *Host) draftLetter(acct *Account, caseID string) {
	st := cases.For(acct.Dir)
	e := h.engine(acct)
	if _, err := st.Update(caseID, func(c *cases.Case) error {
		c.Letter.Status, c.Letter.Error = "drafting", ""
		c.Updated = h.now().UTC()
		return nil
	}); err != nil {
		return
	}
	started := h.job(acct, caseID, func(ctx context.Context) {
		c, err := st.Get(caseID)
		if err != nil {
			return
		}
		l, err := e.Draft(ctx, c, func(id string) string { return st.Text(caseID, id) })
		st.Update(caseID, func(saved *cases.Case) error {
			saved.Updated = h.now().UTC()
			if err != nil {
				h.logf("cases: %s %s letter: %v", acct.ID, caseID, err)
				saved.Letter.Status, saved.Letter.Error = "error", "We couldn't draft the letter just now. Try again."
				return nil
			}
			saved.Letter = l
			saved.Log(h.now(), fmt.Sprintf("Lamdis drafted your appeal: %d of %d facts checked against your documents.", l.Checked.Verified, l.Checked.Facts))
			cases.Refresh(saved, h.now())
			return nil
		})
	})
	if !started {
		// Something else is running on this case (most likely a read);
		// put the letter back so the person can ask again in a moment.
		st.Update(caseID, func(c *cases.Case) error {
			c.Letter.Status, c.Letter.Error = "error", "We were still reading your pages. Try again in a moment."
			return nil
		})
	}
}

// draftLMN writes the doctor's draft letter in the background.
func (h *Host) draftLMN(acct *Account, caseID string) {
	st := cases.For(acct.Dir)
	e := h.engine(acct)
	if h.caseKey() == "" || h.CasePool.Full(h.now()) {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()
		c, err := st.Get(caseID)
		if err != nil {
			return
		}
		l, err := e.DraftLMN(ctx, c, func(id string) string { return st.Text(caseID, id) })
		if err != nil {
			h.logf("cases: %s %s lmn: %v", acct.ID, caseID, err)
			return
		}
		st.Update(caseID, func(saved *cases.Case) error { saved.Doctor.LMN = &l; return nil })
	}()
}

// existingAccount loads an account only if it already exists.
func (h *Host) existingAccount(id string) (*Account, bool) {
	if id == "" || strings.ContainsAny(id, "./\\") {
		return nil, false
	}
	if _, err := os.Stat(filepath.Join(h.Root, ".deleted", id)); err == nil {
		return nil, false
	}
	if _, err := os.Stat(filepath.Join(h.Root, id)); err != nil {
		return nil, false
	}
	a, err := h.load(id, "")
	return a, err == nil
}

// doctorCase resolves a doctor's-office link.
func (h *Host) doctorCase(r *http.Request) (*Account, *cases.Case, bool) {
	acct, ok := h.existingAccount(r.PathValue("acct"))
	if !ok {
		return nil, nil, false
	}
	c, err := cases.For(acct.Dir).FindByCap(r.PathValue("cap"))
	if err != nil {
		return nil, nil, false
	}
	return acct, c, true
}

// registerDoctor mounts the public doctor's-office pages.
func (h *Host) registerDoctor(mux *http.ServeMux) {
	mux.HandleFunc("GET /d/{acct}/{cap}", func(w http.ResponseWriter, r *http.Request) {
		acct, c, ok := h.doctorCase(r)
		if !ok {
			http.Error(w, "This link has expired or was removed.", http.StatusNotFound)
			return
		}
		h.present(acct, c)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Robots-Tag", "noindex")
		io.WriteString(w, doctorHTML(c, r.URL.Path))
	})
	mux.HandleFunc("GET /d/{acct}/{cap}/lmn.html", func(w http.ResponseWriter, r *http.Request) {
		_, c, ok := h.doctorCase(r)
		if !ok || c.Doctor.LMN == nil {
			http.Error(w, "The draft isn't ready yet. Refresh in a minute.", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Robots-Tag", "noindex")
		io.WriteString(w, letterHTML(c, *c.Doctor.LMN, true))
	})
	mux.HandleFunc("POST /d/{acct}/{cap}/upload", func(w http.ResponseWriter, r *http.Request) {
		acct, c, ok := h.doctorCase(r)
		if !ok {
			writeStatusJSON(w, http.StatusNotFound, map[string]any{"error": "This link has expired or was removed."})
			return
		}
		if !allowMail(acct.ID+"/doctor", 30) {
			writeStatusJSON(w, http.StatusTooManyRequests, map[string]any{"error": "Too many uploads today."})
			return
		}
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxFileBytes+1))
		if err != nil {
			writeStatusJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "Files can be up to 20 MB."})
			return
		}
		ref, err := saveAccountFile(acct.Dir, r.URL.Query().Get("name"), data)
		if err != nil {
			writeStatusJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		st := cases.For(acct.Dir)
		st.Update(c.ID, func(c *cases.Case) error {
			if !c.HasDoc(ref.ID) {
				c.Docs = append(c.Docs, cases.Doc{ID: ref.ID, Name: ref.Name, Type: ref.Type, Kind: cases.KindDoctorLetter,
					AddedBy: "doctor", Added: h.now().UTC()})
				c.Log(h.now(), "Your doctor's office added "+ref.Name+".")
			}
			c.Doctor.Uploaded = true
			c.Updated = h.now().UTC()
			cases.Refresh(c, h.now())
			return nil
		})
		h.logf("cases: %s %s doctor upload", acct.ID, c.ID)
		if h.allowCase(acct.Dir, "read") == nil {
			h.readCase(acct, c.ID)
		}
		h.tellPerson(acct, "Your doctor's office added a letter to your appeal",
			"Your doctor's office just added "+ref.Name+" to your appeal on Lamdis.\n\nOpen your case to draft (or redraft) your appeal with it:\n"+strings.TrimRight(h.PublicBase, "/")+"/app#case="+c.ID+"\n\nLamdis")
		writeJSON(w, map[string]any{"ok": true})
	})
}

// tellPerson emails the account's confirmed address, if it has one.
func (h *Host) tellPerson(acct *Account, subject, body string) {
	if h.Mail == nil {
		return
	}
	raw, err := os.ReadFile(filepath.Join(acct.Dir, "email_confirmed"))
	to := strings.TrimSpace(string(raw))
	if err != nil || to == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := h.Mail.Send(ctx, to, subject, body); err != nil {
			h.logf("cases: mail to %s: %v", acct.ID, err)
		}
	}()
}

// Remind emails people before their deadlines. It runs hourly.
func (h *Host) Remind(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		h.remindOnce()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (h *Host) remindOnce() {
	if h.Mail == nil {
		return
	}
	// One bad case must not take the whole host down.
	defer func() {
		if r := recover(); r != nil {
			h.logf("cases: reminders: %v", r)
		}
	}()
	entries, err := os.ReadDir(h.Root)
	if err != nil {
		return
	}
	// Reminders go out in the morning, US Eastern, not at 3 a.m.
	loc, _ := time.LoadLocation("America/New_York")
	if loc == nil {
		loc = time.UTC
	}
	if hr := h.now().In(loc).Hour(); hr < 8 || hr > 20 {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dir := filepath.Join(h.Root, e.Name())
		if !cases.Has(dir) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, "email_confirmed"))
		to := strings.TrimSpace(string(raw))
		if err != nil || to == "" {
			continue
		}
		st := cases.For(dir)
		for _, c := range st.List() {
			if c.Outcome.Result != "" {
				continue
			}
			var send []string
			st.Update(c.ID, func(c *cases.Case) error {
				for _, d := range c.Deadlines {
					t, err := time.Parse("2006-01-02", d.Date)
					if err != nil {
						continue
					}
					left := daysUntil(h.now(), t)
					for _, at := range []int{14, 7, 2, 0} {
						key := fmt.Sprintf("%s:%s:%d", d.ID, d.Date, at)
						if left != at || c.Reminded[key] {
							continue
						}
						if d.Kind == "decision" && at != 0 {
							continue
						}
						if c.Reminded == nil {
							c.Reminded = map[string]bool{}
						}
						c.Reminded[key] = true
						send = append(send, reminderText(c, d, left))
					}
				}
				return nil
			})
			link := strings.TrimRight(h.PublicBase, "/") + "/app#case=" + c.ID
			for _, s := range send {
				subject := strings.SplitN(s, "\n", 2)[0]
				body := s + "\n\nOpen your case:\n" + link + "\n\nYou're getting this because you asked Lamdis to remind you. Reply to stop.\n\nLamdis"
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				if err := h.Mail.Send(ctx, to, subject, body); err != nil {
					h.logf("cases: reminder to %s: %v", e.Name(), err)
				} else {
					h.logf("cases: reminder sent for %s %s", e.Name(), c.ID)
				}
				cancel()
			}
		}
	}
}

func reminderText(c *cases.Case, d cases.Deadline, left int) string {
	when := fmt.Sprintf("in %d days", left)
	if left == 0 {
		when = "today"
	}
	if d.Kind == "decision" {
		return "Your plan's decision is due today: " + c.Title + "\nIf you haven't heard back, call the plan and ask for the decision. Write down who you spoke to."
	}
	return "Your appeal deadline is " + when + ": " + c.Title + "\n" + d.Label + " by " + cases.HumanDate(d.Date) + " (" + d.Basis + ")."
}

// letterHTML renders a letter for printing. Sentences that were not backed
// up and not confirmed are left out.
func letterHTML(c *cases.Case, l cases.Letter, doctor bool) string {
	esc := html.EscapeString
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex">`)
	title := "Appeal letter"
	if doctor {
		title = "Draft letter of medical necessity"
	}
	b.WriteString("<title>" + esc(title) + "</title>")
	b.WriteString(`<style>
body{font:16px/1.55 Georgia,"Times New Roman",serif;color:#111;background:#fff;max-width:720px;margin:0 auto;padding:40px 28px}
.note{font:14px/1.45 -apple-system,system-ui,sans-serif;background:#fff7e0;border:1px solid #f0d58a;border-radius:10px;padding:12px 14px;margin-bottom:28px}
.block{white-space:pre-line;margin:0 0 20px}
p{margin:0 0 14px}
.re{font-weight:bold}
.encl{margin-top:22px}
@media print{.note{display:none}body{padding:0}}
</style></head><body>`)
	left := 0
	for _, s := range l.Sentences {
		if s.Status == cases.SentUnsupported {
			left++
		}
	}
	if doctor {
		b.WriteString(`<div class="note">This is a draft prepared from the patient's documents for the physician to review, correct, complete and sign. Replace every [bracketed] placeholder. Print it on your letterhead, or copy the text into your own template.</div>`)
	} else if left > 0 {
		fmt.Fprintf(&b, `<div class="note">%d sentence(s) we couldn't find in your documents are left out of this copy. Confirm or remove them in your case to finish the letter.</div>`, left)
	}
	b.WriteString(`<div class="block">` + esc(time.Now().Format("January 2, 2006")) + `</div>`)
	if !doctor && c.Facts.Patient != nil && c.Facts.Patient.Verified {
		b.WriteString(`<div class="block">` + esc(c.Facts.Patient.Value) + `</div>`)
	}
	if l.To != "" {
		b.WriteString(`<div class="block">` + esc(l.To) + `</div>`)
	} else if c.Facts.Insurer != nil {
		b.WriteString(`<div class="block">` + esc(c.Facts.Insurer.Value) + "\nAppeals Department</div>")
	}
	if l.Subject != "" {
		b.WriteString(`<p class="re">Re: ` + esc(l.Subject) + `</p>`)
	}
	para := -1
	open := false
	for _, s := range l.Sentences {
		if s.Status == cases.SentUnsupported || s.Status == cases.SentRemoved {
			continue
		}
		if s.Para != para {
			if open {
				b.WriteString("</p>")
			}
			b.WriteString("<p>")
			open, para = true, s.Para
		} else {
			b.WriteString(" ")
		}
		b.WriteString(esc(s.Text))
	}
	if open {
		b.WriteString("</p>")
	}
	if len(l.Enclosures) > 0 {
		b.WriteString(`<div class="encl">Enclosures:<br>`)
		for _, e := range l.Enclosures {
			b.WriteString("&bull; " + esc(e) + "<br>")
		}
		b.WriteString("</div>")
	}
	b.WriteString("</body></html>")
	return b.String()
}
