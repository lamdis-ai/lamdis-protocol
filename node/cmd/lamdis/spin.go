package main

// Something to look at while it thinks.
//
// Waiting without knowing whether anything is happening is the difference
// between a pause and a hang, and it is worth the fifty lines to tell them
// apart. This shows what the agent is doing right now and how long it has
// been at it, and rubs itself out before anything is printed, so the
// transcript afterwards reads as if it was never there.

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

type spinner struct {
	w       io.Writer
	mu      sync.Mutex
	stop    chan struct{}
	done    chan struct{}
	what    string
	started time.Time
	on      bool
}

// newSpinner returns one that draws, or one that does nothing when nobody
// is watching a terminal.
func newSpinner(w *os.File) *spinner {
	s := &spinner{w: w, started: time.Now()}
	s.on = term.IsTerminal(int(w.Fd())) && os.Getenv("TERM") != "dumb" && os.Getenv("NO_COLOR") == ""
	return s
}

// Start begins drawing until Stop.
func (s *spinner) Start(what string) {
	if !s.on || s.stop != nil {
		return
	}
	s.what, s.started = what, time.Now()
	s.stop, s.done = make(chan struct{}), make(chan struct{})
	go s.run()
}

// Say changes the line without restarting the clock, so a long piece of
// work reads as one wait rather than several.
func (s *spinner) Say(what string) {
	s.mu.Lock()
	s.what = what
	s.mu.Unlock()
}

func (s *spinner) run() {
	defer close(s.done)
	// A quiet, even pulse. Braille dots are one cell wide everywhere.
	frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
	t := time.NewTicker(90 * time.Millisecond)
	defer t.Stop()
	i := 0
	for {
		select {
		case <-s.stop:
			s.clear()
			return
		case <-t.C:
			s.mu.Lock()
			what := s.what
			s.mu.Unlock()
			since := time.Since(s.started)
			elapsed := ""
			// Seconds only once there are some, so a quick answer never
			// flashes a number at anybody.
			if since > 2*time.Second {
				elapsed = fmt.Sprintf(" %ds", int(since.Seconds()))
			}
			fmt.Fprintf(s.w, "\r\033[K\033[2m%c %s%s\033[0m", frames[i%len(frames)], what, elapsed)
			i++
		}
	}
}

func (s *spinner) clear() { fmt.Fprint(s.w, "\r\033[K") }

// Stop takes the line back. Whatever is printed next starts on clean paper.
func (s *spinner) Stop() {
	if !s.on || s.stop == nil {
		return
	}
	close(s.stop)
	<-s.done
	s.stop, s.done = nil, nil
}

// Note prints a line above the spinner without disturbing it.
func (s *spinner) Note(format string, a ...any) {
	if s.on && s.stop != nil {
		s.clear()
	}
	fmt.Fprintf(s.w, format, a...)
}

// doing turns a tool call into something worth reading while you wait.
func doing(tool string, args map[string]any) string {
	get := func(k string) string {
		v, _ := args[k].(string)
		return strings.TrimSpace(v)
	}
	short := func(s string, n int) string {
		if len(s) <= n {
			return s
		}
		return s[:n] + "…"
	}
	switch tool {
	case "read_file":
		return "reading " + short(get("path"), 48)
	case "write_file":
		return "writing " + short(get("path"), 48)
	case "edit_file":
		return "editing " + short(get("path"), 48)
	case "list_files":
		return "looking around"
	case "search_files":
		return "searching for " + short(get("pattern"), 36)
	case "run":
		return "running " + short(get("command"), 44)
	case "fetch_url":
		return "fetching " + short(get("url"), 44)
	case "search_context":
		return "searching your record"
	case "read_thread", "list_threads":
		return "reading your record"
	case "open_path":
		return "asking about " + short(get("path"), 40)
	case "where":
		return "checking where it may work"
	case "ask_person":
		return "asking you"
	}
	return "working"
}
