package agent

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every credential is sealed on disk and opens again on load.
func TestTheVaultSealsEveryCredential(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LAMDIS_VAULT_KEY", "")
	cfg := Config{OpenRouterKey: "sk-or-secret", Tools: []ToolServer{{Name: "notion", URL: "https://mcp.notion.com/mcp",
		Auth: "plain-token", OAuth: &OAuthConfig{Access: "access-1", Refresh: "refresh-1", ClientSecret: "cs"}}}}
	if err := SaveConfig(dir, cfg); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "agent.json"))
	for _, secret := range []string{"sk-or-secret", "plain-token", "access-1", "refresh-1", `"cs"`} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("%s was written in the clear", secret)
		}
	}
	if cfg.Tools[0].OAuth.Access != "access-1" {
		t.Fatal("saving sealed the caller's own copy")
	}
	got, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.OpenRouterKey != "sk-or-secret" || got.Tools[0].Auth != "plain-token" || got.Tools[0].OAuth.Refresh != "refresh-1" || got.Tools[0].OAuth.ClientSecret != "cs" {
		t.Fatalf("did not open again: %+v %+v", got, got.Tools[0].OAuth)
	}
}

// With a master key, each account's key is its own: one account's sealed
// secret does not open under another's.
func TestAccountsDoNotShareAVaultKey(t *testing.T) {
	t.Setenv("LAMDIS_VAULT_KEY", base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	root := t.TempDir()
	a, b := filepath.Join(root, "acct-a"), filepath.Join(root, "acct-b")
	os.MkdirAll(a, 0o700)
	os.MkdirAll(b, 0o700)
	if err := SaveConfig(a, Config{OpenRouterKey: "only-a"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(a, "agent.json"))
	os.WriteFile(filepath.Join(b, "agent.json"), raw, 0o600)
	if got, _ := LoadConfig(b); got.OpenRouterKey != "" {
		t.Fatal("account b opened account a's secret")
	}
	if got, _ := LoadConfig(a); got.OpenRouterKey != "only-a" {
		t.Fatal("account a could not open its own secret")
	}
	if _, err := os.Stat(filepath.Join(a, "vault.key")); err == nil {
		t.Fatal("a key file was written next to the data even though a master key exists")
	}
}

// A config written before the vault still reads, and is sealed next save.
func TestOldPlainConfigsStillWork(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LAMDIS_VAULT_KEY", "")
	os.WriteFile(filepath.Join(dir, "agent.json"), []byte(`{"openrouter_key":"sk-old"}`), 0o600)
	c, _ := LoadConfig(dir)
	if c.OpenRouterKey != "sk-old" {
		t.Fatal("an old plain key was lost")
	}
	SaveConfig(dir, c)
	raw, _ := os.ReadFile(filepath.Join(dir, "agent.json"))
	if strings.Contains(string(raw), "sk-old") {
		t.Fatal("the old key was not sealed on save")
	}
}

func TestTheCatalogMatchesWholeWords(t *testing.T) {
	names := func(q string) string {
		var out []string
		for _, e := range Catalog {
			if matches(q, e) {
				out = append(out, e.Name)
			}
		}
		return strings.Join(out, ",")
	}
	if got := names("blink cameras"); got != "Blink" {
		t.Fatalf("blink cameras -> %q", got)
	}
	if got := names("facebook"); !strings.Contains(got, "Meta Ads") || !strings.Contains(got, "Facebook") {
		t.Fatalf("facebook -> %q", got)
	}
	if got := names("metabase"); got != "" {
		t.Fatalf("metabase matched %q", got)
	}
	if got := names("my notion workspace"); got != "Notion" {
		t.Fatalf("notion -> %q", got)
	}
}

func TestRegistryListingsAreRankedAndFiltered(t *testing.T) {
	raw := []byte(`{"servers":[
	 {"server":{"name":"ai.smithery/smithery-notion","remotes":[{"type":"streamable-http","url":"https://server.smithery.ai/@smithery/notion/mcp","headers":[{"name":"Authorization","value":"Bearer {smithery_api_key}"}]}]}},
	 {"server":{"name":"io.example/notion-helper","description":"Helps with Notion. Extra.","remotes":[{"type":"streamable-http","url":"https://helper.example.io/mcp"}]}},
	 {"server":{"name":"com.notion/mcp","title":"Notion","remotes":[{"type":"streamable-http","url":"https://mcp.notion.com/mcp"}]}},
	 {"server":{"name":"com.mcparmory/notion","packages":[{"registryType":"pypi"}]}}]}`)
	cs := rankRegistry("notion", parseRegistry(raw))
	if len(cs) != 2 {
		t.Fatalf("want the two usable remotes, got %+v", cs)
	}
	if cs[0].URL != "https://mcp.notion.com/mcp" || !strings.HasPrefix(cs[0].Note, "Published by notion.com.") {
		t.Fatalf("the vendor's own server was not first: %+v", cs[0])
	}
	if !strings.Contains(cs[1].Note, "not by the service itself") {
		t.Fatalf("a third party was not labelled: %+v", cs[1])
	}
}

// Connecting is offered only when the person is there to press the button.
func TestConnectToolsOnlyWhenSomeoneIsThere(t *testing.T) {
	r := &Runner{}
	for kind, want := range map[string]bool{TriggerChat: true, TriggerDecision: true, TriggerManual: true,
		TriggerSchedule: false, TriggerEntry: false, TriggerPeer: false, TriggerReflect: false, TriggerCode: false} {
		g := r.gateFor(Trigger{Kind: kind}, Brief{}, Config{AutoWeb: "listed"})
		if g.connect != want {
			t.Fatalf("%s: connect=%v", kind, g.connect)
		}
	}
}
