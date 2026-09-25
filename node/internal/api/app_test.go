package api

import (
	"crypto/ed25519"
	"strings"
	"testing"
	"time"
)

func testApp(t *testing.T) *App {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &App{Key: priv, Token: "owner-token",
		Now: func() time.Time { return time.Unix(1_000_000, 0) }}
}

// A capability says which lanes it covers. Widening it is the whole attack.
func TestShareCapabilityCannotBeWidened(t *testing.T) {
	a := testApp(t)
	tok, err := a.mintShare(shareClaim{Thread: "T", Lanes: []string{"summary"},
		Exp: a.now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := a.readShare(tok); !ok || len(c.Lanes) != 1 || c.Lanes[0] != "summary" {
		t.Fatalf("honest capability did not round-trip: %+v ok=%v", c, ok)
	}
	// Re-signing is the only way to change the claim, and the key is not the
	// holder's to use.
	forged, err := a.mintShare(shareClaim{Thread: "T", Lanes: []string{"summary", "content"},
		Exp: a.now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	body, _, _ := cut(forged, ".")
	_, sig, _ := cut(tok, ".")
	if _, ok := a.readShare(body + "." + sig); ok {
		t.Fatal("a capability body swapped onto another signature was accepted")
	}
}

func TestShareCapabilityExpires(t *testing.T) {
	a := testApp(t)
	tok, err := a.mintShare(shareClaim{Thread: "T", Lanes: []string{"summary"},
		Exp: a.now().Add(-time.Second).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := a.readShare(tok); ok {
		t.Fatal("an expired capability was accepted")
	}
}

// A capability minted by one node must not open another's threads.
func TestShareCapabilityIsNodeSpecific(t *testing.T) {
	a, b := testApp(t), testApp(t)
	tok, err := a.mintShare(shareClaim{Thread: "T", Lanes: []string{"summary"},
		Exp: a.now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := b.readShare(tok); ok {
		t.Fatal("another node's capability was accepted")
	}
}

func TestShareCapabilityRejectsGarbage(t *testing.T) {
	a := testApp(t)
	for _, bad := range []string{"", ".", "abc", "abc.def", "!!!.???"} {
		if _, ok := a.readShare(bad); ok {
			t.Fatalf("accepted %q as a capability", bad)
		}
	}
}

func cut(s, sep string) (string, string, bool) {
	for i := 0; i+len(sep) <= len(s); i++ {
		if s[i:i+len(sep)] == sep {
			return s[:i], s[i+len(sep):], true
		}
	}
	return s, "", false
}

// Script that lands in the stylesheet is not a syntax error anywhere, so no
// checker catches it: the page just loads without it. Both halves of the
// page share comment markers, which is how it happened once.
func TestTheStylesheetHoldsNoScript(t *testing.T) {
	for _, bad := range []string{"function ", "=function(", "api(\"/app"} {
		if strings.Contains(appCSS, bad) {
			t.Fatalf("appCSS contains %q: script was pasted into the stylesheet", bad)
		}
	}
	for _, fn := range []string{"function loadTeam(", "function teamSheet(", "function drawWho(", "function moveTo(", "function connectCard("} {
		if !strings.Contains(appJS, fn) {
			t.Fatalf("appJS is missing %s", fn)
		}
	}
}
