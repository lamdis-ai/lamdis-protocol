package exchange

// The register of who would work where, before anybody has an account.
//
// The exchange had no way to record supply intent. An operator could set a
// capacity, but only after signing in, and the bootstrap loop clusters
// capacities to decide where to post real work — so with no capacities on file
// it posts nowhere, and a visitor who arrives, sees nothing they can take and
// leaves is gone with nothing able to reach them.
//
// This is the smallest thing that fixes that ordering. It takes an email, a
// coarse position, what somebody can do and how far they will travel, from
// anyone, with no account. It promises exactly one thing in return: they hear
// first when paid work appears near them. It is not a job, it is not a queue
// position, and nothing here implies either.
//
// What it stores is deliberately thin. Coordinates are rounded to two decimal
// places — about a kilometre, the same grain the open board publishes — before
// they are written, so a street address cannot be reconstructed from this file
// even by whoever holds it. The free-text place is whatever somebody typed and
// is never published below the threshold in coverageReport.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
)

// Coverage is one person saying where they work and what they can do.
type Coverage struct {
	// Email is how they are told when work appears. It is the only
	// identifying thing here and it is never published.
	Email string `json:"email"`
	// LatE7 and LonE7 are coarse: rounded on the way in, never stored finer.
	// Zero means they gave a place name and not a position, which is honest
	// and less useful — nothing can be posted to a point we do not have.
	LatE7 int64 `json:"lat_e7,omitempty"`
	LonE7 int64 `json:"lon_e7,omitempty"`
	// Place is the town or postcode they typed, for reading rather than
	// matching. Nothing geocodes it.
	Place string `json:"place,omitempty"`
	// Skills are what they say they can do, from the same catalogue an
	// operator's capacity uses.
	Skills []api.Skill `json:"skills,omitempty"`
	// RangeMiles is how far they will travel, clamped to the same ceiling
	// api.Capacities.Set clamps a real capacity to.
	RangeMiles int `json:"range_miles"`
	// At is when they registered.
	At time.Time `json:"at"`
	// LastSent bounds how often they hear from us, the same way a Watch does.
	LastSent time.Time `json:"last_sent,omitempty"`
}

// MaxCoverageRangeMiles is the furthest anybody may say they will travel. The
// same ceiling api.Capacities.Set applies, because this becomes a capacity the
// day they sign in and a promise it would then clamp is not a promise.
const MaxCoverageRangeMiles = 60

// DefaultCoverageRangeMiles matches api.DefaultCapacity.
const DefaultCoverageRangeMiles = 12

// CoverageMax is how many entries the register will hold at all. A file-backed
// list that anybody may append to needs a ceiling somewhere.
const CoverageMax = 20000

// Coverages is the register, backed by a file beside the rest of the state.
type Coverages struct {
	mu   sync.Mutex
	all  []*Coverage
	path string
	// seen rate-limits by caller address, the same shape guestAllowed uses.
	seen map[string][]time.Time
	Now  func() time.Time
}

// NewCoverages loads the register, or starts empty.
func NewCoverages(dir string) *Coverages {
	c := &Coverages{seen: map[string][]time.Time{}}
	if dir == "" {
		return c
	}
	c.path = filepath.Join(dir, "coverage.json")
	if b, err := os.ReadFile(c.path); err == nil {
		json.Unmarshal(b, &c.all)
	}
	return c
}

func (cs *Coverages) now() time.Time {
	if cs.Now != nil {
		return cs.Now()
	}
	return time.Now()
}

func (cs *Coverages) saveLocked() {
	if cs.path == "" {
		return
	}
	b, err := json.Marshal(cs.all)
	if err != nil {
		return
	}
	tmp := cs.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		log.Printf("coverage: could not write the register: %v", err)
		return
	}
	if err := os.Rename(tmp, cs.path); err != nil {
		log.Printf("coverage: could not replace the register: %v", err)
	}
}

// CoveragePerHour is how many registrations one address may make in an hour.
// Registering is free and unauthenticated, so it is bounded the way anonymous
// posting is.
const CoveragePerHour = 6

