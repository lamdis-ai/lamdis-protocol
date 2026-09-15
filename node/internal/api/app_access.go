package api

// Who can see a thread, and how that gets decided.
//
// Three ways in, and they are different things:
//
//   - A grant names a principal — somebody you have paired with, or an agent
//     with its own key — and is folded from the signed control lane. Their
//     node can verify it. This is the protocol's own notion of sharing.
//   - A link is a bearer capability minted by this node for somebody with no
//     identity. It is recorded here so it can be listed and revoked; without
//     the record, a link is fire-and-forget and "who can see this" has no
//     honest answer.
//   - A request is somebody asking. Discoverable threads advertise id and
//     title; a request lands in the inbox and becomes a grant or a denial.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/perm"
)

// peerRecord mirrors the CLI's peers.json entry so both surfaces read one file.
type peerRecord struct {
	URL       string `json:"url"`
	Principal string `json:"principal,omitempty"`
}

func (a *App) peersPath() string  { return filepath.Join(a.DataDir, "peers.json") }
func (a *App) sharesPath() string { return filepath.Join(a.DataDir, "shares.json") }
func (a *App) namePath() string   { return filepath.Join(a.DataDir, "name") }

func (a *App) loadPeers() map[string]peerRecord {
	out := map[string]peerRecord{}
	raw, err := os.ReadFile(a.peersPath())
	if err != nil {
		return out
	}
	json.Unmarshal(raw, &out)
	return out
}

func (a *App) savePeers(p map[string]peerRecord) error {
	raw, _ := json.MarshalIndent(p, "", "  ")
	return os.WriteFile(a.peersPath(), raw, 0o600)
}

// displayName resolves a principal to something a person would say.
func (a *App) displayName(principal string) string {
	if principal != "" && principal == a.AgentSelf {
		return "your agent"
	}
	if principal == a.Self {
		if raw, err := os.ReadFile(a.namePath()); err == nil && strings.TrimSpace(string(raw)) != "" {
			return strings.TrimSpace(string(raw))
		}
		return "you"
	}
	for name, p := range a.loadPeers() {
		if p.Principal == principal {
			return name
		}
	}
	return a.name(principal)
}

// --- share registry -----------------------------------------------------

type shareRecord struct {
	ID      string   `json:"id"`
	Thread  string   `json:"thread"`
	Lanes   []string `json:"lanes"`
	Exp     int64    `json:"exp"`
	Created int64    `json:"created"`
	Label   string   `json:"label,omitempty"`
	Revoked bool     `json:"revoked,omitempty"`
}

func (a *App) loadShares() []shareRecord {
	var out []shareRecord
	raw, err := os.ReadFile(a.sharesPath())
	if err != nil {
		return out
	}
	json.Unmarshal(raw, &out)
	return out
}

func (a *App) saveShares(s []shareRecord) error {
	raw, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(a.sharesPath(), raw, 0o600)
}

// shareRevoked is consulted by readShare: a valid signature on a link the
// owner has since withdrawn is still a withdrawn link.
func (a *App) shareRevoked(id string) bool {
	for _, s := range a.loadShares() {
		if s.ID == id {
			return s.Revoked
		}
	}
	// Unknown to the registry: minted before the registry existed, or the
	// file was lost. Honour the signature; the expiry still bounds it.
	return false
}

// --- me -----------------------------------------------------------------

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	peers := a.loadPeers()
	type peerOut struct {
		Name      string `json:"name"`
		URL       string `json:"url"`
		Principal string `json:"principal"`
	}
	list := make([]peerOut, 0, len(peers))
	for n, p := range peers {
		list = append(list, peerOut{Name: n, URL: p.URL, Principal: p.Principal})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	pending := 0
	if ids, err := a.Store.Threads(r.Context()); err == nil {
		for _, id := range ids {
			if tl, err := a.Store.Thread(r.Context(), id); err == nil {
				pending += len(perm.Fold(id, tl.Entries()).PendingRequests())
			}
		}
	}
	writeJSON(w, map[string]any{
		"principal": a.Self,
		"name":      a.displayName(a.Self),
		"peers":     list,
		"pending":   pending,
		"model":     a.Model,
		"can_ask":   a.Runner != nil && a.Runner.Ready() && !a.agentRevoked,
		"agent":     a.AgentSelf,
	})
}

func (a *App) handleSetName(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in) != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(in.Name)
	if len(name) > 80 {
		name = name[:80]
	}
	if err := os.WriteFile(a.namePath(), []byte(name+"\n"), 0o600); err != nil {
		http.Error(w, "could not save", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"name": a.displayName(a.Self)})
}

