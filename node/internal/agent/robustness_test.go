package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A Python package that also ships a package.json for its front-end assets
// is checked as Python when Python changes, and as JavaScript when the
// JavaScript does.
func TestChecksFollowTheChangedFilesLanguage(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"package.json":  `{"scripts": {"test": "jest"}}`,
		"setup.py":      "from setuptools import setup\nsetup()\n",
		"pkg/core.py":   "x = 1\n",
		"static/app.js": "let x = 1\n",
	})
	if p := projectFor(root, filepath.Join(root, "pkg/core.py")); p == nil || p.Kind != "python" {
		t.Fatalf("a .py change was given to %+v", p)
	}
	if p := projectFor(root, filepath.Join(root, "static/app.js")); p == nil || p.Kind != "node" {
		t.Fatalf("a .js change was given to %+v", p)
	}
	kinds := map[string]bool{}
	for _, p := range projectsUnder(root) {
		kinds[p.Kind] = true
	}
	if !kinds["python"] || !kinds["node"] {
		t.Fatalf("the repository profile hid one of its projects: %v", kinds)
	}
}

// A reply cut off part-way through its body is a dropped connection like
// any other, and is asked again rather than ending the task.
func TestABodyCutOffMidwayIsAskedAgain(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Content-Length", "500")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"choices": [{"mess`)
			if hj, ok := w.(http.Hijacker); ok {
				c, _, _ := hj.Hijack()
				c.Close()
			}
			return
		}
		answering("whole")(w, r)
	}))
	defer srv.Close()
	m, _, err := client(srv.URL).Complete(context.Background(), []Message{{Content: "x"}}, nil)
	if err != nil || m.Content != "whole" {
		t.Fatalf("a cut-off body ended the call: %v %q", err, m.Content)
	}
}

// Streaming gets the same patience, as long as nothing has been shown.
func TestAStreamThatDropsBeforeSpeakingIsAskedAgain(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			if hj, ok := w.(http.Hijacker); ok {
				c, _, _ := hj.Hijack()
				c.Close()
			}
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer srv.Close()
	var shown strings.Builder
	m, _, err := client(srv.URL).Stream(context.Background(), []Message{{Content: "x"}}, nil, func(s string) { shown.WriteString(s) })
	if err != nil || m.Content != "hello" || shown.String() != "hello" {
		t.Fatalf("stream: %v %q shown %q", err, m.Content, shown.String())
	}
}

// One slow turn is not the end of a task: it is asked again with a fresh
// allowance, and the task carries on.
func TestASlowTurnIsAskedAgain(t *testing.T) {
	var turn int32
	m := executionModel(func(ctx context.Context, msgs []Message, tools []ToolSpec) (Message, Usage, error) {
		if atomic.AddInt32(&turn, 1) == 1 {
			<-ctx.Done()
			return Message{}, Usage{}, friendly(ctx.Err())
		}
		return say("Answered after a slow start."), Usage{}, nil
	})
	f := setup(t, m)
	f.r.TurnTimeout = 200 * time.Millisecond
	f.post(t, KindNote, map[string]any{"text": "context"}, nil)
	q := f.post(t, KindQuestion, map[string]any{"text": "What is open?"}, nil)
	res := f.r.Run(context.Background(), Trigger{Kind: TriggerChat, Thread: f.thread, Entry: q.ID})
	if !strings.Contains(res.Answer, "Answered after a slow start.") {
		t.Fatalf("a slow turn ended the run: %+v", res)
	}
}

// A task that keeps reading past half its budget is told to commit to a
// change; one that has already changed something is left alone.
func TestHarnessPressesForAChangeWhenNothingHasChanged(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"notes.txt": "a\n"})
	var nudges []string
	turn := 0
	m := executionModel(func(ctx context.Context, msgs []Message, tools []ToolSpec) (Message, Usage, error) {
		turn++
		if last := msgs[len(msgs)-1]; last.Role == "user" && strings.HasPrefix(last.Content, "Harness:") {
			nudges = append(nudges, last.Content)
			return call("write_file", map[string]any{"path": "notes.txt", "content": "b\n"}), Usage{}, nil
		}
		if turn < 40 && len(nudges) == 0 {
			return call("read_file", map[string]any{"path": fmt.Sprintf("notes.txt#%d", turn)}), Usage{}, nil
		}
		if turn < 30 {
			return call("read_file", map[string]any{"path": "notes.txt"}), Usage{}, nil
		}
		return say("Changed notes."), Usage{}, nil
	})
	f := setup(t, m)
	f.r.Workspace = &Workspace{Root: root}
	f.r.RunToCompletion = true
	f.r.Effort = "low"
	f.r.Run(context.Background(), Trigger{Kind: TriggerCode, Thread: f.thread})
	if len(nudges) != 1 || !strings.Contains(nudges[0], "half of this task's budget") {
		t.Fatalf("want exactly one half-budget nudge, then quiet once a file changed; got %q", nudges)
	}
}

// Offline holds for a coding task too, which otherwise always gets the web.
func TestOfflineTakesTheWebAwayEverywhere(t *testing.T) {
	r := &Runner{Offline: true}
	for _, kind := range []string{TriggerCode, TriggerChat, TriggerSchedule} {
		if g := r.gateFor(Trigger{Kind: kind}, Brief{Web: true}, Config{AutoWeb: "any"}); g.web || g.anyHost {
			t.Fatalf("%s still has the web while offline", kind)
		}
	}
}

// A stream that produces nothing is stalled and asked again, even while
// a router keeps it open with keep-alives; one that is slow but producing,
// reasoning included, is left to finish.
func TestNoProgressNotSlownessEndsAStream(t *testing.T) {
	old := streamIdle
	streamIdle = 300 * time.Millisecond
	defer func() { streamIdle = old }()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		if atomic.AddInt32(&calls, 1) == 1 {
			for { // open, kept alive, and going nowhere
				if _, err := fmt.Fprint(w, ": PROCESSING\n\n"); err != nil {
					return
				}
				w.(http.Flusher).Flush()
				select {
				case <-r.Context().Done():
					return
				case <-time.After(100 * time.Millisecond):
				}
			}
		}
		for i := 0; i < 5; i++ { // a second and a quarter of thinking
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"reasoning\":\"hmm \"}}]}\n\n")
			w.(http.Flusher).Flush()
			time.Sleep(250 * time.Millisecond)
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"thought it through\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer srv.Close()
	m, _, err := client(srv.URL).Stream(context.Background(), []Message{{Content: "x"}}, nil, nil)
	if err != nil || m.Content != "thought it through" {
		t.Fatalf("stream: %v %q after %d calls", err, m.Content, calls)
	}
	if calls != 2 {
		t.Fatalf("want the stalled stream retried once, got %d calls", calls)
	}
}

// A provider that fails after the stream has begun says so in a chunk;
// that is a failure to retry, not an empty answer.
func TestAnErrorChunkIsNotAnEmptyAnswer(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if atomic.AddInt32(&calls, 1) == 1 {
			fmt.Fprint(w, "data: {\"error\":{\"message\":\"upstream overloaded\"}}\n\n")
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer srv.Close()
	m, _, err := client(srv.URL).Stream(context.Background(), []Message{{Content: "x"}}, nil, nil)
	if err != nil || m.Content != "ok" {
		t.Fatalf("stream: %v %q", err, m.Content)
	}
}
