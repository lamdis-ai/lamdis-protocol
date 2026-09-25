package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Email: a way to be found, and a way to reach somebody not here yet.
//
// An address counts only once its owner has typed back a code we sent to
// it, so nobody can put someone else's address on their account and receive
// their invitations. The index maps a hash of the address to the account,
// so the host's files do not hold a readable list of everybody's email.
// Searching by email is exact-match only: no prefixes, no guessing.
//
// Inviting an address nobody here has confirmed sends a single-use join
// link to it. Invitations by email are capped per account per day.

func normEmail(e string) (string, bool) {
	a, err := mail.ParseAddress(strings.TrimSpace(e))
	if err != nil || !strings.Contains(a.Address, ".") {
		return "", false
	}
	return strings.ToLower(a.Address), true
}

func emailKey(e string) string {
	h := sha256.Sum256([]byte("lamdis-email\x00" + e))
	return hex.EncodeToString(h[:])
}

func (h *Host) emailIndex() string { return filepath.Join(h.Root, ".emails") }

func (h *Host) accountByEmail(e string) *Account {
	raw, err := os.ReadFile(filepath.Join(h.emailIndex(), emailKey(e)))
	if err != nil {
		return nil
	}
	return h.accountByID(strings.TrimSpace(string(raw)))
}

var (
	emailMu    sync.Mutex
	emailCodes = map[string]pendingEmail{} // account -> pending confirmation
	mailSent   = map[string][]time.Time{}  // account -> recent sends, for limits
)

type pendingEmail struct {
	email, code string
	expires     time.Time
	tries       int
}

// allowMail enforces a per-account ceiling on mail this host sends for them.
func allowMail(acct string, perDay int) bool {
	emailMu.Lock()
	defer emailMu.Unlock()
	var keep []time.Time
	for _, t := range mailSent[acct] {
		if time.Since(t) < 24*time.Hour {
			keep = append(keep, t)
		}
	}
	if len(keep) >= perDay {
		mailSent[acct] = keep
		return false
	}
	mailSent[acct] = append(keep, time.Now())
	return true
}

func sixDigits() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(1000000))
	return fmt.Sprintf("%06d", n.Int64())
}

// GET /app/api/email -> {email, confirmed, can_send}; POST {email} sends a code.
func (h *Host) handleEmail(w http.ResponseWriter, r *http.Request) {
	me, err := h.resolve(r)
	if err != nil {
		writeStatusJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	if r.Method == "GET" {
		raw, _ := os.ReadFile(filepath.Join(me.Dir, "email_confirmed"))
		writeJSON(w, map[string]any{"email": strings.TrimSpace(string(raw)), "confirmed": len(raw) > 0, "can_send": h.Mail != nil})
		return
	}
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
	if other := h.accountByEmail(e); other != nil && other.ID != me.ID {
		writeJSON(w, map[string]any{"error": "That address is already confirmed on another account."})
		return
	}
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
	writeJSON(w, map[string]any{"ok": true, "sent_to": e})
}

// POST /app/api/email/confirm {code}
func (h *Host) handleEmailConfirm(w http.ResponseWriter, r *http.Request) {
	me, err := h.resolve(r)
	if err != nil {
		writeStatusJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	var in struct {
		Code string `json:"code"`
	}
	json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&in)
	emailMu.Lock()
	p, ok := emailCodes[me.ID]
	if ok {
		p.tries++
		emailCodes[me.ID] = p
	}
	emailMu.Unlock()
	if !ok || time.Now().After(p.expires) || p.tries > 5 {
		writeJSON(w, map[string]any{"error": "That code has expired. Send a new one."})
		return
	}
	if strings.TrimSpace(in.Code) != p.code {
		writeJSON(w, map[string]any{"error": "That code is not right."})
		return
	}
	emailMu.Lock()
	delete(emailCodes, me.ID)
	emailMu.Unlock()
	peopleMu.Lock()
	defer peopleMu.Unlock()
	if old, err := os.ReadFile(filepath.Join(me.Dir, "email_confirmed")); err == nil && len(old) > 0 {
		os.Remove(filepath.Join(h.emailIndex(), emailKey(strings.TrimSpace(string(old)))))
	}
	os.MkdirAll(h.emailIndex(), 0o700)
	os.WriteFile(filepath.Join(h.emailIndex(), emailKey(p.email)), []byte(me.ID), 0o600)
	os.WriteFile(filepath.Join(me.Dir, "email_confirmed"), []byte(p.email), 0o600)
	writeJSON(w, map[string]any{"ok": true, "email": p.email})
}

// inviteByEmail invites whoever confirmed this address, or mails a join
// link when nobody here has.
func (h *Host) inviteByEmail(w http.ResponseWriter, r *http.Request, me *Account, thread, title, e string) {
	inviter := displayOf(me)
	if inviter == "" {
		inviter = "@" + h.handleOf(me)
	}
	if them := h.accountByEmail(e); them != nil && them.ID != me.ID {
		if err := me.grant(r.Context(), thread, them.Self); err != nil {
			writeJSON(w, map[string]any{"error": "could not grant access: " + err.Error()})
			return
		}
		h.knowEachOther(me, them)
		tag := h.handleOf(me) // takes peopleMu itself, so not while holding it
		peopleMu.Lock()
		list := append(readInvites(them.Dir), invite{ID: strings.ToLower(randCode(8)), FromAcct: me.ID, FromName: displayOf(me), FromTag: tag, Thread: thread, Title: title, Created: h.now()})
		writeInvites(them.Dir, list)
		peopleMu.Unlock()
		if h.Mail != nil && allowMail(me.ID, 30) {
			h.Mail.Send(r.Context(), e, inviter+" added you to #"+title+" on Lamdis",
				inviter+" added you to the channel #"+title+" on Lamdis.\n\nOpen "+strings.TrimRight(h.PublicBase, "/")+"/app to join it.\n\nLamdis")
		}
		writeJSON(w, map[string]any{"ok": true, "handle": h.handleOf(them), "how": "account"})
		return
	}
	if h.Mail == nil {
		writeJSON(w, map[string]any{"error": "Nobody here has confirmed that address, and this host cannot send email yet. Send them an invite link instead."})
		return
	}
	if !allowMail(me.ID, 30) {
		writeJSON(w, map[string]any{"error": "That is as many email invitations as one account sends in a day."})
		return
	}
	link, err := h.newJoinLink(me, thread)
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	body := inviter + " invited you to the channel #" + title + " on Lamdis, where they work with their AI agent.\n\n" +
		"Join here (no sign-up needed; the link works once, for 7 days):\n" + link + "\n\n" +
		"You will see only this channel. If you were not expecting this, ignore it.\n\nLamdis"
	if err := h.Mail.Send(r.Context(), e, inviter+" invited you to #"+title+" on Lamdis", body); err != nil {
		h.logf("host: mail invite: %v", err)
		writeJSON(w, map[string]any{"error": "Could not send the email just now."})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "how": "email", "sent_to": e})
}
