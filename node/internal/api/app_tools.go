package api

// Connections.
//
// Telling somebody to edit a JSON file is not configuration, and on a hosted
// node it is not even possible: the file is on a machine they will never see.
// So connecting a tool is a form. Paste the address, paste the credential,
// press test, and tick the things the agent may call.
//
// Two rules make this safe to offer. A credential goes in and never comes
// back out: the interface only ever learns whether one is set. And a hosted
// node connects by URL only, because "run this command" from a stranger is
// not a connection, it is a shell.

import (
	"context"
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/agent"
)

type serverOut struct {
	Name     string   `json:"name"`
	URL      string   `json:"url,omitempty"`
	Command  string   `json:"command,omitempty"`
	Allow    []string `json:"allow"`
	Confirm  []string `json:"confirm"`
	Disabled bool     `json:"disabled"`
	HasAuth  bool     `json:"has_auth"`
	SignedIn bool     `json:"signed_in"`
	Header   string   `json:"header,omitempty"`
}

func (a *App) handleToolsGet(w http.ResponseWriter, r *http.Request) {
	cfg, _ := agent.LoadConfig(a.DataDir)
	out := []serverOut{}
	for _, t := range cfg.Tools {
		out = append(out, serverOut{Name: t.Name, URL: t.URL, Command: t.Command,
			Allow: nonNil(t.Allow), Confirm: nonNil(t.Confirm), Disabled: t.Disabled,
			HasAuth: t.Credentialed(), Header: t.Header, SignedIn: t.OAuth.Connected()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, map[string]any{"servers": out, "allow_commands": !a.NoCommands,
		"may_hold_secrets": a.mayHoldSecrets()})
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

type toolIn struct {
	Name     string   `json:"name"`
	URL      string   `json:"url"`
	Command  string   `json:"command"`
	Args     []string `json:"args"`
	Auth     string   `json:"auth"`
	Header   string   `json:"header"`
	Allow    []string `json:"allow"`
	Confirm  []string `json:"confirm"`
	Disabled bool     `json:"disabled"`
}

// server turns what the form sent into a connection, keeping any credential
// already stored when the form left that field blank.
func (a *App) server(in toolIn, existing *agent.ToolServer) (agent.ToolServer, error) {
	s := agent.ToolServer{
		Name: strings.TrimSpace(in.Name), URL: strings.TrimSpace(in.URL),
		Command: strings.TrimSpace(in.Command), Args: in.Args,
		Header: strings.TrimSpace(in.Header), Allow: nonNil(in.Allow),
		Confirm: nonNil(in.Confirm), Disabled: in.Disabled,
	}
	if s.Name == "" {
		return s, errText("give the connection a short name, like github or mail")
	}
	if s.URL == "" && s.Command == "" {
		return s, errText("paste the address of the server, e.g. https://mcp.example.com/mcp")
	}
	if s.Command != "" && a.NoCommands {
		return s, errText("this node connects to tools by address only. Run Lamdis on your own machine to use a local command.")
	}
	if s.URL != "" {
		local := strings.HasPrefix(s.URL, "http://localhost") || strings.HasPrefix(s.URL, "http://127.0.0.1")
		if local && a.NoCommands {
			return s, errText("this node cannot reach an address on its own machine; give it one on the internet")
		}
		if !strings.HasPrefix(s.URL, "https://") && !local {
			return s, errText("the address must start with https://")
		}
	}
	switch {
	case strings.TrimSpace(in.Auth) != "":
		if !a.mayHoldSecrets() {
			return s, errText("Add your email first. A credential stored against a browser tab is one nobody could revoke if that tab were lost, so this node will not keep one until there is an account behind it.")
		}
		s.Auth = strings.TrimSpace(in.Auth)
	case existing != nil:
		s.Auth = existing.Auth
	}
	if existing != nil {
		s.OAuth = existing.OAuth
	}
	return s, nil
}

type textErr string

func (e textErr) Error() string { return string(e) }
func errText(s string) error    { return textErr(s) }

func (a *App) handleToolsSet(w http.ResponseWriter, r *http.Request) {
	var in toolIn
	if json.NewDecoder(io.LimitReader(r.Body, 1<<18)).Decode(&in) != nil {
		http.Error(w, "bad connection", http.StatusBadRequest)
		return
	}
	cfg, _ := agent.LoadConfig(a.DataDir)
	var existing *agent.ToolServer
	for i := range cfg.Tools {
		if cfg.Tools[i].Name == strings.TrimSpace(in.Name) {
			existing = &cfg.Tools[i]
		}
	}
	s, err := a.server(in, existing)
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	if existing != nil {
		*existing = s
	} else {
		if len(cfg.Tools) >= 12 {
			writeJSON(w, map[string]any{"error": "that is as many connections as one node keeps"})
			return
		}
		cfg.Tools = append(cfg.Tools, s)
	}
	if err := agent.SaveConfig(a.DataDir, cfg); err != nil {
		http.Error(w, "could not save", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "name": s.Name})
}

// handleToolsProbe connects and reports what the server offers, so nobody
// has to guess the names of the things they are about to allow.
func (a *App) handleToolsProbe(w http.ResponseWriter, r *http.Request) {
	var in toolIn
	if json.NewDecoder(io.LimitReader(r.Body, 1<<18)).Decode(&in) != nil {
		http.Error(w, "bad connection", http.StatusBadRequest)
		return
	}
	cfg, _ := agent.LoadConfig(a.DataDir)
	var existing *agent.ToolServer
	for i := range cfg.Tools {
		if cfg.Tools[i].Name == strings.TrimSpace(in.Name) {
			existing = &cfg.Tools[i]
		}
	}
	s, err := a.server(in, existing)
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	names, err := agent.Probe(ctx, s, !a.NoCommands)
	if err != nil {
		writeJSON(w, map[string]any{"error": friendlyProbeError(err)})
		return
	}
	sort.Strings(names)
	writeJSON(w, map[string]any{"tools": names})
}

// friendlyProbeError says what a person can do about it.
func friendlyProbeError(err error) string {
	s := err.Error()
	switch {
	case strings.Contains(s, "401") || strings.Contains(s, "403") || strings.Contains(strings.ToLower(s), "unauthor"):
		return "The server refused the credential. Check the token, or whether it wants a header other than Authorization."
	case strings.Contains(s, "no such host") || strings.Contains(s, "cannot resolve"):
		return "That address does not resolve. Check it for a typo."
	case strings.Contains(s, "private address"):
		return "That address is on a private network, which this node will not reach."
	case strings.Contains(s, "deadline") || strings.Contains(s, "timeout"):
		return "The server did not answer in time."
	}
	return s
}

func (a *App) handleToolsRemove(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in) != nil || in.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	cfg, _ := agent.LoadConfig(a.DataDir)
	kept := cfg.Tools[:0]
	for _, t := range cfg.Tools {
		if t.Name != in.Name {
			kept = append(kept, t)
		}
	}
	cfg.Tools = kept
	if err := agent.SaveConfig(a.DataDir, cfg); err != nil {
		http.Error(w, "could not save", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// --- signing in to a server, rather than pasting its secret -------------
//
// The browser does the approving, this only carries the paperwork: discover
// how the server wants to be asked, introduce ourselves, send the person
// over, and keep what comes back. The person never sees a token.

type pendingAuth struct {
	account  string
	name     string
	verifier string
	at       time.Time
}

var authWaiting sync.Map // state -> pendingAuth

// redirectURI is where the authorization server sends people back. It has
// to match what we registered, so it is derived the same way every time.
func (a *App) redirectURI(r *http.Request) string {
	if a.PublicBase != "" {
		return strings.TrimRight(a.PublicBase, "/") + "/app/api/tools/auth/done"
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/app/api/tools/auth/done"
}

// handleToolsAuthStart discovers how a server wants to be signed in to and
// returns the address to send the person to.
func (a *App) handleToolsAuthStart(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in) != nil || strings.TrimSpace(in.Name) == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if !a.mayHoldSecrets() {
		writeJSON(w, map[string]any{"error": "Add your email first. Signing in to a service leaves a credential here, and this node will not keep one for an account nobody can recover."})
		return
	}
	cfg, _ := agent.LoadConfig(a.DataDir)
	idx := -1
	for i := range cfg.Tools {
		if cfg.Tools[i].Name == strings.TrimSpace(in.Name) {
			idx = i
		}
	}
	if idx < 0 {
		writeJSON(w, map[string]any{"error": "save the connection first"})
		return
	}
	url := cfg.Tools[idx].URL
	if strings.TrimSpace(in.URL) != "" {
		url = strings.TrimSpace(in.URL)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	oc, err := agent.DiscoverOAuth(ctx, url, !a.NoCommands)
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	if oc == nil {
		writeJSON(w, map[string]any{"none": true,
			"error": "That server did not ask anyone to sign in. Either it needs nothing, or it wants a token pasted in the field above."})
		return
	}
	redirect := a.redirectURI(r)
	if err := oc.Register(ctx, redirect, "Lamdis"); err != nil {
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	verifier := agent.Verifier()
	state := agent.Verifier()[:32]
	authWaiting.Store(state, pendingAuth{account: a.DataDir, name: cfg.Tools[idx].Name, verifier: verifier, at: time.Now()})
	cfg.Tools[idx].OAuth = oc
	if err := agent.SaveConfig(a.DataDir, cfg); err != nil {
		http.Error(w, "could not save", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"authorize": oc.Authorize(redirect, verifier, state), "issuer": oc.Issuer})
}

// handleToolsAuthDone is where the browser comes back.
func (a *App) handleToolsAuthDone(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	fail := func(msg string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(authClosePage(false, msg)))
	}
	if e := q.Get("error"); e != "" {
		fail(firstNonEmpty(q.Get("error_description"), e))
		return
	}
	code, state := q.Get("code"), q.Get("state")
	v, ok := authWaiting.Load(state)
	if !ok || code == "" {
		fail("That sign-in has expired. Try connecting again.")
		return
	}
	authWaiting.Delete(state)
	p := v.(pendingAuth)
	if time.Since(p.at) > 15*time.Minute {
		fail("That sign-in took too long. Try again.")
		return
	}
	cfg, _ := agent.LoadConfig(p.account)
	idx := -1
	for i := range cfg.Tools {
		if cfg.Tools[i].Name == p.name {
			idx = i
		}
	}
	if idx < 0 || cfg.Tools[idx].OAuth == nil {
		fail("That connection is no longer here.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	if err := cfg.Tools[idx].OAuth.ExchangeCode(ctx, code, p.verifier, a.redirectURI(r)); err != nil {
		fail(err.Error())
		return
	}
	if err := agent.SaveConfig(p.account, cfg); err != nil {
		fail("could not save the connection")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(authClosePage(true, cfg.Tools[idx].Name)))
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// authClosePage is the last thing the person sees before the window shuts.
func authClosePage(ok bool, detail string) string {
	title, body := "Connected", template.HTMLEscapeString(detail)+" is connected. You can close this window."
	if !ok {
		title, body = "Not connected", template.HTMLEscapeString(detail)
	}
	return `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="dark">
<title>` + title + `</title><style>` + appCSS + `</style></head>
<body><div class="notice"><h1>` + title + `</h1><p>` + body + `</p></div>
<script>try{ if(window.opener){ window.opener.postMessage({lamdis:"tools-auth"}, location.origin); setTimeout(function(){window.close()}, 1200) } }catch(e){}</script>
</body></html>`
}
