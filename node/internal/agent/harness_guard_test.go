package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A run that ends part-way through an edit does not leave a file that no
// longer parses: it is put back, and the report says so.
func TestAFileLeftUnparseableIsPutBack(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}
	root := t.TempDir()
	good := "def area(w, h):\n    return w * h\n"
	writeFiles(t, root, map[string]string{"shapes.py": good, "notes.md": "x\n"})
	turn := 0
	m := executionModel(func(ctx context.Context, msgs []Message, tools []ToolSpec) (Message, Usage, error) {
		turn++
		if turn == 1 {
			return call("edit_file", map[string]any{"path": "shapes.py", "old": "    return w * h", "new": "    if w < 0:\n    return w * h"}), Usage{}, nil
		}
		return call("read_file", map[string]any{"path": "notes.md#" + string(rune('a'+turn%26))}), Usage{}, nil
	})
	f := setup(t, m)
	f.r.Workspace = &Workspace{Root: root}
	f.r.RunToCompletion = true
	f.r.Effort = "low"
	res := f.r.Run(context.Background(), Trigger{Kind: TriggerCode, Thread: f.thread})
	raw, _ := os.ReadFile(filepath.Join(root, "shapes.py"))
	if string(raw) != good {
		t.Fatalf("the broken edit was left in place:\n%s", raw)
	}
	if !strings.Contains(res.Answer, "put shapes.py back as it was") {
		t.Fatalf("the report does not say the file was put back: %q", res.Answer)
	}
}

// Only what the change added counts against it.
func TestOnlyNewFailuresAreTheChanges(t *testing.T) {
	before := checkResult{Status: "failed", Failures: []string{"FAIL test_a (0.31s)", "ImportError: no module named yaml"}}
	same := checkResult{Status: "failed", Failures: []string{"FAIL test_a (0.29s)", "ImportError: no module named yaml"}}
	if n := newFailures(same, before); len(n) != 0 {
		t.Fatalf("timings made old failures look new: %q", n)
	}
	worse := checkResult{Status: "failed", Failures: []string{"FAIL test_a (0.3s)", "FAIL test_b"}}
	if n := newFailures(worse, before); len(n) != 1 || n[0] != "FAIL test_b" {
		t.Fatalf("new failures: %q", n)
	}
	if n := newFailures(worse, checkResult{Status: "passed"}); len(n) != 2 {
		t.Fatalf("against a passing original, every failure is new: %q", n)
	}
}

// A check that fails only because of the change is the change's; one that
// failed the same way before is marked as the project's, and the change is
// back in place either way.
func TestJudgeComparesAgainstTheOriginal(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"go.mod":        "module sample\n\ngo 1.21\n",
		"add.go":        "package sample\n\nfunc Add(a, b int) int { return a + b }\n",
		"add_test.go":   "package sample\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"add\")\n\t}\n}\n",
		"other_test.go": "package sample\n\nimport \"testing\"\n\nfunc TestOther(t *testing.T) { t.Fatal(\"always\") }\n",
	})
	f := setup(t, nil)
	f.r.Workspace = &Workspace{Root: root}
	h := newHarness(context.Background(), f.r, "")
	file := filepath.Join(root, "add.go")
	h.before("edit_file", map[string]any{"path": "add.go"})
	h.observe("edit_file", map[string]any{"path": "add.go"}, "ok")
	c := check{Kind: "test", Dir: root, Command: "go test ."}

	// A harmless change: only TestOther fails, as it did before.
	os.WriteFile(file, []byte("package sample\n\n// Add adds.\nfunc Add(a, b int) int { return a + b }\n"), 0o644)
	files := h.changed(context.Background())
	if r := h.judge(context.Background(), files, runCheck(context.Background(), c)); r.Status != statusFailingBefore {
		t.Fatalf("a failure that predates the change was blamed on it: %s %q", r.Status, r.Failures)
	}
	// A breaking change: TestAdd fails now, and that is the change's.
	broken := "package sample\n\nfunc Add(a, b int) int { return a - b }\n"
	os.WriteFile(file, []byte(broken), 0o644)
	r := h.judge(context.Background(), files, runCheck(context.Background(), c))
	if r.Status != "failed" || !strings.Contains(strings.Join(r.Failures, "\n"), "TestAdd") {
		t.Fatalf("the change's own failure was missed: %s %q", r.Status, r.Failures)
	}
	if raw, _ := os.ReadFile(file); string(raw) != broken {
		t.Fatalf("the change was not put back after the comparison:\n%s", raw)
	}
}

