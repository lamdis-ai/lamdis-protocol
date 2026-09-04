// Package bootstrap gets the marketplace going by being its first buyer.
//
// An empty board teaches an operator to leave, and a board with fake work on
// it teaches them the board is fake. This loop does the third thing: it
// spends a small, hard-capped house budget on real observations near where
// signed-in operators actually are, chosen from public map data, and keeps
// what they find as the exchange's first dataset. Everything it posts goes
// through the same gate as any buyer's job; the only thing it can spend is
// the house budget, and it cannot raise it.
package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
)

// Market is what the loop needs from the exchange. Narrow on purpose: the
// adapter in exchange_adapter.go is the only thing that implements it against
// the real server, and a test implements it with a board and a counter.
type Market interface {
	// Operators is every operator's capacity, positioned or not.
	Operators() map[string]api.Capacity
	// Open is every open listing, with Owner intact.
	Open() []*api.Listing
	// Get is one listing, open or not.
	Get(job string) (*api.Listing, bool)
	// PostFunded escrows and lists, applying every check a buyer's job gets.
	PostFunded(ctx context.Context, buyer string, l *api.Listing) error
	// Balance is what a principal has available, in minor units.
	Balance(ctx context.Context, principal string) (int64, error)
	// Topup moves money in from outside. The loop is the only caller, and
	// the budget is the only bound.
	Topup(ctx context.Context, key, principal string, amountMinor int64) error
	// Widen enlarges an open job's geofence.
	Widen(job string, radiusM int64) bool
	// Alert tells operators who could take a job that it exists, over the
	// exchange's own alert path.
	Alert(l *api.Listing)
	// Interested is where people have said they would work without having
	// registered a capacity, in the shape of one. Read only when there are no
	// capacities to cluster at all.
	Interested() []api.Capacity
}

// The knobs, in one place.
const (
	// MaxOpenPerCluster is how many house jobs may be open near one group
	// of operators at once.
	MaxOpenPerCluster = 5
	// JobsPerCycle is the most the loop posts for one cluster in one pass.
	JobsPerCycle = 3
	// WidenAfter is how long a job sits untaken before its area is widened.
	WidenAfter = 24 * time.Hour
	// WidenFactor is how much, once.
	WidenFactor = 2
	// DefaultEvery is the cycle interval.
	DefaultEvery = 30 * time.Minute
	// maxQueryMiles caps how far one Overpass query reaches.
	maxQueryMiles = 10
)

// Config is what a deployment sets.
type Config struct {
	// Enabled is off unless somebody turned it on.
	Enabled bool
	// BudgetMinor is the most the house will ever draw. Required when on.
	BudgetMinor int64
	// DataDir is where spend and findings persist. Required when on.
	DataDir string
	Every   time.Duration
	Places  PlaceSource
	Market  Market
	Now     func() time.Time
	Logf    func(format string, args ...any)
}

// Loop is the running thing.
type Loop struct {
	cfg      Config
	mu       sync.Mutex
	st       *state
	findings *Findings
	lastErr  string
}

// New loads state and findings. It refuses a configuration that is switched
// on without the two things the promises depend on.
func New(cfg Config) (*Loop, error) {
	if cfg.Enabled {
		if cfg.BudgetMinor <= 0 {
			return nil, fmt.Errorf("bootstrap: LAMDIS_HOUSE_BUDGET_MINOR must be set to turn the loop on")
		}
		if cfg.DataDir == "" {
			return nil, fmt.Errorf("bootstrap: a data dir is required, or the budget cannot be honoured across restarts")
		}
		if cfg.Places == nil {
			return nil, fmt.Errorf("bootstrap: no place source")
		}
	}
	if cfg.Every <= 0 {
		cfg.Every = DefaultEvery
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logf == nil {
		cfg.Logf = log.Printf
	}
	st, err := loadState(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: reading state: %w", err)
	}
	f, err := openFindings(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: reading findings: %w", err)
	}
	return &Loop{cfg: cfg, st: st, findings: f}, nil
}

// Enabled reports whether the loop will run.
func (l *Loop) Enabled() bool { return l.cfg.Enabled }

// Start runs cycles on a timer until the context ends. Off means nothing
// starts, and the endpoints still answer.
func (l *Loop) Start(ctx context.Context) {
	if !l.cfg.Enabled || l.cfg.Market == nil {
		return
	}
	go func() {
		l.runOnce(ctx)
		t := time.NewTicker(l.cfg.Every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				l.runOnce(ctx)
			}
		}
	}()
}

