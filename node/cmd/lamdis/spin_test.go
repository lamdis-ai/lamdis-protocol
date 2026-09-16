package main

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

// safeBuf is written by the spinner's goroutine and read by the test.
type safeBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *safeBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}
func (s *safeBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func drawing() (*spinner, *safeBuf) {
	buf := &safeBuf{}
	return &spinner{w: buf, on: true}, buf
}

// A wait has to say something, or it is indistinguishable from a hang.
func TestItSaysSomethingWhileYouWait(t *testing.T) {
	s, buf := drawing()
	s.Start("thinking")
	time.Sleep(300 * time.Millisecond)
	out := buf.String()
	s.Stop()

	if !strings.Contains(out, "thinking") {
		t.Fatalf("it never said what it was doing: %q", out)
	}
	frames := 0
	for _, r := range "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏" {
		frames += strings.Count(out, string(r))
	}
	if frames < 2 {
		t.Fatalf("it is not moving, so it reads as stuck: %d frames", frames)
	}
}

// What it is doing changes as it goes, without restarting the clock.
func TestItKeepsUpWithTheWork(t *testing.T) {
	s, buf := drawing()
	s.Start("thinking")
	time.Sleep(150 * time.Millisecond)
	s.Say("reading notes.txt")
	time.Sleep(150 * time.Millisecond)
	s.Stop()

	out := buf.String()
	if !strings.Contains(out, "thinking") || !strings.Contains(out, "reading notes.txt") {
		t.Fatalf("it did not follow the work: %q", out)
	}
}

// Afterwards the transcript should read as though nothing was ever
// spinning: no half-drawn frame left behind the answer.
func TestItRubsItselfOut(t *testing.T) {
	s, buf := drawing()
	s.Start("thinking")
	time.Sleep(200 * time.Millisecond)
	s.Stop()

	out := buf.String()
	if !strings.HasSuffix(out, "\r\033[K") {
		t.Fatalf("it left something on the line: %q", out[max(0, len(out)-30):])
	}
	// And a note printed mid-flight starts on a clean line.
	s.Start("thinking")
	time.Sleep(120 * time.Millisecond)
	s.Note("  → read_file notes.txt\n")
	s.Stop()
	if !strings.Contains(buf.String(), "\r\033[K  → read_file notes.txt\n") {
		t.Fatal("a note was printed over the spinner")
	}
}

// Nobody is watching a pipe, and a log full of spinner frames is worse
// than useless.
func TestItStaysQuietWhenNobodyIsWatching(t *testing.T) {
	buf := &safeBuf{}
	s := &spinner{w: buf, on: false}
	s.Start("thinking")
	s.Say("reading")
	time.Sleep(150 * time.Millisecond)
	s.Stop()
	if buf.String() != "" {
		t.Fatalf("it drew into a pipe: %q", buf.String())
	}
	// Notes still get through, because those are the actual output.
	s.Note("done\n")
	if buf.String() != "done\n" {
		t.Fatalf("notes were lost: %q", buf.String())
	}
}

// The line should say the thing a person would say.
func TestItNamesTheWorkInWords(t *testing.T) {
	for _, c := range []struct{ tool, want string }{
		{"read_file", "reading"},
		{"run", "running"},
		{"search_files", "searching for"},
		{"fetch_url", "fetching"},
		{"search_context", "searching your record"},
		{"ask_person", "asking you"},
	} {
		got := doing(c.tool, map[string]any{"path": "a.go", "command": "go test ./...",
			"pattern": "func main", "url": "https://example.com"})
		if !strings.Contains(got, c.want) {
			t.Errorf("%s reads as %q, wanted something with %q", c.tool, got, c.want)
		}
	}
	// A long argument is trimmed rather than wrapping the terminal.
	long := doing("run", map[string]any{"command": strings.Repeat("x", 300)})
	if len(long) > 60 {
		t.Fatalf("that will wrap: %d characters", len(long))
	}
}
