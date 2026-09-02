package bootstrap

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/exchange"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/ledger"
)

const (
	detroitLat = 423314000
	detroitLon = -830458000
)

// fixture serves the canned Overpass response.
type fixture struct {
	calls int
	err   error
}

func (f *fixture) Nearby(ctx context.Context, latE7, lonE7 int64, radiusM int) ([]Place, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	data, err := os.ReadFile(filepath.Join("testdata", "overpass.json"))
	if err != nil {
		return nil, err
	}
	return ParseOverpass(data)
}

// fake is a market with a real board and a counter for a ledger.
type fake struct {
	ops     map[string]api.Capacity
	board   *api.Board
	balance int64
	topups  []int64
	widened map[string]int64
	alerted []string
	refuse  func(*api.Listing) error
}

func newFake(now func() time.Time) *fake {
	b := api.NewBoard(api.NewCapabilities())
	b.Now = now
	return &fake{ops: map[string]api.Capacity{}, board: b, widened: map[string]int64{}}
}

func (f *fake) Operators() map[string]api.Capacity { return f.ops }
func (f *fake) Open() []*api.Listing               { return f.board.Listings() }
func (f *fake) Get(job string) (*api.Listing, bool) {
	return f.board.Get(job)
}
func (f *fake) PostFunded(ctx context.Context, buyer string, l *api.Listing) error {
	if f.refuse != nil {
		if err := f.refuse(l); err != nil {
			return err
		}
	}
	if ref := api.Screen(l.Title, l.Detail, l.Instructions, l.Deliverable); ref != nil {
		return ref
	}
	cost := l.PayMinor * int64(l.Slots)
	if f.balance < cost {
		return fmt.Errorf("insufficient funds")
	}
	f.balance -= cost
	l.Owner = buyer
	return f.board.Post(l)
}
func (f *fake) Balance(ctx context.Context, principal string) (int64, error) {
	return f.balance, nil
}
func (f *fake) Topup(ctx context.Context, key, principal string, amountMinor int64) error {
	f.topups = append(f.topups, amountMinor)
	f.balance += amountMinor
	return nil
}
func (f *fake) Widen(job string, radiusM int64) bool {
	f.widened[job] = radiusM
	return f.board.Widen(job, radiusM)
}
func (f *fake) Alert(l *api.Listing) { f.alerted = append(f.alerted, l.Job) }

func detroitOp(rangeMiles int) api.Capacity {
	return api.Capacity{Accepting: true, RangeMiles: rangeMiles, MaxConcurrent: 1,
		LatE7: detroitLat, LonE7: detroitLon}
}