// runOnce is one cycle that cannot take the server down with it.
func (l *Loop) runOnce(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			l.cfg.Logf("bootstrap: cycle panicked and was skipped: %v", r)
		}
	}()
	if _, err := l.Cycle(ctx); err != nil {
		l.cfg.Logf("bootstrap: cycle skipped: %v", err)
	}
}

// Report is what one cycle did.
type Report struct {
	Clusters, Posted, Widened, Skipped int
	BudgetExhausted                    bool
}

// Cycle does one pass: widen what has sat, then post near each group of
// operators that has nothing to do.
//
// An error means the cycle stopped early and the rest was skipped; whatever
// was posted before the error stands, because it was funded and listed.
func (l *Loop) Cycle(ctx context.Context) (Report, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.cfg.Enabled || l.cfg.Market == nil {
		return Report{}, nil
	}
	now := l.cfg.Now()
	m := l.cfg.Market
	var rep Report
	l.lastErr = ""
	defer func() {
		l.st.LastRun = now
		l.st.Clusters = rep.Clusters
		if err := l.st.save(l.cfg.DataDir, now); err != nil {
			l.cfg.Logf("bootstrap: could not save state: %v", err)
		}
		l.cfg.Logf("bootstrap: clusters=%d posted=%d widened=%d skipped=%d budget_left=%d findings=%d%s",
			rep.Clusters, rep.Posted, rep.Widened, rep.Skipped,
			l.cfg.BudgetMinor-l.st.ToppedUpMinor, l.findings.Count(), l.lastErrSuffix())
	}()

	clusters := Clusters(m.Operators())
	if len(clusters) == 0 {
		// Nobody has registered a capacity, so clustering capacities decides
		// nothing and the loop posts nowhere — which is the state that keeps
		// an empty exchange empty. The only other evidence of where people
		// are is who told us where they work, so the first jobs go there.
		// Superseded the moment one real capacity exists: a capacity is a
		// promise to take work, and this is only an intention to.
		clusters = ClustersFromInterest(m.Interested())
		if len(clusters) > 0 {
			l.cfg.Logf("bootstrap: no capacities registered; aiming at %d area(s) "+
				"from the coverage register", len(clusters))
		}
	}
	rep.Clusters = len(clusters)
	open := m.Open()
	rep.Widened = l.widenStale(open, now)
	l.markFilled(open)

	for _, c := range clusters {
		if !l.clusterWantsWork(c, open) {
			rep.Skipped++
			continue
		}
		houseOpen := l.houseOpenIn(c, open)
		want := JobsPerCycle
		if left := MaxOpenPerCluster - houseOpen; left < want {
			want = left
		}
		if want <= 0 {
			rep.Skipped++
			continue
		}
		radius := c.RangeMiles
		if radius > maxQueryMiles {
			radius = maxQueryMiles
		}
		places, err := l.cfg.Places.Nearby(ctx, c.LatE7, c.LonE7, radius*1609)
		if err != nil {
			l.lastErr = err.Error()
			return rep, err
		}
		posted, exhausted, err := l.postFor(ctx, c, places, want, now)
		rep.Posted += posted
		if err != nil {
			l.lastErr = err.Error()
			return rep, err
		}
		if exhausted {
			rep.BudgetExhausted = true
			break
		}
	}
	return rep, nil
}

func (l *Loop) lastErrSuffix() string {
	if l.lastErr == "" {
		return ""
	}
	return " error=" + l.lastErr
}

// clusterWantsWork: nobody else's work is open in reach, and no operator
// there has anything to do.
func (l *Loop) clusterWantsWork(c Cluster, open []*api.Listing) bool {
	for _, x := range open {
		if !api.IsWork(x.Kind) || x.Practice || x.Owner == HousePrincipal {
			continue
		}
		if c.Reaches(x.LatE7, x.LonE7) {
			return false
		}
	}
	return true
}

