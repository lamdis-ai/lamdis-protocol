package api

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/perm"
)

// People on one host: find someone by @handle, invite them into a channel,
// and they join it with one press.
//
// What an invitation does is exactly what pairing two nodes has always
// done, just without anybody typing addresses: the inviter's node grants the
// invitee's key read and contribute on that one channel, and on accepting,
// the invitee's node adds the inviter as a peer and syncs. Access is still a
// signed grant to a key, checked by the inviter's node on every sync, and
// revocable there. A handle is only a way to find a key; it grants nothing.
//
// Somebody not here yet gets a join link instead: single use, a week long,
// for one channel. Opening it starts them an account if they have none.

var handleRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._]{1,22}[a-z0-9]$`)

type invite struct {
	ID       string    `json:"id"`
	FromAcct string    `json:"from_account"`
	FromName string    `json:"from_name"`
	FromTag  string    `json:"from_handle"`
	Thread   string    `json:"thread"`
	Title    string    `json:"title"`
	Created  time.Time `json:"created"`
}

type joinLink struct {
	FromAcct string    `json:"from_account"`
	Thread   string    `json:"thread"`
	Expires  time.Time `json:"expires"`
}

var peopleMu sync.Mutex

func (h *Host) handlesDir() string { return filepath.Join(h.Root, ".handles") }
func (h *Host) joinsPath() string  { return filepath.Join(h.Root, ".joins.json") }

// handleOf returns an account's handle, giving it one the first time.
func (h *Host) handleOf(a *Account) string {
	peopleMu.Lock()
	defer peopleMu.Unlock()
	if raw, err := os.ReadFile(filepath.Join(a.Dir, "handle")); err == nil && len(raw) > 0 {
		return strings.TrimSpace(string(raw))
	}
	base := "user"
	if raw, err := os.ReadFile(filepath.Join(a.Dir, "name")); err == nil {
		b := strings.ToLower(strings.TrimSpace(string(raw)))
		b = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(b, ".")
		b = strings.Trim(b, "._")
		if len(b) >= 3 {
			base = b
		}
	}
	if len(base) > 18 {
		base = base[:18]
	}
	for i := 0; i < 50; i++ {
		h := base
		if i > 0 || base == "user" {
			h = base + "." + strings.ToLower(randCode(3))
		}
		if hs := h; handleRe.MatchString(hs) {
			if err := claimHandle(filepath.Join(a.Dir, ".."), hs, a); err == nil {
				return hs
			}
		}
	}
	return ""
}

func claimHandle(root, handle string, a *Account) error {
	dir := filepath.Join(root, ".handles")
	os.MkdirAll(dir, 0o700)
	f, err := os.OpenFile(filepath.Join(dir, handle), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	f.WriteString(a.ID)
	f.Close()
	return os.WriteFile(filepath.Join(a.Dir, "handle"), []byte(handle), 0o600)
}

func randCode(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return strings.TrimRight(base32.StdEncoding.EncodeToString(b), "=")
}

func (h *Host) accountByHandle(handle string) *Account {
	raw, err := os.ReadFile(filepath.Join(h.handlesDir(), handle))
	if err != nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.accounts[strings.TrimSpace(string(raw))]
}

func (h *Host) accountByID(id string) *Account {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.accounts[id]
}

func displayOf(a *Account) string {
	if raw, err := os.ReadFile(filepath.Join(a.Dir, "name")); err == nil && strings.TrimSpace(string(raw)) != "" {
		return strings.TrimSpace(string(raw))
	}
	return ""
}

// GET /app/api/handle -> your handle; POST {handle} changes it.
func (h *Host) handleHandle(w http.ResponseWriter, r *http.Request) {
	me, err := h.resolve(r)
	if err != nil {
		writeStatusJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	if r.Method == "GET" {
		writeJSON(w, map[string]any{"handle": h.handleOf(me)})
		return
	}
	var in struct {
		Handle string `json:"handle"`
	}
	json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&in)
	want := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(in.Handle), "@"))
	if !handleRe.MatchString(want) {
		writeJSON(w, map[string]any{"error": "3 to 24 letters, numbers, dots or underscores, starting and ending with a letter or number."})
		return
	}
	old := h.handleOf(me)
	if want == old {
		writeJSON(w, map[string]any{"handle": old})
		return
	}
	peopleMu.Lock()
	err = claimHandle(h.Root, want, me)
	if err == nil && old != "" {
		os.Remove(filepath.Join(h.handlesDir(), old))
	}
	peopleMu.Unlock()
	if err != nil {
		writeJSON(w, map[string]any{"error": "@" + want + " is taken."})
		return
	}
	writeJSON(w, map[string]any{"handle": want})
}

// GET /app/api/people?q= finds people on this host by handle.
func (h *Host) handlePeople(w http.ResponseWriter, r *http.Request) {
	me, err := h.resolve(r)
	if err != nil {
		writeStatusJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	raw := strings.TrimSpace(r.URL.Query().Get("q"))
	out := []map[string]string{}
	if e, ok := normEmail(raw); ok && !strings.HasPrefix(raw, "@") {
		if a := h.accountByEmail(e); a != nil && a.ID != me.ID {
			out = append(out, map[string]string{"handle": h.handleOf(a), "name": displayOf(a), "email": e})
		}
		writeJSON(w, map[string]any{"people": out, "email": e, "can_email": h.Mail != nil})
		return
	}
	q := strings.ToLower(strings.TrimPrefix(raw, "@"))
	if len(q) < 2 {
		writeJSON(w, map[string]any{"people": out})
		return
	}
	ents, _ := os.ReadDir(h.handlesDir())
	var names []string
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), q) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names {
		a := h.accountByHandle(n)
		if a == nil || a.ID == me.ID {
			continue
		}
		out = append(out, map[string]string{"handle": n, "name": displayOf(a)})
		if len(out) == 8 {
			break
		}
	}
	writeJSON(w, map[string]any{"people": out})
}

// POST /app/api/invite {thread, handle} grants that person the channel and
// puts an invitation in front of them.
func (h *Host) handleInvite(w http.ResponseWriter, r *http.Request) {
	me, err := h.resolve(r)
	if err != nil {
		writeStatusJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	var in struct {
		Thread string `json:"thread"`
		Handle string `json:"handle"`
	}
	json.NewDecoder(io.LimitReader(r.Body, 1<<12)).Decode(&in)
	if e, ok := normEmail(in.Handle); ok && strings.Contains(in.Handle, "@") && !strings.HasPrefix(strings.TrimSpace(in.Handle), "@") {
		title, err := h.mayInvite(r.Context(), me, in.Thread)
		if err != nil {
			writeJSON(w, map[string]any{"error": err.Error()})
			return
		}
		h.inviteByEmail(w, r, me, in.Thread, title, e)
		return
	}
	them := h.accountByHandle(strings.ToLower(strings.TrimPrefix(strings.TrimSpace(in.Handle), "@")))
	if them == nil {
		writeJSON(w, map[string]any{"error": "Nobody here is called @" + strings.TrimPrefix(in.Handle, "@") + ". Send them an invite link instead."})
		return
	}
	if them.ID == me.ID {
		writeJSON(w, map[string]any{"error": "That's you."})
		return
	}
	title, err := h.mayInvite(r.Context(), me, in.Thread)
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	if err := me.grant(r.Context(), in.Thread, them.Self); err != nil {
		writeJSON(w, map[string]any{"error": "could not grant access: " + err.Error()})
		return
	}
	h.knowEachOther(me, them)
	inv := invite{ID: strings.ToLower(randCode(8)), FromAcct: me.ID, FromName: displayOf(me), FromTag: h.handleOf(me),
		Thread: in.Thread, Title: title, Created: h.now()}
	peopleMu.Lock()
	list := readInvites(them.Dir)
	list = append(list, inv)
	writeInvites(them.Dir, list)
	peopleMu.Unlock()
	writeJSON(w, map[string]any{"ok": true, "handle": h.handleOf(them)})
}

// mayInvite checks the caller owns the channel, and returns its title.
func (h *Host) mayInvite(ctx context.Context, me *Account, thread string) (string, error) {
	tl, err := me.Store.Thread(ctx, thread)
	if err != nil {
		return "", errText("no such channel")
	}
	st := perm.Fold(thread, tl.Entries())
	if !st.Stewards[me.Self] {
		return "", errText("only the channel's owner can bring people in")
	}
	return st.Title, nil
}

// knowEachOther records each side as a peer of the other, under their
// handle, so names read as people and either can invite the other later.
func (h *Host) knowEachOther(a, b *Account) {
	base := strings.TrimRight(h.PublicBase, "/")
	add := func(to, who *Account) {
		peopleMu.Lock()
		defer peopleMu.Unlock()
		peers, _ := loadPeersIn(to.Dir)
		name := h.handleOfLocked(who)
		for n, p := range peers {
			if p.Principal == who.Self && n != name {
				delete(peers, n)
			}
		}
		peers[name] = peerRecord{URL: base + "/n/" + who.ID, Principal: who.Self}
		raw, _ := json.MarshalIndent(peers, "", "  ")
		os.WriteFile(filepath.Join(to.Dir, "peers.json"), raw, 0o600)
	}
	h.handleOf(a)
	h.handleOf(b)
	add(a, b)
	add(b, a)
}

func (h *Host) handleOfLocked(a *Account) string {
	if raw, err := os.ReadFile(filepath.Join(a.Dir, "handle")); err == nil && len(raw) > 0 {
		return "@" + strings.TrimSpace(string(raw))
	}
	return a.ID
}

func readInvites(dir string) []invite {
	var out []invite
	raw, err := os.ReadFile(filepath.Join(dir, "invites.json"))
	if err == nil {
		json.Unmarshal(raw, &out)
	}
	return out
}

func writeInvites(dir string, list []invite) {
	raw, _ := json.MarshalIndent(list, "", "  ")
	os.WriteFile(filepath.Join(dir, "invites.json"), raw, 0o600)
}

// GET /app/api/invites lists what is waiting for you.
func (h *Host) handleInvites(w http.ResponseWriter, r *http.Request) {
	me, err := h.resolve(r)
	if err != nil {
		writeStatusJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	list := readInvites(me.Dir)
	if list == nil {
		list = []invite{}
	}
	writeJSON(w, map[string]any{"invites": list, "handle": h.handleOf(me)})
}

// POST /app/api/invites/answer {id, accept} joins or declines.
func (h *Host) handleInviteAnswer(w http.ResponseWriter, r *http.Request) {
	me, err := h.resolve(r)
	if err != nil {
		writeStatusJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	var in struct {
		ID     string `json:"id"`
		Accept bool   `json:"accept"`
	}
	json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&in)
	peopleMu.Lock()
	list := readInvites(me.Dir)
	var inv *invite
	rest := list[:0]
	for i := range list {
		if list[i].ID == in.ID {
			cp := list[i]
			inv = &cp
			continue
		}
		rest = append(rest, list[i])
	}
	writeInvites(me.Dir, rest)
	peopleMu.Unlock()
	if inv == nil {
		writeJSON(w, map[string]any{"error": "that invitation is gone"})
		return
	}
	if !in.Accept {
		writeJSON(w, map[string]any{"ok": true})
		return
	}
	from := h.accountByID(inv.FromAcct)
	if from == nil {
		writeJSON(w, map[string]any{"error": "the person who invited you is no longer here"})
		return
	}
	h.join(r.Context(), me, from)
	writeJSON(w, map[string]any{"ok": true, "thread": inv.Thread})
}

// join makes the invitee's node fetch what it has been granted, now rather
// than on its next round.
func (h *Host) join(ctx context.Context, me, from *Account) {
	h.knowEachOther(me, from)
	if me.Sched != nil && me.Sched.Sync != nil {
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if err := me.Sched.Sync(ctx); err != nil {
			h.logf("host: join sync for %s: %v", me.ID, err)
		}
	}
}

// POST /app/api/joinlink {thread} makes a single-use link for somebody not
// here yet.
func (h *Host) handleJoinLink(w http.ResponseWriter, r *http.Request) {
	me, err := h.resolve(r)
	if err != nil {
		writeStatusJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	var in struct {
		Thread string `json:"thread"`
	}
	json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&in)
	if _, err := h.mayInvite(r.Context(), me, in.Thread); err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	url, err := h.newJoinLink(me, in.Thread)
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"url": url, "expires_in_days": 7})
}

func (h *Host) newJoinLink(me *Account, thread string) (string, error) {
	code := strings.ToLower(randCode(10))
	peopleMu.Lock()
	joins := map[string]joinLink{}
	if raw, err := os.ReadFile(h.joinsPath()); err == nil {
		json.Unmarshal(raw, &joins)
	}
	for k, j := range joins {
		if h.now().After(j.Expires) {
			delete(joins, k)
		}
	}
	joins[code] = joinLink{FromAcct: me.ID, Thread: thread, Expires: h.now().Add(7 * 24 * time.Hour)}
	raw, _ := json.Marshal(joins)
	err := os.WriteFile(h.joinsPath(), raw, 0o600)
	peopleMu.Unlock()
	return strings.TrimRight(h.PublicBase, "/") + "/app?join=" + code, err
}

// POST /app/api/join {code} redeems a link: grant, then join.
func (h *Host) handleJoin(w http.ResponseWriter, r *http.Request) {
	me, err := h.resolve(r)
	if err != nil {
		writeStatusJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	var in struct {
		Code string `json:"code"`
	}
	json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&in)
	peopleMu.Lock()
	joins := map[string]joinLink{}
	if raw, err := os.ReadFile(h.joinsPath()); err == nil {
		json.Unmarshal(raw, &joins)
	}
	j, ok := joins[strings.ToLower(strings.TrimSpace(in.Code))]
	if ok {
		delete(joins, strings.ToLower(strings.TrimSpace(in.Code))) // one link, one person
		raw, _ := json.Marshal(joins)
		os.WriteFile(h.joinsPath(), raw, 0o600)
	}
	peopleMu.Unlock()
	if !ok || h.now().After(j.Expires) {
		writeJSON(w, map[string]any{"error": "That invite link has been used or has expired. Ask for a new one."})
		return
	}
	from := h.accountByID(j.FromAcct)
	if from == nil || from.ID == me.ID {
		writeJSON(w, map[string]any{"error": "That link is yours, or its owner has left."})
		return
	}
	if err := from.grant(r.Context(), j.Thread, me.Self); err != nil {
		writeJSON(w, map[string]any{"error": "could not join: " + err.Error()})
		return
	}
	h.join(r.Context(), me, from)
	writeJSON(w, map[string]any{"ok": true, "thread": j.Thread})
}
