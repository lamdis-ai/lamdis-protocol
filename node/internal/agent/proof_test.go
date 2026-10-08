package agent

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func goProject(t *testing.T, add string) string {
	t.Helper()
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"go.mod":      "module sample\n\ngo 1.21\n",
		"add.go":      "package sample\n\nfunc Add(a, b int) int { return " + add + " }\n",
		"add_test.go": "package sample\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"Add(1, 2) != 3\")\n\t}\n}\n",
	})
	return root
}

// The level rises with the evidence, step by step, and the record keeps
// each advance for the change it was about.
func TestProofLevelRisesWithEvidence(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	root := goProject(t, "a * b")
	turn := 0
	m := executionModel(func(ctx context.Context, msgs []Message, tools []ToolSpec) (Message, Usage, error) {
		turn++
		switch turn {
		case 1:
			return call("read_file", map[string]any{"path": "add.go"}), Usage{}, nil
		case 2:
			return call("edit_file", map[string]any{"path": "add.go", "old": "a * b", "new": "a + b"}), Usage{}, nil
		}
		return say("Fixed Add."), Usage{}, nil
	})
	f := setup(t, m)
	f.r.Workspace = &Workspace{Root: root}
	f.r.RunToCompletion, f.r.UntilStuck, f.r.Effort = true, true, "low"
	res := f.r.Run(context.Background(), Trigger{Kind: TriggerCode, Thread: f.thread})
	es := f.entries(t)
	var rec runRec
	json.Unmarshal(es[len(es)-1].Body, &rec)
	if rec.Task == nil || proofLevel(rec.Task.Level) != levelTested {
		t.Fatalf("want level tested, got %+v", rec.Task)
	}
	last := -1
	for _, s := range rec.Task.Proof {
		if s.Level <= last && s.Change == rec.Task.Proof[len(rec.Task.Proof)-1].Change {
			t.Fatalf("the level fell or repeated for one change: %+v", rec.Task.Proof)
		}
		last = s.Level
	}
	if !strings.Contains(res.Answer, "Proof: tested (level 5 of 6)") {
		t.Fatalf("the report does not state the proof level: %q", res.Answer)
	}
}

// With the proof state off, the model's own "done" ends the task: nothing
// is checked or sent back, and the report claims nothing on its behalf.
func TestProofOffTakesTheModelsWord(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	root := goProject(t, "a * b")
	turn := 0
	m := executionModel(func(ctx context.Context, msgs []Message, tools []ToolSpec) (Message, Usage, error) {
		turn++
		if turn == 1 {
			if strings.Contains(msgs[0].Content, "harness that verifies") {
				t.Error("the prompt promises verification that will not happen")
			}
			return call("edit_file", map[string]any{"path": "add.go", "old": "func Add", "new": "// Add.\nfunc Add"}), Usage{}, nil
		}
		return say("Done."), Usage{}, nil
	})
	f := setup(t, m)
	f.r.Workspace = &Workspace{Root: root}
	f.r.RunToCompletion, f.r.UntilStuck, f.r.ProofOff, f.r.Effort = true, true, true, "low"
	res := f.r.Run(context.Background(), Trigger{Kind: TriggerCode, Thread: f.thread})
	if strings.TrimSpace(res.Answer) != "Done." || turn != 2 {
		t.Fatalf("proof off should end on the model's word after 2 turns; got %d turns, %q", turn, res.Answer)
	}
}