func (l *Loop) houseOpenIn(c Cluster, open []*api.Listing) int {
	n := 0
	for _, x := range open {
		if x.Owner == HousePrincipal && c.Reaches(x.LatE7, x.LonE7) {
			n++
		}
	}
	return n
}

// postFor puts up to want jobs near a cluster, from places nobody has been
// sent to lately.
func (l *Loop) postFor(ctx context.Context, c Cluster, places []Place, want int, now time.Time) (posted int, exhausted bool, err error) {
	area := commonCity(places)
	for _, p := range places {
		if posted >= want {
			break
		}
		if !Eligible(p) || l.st.seen(p.ID, now) || !c.Reaches(p.LatE7, p.LonE7) {
			continue
		}
		shape := shapeFor(p, l.st.Posted)
		job := fmt.Sprintf("house-%s-%d", shape, now.UnixNano()+int64(posted))
		listing := Compose(job, p, shape, area, now)
		if listing.Kind != api.KindObserve {
			// Belt and braces: the loop never asks anybody to do anything.
			continue
		}
		cost := (listing.PayMinor + listing.BonusMinor + listing.ExpenseCapMinor) * int64(listing.Slots)
		if listing.AttemptMinor > listing.PayMinor {
			cost = listing.AttemptMinor * int64(listing.Slots)
		}
		ok, ferr := l.ensureFunds(ctx, cost)
		if ferr != nil {
			return posted, false, ferr
		}
		if !ok {
			return posted, true, nil
		}
		if perr := l.cfg.Market.PostFunded(ctx, HousePrincipal, listing); perr != nil {
			// Refused — by screening, by the board, by the ledger. Say so
			// and move to the next place; the money was not moved.
			l.cfg.Logf("bootstrap: %s refused: %v", job, perr)
			continue
		}
		l.st.Seen[p.ID] = now
		l.st.Posted++
		l.st.Jobs[job] = &jobRec{
			Place: p.ID, Shape: shape, Question: listing.Title, Area: listing.Area,
			Cluster: c.Key, Posted: now, RadiusM: listing.RadiusM,
			LatE7: p.LatE7, LonE7: p.LonE7, Expires: listing.Expires,
		}
		posted++
	}
	return posted, false, nil
}

// ensureFunds makes sure the house balance covers one job, drawing on the
// budget only for the shortfall and never past it.
//
// The state is written before the ledger moves: a crash between the two
// leaves budget counted and unspent, which is the safe side. The ledger key
// is the cumulative figure, so a replay after that crash moves nothing twice.
func (l *Loop) ensureFunds(ctx context.Context, need int64) (bool, error) {
	bal, err := l.cfg.Market.Balance(ctx, HousePrincipal)
	if err != nil {
		return false, err
	}
	if bal >= need {
		return true, nil
	}
	short := need - bal
	if l.st.ToppedUpMinor+short > l.cfg.BudgetMinor {
		return false, nil
	}
	l.st.ToppedUpMinor += short
	if err := l.st.save(l.cfg.DataDir, l.cfg.Now()); err != nil {
		l.st.ToppedUpMinor -= short
		return false, err
	}
	key := fmt.Sprintf("house-topup:%d", l.st.ToppedUpMinor)
	if err := l.cfg.Market.Topup(ctx, key, HousePrincipal, short); err != nil {
		return false, err
	}
	return true, nil
}

// widenStale doubles the area of any house job untaken for a day, once.
func (l *Loop) widenStale(open []*api.Listing, now time.Time) int {
	n := 0
	for _, x := range open {
		rec, ok := l.st.Jobs[x.Job]
		if !ok || rec.Widened || x.Taken > 0 || now.Sub(x.Posted) < WidenAfter {
			continue
		}
		if l.cfg.Market.Widen(x.Job, x.RadiusM*WidenFactor) {
			rec.Widened = true
			rec.RadiusM = x.RadiusM * WidenFactor
			n++
			// Same signal a new job sends, to the same people, over the same
			// rate-limited path.
			l.cfg.Market.Alert(x)
		}
	}
	return n
}