// A comparison interrupted by a crash leaves the originals in place; the
// next task in the workspace puts the change back before it starts.
func TestAnInterruptedComparisonIsUndone(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"app.py": "original\n"})
	f := setup(t, nil)
	f.r.Workspace = &Workspace{Root: root}
	h := &harness{r: f.r, ws: f.r.Workspace, root: resolveSymlinks(root)}
	journal := map[string]*string{}
	body := base64.StdEncoding.EncodeToString([]byte("the change\n"))
	journal[filepath.Join(h.root, "app.py")] = &body
	raw, _ := json.Marshal(journal)
	os.MkdirAll(filepath.Dir(h.swapJournal()), 0o700)
	os.WriteFile(h.swapJournal(), raw, 0o600)

	newHarness(context.Background(), f.r, "")
	if got, _ := os.ReadFile(filepath.Join(root, "app.py")); string(got) != "the change\n" {
		t.Fatalf("the change was not restored: %q", got)
	}
	if _, err := os.Stat(h.swapJournal()); err == nil {
		t.Fatal("the journal was left behind")
	}
}

// The tests nearest a change include the ones that import it, which are
// often not named after the file.
func TestTestsThatImportAModuleAreFound(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"lib/pkg/cbook.py":             "x = 1\n",
		"lib/pkg/tests/test_pickle.py": "from pkg import cbook\n",
		"lib/pkg/tests/test_dotted.py": "import pkg.cbook as cb\n",
		"lib/pkg/tests/test_other.py":  "import pkg.figure\n",
		"lib/pkg/tests/test_cbook.py":  "x = 2\n",
	})
	var got []string
	for _, p := range testsCovering(root, filepath.Join(root, "lib/pkg/cbook.py"), 8) {
		got = append(got, filepath.Base(p))
	}
	joined := strings.Join(got, ",")
	for _, want := range []string{"test_cbook.py", "test_pickle.py", "test_dotted.py"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missed %s: %v", want, got)
		}
	}
	if strings.Contains(joined, "test_other.py") {
		t.Fatalf("a test that does not import it was included: %v", got)
	}
}

// The last fifth of a run, between ninety seconds and four minutes, is kept
// for checking and review.
func TestTheReserveIsAFifthOfTheRun(t *testing.T) {
	start := time.Now()
	ctx, cancel := context.WithDeadline(context.Background(), start.Add(20*time.Minute))
	defer cancel()
	if reserveReached(ctx, start) {
		t.Fatal("reserve reached at the start of a run")
	}
	ctx2, cancel2 := context.WithDeadline(context.Background(), time.Now().Add(3*time.Minute))
	defer cancel2()
	if !reserveReached(ctx2, start.Add(-17*time.Minute)) {
		t.Fatal("three minutes left of twenty is inside the four-minute reserve")
	}
}

// A change to project code that nothing ran goes back once, to be run.
func TestUntestedChangeGoesBack(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"setup.py": "from setuptools import setup\nsetup()\n", "pkg/mod.py": "X = 1\n"})
	var sentBack bool
	turn := 0
	m := executionModel(func(ctx context.Context, msgs []Message, tools []ToolSpec) (Message, Usage, error) {
		if len(tools) == 1 && tools[0].Name == "submit_review" {
			return call("submit_review", map[string]any{"approved": true, "issues": []map[string]any{}}), Usage{}, nil
		}
		turn++
		switch turn {
		case 1:
			return call("edit_file", map[string]any{"path": "pkg/mod.py", "old": "X = 1", "new": "X = 2"}), Usage{}, nil
		case 2:
			return say("Changed X."), Usage{}, nil
		case 3:
			sentBack = strings.Contains(msgs[len(msgs)-1].Content, "Nothing has run this change yet")
			return call("run", map[string]any{"command": "python3 -c 'import pkg.mod'"}), Usage{}, nil
		}
		return say("Changed X and imported it."), Usage{}, nil
	})
	f := setup(t, m)
	f.r.Workspace = &Workspace{Root: root}
	f.r.RunToCompletion = true
	q := f.post(t, KindQuestion, map[string]any{"text": "set X to 2"}, nil)
	f.r.Run(context.Background(), Trigger{Kind: TriggerCode, Thread: f.thread, Entry: q.ID})
	if !sentBack {
		t.Fatal("an untested change was accepted without being sent back")
	}
	if turn > 4 {
		t.Fatalf("sent back more than once: %d turns", turn)
	}
}
