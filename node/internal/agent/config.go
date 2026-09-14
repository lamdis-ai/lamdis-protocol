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
	// "*.sec.gov"). Empty means autonomous runs have no web at all. A person
	// chatting can ask it to fetch any public page; that fetch is recorded.
	AllowDomains []string `json:"allow_domains"`
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
}

// ToolServer is one external MCP server the agent may call.
type ToolServer struct {
	Name    string   `json:"name"`
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	Env     []string `json:"env,omitempty"`
	URL     string   `json:"url,omitempty"`
	Allow   []string `json:"allow"`
	Confirm []string `json:"confirm,omitempty"`
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