// --- threads --------------------------------------------------------------

func (a *App) handleCreateThread(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title        string `json:"title"`
		Discoverable bool   `json:"discoverable"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in) != nil || strings.TrimSpace(in.Title) == "" {
		http.Error(w, "a title is required", http.StatusBadRequest)
		return
	}
	l, genesis, err := protolog.NewThreadWith(a.Key, strings.TrimSpace(in.Title), in.Discoverable, nil)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := a.Store.AppendEntries(r.Context(), l.Entries()); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"id": genesis.ID, "title": strings.TrimSpace(in.Title)})
}

// --- access ---------------------------------------------------------------

type accessGrant struct {
	Principal string   `json:"principal"`
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	Expires   string   `json:"expires,omitempty"`
}

type accessLink struct {
	ID      string   `json:"id"`
	Lanes   []string `json:"lanes"`
	Expires string   `json:"expires"`
	Created string   `json:"created"`
	Label   string   `json:"label,omitempty"`
	Path    string   `json:"path"`
}

type accessRequest struct {
	Principal string   `json:"principal"`
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	Reason    string   `json:"reason"`
	At        string   `json:"at"`
}

func (a *App) handleAccess(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()
	tl, err := a.Store.Thread(ctx, id)
	if err != nil {
		http.Error(w, "no such thread", http.StatusNotFound)
		return
	}
	st := perm.Fold(id, tl.Entries())
	now := a.now()

	grants := []accessGrant{}
	seen := map[string]bool{}
	for _, e := range tl.Entries() {
		if e.Kind != protolog.KindGrant {
			continue
		}
		var b struct {
			Principal  string `json:"principal"`
			TTLSeconds int64  `json:"ttl_seconds"`
		}
		if json.Unmarshal(e.Body, &b) != nil || b.Principal == "" || seen[b.Principal] {
			continue
		}
		eff := st.EffectiveScopes(b.Principal, now)
		if len(eff) == 0 {
			continue
		}
		seen[b.Principal] = true
		scopes := make([]string, 0, len(eff))
		for s := range eff {
			scopes = append(scopes, string(s))
		}
		sort.Strings(scopes)
		g := accessGrant{Principal: b.Principal, Name: a.displayName(b.Principal), Scopes: scopes}
		if b.TTLSeconds > 0 {
			if ts, err := time.Parse(time.RFC3339, e.TS); err == nil {
				g.Expires = ts.Add(time.Duration(b.TTLSeconds) * time.Second).Format(time.RFC3339)
			}
		}
		grants = append(grants, g)
	}

	links := []accessLink{}
	for _, s := range a.loadShares() {
		if s.Thread != id || s.Revoked || (s.Exp > 0 && now.Unix() > s.Exp) {
			continue
		}
		tok, err := a.mintShare(shareClaim{ID: s.ID, Thread: s.Thread, Lanes: s.Lanes, Exp: s.Exp})
		if err != nil {
			continue
		}
		links = append(links, accessLink{
			ID: s.ID, Lanes: s.Lanes, Label: s.Label, Path: "/s/" + tok,
			Expires: time.Unix(s.Exp, 0).Format(time.RFC3339),
			Created: time.Unix(s.Created, 0).Format(time.RFC3339),
		})
	}

	reqs := []accessRequest{}
	for _, q := range st.PendingRequests() {
		reqs = append(reqs, accessRequest{Principal: q.Principal, Name: a.displayName(q.Principal),
			Scopes: q.Scopes, Reason: q.Reason, At: q.TS})
	}

	writeJSON(w, map[string]any{
		"thread": id, "title": st.Title, "discoverable": st.Discoverable,
		"grants": grants, "links": links, "requests": reqs,
	})
}

// writeControl appends one person-signed control entry. Grants, denials and
// revocations all go through here so they are indistinguishable from the ones
// the CLI or the portal would have written.
func (a *App) writeControl(ctx context.Context, thread, kind string, body map[string]any) error {
	tl, err := a.Store.Thread(ctx, thread)
	if err != nil {
		return err
	}
	author, err := protolog.NewAuthor(tl, a.Key)
	if err != nil {
		return err
	}
	e, err := author.Append(protolog.Draft{Kind: kind, Lane: protolog.LaneControl, Body: body})
	if err != nil {
		return err
	}
	return a.Store.AppendEntries(ctx, []*protolog.Entry{e})
}

var errUnknownPeer = errors.New("no peer by that name")

// resolvePrincipal accepts either a full principal id or a peer's name.
func (a *App) resolvePrincipal(s string) (string, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "ed25519:") {
		return s, nil
	}
	for name, p := range a.loadPeers() {
		if strings.EqualFold(name, s) && p.Principal != "" {
			return p.Principal, nil
		}
	}
	return "", errUnknownPeer
}

func (a *App) handleGrant(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in struct {
		To     string   `json:"to"` // principal id or peer name
		Scopes []string `json:"scopes"`
		Days   int      `json:"days"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in) != nil || in.To == "" {
		http.Error(w, "who to grant to is required", http.StatusBadRequest)
		return
	}
	principal, err := a.resolvePrincipal(in.To)
	if err != nil {
		http.Error(w, "pair with them first, or paste their full principal id", http.StatusBadRequest)
		return
	}
	if len(in.Scopes) == 0 {
		in.Scopes = []string{string(perm.ScopeSummary)}
	}
	for _, s := range in.Scopes {
		if !perm.ValidScope(perm.Scope(s)) {
			http.Error(w, "invalid scope "+s, http.StatusBadRequest)
			return
		}
	}
	body := map[string]any{"principal": principal, "scopes": in.Scopes}
	if in.Days > 0 {
		body["ttl_seconds"] = int64(in.Days) * 86400
	}
	if err := a.writeControl(r.Context(), id, protolog.KindGrant, body); err != nil {
		http.Error(w, "could not write the grant", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "principal": principal, "name": a.displayName(principal)})
}

