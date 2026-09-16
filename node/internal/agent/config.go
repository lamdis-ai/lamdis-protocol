package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Config is <data>/agent.json: what the agent may reach outside the node and
// how much it may do in a day. Edited by hand or from Settings.
type Config struct {
	// AllowDomains are hosts the agent may fetch on its own (globs such as
	// "*.sec.gov").
	AllowDomains []string `json:"allow_domains"`
	// AutoWeb says how far the agent may reach when nobody asked it to:
	// "listed" (only AllowDomains, the default), "any" (any public page,
	// same as when you ask it yourself), or "off" (no web unless asked).
	// Asking it something yourself is always "any": the reach it has on its
	// own is the only part that needs a policy.
	AutoWeb string `json:"auto_web,omitempty"`
	// Tools are external MCP servers. Only the tools listed under allow are
	// shown to the model; tools listed under confirm need the person's yes
	// each time, which the agent asks for as a decision.
	Tools            []ToolServer `json:"tools"`
	MaxToolCalls     int          `json:"max_tool_calls"`
	MaxRunsPerDay    int          `json:"max_runs_per_day"`
	MaxFetchesPerDay int          `json:"max_fetches_per_day"`
	MaxTokensPerDay  int          `json:"max_tokens_per_day"`
	SyncEvery        string       `json:"sync_every"`
	// Brief is the node-wide standing instruction used when a thread has none.
	Brief string `json:"brief"`
	// Model is the model id (OpenRouter's, or whatever ModelURL serves).
	// Empty means the LAMDIS_MODEL environment or the default.
	Model string `json:"model,omitempty"`
	// OpenRouterKey lets the interface set the key; the environment wins
	// when both exist. The file is 0600 in the person's own data directory.
	OpenRouterKey string `json:"openrouter_key,omitempty"`
	// ModelURL is an OpenAI-compatible base URL for a local or private
	// model server, e.g. http://localhost:11434/v1. No key needed.
	ModelURL string `json:"model_url,omitempty"`
	// Trust is how much of this machine the agent may work in without
	// asking: "project" where you started it, "home" everything under your
	// home directory, "all" the whole machine. Set once, remembered.
	Trust string `json:"trust,omitempty"`
	// AllowPaths are directories you have already said yes to, so nobody is
	// asked the same question twice.
	AllowPaths []string `json:"allow_paths,omitempty"`
	// Unguarded turns off the refusal to read credential stores. Off by
	// default, because an agent you can direct from a phone reading your
	// ssh keys is a different proposition from one reading your code.
	Unguarded bool `json:"unguarded,omitempty"`
	// ModelURLKey is a credential for ModelURL alone. The OpenRouter key is
	// never sent anywhere but OpenRouter, so a private endpoint that needs
	// authentication gets its own.
	ModelURLKey string `json:"model_url_key,omitempty"`
}

// ToolServer is one external MCP server the agent may call.
//
// A server is reached either by running a command on this machine or by
// calling a URL. The first is only sensible where the person owns the
// machine; a hosted node refuses it, because "run this command" from a
// stranger is not a connection, it is a shell.
type ToolServer struct {
	Name    string   `json:"name"`
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	Env     []string `json:"env,omitempty"`
	URL     string   `json:"url,omitempty"`
	// Auth is the credential this server needs, sent as a bearer token on
	// every call. It is never returned by the interface, only replaced.
	Auth string `json:"auth,omitempty"`
	// Header names a header other than Authorization, for services that
	// want their own (e.g. X-Api-Key).
	Header string `json:"header,omitempty"`
	// OAuth is what the server told us about signing in, and whatever it
	// has granted. Present means nobody has to type a secret.
	OAuth    *OAuthConfig `json:"oauth,omitempty"`
	Allow    []string     `json:"allow"`
	Confirm  []string     `json:"confirm,omitempty"`
	Disabled bool         `json:"disabled,omitempty"`
}

// Credentialed reports whether this server has something to authenticate
// with, by either route.
func (t ToolServer) Credentialed() bool { return t.Auth != "" || t.OAuth.Connected() }

// Reachable reports whether this server can be used at all here.
func (t ToolServer) Reachable(allowCommands bool) bool {
	if t.Disabled {
		return false
	}
	if t.URL != "" {
		return true
	}
	return t.Command != "" && allowCommands
}

func configPath(dataDir string) string { return filepath.Join(dataDir, "agent.json") }

// LoadConfig reads agent.json with defaults filled in. A missing file is the
// defaults; a corrupt one is reported through the returned error but still
// yields usable defaults so the node keeps serving.
func LoadConfig(dataDir string) (Config, error) {
	c := Config{}
	raw, err := os.ReadFile(configPath(dataDir))
	var perr error
	if err == nil {
		if e := json.Unmarshal(raw, &c); e != nil {
			perr = e
		}
	}
	if c.MaxToolCalls <= 0 {
		c.MaxToolCalls = 12
	}
	if c.MaxRunsPerDay <= 0 {
		c.MaxRunsPerDay = 60
	}
	if c.MaxFetchesPerDay <= 0 {
		c.MaxFetchesPerDay = 100
	}
	if c.MaxTokensPerDay <= 0 {
		c.MaxTokensPerDay = 2_000_000
	}
	if c.SyncEvery == "" {
		c.SyncEvery = "2m"
	}
	if c.AllowDomains == nil {
		c.AllowDomains = []string{}
	}
	switch c.AutoWeb {
	case "listed", "any", "off":
	default:
		c.AutoWeb = "listed"
	}
	switch c.Trust {
	case "project", "home", "all":
	default:
		c.Trust = "project"
	}
	if c.Tools == nil {
		c.Tools = []ToolServer{}
	}
	return c, perr
}

// SaveConfig writes agent.json. Only the owner's process can reach the data
// directory, so the file is the permission boundary.
func SaveConfig(dataDir string, c Config) error {
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(dataDir), raw, 0o600)
}

// domainAllowed reports whether host matches one of the globs. "*.x.com"
// matches "a.x.com" and "x.com"; "x.com" matches only itself.
func domainAllowed(host string, globs []string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, g := range globs {
		g = strings.ToLower(strings.TrimSpace(g))
		if g == "" {
			continue
		}
		if g == "*" {
			return true
		}
		if strings.HasPrefix(g, "*.") {
			base := g[2:]
			if host == base || strings.HasSuffix(host, "."+base) {
				return true
			}
			continue
		}
		if host == g {
			return true
		}
	}
	return false
}
