package main

// `lamdis link CODE` connects this machine to an account.
//
// The code comes from the account, is good for fifteen minutes and one
// machine, and buys exactly one thing: access to one thread. After that the
// two nodes sync like any other pair, and the account can revoke it in the
// same place it revokes anything else.

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/store"
	syncp "github.com/lamdis-ai/lamdis-protocol/node/internal/sync"
)

func cmdLink(ctx context.Context, dataDir string, s store.Store, args []string) error {
	fs := flag.NewFlagSet("link", flag.ContinueOnError)
	host := fs.String("host", envOr("LAMDIS_HOST_URL", "https://app.lamdis.ai"), "the account's address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	code := strings.ToLower(strings.TrimSpace(strings.Join(fs.Args(), "")))
	if code == "" {
		fmt.Fprintf(os.Stderr, `Connect this machine to your account.

  1. Open %s/app on any device
  2. Settings, then "Connect a machine", and pick a thread
  3. Run the command it gives you, here

The code lasts fifteen minutes and works once. It gives this machine access
to that one thread and nothing else, and you can take it back from the same
place you gave it.
`, *host)
		return fmt.Errorf("no code given")
	}

	priv, pid, err := loadKey(dataDir)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{"code": code, "name": hostname()})
	url := strings.TrimRight(*host, "/") + "/v1/link"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	// Signing proves this machine holds the key it is asking us to trust.
	if err := api.Sign(req, priv, body); err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("could not reach %s: %w", *host, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out struct {
		OK        bool   `json:"ok"`
		Thread    string `json:"thread"`
		Principal string `json:"principal"`
		SyncURL   string `json:"sync_url"`
		Name      string `json:"name"`
		Error     string `json:"error"`
	}
	json.Unmarshal(raw, &out)
	if resp.StatusCode >= 300 || !out.OK {
		if out.Error != "" {
			return fmt.Errorf("%s", out.Error)
		}
		return fmt.Errorf("the account refused this machine (HTTP %d)", resp.StatusCode)
	}

	name := "account"
	if out.Name != "" && out.Name != "lamdis" {
		name = strings.Split(out.Name, "@")[0]
	}
	if err := savePeerRecord(dataDir, name, out.SyncURL, out.Principal); err != nil {
		return err
	}
	c := &syncp.Client{Store: s, Peer: api.NewHTTPTransport(out.SyncURL, priv), Self: pid}
	counts, err := c.SyncAll(ctx)
	if err != nil {
		return fmt.Errorf("connected, but the first sync failed: %w", err)
	}
	n := 0
	for _, v := range counts {
		n += v
	}
	fmt.Printf("✓ connected to %s\n", *host)
	fmt.Printf("  %d entries pulled into this machine\n", n)
	fmt.Printf("\nNow leave an agent running here:\n")
	fmt.Printf("  lamdis agent -thread %s\n", out.Thread[:12])
	fmt.Printf("\nAnything you write in that thread, from anywhere, it will act on here.\n")
	return nil
}

func hostname() string {
	h, _ := os.Hostname()
	if h == "" {
		return "a machine"
	}
	return h
}

// savePeerRecord remembers an account the way pairing with a person does,
// so everything downstream treats it as an ordinary peer.
func savePeerRecord(dataDir, name, url, principal string) error {
	peers, err := loadPeers(dataDir)
	if err != nil {
		return err
	}
	// Two accounts with the same first name should not collide silently.
	base := name
	for i := 2; ; i++ {
		if p, taken := peers[name]; !taken || p.URL == url {
			break
		}
		name = fmt.Sprintf("%s-%d", base, i)
	}
	peers[name] = Peer{URL: url, Principal: principal}
	return savePeers(dataDir, peers)
}