// allow reports whether this caller may register again.
func (cs *Coverages) allow(ip string, now time.Time) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if len(cs.all) >= CoverageMax {
		return fmt.Errorf("the register is full; write to us instead")
	}
	if cs.seen == nil {
		cs.seen = map[string][]time.Time{}
	}
	recent := cs.seen[ip][:0]
	for _, t := range cs.seen[ip] {
		if now.Sub(t) < time.Hour {
			recent = append(recent, t)
		}
	}
	if len(recent) >= CoveragePerHour {
		cs.seen[ip] = recent
		return fmt.Errorf("that is enough registrations from one place for an hour")
	}
	cs.seen[ip] = append(recent, now)
	return nil
}

// Add records somebody, or updates what they said last time.
//
// Keyed on the email: somebody who moves, or who adds a skill, has one entry
// and not two. Everything a caller can set is clamped or dropped here, which
// is the only place it is done.
func (cs *Coverages) Add(in Coverage) (Coverage, error) {
	email := strings.TrimSpace(strings.ToLower(in.Email))
	if !looksLikeEmail(email) {
		return Coverage{}, fmt.Errorf("that does not look like an email address")
	}
	c := Coverage{
		Email:      email,
		Place:      clip(strings.TrimSpace(in.Place), 80),
		Skills:     api.NormalizeSkills(in.Skills),
		RangeMiles: in.RangeMiles,
		At:         cs.now(),
	}
	if c.RangeMiles < 1 {
		c.RangeMiles = DefaultCoverageRangeMiles
	}
	if c.RangeMiles > MaxCoverageRangeMiles {
		c.RangeMiles = MaxCoverageRangeMiles
	}
	// Coarsened before it is stored, not on the way out. A file that never
	// held the precise point cannot leak it.
	if api.HasPosition(in.LatE7, in.LonE7) {
		c.LatE7, c.LonE7 = api.CoarseE7(in.LatE7), api.CoarseE7(in.LonE7)
	}
	if c.Place == "" && !api.HasPosition(c.LatE7, c.LonE7) {
		return Coverage{}, fmt.Errorf("say roughly where you work: share your location, or type a town or postcode")
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for i, x := range cs.all {
		if x.Email == email {
			// Keep the clock on alerts across an edit, so re-registering is
			// not a way to be emailed again straight away.
			c.LastSent = x.LastSent
			cs.all[i] = &c
			cs.saveLocked()
			return c, nil
		}
	}
	if len(cs.all) >= CoverageMax {
		return Coverage{}, fmt.Errorf("the register is full; write to us instead")
	}
	cs.all = append(cs.all, &c)
	cs.saveLocked()
	return c, nil
}

// Forget removes somebody at their own request.
func (cs *Coverages) Forget(email string) bool {
	email = strings.TrimSpace(strings.ToLower(email))
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for i, x := range cs.all {
		if x.Email == email {
			cs.all = append(cs.all[:i], cs.all[i+1:]...)
			cs.saveLocked()
			return true
		}
	}
	return false
}

// Count is how many people are registered.
func (cs *Coverages) Count() int {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return len(cs.all)
}

// Positioned returns the entries that carry a coarse position, as capacities.
//
// The bootstrap loop clusters capacities to decide where to post; an entry
// here is the same three facts a capacity carries — a point, a range, and what
// somebody will take — so it is handed over in that shape rather than teaching
// the clusterer a second type. Entries with only a place name are left out:
// nothing can be posted to a point nobody gave us.
func (cs *Coverages) Positioned() []api.Capacity {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	out := make([]api.Capacity, 0, len(cs.all))
	for _, x := range cs.all {
		if !api.HasPosition(x.LatE7, x.LonE7) {
			continue
		}
		out = append(out, api.Capacity{
			Accepting: true, MaxConcurrent: 1,
			RangeMiles: x.RangeMiles, Skills: x.Skills,
			LatE7: x.LatE7, LonE7: x.LonE7,
		})
	}
	return out
}

// Due lists who should be told about a job: registered, in range, and
// qualified, respecting how recently they last heard from us.
//
// The same rule Watches.Due applies, for the same reason. Somebody with no
// account cannot mute us from a console, so the interval doing the work here
// matters more, not less.
func (cs *Coverages) Due(l *api.Listing, now time.Time) []Coverage {
	if l == nil {
		return nil
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	var out []Coverage
	for _, x := range cs.all {
		if !api.HasPosition(x.LatE7, x.LonE7) {
			continue
		}
		if !api.MeetsSkills(l.Skills, x.Skills) {
			continue
		}
		if !api.InRange(l.LatE7, l.LonE7, x.LatE7, x.LonE7, x.RangeMiles) {
			continue
		}
		if now.Sub(x.LastSent) < MinBetweenAlerts {
			continue
		}
		x.LastSent = now
		out = append(out, *x)
	}
	if len(out) > 0 {
		cs.saveLocked()
	}
	return out
}

// CoverageArea is one coarse cell of the map and how thick supply is in it.
//
// Counts are buckets, never figures. One person who registered from their
// kitchen is a person; "1 operator at 42.33, -83.05" is close to a name.
type CoverageArea struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
	// Place is published only once enough people in the cell have named one,
	// because a single postcode typed into a public feed is an address.
	Place string `json:"place,omitempty"`
	// Operators is how many have registered a capacity here; Interested is
	// how many have registered intent without an account.
	Operators  string `json:"operators"`
	Interested string `json:"interested"`
	// AtLeast is the floor of the combined bucket, for anything that has to
	// sort or threshold. It is a floor, not a count.
	AtLeast int `json:"at_least"`
}

// coverageBuckets is the ladder. Below five is one bucket on purpose: it is
// the range where an exact number identifies somebody.
var coverageBuckets = []struct {
	Floor int
	Label string
}{
	{100, "100 or more"},
	{25, "25 to 99"},
	{10, "10 to 24"},
	{5, "5 to 9"},
	{1, "fewer than 5"},
	{0, "none"},
}

// coverageBucket says how many without saying exactly.
func coverageBucket(n int) (string, int) {
	for _, b := range coverageBuckets {
		if n >= b.Floor {
			if b.Floor == 1 {
				return b.Label, 1
			}
			return b.Label, b.Floor
		}
	}
	return "none", 0
}

// PlaceThreshold is how many people must have named a cell before the name is
// published with it.
const PlaceThreshold = 5

// Areas reports supply per coarse cell, from both the register and the
// capacities operators have actually set.
func (cs *Coverages) Areas(ops map[string]api.Capacity) []CoverageArea {
	type cell struct {
		latE7, lonE7 int64
		operators    int
		interested   int
		places       map[string]int
	}
	cells := map[string]*cell{}
	at := func(latE7, lonE7 int64) *cell {
		latE7, lonE7 = api.CoarseE7(latE7), api.CoarseE7(lonE7)
		k := fmt.Sprintf("%d,%d", latE7, lonE7)
		c, ok := cells[k]
		if !ok {
			c = &cell{latE7: latE7, lonE7: lonE7, places: map[string]int{}}
			cells[k] = c
		}
		return c
	}
	for _, c := range ops {
		if !api.HasPosition(c.LatE7, c.LonE7) {
			continue
		}
		at(c.LatE7, c.LonE7).operators++
	}
	cs.mu.Lock()
	for _, x := range cs.all {
		if !api.HasPosition(x.LatE7, x.LonE7) {
			continue
		}
		c := at(x.LatE7, x.LonE7)
		c.interested++
		if x.Place != "" {
			c.places[x.Place]++
		}
	}
	cs.mu.Unlock()

	out := make([]CoverageArea, 0, len(cells))
	for _, c := range cells {
		opsLabel, _ := coverageBucket(c.operators)
		intLabel, _ := coverageBucket(c.interested)
		_, floor := coverageBucket(c.operators + c.interested)
		a := CoverageArea{
			Lat: api.Deg(c.latE7), Lon: api.Deg(c.lonE7),
			Operators: opsLabel, Interested: intLabel, AtLeast: floor,
		}
		if c.operators+c.interested >= PlaceThreshold {
			best, n := "", 0
			for p, k := range c.places {
				if k > n || (k == n && p < best) {
					best, n = p, k
				}
			}
			a.Place = best
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AtLeast != out[j].AtLeast {
			return out[i].AtLeast > out[j].AtLeast
		}
		if out[i].Lat != out[j].Lat {
			return out[i].Lat < out[j].Lat
		}
		return out[i].Lon < out[j].Lon
	})
	return out
}

// CoverageServer is the public register: one route to join it, one to read it
// back in aggregate, and one to leave.
type CoverageServer struct {
	Server *Server
}

func (cv *CoverageServer) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/coverage", cv.handleAdd)
	mux.HandleFunc("GET /v1/coverage", cv.handleReport)
	mux.HandleFunc("GET /coverage/stop", cv.handleStop)
	// The page a marketing link lands on. It asks the same question the board
	// asks when the board has nothing paid on it.
	mux.HandleFunc("GET /coverage", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Referrer-Policy", "no-referrer")
		fmt.Fprint(w, api.CoveragePage())
	})
}

