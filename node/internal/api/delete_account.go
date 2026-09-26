package api

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Deleting an account: everything goes. The node's directory (channels,
// agent, keys, sealed credentials) is removed, and so is every way of
// finding it: its handle, its confirmed email, its passkeys. Copies other
// people already synced of channels they were in stay theirs, as they
// always have; this account can no longer write to them.
//
// POST /app/api/account/delete {"confirm":"delete"}
func (h *Host) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	me, err := h.resolve(r)
	if err != nil {
		writeStatusJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	var in struct {
		Confirm string `json:"confirm"`
	}
	json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&in)
	if strings.ToLower(strings.TrimSpace(in.Confirm)) != "delete" {
		writeJSON(w, map[string]any{"error": `send {"confirm":"delete"} to delete this account`})
		return
	}
	peopleMu.Lock()
	if raw, err := os.ReadFile(filepath.Join(me.Dir, "handle")); err == nil {
		if tag := strings.TrimSpace(string(raw)); tag != "" {
			os.Remove(filepath.Join(h.handlesDir(), tag))
		}
	}
	if raw, err := os.ReadFile(filepath.Join(me.Dir, "email_confirmed")); err == nil {
		if e := strings.TrimSpace(string(raw)); e != "" {
			os.Remove(filepath.Join(h.emailIndex(), emailKey(e)))
		}
	}
	for _, c := range readCreds(me.Dir) {
		os.Remove(filepath.Join(h.passkeyIndex(), base64.RawURLEncoding.EncodeToString(c.ID)))
	}
	peopleMu.Unlock()

	h.mu.Lock()
	delete(h.accounts, me.ID)
	h.mu.Unlock()
	if me.Store != nil {
		me.Store.Close()
	}
	if err := os.RemoveAll(me.Dir); err != nil {
		h.logf("host: could not delete %s: %v", me.ID, err)
		writeJSON(w, map[string]any{"error": "could not finish deleting; try again"})
		return
	}
	// Its tokens still verify, so say it is gone rather than let one
	// quietly start an empty account under the same name.
	os.MkdirAll(filepath.Join(h.Root, ".deleted"), 0o700)
	os.WriteFile(filepath.Join(h.Root, ".deleted", me.ID), nil, 0o600)
	h.logf("host: account deleted at its owner's request")
	writeJSON(w, map[string]any{"ok": true})
}

// POST /app/api/report {thread, entry, reason}: somebody reports something
// another person wrote. It goes to the people who run this host, with the
// text, so it can be acted on; the reporter can also remove that person.
func (h *Host) handleReport(w http.ResponseWriter, r *http.Request) {
	me, err := h.resolve(r)
	if err != nil {
		writeStatusJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	var in struct {
		Thread string `json:"thread"`
		Entry  string `json:"entry"`
		Reason string `json:"reason"`
	}
	json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&in)
	tl, err := me.Store.Thread(r.Context(), in.Thread)
	if err != nil {
		writeJSON(w, map[string]any{"error": "no such channel"})
		return
	}
	e := tl.Get(in.Entry)
	if e == nil {
		writeJSON(w, map[string]any{"error": "no such message"})
		return
	}
	var b struct {
		Text string `json:"text"`
	}
	json.Unmarshal(e.Body, &b)
	if !allowMail(me.ID+":report", 20) {
		writeJSON(w, map[string]any{"error": "That is as many reports as one account sends in a day. Write to support@lamdis.ai."})
		return
	}
	report := "Reported by account " + me.ID + " (@" + h.handleOf(me) + ")\nChannel: " + in.Thread + "\nMessage: " + in.Entry +
		"\nAuthor: " + e.Author + "\nReason: " + strings.TrimSpace(in.Reason) + "\n\nText:\n" + b.Text + "\n"
	h.logf("host: report filed on %s in %s", in.Entry, in.Thread)
	os.MkdirAll(filepath.Join(h.Root, ".reports"), 0o700)
	os.WriteFile(filepath.Join(h.Root, ".reports", in.Entry+".txt"), []byte(report), 0o600)
	if h.Mail != nil {
		h.Mail.Send(r.Context(), envOrDefault("LAMDIS_REPORTS_TO", "support@lamdis.ai"), "Lamdis report: message "+in.Entry, report)
	}
	writeJSON(w, map[string]any{"ok": true})
}
