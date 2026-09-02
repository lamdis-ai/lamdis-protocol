package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// A capability lets someone with no keypair do exactly one narrow thing.
//
// The existing scheme assumes every caller is a principal. A gig worker or a
// reviewer on a phone is not, and handing them a real key is not realistic. So
// they get a link instead — and the design goal is that the link can never be
// escalated into a principal.
//
// Three properties do that work:
//
//   - The secret lives in the URL *fragment*, so it never reaches a server
//     log, an access log, or a Referer header.
//   - Only sha256(secret) is stored, so a dump of our database yields no
//     working links.
//   - Capability handlers live under their own path prefix and take a
//     *Capability, never a principal id. That is a compile-time guarantee that
//     no principal-authenticated handler can be reached with one.
//
// Requests are authenticated with the same signing input the Ed25519 scheme
// uses — method, path, timestamp, body hash — so a capability inherits every
// replay and tamper property of the real thing. Only the key type differs:
// symmetric proof-of-possession instead of a signature.
type Capability struct {
	// Job is the single job this capability may act on.
	Job string
	// Actions bounds what it may do.
	Actions []string
	// Holder is sha256 of the secret. The secret itself is never stored.
	Holder string
	// Expires is when the link stops working.
	Expires time.Time
	// EnrollmentsLeft is how many times a device key may still be bound. One,
	// normally: the first phone to open the link claims it.
	EnrollmentsLeft int
	// DevicePrincipal is set once a phone has proved it can hold a real key,
	// after which its submissions carry its own signature rather than ours.
	DevicePrincipal string
	// Label is what the human is being asked to do, for the page.
	Label string
}

// Capability actions.
const (
	ActionView    = "view"
	ActionAccept  = "accept"
	ActionSubmit  = "submit_evidence"
	ActionReview  = "review"
	ActionDecline = "decline"
)

// Can reports whether this capability permits an action.
func (c *Capability) Can(action string) bool {
	for _, a := range c.Actions {
		if a == action {
			return true
		}
	}
	return false
}

// Attestation reports how a submission under this capability should be
// recorded. A device-key holder signs for themselves; everyone else is signed
// for by the exchange, and the trail must say so rather than imply a signature
// that was never made.
func (c *Capability) Attestation() string {
	if c.DevicePrincipal != "" {
		return "device_key"
	}
	return "capability"
}

// Capabilities issues and checks capability links.
type Capabilities struct {
	mu   sync.Mutex
	byID map[string]*Capability // keyed by sha256(secret)
	// path is where the registry is written, so a link a worker is holding
	// survives a deploy. Empty means memory only. See persist.go.
	path string
	Now  func() time.Time
}

func NewCapabilities() *Capabilities {
	return &Capabilities{byID: map[string]*Capability{}, Now: time.Now}
}

func (cs *Capabilities) now() time.Time {
	if cs.Now != nil {
		return cs.Now()
	}
	return time.Now()
}

// secretAlphabet is Crockford base32, matching the protocol's principal ids:
// no I, L, O or U, so a code read aloud or copied by hand is unambiguous.
var secretAlphabet = base32.NewEncoding("0123456789ABCDEFGHJKMNPQRSTVWXYZ").WithPadding(base32.NoPadding)

// Issue mints a capability and returns the secret exactly once. The secret is
// never stored and cannot be recovered — losing it means issuing a new link.
func (cs *Capabilities) Issue(job, label string, actions []string, ttl time.Duration) (string, *Capability, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("capability: generating secret: %w", err)
	}
	secret := secretAlphabet.EncodeToString(raw)
	sum := sha256.Sum256([]byte(secret))
	holder := hex.EncodeToString(sum[:])

	c := &Capability{
		Job: job, Actions: actions, Holder: holder, Label: label,
		Expires:         cs.now().Add(ttl),
		EnrollmentsLeft: 1,
	}
	cs.mu.Lock()
	cs.byID[holder] = c
	cs.saveLocked()
	cs.mu.Unlock()
	return secret, c, nil
}

// Lookup finds a capability by its secret.
func (cs *Capabilities) Lookup(secret string) (*Capability, bool) {
	sum := sha256.Sum256([]byte(secret))
	cs.mu.Lock()
	defer cs.mu.Unlock()
	c, ok := cs.byID[hex.EncodeToString(sum[:])]
	if !ok || cs.now().After(c.Expires) {
		return nil, false
	}
	return c, true
}

// Enroll binds a device public key to a capability, spending the one-time
// enrollment. After this the phone signs for itself.
func (cs *Capabilities) Enroll(secret, principal string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	sum := sha256.Sum256([]byte(secret))
	c, ok := cs.byID[hex.EncodeToString(sum[:])]
	if !ok || cs.now().After(c.Expires) {
		return fmt.Errorf("capability: unknown or expired")
	}
	if c.EnrollmentsLeft < 1 {
		return fmt.Errorf("capability: enrollment already spent")
	}
	c.EnrollmentsLeft--
	c.DevicePrincipal = principal
	cs.saveLocked()
	return nil
}

const hdrCapability = "X-Lamdis-Capability"

// proof builds the HMAC a capability holder presents. The signing input is
// identical to the Ed25519 scheme's, so the binding to method, path, time and
// body is the same.
func proof(secret, method, path, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(signingInput(method, path, timestamp, body))
	return hex.EncodeToString(mac.Sum(nil))
}

// SignCapabilityRequest adds the headers a capability holder must present.
func SignCapabilityRequest(req *http.Request, job, secret string, body []byte, now time.Time) {
	ts := now.UTC().Format(time.RFC3339)
	req.Header.Set(hdrTimestamp, ts)
	req.Header.Set(hdrCapability, job+"."+proof(secret, req.Method, req.URL.Path, ts, body))
}

// authenticateCapability verifies a capability-authenticated request.
//
// It takes the secret from storage rather than the wire: the holder proves
// possession by computing the HMAC, and never transmits the secret itself.
// Because we store only the hash, verification tries each capability for the
// named job — in practice one.
func (cs *Capabilities) authenticate(r *http.Request, body []byte, now time.Time, secrets func(job string) []string) (*Capability, error) {
	header := r.Header.Get(hdrCapability)
	ts := r.Header.Get(hdrTimestamp)
	if header == "" || ts == "" {
		return nil, fmt.Errorf("missing capability headers")
	}
	job, mac, ok := strings.Cut(header, ".")
	if !ok || job == "" || mac == "" {
		return nil, fmt.Errorf("malformed capability header")
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return nil, fmt.Errorf("bad timestamp")
	}
	if d := now.Sub(t); d > maxSkew || d < -maxSkew {
		return nil, fmt.Errorf("timestamp outside allowed skew")
	}
	want, err := hex.DecodeString(mac)
	if err != nil {
		return nil, fmt.Errorf("bad proof encoding")
	}
	if secrets == nil {
		// A missing secret source is a configuration error, not a caller
		// error. Refusing is the only safe reading: with no candidates there
		// is nothing to verify against, and panicking would take the
		// connection down.
		return nil, fmt.Errorf("no capability secrets available")
	}
	for _, secret := range secrets(job) {
		got, err := hex.DecodeString(proof(secret, r.Method, r.URL.Path, ts, body))
		if err != nil {
			continue
		}
		if hmac.Equal(got, want) {
			c, ok := cs.Lookup(secret)
			if !ok {
				return nil, fmt.Errorf("capability expired")
			}
			if c.Job != job {
				return nil, fmt.Errorf("capability is for another job")
			}
			return c, nil
		}
	}
	return nil, fmt.Errorf("capability verification failed")
}
