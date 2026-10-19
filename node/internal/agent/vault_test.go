package agent

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

// A hosted account sealed under its own key file keeps its secrets when the
// host gains a master key, and the key file beside them goes away.
func TestMovingToAMasterKeyLosesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "acct1")
	t.Setenv("LAMDIS_VAULT_KEY", "")
	if err := SaveConfig(dir, Config{OpenRouterKey: "sk-or-theirs"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vault.key")); err != nil {
		t.Fatal("expected a key file before the master key existed")
	}

	t.Setenv("LAMDIS_VAULT_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	if c, _ := LoadConfig(dir); c.OpenRouterKey != "sk-or-theirs" {
		t.Fatalf("an old seal stopped opening once a master key was set: %q", c.OpenRouterKey)
	}
	moved, err := MoveToMasterKey(dir)
	if err != nil || !moved {
		t.Fatalf("moved=%v err=%v", moved, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vault.key")); !os.IsNotExist(err) {
		t.Fatal("the key file is still beside the secrets")
	}
	if c, _ := LoadConfig(dir); c.OpenRouterKey != "sk-or-theirs" {
		t.Fatalf("the key was lost in the move: %q", c.OpenRouterKey)
	}
	raw, _ := os.ReadFile(configPath(dir))
	if bytes.Contains(raw, []byte("sk-or-theirs")) {
		t.Fatal("the key is readable in agent.json")
	}
	if again, _ := MoveToMasterKey(dir); again {
		t.Fatal("a second start moved it again")
	}

	// Without the master key the volume alone opens nothing.
	t.Setenv("LAMDIS_VAULT_KEY", "")
	if c, _ := LoadConfig(dir); c.OpenRouterKey != "" {
		t.Fatal("a copy of the volume opened the key without the master key")
	}
}
