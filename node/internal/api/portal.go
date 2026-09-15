package api

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/perm"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/store"
)

// Portal is the owner's approval inbox, served by the node at /portal.
// It is authenticated by a local bearer token (never by peer signatures):
// only the person who runs this node can approve, and an approval becomes
// the same person-signed core.grant entry the CLI would write.
type Portal struct {
	Store store.Store
	Key   ed25519.PrivateKey
	Self  string
	Token string
	// ExposeApp mirrors App.ExposeApp: the approval inbox is yours alone.
	ExposeApp bool
	// Names maps principal -> friendly peer name for display.
	Names func(principal string) string
}

// NewPortalToken mints the local bearer token.
func NewPortalToken() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func (p *Portal) name(principal string) string {
	if p.Names != nil {
		if n := p.Names(principal); n != "" {
			return n
		}
	}
	if len(principal) > 20 {
		return principal[:20] + "…"
	}
	return principal
}

func (p *Portal) authed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !p.ExposeApp && !fromThisMachine(r) {
			http.Error(w, "this interface answers only on this machine", http.StatusForbidden)
			return
		}
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if tok == "" || subtle.ConstantTimeCompare([]byte(tok), []byte(p.Token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// Register mounts the portal onto mux.
func (p *Portal) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /portal", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(portalHTML))
	})
	mux.HandleFunc("GET /portal/api/state", p.authed(p.handleState))
	mux.HandleFunc("POST /portal/api/decide", p.authed(p.handleDecide))
}

type portalGrant struct {
	Principal string   `json:"principal"`
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
}

type portalRequest struct {
	Principal string   `json:"principal"`
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	Reason    string   `json:"reason"`
	TS        string   `json:"ts"`
}

type portalThread struct {
	ID           string          `json:"id"`
	Title        string          `json:"title"`
	Discoverable bool            `json:"discoverable"`
	Mine         bool            `json:"mine"`
	Grants       []portalGrant   `json:"grants"`
	Pending      []portalRequest `json:"pending"`
}

func (p *Portal) handleState(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ids, err := p.Store.Threads(ctx)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	var threads []portalThread
	for _, id := range ids {
		tl, err := p.Store.Thread(ctx, id)
		if err != nil {
			continue
		}
		st := perm.Fold(id, tl.Entries())
		pt := portalThread{ID: id, Title: st.Title, Discoverable: st.Discoverable, Mine: st.Stewards[p.Self]}
		for _, req := range st.PendingRequests() {
			pt.Pending = append(pt.Pending, portalRequest{
				Principal: req.Principal, Name: p.name(req.Principal),
				Scopes: req.Scopes, Reason: req.Reason, TS: req.TS,
			})
		}
		for _, e := range tl.Entries() {
			if e.Kind != protolog.KindGrant {
				continue
			}
			var body struct {
				Principal string `json:"principal"`
			}
			if json.Unmarshal(e.Body, &body) != nil || body.Principal == "" {
				continue
			}
			eff := st.EffectiveScopes(body.Principal, time.Now())
			if len(eff) == 0 {
				continue
			}
			var scopes []string
			for sc := range eff {
				scopes = append(scopes, string(sc))
			}
			dup := false
			for _, g := range pt.Grants {
				if g.Principal == body.Principal {
					dup = true
				}
			}
			if !dup {
				pt.Grants = append(pt.Grants, portalGrant{Principal: body.Principal, Name: p.name(body.Principal), Scopes: scopes})
			}
		}
		threads = append(threads, pt)
	}
	writeJSON(w, map[string]any{"self": p.Self, "threads": threads})
}

// handleDecide writes the person-signed answer: approve (grant), deny, or revoke.
func (p *Portal) handleDecide(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Thread    string   `json:"thread"`
		Principal string   `json:"principal"`
		Decision  string   `json:"decision"` // approve | deny | revoke
		Scopes    []string `json:"scopes,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Thread == "" || req.Principal == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	tl, err := p.Store.Thread(ctx, req.Thread)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	st := perm.Fold(req.Thread, tl.Entries())
	var kind string
	body := map[string]any{"principal": req.Principal}
	switch req.Decision {
	case "approve":
		kind = protolog.KindGrant
		scopes := req.Scopes
		var requestID string
		for _, pr := range st.PendingRequests() {
			if pr.Principal == req.Principal {
				if len(scopes) == 0 {
					scopes = pr.Scopes
				}
				requestID = pr.EntryID
			}
		}
		if len(scopes) == 0 {
			http.Error(w, "no pending request and no scopes given", http.StatusBadRequest)
			return
		}
		for _, sc := range scopes {
			if !perm.ValidScope(perm.Scope(sc)) {
				http.Error(w, "invalid scope", http.StatusBadRequest)
				return
			}
		}
		body["scopes"] = scopes
		if requestID != "" {
			body["request"] = requestID
		}
	case "deny":
		kind = protolog.KindDeny
	case "revoke":
		kind = protolog.KindRevoke
	default:
		http.Error(w, "bad decision", http.StatusBadRequest)
		return
	}
	author, err := protolog.NewAuthor(tl, p.Key)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	e, err := author.Append(protolog.Draft{Kind: kind, Lane: protolog.LaneControl, Body: body})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := p.Store.AppendEntries(ctx, []*protolog.Entry{e}); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "entry": e.ID})
}
