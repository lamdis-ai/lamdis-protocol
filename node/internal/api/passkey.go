package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// Passkeys: sign-in that the node does itself.
//
// No identity provider, no passwords, no email codes: an account is kept by
// a passkey on the person's own device (Face ID, Touch ID, a phone, a
// security key), and signing in anywhere else is that same passkey, synced
// by their password manager. Nothing phishable is ever typed, nothing
// reusable is stored here (only public keys), and any self-hosted node can
// offer the same thing, because it needs nothing but itself.
//
// A visitor who saves their account with a passkey becomes a kept account:
// the reaper leaves it alone and it may now hold credentials for connected
// services. Signing in on another device returns a session that lasts thirty
// days and names the account, signed like a visitor token.

type passkeyUser struct {
	id    string
	name  string
	creds []webauthn.Credential
}

func (u passkeyUser) WebAuthnID() []byte                         { return []byte(u.id) }
func (u passkeyUser) WebAuthnName() string                       { return u.name }
func (u passkeyUser) WebAuthnDisplayName() string                { return u.name }
func (u passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

var (
	passkeyMu       sync.Mutex
	passkeySessions = map[string]passkeySession{} // ceremony id -> session
)

type passkeySession struct {
	data    webauthn.SessionData
	account string // set for registration
	expires time.Time
}

// relyingParty is this host as passkeys see it: its public address, or the
// address the request came in on for a local host.
func (h *Host) relyingParty(r *http.Request) (*webauthn.WebAuthn, error) {
	origin := strings.TrimRight(h.PublicBase, "/")
	if origin == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		origin = scheme + "://" + r.Host
	}
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("this host does not know its own address")
	}
	return webauthn.New(&webauthn.Config{RPID: u.Hostname(), RPDisplayName: "Lamdis", RPOrigins: []string{u.Scheme + "://" + u.Host}})
}

func credsPath(dir string) string { return filepath.Join(dir, "passkeys.json") }

func readCreds(dir string) []webauthn.Credential {
	var out []webauthn.Credential
	if raw, err := os.ReadFile(credsPath(dir)); err == nil {
		json.Unmarshal(raw, &out)
	}
	return out
}

func (h *Host) passkeyIndex() string { return filepath.Join(h.Root, ".passkeys") }

func stash(s passkeySession) string {
	id := strings.ToLower(randCode(12))
	passkeyMu.Lock()
	defer passkeyMu.Unlock()
	for k, v := range passkeySessions {
		if time.Now().After(v.expires) {
			delete(passkeySessions, k)
		}
	}
	passkeySessions[id] = s
	return id
}

func unstash(id string) (passkeySession, bool) {
	passkeyMu.Lock()
	defer passkeyMu.Unlock()
	s, ok := passkeySessions[id]
	delete(passkeySessions, id)
	if !ok || time.Now().After(s.expires) {
		return s, false
	}
	return s, true
}

// POST /app/api/passkey/register/begin — for the account already in use.
func (h *Host) handlePasskeyRegisterBegin(w http.ResponseWriter, r *http.Request) {
	me, err := h.resolve(r)
	if err != nil {
		writeStatusJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	rp, err := h.relyingParty(r)
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	name := displayOf(me)
	if tag := h.handleOf(me); tag != "" {
		name = "@" + tag
	}
	if name == "" {
		name = "Lamdis account"
	}
	u := passkeyUser{id: me.ID, name: name, creds: readCreds(me.Dir)}
	var exclude []protocol.CredentialDescriptor
	for _, c := range u.creds {
		exclude = append(exclude, c.Descriptor())
	}
	opts, sess, err := rp.BeginRegistration(u,
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithExclusions(exclude))
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	id := stash(passkeySession{data: *sess, account: me.ID, expires: time.Now().Add(5 * time.Minute)})
	writeJSON(w, map[string]any{"ceremony": id, "options": opts})
}

// POST /app/api/passkey/register/finish?ceremony=
func (h *Host) handlePasskeyRegisterFinish(w http.ResponseWriter, r *http.Request) {
	me, err := h.resolve(r)
	if err != nil {
		writeStatusJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	s, ok := unstash(r.URL.Query().Get("ceremony"))
	if !ok || s.account != me.ID {
		writeJSON(w, map[string]any{"error": "that took too long; try again"})
		return
	}
	rp, err := h.relyingParty(r)
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	u := passkeyUser{id: me.ID, creds: readCreds(me.Dir)}
	cred, err := rp.FinishRegistration(u, s.data, r)
	if err != nil {
		writeJSON(w, map[string]any{"error": "the passkey was not accepted: " + err.Error()})
		return
	}
	peopleMu.Lock()
	creds := append(readCreds(me.Dir), *cred)
	raw, _ := json.Marshal(creds)
	os.WriteFile(credsPath(me.Dir), raw, 0o600)
	os.MkdirAll(h.passkeyIndex(), 0o700)
	os.WriteFile(filepath.Join(h.passkeyIndex(), base64.RawURLEncoding.EncodeToString(cred.ID)), []byte(me.ID), 0o600)
	// A kept account: never reaped, and trusted to hold connection secrets.
	if _, err := os.Stat(filepath.Join(me.Dir, "subject")); os.IsNotExist(err) {
		os.WriteFile(filepath.Join(me.Dir, "subject"), []byte("passkey"), 0o600)
	}
	peopleMu.Unlock()
	writeJSON(w, map[string]any{"ok": true, "passkeys": len(creds), "handle": h.handleOf(me)})
}

// POST /app/api/passkey/login/begin — public: whoever holds a passkey.
func (h *Host) handlePasskeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	rp, err := h.relyingParty(r)
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	opts, sess, err := rp.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationPreferred))
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	id := stash(passkeySession{data: *sess, expires: time.Now().Add(5 * time.Minute)})
	writeJSON(w, map[string]any{"ceremony": id, "options": opts})
}

