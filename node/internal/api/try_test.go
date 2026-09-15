package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/agent"
)

type fakeModel struct {
	reply  string
	calls  int
	lastIn string
}

func (f *fakeModel) Complete(ctx context.Context, msgs []agent.Message, tools []agent.ToolSpec) (agent.Message, agent.Usage, error) {
	f.calls++
	if len(msgs) > 1 {
		f.lastIn = msgs[1].Content
	}
	return agent.Message{Role: "assistant", Content: f.reply}, agent.Usage{Prompt: 100, Completion: 20}, nil
}

func tryServer(t *testing.T, m agent.Model) (*Try, http.Handler) {
	t.Helper()
	tr := &Try{Model: m, ModelName: "test", Origin: "https://lamdis.ai",
		PerVisitorPerDay: 3, PerVisitorPerMin: 2, GlobalPerDay: 5,
		Logf: func(string, ...any) {}}
	return tr, tr.Handler()
}

func ask(h http.Handler, ip, session, text string) (int, map[string]any) {
	body, _ := json.Marshal(map[string]string{"session": session, "text": text})
	r := httptest.NewRequest("POST", "/v1/try", strings.NewReader(string(body)))
	r.Header.Set("X-Forwarded-For", "203.0.113.9, "+ip)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var out map[string]any
	json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

// Somebody types a question and gets a real answer against a record that is
// already there. The question and the answer both land in the thread, which
// is the thing the page is trying to show.
func TestTryAnswersIntoTheThread(t *testing.T) {
	m := &fakeModel{reply: "Dalton is cheaper by £500 once the migration is counted."}
	_, h := tryServer(t, m)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v1/try", nil))
	var start map[string]any
	json.Unmarshal(w.Body.Bytes(), &start)
	if start["on"] != true {
		t.Fatal("the demo says it is off")
	}
	seeded := start["entries"].([]any)
	if len(seeded) < 4 {
		t.Fatalf("the record should already have something in it: %v", seeded)
	}
	sess, _ := start["session"].(string)

	code, out := ask(h, "198.51.100.7", sess, "which one actually costs less?")
	if code != http.StatusOK || out["error"] != nil {
		t.Fatalf("ask: %d %v", code, out)
	}
	if out["answer"] != m.reply {
		t.Fatalf("answer: %v", out["answer"])
	}
	entries := out["entries"].([]any)
	if len(entries) != len(seeded)+2 {
		t.Fatalf("expected the question and the answer to be added: %d -> %d", len(seeded), len(entries))
	}
	last := entries[len(entries)-1].(map[string]any)
	if last["agent"] != true || last["text"] != m.reply {
		t.Fatalf("the answer is not marked as the agent's: %v", last)
	}
	asked := entries[len(entries)-2].(map[string]any)
	if asked["asked"] != true {
		t.Fatalf("the question is not marked: %v", asked)
	}
	// The model was given the record, not a blank page.
	if !strings.Contains(m.lastIn, "Marek") || !strings.Contains(strings.ToLower(m.lastIn), "migration") {
		t.Fatalf("the record did not reach the model: %s", m.lastIn)
	}
	if !strings.Contains(m.lastIn, "which one actually costs less?") {
		t.Fatalf("the question did not reach the model: %s", m.lastIn)
	}
}

// Three limits, and the one that matters is not any of them: it is the cap
// on the key. These only make getting there tedious.
func TestTryLimitsAbuse(t *testing.T) {
	m := &fakeModel{reply: "ok"}
	tr, h := tryServer(t, m) // 3/visitor/day, 2/visitor/min, 5/day total

	// Two in a minute are fine, the third is asked to slow down.
	for i := 0; i < 2; i++ {
		if _, out := ask(h, "198.51.100.1", "", "q"); out["error"] != nil {
			t.Fatalf("question %d refused: %v", i, out["error"])
		}
	}
	_, out := ask(h, "198.51.100.1", "", "q")
	if out["error"] == nil || !strings.Contains(out["error"].(string), "minute") {
		t.Fatalf("the per-minute limit did not bite: %v", out)
	}

	// A minute later the same visitor gets their third, then runs out for the day.
	base := time.Now()
	tr.Now = func() time.Time { return base.Add(2 * time.Minute) }
	if _, out := ask(h, "198.51.100.1", "", "q"); out["error"] != nil {
		t.Fatalf("after a minute it should answer: %v", out["error"])
	}
	if _, out := ask(h, "198.51.100.1", "", "q"); out["error"] == nil {
		t.Fatal("the per-visitor daily limit did not bite")
	}

	// A different visitor is unaffected until the whole day runs out.
	if _, out := ask(h, "198.51.100.2", "", "q"); out["error"] != nil {
		t.Fatalf("a different visitor was blocked: %v", out["error"])
	}
	if _, out := ask(h, "198.51.100.3", "", "q"); out["error"] != nil {
		t.Fatalf("a third visitor was blocked: %v", out["error"])
	}
	_, out = ask(h, "198.51.100.4", "", "q")
	if out["error"] == nil || !strings.Contains(out["error"].(string), "today") {
		t.Fatalf("the daily total did not bite: %v", out)
	}
	if m.calls > 5 {
		t.Fatalf("the model was called %d times past a limit of 5", m.calls)
	}
}

func TestTrySaysSoWhenItIsOff(t *testing.T) {
	_, h := tryServer(t, nil)
	_, out := ask(h, "198.51.100.5", "", "hello")
	if out["error"] == nil || !strings.Contains(out["error"].(string), "off") {
		t.Fatalf("an off demo should say so: %v", out)
	}
}

func TestTryTruncatesAndAnswersOnlyTheSite(t *testing.T) {
	m := &fakeModel{reply: "ok"}
	tr, h := tryServer(t, m)
	tr.MaxChars = 20
	long := strings.Repeat("x", 5000)
	if _, out := ask(h, "198.51.100.6", "", long); out["error"] != nil {
		t.Fatalf("refused: %v", out["error"])
	}
	if strings.Contains(m.lastIn, strings.Repeat("x", 25)) {
		t.Fatalf("a long question was not trimmed: %d characters reached the model", strings.Count(m.lastIn, "x"))
	}

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("OPTIONS", "/v1/try", nil))
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://lamdis.ai" {
		t.Fatalf("the demo answers the wrong origin: %q", got)
	}
}

// Sessions are forgotten, because the demo is not a place to keep anything.
func TestTryForgetsOldSessions(t *testing.T) {
	m := &fakeModel{reply: "ok"}
	tr, h := tryServer(t, m)
	_, out := ask(h, "198.51.100.8", "", "q")
	sess := out["session"].(string)
	base := time.Now()
	tr.Now = func() time.Time { return base.Add(2 * time.Hour) }
	_, out = ask(h, "198.51.100.9", sess, "q")
	if out["session"] == sess {
		t.Fatal("an hour-old session survived")
	}
	entries := out["entries"].([]any)
	if len(entries) != len(seeded(base))+2 {
		t.Fatalf("a forgotten session should start fresh: %d entries", len(entries))
	}
}
