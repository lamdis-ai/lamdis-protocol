package api

// The hosted front door.
//
// An agent that only runs while your laptop is open is not an agent that
// keeps working while you are away, so "hosted" is not a different product
// here: it is the same node, one data directory per person, kept running.
// Each account holds a person keypair, a SQLite file, and an agent config in
// exactly the layout `lamdis -data DIR serve` expects, so anybody can take
// their directory and walk. That is what makes hosting honest rather than a
// lock-in: we are running your node, not holding your record hostage.
//
// Identity comes from a Cognito user pool, verified locally against the
// pool's public keys. Whoever the token says you are picks the directory; the
// application underneath never learns there was a front door.

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/agent"
	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/store"
	syncp "github.com/lamdis-ai/lamdis-protocol/node/internal/sync"
)

// Host serves many people's nodes from one process.
type Host struct {
	// Root holds one directory per account.
	Root    string
	Cognito *Cognito
	// Model is the default model id for new accounts.
	Model string
	// SharedKey is a model credential every account falls back to when it
	// has none of its own. Cap it: everyone on this host spends it.
	SharedKey string
	// AllowedModels is the menu offered to somebody spending SharedKey.
	AllowedModels []string
	// Per-account daily limits, applied when an account is created. They
	// bound what one visitor can spend of a credential that is not theirs.
	AccountRunsPerDay    int
	AccountTokensPerDay  int
	AccountFetchesPerDay int
	// SignIn tells the page where to send people to prove who they are.
	SignIn SignIn
	// Starter mints a capped key for each new account while the ceiling
	// allows it. Nil means new accounts arrive without one and are told how
	// to ask. The cap never refills, so this cannot run away.
	Starter     *agent.Provisioner
	StarterCap  float64
	KeyCeiling  float64
	MaxAccounts int
	// Guests lets somebody start without saying who they are. The node is
	// real from the first keystroke; signing in later attaches an identity
	// to it rather than moving anything.
	Guests bool
	// Try, when set, serves the public demo alongside the accounts.
	Try *Try

	Now  func() time.Time
	Logf func(format string, args ...any)

	mu       sync.Mutex
	accounts map[string]*Account
	secret   []byte
	ctx      context.Context
}

// SignIn is the hosted user pool's browser endpoints.
type SignIn struct {
	Domain   string // e.g. auth.lamdis.ai or lamdis.auth.us-east-1.amazoncognito.com
	ClientID string
	Redirect string // where Cognito sends people back, e.g. https://app.lamdis.ai/app
}

func (h *SignIn) configured() bool { return h.Domain != "" && h.ClientID != "" }

// Account is one person's node.
type Account struct {
	ID    string // opaque, derived from the identity provider's subject
	Email string
	Dir   string

	Store store.Store
	Key   ed25519.PrivateKey
	Self  string
	App   *App
	Sched *agent.Scheduler

	mux  *http.ServeMux
	sync *Server
}

func (h *Host) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

func (h *Host) logf(f string, a ...any) {
	if h.Logf != nil {
		h.Logf(f, a...)
	}
}

// accountID is stable for a given identity and safe as a directory name. The
// provider's subject never appears on disk or in a URL.
func accountID(subject string) string {
	sum := sha256.Sum256([]byte("lamdis-account\x00" + subject))
	return hex.EncodeToString(sum[:8])
}

