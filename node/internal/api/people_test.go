package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func start(t *testing.T, h *Host, srvURL string) string {
	t.Helper()
	w := call(h.Handler(), "POST", "/app/api/start", "", "")
	var d struct {
		Token string `json:"token"`
	}
	json.Unmarshal(w.Body.Bytes(), &d)
	if d.Token == "" {
		t.Fatalf("start: %s", w.Body.String())
	}
	return d.Token
}

// Sam finds Ray by handle, invites him into one channel of a project, and
// Ray's account ends up with that channel and only that channel, pulled
// over the ordinary signed sync.
func TestInvitingSomebodyByHandle(t *testing.T) {
	h, _, _, _ := testHost(t)
	h.Guests, h.MaxAccounts = true, 10
	srv := httptest.NewServer(h.Handler())
	defer srv.Close()
	h.PublicBase = srv.URL
	hd := h.Handler()

	sam, ray := start(t, h, srv.URL), start(t, h, srv.URL)
	call(hd, "POST", "/app/api/me", sam, `{"name":"Sam Rivera"}`)
	call(hd, "POST", "/app/api/me", ray, `{"name":"Ray"}`)
	if w := call(hd, "POST", "/app/api/handle", ray, `{"handle":"@ray.builds"}`); !strings.Contains(w.Body.String(), `"ray.builds"`) {
		t.Fatalf("handle: %s", w.Body.String())
	}
	if w := call(hd, "POST", "/app/api/handle", sam, `{"handle":"ray.builds"}`); !strings.Contains(w.Body.String(), "taken") {
		t.Fatalf("a taken handle was given twice: %s", w.Body.String())
	}
	if w := call(hd, "GET", "/app/api/people?q=ray", sam, ""); !strings.Contains(w.Body.String(), `"handle":"ray.builds"`) {
		t.Fatalf("search: %s", w.Body.String())
	}

	// A project with two tilers; Ray is invited into one.
	pj := call(hd, "POST", "/app/api/projects", sam, `{"title":"Kitchen reno"}`)
	var p struct{ ID string }
	json.Unmarshal(pj.Body.Bytes(), &p)
	mk := func(title string) string {
		w := call(hd, "POST", "/app/api/project/"+p.ID+"/channels", sam, `{"title":"`+title+`"}`)
		var c struct{ ID string }
		json.Unmarshal(w.Body.Bytes(), &c)
		return c.ID
	}
	mine, other := mk("tiler-ray"), mk("tiler-rival")
	call(hd, "POST", "/app/api/post", sam, `{"thread":"`+mine+`","text":"Ray, can you do 7 October?","lane":"content"}`)
	call(hd, "POST", "/app/api/post", sam, `{"thread":"`+other+`","text":"Rival quotes 2,450.","lane":"content"}`)

	if w := call(hd, "POST", "/app/api/invite", ray, `{"thread":"`+mine+`","handle":"@sam"}`); !strings.Contains(w.Body.String(), "error") {
		t.Fatalf("someone who does not own the channel invited people: %s", w.Body.String())
	}
	if w := call(hd, "POST", "/app/api/invite", sam, `{"thread":"`+mine+`","handle":"@ray.builds"}`); !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("invite: %s", w.Body.String())
	}
	w := call(hd, "GET", "/app/api/invites", ray, "")
	var inv struct {
		Invites []struct{ ID, Title string } `json:"invites"`
	}
	json.Unmarshal(w.Body.Bytes(), &inv)
	if len(inv.Invites) != 1 || inv.Invites[0].Title != "tiler-ray" {
		t.Fatalf("invites: %s", w.Body.String())
	}
	if w := call(hd, "POST", "/app/api/invites/answer", ray, `{"id":"`+inv.Invites[0].ID+`","accept":true}`); !strings.Contains(w.Body.String(), mine) {
		t.Fatalf("accept: %s", w.Body.String())
	}

	// Ray now has the channel, with Sam's message in it, and nothing else.
	var got string
	for i := 0; i < 20; i++ {
		got = call(hd, "GET", "/app/api/thread/"+mine, ray, "").Body.String()
		if strings.Contains(got, "7 October") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !strings.Contains(got, "7 October") {
		t.Fatalf("the invited channel did not arrive: %s", got)
	}
	for _, id := range []string{other, p.ID} {
		if w := call(hd, "GET", "/app/api/thread/"+id, ray, ""); strings.Contains(w.Body.String(), "2,450") || w.Code == 200 {
			t.Fatalf("Ray could open %s: %d %s", id, w.Code, w.Body.String())
		}
	}
	// And what Ray writes reaches Sam.
	call(hd, "POST", "/app/api/post", ray, `{"thread":"`+mine+`","text":"Yes, 7 October works.","lane":"content"}`)
	for _, a := range h.accounts {
		if a.Sched != nil && a.Sched.Sync != nil {
			a.Sched.Sync(t.Context())
		}
	}
	if !strings.Contains(call(hd, "GET", "/app/api/thread/"+mine, sam, "").Body.String(), "7 October works") {
		t.Fatal("Ray's reply did not reach Sam")
	}
}

// A join link works once, for one person.
func TestAJoinLinkIsSingleUse(t *testing.T) {
	h, _, _, _ := testHost(t)
	h.Guests, h.MaxAccounts = true, 10
	srv := httptest.NewServer(h.Handler())
	defer srv.Close()
	h.PublicBase = srv.URL
	hd := h.Handler()
	sam, ray, eve := start(t, h, srv.URL), start(t, h, srv.URL), start(t, h, srv.URL)
	w := call(hd, "POST", "/app/api/threads", sam, `{"title":"quotes"}`)
	var c struct{ ID string }
	json.Unmarshal(w.Body.Bytes(), &c)
	w = call(hd, "POST", "/app/api/joinlink", sam, `{"thread":"`+c.ID+`"}`)
	var l struct{ URL string }
	json.Unmarshal(w.Body.Bytes(), &l)
	code := l.URL[strings.Index(l.URL, "join=")+5:]
	if w := call(hd, "POST", "/app/api/join", ray, `{"code":"`+code+`"}`); !strings.Contains(w.Body.String(), c.ID) {
		t.Fatalf("join: %s", w.Body.String())
	}
	if w := call(hd, "POST", "/app/api/join", eve, `{"code":"`+code+`"}`); !strings.Contains(w.Body.String(), "used") {
		t.Fatalf("a used link worked again: %s", w.Body.String())
	}
}

// A passkey session names one account, expires, and cannot be forged.
func TestPasskeySessions(t *testing.T) {
	h, _, _, _ := testHost(t)
	tok, _ := h.mintSession("gabc", h.now().Add(time.Hour))
	if id, ok := h.readSession(tok); !ok || id != "gabc" {
		t.Fatal("a good session did not read")
	}
	if _, ok := h.readSession(strings.Replace(tok, "gabc", "gxyz", 1)); ok {
		t.Fatal("a session was moved to another account")
	}
	old, _ := h.mintSession("gabc", h.now().Add(-time.Minute))
	if _, ok := h.readSession(old); ok {
		t.Fatal("an expired session was accepted")
	}
}