// markFilled notes which house jobs somebody has taken.
func (l *Loop) markFilled(open []*api.Listing) {
	for job, rec := range l.st.Jobs {
		if rec.Filled {
			continue
		}
		if x, ok := l.cfg.Market.Get(job); ok && x.Taken > 0 {
			rec.Filled = true
		}
	}
	_ = open
}

// RecordAccepted is the hook the exchange calls when a submission passes
// verification. Anything that is not a house job is ignored.
func (l *Loop) RecordAccepted(x *api.Listing, sub api.Submission) {
	if x == nil || x.Owner != HousePrincipal || !sub.Verified {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	f := Finding{
		Job: x.Job, Question: x.Title, Area: x.Area,
		At:  sub.At,
		Lat: coarse(x.LatE7), Lon: coarse(x.LonE7),
	}
	if f.At.IsZero() {
		f.At = l.cfg.Now()
	}
	if sub.Finding {
		f.Verdict = "yes"
	} else {
		f.Verdict = "no"
	}
	if len(sub.Artifacts) > 0 {
		f.PhotoSHA = sub.Artifacts[0].SHA256
	}
	if rec, ok := l.st.Jobs[x.Job]; ok {
		f.Place, f.Shape = rec.Place, rec.Shape
		rec.Filled, rec.Settled = true, true
		if err := l.st.save(l.cfg.DataDir, l.cfg.Now()); err != nil {
			l.cfg.Logf("bootstrap: could not save state: %v", err)
		}
	}
	if err := l.findings.Add(f); err != nil {
		l.cfg.Logf("bootstrap: could not write finding for %s: %v", x.Job, err)
	}
}

// Status is what /v1/bootstrap reports.
type Status struct {
	Enabled       bool      `json:"enabled"`
	BudgetMinor   int64     `json:"budget_minor"`
	ToppedUpMinor int64     `json:"topped_up_minor"`
	SpentMinor    int64     `json:"spent_minor"`
	BalanceMinor  int64     `json:"balance_minor"`
	Currency      string    `json:"currency"`
	Posted        int       `json:"jobs_posted"`
	Filled        int       `json:"jobs_filled"`
	Settled       int       `json:"jobs_settled"`
	Findings      int       `json:"findings"`
	LastRun       time.Time `json:"last_run,omitempty"`
	Clusters      int       `json:"clusters_seen"`
	EverySeconds  int       `json:"every_s"`
	LastError     string    `json:"last_error,omitempty"`
}

// Status reads the counters.
func (l *Loop) Status(ctx context.Context) Status {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := Status{
		Enabled: l.cfg.Enabled, BudgetMinor: l.cfg.BudgetMinor,
		ToppedUpMinor: l.st.ToppedUpMinor, Currency: "USD",
		Posted: l.st.Posted, Findings: l.findings.Count(),
		LastRun: l.st.LastRun, Clusters: l.st.Clusters,
		EverySeconds: int(l.cfg.Every.Seconds()), LastError: l.lastErr,
	}
	for _, rec := range l.st.Jobs {
		if rec.Filled {
			s.Filled++
		}
		if rec.Settled {
			s.Settled++
		}
	}
	if l.cfg.Market != nil {
		if bal, err := l.cfg.Market.Balance(ctx, HousePrincipal); err == nil {
			s.BalanceMinor = bal
			s.SpentMinor = l.st.ToppedUpMinor - bal
		}
	}
	return s
}

// Register mounts the two public, read-only endpoints.
func (l *Loop) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, l.Status(r.Context()))
	})
	mux.HandleFunc("GET /v1/findings", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"count":    l.findings.Count(),
			"findings": l.findings.Recent(1000),
			"note": "verified observations of storefronts, posted by the exchange " +
				"and answered by operators. Coordinates are rounded to about a kilometre.",
		})
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// commonCity is the city most places in a response name, for the job's area.
func commonCity(places []Place) string {
	count := map[string]int{}
	best, n := "", 0
	for _, p := range places {
		if p.City == "" {
			continue
		}
		count[p.City]++
		if count[p.City] > n {
			best, n = p.City, count[p.City]
		}
	}
	return best
}
