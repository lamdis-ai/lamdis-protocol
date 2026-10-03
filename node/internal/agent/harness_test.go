package agent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for p, c := range files {
		abs := filepath.Join(root, p)
		os.MkdirAll(filepath.Dir(abs), 0o755)
		if err := os.WriteFile(abs, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// The model says it is done with a broken change; the harness runs the
// project's tests itself, sends the failure back, and only accepts the
// answer once they pass.
func TestHarnessSendsFailingChecksBackForRepair(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"go.mod":      "module sample\n\ngo 1.21\n",
		"add.go":      "package sample\n\nfunc Add(a, b int) int { return a * b }\n",
		"add_test.go": "package sample\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"Add(1, 2) != 3\")\n\t}\n}\n",
	})
	var sawFailure bool
	turn := 0
	m := executionModel(func(ctx context.Context, msgs []Message, tools []ToolSpec) (Message, Usage, error) {
		turn++
		switch turn {
		case 1:
			return call("edit_file", map[string]any{"path": "add.go", "old": "func Add", "new": "// Add adds.\nfunc Add"}), Usage{}, nil
		case 2:
			return say("Documented Add. All good."), Usage{}, nil
		case 3:
			last := msgs[len(msgs)-1].Content
			sawFailure = strings.Contains(last, "go test") && strings.Contains(last, "Add(1, 2) != 3")
			return call("edit_file", map[string]any{"path": "add.go", "old": "a * b", "new": "a + b"}), Usage{}, nil
		}
		return say("Fixed Add to add."), Usage{}, nil
	})
	f := setup(t, m)
	f.r.Workspace = &Workspace{Root: root}
	f.r.RunToCompletion = true
	f.r.Effort = "low"
	res := f.r.Run(context.Background(), Trigger{Kind: TriggerCode, Thread: f.thread})
	if !sawFailure {
		t.Fatal("the failing test was not sent back to the model")
	}
	if !strings.HasPrefix(res.Answer, "Fixed Add to add.") || !strings.Contains(res.Answer, "go test . passed") {
		t.Fatalf("answer does not carry the harness's verification: %q", res.Answer)
	}
	es := f.entries(t)
	var rec runRec
	json.Unmarshal(es[len(es)-1].Body, &rec)
	if rec.Task == nil || rec.Task.Repairs != 1 || rec.Task.Phase != phaseComplete || len(rec.Task.Changed) != 1 {
		t.Fatalf("run record missing the task: %+v", rec.Task)
	}
}

// When repairs run out, the answer says the checks still fail rather than
// letting the model's "done" stand.
func TestHarnessReportsUnresolvedFailures(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"go.mod":      "module sample\n\ngo 1.21\n",
		"add.go":      "package sample\n\nfunc Add(a, b int) int { return a * b }\n",
		"add_test.go": "package sample\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"no\")\n\t}\n}\n",
	})
	n := 0
	m := executionModel(func(ctx context.Context, msgs []Message, tools []ToolSpec) (Message, Usage, error) {
		n++
		if n%2 == 1 {
			return call("edit_file", map[string]any{"path": "add.go", "old": "func Add", "new": "// x\nfunc Add"}), Usage{}, nil
		}
		return say("Done."), Usage{}, nil
	})
	f := setup(t, m)
	f.r.Workspace = &Workspace{Root: root}
	f.r.RunToCompletion = true
	f.r.Effort = "low"
	res := f.r.Run(context.Background(), Trigger{Kind: TriggerCode, Thread: f.thread})
	if !strings.Contains(res.Answer, "go test . failed") || !strings.Contains(res.Answer, "Unresolved") {
		t.Fatalf("%q", res.Answer)
	}
}

