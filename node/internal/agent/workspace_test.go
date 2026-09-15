package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceStaysInsideRoot(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello\nworld\n"), 0o644)
	outside := filepath.Join(filepath.Dir(root), "outside.txt")
	os.WriteFile(outside, []byte("secret"), 0o644)
	defer os.Remove(outside)
	os.Symlink(outside, filepath.Join(root, "link.txt"))
	w := &Workspace{Root: root}
	ctx := context.Background()
	for _, p := range []string{"../outside.txt", outside, "link.txt", "/etc/passwd"} {
		out, _ := w.Call(ctx, "read_file", map[string]any{"path": p})
		if !strings.HasPrefix(out, "error:") {
			t.Fatalf("%s should be refused, got %q", p, out)
		}
	}
	out, _ := w.Call(ctx, "read_file", map[string]any{"path": "a.txt", "start": 2.0})
	if !strings.Contains(out, "2\tworld") || strings.Contains(out, "hello") {
		t.Fatalf("line range wrong: %q", out)
	}
	if out, _ := w.Call(ctx, "write_file", map[string]any{"path": "../escape.txt", "content": "x"}); !strings.HasPrefix(out, "error:") {
		t.Fatal("write outside the root was allowed")
	}
}

func TestEditRequiresUniqueMatch(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "m.go"), []byte("func a() {}\nfunc b() {}\nfunc a() {}\n"), 0o644)
	w := &Workspace{Root: root}
	ctx := context.Background()
	if out, _ := w.Call(ctx, "edit_file", map[string]any{"path": "m.go", "old": "func a() {}", "new": "func a() int { return 1 }"}); !strings.Contains(out, "more than once") {
		t.Fatalf("ambiguous edit accepted: %q", out)
	}
	if out, _ := w.Call(ctx, "edit_file", map[string]any{"path": "m.go", "old": "func zz() {}", "new": "x"}); !strings.Contains(out, "not found") {
		t.Fatalf("missing edit accepted: %q", out)
	}
	if out, _ := w.Call(ctx, "edit_file", map[string]any{"path": "m.go", "old": "func b() {}", "new": "func b() int { return 2 }"}); !strings.HasPrefix(out, "edited") {
		t.Fatalf("unique edit failed: %q", out)
	}
	raw, _ := os.ReadFile(filepath.Join(root, "m.go"))
	if !strings.Contains(string(raw), "return 2") {
		t.Fatal("edit not written")
	}
}

func TestRunIsDistilledAndScoped(t *testing.T) {
	root := t.TempDir()
	w := &Workspace{Root: root}
	ctx := context.Background()
	out, _ := w.Call(ctx, "run", map[string]any{"command": "pwd && seq 1 500"})
	if !strings.Contains(out, "exit 0") || !strings.Contains(out, "lines omitted") || !strings.Contains(out, "\n500") {
		t.Fatalf("run output not distilled as expected:\n%s", out[:min(len(out), 400)])
	}
	if !strings.Contains(out, filepath.Base(root)) {
		t.Fatal("command did not run in the workspace")
	}
	out, _ = w.Call(ctx, "run", map[string]any{"command": "exit 3"})
	if !strings.Contains(out, "exit 3") {
		t.Fatalf("exit status lost: %s", out)
	}
	out, _ = w.Call(ctx, "search_files", map[string]any{"pattern": "nothing-here-\\d+"})
	if out != "no matches" {
		t.Fatalf("search: %q", out)
	}
}

func TestMapSkipsNoise(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "node_modules", "x"), 0o755)
	os.WriteFile(filepath.Join(root, "node_modules", "x", "i.js"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0o644)
	m := (&Workspace{Root: root}).Map()
	if !strings.Contains(m, "main.go") || strings.Contains(m, "node_modules") {
		t.Fatalf("map: %q", m)
	}
}
