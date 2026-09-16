package api

// Letting people in without asking who they are means most of them look
// once and leave. If every one of those keeps a slot forever, the door
// closes on the next person, which is exactly what happened here.
//
// So an account that was never used is not kept. "Never used" is a narrow
// thing on purpose: no identity attached, nothing written beyond the empty
// thread it was given, and untouched for a while. Anything somebody typed
// into, and anything with an email behind it, stays regardless of age.

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/agent"
	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
)

// untouchedFor is how long an empty visitor account is kept before the slot
// goes back. Long enough that somebody who wandered off mid-sentence and
// came back after lunch still finds their thread.
const untouchedFor = 24 * time.Hour

// abandoned reports whether an account may be reaped, and why not when it
// may not, which is what makes this safe to run automatically.
func (h *Host) abandoned(id string, now time.Time) (bool, string) {
	dir := filepath.Join(h.Root, id)
	if _, err := os.Stat(filepath.Join(dir, "subject")); err == nil {
		return false, "somebody signed in to it"
	}
	if _, err := os.Stat(filepath.Join(dir, "guest")); err != nil {
		return false, "it is not a visitor account"
	}
	// Anything the person set up counts as use, even with no threads. The
	// host writes agent.json itself at creation, so its existence proves
	// nothing; what is inside it does.
	for _, f := range []string{"peers.json", "shares.json", "name"} {
		if st, err := os.Stat(filepath.Join(dir, f)); err == nil && st.Size() > 2 {
			return false, "it was set up"
		}
	}
	if cfg, err := agent.LoadConfig(dir); err == nil {
		if len(cfg.Tools) > 0 || cfg.OpenRouterKey != "" || cfg.ModelURL != "" ||
			len(cfg.AllowDomains) > 0 || cfg.Brief != "" {
			return false, "it was set up"
		}
	}
	h.mu.Lock()
	acct := h.accounts[id]
	h.mu.Unlock()
	if acct == nil {
		return false, "it is not loaded"
	}
	ctx := context.Background()
	ids, err := acct.Store.Threads(ctx)
	if err != nil {
		return false, "its record could not be read"
	}
	newest := time.Time{}
	for _, t := range ids {
		tl, err := acct.Store.Thread(ctx, t)
		if err != nil {
			return false, "its record could not be read"
		}
		for _, e := range tl.Entries() {
			// The genesis of the thread it was handed does not count as use.
			if e.Kind == protolog.KindThread {
				continue
			}
			return false, "somebody wrote in it"
		}
		for _, e := range tl.Entries() {
			if ts, err := time.Parse(time.RFC3339, e.TS); err == nil && ts.After(newest) {
				newest = ts
			}
		}
	}
	if newest.IsZero() {
		if st, err := os.Stat(dir); err == nil {
			newest = st.ModTime()
		}
	}
	if now.Sub(newest) < untouchedFor {
		return false, "it is recent"
	}
	return true, ""
}

// reap frees the slots held by accounts nobody ever used. It returns how
// many it took back.
func (h *Host) reap(now time.Time) int {
	entries, err := os.ReadDir(h.Root)
	if err != nil {
		return 0
	}
	freed := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		ok, _ := h.abandoned(id, now)
		if !ok {
			continue
		}
		h.mu.Lock()
		acct := h.accounts[id]
		delete(h.accounts, id)
		h.mu.Unlock()
		if acct != nil && acct.Store != nil {
			acct.Store.Close()
		}
		if err := os.RemoveAll(filepath.Join(h.Root, id)); err != nil {
			h.logf("host: could not reap %s: %v", id, err)
			continue
		}
		freed++
	}
	if freed > 0 {
		h.logf("host: %d unused visitor accounts released", freed)
	}
	return freed
}

// Tidy runs the reaper on a schedule, and once at startup.
func (h *Host) Tidy(ctx context.Context) {
	h.reap(h.now())
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.reap(h.now())
		}
	}
}