// The reviewer sees only the task and the diff, and its blocking issues go
// back to the implementer.
func TestReviewerFindingsGoBack(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"notes.txt": "hello\n"})
	var reviewerSaw string
	var implementerGotIssue bool
	turn := 0
	m := executionModel(func(ctx context.Context, msgs []Message, tools []ToolSpec) (Message, Usage, error) {
		if len(tools) == 1 && tools[0].Name == "submit_review" {
			reviewerSaw = msgs[1].Content
			if len(msgs) != 2 {
				t.Errorf("reviewer was given the implementer's transcript: %d messages", len(msgs))
			}
			return call("submit_review", map[string]any{"approved": false, "issues": []map[string]any{
				{"severity": "high", "file": "notes.txt", "line": 1, "reason": "greeting lost its capital", "suggested_fix": "Hello"}}}), Usage{}, nil
		}
		turn++
		switch turn {
		case 1:
			return call("edit_file", map[string]any{"path": "notes.txt", "old": "hello", "new": "hello world"}), Usage{}, nil
		case 2:
			return say("Added world."), Usage{}, nil
		case 3:
			implementerGotIssue = strings.Contains(msgs[len(msgs)-1].Content, "greeting lost its capital")
			return call("edit_file", map[string]any{"path": "notes.txt", "old": "hello world", "new": "Hello world"}), Usage{}, nil
		}
		return say("Capitalised."), Usage{}, nil
	})
	f := setup(t, m)
	f.r.Workspace = &Workspace{Root: root}
	f.r.RunToCompletion = true
	q := f.post(t, KindQuestion, map[string]any{"text": "add world to the greeting"}, nil)
	res := f.r.Run(context.Background(), Trigger{Kind: TriggerCode, Thread: f.thread, Entry: q.ID})
	if !strings.Contains(reviewerSaw, "add world to the greeting") || !strings.Contains(reviewerSaw, "+hello world") {
		t.Fatalf("reviewer did not get the task and diff: %q", reviewerSaw)
	}
	if !implementerGotIssue {
		t.Fatal("review issue did not reach the implementer")
	}
	if !strings.Contains(res.Answer, "not re-reviewed") {
		t.Fatalf("%q", res.Answer)
	}
}

func TestStructuredRetriesOnceWithTheError(t *testing.T) {
	n := 0
	var retryPrompt string
	m := executionModel(func(ctx context.Context, msgs []Message, tools []ToolSpec) (Message, Usage, error) {
		n++
		if n == 1 {
			return call("submit_review", map[string]any{"approved": "yes"}), Usage{}, nil
		}
		retryPrompt = msgs[len(msgs)-1].Content
		return call("submit_review", map[string]any{"approved": true, "issues": []any{}}), Usage{}, nil
	})
	rec := runRec{Tokens: map[string]int{}}
	v, err := structured(context.Background(), m, []Message{{Role: "user", Content: "review"}}, reviewSpec, &rec)
	if err != nil || v["approved"] != true {
		t.Fatalf("%v %v", v, err)
	}
	if !strings.Contains(retryPrompt, "issues is required") && !strings.Contains(retryPrompt, "approved must be true or false") {
		t.Fatalf("retry did not carry the validation error: %q", retryPrompt)
	}
}

func TestValidateShape(t *testing.T) {
	ok := map[string]any{"approved": false, "issues": []any{map[string]any{"severity": "low", "file": "a", "reason": "r", "line": 3.0}}}
	if err := validateShape(ok, reviewSpec.Parameters, "r"); err != nil {
		t.Fatal(err)
	}
	bad := map[string]any{"approved": false, "issues": []any{map[string]any{"severity": "urgent", "file": "a", "reason": "r"}}}
	if err := validateShape(bad, reviewSpec.Parameters, "r"); err == nil || !strings.Contains(err.Error(), "severity") {
		t.Fatalf("enum not enforced: %v", err)
	}
}

func TestCompactionKeepsPairsAndThePersonsWords(t *testing.T) {
	tc := func(id, name string, args map[string]any) Message {
		m := call(name, args)
		m.Role = "assistant"
		m.ToolCalls[0].ID = id
		return m
	}
	msgs := []Message{{Role: "system", Content: "sys"}, {Role: "user", Content: "task"}}
	msgs = append(msgs, tc("1", "read_file", map[string]any{"path": "big.go"}), Message{Role: "tool", ToolCallID: "1", Content: strings.Repeat("line\n", 5000)})
	msgs = append(msgs, Message{Role: "user", Content: "also handle nil"})
	msgs = append(msgs, tc("2", "run", map[string]any{"command": "go test ./..."}), Message{Role: "tool", ToolCallID: "2", Content: "$ go test ./...\n[exit 1, 2s]\n--- FAIL: TestX\nx_test.go:9: boom"})
	msgs = append(msgs, tc("3", "read_file", map[string]any{"path": "x.go"}), Message{Role: "tool", ToolCallID: "3", Content: "1\tpackage x"})
	out, ok := compactMessages(msgs, 1, "Task state, kept by the harness:\ngoal: g\n")
	if !ok {
		t.Fatal("did not compact")
	}
	if len(out) != 5 || out[3].Role != "assistant" || out[4].ToolCallID != "3" {
		t.Fatalf("recent turn not kept verbatim with its result: %+v", out)
	}
	c := out[2].Content
	for _, want := range []string{"read big.go (5000 lines)", "also handle nil", "--- FAIL: TestX", "goal: g"} {
		if !strings.Contains(c, want) {
			t.Errorf("compacted context lacks %q:\n%s", want, c)
		}
	}
	if transcriptSize(out) > 2000 {
		t.Errorf("compaction kept too much: %d", transcriptSize(out))
	}
}

