package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The interface is for the person at the machine. Exposing the node so that
// peers can sync must not put the owner's threads on the network behind a
// single bearer token, so owner routes refuse anything that did not arrive
// over loopback unless the person asked for that explicitly.
func TestInterfaceAnswersOnlyThisMachine(t *testing.T) {
	a := testApp(t)
	hit := false
	h := a.owner(func(w http.ResponseWriter, r *http.Request) { hit = true })

	remote := httptest.NewRequest("GET", "/app/api/threads", nil)
	remote.RemoteAddr = "192.168.1.50:51234"
	remote.Header.Set("Authorization", "Bearer owner-token")
	w := httptest.NewRecorder()
	h(w, remote)
	if hit || w.Code != http.StatusForbidden {
		t.Fatalf("a request from the network reached the interface: code=%d handled=%v", w.Code, hit)
	}

	local := httptest.NewRequest("GET", "/app/api/threads", nil)
	local.RemoteAddr = "127.0.0.1:51234"
	local.Header.Set("Authorization", "Bearer owner-token")
	w = httptest.NewRecorder()
	h(w, local)
	if !hit || w.Code != http.StatusOK {
		t.Fatalf("a request from this machine was refused: code=%d handled=%v", w.Code, hit)
	}

	// A correct loopback address with the wrong token is still nothing.
	hit = false
	bad := httptest.NewRequest("GET", "/app/api/threads", nil)
	bad.RemoteAddr = "127.0.0.1:51234"
	bad.Header.Set("Authorization", "Bearer not-the-token")
	w = httptest.NewRecorder()
	h(w, bad)
	if hit || w.Code != http.StatusUnauthorized {
		t.Fatalf("the token check stopped working: code=%d handled=%v", w.Code, hit)
	}

	// And with -expose-app, the person has said they want it reachable.
	a.ExposeApp = true
	hit = false
	w = httptest.NewRecorder()
	remote2 := httptest.NewRequest("GET", "/app/api/threads", nil)
	remote2.RemoteAddr = "192.168.1.50:51234"
	remote2.Header.Set("Authorization", "Bearer owner-token")
	h(w, remote2)
	if !hit {
		t.Fatal("-expose-app did not take effect")
	}
}