// coverageRequest is what the form sends. Coordinates arrive in the stored
// integer form the rest of the API uses.
type coverageRequest struct {
	Email      string      `json:"email"`
	Place      string      `json:"place"`
	LatE7      int64       `json:"lat_e7"`
	LonE7      int64       `json:"lon_e7"`
	Skills     []api.Skill `json:"skills"`
	RangeMiles int         `json:"range_miles"`
}

func (cv *CoverageServer) handleAdd(w http.ResponseWriter, r *http.Request) {
	s := cv.Server
	if s.Coverage == nil {
		writeError(w, http.StatusServiceUnavailable, "the register is not switched on here")
		return
	}
	if err := s.Coverage.allow(callerIP(r), s.now()); err != nil {
		writeError(w, http.StatusTooManyRequests, err.Error())
		return
	}
	var req coverageRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<15)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "could not read that")
		return
	}
	c, err := s.Coverage.Add(Coverage{
		Email: req.Email, Place: req.Place,
		LatE7: req.LatE7, LonE7: req.LonE7,
		Skills: req.Skills, RangeMiles: req.RangeMiles,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Say exactly what was kept and exactly what was promised. Anything
	// warmer than this would be describing work that does not exist.
	out := map[string]any{
		"registered":  true,
		"place":       c.Place,
		"range_miles": c.RangeMiles,
		"skills":      c.Skills,
		"positioned":  api.HasPosition(c.LatE7, c.LonE7),
		"email_ready": s.Mail != nil,
		"note": "You are on the register, not on a job. There is no paid work " +
			"waiting for you. When work is posted that you could take, you are " +
			"emailed — at most once every " + hours(MinBetweenAlerts) + ".",
	}
	if !api.HasPosition(c.LatE7, c.LonE7) {
		out["note"] = out["note"].(string) + " You gave a place and not a position, " +
			"so nothing here can measure a distance from you; share a location " +
			"to be matched by range."
	}
	if s.Mail == nil {
		out["note"] = out["note"].(string) + " Email is not configured on this " +
			"exchange yet, so this is recorded and cannot yet be sent."
	}
	out["stop"] = strings.TrimSuffix(s.BaseURL, "/") + "/coverage/stop?e=" +
		urlQuery(c.Email) + "&t=" + s.CoverageToken(c.Email)
	writeJSONResponse(w, out)
}

func (cv *CoverageServer) handleReport(w http.ResponseWriter, r *http.Request) {
	s := cv.Server
	var areas []CoverageArea
	operators, interested := 0, 0
	ops := map[string]api.Capacity{}
	if s.Capacities != nil {
		ops = s.Capacities.All()
	}
	for _, c := range ops {
		if api.HasPosition(c.LatE7, c.LonE7) {
			operators++
		}
	}
	if s.Coverage != nil {
		areas = s.Coverage.Areas(ops)
		interested = s.Coverage.Count()
	}
	opsLabel, _ := coverageBucket(operators)
	intLabel, _ := coverageBucket(interested)
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSONResponse(w, map[string]any{
		"areas":      areas,
		"operators":  opsLabel,
		"interested": intLabel,
		"note": "Where supply is, coarsely. Points are rounded to two decimal " +
			"places of a degree, about a kilometre, and counts are buckets — " +
			"under five is never a figure. Nobody is named.",
	})
}

// handleStop is the link in every alert. It is a GET because it is clicked
// from an email, and it is capability-gated so one address cannot remove
// another.
func (cv *CoverageServer) handleStop(w http.ResponseWriter, r *http.Request) {
	s := cv.Server
	email := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("e")))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if email == "" || r.URL.Query().Get("t") != s.CoverageToken(email) || s.Coverage == nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, api.GuestNotice("That link is not valid",
			"Use the link at the bottom of the email we sent you."))
		return
	}
	s.Coverage.Forget(email)
	fmt.Fprint(w, api.GuestNotice("Removed",
		"You are off the register and will hear nothing further. Nothing of "+
			"yours is kept."))
}