func newLoop(t *testing.T, dir string, budget int64, m Market, now func() time.Time) *Loop {
	t.Helper()
	l, err := New(Config{
		Enabled: true, BudgetMinor: budget, DataDir: dir, Every: time.Hour,
		Places: &fixture{}, Market: m, Now: now,
		Logf: func(format string, args ...any) { t.Logf(format, args...) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestClustersFromCapacities(t *testing.T) {
	ops := map[string]api.Capacity{
		"a": detroitOp(12),
		"b": {Accepting: true, RangeMiles: 8, LatE7: detroitLat + 20000, LonE7: detroitLon + 20000}, // ~2 miles off
		"c": {Accepting: true, RangeMiles: 25, LatE7: 334484000, LonE7: -1120740000},                // Phoenix
		"d": {Accepting: false, RangeMiles: 12, LatE7: detroitLat, LonE7: detroitLon},               // switched off
		"e": {Accepting: true, RangeMiles: 12},                                                      // no position
		"f": {Accepting: true, RangeMiles: 12, Kinds: []string{api.KindDo}, LatE7: detroitLat, LonE7: detroitLon},
	}
	cs := Clusters(ops)
	if len(cs) != 2 {
		t.Fatalf("want 2 clusters, got %d: %+v", len(cs), cs)
	}
	var detroit *Cluster
	for i := range cs {
		if len(cs[i].Workers) == 2 {
			detroit = &cs[i]
		}
	}
	if detroit == nil {
		t.Fatalf("no two-operator cluster: %+v", cs)
	}
	if detroit.RangeMiles != 8 {
		t.Errorf("cluster range should be the smallest member's, got %d", detroit.RangeMiles)
	}
	if strings.Join(detroit.Workers, ",") != "a,b" {
		t.Errorf("workers = %v", detroit.Workers)
	}
	if !strings.HasPrefix(detroit.Key, "42.3") {
		t.Errorf("key = %q", detroit.Key)
	}
}

func TestJobsFromCannedOverpass(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	m := newFake(clock)
	m.ops["a"] = detroitOp(12)
	l := newLoop(t, t.TempDir(), 10000, m, clock)

	rep, err := l.Cycle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Clusters != 1 || rep.Posted != JobsPerCycle {
		t.Fatalf("report = %+v", rep)
	}
	open := m.board.Listings()
	if len(open) != JobsPerCycle {
		t.Fatalf("board has %d jobs", len(open))
	}
	seen := map[string]bool{}
	for _, x := range open {
		if x.Kind != api.KindObserve {
			t.Errorf("%s: kind %q — the loop must never post do-jobs", x.Job, x.Kind)
		}
		if !x.PostedByAgent || x.Owner != HousePrincipal || x.Tier != "V2" {
			t.Errorf("%s: posted_by_agent=%v owner=%q tier=%q", x.Job, x.PostedByAgent, x.Owner, x.Tier)
		}
		if x.RadiusM != JobRadiusM || !api.HasPosition(x.LatE7, x.LonE7) {
			t.Errorf("%s: no geofence", x.Job)
		}
		if x.Area != "Detroit" {
			t.Errorf("%s: area %q", x.Job, x.Area)
		}
		if got := x.Expires.Sub(x.Posted); got != JobTTL {
			t.Errorf("%s: ttl %v", x.Job, got)
		}
		if x.PayMinor < 400 || x.PayMinor > 800 || x.AttemptMinor != AttemptMinor {
			t.Errorf("%s: pay %d attempt %d", x.Job, x.PayMinor, x.AttemptMinor)
		}
		if !strings.Contains(x.Deliverable, "code") || !strings.Contains(x.Deliverable, "sign") &&
			!strings.Contains(x.Deliverable, "house number") {
			t.Errorf("%s: deliverable does not name the sign or number: %q", x.Job, x.Deliverable)
		}
		if x.Where == "" {
			t.Errorf("%s: no place", x.Job)
		}
		for _, bad := range []string{"Home Bakery", "Guest House", "Family Shelter", "Phoenix"} {
			if strings.Contains(x.Where, bad) || strings.Contains(x.Title, bad) {
				t.Errorf("%s: posted at an ineligible place: %q", x.Job, x.Where)
			}
		}
		seen[x.Job] = true
	}
	if len(l.st.Jobs) != JobsPerCycle {
		t.Errorf("state tracks %d jobs", len(l.st.Jobs))
	}
	for job := range l.st.Jobs {
		if !seen[job] {
			t.Errorf("state tracks %s which is not on the board", job)
		}
	}
	// Spent only what the jobs cost, from a budget that has plenty.
	var topped int64
	for _, x := range m.topups {
		topped += x
	}
	if topped != l.st.ToppedUpMinor || topped > 10000 || topped < 1200 {
		t.Errorf("topped up %d, state says %d", topped, l.st.ToppedUpMinor)
	}
}

func TestEligibilityRefusesHomes(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "overpass.json"))
	if err != nil {
		t.Fatal(err)
	}
	places, err := ParseOverpass(data)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"node/101": true, "node/102": true, "way/201": true, "node/103": true,
		"node/104": false, // shop in a building=house
		"node/105": false, // no name
		"node/106": false, // guest house
		"node/107": true,  // museum
		"node/108": false, // shelter
		"node/109": true, "node/110": true, "node/111": true, "node/112": true, "node/113": true,
	}
	for _, p := range places {
		if got := Eligible(p); got != want[p.ID] {
			t.Errorf("%s (%s): eligible=%v want %v", p.ID, p.Name, got, want[p.ID])
		}
	}
	if q := overpassQuery(detroitLat, detroitLon, 5000); !strings.Contains(q, `["shop"][name]`) ||
		strings.Contains(q, "building") {
		t.Errorf("query = %s", q)
	}
}

func TestBudgetCapHoldsAcrossCyclesAndRestarts(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	dir := t.TempDir()
	m := newFake(clock)
	m.ops["a"] = detroitOp(12)
	const budget = 1000 // two cheap jobs at most

	l := newLoop(t, dir, budget, m, clock)
	rep, err := l.Cycle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !rep.BudgetExhausted || rep.Posted == 0 || rep.Posted >= JobsPerCycle {
		t.Fatalf("report = %+v", rep)
	}
	sum := func(xs []int64) (s int64) {
		for _, x := range xs {
			s += x
		}
		return
	}
	if sum(m.topups) > budget {
		t.Fatalf("topped up %d over a budget of %d", sum(m.topups), budget)
	}
	first := sum(m.topups)

	// Another cycle in the same process: nothing more.
	if _, err := l.Cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sum(m.topups) != first {
		t.Fatalf("second cycle topped up more: %d", sum(m.topups))
	}

	// A restart with an empty balance and a fresh ledger. The state file is
	// the only memory, and it must hold.
	m2 := newFake(clock)
	m2.ops["a"] = detroitOp(12)
	l2 := newLoop(t, dir, budget, m2, clock)
	if l2.st.ToppedUpMinor != first {
		t.Fatalf("restart forgot spend: %d vs %d", l2.st.ToppedUpMinor, first)
	}
	if _, err := l2.Cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if first+sum(m2.topups) > budget {
		t.Fatalf("across restarts topped up %d over a budget of %d", first+sum(m2.topups), budget)
	}
	// And a bigger budget passed on restart is honoured only up from the
	// recorded spend, never from zero.
	l3 := newLoop(t, dir, budget+400, m2, clock)
	if _, err := l3.Cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if first+sum(m2.topups) > budget+400 {
		t.Fatalf("raised budget overspent: %d", first+sum(m2.topups))
	}
}

func TestDedupeByPlaceAndCapPerCluster(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	m := newFake(clock)
	m.ops["a"] = detroitOp(12)
	l := newLoop(t, t.TempDir(), 100000, m, clock)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, err := l.Cycle(ctx); err != nil {
			t.Fatal(err)
		}
	}
	// Three, then two more to the cap of five, then none.
	if n := len(m.board.Listings()); n != MaxOpenPerCluster {
		t.Fatalf("board has %d house jobs, cap is %d", n, MaxOpenPerCluster)
	}
	places := map[string]int{}
	for _, rec := range l.st.Jobs {
		places[rec.Place]++
	}
	for p, n := range places {
		if n > 1 {
			t.Errorf("%s posted %d times", p, n)
		}
	}

	// Everything expires. The same places are still within the month, so
	// nothing is reposted.
	now = now.Add(JobTTL + time.Hour)
	before := l.st.Posted
	if _, err := l.Cycle(ctx); err != nil {
		t.Fatal(err)
	}
	if l.st.Posted == before {
		// There were more eligible places than five in the fixture, so a
		// few new ones are fine; what must not happen is a repeat.
		t.Logf("no new places left")
	}
	for p := range places {
		if at, ok := l.st.Seen[p]; !ok || at.After(now.Add(-JobTTL)) {
			t.Errorf("%s was reposted inside the window", p)
		}
	}

	// A month on, the places are fair game again.
	now = now.Add(SeenFor + time.Hour)
	if _, err := l.Cycle(ctx); err != nil {
		t.Fatal(err)
	}
	if len(l.st.Seen) == 0 || l.st.Posted <= before {
		t.Errorf("nothing reposted after the window: posted=%d seen=%d", l.st.Posted, len(l.st.Seen))
	}
}

