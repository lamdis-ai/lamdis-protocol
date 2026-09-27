package api

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http/httptest"
	"strings"
	"testing"
)

func tinyPNG(t *testing.T) string {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 1, color.RGBA{255, 0, 0, 255})
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.String()
}

// Sam attaches a photo to a channel; it lands on the message, opens from its
// signed address for anyone the channel reaches, and nothing else does.
func TestAttachingAFile(t *testing.T) {
	h, _, _, _ := testHost(t)
	h.Guests, h.MaxAccounts = true, 10
	srv := httptest.NewServer(h.Handler())
	defer srv.Close()
	hd := h.Handler()
	sam := start(t, h, srv.URL)

	var c struct{ ID string }
	json.Unmarshal(call(hd, "POST", "/app/api/threads", sam, `{"title":"kitchen"}`).Body.Bytes(), &c)
	if c.ID == "" {
		t.Fatal("no channel")
	}
	w := call(hd, "POST", "/app/api/upload?name=tile%20sample.png", sam, tinyPNG(t))
	var f FileRef
	json.Unmarshal(w.Body.Bytes(), &f)
	if f.Type != "image/png" || !strings.HasPrefix(f.URL, "/f/") {
		t.Fatalf("upload: %s", w.Body.String())
	}
	if w := call(hd, "POST", "/app/api/upload?name=x.html", sam, "<html><script>alert(1)</script>"); w.Code != 415 {
		t.Fatalf("an html upload was accepted: %d %s", w.Code, w.Body.String())
	}
	if w := call(hd, "POST", "/app/api/post", sam, `{"thread":"`+c.ID+`","text":"which grout?","files":["`+f.ID+`","nonsense"]}`); w.Code != 200 {
		t.Fatalf("post: %d %s", w.Code, w.Body.String())
	}
	body := call(hd, "GET", "/app/api/thread/"+c.ID, sam, "").Body.String()
	if !strings.Contains(body, `"name":"tile sample.png"`) || strings.Contains(body, "nonsense") {
		t.Fatalf("thread does not carry the file right: %s", body)
	}
	// The signed address works with no token at all; a changed one does not.
	if w := call(hd, "GET", f.URL, "", ""); w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("signed file: %d %v", w.Code, w.Header())
	}
	bad := f.URL[:len(f.URL)-2] + "xx"
	if w := call(hd, "GET", bad, "", ""); w.Code != 404 {
		t.Fatalf("a forged address opened the file: %d", w.Code)
	}
	// The host resolves it for another account's agent the same way.
	if b, err := h.openFile(f.URL, f.ID); err != nil || len(b) == 0 {
		t.Fatalf("openFile: %v", err)
	}
	if _, err := h.openFile(bad, f.ID); err == nil {
		t.Fatal("openFile followed a forged address")
	}
}