// Start loads every account that already exists and starts its agent, so
// standing instructions run for people who are not looking at the screen.
func (h *Host) Start(ctx context.Context) error {
	h.ctx = ctx
	h.mu.Lock()
	if h.accounts == nil {
		h.accounts = map[string]*Account{}
	}
	h.mu.Unlock()
	if err := os.MkdirAll(h.Root, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(h.Root)
	if err != nil {
		return err
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := h.load(e.Name(), ""); err != nil {
			h.logf("host: account %s did not load: %v", e.Name(), err)
			continue
		}
		n++
	}
	h.logf("host: %d accounts running", n)
	return nil
}

// sharedModel is the fallback credential, or nil when each account brings
// its own.
func (h *Host) sharedModel() agent.Model {
	if h.SharedKey == "" {
		return nil
	}
	if m := agent.NewOpenRouter(h.SharedKey, h.Model); m != nil {
		return m
	}
	return nil
}

// Close releases every account's database. Used by tests and by a shutdown
// that wants to leave the files consistent.
func (h *Host) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, a := range h.accounts {
		if a.Store != nil {
			a.Store.Close()
		}
	}
	h.accounts = map[string]*Account{}
}

// Count is how many accounts exist.
func (h *Host) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.accounts)
}

// load opens an account, creating its directory the first time. email is
// recorded when known so an operator can tell who is who.
func (h *Host) load(id, email string) (*Account, error) {
	h.mu.Lock()
	if a := h.accounts[id]; a != nil {
		h.mu.Unlock()
		return a, nil
	}
	h.mu.Unlock()

	dir := filepath.Join(h.Root, id)
	fresh := false
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		fresh = true
		if h.MaxAccounts > 0 && h.Count() >= h.MaxAccounts {
			return nil, fmt.Errorf("this host is full; write to support@lamdis.ai")
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
	}
	key, self, err := loadOrMintPersonKey(dir)
	if err != nil {
		return nil, err
	}
	if email != "" {
		os.WriteFile(filepath.Join(dir, "email"), []byte(email), 0o600)
	} else if raw, err := os.ReadFile(filepath.Join(dir, "email")); err == nil {
		email = strings.TrimSpace(string(raw))
	}
	st, err := store.OpenSQLite(filepath.Join(dir, "lamdis.db"))
	if err != nil {
		return nil, err
	}
	acct := &Account{ID: id, Email: email, Dir: dir, Store: st, Key: key, Self: self}

	// The agent, exactly as it runs on a laptop.
	agentKey, agentPID, err := agent.LoadOrMintAgentKey(dir)
	if err != nil {
		return nil, err
	}
	state := agent.LoadState(dir)
	runner := &agent.Runner{Store: st, PersonKey: key, Person: self, AgentKey: agentKey, Agent: agentPID,
		Model: h.sharedModel(), ModelName: h.Model, DataDir: dir, State: state, NoCommands: true,
		AllowedModels: h.AllowedModels,
		Names:         func(p string) string { return "" },
		Logf:          func(f string, a ...any) { h.logf("host: "+id+": "+f, a...) }}
	sched := &agent.Scheduler{Runner: runner, State: state,
		Logf: func(f string, a ...any) { h.logf("host: "+id+": "+f, a...) }}
	sched.Sync = func(ctx context.Context) error { return hostSync(ctx, dir, st, key, self, agentPID) }

	app := &App{Store: st, Key: key, Self: self, DataDir: dir, Model: h.Model,
		Runner: runner, Scheduler: sched, AgentSelf: agentPID,
		SharePrefix: "/s/" + id,
		NoCommands:  true,
		// Identity was established at the front door.
		Auth: func(r *http.Request) bool { return true },
	}
	app.Ask = func(ctx context.Context, q string, entries []string) (string, error) {
		cfg, _ := agent.LoadConfig(dir)
		m, _ := runner.ModelFor(cfg)
		if m == nil {
			return "", fmt.Errorf("no model is configured for this account yet")
		}
		return AskWith(ctx, m, q, entries)
	}
	acct.App = app
	acct.Sched = sched
	acct.sync = &Server{Sync: &syncp.Server{Store: st}, Principal: self}

	mux := http.NewServeMux()
	app.Register(mux)
	acct.mux = mux

	h.mu.Lock()
	if existing := h.accounts[id]; existing != nil {
		h.mu.Unlock()
		st.Close()
		return existing, nil
	}
	h.accounts[id] = acct
	h.mu.Unlock()

	if fresh {
		h.budget(dir)
		h.welcome(acct)
	}
	if h.ctx != nil {
		go sched.Start(h.ctx)
	}
	return acct, nil
}

