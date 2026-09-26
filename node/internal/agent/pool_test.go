package agent

import (
	"testing"
	"time"
)

func TestPoolFillsAndResetsDaily(t *testing.T) {
	p := &Pool{Tokens: 100}
	day := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	p.Add(day, 60)
	if p.Full(day) {
		t.Fatal("full at 60 of 100")
	}
	p.Add(day, 40)
	if !p.Full(day) {
		t.Fatal("not full at 100 of 100")
	}
	if p.Full(day.Add(24 * time.Hour)) {
		t.Fatal("still full the next day")
	}
	var none *Pool
	none.Add(day, 5)
	if none.Full(day) {
		t.Fatal("a missing pool is never full")
	}
}

func TestOwnKeySkipsSharedLimitsOnlyOnAHost(t *testing.T) {
	own := Config{OpenRouterKey: "sk-own"}
	if (&Runner{SharedOnly: true}).limited(own) {
		t.Fatal("own key on a host is held to the shared limits")
	}
	if !(&Runner{SharedOnly: true}).limited(Config{}) {
		t.Fatal("shared key on a host is not limited")
	}
	if !(&Runner{}).limited(own) {
		t.Fatal("a laptop's own limits stopped applying")
	}
}