func TestPlanChecksDetectsProjects(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"go.mod": "module m\n", "pkg/a.go": "package pkg\n",
		"web/package.json":   `{"scripts":{"test":"vitest run","lint":"eslint .","typecheck":"tsc --noEmit"},"devDependencies":{"vitest":"1","typescript":"5"}}`,
		"web/pnpm-lock.yaml": "", "web/src/x.ts": "export const x = 1\n",
	})
	cs := planChecks(root, []string{filepath.Join(root, "pkg/a.go"), filepath.Join(root, "web/src/x.ts")}, false)
	var got []string
	for _, c := range cs {
		got = append(got, c.Kind+":"+c.Command)
	}
	s := strings.Join(got, " | ")
	for _, want := range []string{"test:go test ./pkg", "lint:go vet ./pkg", "typecheck:pnpm run typecheck", "lint:pnpm run lint", "test:pnpm exec vitest related --run src/x.ts"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in %s", want, s)
		}
	}
}

func TestCodeToolsFindSymbolsAndTests(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"go.mod":               "module m\n",
		"auth/service.go":      "package auth\n\ntype AuthService struct{}\n\nfunc (s *AuthService) Refresh() error { return nil }\n",
		"auth/service_test.go": "package auth\n\nfunc TestRefresh(t *testing.T) { new(AuthService).Refresh() }\n",
		"web/token.ts":         "export function refreshToken() {}\n",
		"web/token.test.ts":    "import { refreshToken } from './token'\n",
	})
	w := &Workspace{Root: root}
	ctx := context.Background()
	if out, _ := w.Call(ctx, "find_symbol", map[string]any{"name": "AuthService.Refresh"}); !strings.Contains(out, "auth/service.go:5") {
		t.Errorf("method: %s", out)
	}
	if out, _ := w.Call(ctx, "find_symbol", map[string]any{"name": "refreshToken"}); !strings.Contains(out, "web/token.ts:1") {
		t.Errorf("function: %s", out)
	}
	if out, _ := w.Call(ctx, "find_tests", map[string]any{"path": "web/token.ts"}); !strings.Contains(out, "web/token.test.ts") {
		t.Errorf("tests: %s", out)
	}
	if out, _ := w.Call(ctx, "find_references", map[string]any{"name": "AuthService"}); strings.Count(out, "\n") < 2 {
		t.Errorf("refs: %s", out)
	}
	p := w.RepoProfile()
	if !strings.Contains(p, "Go") || !strings.Contains(p, "AuthService") || !strings.Contains(p, "go test") {
		t.Errorf("profile: %s", p)
	}
}

func TestInvalidToolArgumentsAreSentBack(t *testing.T) {
	var got string
	n := 0
	m := executionModel(func(ctx context.Context, msgs []Message, tools []ToolSpec) (Message, Usage, error) {
		n++
		if n == 1 {
			bad := call("read_file", nil)
			bad.ToolCalls[0].Function.Arguments = `{"path": "a.go"`
			return bad, Usage{}, nil
		}
		got = msgs[len(msgs)-1].Content
		return say("ok"), Usage{}, nil
	})
	f := setup(t, m)
	f.r.Workspace = &Workspace{Root: t.TempDir()}
	f.r.Run(context.Background(), Trigger{Kind: TriggerCode, Thread: f.thread})
	if !strings.Contains(got, "not valid JSON") {
		t.Fatalf("%q", got)
	}
}
