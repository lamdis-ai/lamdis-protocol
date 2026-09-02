package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Capacity is what an operator will take, how much at once, and how far out.
//
// It is a promise in both directions. The operator is telling us what they can
// actually service, and we are agreeing not to dispatch outside it — which is
// what makes "auto-accept" safe to offer, and what stops a queue full of work
// somebody was never going to take.
//
// The same three controls serve a person and a fleet. A person holds two jobs
// inside twelve miles; a fleet holds twenty-four inside eight. The shape of the
// decision does not change with scale, so neither does the interface.
type Capacity struct {
	// MaxConcurrent is how many jobs they will hold at once. It is capped by
	// what they have earned — see Board.allowanceFor — so this can lower the
	// ceiling but never raise it above their standing.
	MaxConcurrent int `json:"max_concurrent"`
	// RangeMiles is how far from base they will travel.
	RangeMiles int `json:"range_miles"`
	// Kinds are the job kinds they accept. Empty means all of them.
	Kinds []string `json:"kinds"`
	// Accepting is the master switch. Off means finish what you hold and stop,
	// rather than disappearing mid-job.
	Accepting bool `json:"accepting"`
	// AutoAccept takes matching work without asking. Only meaningful for an
	// operator that can answer a webhook, so it is refused for anyone without
	// an endpoint registered.
	AutoAccept bool `json:"auto_accept"`
	// LatE7 and LonE7 are where the operator works from. Without them the
	// range control is decoration: there is nothing to measure from.
	LatE7 int64 `json:"lat_e7,omitempty"`
	LonE7 int64 `json:"lon_e7,omitempty"`
	// Skills are what this operator is qualified for. Claimed here, and for
	// licensed trades checked against nothing yet — the exchange holds the
	// claim so it can be shown to the buyer and revoked, not so it can be
	// believed.
	Skills []Skill `json:"skills,omitempty"`
	// WebhookSecret signs offers so the operator can tell ours from anyone
	// else's POST. Generated here, shown once, never accepted from a client:
	// a secret the caller chooses is a secret the caller can also guess.
	WebhookSecret string `json:"webhook_secret,omitempty"`
	// Webhook is where dispatch offers are sent, for operators that take work
	// over the API rather than by opening a page.
	Webhook string `json:"webhook,omitempty"`
}

// DefaultCapacity is deliberately conservative: one job, a short range, and
// everything switched on that cannot cause a surprise.
func DefaultCapacity() Capacity {
	return Capacity{MaxConcurrent: 1, RangeMiles: 12, Accepting: true}
}

// Positioned reports whether we know where this operator is.
func (c Capacity) Positioned() bool { return HasPosition(c.LatE7, c.LonE7) }

// Takes reports whether a job of this kind is one they said they would take.
func (c Capacity) Takes(kind string) bool {
	if len(c.Kinds) == 0 {
		return true
	}
	for _, k := range c.Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// Capacities stores what each operator will accept.
type Capacities struct {
	mu   sync.Mutex
	by   map[string]Capacity
	path string
}

func NewCapacities() *Capacities { return &Capacities{by: map[string]Capacity{}} }

func (cs *Capacities) Get(worker string) Capacity {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	c, ok := cs.by[worker]
	if !ok {
		return DefaultCapacity()
	}
	return c
}

// Set records a change, clamping anything a caller could otherwise use to
// promise more than the exchange will allow.
func (cs *Capacities) Set(worker string, c Capacity) Capacity {
	if c.MaxConcurrent < 1 {
		c.MaxConcurrent = 1
	}
	if c.MaxConcurrent > 40 {
		c.MaxConcurrent = 40
	}
	if c.RangeMiles < 1 {
		c.RangeMiles = 1
	}
	c.Skills = NormalizeSkills(c.Skills)
	if c.RangeMiles > 60 {
		c.RangeMiles = 60
	}
	// Auto-accept without somewhere to send the offer is a setting that does
	// nothing, which is worse than one that is refused.
	if c.Webhook == "" {
		c.AutoAccept = false
	}
	if c.Webhook != "" && !strings.HasPrefix(c.Webhook, "https://") {
		c.Webhook = ""
		c.AutoAccept = false
	}
	cs.mu.Lock()
	// Keep the existing secret across edits; mint one the first time an
	// endpoint appears. Rotating on every save would break a working
	// integration every time somebody nudged their range slider.
	prev := cs.by[worker]
	c.WebhookSecret = prev.WebhookSecret
	if c.Webhook == "" {
		c.WebhookSecret = ""
	} else if c.WebhookSecret == "" {
		c.WebhookSecret = newWebhookSecret()
	}
	cs.by[worker] = c
	cs.saveLocked()
	cs.mu.Unlock()
	return c
}

// CapacityServer exposes the controls.
type CapacityServer struct {
	Workers    *Workers
	Capacities *Capacities
	Board      *Board
	Now        func() time.Time
}

func (s *CapacityServer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *CapacityServer) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/capacity", s.handleGet)
	mux.HandleFunc("PUT /v1/capacity", s.handleSet)
}

func (s *CapacityServer) worker(r *http.Request, body []byte) (*Worker, bool) {
	w, err := s.Workers.Authenticate(r, body, s.now())
	if err != nil {
		return nil, false
	}
	return w, true
}

func (s *CapacityServer) handleGet(w http.ResponseWriter, r *http.Request) {
	body, _ := readBody(r)
	worker, ok := s.worker(r, body)
	if !ok {
		refuse(w)
		return
	}
	done, abandoned, allowance, cool := s.Board.Standing(worker.ID)
	writeWork(w, http.StatusOK, map[string]any{
		"capacity": s.Capacities.Get(worker.ID),
		"standing": map[string]any{
			"completed": done, "abandoned": abandoned,
			"allowance": allowance,
			"cooldown_until": func() any {
				if cool.IsZero() {
					return nil
				}
				return cool.Format(time.RFC3339)
			}(),
		},
		// The ceiling is earned, so say what it is rather than letting somebody
		// set a number the exchange will silently ignore.
		"ceiling": allowance,
	})
}

func (s *CapacityServer) handleSet(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		refuse(w)
		return
	}
	worker, ok := s.worker(r, body)
	if !ok {
		refuse(w)
		return
	}
	var in Capacity
	if err := json.Unmarshal(body, &in); err != nil {
		writeWork(w, http.StatusBadRequest, map[string]string{"error": "malformed request"})
		return
	}
	saved := s.Capacities.Set(worker.ID, in)
	_, _, allowance, _ := s.Board.Standing(worker.ID)
	var note string
	if saved.MaxConcurrent > allowance {
		note = "You can hold " + itoa(allowance) +
			" at once for now; finishing jobs raises it."
	}
	writeWork(w, http.StatusOK, map[string]any{
		"capacity": saved, "ceiling": allowance, "note": note,
	})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func newWebhookSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// A predictable webhook secret is worse than no webhook, because it
		// invites an operator to trust an offer anyone could have forged.
		return ""
	}
	return "lam_whsec_" + hex.EncodeToString(b)
}

// All returns every operator's settings, copied.
//
// Dispatch has to iterate the whole set to answer "who could take this", and
// doing that under the store's lock while making HTTP calls would block every
// operator editing their settings for as long as the slowest endpoint takes.
func (cs *Capacities) All() map[string]Capacity {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	out := make(map[string]Capacity, len(cs.by))
	for k, v := range cs.by {
		out[k] = v
	}
	return out
}
