package api

// Starting without being asked who you are.
//
// The moment a product is worth anything is the moment somebody uses it for
// their own thing, and every step before that loses most of the people who
// were curious. So there is no sign-up here: the first visit quietly makes a
// node, hands the browser a key to it, and drops the person into an empty
// thread with the cursor blinking.
//
// The account is real from the first keystroke — a keypair, a signed log, an
// agent of its own — so nothing has to be migrated later. Signing in with an
// email attaches an identity to the node that already exists rather than
// moving anything, which is the only way a guest start is honest.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// guestSecret is the key the host signs visitor tokens with. It lives beside
// the accounts, so restarting the process does not log everybody out.
func (h *Host) guestSecret() ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.secret) > 0 {
		return h.secret, nil
	}
	path := filepath.Join(h.Root, "guest.key")
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(h.Root, 0o700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, []byte(hex.EncodeToString(buf)), 0o600); err != nil {
			return nil, err
		}
		h.secret = buf
		return buf, nil
	} else if err != nil {
		return nil, err
	}
	buf, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(buf) < 16 {
		return nil, fmt.Errorf("guest key in %s is corrupt", h.Root)
	}
	h.secret = buf
	return buf, nil
}

// mintGuest signs a token naming one account.
func (h *Host) mintGuest(id string) (string, error) {
	secret, err := h.guestSecret()
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("guest\x00" + id))
	return "g." + id + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// readGuest returns the account a token names, if the signature holds.
func (h *Host) readGuest(tok string) (string, bool) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 || parts[0] != "g" {
		return "", false
	}
	secret, err := h.guestSecret()
	if err != nil {
		return "", false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("guest\x00" + parts[1]))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(parts[2])) {
		return "", false
	}
	return parts[1], true
}

// handleStart makes a node for somebody who has not said who they are, and
// hands their browser the only key to it.
func (h *Host) handleStart(w http.ResponseWriter, r *http.Request) {
	if !h.Guests {
		writeStatusJSON(w, http.StatusForbidden, map[string]any{"error": "this host asks people to sign in first"})
		return
	}
	buf := make([]byte, 8)
	rand.Read(buf)
	id := "g" + hex.EncodeToString(buf)
	acct, err := h.load(id, "")
	if err != nil {
		writeStatusJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error()})
		return
	}
	os.WriteFile(filepath.Join(acct.Dir, "guest"), []byte("1"), 0o600)
	tok, err := h.mintGuest(id)
	if err != nil {
		writeStatusJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not start"})
		return
	}
	writeJSON(w, map[string]any{"token": tok, "guest": true})
}

// attach binds an email identity to the node somebody already started, so
// signing in keeps everything rather than beginning again. The mapping is a
// file in the account, and a pointer beside the accounts so the subject can
// be found on the next visit.
func (h *Host) attach(subject, email, guestID string) (*Account, error) {
	acct, err := h.load(guestID, email)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(acct.Dir, "subject"), []byte(subject), 0o600); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(h.Root, ".by-subject"), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(h.Root, ".by-subject", accountID(subject)), []byte(guestID), 0o600); err != nil {
		return nil, err
	}
	os.Remove(filepath.Join(acct.Dir, "guest"))
	h.mu.Lock()
	acct.Email = email
	h.mu.Unlock()
	return acct, nil
}

// accountFor resolves an email identity to its node, following an earlier
// attachment when there is one.
func (h *Host) accountFor(subject, email string) (*Account, error) {
	if raw, err := os.ReadFile(filepath.Join(h.Root, ".by-subject", accountID(subject))); err == nil {
		if id := strings.TrimSpace(string(raw)); id != "" {
			return h.load(id, email)
		}
	}
	return h.load(accountID(subject), email)
}
