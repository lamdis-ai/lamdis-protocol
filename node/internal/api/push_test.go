package api

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// A phone that allowed notifications hears what its agent said, signed the
// way Apple checks, no more than the day's cap, and a dead token is dropped.
func TestAgentRepliesReachThePhone(t *testing.T) {
	h, _, _, handler := testHost(t)
	h.Guests = true
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	var mu sync.Mutex
	var got []map[string]any
	var paths []string
	apple := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jwt := strings.TrimPrefix(r.Header.Get("authorization"), "bearer ")
		parts := strings.Split(jwt, ".")
		sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
		sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		if len(sig) != 64 || !ecdsa.Verify(&key.PublicKey, sum[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
			t.Error("the push was not signed with the key")
		}
		if r.Header.Get("apns-topic") != "ai.lamdis.app" {
			t.Errorf("topic %q", r.Header.Get("apns-topic"))
		}
		if strings.HasSuffix(r.URL.Path, strings.Repeat("b", 64)) {
			w.WriteHeader(http.StatusGone)
			io.WriteString(w, `{"reason":"Unregistered"}`)
			return
		}
		var p map[string]any
		json.NewDecoder(r.Body).Decode(&p)
		mu.Lock()
		got, paths = append(got, p), append(paths, r.URL.Path)
		mu.Unlock()
	}))
	defer apple.Close()
	h.push = &apns{keyID: "K1", team: "JNA2Z7W52M", topic: "ai.lamdis.app", key: key, client: apple.Client(), base: apple.URL}

	w := call(handler, "POST", "/app/api/start", "", "")
	var started struct{ Token string }
	json.Unmarshal(w.Body.Bytes(), &started)
	for _, tok := range []string{strings.Repeat("a", 64), strings.Repeat("b", 64)} {
		if w := call(handler, "POST", "/app/api/push", started.Token, `{"token":"`+tok+`"}`); w.Code != http.StatusOK {
			t.Fatalf("register: %d %s", w.Code, w.Body.String())
		}
	}
	if w := call(handler, "POST", "/app/api/push", started.Token, `{"token":"not a token"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("a junk token was kept: %d", w.Code)
	}
	if w := call(handler, "POST", "/app/api/push", "", `{"token":"`+strings.Repeat("c", 64)+`"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("a stranger registered a phone: %d", w.Code)
	}
	me, err := h.resolve(func() *http.Request {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Authorization", "Bearer "+started.Token)
		return r
	}())
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < pushesPerDay+3; i++ {
		h.notify(me.Dir, "th1", "# Notes", pushLine("**Roof:** the quote is in.\nMore detail below."))
	}
	if len(got) != pushesPerDay {
		t.Fatalf("%d pushes, want the cap of %d", len(got), pushesPerDay)
	}
	alert := got[0]["aps"].(map[string]any)["alert"].(map[string]any)
	if alert["title"] != "# Notes" || alert["body"] != "Roof: the quote is in." || got[0]["thread"] != "th1" {
		t.Fatalf("wrong push: %v", got[0])
	}
	if devs := readPush(me.Dir).Devices; len(devs) != 1 || devs[0].Token != strings.Repeat("a", 64) {
		t.Fatalf("the dead token was not dropped: %+v", devs)
	}
}
