package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type executionModel func(context.Context, []Message, []ToolSpec) (Message, Usage, error)

func (m executionModel) Complete(ctx context.Context, msgs []Message, tools []ToolSpec) (Message, Usage, error) {
	return m(ctx, msgs, tools)
}

func TestAlternatingLoopProducesSummary(t *testing.T) {
	var turns []Message
	// a b a b a(warned) b(warned) a b a(fifth: stop)
	for i := 0; i < 9; i++ {
		turns = append(turns, call("read_thread", map[string]any{"thread": string(rune('a' + i%2))}))
	}
	m := &script{turns: append(turns, say("Stopped; no changes made."))}
	f := setup(t, m)
	f.r.RunToCompletion = true
	res := f.r.Run(context.Background(), Trigger{Kind: TriggerCode, Thread: f.thread})
	if res.Outcome != "answered" || res.Answer != "Stopped; no changes made." {
		t.Fatalf("%+v", res)
	}
	if len(m.tools[len(m.tools)-1]) != 0 {
		t.Fatal("summary still has tools")
	}
}

func TestExploreAgentsOverlapAndCannotWrite(t *testing.T) {
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	var parents atomic.Int32
	model := executionModel(func(ctx context.Context, msgs []Message, tools []ToolSpec) (Message, Usage, error) {
		child := len(msgs) > 2 && strings.Contains(msgs[2].Content, "read-only explore subagent")
		if child {
			for _, s := range tools {
				if s.Name != "submit_findings" && (!parallelRead(s.Name) || s.Name == "explore") {
					t.Errorf("unsafe child tool: %s", s.Name)
				}
			}
			ready <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
				return Message{}, Usage{}, ctx.Err()
			}
			return call("submit_findings", map[string]any{"summary": "Evidence from child.", "files": []string{"a.go"}}), Usage{Prompt: 7, Completion: 3}, nil
		}
		if parents.Add(1) == 1 {
			a := call("explore", map[string]any{"task": "Inspect A"})
			b := call("explore", map[string]any{"task": "Inspect B"})
			b.ToolCalls[0].ID = "c2"
			a.ToolCalls = append(a.ToolCalls, b.ToolCalls...)
			return a, Usage{}, nil
		}
		if !strings.Contains(msgs[len(msgs)-1].Content, "Evidence from child.") {
			t.Errorf("findings did not reach the parent: %q", msgs[len(msgs)-1].Content)
		}
		return say("Research complete."), Usage{}, nil
	})
	f := setup(t, model)
	f.r.Workspace = &Workspace{Root: t.TempDir()}
	done := make(chan Result, 1)
	go func() { done <- f.r.Run(context.Background(), Trigger{Kind: TriggerCode, Thread: f.thread}) }()
	for i := 0; i < 2; i++ {
		select {
		case <-ready:
		case <-time.After(5 * time.Second):
			close(release)
			t.Fatal("subagents did not overlap")
		}
	}
	close(release)
	res := <-done
	if res.Answer != "Research complete." {
		t.Fatalf("%+v", res)
	}
	es := f.entries(t)
	var rec runRec
	json.Unmarshal(es[len(es)-1].Body, &rec)
	if rec.Tokens["prompt"] != 14 || rec.Tokens["completion"] != 6 {
		t.Fatalf("child usage missing: %+v", rec.Tokens)
	}
}

func TestWritesAreBarriers(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	m := executionModel(func(ctx context.Context, msgs []Message, tools []ToolSpec) (Message, Usage, error) {
		if len(msgs) == 2 {
			var batch Message
			for i, c := range []Message{
				call("read_file", map[string]any{"path": "sample.txt"}),
				call("write_file", map[string]any{"path": "sample.txt", "content": "new"}),
				call("read_file", map[string]any{"path": "sample.txt"}),
			} {
				c.ToolCalls[0].ID = string(rune('a' + i))
				batch.ToolCalls = append(batch.ToolCalls, c.ToolCalls...)
			}
			return batch, Usage{}, nil
		}
		if !strings.Contains(msgs[3].Content, "old") || !strings.Contains(msgs[5].Content, "new") {
			t.Errorf("write barrier broken: %+v", msgs[3:])
		}
		return say("Done."), Usage{}, nil
	})
	f := setup(t, m)
	f.r.Workspace = &Workspace{Root: root}
	f.r.Effort = "low" // no reviewer: this test is about ordering
	res := f.r.Run(context.Background(), Trigger{Kind: TriggerCode, Thread: f.thread})
	if !strings.HasPrefix(res.Answer, "Done.") || !strings.Contains(res.Answer, "Changed: sample.txt") {
		t.Fatalf("%+v", res)
	}
}

func TestModelFailureStillAnswers(t *testing.T) {
	f := setup(t, executionModel(func(context.Context, []Message, []ToolSpec) (Message, Usage, error) {
		return Message{}, Usage{}, errors.New("model unavailable")
	}))
	res := f.r.Run(context.Background(), Trigger{Kind: TriggerCode, Thread: f.thread})
	if res.Outcome != "answered" || !strings.Contains(res.Answer, "model unavailable") {
		t.Fatalf("%+v", res)
	}
}

func TestEmptyResponseRequestsToolsFreeSummary(t *testing.T) {
	m := &script{turns: []Message{say(""), say("I could not complete this task.")}}
	f := setup(t, m)
	res := f.r.Run(context.Background(), Trigger{Kind: TriggerCode, Thread: f.thread})
	if res.Answer != "I could not complete this task." || len(m.tools[1]) != 0 {
		t.Fatalf("%+v", res)
	}
}

func TestInterjectionsAreBatched(t *testing.T) {
	m := executionModel(func(ctx context.Context, msgs []Message, tools []ToolSpec) (Message, Usage, error) {
		if len(msgs) != 3 || !strings.Contains(msgs[2].Content, "first\nsecond") {
			t.Errorf("not batched: %+v", msgs)
		}
		return say("Answered."), Usage{}, nil
	})
	f := setup(t, m)
	ch := make(chan string, 2)
	ch <- "first"
	ch <- "second"
	close(ch)
	f.r.Interjections = ch
	res := f.r.Run(context.Background(), Trigger{Kind: TriggerCode, Thread: f.thread})
	if res.Answer != "Answered." {
		t.Fatalf("%+v", res)
	}
}

func TestParallelAllowlist(t *testing.T) {
	for _, name := range []string{"run", "write_file", "edit_file", "ask_person", "post_note", "browser", "open_path", "search_context", "unknown", "mcp.read", "run_tests", "run_checks"} {
		if parallelRead(name) {
			t.Errorf("unsafe parallel tool: %s", name)
		}
	}
}
