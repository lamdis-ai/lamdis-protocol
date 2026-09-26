package api

import (
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func lastCode(t *testing.T, m *fakeMail) string {
	t.Helper()
	if len(m.sent) == 0 {
		t.Fatal("no mail sent")
	}
	c := regexp.MustCompile(`\b\d{6}\b`).FindString(m.sent[len(m.sent)-1])
	if c == "" {
		t.Fatalf("no code in %q", m.sent[len(m.sent)-1])
	}
	return c
}

// Sam starts on a laptop, adds an email, then opens Lamdis on a phone,
// which begins as a stranger. Signing in with the same address makes the
// phone Sam, and a device code does the same without email.
func TestOnePersonEveryDevice(t *testing.T) {
	h, _, _, _ := testHost(t)
	h.Guests, h.MaxAccounts = true, 10
	srv := httptest.NewServer(h.Handler())
	defer srv.Close()
	h.PublicBase = srv.URL
	mail := &fakeMail{}
	h.Mail = mail
	hd := h.Handler()

	laptop := start(t, h, srv.URL)
	who := func(tok string) (d struct {
		Handle string
		Email  string
		Kept   bool
	}) {
		json.Unmarshal(call(hd, "GET", "/app/api/whoami", tok, "").Body.Bytes(), &d)
		return
	}
	if who(laptop).Kept {
		t.Fatal("a fresh guest is kept")
	}
	// New address: the code attaches it to the laptop's account.
	if w := call(hd, "POST", "/app/api/signin/email", laptop, `{"email":"Sam@Example.com"}`); !strings.Contains(w.Body.String(), `"existing":false`) {
		t.Fatalf("attach: %s", w.Body.String())
	}
	if w := call(hd, "POST", "/app/api/signin/email/confirm", laptop, `{"email":"sam@example.com","code":"`+lastCode(t, mail)+`"}`); !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("confirm: %s", w.Body.String())
	}
	sam := who(laptop)
	if !sam.Kept || sam.Email != "sam@example.com" {
		t.Fatalf("after confirm: %+v", sam)
	}

	// The phone is somebody else until it signs in.
	phone := start(t, h, srv.URL)
	if who(phone).Handle == sam.Handle {
		t.Fatal("phone was Sam before signing in")
	}
	if w := call(hd, "POST", "/app/api/signin/email", phone, `{"email":"sam@example.com"}`); !strings.Contains(w.Body.String(), `"existing":true`) {
		t.Fatalf("sign-in: %s", w.Body.String())
	}
	if w := call(hd, "POST", "/app/api/signin/email/confirm", phone, `{"email":"sam@example.com","code":"000000x"}`); !strings.Contains(w.Body.String(), "not right") {
		t.Fatalf("a wrong code: %s", w.Body.String())
	}
	var in struct{ Token string }
	json.Unmarshal(call(hd, "POST", "/app/api/signin/email/confirm", phone, `{"email":"sam@example.com","code":"`+lastCode(t, mail)+`"}`).Body.Bytes(), &in)
	if in.Token == "" || who(in.Token).Handle != sam.Handle {
		t.Fatalf("phone did not become Sam: %q", in.Token)
	}

	// A device code, no email: a tablet becomes Sam once, and only once.
	var dc struct{ Code, URL string }
	json.Unmarshal(call(hd, "POST", "/app/api/device/code", laptop, "").Body.Bytes(), &dc)
	if !strings.Contains(dc.URL, "device="+dc.Code) {
		t.Fatalf("device code: %+v", dc)
	}
	var tab struct{ Token string }
	json.Unmarshal(call(hd, "POST", "/app/api/device/redeem", "", `{"code":"`+strings.ToLower(strings.ReplaceAll(dc.Code, "-", ""))+`"}`).Body.Bytes(), &tab)
	if tab.Token == "" || who(tab.Token).Handle != sam.Handle {
		t.Fatal("tablet did not become Sam")
	}
	if w := call(hd, "POST", "/app/api/device/redeem", "", `{"code":"`+dc.Code+`"}`); !strings.Contains(w.Body.String(), "used") {
		t.Fatalf("a device code worked twice: %s", w.Body.String())
	}
}
