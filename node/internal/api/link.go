package api

// Connecting a machine to an account.
//
// The hard part of "write from your phone, have it happen on your laptop"
// is not the writing or the happening; both already work. It is that the
// laptop and the account have to agree they are the same person, without
// anybody copying a key around.
//
// So: the account asks for a code, the laptop redeems it. Redeeming proves
// the laptop holds a keypair, and the account grants that key access to one
// thread. After that the two nodes sync the way any two nodes do, and every
// rule about who may read what is the one the protocol already enforces.
//
// The code is short-lived, single-use, and worth exactly one grant on one
// thread. Losing it costs you that thread and nothing else.

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/perm"
)

type linkOffer struct {
	account string
	dataDir string
	thread  string
	syncURL string
	expires time.Time
}

var linkOffers sync.Map // code -> linkOffer

// newCode is short enough to type and long enough not to guess.
func newCode() string {
	b := make([]byte, 10)
	rand.Read(b)
	c := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
	return c[:4] + "-" + c[4:8] + "-" + c[8:12] + "-" + c[12:16]
}

// handleLinkOffer hands out a code for one thread.
func (a *App) handleLinkOffer(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Thread string `json:"thread"`
	}
	json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in)
	thread := strings.TrimSpace(in.Thread)
	if thread == "" {
		writeJSON(w, map[string]any{"error": "pick a thread to share with the machine"})
		return
	}
	tl, err := a.Store.Thread(r.Context(), thread)
	if err != nil {
		http.Error(w, "no such thread", http.StatusNotFound)
		return
	}
	if !perm.Fold(thread, tl.Entries()).Stewards[a.Self] {
		writeJSON(w, map[string]any{"error": "only the owner of a thread can connect a machine to it"})
		return
	}
	if a.SyncBase == "" {
		writeJSON(w, map[string]any{"error": "this node does not know its own address, so it cannot be synced with"})
		return
	}
	code := newCode()
	linkOffers.Store(code, linkOffer{
		account: a.Self, dataDir: a.DataDir, thread: thread,
		syncURL: a.SyncBase, expires: a.now().Add(15 * time.Minute),
	})
	writeJSON(w, map[string]any{"code": code, "command": "lamdis link " + code,
		"expires_in": 900, "thread": thread})
}

// LinkHandler is the public half: a machine redeems a code here. It is not
// behind the owner check, because the machine redeeming it has no session,
// only the code and a keypair it proves it holds.
func (h *Host) LinkHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	// The machine signs its request, which is how we know the principal it
	// names is one it actually holds.
	principal, err := authenticate(r, body, h.now())
	if err != nil {
		writeStatusJSON(w, http.StatusUnauthorized, map[string]any{"error": "sign the request with your node key"})
		return
	}
	var in struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if json.Unmarshal(body, &in) != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	code := strings.ToLower(strings.TrimSpace(in.Code))
	v, ok := linkOffers.Load(code)
	if !ok {
		writeStatusJSON(w, http.StatusNotFound, map[string]any{"error": "that code is not one we are waiting for. Ask for a new one."})
		return
	}
	off := v.(linkOffer)
	if h.now().After(off.expires) {
		linkOffers.Delete(code)
		writeStatusJSON(w, http.StatusGone, map[string]any{"error": "that code has expired. Ask for a new one."})
		return
	}
	// One code, one machine.
	linkOffers.Delete(code)

	h.mu.Lock()
	var acct *Account
	for _, a := range h.accounts {
		if a.Self == off.account {
			acct = a
		}
	}
	h.mu.Unlock()
	if acct == nil {
		writeStatusJSON(w, http.StatusNotFound, map[string]any{"error": "that account is no longer here"})
		return
	}

	// Grant the machine's key what it needs to work in that one thread.
	if err := acct.grant(r.Context(), off.thread, principal); err != nil {
		writeStatusJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{
		"ok": true, "thread": off.thread, "principal": acct.Self,
		"sync_url": strings.TrimRight(off.syncURL, "/") + "/n/" + acct.ID,
		"name":     firstNonEmpty(acct.Email, "lamdis"),
	})
}

// grant writes the control entry that lets another key work in one thread.
func (a *Account) grant(ctx context.Context, thread, principal string) error {
	tl, err := a.Store.Thread(ctx, thread)
	if err != nil {
		return err
	}
	author, err := protolog.NewAuthor(tl, a.Key)
	if err != nil {
		return err
	}
	e, err := author.Append(protolog.Draft{Kind: protolog.KindGrant, Lane: protolog.LaneControl,
		Body: map[string]any{"principal": principal, "scopes": []string{"read", "contribute"}}})
	if err != nil {
		return err
	}
	return a.Store.AppendEntries(ctx, []*protolog.Entry{e})
}
