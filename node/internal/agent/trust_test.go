package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The point of a setting is that it is chosen once. "home" should mean the
// agent works anywhere under home without asking again.
func TestReachFollowsTheSetting(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	root := filepath.Join(home, "work", "thing")

	roots, ask := Reach(Config{Trust: TrustProject}, root)
	if len(roots) != 0 || !ask {
		t.Fatalf("project should add nothing and still ask: %v %v", roots, ask)
	}

	roots, ask = Reach(Config{Trust: TrustHome}, root)
	if len(roots) != 1 || roots[0] != home || !ask {
		t.Fatalf("home should reach home and still ask beyond it: %v %v", roots, ask)
	}

	roots, ask = Reach(Config{Trust: TrustAll}, root)
	if len(roots) != 1 || ask {
		t.Fatalf("all should reach everything and never ask: %v %v", roots, ask)
	}

	// Somewhere allowed once is allowed at every level.
	roots, _ = Reach(Config{Trust: TrustProject, AllowPaths: []string{"/srv/data"}}, root)
	if len(roots) != 1 || roots[0] != "/srv/data" {
		t.Fatalf("a remembered directory was dropped: %v", roots)
	}
}

// "Work anywhere in my home directory" is not the same sentence as "read my
// ssh keys", and somebody saying the first rarely means the second.
func TestCredentialsAreRefusedEvenWhereWorkIsAllowed(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	w := &Workspace{Root: home, Guard: true}

	for _, p := range []string{
		filepath.Join(home, ".ssh", "id_rsa"),
		filepath.Join(home, ".aws", "credentials"),
		filepath.Join(home, ".gnupg"),
		filepath.Join(home, "Library", "Keychains", "login.keychain-db"),
		filepath.Join(home, "work", "project", ".env"),
		filepath.Join(home, ".lamdis", "person.key"),
	} {
		if _, err := w.resolve(p); err == nil {
			t.Errorf("%s was readable with the guard on", p)
		} else if !strings.Contains(err.Error(), "credential") {
			t.Errorf("%s was refused for the wrong reason: %v", p, err)
		}
	}
	// Ordinary work in the same tree is unaffected.
	ordinary := filepath.Join(home, "work", "project", "main.go")
	if _, err := w.resolve(ordinary); err != nil {
		t.Fatalf("ordinary work was refused: %v", err)
	}

	// And with the guard off, it is the person's call.
	w.Guard = false
	if _, err := w.resolve(filepath.Join(home, ".ssh", "id_rsa")); err != nil {
		t.Fatalf("turning the guard off did not: %v", err)
	}
}

// Asking twice for the same place is the thing to avoid.
func TestAnAnswerIsRememberedAndCoversWhatIsBelowIt(t *testing.T) {
	root := t.TempDir()
	deeper := filepath.Join(root, "a", "b")
	os.MkdirAll(deeper, 0o755)

	asked := 0
	var remembered []string
	w := &Workspace{Root: filepath.Join(root, "start"), Guard: true,
		Ask:      func(ctx context.Context, path, why string) (bool, error) { asked++; return true, nil },
		Remember: func(p string) { remembered = append(remembered, p) }}
	os.MkdirAll(w.Root, 0o755)

	if _, err := w.RequestPath(context.Background(), filepath.Join(root, "a"), "because"); err != nil {
		t.Fatal(err)
	}
	if asked != 1 || len(remembered) != 1 {
		t.Fatalf("the first request should ask once and be kept: asked=%d kept=%v", asked, remembered)
	}
	// Anything below it is already answered.
	if _, err := w.RequestPath(context.Background(), deeper, "again"); err != nil {
		t.Fatal(err)
	}
	if asked != 1 {
		t.Fatalf("it asked again for somewhere already allowed: %d", asked)
	}
	if _, err := w.resolve(filepath.Join(deeper, "x.txt")); err != nil {
		t.Fatalf("a file below an allowed directory was refused: %v", err)
	}
}
