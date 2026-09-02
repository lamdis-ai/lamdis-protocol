package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The reuse corpus must survive a restart. In memory it forgot every
// photograph at each deploy, and the same picture earned again the morning
// after.
func TestCorpusSurvivesARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corpus.jsonl")
	c, err := OpenCorpus(path)
	if err != nil {
		t.Fatal(err)
	}
	c.Add(Evidence{EntryID: "job-1:w1#0", SHA256: "aa11", PerceptualHash: 0xF0F0F0F0F0F0F0F0})

	again, err := OpenCorpus(path)
	if err != nil {
		t.Fatal(err)
	}
	if exact, _, prior := again.Seen(Evidence{EntryID: "job-2:w2#0", SHA256: "aa11"}); !exact || prior != "job-1:w1#0" {
		t.Fatalf("an exact reuse was forgotten across a restart: %v %q", exact, prior)
	}
	if _, near, _ := again.Seen(Evidence{EntryID: "job-2:w2#0", SHA256: "bb22",
		PerceptualHash: 0xF0F0F0F0F0F0F0F1}); !near {
		t.Fatal("a near-duplicate was forgotten across a restart")
	}
	// The same entry does not collide with itself.
	if exact, near, _ := again.Seen(Evidence{EntryID: "job-1:w1#0", SHA256: "aa11",
		PerceptualHash: 0xF0F0F0F0F0F0F0F0}); exact || near {
		t.Fatal("an entry matched itself")
	}
	// Additions after reopening keep appending.
	again.Add(Evidence{EntryID: "job-3:w3#0", SHA256: "cc33"})
	b, _ := os.ReadFile(path)
	if lines := strings.Count(strings.TrimSpace(string(b)), "\n") + 1; lines != 2 {
		t.Fatalf("corpus file holds %d lines, want 2:\n%s", lines, b)
	}
	// A duplicate add writes nothing new.
	again.Add(Evidence{EntryID: "job-4:w4#0", SHA256: "cc33"})
	b, _ = os.ReadFile(path)
	if lines := strings.Count(strings.TrimSpace(string(b)), "\n") + 1; lines != 2 {
		t.Fatalf("a duplicate add grew the file to %d lines", lines)
	}
}

// A file that does not exist yet is an empty corpus, not an error; a
// directory that cannot be written is.
func TestOpenCorpusOnAMissingFile(t *testing.T) {
	c, err := OpenCorpus(filepath.Join(t.TempDir(), "new.jsonl"))
	if err != nil || c == nil {
		t.Fatalf("a missing corpus file failed to open: %v", err)
	}
	if _, err := OpenCorpus(filepath.Join(t.TempDir(), "missing-dir", "x.jsonl")); err == nil {
		t.Fatal("an unwritable corpus location opened without complaint")
	}
}