// budget writes the limits a new account lives within. They are in the
// account's own config so the agent enforces them exactly as it does on
// somebody's laptop, and the interface never offers to raise them.
func (h *Host) budget(dir string) {
	cfg, _ := agent.LoadConfig(dir)
	if h.AccountRunsPerDay > 0 {
		cfg.MaxRunsPerDay = h.AccountRunsPerDay
	}
	if h.AccountTokensPerDay > 0 {
		cfg.MaxTokensPerDay = h.AccountTokensPerDay
	}
	if h.AccountFetchesPerDay > 0 {
		cfg.MaxFetchesPerDay = h.AccountFetchesPerDay
	}
	if cfg.Model == "" {
		cfg.Model = h.Model
	}
	agent.SaveConfig(dir, cfg)
}

// welcome opens an empty thread and, while the ceiling allows, gives the
// account a capped key so the agent answers the very first question. No
// explanatory note: somebody who has just arrived wants to type their own
// thing, and the empty thread already says so.
func (h *Host) welcome(a *Account) {
	if _, genesis, err := protolog.NewThreadWith(a.Key, "Notes", false, nil); err == nil {
		a.Store.AppendEntries(context.Background(), []*protolog.Entry{genesis})
	}
	h.grantStarterKey(a)
}

// grantStarterKey mints a small capped key for an account that has none.
// It stops at the ceiling and never retries in a loop: past the ceiling the
// interface tells people to write in, which is the manual path.
func (h *Host) grantStarterKey(a *Account) {
	if h.Starter == nil || h.StarterCap <= 0 {
		return
	}
	cfg, _ := agent.LoadConfig(a.Dir)
	if cfg.OpenRouterKey != "" {
		return
	}
	label := a.Email
	if label == "" {
		label = "account " + a.ID
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	k, err := h.Starter.MintWithin(ctx, label, h.StarterCap, h.KeyCeiling)
	if err != nil {
		h.logf("host: no starter key for %s: %v", a.ID, err)
		return
	}
	cfg.OpenRouterKey = k.Secret
	if cfg.Model == "" {
		cfg.Model = h.Model
	}
	if err := agent.SaveConfig(a.Dir, cfg); err != nil {
		h.logf("host: could not save the starter key for %s: %v", a.ID, err)
		return
	}
	h.logf("host: starter key for %s, cap $%.2f", a.ID, k.Limit)
}

// loadOrMintPersonKey uses the same file a local node does, so an account
// directory is a node directory.
func loadOrMintPersonKey(dir string) (ed25519.PrivateKey, string, error) {
	path := filepath.Join(dir, "person.key")
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		_, priv, err := protolog.GenerateKeypair()
		if err != nil {
			return nil, "", err
		}
		if err := os.WriteFile(path, []byte(hex.EncodeToString(priv.Seed())+"\n"), 0o600); err != nil {
			return nil, "", err
		}
		raw = []byte(hex.EncodeToString(priv.Seed()))
	} else if err != nil {
		return nil, "", err
	}
	seed, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, "", fmt.Errorf("person key in %s is corrupt", dir)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	pid, err := protolog.PrincipalID(priv.Public().(ed25519.PublicKey))
	if err != nil {
		return nil, "", err
	}
	return priv, pid, nil
}

// hostSync syncs a hosted account with whatever peers it has, which is how
// somebody's laptop and their hosted node stay one record.
func hostSync(ctx context.Context, dir string, s store.Store, key ed25519.PrivateKey, self, agentPID string) error {
	peers, err := loadPeersIn(dir)
	if err != nil || len(peers) == 0 {
		return nil
	}
	var first error
	for name, p := range peers {
		c := &syncp.Client{Store: s, Peer: NewHTTPTransport(p.URL, key), Self: self,
			SelfKeys: map[string]bool{agentPID: true}}
		if _, err := c.SyncAll(ctx); err != nil && first == nil {
			first = fmt.Errorf("%s: %w", name, err)
		}
	}
	return first
}

