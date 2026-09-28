package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A script minting guests from one network runs out fast, a guest gets the
// small allowance, and confirming an email lifts it to the full one.
func TestGuestsAreBoundedAndConfirmingLiftsIt(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		if !allowStart("198.51.100.7", now, 5, 15) {
			t.Fatalf("start %d refused", i)
		}
	}
	if allowStart("198.51.100.7", now, 5, 15) {
		t.Fatal("a sixth start in an hour was allowed")
	}
	if !allowStart("198.51.100.8", now, 5, 15) {
		t.Fatal("another network was refused")
	}
	if !allowStart("198.51.100.7", now.Add(61*time.Minute), 5, 15) {
		t.Fatal("the next hour was refused")
	}

	h, _, _, _ := testHost(t)
	h.Guests, h.MaxAccounts = true, 10
	h.AccountTokensPerDay, h.GuestTokensPerDay = 1_000_000, 150_000
	mail := &fakeMail{}
	h.Mail = mail
	srv := httptest.NewServer(h.Handler())
	defer srv.Close()
	hd := h.Handler()
	tok := start(t, h, srv.URL)
	limit := func() int {
		var d struct {
			Reach struct {
				Max int `json:"max_tokens_per_day"`
			} `json:"reach"`
		}
		json.Unmarshal(call(hd, "GET", "/app/api/agent", tok, "").Body.Bytes(), &d)
		return d.Reach.Max
	}
	if n := limit(); n != 150_000 {
		t.Fatalf("a guest's allowance is %d", n)
	}
	call(hd, "POST", "/app/api/signin/email", tok, `{"email":"sam@example.com"}`)
	if w := call(hd, "POST", "/app/api/signin/email/confirm", tok, `{"email":"sam@example.com","code":"`+lastCode(t, mail)+`"}`); !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("confirm: %s", w.Body.String())
	}
	if n := limit(); n != 1_000_000 {
		t.Fatalf("confirming did not lift the allowance: %d", n)
	}
}

func TestClientIPFromTheEdge(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Forwarded-For", "203.0.113.9, 130.176.1.1")
	if ip := clientIP(r); ip != "203.0.113.9" {
		t.Fatalf("xff: %s", ip)
	}
	r.Header.Set("CloudFront-Viewer-Address", "2001:db8:1:2:3:4:5:6:443")
	if ip := clientIP(r); ip != "2001:db8:1:2::/64" {
		t.Fatalf("v6: %s", ip)
	}
}
