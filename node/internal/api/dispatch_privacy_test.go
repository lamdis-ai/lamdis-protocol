package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// An offer goes to every endpoint in range before anybody has claimed
// anything. It must carry what the public board carries and nothing more:
// the coarse area, never the street address.
func TestOffersDoNotCarryTheAddress(t *testing.T) {
	var (
		mu   sync.Mutex
		body string
	)
	endpoint := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		body = string(b)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer endpoint.Close()

	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	caps := NewCapacities()
	board := NewBoard(NewCapabilities())
	board.Now = func() time.Time { return now }
	board.Capacities = caps
	caps.Set("fleet", Capacity{
		MaxConcurrent: 4, RangeMiles: 25, Accepting: true,
		LatE7: E7(37.7749), LonE7: E7(-122.4194), Webhook: endpoint.URL,
	})
	d := &Dispatcher{Board: board, Capacities: caps, BaseURL: "https://exchange.example",
		Now: func() time.Time { return now }, AllowPrivateHosts: true, Client: endpoint.Client()}

	l := &Listing{
		Job: "observe-1", Kind: KindObserve, Title: "Photograph the loading dock",
		Where: "1400 Industrial Way, Unit 7", Area: "Bayview", Access: "gate code 4471",
		PayMinor: 1800, Currency: "usd", Slots: 1,
		LatE7: E7(37.7849123), LonE7: E7(-122.4094456),
		Expires: now.Add(6 * time.Hour), Posted: now,
	}
	if err := board.Post(l); err != nil {
		t.Fatal(err)
	}
	if n := d.Announce(context.Background(), l); n != 1 {
		t.Fatalf("offered to %d operators", n)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, leak := range []string{"Industrial Way", "Unit 7", "4471", "37.7849123", "122.4094456", `"where"`} {
		if strings.Contains(body, leak) {
			t.Errorf("the offer leaks %q: %s", leak, body)
		}
	}
	for _, want := range []string{`"area":"Bayview"`, `"area_lat":37.78`, `"area_lon":-122.41`, `"distance_miles"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the offer lacks %s: %s", want, body)
		}
	}
}

// An operator with no position is not "in range" of everything. They are
// offered nothing until they say where they are.
func TestAnUnpositionedOperatorIsOfferedNothing(t *testing.T) {
	var hits int
	var mu sync.Mutex
	endpoint := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer endpoint.Close()

	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	caps := NewCapacities()
	board := NewBoard(NewCapabilities())
	board.Now = func() time.Time { return now }
	board.Capacities = caps
	caps.Set("nowhere", Capacity{MaxConcurrent: 4, RangeMiles: 60, Accepting: true,
		Webhook: endpoint.URL})
	d := &Dispatcher{Board: board, Capacities: caps, Now: func() time.Time { return now },
		AllowPrivateHosts: true, Client: endpoint.Client()}

	for _, l := range []*Listing{
		{Job: "located", Kind: KindObserve, Title: "x", PayMinor: 500, Currency: "usd", Slots: 1,
			LatE7: E7(37.78), LonE7: E7(-122.41), Expires: now.Add(time.Hour), Posted: now},
		{Job: "unlocated", Kind: KindObserve, Title: "y", PayMinor: 500, Currency: "usd", Slots: 1,
			Expires: now.Add(time.Hour), Posted: now},
	} {
		if err := board.Post(l); err != nil {
			t.Fatal(err)
		}
		if n := d.Announce(context.Background(), l); n != 0 {
			t.Errorf("%s was offered to an operator with no position", l.Job)
		}
	}
	mu.Lock()
	quiet := hits
	mu.Unlock()
	if quiet != 0 {
		t.Errorf("%d offers reached an endpoint that never said where it was", quiet)
	}
	// Setting a position opens the flow.
	caps.Set("nowhere", Capacity{MaxConcurrent: 4, RangeMiles: 60, Accepting: true,
		LatE7: E7(37.77), LonE7: E7(-122.42), Webhook: endpoint.URL})
	l, _ := board.Get("located")
	if n := d.Announce(context.Background(), l); n != 1 {
		t.Errorf("after setting a position, offered to %d", n)
	}
}