func TestSkipsClustersWithRealWorkAndOperatorsOutOfReach(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	m := newFake(clock)
	m.ops["a"] = detroitOp(12)
	// Somebody else's open job in reach: real demand exists, stay out.
	if err := m.board.Post(&api.Listing{
		Job: "real-1", Kind: api.KindObserve, Title: "Is the sign up?",
		Owner: "a-buyer", LatE7: detroitLat, LonE7: detroitLon, PayMinor: 500,
		Expires: now.Add(time.Hour), Posted: now,
	}); err != nil {
		t.Fatal(err)
	}
	l := newLoop(t, t.TempDir(), 10000, m, clock)
	rep, err := l.Cycle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Posted != 0 || rep.Skipped != 1 {
		t.Fatalf("report = %+v", rep)
	}

	// An operator whose range reaches nothing in the response: no jobs.
	m2 := newFake(clock)
	m2.ops["far"] = api.Capacity{Accepting: true, RangeMiles: 1, LatE7: 420000000, LonE7: -830000000}
	l2 := newLoop(t, t.TempDir(), 10000, m2, clock)
	rep, err = l2.Cycle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Posted != 0 || len(m2.topups) != 0 {
		t.Fatalf("posted out of reach: %+v topups=%v", rep, m2.topups)
	}

	// Overpass down: the cycle is skipped and reported, nothing moves.
	m3 := newFake(clock)
	m3.ops["a"] = detroitOp(12)
	l3 := newLoop(t, t.TempDir(), 10000, m3, clock)
	l3.cfg.Places = &fixture{err: fmt.Errorf("overpass: status 504")}
	if _, err := l3.Cycle(context.Background()); err == nil {
		t.Fatal("expected the cycle to report the failure")
	}
	if len(m3.topups) != 0 || len(m3.board.Listings()) != 0 {
		t.Fatal("money moved on a failed cycle")
	}
	if st := l3.Status(context.Background()); st.LastError == "" {
		t.Error("status hides the failure")
	}
}