// resolve turns a request's bearer token into that person's node. A visitor
// token names a node directly; an email token is looked up, following an
// earlier attachment when there is one.
func (h *Host) resolve(r *http.Request) (*Account, error) {
	tok := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if tok == "" {
		return nil, fmt.Errorf("start first")
	}
	if id, ok := h.readGuest(tok); ok {
		return h.load(id, "")
	}
	claims, err := h.Cognito.Verify(tok)
	if err != nil {
		return nil, err
	}
	if !claims.EmailVerified {
		return nil, fmt.Errorf("confirm your email address first")
	}
	// Somebody who started as a visitor and is now signing in keeps the node
	// they have been using.
	if g := strings.TrimSpace(r.Header.Get("X-Lamdis-Guest")); g != "" {
		if id, ok := h.readGuest(g); ok {
			if _, err := os.Stat(filepath.Join(h.Root, id, "subject")); os.IsNotExist(err) {
				return h.attach(claims.Subject, claims.Email, id)
			}
		}
	}
	return h.accountFor(claims.Subject, claims.Email)
}

// Handler is the whole hosted surface.
func (h *Host) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "accounts": h.Count()})
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/app", http.StatusFound)
	})

	// The page itself is public: the script signs in, then calls the API.
	page := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(hostedAppHTML(h.SignIn)))
	}
	mux.HandleFunc("GET /app", page)
	mux.HandleFunc("GET /app/{$}", page)
	// Starting is public: that is the whole point of it.
	mux.HandleFunc("POST /app/api/start", h.handleStart)

	mux.HandleFunc("GET /app/app.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write([]byte(appJS))
	})

	// Everything under /app/api belongs to one person.
	mux.HandleFunc("/app/api/", func(w http.ResponseWriter, r *http.Request) {
		acct, err := h.resolve(r)
		if err != nil {
			writeStatusJSON(w, http.StatusUnauthorized, map[string]any{"error": err.Error()})
			return
		}
		acct.mux.ServeHTTP(w, r)
	})

	// Share links carry the account in the path, because one address now
	// serves many nodes and a capability names a thread, not a person.
	mux.HandleFunc("GET /s/{acct}/{cap}", h.shared)
	mux.HandleFunc("GET /s/{acct}/{cap}/api/thread", h.shared)

	// The public demo, if this host offers one, so the site and the app can
	// live at one address.
	if h.Try != nil {
		demo := h.Try.Handler()
		mux.Handle("/v1/try", demo)
	}

	// Peer sync, per account, so a laptop can pair with its hosted node.
	mux.HandleFunc("/n/{acct}/", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("acct")
		h.mu.Lock()
		acct := h.accounts[id]
		h.mu.Unlock()
		if acct == nil {
			http.NotFound(w, r)
			return
		}
		http.StripPrefix("/n/"+id, acct.sync.Handler()).ServeHTTP(w, r)
	})
	return mux
}

func (h *Host) shared(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("acct")
	h.mu.Lock()
	acct := h.accounts[id]
	h.mu.Unlock()
	if acct == nil {
		http.NotFound(w, r)
		return
	}
	acct.mux.ServeHTTP(w, r)
}

func writeStatusJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	raw, _ := json.Marshal(v)
	w.Write(raw)
}

// loadPeersIn reads a data directory's peers.json, the same file the CLI
// writes, so a hosted account pairs exactly like a local one.
func loadPeersIn(dir string) (map[string]peerRecord, error) {
	out := map[string]peerRecord{}
	raw, err := os.ReadFile(filepath.Join(dir, "peers.json"))
	if err != nil {
		return out, nil
	}
	return out, json.Unmarshal(raw, &out)
}
