package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// State is what the scheduler needs to remember between ticks and restarts:
// where it last looked in each thread, and how much the agent has done today.
// The log is the record of what happened; this is only bookkeeping.
type State struct {
	mu   sync.Mutex
	path string

	Day     string `json:"day"`
	Runs    int    `json:"runs"`
	Fetches int    `json:"fetches"`
	Tokens  int    `json:"tokens"`

	Threads map[string]*ThreadState `json:"threads"`

	LastRun       string `json:"last_run,omitempty"`
	LastError     string `json:"last_error,omitempty"`
	LastSync      string `json:"last_sync,omitempty"`
	LastSyncError string `json:"last_sync_error,omitempty"`

	// Running is the thread a run is in progress on; not persisted.
	Running string `json:"-"`
}

// ThreadState is per-thread bookkeeping.
type ThreadState struct {
	// Heads is chain head seq per "author|lane", so new entries can be found
	// without an in-process hook (the MCP server is a separate process).
	Heads   map[string]uint64 `json:"heads"`
	LastRun string            `json:"last_run,omitempty"`
	LastNew time.Time         `json:"last_new,omitempty"`
	Pending string            `json:"pending,omitempty"` // triggering entry id
	Runs    int               `json:"runs"`
	Seen    bool              `json:"seen"`
}

func LoadState(dataDir string) *State {
	st := &State{path: filepath.Join(dataDir, "agent-state.json"), Threads: map[string]*ThreadState{}}
	if raw, err := os.ReadFile(st.path); err == nil {
		json.Unmarshal(raw, st)
	}
	if st.Threads == nil {
		st.Threads = map[string]*ThreadState{}
	}
	return st
}

func (s *State) thread(id string) *ThreadState {
	t := s.Threads[id]
	if t == nil {
		t = &ThreadState{Heads: map[string]uint64{}}
		s.Threads[id] = t
	}
	if t.Heads == nil {
		t.Heads = map[string]uint64{}
	}
	return t
}

// roll resets the daily counters when the day changes. Caller holds mu.
func (s *State) roll(now time.Time) {
	day := now.UTC().Format("2006-01-02")
	if s.Day != day {
		s.Day, s.Runs, s.Fetches, s.Tokens = day, 0, 0, 0
		for _, t := range s.Threads {
			t.Runs = 0
		}
	}
}

// save writes atomically. Caller holds mu.
func (s *State) save() {
	if s.path == "" {
		return
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if os.WriteFile(tmp, raw, 0o600) == nil {
		os.Rename(tmp, s.path)
	}
}

// Update runs fn under the lock, rolling the day first, and persists.
func (s *State) Update(now time.Time, fn func(*State)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.roll(now)
	fn(s)
	s.save()
}

// Snapshot copies the public fields for the interface.
func (s *State) Snapshot(now time.Time) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.roll(now)
	return map[string]any{
		"runs_today": s.Runs, "fetches_today": s.Fetches, "tokens_today": s.Tokens,
		"last_run": s.LastRun, "last_error": s.LastError,
		"last_sync": s.LastSync, "last_sync_error": s.LastSyncError,
		"running": s.Running,
	}
}
