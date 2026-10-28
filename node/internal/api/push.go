package api

// Push tells somebody's phone that their agent said something while they
// were away. The phone hands us its device token; when a run the scheduler
// fired posts or asks, Apple's push service carries the first line of it.
// Without a key configured the tokens are still kept, so turning push on
// later reaches everybody who already allowed it.

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// pushesPerDay caps what one account's phones hear in a day: an hourly
// schedule should not buzz somebody's pocket twenty-four times.
const pushesPerDay = 12

// maxDevices is how many phones one account keeps tokens for.
const maxDevices = 5

var deviceTokenRe = regexp.MustCompile(`^[0-9a-f]{64,200}$`)

type device struct {
	Token string `json:"token"`
	// Sandbox is a development build talking to Apple's test service.
	Sandbox bool   `json:"sandbox,omitempty"`
	At      string `json:"at"`
}

type pushFile struct {
	Devices []device `json:"devices"`
	Day     string   `json:"day,omitempty"`
	Sent    int      `json:"sent,omitempty"`
}

var pushMu sync.Mutex

func readPush(dir string) pushFile {
	var pf pushFile
	if raw, err := os.ReadFile(filepath.Join(dir, "push.json")); err == nil {
		json.Unmarshal(raw, &pf)
	}
	return pf
}

func writePush(dir string, pf pushFile) error {
	raw, _ := json.Marshal(pf)
	return os.WriteFile(filepath.Join(dir, "push.json"), raw, 0o600)
}

// handlePush keeps the device token a phone sends after somebody allows
// notifications, or forgets it when they turn them off.
func (h *Host) handlePush(w http.ResponseWriter, r *http.Request) {
	me, err := h.resolve(r)
	if err != nil {
		writeStatusJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
		return
	}
	var in struct {
		Token   string `json:"token"`
		Sandbox bool   `json:"sandbox"`
		Off     bool   `json:"off"`
	}
	json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&in)
	tok := strings.ToLower(strings.TrimSpace(in.Token))
	if !deviceTokenRe.MatchString(tok) {
		writeStatusJSON(w, http.StatusBadRequest, map[string]any{"error": "that is not a device token"})
		return
	}
	pushMu.Lock()
	defer pushMu.Unlock()
	pf := readPush(me.Dir)
	kept := pf.Devices[:0]
	for _, d := range pf.Devices {
		if d.Token != tok {
			kept = append(kept, d)
		}
	}
	if !in.Off {
		kept = append(kept, device{Token: tok, Sandbox: in.Sandbox, At: h.now().UTC().Format(time.RFC3339)})
	}
	if len(kept) > maxDevices {
		kept = kept[len(kept)-maxDevices:]
	}
	pf.Devices = kept
	if err := writePush(me.Dir, pf); err != nil {
		writeStatusJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not save that"})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "on": h.push != nil})
}

// notify sends one line to every phone on an account, within the day's cap.
func (h *Host) notify(dir, thread, title, text string) {
	if h.push == nil {
		return
	}
	pushMu.Lock()
	pf := readPush(dir)
	today := h.now().UTC().Format("2006-01-02")
	if pf.Day != today {
		pf.Day, pf.Sent = today, 0
	}
	if len(pf.Devices) == 0 || pf.Sent >= pushesPerDay {
		pushMu.Unlock()
		return
	}
	pf.Sent++
	writePush(dir, pf)
	devices := append([]device(nil), pf.Devices...)
	pushMu.Unlock()

	var gone []string
	for _, d := range devices {
		if err := h.push.send(d, thread, title, text); errors.Is(err, errDeviceGone) {
			gone = append(gone, d.Token)
		} else if err != nil {
			h.logf("host: push: %v", err)
		}
	}
	if len(gone) > 0 {
		pushMu.Lock()
		pf := readPush(dir)
		kept := pf.Devices[:0]
		for _, d := range pf.Devices {
			drop := false
			for _, g := range gone {
				drop = drop || d.Token == g
			}
			if !drop {
				kept = append(kept, d)
			}
		}
		pf.Devices = kept
		writePush(dir, pf)
		pushMu.Unlock()
	}
}

