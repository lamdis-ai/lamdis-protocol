package exchange

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
)

// The failure these cover: an infeasible quote is the single most valuable
// signal this exchange produces — somebody wanted work done where there is no
// supply — and it was answered and forgotten. Nothing recorded that anyone had
// ever asked, so there was no way to decide where to recruit except by guess.

func demandServer(t *testing.T) (*Server, http.Handler, string) {
	t.Helper()
	dir := t.TempDir()
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	s, err := Open(key, "https://example.test", Options{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	return s, s.Handler(), dir
}

func askQuote(t *testing.T, h http.Handler, body map[string]any, from string) Quote {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/v1/quote", bytes.NewReader(raw))
	r.Header.Set("X-Forwarded-For", from)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("quote: %d %s", w.Code, w.Body.String())
	}
	var q Quote
	if err := json.Unmarshal(w.Body.Bytes(), &q); err != nil {
		t.Fatal(err)
	}
	return q
}

// The answer stays honest and stops being a dead end.
func TestAnInfeasibleQuoteIsRecordedAndOffersTheSandbox(t *testing.T) {
	s, h, dir := demandServer(t)
	q := askQuote(t, h, map[string]any{
		"kind": "do", "predicate": "the gutters are clear",
		"instructions": "clear the north gutter",
		"skills":       []string{"ladder"},
		"lat":          42.33141, "lon": -83.04582,
	}, "198.51.100.7")

	if q.Feasible || q.Reachable != "none" {
		t.Fatalf("this exchange has no supply and said otherwise: %+v", q)
	}
	if !strings.Contains(q.Why, "nobody within range") {
		t.Fatalf("the honest sentence is gone: %q", q.Why)
	}
	if !q.Recorded {
		t.Fatalf("the request was not recorded: %+v", q)
	}
	advice := strings.Join(q.Advice, "\n")
	if !strings.Contains(advice, "sandbox") {
		t.Fatalf("an infeasible answer does not mention the sandbox: %q", advice)
	}
	if !strings.Contains(advice, "/v1/demand") {
		t.Fatalf("an infeasible answer does not say the request was filed: %q", advice)
	}
	if s.Demand.Count() != 1 {
		t.Fatalf("the register holds %d", s.Demand.Count())
	}

	// What was written down, and what was not. The predicate, the
	// instructions and the precise position must not be on disk anywhere.
	raw, err := os.ReadFile(filepath.Join(dir, "demand.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"gutters are clear", "north gutter", "423314100"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("the demand register kept %q:\n%s", leak, raw)
		}
	}
	// Coarsened on the way in, to the same grain the coverage register uses.
	if !strings.Contains(string(raw), "4233000") || !strings.Contains(string(raw), "-8305000") {
		t.Fatalf("the coarse cell is not what was stored:\n%s", raw)
	}
}

// A feasible answer is not demand, and a refused one is not demand we would
// ever want to recruit for.
func TestOnlyUnservedLiveRequestsAreRecorded(t *testing.T) {
	s, h, _ := demandServer(t)

	// The sandbox is always feasible, so asking it is not evidence that
	// anybody wanted anything done anywhere.
	q := askQuote(t, h, map[string]any{
		"kind": "do", "predicate": "the gutters are clear",
		"sandbox": true, "lat": 42.33141, "lon": -83.04582,
	}, "198.51.100.7")
	if !q.Feasible || !q.Sandbox || q.Reachable != "simulated" {
		t.Fatalf("the sandbox did not answer for itself: %+v", q)
	}
	if q.Recorded || s.Demand.Count() != 0 {
		t.Fatalf("a sandbox question was filed as real demand: %d", s.Demand.Count())
	}

	// A request with no position cannot say where to recruit, so it is not
	// kept at 0,0 where it would look like demand in the Atlantic.
	q = askQuote(t, h, map[string]any{
		"kind": "do", "predicate": "the gutters are clear",
	}, "198.51.100.7")
	if q.Recorded || s.Demand.Count() != 0 {
		t.Fatalf("a request with no position was filed: %d", s.Demand.Count())
	}
}

// The published report follows the coverage report's discipline: coarse
// points, bucketed counts, and nothing specific below five.
func TestTheDemandReportBucketsAndWithholds(t *testing.T) {
	s, h, _ := demandServer(t)
	rec := func() {
		if !s.Demand.Record("198.51.100.7", Demand{
			LatE7: 423314100, LonE7: -830458200,
			Kind: api.KindDo, Skills: []api.Skill{"ladder"},
		}) {
			t.Fatal("the register refused a record")
		}
	}

	rec()
	report := func() map[string]any {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/v1/demand", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("demand: %d %s", w.Code, w.Body.String())
		}
		var out map[string]any
		json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}

	out := report()
	areas, _ := out["areas"].([]any)
	if len(areas) != 1 {
		t.Fatalf("expected one cell: %v", out)
	}
	a, _ := areas[0].(map[string]any)
	if a["asked"] != "fewer than 5" {
		t.Fatalf("one request was published as %v", a["asked"])
	}
	if lat, _ := a["lat"].(float64); lat != 42.33 {
		t.Fatalf("the point is not coarse: %v", a["lat"])
	}
	// Below the threshold, a trade and a date in a one-kilometre square
	// narrows to one request, so neither is published.
	if _, ok := a["skills"]; ok {
		t.Fatalf("skills published below the threshold: %v", a)
	}
	if _, ok := a["last_asked"]; ok {
		t.Fatalf("a date published below the threshold: %v", a)
	}

	for i := 0; i < 4; i++ {
		rec()
	}
	a, _ = report()["areas"].([]any)[0].(map[string]any)
	if a["asked"] != "5 to 9" {
		t.Fatalf("five requests were published as %v", a["asked"])
	}
	skills, _ := a["skills"].([]any)
	if len(skills) != 1 || skills[0] != "ladder" {
		t.Fatalf("at the threshold the trades are still withheld: %v", a)
	}
	if _, ok := a["last_asked"].(string); !ok {
		t.Fatalf("at the threshold the date is still withheld: %v", a)
	}
}

// Recording is free and unauthenticated, so it is bounded the way anonymous
// posting and the coverage register are.
func TestOneCallerCannotInventAMarket(t *testing.T) {
	s, _, _ := demandServer(t)
	kept := 0
	for i := 0; i < DemandPerHour+20; i++ {
		if s.Demand.Record("198.51.100.7", Demand{
			LatE7: 423314100, LonE7: -830458200, Kind: api.KindDo,
		}) {
			kept++
		}
	}
	if kept != DemandPerHour {
		t.Fatalf("one address filed %d records in an hour", kept)
	}
	// A different caller is unaffected.
	if !s.Demand.Record("203.0.113.9", Demand{
		LatE7: 423314100, LonE7: -830458200, Kind: api.KindDo,
	}) {
		t.Fatal("one busy caller silenced everybody else")
	}
}