// POST /app/api/passkey/login/finish?ceremony= — returns a session token.
func (h *Host) handlePasskeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	s, ok := unstash(r.URL.Query().Get("ceremony"))
	if !ok {
		writeJSON(w, map[string]any{"error": "that took too long; try again"})
		return
	}
	rp, err := h.relyingParty(r)
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	var found string
	user, cred, err := rp.FinishPasskeyLogin(func(rawID, userHandle []byte) (webauthn.User, error) {
		raw, err := os.ReadFile(filepath.Join(h.passkeyIndex(), base64.RawURLEncoding.EncodeToString(rawID)))
		if err != nil {
			return nil, fmt.Errorf("unknown passkey")
		}
		acct := strings.TrimSpace(string(raw))
		if acct != string(userHandle) {
			return nil, fmt.Errorf("passkey does not match its account")
		}
		found = acct
		return passkeyUser{id: acct, creds: readCreds(filepath.Join(h.Root, acct))}, nil
	}, s.data, r)
	if err != nil || user == nil || found == "" {
		msg := "that passkey is not for an account here"
		if err != nil && !strings.Contains(err.Error(), "unknown passkey") {
			msg = "sign-in failed: " + err.Error()
		}
		writeJSON(w, map[string]any{"error": msg})
		return
	}
	// Keep the sign counter current; a counter going backwards is a cloned
	// authenticator, which the library reports.
	peopleMu.Lock()
	creds := readCreds(filepath.Join(h.Root, found))
	for i := range creds {
		if string(creds[i].ID) == string(cred.ID) {
			creds[i].Authenticator = cred.Authenticator
		}
	}
	raw, _ := json.Marshal(creds)
	os.WriteFile(credsPath(filepath.Join(h.Root, found)), raw, 0o600)
	peopleMu.Unlock()
	if _, err := h.load(found, ""); err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	tok, err := h.mintSession(found, h.now().Add(30*24*time.Hour))
	if err != nil {
		writeJSON(w, map[string]any{"error": "could not sign in"})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "token": tok})
}

// GET /app/api/passkey — how this account is kept.
func (h *Host) handlePasskeyStatus(w http.ResponseWriter, r *http.Request) {
	me, err := h.resolve(r)
	if err != nil {
		writeStatusJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"passkeys": len(readCreds(me.Dir)), "handle": h.handleOf(me)})
}

// mintSession signs "s.<account>.<expiry>.<mac>".
func (h *Host) mintSession(id string, exp time.Time) (string, error) {
	secret, err := h.guestSecret()
	if err != nil {
		return "", err
	}
	e := strconv.FormatInt(exp.Unix(), 10)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("session\x00" + id + "\x00" + e))
	return "s." + id + "." + e + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (h *Host) readSession(tok string) (string, bool) {
	p := strings.Split(tok, ".")
	if len(p) != 4 || p[0] != "s" {
		return "", false
	}
	exp, err := strconv.ParseInt(p[2], 10, 64)
	if err != nil || h.now().Unix() > exp {
		return "", false
	}
	secret, err := h.guestSecret()
	if err != nil {
		return "", false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("session\x00" + p[1] + "\x00" + p[2]))
	if !hmac.Equal([]byte(base64.RawURLEncoding.EncodeToString(mac.Sum(nil))), []byte(p[3])) {
		return "", false
	}
	return p[1], true
}
