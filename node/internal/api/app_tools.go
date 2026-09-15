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
	"io"
	"net/http"
	"sort"
	"strings"
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
	Header   string   `json:"header,omitempty"`
}

func (a *App) handleToolsGet(w http.ResponseWriter, r *http.Request) {
	cfg, _ := agent.LoadConfig(a.DataDir)
	out := []serverOut{}
	for _, t := range cfg.Tools {
		out = append(out, serverOut{Name: t.Name, URL: t.URL, Command: t.Command,
			Allow: nonNil(t.Allow), Confirm: nonNil(t.Confirm), Disabled: t.Disabled,
			HasAuth: t.Auth != "", Header: t.Header})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, map[string]any{"servers": out, "allow_commands": !a.NoCommands})
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
		if !strings.HasPrefix(s.URL, "https://") && !strings.HasPrefix(s.URL, "http://localhost") && !strings.HasPrefix(s.URL, "http://127.0.0.1") {
			return s, errText("the address must start with https://")
		}
	}
	switch {
	case strings.TrimSpace(in.Auth) != "":
		s.Auth = strings.TrimSpace(in.Auth)
	case existing != nil:
		s.Auth = existing.Auth
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
