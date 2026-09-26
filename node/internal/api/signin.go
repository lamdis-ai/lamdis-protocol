package api

import (
	"crypto/rand"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// One person, every device.
//
// Somebody starts on a laptop as a guest, gets an invite link on their
// phone, and should land in the same account, not a stranger's. Two ways
// in, both without a password:
//
//   - Email. An address confirmed on an account signs into it: we send a
//     code, they type it back. An address nobody here has confirmed is
//     attached to the account they are already using, which is how a guest
//     becomes somebody who can come back.
//   - Another device. A signed-in device shows a short code (and a link that
//     carries it); typing it on the new device signs that device in. Single
//     use, ten minutes.
//
// Passkeys remain the third way and work as before.

var (
	signinMu    sync.Mutex
	signinCodes = map[string]pendingEmail{} // by address: signing into the account that owns it
	deviceCodes = map[string]deviceCode{}   // by code
)

type deviceCode struct {
	acct    string
	expires time.Time
}

// GET /app/api/whoami — who this device is signed in as, and whether the
// account could be reached again from another device.
func (h *Host) handleWhoami(w http.ResponseWriter, r *http.Request) {
	me, err := h.resolve(r)
	if err != nil {
		writeStatusJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	raw, _ := os.ReadFile(filepath.Join(me.Dir, "email_confirmed"))
	email := strings.TrimSpace(string(raw))
	pk := len(readCreds(me.Dir))
	writeJSON(w, map[string]any{
		"handle": h.handleOf(me), "name": displayOf(me), "email": email, "passkeys": pk,
		// Kept: there is a way back into this account from somewhere else.
		"kept": email != "" || pk > 0,
	})
}

// POST /app/api/signin/email {email} sends a code. For an address that is
// already somebody's, the code signs into that account; otherwise it will
// attach the address to the account this device is using.
func (h *Host) handleSigninEmail(w http.ResponseWriter, r *http.Request) {
	if h.Mail == nil {
		writeJSON(w, map[string]any{"error": "This host cannot send email yet."})
		return
	}
	var in struct {
		Email string `json:"email"`
	}
	json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&in)
	e, ok := normEmail(in.Email)
	if !ok {
		writeJSON(w, map[string]any{"error": "That does not look like an email address."})
		return
	}
	owner := h.accountByEmail(e)
	me, _ := h.resolve(r)
	if owner == nil {
		if me == nil {
			writeJSON(w, map[string]any{"error": "start first"})
			return
		}
		// Nobody has this address yet: confirm it onto this account.
		h.sendConfirm(w, r, me, e)
		return
	}
	if me != nil && me.ID == owner.ID {
		writeJSON(w, map[string]any{"ok": true, "already": true, "email": e})
		return
	}
	// Per address, so nobody can use this to fill somebody's inbox.
	if !allowMail("signin:"+e, 6) {
		writeJSON(w, map[string]any{"error": "Too many codes for that address today. Try again tomorrow, or use a passkey."})
		return
	}
	code := sixDigits()
	signinMu.Lock()
	signinCodes[e] = pendingEmail{email: e, code: code, expires: time.Now().Add(15 * time.Minute)}
	signinMu.Unlock()
	body := "Your Lamdis sign-in code is " + code + ".\n\nType it on the device you are signing in on. It works for 15 minutes. If you did not ask for it, ignore this email; nobody gets in without the code.\n\nLamdis"
	if err := h.Mail.Send(r.Context(), e, "Your Lamdis sign-in code: "+code, body); err != nil {
		h.logf("host: mail sign-in: %v", err)
		writeJSON(w, map[string]any{"error": "Could not send the email just now."})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "sent_to": e, "existing": true})
}