// pushLine is the first sentence-ish of what the agent said, short enough
// for a lock screen.
func pushLine(text string) string {
	text = strings.TrimSpace(text)
	if i := strings.Index(text, "\n"); i > 0 {
		text = text[:i]
	}
	text = strings.NewReplacer("**", "", "__", "", "`", "", "# ", "").Replace(text)
	if r := []rune(text); len(r) > 180 {
		text = strings.TrimSpace(string(r[:177])) + "…"
	}
	return text
}

var errDeviceGone = errors.New("device token no longer valid")

// apns talks to Apple's push service with a token-based key.
type apns struct {
	keyID, team, topic string
	key                *ecdsa.PrivateKey
	client             *http.Client
	// base replaces Apple's address in tests.
	base string

	mu     sync.Mutex
	jwt    string
	minted time.Time
}

// newAPNS reads the key from the environment; nil when push is not set up.
func newAPNS() (*apns, error) {
	raw, id := os.Getenv("LAMDIS_APNS_KEY"), os.Getenv("LAMDIS_APNS_KEY_ID")
	if raw == "" || id == "" {
		return nil, nil
	}
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, errors.New("LAMDIS_APNS_KEY is not a PEM key")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	ec, ok := k.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("LAMDIS_APNS_KEY is not an EC key")
	}
	app := envOrDefault("LAMDIS_IOS_APP_ID", "JNA2Z7W52M.ai.lamdis.app")
	team, topic, _ := strings.Cut(app, ".")
	return &apns{keyID: id, team: envOrDefault("LAMDIS_APNS_TEAM", team), topic: topic, key: ec,
		client: &http.Client{Timeout: 10 * time.Second}}, nil
}

// token is the signed bearer Apple wants, reused for up to 40 minutes as
// it asks (it refuses ones older than an hour and dislikes fresh ones per call).
func (a *apns) token() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.jwt != "" && time.Since(a.minted) < 40*time.Minute {
		return a.jwt, nil
	}
	enc := base64.RawURLEncoding
	head := enc.EncodeToString([]byte(fmt.Sprintf(`{"alg":"ES256","kid":%q}`, a.keyID)))
	now := time.Now()
	claims := enc.EncodeToString([]byte(fmt.Sprintf(`{"iss":%q,"iat":%d}`, a.team, now.Unix())))
	sum := sha256.Sum256([]byte(head + "." + claims))
	r, s, err := ecdsa.Sign(rand.Reader, a.key, sum[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	a.jwt, a.minted = head+"."+claims+"."+enc.EncodeToString(sig), now
	return a.jwt, nil
}

func (a *apns) send(d device, thread, title, text string) error {
	jwt, err := a.token()
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{
		"aps":    map[string]any{"alert": map[string]any{"title": title, "body": text}, "sound": "default", "thread-id": thread},
		"thread": thread,
	})
	host := "https://api.push.apple.com"
	if d.Sandbox {
		host = "https://api.sandbox.push.apple.com"
	}
	if a.base != "" {
		host = a.base
	}
	req, _ := http.NewRequest("POST", host+"/3/device/"+d.Token, bytes.NewReader(payload))
	req.Header.Set("authorization", "bearer "+jwt)
	req.Header.Set("apns-topic", a.topic)
	req.Header.Set("apns-push-type", "alert")
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	var why struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(io.LimitReader(resp.Body, 1<<10)).Decode(&why)
	if resp.StatusCode == http.StatusGone || why.Reason == "BadDeviceToken" || why.Reason == "Unregistered" || why.Reason == "DeviceTokenNotForTopic" {
		return errDeviceGone
	}
	return fmt.Errorf("apple said %d %s", resp.StatusCode, why.Reason)
}
