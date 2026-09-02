package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"time"
)

// Sending work to operators who asked to be told about it.
//
// Until this file existed, an operator could configure an endpoint, switch on
// auto-accept, and wait forever: the settings were stored, validated, and read
// by nothing. A control that does nothing is a worse lie than a missing
// feature, because the operator has no way to discover the difference.
//
// The shape here is an offer, not an assignment. The exchange POSTs "this
// exists, do you want it"; a 2xx means yes. Assignment still goes through
// Claim, so every rule that bounds abuse — concurrency, cooldown, range,
// skills, seats — applies identically whether a person tapped a button or a
// program answered a webhook. Dispatch is a notification channel, not a
// second way in.

// Offer is what an operator's endpoint receives.
type Offer struct {
	Offer string `json:"offer"`
	Job   string `json:"job"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
	// Area is the coarse locality, and AreaLat and AreaLon the same point the
	// public board publishes: two decimal places, about a kilometre. The exact
	// address is deliberately absent. An offer goes to every endpoint in
	// range before anybody has claimed anything, and an operator who
	// registered a webhook to harvest front doors must get nothing a stranger
	// reading the board could not. The address rides only in WorkURL, behind
	// the capability that a claim mints.
	Area          string    `json:"area,omitempty"`
	AreaLat       float64   `json:"area_lat,omitempty"`
	AreaLon       float64   `json:"area_lon,omitempty"`
	DistanceMiles float64   `json:"distance_miles,omitempty"`
	Skills        []Skill   `json:"skills,omitempty"`
	PayMinor      int64     `json:"pay_minor"`
	BonusMinor    int64     `json:"bonus_minor,omitempty"`
	Currency      string    `json:"currency"`
	Tier          string    `json:"tier,omitempty"`
	Expires       time.Time `json:"expires"`
	// ClaimURL is where to take it. Present whether or not auto-accept is on,
	// because an operator who declines the auto path still needs the address.
	ClaimURL string `json:"claim_url"`
	// AutoAccepted says the exchange already claimed it on their behalf
	// because they asked it to. When true the work is theirs as of this POST.
	AutoAccepted bool `json:"auto_accepted"`
	// WorkURL is where the job actually is, capability secret included in the
	// fragment, exactly as the claim endpoint returns it to a person.
	//
	// Only set on an auto-accepted offer, and it is what makes auto-accept
	// usable rather than merely true. Claiming mints the capability that
	// authorises reading the brief and submitting evidence; this path was
	// discarding it. The operator was told the work was theirs and handed a
	// ClaimURL that would now conflict — a seat held, a job it could not open,
	// and nothing in the payload admitting it. A fleet cannot ask a human what
	// happened, so an offer it cannot act on is the same as no offer at all.
	WorkURL string `json:"work_url,omitempty"`
}

// Dispatcher offers new work to operators who published an endpoint.
type Dispatcher struct {
	Board      *Board
	Capacities *Capacities
	BaseURL    string
	Now        func() time.Time
	// Client is the outbound HTTP client. Replaced in tests; in production it
	// carries a short timeout, because a slow endpoint must not hold a job out
	// of everyone else's queue.
	Client *http.Client
	// AllowPrivateHosts disables the address check. Only a test server on
	// loopback sets it; production never does, which is why it is a field
	// rather than an environment variable somebody could flip in a container.
	AllowPrivateHosts bool

	mu sync.Mutex
	// offered records job→operator so a retry or a re-post does not offer the
	// same work twice. An operator whose phone buzzes twice for one job stops
	// trusting the channel.
	offered map[string]map[string]bool
	// failures counts consecutive delivery failures per operator, so a dead
	// endpoint stops being retried forever.
	failures map[string]int
}

// maxWebhookFailures is how many consecutive failures before we stop trying.
//
// An endpoint that has refused ten offers in a row is not going to accept the
// eleventh, and each attempt costs a live job several seconds of nobody
// seeing it.
const maxWebhookFailures = 10

func (d *Dispatcher) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

func (d *Dispatcher) client() *http.Client {
	if d.Client != nil {
		return d.Client
	}
	return &http.Client{
		Timeout: 6 * time.Second,
		// Redirects are refused rather than followed. An endpoint that 302s to
		// somewhere internal is the standard way to turn a webhook into a
		// request forgery.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return fmt.Errorf("dispatch: refusing to follow a redirect")
		},
	}
}

// Announce offers one listing to every operator who qualifies for it.
//
// Called when work appears and when a seat frees up. It returns the number of
// endpoints that accepted, which is what the caller logs — a job announced to
// nine fleets and accepted by none is a supply problem worth seeing.
func (d *Dispatcher) Announce(ctx context.Context, l *Listing) int {
	if l == nil || !IsWork(l.Kind) || d.Capacities == nil || d.Board == nil {
		return 0
	}
	type target struct {
		worker string
		cap    Capacity
		miles  float64
	}
	var targets []target
	for worker, cap := range d.Capacities.All() {
		if cap.Webhook == "" || !cap.Accepting || !cap.Takes(l.Kind) {
			continue
		}
		// No position, no offers. The board lets an unpositioned person browse
		// everything because they are looking; a webhook is not looking, it
		// is receiving, and "in range" of an operator who is nowhere was
		// every job in the country.
		if !cap.Positioned() {
			continue
		}
		if !MeetsSkills(l.Skills, cap.Skills) {
			continue
		}
		if !InRange(l.LatE7, l.LonE7, cap.LatE7, cap.LonE7, cap.RangeMiles) {
			continue
		}
		if d.tooManyFailures(worker) || d.alreadyOffered(l.Job, worker) {
			continue
		}
		t := target{worker: worker, cap: cap}
		if HasPosition(l.LatE7, l.LonE7) && cap.Positioned() {
			t.miles = round1(MilesBetween(l.LatE7, l.LonE7, cap.LatE7, cap.LonE7))
		}
		targets = append(targets, t)
	}
	// Nearest first. When several fleets could take a job, the closest one
	// doing it is the outcome the buyer wanted anyway, and offering in
	// distance order makes that the default without a bidding round.
	sort.Slice(targets, func(i, j int) bool { return targets[i].miles < targets[j].miles })

	var accepted int
	for _, t := range targets {
		// Stop as soon as the job is full. Offering a taken job wastes the
		// operator's time and teaches them the offers are unreliable.
		if !d.Board.HasOpenSeat(l.Job) {
			break
		}
		if d.deliver(ctx, t.worker, t.cap, l, t.miles) {
			accepted++
		}
	}
	return accepted
}

func (d *Dispatcher) deliver(ctx context.Context, worker string, cap Capacity, l *Listing, miles float64) bool {
	d.markOffered(l.Job, worker)

	// Built from the public view, so a field this file forgets to strip is
	// one the board already withholds.
	pub := l.Public()
	off := Offer{
		Offer: fmt.Sprintf("%s:%s", l.Job, shortID(worker)),
		Job:   l.Job, Kind: l.Kind, Title: pub.Title,
		Area: pub.Area, AreaLat: pub.AreaLat, AreaLon: pub.AreaLon,
		DistanceMiles: miles, Skills: l.Skills,
		PayMinor: l.PayMinor, BonusMinor: l.BonusMinor,
		Currency: l.Currency, Tier: l.Tier, Expires: l.Expires,
		ClaimURL: d.BaseURL + "/v1/workers/claim/" + url.PathEscape(l.Job),
	}

	// Auto-accept claims first, then tells them. The other order — announce,
	// wait for a reply, then claim — loses the race against a person tapping
	// the same job on the board, and an operator told "this is yours" who then
	// finds it taken has been lied to.
	var claimed bool
	if cap.AutoAccept {
		if secret, _, err := d.Board.Claim(l.Job, worker); err == nil {
			claimed = true
			off.AutoAccepted = true
			// Same shape the claim endpoint hands a person: the secret rides
			// in the fragment. One format for both, so a fleet and a phone
			// consume the identical thing.
			off.WorkURL = d.BaseURL + "/w/" + url.PathEscape(l.Job) + "#" + secret
		} else {
			// Nothing to auto-accept means nothing worth sending: they asked
			// for work, not for a log of work they could not have.
			return false
		}
	}

	body, err := json.Marshal(off)
	if err != nil {
		return false
	}
	ok := d.post(ctx, cap, body)
	if !ok && claimed {
		// We took a seat on their behalf and could not tell them. Holding it
		// would strand the job for the full lease. Give it back.
		d.Board.GiveBack(l.Job, worker)
		return false
	}
	return ok
}

func (d *Dispatcher) post(ctx context.Context, cap Capacity, body []byte) bool {
	if !d.AllowPrivateHosts {
		if err := safeWebhookHost(cap.Webhook); err != nil {
			return false
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", cap.Webhook, bytes.NewReader(body))
	if err != nil {
		return false
	}
	ts := d.now().UTC().Format(time.RFC3339)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "lamdis-exchange/1")
	req.Header.Set("X-Lamdis-Timestamp", ts)
	// Sign timestamp and body together. Signing only the body would let
	// anyone who captured one offer replay it forever.
	mac := hmac.New(sha256.New, []byte(cap.WebhookSecret))
	mac.Write([]byte(ts))
	mac.Write([]byte("\n"))
	mac.Write(body)
	req.Header.Set("X-Lamdis-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))

	resp, err := d.client().Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// safeWebhookHost refuses endpoints that resolve inside the network.
//
// The exchange makes an outbound request to an address a stranger chose. Left
// unchecked that is a request forgery primitive pointed at our own metadata
// service, and the fact that we require https:// stops none of it — a name can
// have a TLS certificate and still resolve to 169.254.169.254.
func safeWebhookHost(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("dispatch: endpoint must be an https url")
	}
	host := u.Hostname()
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("dispatch: cannot resolve %s", host)
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
			ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("dispatch: %s resolves to a private address", host)
		}
	}
	return nil
}

func (d *Dispatcher) alreadyOffered(job, worker string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.offered[job][worker]
}

func (d *Dispatcher) markOffered(job, worker string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.offered == nil {
		d.offered = map[string]map[string]bool{}
	}
	if d.offered[job] == nil {
		d.offered[job] = map[string]bool{}
	}
	d.offered[job][worker] = true
}

func (d *Dispatcher) tooManyFailures(worker string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.failures[worker] >= maxWebhookFailures
}

// Delivered records the outcome so a dead endpoint eventually stops costing
// every new job several seconds.
func (d *Dispatcher) Delivered(worker string, ok bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.failures == nil {
		d.failures = map[string]int{}
	}
	if ok {
		delete(d.failures, worker)
		return
	}
	d.failures[worker]++
}

func shortID(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
