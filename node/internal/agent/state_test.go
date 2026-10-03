package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStateSkipsUnchangedWritesAcrossReload(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	s := LoadState(dir)
	s.Update(now, func(s *State) { s.thread("one").Seen = true })
	before, err := os.Stat(s.path)
	if err != nil {
		t.Fatal(err)
	}
	// Both an ordinary no-op and a transient UI change leave the durable
	// file alone, including after the process loads it again.
	s = LoadState(dir)
	s.Update(now, func(s *State) { s.Running = "one" })
	s.Update(now, func(s *State) { s.thread("one").Seen = true })
	after, err := os.Stat(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("unchanged state rewrote the file")
	}
	s.Update(now, func(s *State) { s.Runs = 2 })
	if got := LoadState(dir).Runs; got != 2 {
		t.Fatalf("changed state was not persisted: runs=%d", got)
	}
	s.Update(now.Add(24*time.Hour), func(*State) {})
	loaded := LoadState(dir)
	if loaded.Day != "2026-10-02" || loaded.Runs != 0 {
		t.Fatalf("day rollover was not persisted: day=%s runs=%d", loaded.Day, loaded.Runs)
	}
}

func TestStateBatchesEditsAndRetriesFailedSave(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	s := LoadState(dir)
	s.Update(now, func(*State) {})
	s.edit(now, func(s *State) { s.thread("one").Seen = true })
	s.edit(now, func(s *State) { s.thread("two").Pending = "entry" })
	if got := len(LoadState(dir).Threads); got != 0 {
		t.Fatalf("edits persisted before the batch finished: %d threads", got)
	}
	// A failed atomic rename must not mark these edits as saved. Moving
	// the original aside and putting a directory at its path forces it.
	backup := filepath.Join(dir, "original.json")
	if err := os.Rename(s.path, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(s.path, 0o700); err != nil {
		t.Fatal(err)
	}
	s.flush()
	if err := os.Remove(s.path); err != nil {
		t.Fatal(err)
	}
	s.flush()
	loaded := LoadState(dir)
	if len(loaded.Threads) != 2 || !loaded.thread("one").Seen || loaded.thread("two").Pending != "entry" {
		t.Fatal("the batch was lost after retrying the failed save")
	}
}

func TestSchedulerIdlePollDoesNotRewriteState(t *testing.T) {
	f := setup(t, &script{})
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	f.r.Now = func() time.Time { return now }
	s := &Scheduler{Runner: f.r, State: f.r.State}
	ctx := context.Background()
	s.poll(ctx, true)
	before, err := os.Stat(s.State.path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		now = now.Add(pollEvery)
		s.poll(ctx, false)
	}
	after, err := os.Stat(s.State.path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("idle polling rewrote state")
	}
	e := f.post(t, KindNote, map[string]any{"text": "a new entry"}, nil)
	s.poll(ctx, false)
	loaded := LoadState(f.dir)
	tl, err := f.st.Thread(ctx, f.thread)
	if err != nil {
		t.Fatal(err)
	}
	for key, seq := range tl.Heads() {
		if got := loaded.thread(f.thread).Heads[headKey(key)]; got != seq {
			t.Fatalf("entry %s not durably consumed: head=%d want=%d", e.ID, got, seq)
		}
	}
}
