package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Watcher scans forward through confirmed blocks and hands each new transfer
// to a handler exactly once.
//
// Two things are persisted, and both for the same reason: a restart must not
// credit a transfer twice, and must not skip one. The last block fully
// handled says where to resume; the set of transfers already handled catches
// the case where the process died between funding a job and recording the
// block.
type Watcher struct {
	Client        *Client
	Confirmations uint64

	mu    sync.Mutex
	path  string
	state watchState
}

type watchState struct {
	LastBlock uint64            `json:"last_block"`
	Seen      map[string]string `json:"seen"`
	Unmatched []Transfer        `json:"unmatched,omitempty"`
}

// NewWatcher loads the scan position from dir, or starts at the head.
func NewWatcher(c *Client, dir string, confirmations uint64) *Watcher {
	w := &Watcher{Client: c, Confirmations: confirmations, state: watchState{Seen: map[string]string{}}}
	if dir != "" {
		w.path = filepath.Join(dir, "usdc-watch.json")
		if b, err := os.ReadFile(w.path); err == nil {
			_ = json.Unmarshal(b, &w.state)
			if w.state.Seen == nil {
				w.state.Seen = map[string]string{}
			}
		}
	}
	return w
}

// Handler is asked what to do with a transfer. Return matched=true to record
// it against a label (a job id) so it is never offered again; false leaves it
// in the unmatched list for a person. An error stops the scan before the
// block is recorded, so the transfer is offered again next time.
type Handler func(ctx context.Context, t Transfer) (label string, matched bool, err error)

// Scan reads every confirmed block since the last one and offers each new
// transfer to handle. Returns how many were matched.
func (w *Watcher) Scan(ctx context.Context, handle Handler) (int, error) {
	head, err := w.Client.BlockNumber(ctx)
	if err != nil {
		return 0, err
	}
	if head < w.Confirmations {
		return 0, nil
	}
	safe := head - w.Confirmations
	w.mu.Lock()
	last := w.state.LastBlock
	w.mu.Unlock()
	if last == 0 {
		// First run: nothing before now is ours to interpret. A job quoted
		// before the watcher existed cannot have been quoted at all.
		w.mu.Lock()
		w.state.LastBlock = safe
		err := w.saveLocked()
		w.mu.Unlock()
		return 0, err
	}
	if safe <= last {
		return 0, nil
	}
	transfers, err := w.Client.Transfers(ctx, last+1, safe)
	if err != nil {
		return 0, err
	}
	matched := 0
	for _, t := range transfers {
		if _, done := w.Seen(t.Key()); done {
			continue
		}
		label, ok, err := handle(ctx, t)
		if err != nil {
			return matched, fmt.Errorf("chain: handling %s: %w", t.Key(), err)
		}
		w.mu.Lock()
		if ok {
			w.state.Seen[t.Key()] = label
			matched++
		} else {
			w.state.Unmatched = append(w.state.Unmatched, t)
			w.state.Seen[t.Key()] = ""
		}
		err = w.saveLocked()
		w.mu.Unlock()
		if err != nil {
			return matched, err
		}
	}
	w.mu.Lock()
	w.state.LastBlock = safe
	err = w.saveLocked()
	w.mu.Unlock()
	return matched, err
}

// Seen reports whether a transfer has been handled, and its label.
func (w *Watcher) Seen(key string) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	l, ok := w.state.Seen[key]
	return l, ok
}

// LastBlock is the newest block whose transfers have all been handled.
func (w *Watcher) LastBlock() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.state.LastBlock
}

// Unmatched lists transfers that arrived for no job: money a person has to
// send back by hand.
func (w *Watcher) Unmatched() []Transfer {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]Transfer(nil), w.state.Unmatched...)
}

func (w *Watcher) saveLocked() error {
	if w.path == "" {
		return nil
	}
	b, err := json.MarshalIndent(w.state, "", "  ")
	if err != nil {
		return err
	}
	tmp := w.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, w.path)
}