func (a *App) handleRevoke(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in struct {
		Principal string `json:"principal"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in) != nil || in.Principal == "" {
		http.Error(w, "principal is required", http.StatusBadRequest)
		return
	}
	if err := a.writeControl(r.Context(), id, protolog.KindRevoke,
		map[string]any{"principal": in.Principal}); err != nil {
		http.Error(w, "could not write the revocation", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// handleDecide answers a pending request: approve with scopes, or deny.
func (a *App) handleDecide(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in struct {
		Principal string   `json:"principal"`
		Approve   bool     `json:"approve"`
		Scopes    []string `json:"scopes"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in) != nil || in.Principal == "" {
		http.Error(w, "principal is required", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if !in.Approve {
		if err := a.writeControl(ctx, id, protolog.KindDeny, map[string]any{"principal": in.Principal}); err != nil {
			http.Error(w, "could not write the denial", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
		return
	}
	tl, err := a.Store.Thread(ctx, id)
	if err != nil {
		http.Error(w, "no such thread", http.StatusNotFound)
		return
	}
	st := perm.Fold(id, tl.Entries())
	body := map[string]any{"principal": in.Principal}
	scopes := in.Scopes
	for _, q := range st.PendingRequests() {
		if q.Principal == in.Principal {
			if len(scopes) == 0 {
				scopes = q.Scopes
			}
			body["request"] = q.EntryID
		}
	}
	if len(scopes) == 0 {
		http.Error(w, "no scopes to grant", http.StatusBadRequest)
		return
	}
	for _, s := range scopes {
		if !perm.ValidScope(perm.Scope(s)) {
			http.Error(w, "invalid scope "+s, http.StatusBadRequest)
			return
		}
	}
	body["scopes"] = scopes
	if err := a.writeControl(ctx, id, protolog.KindGrant, body); err != nil {
		http.Error(w, "could not write the grant", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) handleRevokeLink(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID string `json:"id"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in) != nil || in.ID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	shares := a.loadShares()
	found := false
	for i := range shares {
		if shares[i].ID == in.ID {
			shares[i].Revoked = true
			found = true
		}
	}
	if !found {
		http.Error(w, "no such link", http.StatusNotFound)
		return
	}
	if err := a.saveShares(shares); err != nil {
		http.Error(w, "could not save", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// --- peers ----------------------------------------------------------------

func (a *App) handleAddPeer(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in) != nil ||
		strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.URL) == "" {
		http.Error(w, "a name and their node's URL are required", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(in.Name)
	url := strings.TrimRight(strings.TrimSpace(in.URL), "/")
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}
	peers := a.loadPeers()
	rec := peerRecord{URL: url}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	principal, err := FetchNodeInfo(ctx, url)
	reached := err == nil
	if reached {
		rec.Principal = principal
	}
	peers[name] = rec
	if err := a.savePeers(peers); err != nil {
		http.Error(w, "could not save", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"name": name, "url": url, "principal": rec.Principal, "reached": reached,
		"note": map[bool]string{
			true:  "Paired. You can grant them access to any thread now.",
			false: "Saved, but their node could not be reached. Pair again when it is up, or grant by their full principal id.",
		}[reached],
	})
}
