package agent

// Handing someone inference without handing them your wallet.
//
// The shape that makes this safe is OpenRouter's own: a management key can
// mint ordinary API keys, each with a credit limit. A limit only refills if
// the key is created with limit_reset set, so leaving it unset gives a hard
// cap that never comes back. Mint one per person, cap it at what you are
// willing to lose, and the worst case for any account is exactly that number.
//
// The minted key goes to the person and lives on their machine, so their
// prompts still travel from their computer to OpenRouter and never through
// anything of ours. That is the point: the cap is the only thing we hold.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ProvisionedKey is one minted key. Secret is returned only at creation;
// afterwards a key is identified by its hash.
type ProvisionedKey struct {
	Hash      string  `json:"hash"`
	Name      string  `json:"name"`
	Secret    string  `json:"key,omitempty"`
	Limit     float64 `json:"limit"`
	Usage     float64 `json:"usage"`
	Remaining float64 `json:"limit_remaining"`
	Disabled  bool    `json:"disabled"`
	Created   string  `json:"created_at"`
}

// Spent is what this key has used, in dollars.
func (k ProvisionedKey) Spent() float64 { return k.Usage }

// Left is what remains under the cap.
func (k ProvisionedKey) Left() float64 {
	if k.Limit <= 0 {
		return 0
	}
	if k.Remaining != 0 {
		return k.Remaining
	}
	return k.Limit - k.Usage
}

// Provisioner mints and revokes capped keys with a management key.
type Provisioner struct {
	ManagementKey string
	HTTP          *http.Client
	// BaseURL is for tests; empty means OpenRouter.
	BaseURL string
}

func (p *Provisioner) base() string {
	if p.BaseURL != "" {
		return strings.TrimRight(p.BaseURL, "/")
	}
	return openRouterURL
}

func (p *Provisioner) client() *http.Client {
	if p.HTTP != nil {
		return p.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (p *Provisioner) do(ctx context.Context, method, path string, in any, out any) error {
	if strings.TrimSpace(p.ManagementKey) == "" {
		return fmt.Errorf("no management key: set LAMDIS_OPENROUTER_MANAGEMENT_KEY (create one at openrouter.ai/settings/management-keys)")
	}
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.base()+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.ManagementKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client().Do(req)
	if err != nil {
		return fmt.Errorf("could not reach OpenRouter: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(raw, &e) == nil && e.Error.Message != "" {
			return fmt.Errorf("%s", e.Error.Message)
		}
		return fmt.Errorf("OpenRouter returned HTTP %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// keyEnvelope handles both the bare object and the {"data": …} wrapper.
type keyEnvelope struct {
	Data *ProvisionedKey `json:"data"`
	ProvisionedKey
}

func (e keyEnvelope) key() ProvisionedKey {
	if e.Data != nil {
		// The secret sits beside the wrapper on creation.
		k := *e.Data
		if k.Secret == "" {
			k.Secret = e.ProvisionedKey.Secret
		}
		return k
	}
	return e.ProvisionedKey
}

// Mint creates a key capped at limit dollars. The cap never refills: no
// limit_reset is sent, which is the whole reason this is safe to hand out.
func (p *Provisioner) Mint(ctx context.Context, label string, limit float64) (ProvisionedKey, error) {
	if limit <= 0 {
		return ProvisionedKey{}, fmt.Errorf("a key without a cap is a blank cheque; pass a limit")
	}
	var env keyEnvelope
	if err := p.do(ctx, "POST", "/keys", map[string]any{"name": label, "limit": limit}, &env); err != nil {
		return ProvisionedKey{}, err
	}
	k := env.key()
	if k.Secret == "" {
		return k, fmt.Errorf("OpenRouter did not return the key; check openrouter.ai/settings/keys")
	}
	return k, nil
}

// List returns every key the management key can see, with usage.
func (p *Provisioner) List(ctx context.Context) ([]ProvisionedKey, error) {
	var out struct {
		Data []ProvisionedKey `json:"data"`
	}
	if err := p.do(ctx, "GET", "/keys", nil, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// Revoke deletes a key. Anything signed in with it stops working at once.
func (p *Provisioner) Revoke(ctx context.Context, hash string) error {
	if strings.TrimSpace(hash) == "" {
		return fmt.Errorf("a key hash is required (see lamdis keys list)")
	}
	return p.do(ctx, "DELETE", "/keys/"+hash, nil, nil)
}

// SetLimit raises or lowers a key's cap.
func (p *Provisioner) SetLimit(ctx context.Context, hash string, limit float64) error {
	if strings.TrimSpace(hash) == "" {
		return fmt.Errorf("a key hash is required")
	}
	return p.do(ctx, "PATCH", "/keys/"+hash, map[string]any{"limit": limit}, nil)
}