// sendConfirm mails the code that attaches an address to me.
func (h *Host) sendConfirm(w http.ResponseWriter, r *http.Request, me *Account, e string) {
	if !allowMail(me.ID, 10) {
		writeJSON(w, map[string]any{"error": "Too many emails today. Try again tomorrow."})
		return
	}
	code := sixDigits()
	emailMu.Lock()
	emailCodes[me.ID] = pendingEmail{email: e, code: code, expires: time.Now().Add(15 * time.Minute)}
	emailMu.Unlock()
	body := "Your Lamdis confirmation code is " + code + ".\n\nIt works for 15 minutes. If you did not ask for it, ignore this email; nothing changes without the code.\n\nLamdis"
	if err := h.Mail.Send(r.Context(), e, "Your Lamdis code: "+code, body); err != nil {
		h.logf("host: mail to confirm: %v", err)
		writeJSON(w, map[string]any{"error": "Could not send the email just now."})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "sent_to": e, "existing": false})
}

// POST /app/api/signin/email/confirm {email, code} — a sign-in code returns
// a session for the account that owns the address; a confirmation code
// attaches the address to this one.
func (h *Host) handleSigninEmailConfirm(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<10))
	json.Unmarshal(raw, &in)
	e, _ := normEmail(in.Email)
	signinMu.Lock()
	p, ok := signinCodes[e]
	if ok {
		p.tries++
		signinCodes[e] = p
	}
	signinMu.Unlock()
	if !ok {
		// Not a sign-in: this is the address being attached here.
		r.Body = io.NopCloser(strings.NewReader(string(raw)))
		h.handleEmailConfirm(w, r)
		return
	}
	if time.Now().After(p.expires) || p.tries > 5 {
		writeJSON(w, map[string]any{"error": "That code has expired. Send a new one."})
		return
	}
	if strings.TrimSpace(in.Code) != p.code {
		writeJSON(w, map[string]any{"error": "That code is not right."})
		return
	}
	signinMu.Lock()
	delete(signinCodes, e)
	signinMu.Unlock()
	owner := h.accountByEmail(e)
	if owner == nil {
		writeJSON(w, map[string]any{"error": "That account is no longer here."})
		return
	}
	h.signedIn(w, owner.ID)
}

// signedIn answers with a thirty-day session for acct.
func (h *Host) signedIn(w http.ResponseWriter, acct string) {
	a, err := h.load(acct, "")
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	tok, err := h.mintSession(acct, h.now().Add(30*24*time.Hour))
	if err != nil {
		writeJSON(w, map[string]any{"error": "could not sign in"})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "token": tok, "handle": h.handleOf(a)})
}

// deviceAlphabet leaves out letters that read as digits and vice versa.
const deviceAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

func newDeviceCode() string {
	b := make([]byte, 8)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(deviceAlphabet))))
		b[i] = deviceAlphabet[n.Int64()]
	}
	return string(b[:4]) + "-" + string(b[4:])
}

func normDeviceCode(s string) string {
	s = strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(s)))
	if len(s) != 8 {
		return ""
	}
	return s[:4] + "-" + s[4:]
}

// POST /app/api/device/code — a code another device can use to sign in here.
func (h *Host) handleDeviceCode(w http.ResponseWriter, r *http.Request) {
	me, err := h.resolve(r)
	if err != nil {
		writeStatusJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	code := newDeviceCode()
	signinMu.Lock()
	for c, d := range deviceCodes {
		if d.acct == me.ID || time.Now().After(d.expires) {
			delete(deviceCodes, c) // one live code per account
		}
	}
	deviceCodes[code] = deviceCode{acct: me.ID, expires: time.Now().Add(10 * time.Minute)}
	signinMu.Unlock()
	url := strings.TrimRight(h.PublicBase, "/") + "/app?device=" + code
	writeJSON(w, map[string]any{"code": code, "url": url, "expires_in": 600})
}

// POST /app/api/device/redeem {code} — sign this device into the account
// that made the code.
func (h *Host) handleDeviceRedeem(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code string `json:"code"`
	}
	json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&in)
	code := normDeviceCode(in.Code)
	signinMu.Lock()
	d, ok := deviceCodes[code]
	if ok {
		delete(deviceCodes, code)
	}
	signinMu.Unlock()
	if !ok || time.Now().After(d.expires) {
		writeJSON(w, map[string]any{"error": "That code has been used or has expired. Make a new one on your other device."})
		return
	}
	h.signedIn(w, d.acct)
}
