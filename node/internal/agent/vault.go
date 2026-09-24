package agent

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// The vault: every credential a node holds is sealed before it touches disk.
//
// agent.json keeps the shape it always had, so a person can still read their
// own configuration, but the values that would let someone act as them (a
// service's token, a refresh token, a model key) are stored as
// "vault:v1:<nonce+ciphertext>" under AES-256-GCM.
//
// Where the key comes from decides what the seal protects against:
//
//   - On a hosted machine, LAMDIS_VAULT_KEY is a master key kept outside the
//     data volume (a secrets manager). Each account's key is derived from it
//     and the account's directory name, so a copy of the volume, a backup or
//     one account's files is not enough to use anybody's connections, and no
//     account's key opens another's.
//   - On a person's own machine there is no secrets manager, so the key is a
//     file beside agent.json (vault.key, 0600). That keeps secrets out of
//     anything that copies or prints the config alone; the machine itself is
//     still the boundary, as it always was.
//
// Values written before the vault existed are read as they are and sealed on
// the next save. A value that cannot be opened (the key changed) reads as
// empty: the connection asks to be signed in again rather than failing.

const sealPrefix = "vault:v1:"

// vaultKey returns the key for one data directory.
func vaultKey(dataDir string) ([]byte, error) {
	if m := strings.TrimSpace(os.Getenv("LAMDIS_VAULT_KEY")); m != "" {
		master, err := base64.StdEncoding.DecodeString(m)
		if err != nil || len(master) < 32 {
			return nil, errors.New("LAMDIS_VAULT_KEY must be at least 32 bytes, base64")
		}
		mac := hmac.New(sha256.New, master)
		mac.Write([]byte("lamdis-vault-v1\x00" + filepath.Base(filepath.Clean(dataDir))))
		return mac.Sum(nil), nil
	}
	p := filepath.Join(dataDir, "vault.key")
	if raw, err := os.ReadFile(p); err == nil && len(raw) == 32 {
		return raw, nil
	}
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(p, k, 0o600); err != nil {
		return nil, err
	}
	return k, nil
}

func seal(key []byte, plain string) (string, error) {
	if plain == "" || strings.HasPrefix(plain, sealPrefix) {
		return plain, nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := gcm.Seal(nonce, nonce, []byte(plain), []byte("lamdis-secret"))
	return sealPrefix + base64.RawURLEncoding.EncodeToString(out), nil
}

func unseal(key []byte, stored string) string {
	if !strings.HasPrefix(stored, sealPrefix) {
		return stored // written before the vault; sealed on the next save
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(stored, sealPrefix))
	if err != nil {
		return ""
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return ""
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(raw) < gcm.NonceSize() {
		return ""
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], []byte("lamdis-secret"))
	if err != nil {
		return ""
	}
	return string(plain)
}

// secrets lists every credential field in a config, so sealing and opening
// cannot drift apart.
func secrets(c *Config) []*string {
	out := []*string{&c.OpenRouterKey, &c.ModelURLKey}
	for i := range c.Tools {
		out = append(out, &c.Tools[i].Auth)
		if o := c.Tools[i].OAuth; o != nil {
			out = append(out, &o.Access, &o.Refresh, &o.ClientSecret)
		}
	}
	return out
}

// sealedCopy returns c with every secret sealed, leaving c itself untouched
// (its tool list and OAuth records are copied, not shared).
func sealedCopy(dataDir string, c Config) (Config, error) {
	cp := c
	cp.Tools = make([]ToolServer, len(c.Tools))
	copy(cp.Tools, c.Tools)
	for i := range cp.Tools {
		if o := cp.Tools[i].OAuth; o != nil {
			oc := *o
			cp.Tools[i].OAuth = &oc
		}
	}
	ptrs := secrets(&cp)
	needs := false
	for _, p := range ptrs {
		if *p != "" {
			needs = true
		}
	}
	if !needs {
		return cp, nil
	}
	key, err := vaultKey(dataDir)
	if err != nil {
		return cp, err
	}
	for _, p := range ptrs {
		s, err := seal(key, *p)
		if err != nil {
			return cp, err
		}
		*p = s
	}
	return cp, nil
}

// openSecrets unseals every secret in c in place.
func openSecrets(dataDir string, c *Config) {
	ptrs := secrets(c)
	sealed := false
	for _, p := range ptrs {
		if strings.HasPrefix(*p, sealPrefix) {
			sealed = true
		}
	}
	if !sealed {
		return
	}
	key, err := vaultKey(dataDir)
	if err != nil {
		for _, p := range ptrs {
			if strings.HasPrefix(*p, sealPrefix) {
				*p = ""
			}
		}
		return
	}
	for _, p := range ptrs {
		*p = unseal(key, *p)
	}
}
