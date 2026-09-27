package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
)

// The browser tool: what the agent may do in the person's browser, and the
// one thing it may never do alone.
//
// Reading, searching, navigating and filling in forms are the agent's to do.
// Pressing anything that sends, books, buys, pays, posts, deletes or agrees
// is the person's: the agent names the button and says exactly what it will
// do, the person answers, and only a yes presses it — in full auto as well.

// BrowserConfirmTool marks a decision whose yes presses a button.
const BrowserConfirmTool = "browser.confirm"

// BrowserYes is the answer that presses it.
const BrowserYes = "Yes, do it"

func (r *Runner) browse(ctx context.Context, t Trigger, g gate, canWrite bool, rec *runRec, args map[string]any) (string, bool, string) {
	if !g.web || !canWrite || r.Browser == nil {
		return "the browser is not available on this run", false, ""
	}
	str := func(k string) string { v, _ := args[k].(string); return strings.TrimSpace(v) }
	id := 0
	if f, ok := args["id"].(float64); ok {
		id = int(f)
	}
	action := str("action")

	// A browser step costs like a fetch, against the same daily budget.
	cfg, _ := LoadConfig(r.DataDir)
	over := false
	r.State.Update(r.now(), func(st *State) {
		if r.limited(cfg) && st.Fetches >= cfg.MaxFetchesPerDay {
			over = true
		} else {
			st.Fetches++
		}
	})
	if over {
		return "refused: today's web budget is used up", false, ""
	}

	s, err := r.Browser.Session(r.DataDir)
	if err != nil {
		return "error: " + err.Error(), false, ""
	}
	note := func(what string, err error) {
		fr := FetchRecord{URL: "browser: " + what}
		if err != nil {
			fr.Error = err.Error()
		}
		rec.Fetches = append(rec.Fetches, fr)
	}
	page := func() (string, bool, string) {
		text, err := s.Read()
		if err != nil {
			return "error reading the page: " + err.Error(), false, ""
		}
		u, _ := s.URL()
		return untrusted(u, text), false, ""
	}

	switch action {
	case "open":
		u := str("url")
		err := s.Open(u)
		note("open "+u, err)
		if err != nil {
			return "refused: " + err.Error(), false, ""
		}
		return page()
	case "read":
		return page()
	case "click":
		if id == 0 {
			return "id is required", false, ""
		}
		err := s.Click(id)
		note(fmt.Sprintf("click %d", id), err)
		if err != nil {
			return "could not click that: " + err.Error() + " (read the page again; numbers change when it does)", false, ""
		}
		return page()
	case "type":
		if id == 0 {
			return "id is required", false, ""
		}
		submit, _ := args["submit"].(bool)
		err := s.Type(id, str("text"), submit)
		note(fmt.Sprintf("type into %d", id), err)
		if err != nil {
			return "could not type there: " + err.Error(), false, ""
		}
		return page()
	case "choose":
		err := s.Choose(id, str("text"))
		note(fmt.Sprintf("choose in %d", id), err)
		if err != nil {
			return "could not choose that: " + err.Error(), false, ""
		}
		return page()
	case "scroll_down", "scroll_up":
		if err := s.Scroll(action == "scroll_down"); err != nil {
			return "error: " + err.Error(), false, ""
		}
		return page()
	case "back":
		err := s.Back()
		note("back", err)
		if err != nil {
			return "error: " + err.Error(), false, ""
		}
		return page()
	case "handoff":
		what := str("text")
		if what == "" {
			what = "Take over the browser"
		}
		u, title := s.URL()
		s.Save()
		s.Pin(3 * time.Hour)
		body := map[string]any{"text": what + ". Open the browser, do it there yourself, then press Done.",
			"options": []string{"Done"}, "browser": map[string]any{"url": u, "title": title},
			"trigger": t.Kind, "chain": t.Chain}
		return r.decide(ctx, t, rec, body, what)
	case "confirm":
		what := str("text")
		if id == 0 || what == "" {
			return "confirm needs the id of the button and exactly what pressing it will do", false, ""
		}
		u, title := s.URL()
		s.Pin(3 * time.Hour)
		body := map[string]any{"text": "May I press this on " + title + "? " + what,
			"options": []string{BrowserYes, "No"}, "tool": BrowserConfirmTool,
			"args":    map[string]any{"id": id, "url": u, "what": what},
			"browser": map[string]any{"url": u, "title": title},
			"trigger": t.Kind, "chain": t.Chain}
		return r.decide(ctx, t, rec, body, what)
	}
	return "unknown browser action " + action, false, ""
}

// decide records a question for the person and ends the run.
func (r *Runner) decide(ctx context.Context, t Trigger, rec *runRec, body map[string]any, summary string) (string, bool, string) {
	var refs *protolog.Refs
	if t.Entry != "" {
		refs = &protolog.Refs{DerivedFrom: []string{t.Entry}}
	}
	id, err := r.append(ctx, t.Thread, protolog.Draft{Kind: KindDecision, Lane: protolog.LaneContent, Refs: refs, Body: body})
	if err != nil {
		return "error: " + err.Error(), false, ""
	}
	rec.Outputs = append(rec.Outputs, id)
	return summary, true, "waiting"
}

// pressConfirmed presses the button the person said yes to, if the browser
// is still on the page they saw.
func (r *Runner) pressConfirmed(args map[string]any) string {
	if r.Browser == nil {
		return "The browser is not available here, so nothing was pressed."
	}
	s := r.Browser.Existing(r.DataDir)
	if s == nil {
		return "The browser session ended before the answer came, so nothing was pressed. Open the page again, get back to that step, and confirm again."
	}
	want, _ := args["url"].(string)
	if u, _ := s.URL(); u != want {
		return "The browser is no longer on the page the person approved (" + want + "), so nothing was pressed. Get back to that step and confirm again."
	}
	id := 0
	if f, ok := args["id"].(float64); ok {
		id = int(f)
	} else if n, ok := args["id"].(int); ok {
		id = n
	}
	if err := s.Click(id); err != nil {
		return "Pressing it failed: " + err.Error()
	}
	text, _ := s.Read()
	u, _ := s.URL()
	return "Pressed. The page now:\n" + untrusted(u, text)
}
