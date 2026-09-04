package exchange

import (
	"bytes"
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
)

// The failure these cover: the exchange had nowhere to put "I would work
// here" from somebody without an account, so a visitor who arrived, found
// nothing they could take and left was gone for good — and the loop that
// decides where to post the first real work had nothing to aim at.

func coverageServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	s, err := Open(key, "https://example.test", Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return s, s.Handler()
}

func registerCoverage(t *testing.T, h http.Handler, body map[string]any, from string) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/v1/coverage", bytes.NewReader(raw))
	r.Header.Set("X-Forwarded-For", from)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// Somebody with no account, no key and no balance can say where they work.
func TestAnyoneCanRegisterWhereTheyWork(t *testing.T) {
	_, h := coverageServer(t)
	w := registerCoverage(t, h, map[string]any{
		"email": "Marcus@Example.com", "place": "Detroit, MI",
		"lat_e7": 423314000, "lon_e7": -830458000,
		"range_miles": 25, "skills": []string{"ladder", "vehicle"},
	}, "198.51.100.7")
	if w.Code != http.StatusOK {
		t.Fatalf("registering interest: %d %s", w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["registered"] != true {
		t.Fatalf("the register did not confirm: %v", out)
	}
	// The reply must not imply work exists. It says the opposite.
	note, _ := out["note"].(string)
	if !strings.Contains(note, "no paid work waiting") &&
		!strings.Contains(note, "There is no paid work") {
		t.Errorf("the confirmation does not say there is no work waiting: %q", note)
	}
	if !strings.Contains(note, "six hours") {
		t.Errorf("the confirmation does not state the alert interval: %q", note)
	}
}

// An address is not a thing this form is allowed to keep.
func TestRegisteredPositionsAreCoarsenedBeforeTheyAreStored(t *testing.T) {
	dir := t.TempDir()
	cs := NewCoverages(dir)
	// A precise point: to seven decimal places, a doorstep.
	got, err := cs.Add(Coverage{
		Email: "marcus@example.com", LatE7: 423314567, LonE7: -830458912,
		RangeMiles: 12,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.LatE7 != 423300000 || got.LonE7 != -830500000 {
		t.Fatalf("the stored point is finer than two decimal places: %d,%d",
			got.LatE7, got.LonE7)
	}
	// And the file on disk holds no more than that, because it was never
	// given more.
	raw, err := os.ReadFile(filepath.Join(dir, "coverage.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("423314567")) || bytes.Contains(raw, []byte("830458912")) {
		t.Fatalf("the precise position reached the file: %s", raw)
	}
}

// The one rule everything public shares.
func TestCoarseE7RoundsToTwoDecimalPlaces(t *testing.T) {
	for _, c := range []struct{ in, want int64 }{
		{423314567, 423300000},
		{-830458912, -830500000},
		{0, 0},
		{515074000, 515100000},
	} {
		if got := api.CoarseE7(c.in); got != c.want {
			t.Errorf("CoarseE7(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

// Free and unauthenticated means bounded, the same way anonymous posting is.
func TestRegisteringIsRateLimitedPerAddress(t *testing.T) {
	_, h := coverageServer(t)
	for i := 0; i < CoveragePerHour; i++ {
		w := registerCoverage(t, h, map[string]any{
			"email": fmt.Sprintf("person%d@example.com", i), "place": "Detroit, MI",
		}, "203.0.113.9")
		if w.Code != http.StatusOK {
			t.Fatalf("registration %d refused early: %d %s", i, w.Code, w.Body.String())
		}
	}
	w := registerCoverage(t, h, map[string]any{
		"email": "one-too-many@example.com", "place": "Detroit, MI",
	}, "203.0.113.9")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("the %dth registration from one address was allowed: %d",
			CoveragePerHour+1, w.Code)
	}
	// Somebody else is unaffected: the limit is on an address, not on the form.
	if w := registerCoverage(t, h, map[string]any{
		"email": "elsewhere@example.com", "place": "Ann Arbor, MI",
	}, "203.0.113.10"); w.Code != http.StatusOK {
		t.Fatalf("a different address was caught by another's limit: %d", w.Code)
	}
}

// Nothing published may identify anybody: no exact counts under five, and no
// point finer than the board publishes.
func TestTheCoverageReportIsCoarseAndCountsNobodyPrecisely(t *testing.T) {
	s, h := coverageServer(t)
	for i := 0; i < 3; i++ {
		if _, err := s.Coverage.Add(Coverage{
			Email: fmt.Sprintf("p%d@example.com", i), Place: "Detroit, MI",
			LatE7: 423314567 + int64(i), LonE7: -830458912, RangeMiles: 12,
		}); err != nil {
			t.Fatal(err)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/v1/coverage", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("reading the register: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		Areas      []CoverageArea `json:"areas"`
		Operators  string         `json:"operators"`
		Interested string         `json:"interested"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Areas) != 1 {
		t.Fatalf("three neighbours were not one area: %+v", out.Areas)
	}
	a := out.Areas[0]
	if a.Interested != "fewer than 5" {
		t.Errorf("a count under five was published as %q", a.Interested)
	}
	if a.Place != "" {
		t.Errorf("a place name was published on the evidence of three people: %q", a.Place)
	}
	if got := fmt.Sprintf("%.10f", a.Lat); !strings.HasPrefix(got, "42.3300000") {
		t.Errorf("the published point is finer than two decimals: %v", a.Lat)
	}
	if out.Interested != "fewer than 5" {
		t.Errorf("the total was published as %q", out.Interested)
	}
	if strings.Contains(w.Body.String(), "example.com") {
		t.Error("an email address reached the public report")
	}

	// Past the threshold the bucket moves and the place is nameable.
	for i := 3; i < 6; i++ {
		if _, err := s.Coverage.Add(Coverage{
			Email: fmt.Sprintf("p%d@example.com", i), Place: "Detroit, MI",
			LatE7: 423300000, LonE7: -830500000, RangeMiles: 12,
		}); err != nil {
			t.Fatal(err)
		}
	}
	areas := s.Coverage.Areas(nil)
	if len(areas) != 1 || areas[0].Interested != "5 to 9" || areas[0].Place != "Detroit, MI" {
		t.Fatalf("six people did not read as an area worth naming: %+v", areas)
	}
}

// Somebody who registers twice is one person, not two.
func TestRegisteringAgainReplacesTheSameEntry(t *testing.T) {
	cs := NewCoverages(t.TempDir())
	for _, miles := range []int{5, 40} {
		if _, err := cs.Add(Coverage{
			Email: "marcus@example.com", Place: "Detroit, MI", RangeMiles: miles,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if n := cs.Count(); n != 1 {
		t.Fatalf("re-registering made %d entries", n)
	}
}

// A promise the server would clamp is not a promise.
func TestARegisteredRangeIsClampedTheWayACapacityIs(t *testing.T) {
	cs := NewCoverages(t.TempDir())
	got, err := cs.Add(Coverage{Email: "a@example.com", Place: "Detroit", RangeMiles: 5000})
	if err != nil {
		t.Fatal(err)
	}
	if got.RangeMiles != MaxCoverageRangeMiles {
		t.Errorf("range %d was not clamped to %d", got.RangeMiles, MaxCoverageRangeMiles)
	}
	if got, _ := cs.Add(Coverage{Email: "b@example.com", Place: "Detroit"}); got.RangeMiles != DefaultCoverageRangeMiles {
		t.Errorf("an unstated range became %d, not the default %d",
			got.RangeMiles, DefaultCoverageRangeMiles)
	}
}

// Saying nothing about where you are is refused, because it cannot be used.
func TestRegisteringNeedsAnEmailAndSomewhere(t *testing.T) {
	cs := NewCoverages(t.TempDir())
	if _, err := cs.Add(Coverage{Email: "not-an-address", Place: "Detroit"}); err == nil {
		t.Error("an address with no domain was accepted")
	}
	if _, err := cs.Add(Coverage{Email: "a@example.com"}); err == nil {
		t.Error("somebody with no place and no position was accepted")
	}
}

// The people on the register are exactly who the first real job is for.
func TestNewWorkReachesTheRegister(t *testing.T) {
	s, _ := coverageServer(t)
	f := &fakeMail{}
	s.Mail = f
	if _, err := s.Coverage.Add(Coverage{
		Email: "marcus@example.com", Place: "Detroit, MI",
		LatE7: 423314000, LonE7: -830458000, RangeMiles: 25,
	}); err != nil {
		t.Fatal(err)
	}
	// Somebody too far away hears nothing.
	if _, err := s.Coverage.Add(Coverage{
		Email: "phoenix@example.com", Place: "Phoenix, AZ",
		LatE7: 334484000, LonE7: -1120740000, RangeMiles: 25,
	}); err != nil {
		t.Fatal(err)
	}
	job := &api.Listing{
		Job: "obs_1", Kind: api.KindObserve, Title: "is the sign up",
		Area: "Detroit, MI", LatE7: 423320000, LonE7: -830460000,
		PayMinor: 500, Currency: "USD", Slots: 1,
		Expires: s.now().Add(time.Hour), Owner: "buyer",
	}
	if n := s.AlertCoverage(context.Background(), job); n != 1 {
		t.Fatalf("the register was told %d times, want 1", n)
	}
	sent := mailed(s, f)
	if len(to(sent, "marcus@example.com")) != 1 {
		t.Fatal("the person in range was not told")
	}
	if len(to(sent, "phoenix@example.com")) != 0 {
		t.Fatal("somebody two thousand miles away was told about a job")
	}
	// Every message carries the way out, and a second job in the same window
	// does not send a second message.
	if !strings.Contains(sent[0].body, "/coverage/stop?e=") {
		t.Errorf("no way off the register in the email:\n%s", sent[0].body)
	}
	if n := s.AlertCoverage(context.Background(), job); n != 0 {
		t.Errorf("a second alert went out inside the quiet window: %d", n)
	}

	// A practice job is not work and reaches nobody.
	s.Coverage.Add(Coverage{Email: "marcus@example.com", Place: "Detroit, MI",
		LatE7: 423314000, LonE7: -830458000, RangeMiles: 25})
	practice := *job
	practice.Practice = true
	if n := s.AlertCoverage(context.Background(), &practice); n != 0 {
		t.Errorf("a practice run was emailed to %d people", n)
	}
}

// The link at the bottom of the email has to work, and only for its owner.
func TestSomebodyCanTakeThemselvesOffTheRegister(t *testing.T) {
	s, h := coverageServer(t)
	if _, err := s.Coverage.Add(Coverage{
		Email: "marcus@example.com", Place: "Detroit, MI", RangeMiles: 12,
	}); err != nil {
		t.Fatal(err)
	}
	bad := httptest.NewRecorder()
	h.ServeHTTP(bad, httptest.NewRequest("GET", "/coverage/stop?e=marcus@example.com&t=nope", nil))
	if bad.Code == http.StatusOK || s.Coverage.Count() != 1 {
		t.Fatal("a guessed token removed somebody")
	}
	ok := httptest.NewRecorder()
	h.ServeHTTP(ok, httptest.NewRequest("GET",
		"/coverage/stop?e=marcus@example.com&t="+s.CoverageToken("marcus@example.com"), nil))
	if ok.Code != http.StatusOK {
		t.Fatalf("the unsubscribe link failed: %d", ok.Code)
	}
	if s.Coverage.Count() != 0 {
		t.Fatal("the unsubscribe link did not remove anybody")
	}
}
