package api

import (
	"encoding/json"
	"io"
	"net/http"
)

// The person's view of the agent's browser. The agent hands it over when a
// site needs something only the person may do, usually signing in: they see
// the live page, click and type into it themselves, and press Done. Their
// password goes to the site, never to the agent; what the site leaves behind
// is sealed in the account's vault by the browser itself.

func (a *App) registerBrowser(mux *http.ServeMux) {
	mux.HandleFunc("GET /app/api/browser", a.owner(a.handleBrowserState))
	mux.HandleFunc("GET /app/api/browser/shot", a.owner(a.handleBrowserShot))
	mux.HandleFunc("POST /app/api/browser/input", a.owner(a.handleBrowserInput))
	mux.HandleFunc("POST /app/api/browser/forget", a.owner(a.handleBrowserForget))
}

func (a *App) browserOK(w http.ResponseWriter) bool {
	if a.Runner == nil || a.Runner.Browser == nil {
		writeJSON(w, map[string]any{"error": "There is no browser on this host."})
		return false
	}
	return true
}

// GET /app/api/browser -> {available, open, url, title}
func (a *App) handleBrowserState(w http.ResponseWriter, r *http.Request) {
	if a.Runner == nil || a.Runner.Browser == nil {
		writeJSON(w, map[string]any{"available": false})
		return
	}
	s := a.Runner.Browser.Existing(a.DataDir)
	if s == nil {
		writeJSON(w, map[string]any{"available": true, "open": false})
		return
	}
	u, t := s.URL()
	writeJSON(w, map[string]any{"available": true, "open": true, "url": u, "title": t, "width": 1280, "height": 800})
}

// GET /app/api/browser/shot -> the page as a JPEG, starting a browser if needed.
func (a *App) handleBrowserShot(w http.ResponseWriter, r *http.Request) {
	if !a.browserOK(w) {
		return
	}
	s, err := a.Runner.Browser.Session(a.DataDir)
	if err != nil {
		writeStatusJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error()})
		return
	}
	img, err := s.Screenshot()
	if err != nil {
		writeStatusJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(img)
}

// POST /app/api/browser/input {kind: click|type|key|open|scroll|back|done, x, y, text, url, dy}
func (a *App) handleBrowserInput(w http.ResponseWriter, r *http.Request) {
	if !a.browserOK(w) {
		return
	}
	var in struct {
		Kind string  `json:"kind"`
		X    float64 `json:"x"`
		Y    float64 `json:"y"`
		Text string  `json:"text"`
		URL  string  `json:"url"`
		Down bool    `json:"down"`
	}
	json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&in)
	s, err := a.Runner.Browser.Session(a.DataDir)
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	switch in.Kind {
	case "click":
		if in.X < 0 || in.Y < 0 || in.X > 1280 || in.Y > 800 {
			writeJSON(w, map[string]any{"error": "outside the page"})
			return
		}
		err = s.ClickAt(in.X, in.Y)
	case "type":
		err = s.TypeText(in.Text)
	case "key":
		err = s.Keys(in.Text)
	case "open":
		err = s.Open(in.URL)
	case "scroll":
		err = s.Scroll(in.Down)
	case "back":
		err = s.Back()
	case "done":
		s.Save()
	default:
		writeJSON(w, map[string]any{"error": "unknown input"})
		return
	}
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	u, t := s.URL()
	writeJSON(w, map[string]any{"ok": true, "url": u, "title": t})
}

// POST /app/api/browser/forget clears saved sign-ins and closes the browser.
func (a *App) handleBrowserForget(w http.ResponseWriter, r *http.Request) {
	if !a.browserOK(w) {
		return
	}
	a.Runner.Browser.Forget(a.DataDir)
	writeJSON(w, map[string]any{"ok": true})
}