func TestWidensAStaleJobOnce(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	m := newFake(clock)
	m.ops["a"] = detroitOp(12)
	l := newLoop(t, t.TempDir(), 10000, m, clock)
	ctx := context.Background()
	if _, err := l.Cycle(ctx); err != nil {
		t.Fatal(err)
	}
	now = now.Add(WidenAfter + time.Minute)
	rep, err := l.Cycle(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Widened != JobsPerCycle || len(m.alerted) != JobsPerCycle {
		t.Fatalf("report = %+v alerted=%v", rep, m.alerted)
	}
	// The stale three are wider; anything posted this cycle is not.
	for _, x := range m.board.Listings() {
		rec := l.st.Jobs[x.Job]
		want := JobRadiusM
		if rec.Widened {
			want = JobRadiusM * WidenFactor
		}
		if x.RadiusM != want || rec.Widened != (rec.Posted.Before(now.Add(-WidenAfter))) {
			t.Errorf("%s radius %d widened=%v posted=%v", x.Job, x.RadiusM, rec.Widened, rec.Posted)
		}
	}
	now = now.Add(time.Hour)
	rep, err = l.Cycle(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Widened != 0 {
		t.Fatalf("widened twice: %+v", rep)
	}
}

func TestFindingsWrittenOnAcceptedEvidence(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	dir := t.TempDir()
	m := newFake(clock)
	m.ops["a"] = detroitOp(12)
	l := newLoop(t, dir, 10000, m, clock)
	if _, err := l.Cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	house := m.board.Listings()[0]
	sub := api.Submission{
		Job: house.Job, Holder: "cap-1", Verified: true, Finding: true,
		At:        now.Add(time.Hour),
		Artifacts: []api.Artifact{{SHA256: "abc123", Kind: "image"}},
	}
	l.RecordAccepted(house, sub)
	// Not ours, and not verified: ignored.
	l.RecordAccepted(&api.Listing{Job: "x", Owner: "someone", Title: "t"}, sub)
	unverified := sub
	unverified.Verified = false
	l.RecordAccepted(house, unverified)

	if l.findings.Count() != 1 {
		t.Fatalf("findings = %d", l.findings.Count())
	}
	data, err := os.ReadFile(filepath.Join(dir, findingsFile))
	if err != nil {
		t.Fatal(err)
	}
	var f Finding
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &f); err != nil {
		t.Fatal(err)
	}
	if f.Job != house.Job || f.Verdict != "yes" || f.PhotoSHA != "abc123" ||
		f.Place == "" || f.Question != house.Title || f.Area != "Detroit" {
		t.Errorf("finding = %+v", f)
	}
	if f.Lat != coarse(house.LatE7) || fmt.Sprint(f.Lat) != "42.33" {
		t.Errorf("lat %v is not coarse", f.Lat)
	}
	if strings.Contains(string(data), house.Where) {
		t.Errorf("finding leaks the address: %s", data)
	}

	mux := http.NewServeMux()
	l.Register(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/findings", nil))
	var out struct {
		Count    int       `json:"count"`
		Findings []Finding `json:"findings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Count != 1 || len(out.Findings) != 1 || out.Findings[0].Job != house.Job {
		t.Errorf("GET /v1/findings = %s", rec.Body.String())
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/bootstrap", nil))
	var st Status
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if !st.Enabled || st.Findings != 1 || st.Settled != 1 || st.Filled != 1 || st.Posted != JobsPerCycle ||
		st.BudgetMinor != 10000 || st.Clusters != 1 || st.LastRun.IsZero() {
		t.Errorf("GET /v1/bootstrap = %s", rec.Body.String())
	}

	// Survives a restart.
	l2 := newLoop(t, dir, 10000, m, clock)
	if l2.findings.Count() != 1 {
		t.Errorf("findings lost on restart")
	}
}

func TestLoopIsOffByDefault(t *testing.T) {
	l, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if l.Enabled() {
		t.Fatal("enabled with no configuration")
	}
	m := newFake(time.Now)
	m.ops["a"] = detroitOp(12)
	l.cfg.Market = m
	l.Start(context.Background())
	if rep, err := l.Cycle(context.Background()); err != nil || rep.Posted != 0 {
		t.Fatalf("an off loop did something: %+v %v", rep, err)
	}
	if len(m.topups) != 0 || len(m.board.Listings()) != 0 {
		t.Fatal("an off loop moved money")
	}
	mux := http.NewServeMux()
	l.Register(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/bootstrap", nil))
	if !strings.Contains(rec.Body.String(), `"enabled":false`) {
		t.Errorf("status = %s", rec.Body.String())
	}

	// On without a budget or a data dir is refused, not silently off.
	if _, err := New(Config{Enabled: true, DataDir: t.TempDir(), Places: &fixture{}}); err == nil {
		t.Error("enabled with no budget was accepted")
	}
	if _, err := New(Config{Enabled: true, BudgetMinor: 100, Places: &fixture{}}); err == nil {
		t.Error("enabled with no data dir was accepted")
	}
}

// The real gate: house jobs go through the exchange's own PostFunded, with
// screening, escrow and the board's funded check, and a place whose name
// trips screening is refused without stopping the cycle.
func TestPostsThroughTheExchange(t *testing.T) {
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	srv, err := exchange.Open(key, "http://example.test", exchange.Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	srv.Now = func() time.Time { return now }
	srv.Board.Now = srv.Now
	srv.Capacities.Set("op-1", detroitOp(12))
	srv.Handler()

	dir := t.TempDir()
	l, err := New(Config{
		Enabled: true, BudgetMinor: 100000, DataDir: dir, Places: &fixture{},
		Now: srv.Now, Logf: func(f string, a ...any) { t.Logf(f, a...) },
	})
	if err != nil {
		t.Fatal(err)
	}
	l.Attach(srv)
	ctx := context.Background()

	// Walk the rotation until the bank whose name reads like account
	// creation comes up, so the screening path is exercised.
	var refusedSeen bool
	for i := 0; i < 6; i++ {
		rep, err := l.Cycle(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if rep.Posted == 0 {
			break
		}
		now = now.Add(JobTTL + time.Hour)
	}
	for _, x := range srv.Board.All() {
		if strings.Contains(x.Where, "Open An Account") {
			t.Errorf("screening did not refuse %q", x.Title)
		}
		held, err := srv.Ledger.Held(ctx, x.Job, "USD")
		if err != nil {
			t.Fatal(err)
		}
		if x.Open(now) && held < x.PayMinor {
			t.Errorf("%s listed with %d held", x.Job, held)
		}
		if x.Owner != HousePrincipal || !x.PostedByAgent {
			t.Errorf("%s owner=%q agent=%v", x.Job, x.Owner, x.PostedByAgent)
		}
	}
	if _, ok := l.st.Seen["node/112"]; ok {
		refusedSeen = true
	}
	if refusedSeen {
		t.Error("a refused place was recorded as posted")
	}
	if l.st.Posted == 0 {
		t.Fatal("nothing posted through the exchange")
	}
	bal, err := srv.Ledger.Balance(ctx, ledger.BalanceOf(HousePrincipal), "USD")
	if err != nil {
		t.Fatal(err)
	}
	if bal < 0 || l.st.ToppedUpMinor > 100000 {
		t.Errorf("balance %d topped up %d", bal, l.st.ToppedUpMinor)
	}
	// The hook is wired: an accepted submission becomes a finding.
	if srv.OnAccepted == nil {
		t.Fatal("OnAccepted not attached")
	}
	x := srv.Board.All()[0]
	srv.OnAccepted(x, api.Submission{Job: x.Job, Verified: true, At: now})
	if l.findings.Count() != 1 {
		t.Errorf("findings = %d", l.findings.Count())
	}
}
