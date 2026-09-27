package agent

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The agent reads a page as numbered things, types into a field, and the
// button that buys only presses after the person's yes, on the page they saw.
func TestBrowserReadTypeAndConfirm(t *testing.T) {
	if localChrome() == "" {
		t.Skip("no local Chrome")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bought" {
			fmt.Fprint(w, `<h1>Order placed for `+r.URL.Query().Get("who")+`</h1>`)
			return
		}
		fmt.Fprint(w, `<h1>Tickets</h1><form action="/bought"><label>Name <input name="who"></label><button>Buy now</button></form>`)
	}))
	defer srv.Close()
	browserAllowPrivate = true
	defer func() { browserAllowPrivate = false }()

	p := NewBrowserPool("")
	dir := t.TempDir()
	s, err := p.Session(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Forget(dir)
	if err := s.Open(srv.URL); err != nil {
		t.Fatal(err)
	}
	page, err := s.Read()
	if err != nil || !strings.Contains(page, "Tickets") || !strings.Contains(page, `[1] input "Name"`) || !strings.Contains(page, `button "Buy now"`) {
		t.Fatalf("read: %v\n%s", err, page)
	}
	if err := s.Type(1, "Sam", false); err != nil {
		t.Fatal(err)
	}
	s.Read()

	r := &Runner{Browser: p, DataDir: dir}
	u, _ := s.URL()
	// The page changed since the person saw it: nothing is pressed.
	if out := r.pressConfirmed(map[string]any{"id": float64(2), "url": u + "?elsewhere"}); !strings.Contains(out, "nothing was pressed") {
		t.Fatalf("pressed on a different page: %s", out)
	}
	if out := r.pressConfirmed(map[string]any{"id": float64(2), "url": u}); !strings.Contains(out, "Order placed for Sam") {
		t.Fatalf("confirmed press: %s", out)
	}
}

func TestBrowserRefusesPrivateAddresses(t *testing.T) {
	for _, u := range []string{"http://127.0.0.1:8080/", "http://169.254.169.254/latest/meta-data", "http://10.0.0.5/", "file:///etc/passwd"} {
		if _, err := browsableURL(u); err == nil {
			t.Errorf("%s was allowed", u)
		}
	}
	s := &BrowserSession{}
	if s.allowedURL("http://169.254.170.2/v2/credentials") {
		t.Error("a page could reach the task's credentials endpoint")
	}
	if !s.allowedURL("data:text/plain,hi") {
		t.Error("data: URLs are part of ordinary pages")
	}
}