// CoverageToken is the capability that lets somebody remove themselves.
// Derived from the exchange key, so nothing is stored and a token for an
// address nobody registered is as good as random.
func (s *Server) CoverageToken(email string) string {
	return s.BuyerToken("coverage:" + strings.ToLower(strings.TrimSpace(email)))
}

// callerIP is the address a request came from, near enough to rate-limit on.
//
// Shared by anonymous posting and the coverage register so the two cannot
// disagree about what one caller is. It is an address and not an identity;
// see clientOf in the api package for the same caveat.
func callerIP(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if i := strings.IndexByte(ip, ','); i >= 0 {
		ip = ip[:i]
	}
	ip = strings.TrimSpace(ip)
	if ip == "" {
		ip = r.RemoteAddr
		if i := strings.LastIndexByte(ip, ':'); i >= 0 {
			ip = ip[:i]
		}
	}
	return ip
}

// AlertCoverage tells the people on the register that work they could take
// exists. Best effort, over the same mailer everything else uses.
func (s *Server) AlertCoverage(ctx context.Context, l *api.Listing) int {
	if s.Mail == nil || s.Coverage == nil || l == nil || !api.IsWork(l.Kind) || l.Practice {
		return 0
	}
	due := s.Coverage.Due(l, s.now())
	if len(due) == 0 {
		return 0
	}
	base := trimSlash(s.BaseURL)
	subject := "Work near you: " + l.Title
	sent := 0
	for _, c := range due {
		body := fmt.Sprintf(
			"%s\n\n%s\n\nPays %s. %s\n\nYou are getting this because you told "+
				"this exchange you would work near here. To take it you need an "+
				"account, which is an email and a code: %s/signin\n\nThe job: "+
				"%s/j/%s\n\nTo come off the register: %s/coverage/stop?e=%s&t=%s\n",
			l.Title, orDash(l.Area), money(l.PayMinor, l.Currency),
			orDash(l.Window()), base, base, l.Job,
			base, urlQuery(c.Email), s.CoverageToken(c.Email))
		if err := s.Mail.Send(ctx, c.Email, subject, body); err != nil {
			log.Printf("coverage: could not reach a registered address: %v", err)
			continue
		}
		sent++
	}
	if sent > 0 {
		log.Printf("coverage: told %d registered person(s) about %s", sent, l.Job)
	}
	return sent
}

// looksLikeEmail is the weakest check that still refuses the mistakes people
// actually make. Validating an address properly means sending to it.
func looksLikeEmail(s string) bool {
	at := strings.IndexByte(s, '@')
	if at < 1 || at == len(s)-1 || strings.ContainsAny(s, " \t\r\n,;") {
		return false
	}
	dom := s[at+1:]
	return strings.Contains(dom, ".") && !strings.HasPrefix(dom, ".") &&
		!strings.HasSuffix(dom, ".") && strings.IndexByte(dom, '@') < 0
}

// urlQuery escapes a value for a query string.
func urlQuery(s string) string { return url.QueryEscape(s) }

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// hours renders an interval the way the copy says it, so a change to
// MinBetweenAlerts changes the sentence rather than making it a lie.
func hours(d time.Duration) string {
	h := int(d.Hours())
	names := []string{"zero", "one", "two", "three", "four", "five", "six",
		"seven", "eight", "nine", "ten", "eleven", "twelve"}
	if h == 1 {
		return "hour"
	}
	if h < len(names) {
		return names[h] + " hours"
	}
	return fmt.Sprintf("%d hours", h)
}
