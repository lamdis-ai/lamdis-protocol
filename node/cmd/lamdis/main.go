// Command lamdis is the Lamdis Protocol node: an embedded store of
// permissioned, hash-chained context threads that agents read, write, and
// search. M0 surface: local CLI. REST/MCP/sync land in later milestones.
package main

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/agent"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/embed"
	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
	nodemcp "github.com/lamdis-ai/lamdis-protocol/node/internal/mcp"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/perm"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/store"
	syncp "github.com/lamdis-ai/lamdis-protocol/node/internal/sync"
)

const version = "0.1.0-dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "lamdis:", err)
		os.Exit(1)
	}
}

var commandNames = []string{"help", "-h", "--help", "app", "keys", "init", "demo", "exchange", "review", "gauntlet", "wallet", "buy", "verify-photo",
	"whoami", "thread", "threads", "post", "read", "search", "mcp", "serve", "peer", "peers", "sync", "share",
	"discover", "request", "requests", "approve", "deny", "grant", "revoke", "access"}

func knownCommand(s string) bool {
	for _, c := range commandNames {
		if s == c {
			return true
		}
	}
	return false
}

func usage() error {
	fmt.Fprint(os.Stderr, `usage: lamdis [-data DIR] [task...]     the agent, in this directory
       lamdis [-data DIR] <command> [args]

agent:
  lamdis                      interactive: type tasks, the agent works in the
                              current repository and records every run
  lamdis "add a test for X"   one task, then exit
     [-model ID] [-url URL]   pick a model, or an OpenAI-compatible local server
     [-dir PATH] [-q]         workspace (default: git root); -q prints only the answer

commands:
  init                        create a person keypair and empty store
  whoami                      print this node's person principal id
  thread new [-discoverable] <title>   create a thread; prints its id
                              (-discoverable lets paired peers see it exists and ask)
  threads                     list thread ids and titles
  post <thread> <text>        append a core.message to a thread
  post -kind K -lane L ...    append any kind/lane (body from -text or -json)
  read <thread>               print a thread's entries (all lanes)
  search <query>              hybrid search across all local threads

  exchange -base-url URL      run the exchange as a service: reviewer surface,
       [-addr :8080]          panel creation, health. Reads LAMDIS_BASE_URL,
                              LAMDIS_ADDR, LAMDIS_EXCHANGE_KEY from the env.

  review -image FILE          serve a human-review panel: prints one link per
      -question "..."         reviewer, shows the photo on their phone, and
      [-reviewers 3]          settles the panel when enough have answered.
      [-agreement 2]          No model calls, no AWS.

  buy -predicate "..."        run the whole outcome lifecycle against a real
      -image FILE             photograph: quote, escrow, submit, verify with a
      [-tier V1] [-lat/-lon]  real model, settle. Calls Bedrock; a few cents.

  verify-photo -image FILE    run the real verification pipeline over one
       [-predicate "..."]     image: deterministic checks, a blind model
       [-nonce CODE]          description, the challenge-code comparison, then
       [-profile aws-admin]   adjudication. Calls Bedrock; costs a few cents.

  demo [-seed 42] [-json]     buy an outcome end to end against simulated
                              providers: quote, escrow, execute, verify,
                              settle. Runs an honest provider and a
                              fraudulent one; exits non-zero if any
                              invariant fails.

  app                         open your threads in a browser (starts the node
                              if it isn't running already)
  keys mint <who> [-limit 2]  let someone else use your inference without
  keys list | revoke <hash>   being able to drain you: each key carries a hard
                              credit cap that never refills. Needs
                              LAMDIS_OPENROUTER_MANAGEMENT_KEY.

  mcp                         serve MCP over stdio (add to your agent's .mcp.json)
  serve [-addr 127.0.0.1:8420]  run the node: the app, approvals, and sync.
       [-expose-app]          Listens on this machine only unless you set
                              -addr :8420 for peers; even then the app stays
                              local unless -expose-app.
  peer add <name> <url>       pair with a person's node (exchanges identities)
  peers                       list who you're paired with
  sync [peer] [-watch 30s]    sync permitted threads with peers (default: all)

  share <thread> <peer>       host a thread on another node (e.g. an always-on
                              hub both of you sync against; run once per thread)
  discover <peer>             see which of a peer's threads you could request
  request <peer> <thread> <scopes> [reason...]   ask for access
  requests                    pending requests on YOUR threads
  approve <thread> <who> [scopes]                answer a request (default: as asked)
  deny <thread> <who>                            decline a request

  grant <thread> <who> <scopes>   share a thread: thread by title or id,
       [-ttl 168h]                who by peer name (e.g. jane) or principal id.
                                  scopes: summary (the gist, never raw notes),
                                  search, read (everything), contribute (can post)
  revoke <thread> <who>           stop sharing
  access <thread>                 who can see this thread, and how much

environment:
  LAMDIS_DATA        data directory (default ~/.lamdis)
  LAMDIS_EMBED_URL   OpenAI-compatible base URL (e.g. http://localhost:11434/v1)
  LAMDIS_EMBED_MODEL embedding model name
  LAMDIS_EMBED_KEY   API key if the endpoint needs one
`)
	return fmt.Errorf("missing or unknown command")
}

func run(args []string) error {
	fs := flag.NewFlagSet("lamdis", flag.ContinueOnError)
	defaultData := os.Getenv("LAMDIS_DATA")
	if defaultData == "" {
		home, _ := os.UserHomeDir()
		defaultData = filepath.Join(home, ".lamdis")
	}
	dataDir := fs.String("data", defaultData, "data directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	// No subcommand, or a first word that is not one: the agent, here, now.
	if len(rest) == 0 || !knownCommand(rest[0]) {
		loadDotEnv(*dataDir)
		s, err := openStore(*dataDir)
		if err != nil {
			return err
		}
		defer s.Close()
		return cmdCode(context.Background(), *dataDir, s, rest)
	}
	// Secrets live beside the data they belong to, not in the shell.
	//
	// Asking somebody to export an environment variable before every run is
	// how a one-command install becomes a four-command install, and how a key
	// ends up in shell history. A real environment variable still wins, so
	// containers and CI are unaffected.
	loadDotEnv(*dataDir)
	ctx := context.Background()
	cmd, rest := rest[0], rest[1:]

	switch cmd {
	case "help", "-h", "--help":
		return usage()
	case "init":
		return cmdInit(*dataDir)
	case "demo":
		// Self-contained: no store, no keys, no network.
		return cmdDemo(rest)
	case "exchange":
		// Run the exchange as a service.
		return cmdServeExchange(rest)
	case "review":
		// Serve a human-review panel and wait for people to answer it.
		return cmdReview(rest)
	case "gauntlet":
		// A sequence of jobs where later ones try to reuse earlier evidence.
		return cmdGauntlet(rest)
	case "wallet":
		// Balances, keys and payout thresholds, against a real ledger.
		return cmdWallet(rest)
	case "buy":
		// The whole lifecycle against a real photograph.
		return cmdBuy(rest)
	case "verify-photo":
		// Real image, real model call. Needs AWS credentials, not the store.
		return cmdVerifyPhoto(rest)
	case "keys":
		// Operator side: no store, no keys of ours, just OpenRouter.
		return cmdKeys(ctx, rest)
	case "whoami":
		_, pid, err := loadKey(*dataDir)
		if err != nil {
			return err
		}
		fmt.Println(pid)
		return nil
	}

	// Every other command needs the store open.
	s, err := openStore(*dataDir)
	if err != nil {
		return err
	}
	defer s.Close()

	switch cmd {
	case "thread":
		if len(rest) < 2 || rest[0] != "new" {
			return usage()
		}
		disc := false
		args := rest[1:]
		if args[0] == "-discoverable" {
			disc, args = true, args[1:]
		}
		return cmdThreadNew(ctx, *dataDir, s, strings.Join(args, " "), disc)
	case "threads":
		return cmdThreads(ctx, s)
	case "post":
		return cmdPost(ctx, *dataDir, s, rest)
	case "read":
		if len(rest) != 1 {
			return usage()
		}
		thread, err := resolveThread(ctx, s, rest[0])
		if err != nil {
			return err
		}
		return cmdRead(ctx, s, thread)
	case "search":
		if len(rest) == 0 {
			return usage()
		}
		return cmdSearch(ctx, s, strings.Join(rest, " "))
	case "app":
		return cmdApp(ctx, *dataDir, s, rest)
	case "mcp":
		priv, pid, err := loadKey(*dataDir)
		if err != nil {
			return err
		}
		peers, err := loadPeers(*dataDir)
		if err != nil {
			return err
		}
		peerURLs := map[string]string{}
		for name, pr := range peers {
			peerURLs[name] = pr.URL
		}
		return nodemcp.RunStdio(ctx, &nodemcp.Node{
			Store: s, Key: priv, Self: pid, Peers: peerURLs, Embedder: embedderFromEnv(),
		}, version)
	case "serve":
		return cmdServe(*dataDir, s, rest)
	case "peer":
		if len(rest) == 3 && rest[0] == "add" {
			return cmdPeerAdd(ctx, *dataDir, rest[1], rest[2])
		}
		return usage()
	case "peers":
		return cmdPeers(*dataDir)
	case "sync":
		return cmdSync(ctx, *dataDir, s, rest)
	case "share":
		if len(rest) != 2 {
			return usage()
		}
		return cmdShare(ctx, *dataDir, s, rest[0], rest[1])
	case "discover":
		if len(rest) != 1 {
			return usage()
		}
		return cmdDiscover(ctx, *dataDir, rest[0])
	case "request":
		if len(rest) < 3 {
			return usage()
		}
		return cmdRequest(ctx, *dataDir, rest[0], rest[1], rest[2], strings.Join(rest[3:], " "))
	case "requests":
		return cmdRequests(ctx, *dataDir, s)
	case "approve":
		if len(rest) < 2 || len(rest) > 3 {
			return usage()
		}
		return cmdApprove(ctx, *dataDir, s, rest)
	case "deny":
		if len(rest) != 2 {
			return usage()
		}
		thread, err := resolveThread(ctx, s, rest[0])
		if err != nil {
			return err
		}
		who, err := resolveWho(*dataDir, rest[1])
		if err != nil {
			return err
		}
		return cmdGrantWrite(ctx, *dataDir, s, protolog.KindDeny, thread,
			map[string]any{"principal": who})
	case "grant":
		return cmdGrant(ctx, *dataDir, s, rest)
	case "revoke":
		if len(rest) != 2 {
			return usage()
		}
		thread, err := resolveThread(ctx, s, rest[0])
		if err != nil {
			return err
		}
		who, err := resolveWho(*dataDir, rest[1])
		if err != nil {
			return err
		}
		return cmdGrantWrite(ctx, *dataDir, s, protolog.KindRevoke, thread,
			map[string]any{"principal": who})
	case "access":
		if len(rest) != 1 {
			return usage()
		}
		thread, err := resolveThread(ctx, s, rest[0])
		if err != nil {
			return err
		}
		return cmdAccess(ctx, *dataDir, s, thread)
	default:
		return usage()
	}
}

func peersPath(dataDir string) string { return filepath.Join(dataDir, "peers.json") }

// Peer is a person you've paired with: where their node lives and who they
// are. Pairing fetches the principal so humans never copy key strings.
type Peer struct {
	URL       string `json:"url"`
	Principal string `json:"principal,omitempty"`
}

func loadPeers(dataDir string) (map[string]Peer, error) {
	peers := map[string]Peer{}
	raw, err := os.ReadFile(peersPath(dataDir))
	if os.IsNotExist(err) {
		return peers, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &peers); err == nil {
		return peers, nil
	}
	// Legacy format: {name: "url"}.
	legacy := map[string]string{}
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return nil, fmt.Errorf("peers.json is corrupt: %w", err)
	}
	for name, url := range legacy {
		peers[name] = Peer{URL: url}
	}
	return peers, nil
}

func savePeers(dataDir string, peers map[string]Peer) error {
	raw, _ := json.MarshalIndent(peers, "", "  ")
	return os.WriteFile(peersPath(dataDir), raw, 0o600)
}

func cmdPeerAdd(ctx context.Context, dataDir, name, url string) error {
	peers, err := loadPeers(dataDir)
	if err != nil {
		return err
	}
	p := Peer{URL: strings.TrimRight(url, "/")}
	principal, err := api.FetchNodeInfo(ctx, p.URL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lamdis: could not reach %s to exchange identities (%v)\n"+
			"  pairing saved anyway — rerun `lamdis peer add %s %s` when their node is up,\n"+
			"  or grant them by full principal id\n", p.URL, err, name, url)
	} else {
		p.Principal = principal
	}
	peers[name] = p
	if err := savePeers(dataDir, peers); err != nil {
		return err
	}
	if p.Principal != "" {
		fmt.Printf("✓ paired with %s (%s…) — you can now `lamdis grant <thread> %s <scopes>`\n",
			name, p.Principal[:16], name)
	}
	return nil
}

func cmdPeers(dataDir string) error {
	peers, err := loadPeers(dataDir)
	if err != nil {
		return err
	}
	for name, p := range peers {
		id := p.Principal
		if id == "" {
			id = "(identity not exchanged yet)"
		}
		fmt.Printf("%-12s %s  %s\n", name, p.URL, id)
	}
	return nil
}

// resolveWho turns a peer name (or a raw principal id) into a principal id.
func resolveWho(dataDir, who string) (string, error) {
	if strings.HasPrefix(who, "ed25519:") {
		if _, err := protolog.PublicKey(who); err != nil {
			return "", err
		}
		return who, nil
	}
	peers, err := loadPeers(dataDir)
	if err != nil {
		return "", err
	}
	if p, ok := peers[who]; ok {
		if p.Principal == "" {
			return "", fmt.Errorf("peer %q has no identity yet — rerun `lamdis peer add %s %s` while their node is up", who, who, p.URL)
		}
		return p.Principal, nil
	}
	var known []string
	for name := range peers {
		known = append(known, name)
	}
	return "", fmt.Errorf("%q is not a peer name or principal id (peers: %s; add one with `lamdis peer add <name> <url>`)",
		who, strings.Join(known, ", "))
}

// peerName maps a principal back to a friendly name for display.
func peerName(dataDir, principal string) string {
	peers, err := loadPeers(dataDir)
	if err == nil {
		for name, p := range peers {
			if p.Principal == principal {
				return name
			}
		}
	}
	return ""
}

// resolveThread accepts a thread id, a unique id prefix, or a unique
// case-insensitive title fragment.
func resolveThread(ctx context.Context, s store.Store, ref string) (string, error) {
	ids, err := s.Threads(ctx)
	if err != nil {
		return "", err
	}
	var matches []string
	var titles = map[string]string{}
	lowRef := strings.ToLower(ref)
	for _, id := range ids {
		title := ""
		if tl, err := s.Thread(ctx, id); err == nil {
			if g := tl.Get(id); g != nil {
				var body struct {
					Title string `json:"title"`
				}
				json.Unmarshal(g.Body, &body)
				title = body.Title
			}
		}
		titles[id] = title
		if id == ref {
			return id, nil
		}
		if strings.HasPrefix(id, strings.ToUpper(ref)) || strings.Contains(strings.ToLower(title), lowRef) {
			matches = append(matches, id)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("no thread matches %q (see `lamdis threads`)", ref)
	default:
		var opts []string
		for _, id := range matches {
			opts = append(opts, fmt.Sprintf("%s (%s)", titles[id], id))
		}
		return "", fmt.Errorf("%q matches several threads — be more specific: %s", ref, strings.Join(opts, "; "))
	}
}

func cmdServe(dataDir string, s store.Store, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:8420", "listen address; use :8420 to let peers reach you")
	exposeApp := fs.Bool("expose-app", false, "let the interface answer from other machines (off by default)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	priv, pid, err := loadKey(dataDir)
	if err != nil {
		return err
	}
	tokenPath := filepath.Join(dataDir, "portal.token")
	tokenRaw, err := os.ReadFile(tokenPath)
	if err != nil {
		tok, err := api.NewPortalToken()
		if err != nil {
			return err
		}
		if err := os.WriteFile(tokenPath, []byte(tok), 0o600); err != nil {
			return err
		}
		tokenRaw = []byte(tok)
	}
	token := strings.TrimSpace(string(tokenRaw))
	mux := (&api.Server{Sync: &syncp.Server{Store: s}, Principal: pid}).Handler()
	names := func(principal string) string { return peerName(dataDir, principal) }
	portal := &api.Portal{Store: s, Key: priv, Self: pid, Token: token, Names: names, ExposeApp: *exposeApp}
	portal.Register(mux)
	ask, model := api.AskFromEnv()
	app := &api.App{Store: s, Key: priv, Self: pid, Token: token, Names: names,
		Ask: ask, Model: model, DataDir: dataDir, ExposeApp: *exposeApp}
	// The built-in agent: its own key, delegated per thread, and a scheduler
	// that lets it work when nobody is looking.
	agentKey, agentPID, err := agent.LoadOrMintAgentKey(dataDir)
	if err != nil {
		return err
	}
	state := agent.LoadState(dataDir)
	var mdl agent.Model
	if or := agent.NewOpenRouter(os.Getenv("LAMDIS_OPENROUTER_KEY"), model); or != nil {
		mdl = or
	}
	runner := &agent.Runner{Store: s, PersonKey: priv, Person: pid, AgentKey: agentKey, Agent: agentPID,
		Model: mdl, ModelName: model, DataDir: dataDir, Names: names, Embedder: embedderFromEnv(), State: state,
		Logf: func(f string, a ...any) { fmt.Fprintf(os.Stderr, f+"\n", a...) }}
	sched := &agent.Scheduler{Runner: runner, State: state,
		Logf: func(f string, a ...any) { fmt.Fprintf(os.Stderr, f+"\n", a...) }}
	sched.Sync = func(ctx context.Context) error {
		peers, err := loadPeers(dataDir)
		if err != nil || len(peers) == 0 {
			return nil
		}
		var firstErr error
		for name, p := range peers {
			client := &syncp.Client{Store: s, Peer: api.NewHTTPTransport(p.URL, priv), Self: pid,
				SelfKeys: map[string]bool{agentPID: true}}
			if _, err := client.SyncAll(ctx); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", name, err)
			}
		}
		drainEmbeds(ctx, s)
		return firstErr
	}
	app.Runner, app.Scheduler, app.AgentSelf = runner, sched, agentPID
	// Summaries use the same model the agent does, whatever Settings says.
	app.Ask = func(ctx context.Context, question string, entries []string) (string, error) {
		cfg, _ := agent.LoadConfig(dataDir)
		m, _ := runner.ModelFor(cfg)
		if m == nil {
			return "", fmt.Errorf("no model is configured; add an OpenRouter key in Settings")
		}
		return api.AskWith(ctx, m, question, entries)
	}
	if ask == nil && runner.Ready() {
		ask = app.Ask
	}
	go sched.Start(context.Background())
	app.Register(mux)
	host := *addr
	if strings.HasPrefix(host, ":") {
		host = "localhost" + host
	}
	// The address to open comes first: it is the only line most people need,
	// and burying it under the principal made them hunt for it.
	fmt.Printf("open       http://%s/app?token=%s\n", host, token)
	fmt.Printf("approvals  http://%s/portal?token=%s\n", host, token)
	fmt.Printf("principal  %s\n", pid)
	fmt.Printf("agent      %s acting for you; revoke from Settings\n", agentPID)
	if ask == nil {
		fmt.Printf("asking     off - set LAMDIS_OPENROUTER_KEY to answer questions about a thread\n")
	} else {
		fmt.Printf("asking     %s\n", model)
	}
	if strings.HasPrefix(*addr, "127.0.0.1") || strings.HasPrefix(*addr, "localhost") {
		fmt.Printf("network    this machine only. For peers: lamdis serve -addr :8420\n")
	} else {
		fmt.Printf("network    reachable from your network. Peers authenticate with signatures;\n")
		if *exposeApp {
			fmt.Printf("           the interface is EXPOSED and protected only by the token above\n")
		} else {
			fmt.Printf("           the interface stays on this machine (-expose-app to change)\n")
		}
	}
	fmt.Printf("peers      give them this URL; grants decide what they can pull\n")
	// Open the interface. The address carries a token and nobody should have
	// to copy it out of a terminal; a person who just typed one command gets a
	// window, which is what "one command" means. Disable with LAMDIS_NO_OPEN=1.
	if os.Getenv("LAMDIS_NO_OPEN") == "" {
		go openBrowser(fmt.Sprintf("http://%s/app?token=%s", host, token))
	}
	return http.ListenAndServe(*addr, mux)
}

func cmdSync(ctx context.Context, dataDir string, s store.Store, args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	watch := fs.Duration("watch", 0, "keep syncing on this interval (e.g. 30s); 0 = once")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	priv, pid, err := loadKey(dataDir)
	if err != nil {
		return err
	}
	peers, err := loadPeers(dataDir)
	if err != nil {
		return err
	}
	if len(rest) == 1 {
		p, ok := peers[rest[0]]
		if !ok {
			return fmt.Errorf("unknown peer %q (add with `lamdis peer add`)", rest[0])
		}
		peers = map[string]Peer{rest[0]: p}
	}
	if len(peers) == 0 {
		return fmt.Errorf("no peers configured (add with `lamdis peer add <name> <url>`)")
	}
	round := func() {
		for name, p := range peers {
			client := &syncp.Client{Store: s, Peer: api.NewHTTPTransport(p.URL, priv), Self: pid, SelfKeys: selfKeys(dataDir)}
			counts, err := client.SyncAll(ctx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "lamdis: sync %s: %v\n", name, err)
				continue
			}
			total := 0
			for _, n := range counts {
				total += n
			}
			if *watch == 0 || total > 0 {
				fmt.Printf("%s: %d threads visible, %d entries exchanged\n", name, len(counts), total)
			}
		}
		drainEmbeds(ctx, s)
	}
	round()
	if *watch <= 0 {
		return nil
	}
	fmt.Printf("watching every %s (ctrl-c to stop)\n", *watch)
	tick := time.NewTicker(*watch)
	defer tick.Stop()
	for range tick.C {
		round()
	}
	return nil
}

func cmdGrant(ctx context.Context, dataDir string, s store.Store, args []string) error {
	fs := flag.NewFlagSet("grant", flag.ContinueOnError)
	ttl := fs.Duration("ttl", 0, "grant expiry (e.g. 168h); 0 = no expiry")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 3 {
		return fmt.Errorf("usage: grant [-ttl 168h] <thread> <who> <scope,scope> — thread by title or id, who by peer name or principal")
	}
	thread, err := resolveThread(ctx, s, rest[0])
	if err != nil {
		return err
	}
	principal, err := resolveWho(dataDir, rest[1])
	if err != nil {
		return err
	}
	scopesArg := rest[2]
	var scopes []string
	for _, sc := range strings.Split(scopesArg, ",") {
		sc = strings.TrimSpace(sc)
		if !perm.ValidScope(perm.Scope(sc)) {
			return fmt.Errorf("invalid scope %q (valid: contribute, read, summary, search)", sc)
		}
		scopes = append(scopes, sc)
	}
	body := map[string]any{"principal": principal, "scopes": scopes}
	if *ttl > 0 {
		body["ttl_seconds"] = int64(ttl.Seconds())
	}
	return cmdGrantWrite(ctx, dataDir, s, protolog.KindGrant, thread, body)
}

// cmdGrantWrite signs a control-lane entry with the PERSON key. This CLI
// invocation is the human approval step: only a human at this keyboard
// holds the person key.
func cmdGrantWrite(ctx context.Context, dataDir string, s store.Store, kind, thread string, body map[string]any) error {
	priv, _, err := loadKey(dataDir)
	if err != nil {
		return err
	}
	tl, err := s.Thread(ctx, thread)
	if err != nil {
		return err
	}
	a, err := protolog.NewAuthor(tl, priv)
	if err != nil {
		return err
	}
	e, err := a.Append(protolog.Draft{Kind: kind, Lane: protolog.LaneControl, Body: body})
	if err != nil {
		return err
	}
	if err := s.AppendEntries(ctx, []*protolog.Entry{e}); err != nil {
		return err
	}
	fmt.Println(e.ID)
	return nil
}

func cmdAccess(ctx context.Context, dataDir string, s store.Store, thread string) error {
	tl, err := s.Thread(ctx, thread)
	if err != nil {
		return err
	}
	display := func(principal string) string {
		if name := peerName(dataDir, principal); name != "" {
			return fmt.Sprintf("%s (%s…)", name, principal[:16])
		}
		return principal
	}
	st := perm.Fold(thread, tl.Entries())
	for p := range st.Stewards {
		fmt.Printf("steward     %s\n", display(p))
	}
	for _, e := range tl.Entries() {
		if e.Kind != protolog.KindGrant {
			continue
		}
		var body struct {
			Principal string   `json:"principal"`
			Scopes    []string `json:"scopes"`
		}
		if json.Unmarshal(e.Body, &body) != nil {
			continue
		}
		eff := st.EffectiveScopes(body.Principal, time.Now())
		if len(eff) == 0 {
			continue // revoked, denied, or expired
		}
		var have []string
		for sc := range eff {
			have = append(have, string(sc))
		}
		fmt.Printf("granted     %s  %s\n", display(body.Principal), strings.Join(have, ","))
	}
	return nil
}

func keyPath(dataDir string) string { return filepath.Join(dataDir, "person.key") }
func dbPath(dataDir string) string  { return filepath.Join(dataDir, "lamdis.db") }

func cmdInit(dataDir string) error {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	if _, err := os.Stat(keyPath(dataDir)); err == nil {
		return fmt.Errorf("%s already exists; refusing to overwrite a person key", keyPath(dataDir))
	}
	pid, priv, err := protolog.GenerateKeypair()
	if err != nil {
		return err
	}
	if err := os.WriteFile(keyPath(dataDir), []byte(hex.EncodeToString(priv.Seed())+"\n"), 0o600); err != nil {
		return err
	}
	s, err := store.OpenSQLite(dbPath(dataDir))
	if err != nil {
		return err
	}
	s.Close()
	fmt.Printf("initialized %s\nperson principal: %s\n", dataDir, pid)
	return nil
}

// ensureIdentity makes a first run work.
//
// A node that refuses to start until you have run a separate init command is a
// node most people never start. Someone adds this as an MCP server with one
// command, the server exits with "run init first", and the client reports a
// failed connection with no way to act on it. So absence of a key is treated
// as a first run rather than as an error: generate one, keep 0600, and carry
// on. An existing key is never touched, and a corrupt one still fails loudly,
// because silently replacing a key would orphan every entry it ever signed.
func ensureIdentity(dataDir string) error {
	if _, err := os.Stat(keyPath(dataDir)); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	_, priv, err := protolog.GenerateKeypair()
	if err != nil {
		return err
	}
	return os.WriteFile(keyPath(dataDir), []byte(hex.EncodeToString(priv.Seed())+"\n"), 0o600)
}

func loadKey(dataDir string) (ed25519.PrivateKey, string, error) {
	if err := ensureIdentity(dataDir); err != nil {
		return nil, "", err
	}
	raw, err := os.ReadFile(keyPath(dataDir))
	if err != nil {
		return nil, "", fmt.Errorf("cannot read the person key at %s: %w", keyPath(dataDir), err)
	}
	seed, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, "", fmt.Errorf("person key at %s is corrupt", keyPath(dataDir))
	}
	priv := ed25519.NewKeyFromSeed(seed)
	pid, err := protolog.PrincipalID(priv.Public().(ed25519.PublicKey))
	if err != nil {
		return nil, "", err
	}
	return priv, pid, nil
}

// openStore opens the node's database, creating it on a first run.
//
// Same reasoning as ensureIdentity: an empty node is a valid state, and the
// first thing a new user does should not be reading an error.
// selfKeys lists the delegated keys this node also authors as, so their
// chains are offered to peers. Only the built-in agent for now.
func selfKeys(dataDir string) map[string]bool {
	if !agent.HasAgentKey(dataDir) {
		return nil
	}
	if _, pid, err := agent.LoadOrMintAgentKey(dataDir); err == nil {
		return map[string]bool{pid: true}
	}
	return nil
}

func openStore(dataDir string) (store.Store, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	return store.OpenSQLite(dbPath(dataDir))
}

func embedderFromEnv() embed.Embedder {
	url := os.Getenv("LAMDIS_EMBED_URL")
	model := os.Getenv("LAMDIS_EMBED_MODEL")
	if url == "" || model == "" {
		return nil
	}
	return embed.NewOpenAICompat(url, os.Getenv("LAMDIS_EMBED_KEY"), model)
}

// drainEmbeds opportunistically embeds pending entries; without a configured
// embedder it is a no-op and search degrades to FTS.
func drainEmbeds(ctx context.Context, s store.Store) {
	e := embedderFromEnv()
	if e == nil {
		return
	}
	w := embed.Worker{Store: s, Embedder: e}
	for {
		n, err := w.Tick(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "lamdis: embed worker:", err)
			return
		}
		if n == 0 {
			return
		}
	}
}

func cmdThreadNew(ctx context.Context, dataDir string, s store.Store, title string, discoverable bool) error {
	priv, _, err := loadKey(dataDir)
	if err != nil {
		return err
	}
	l, genesis, err := protolog.NewThreadWith(priv, title, discoverable, nil)
	if err != nil {
		return err
	}
	if err := s.AppendEntries(ctx, l.Entries()); err != nil {
		return err
	}
	fmt.Println(genesis.ID)
	return nil
}

func cmdThreads(ctx context.Context, s store.Store) error {
	ids, err := s.Threads(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		title := ""
		if tl, err := s.Thread(ctx, id); err == nil {
			if g := tl.Get(id); g != nil {
				var body struct {
					Title string `json:"title"`
				}
				json.Unmarshal(g.Body, &body)
				title = body.Title
			}
		}
		fmt.Printf("%s  %s\n", id, title)
	}
	return nil
}

func cmdPost(ctx context.Context, dataDir string, s store.Store, args []string) error {
	fs := flag.NewFlagSet("post", flag.ContinueOnError)
	kind := fs.String("kind", "", "entry kind (default core.message; core.summary when -lane summary)")
	lane := fs.String("lane", string(protolog.LaneContent), "lane: content|summary|control")
	jsonBody := fs.String("json", "", "raw JSON body (overrides text)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *kind == "" {
		*kind = protolog.KindMessage
		if *lane == string(protolog.LaneSummary) {
			*kind = protolog.KindSummary
		}
	}
	rest := fs.Args()
	if len(rest) < 1 {
		return fmt.Errorf("post: thread id required")
	}
	threadID, err := resolveThread(ctx, s, rest[0])
	if err != nil {
		return err
	}
	var body any
	if *jsonBody != "" {
		body = json.RawMessage(*jsonBody)
	} else if len(rest) > 1 {
		body = map[string]any{"text": strings.Join(rest[1:], " ")}
	} else {
		return fmt.Errorf("post: text or -json body required")
	}
	priv, _, err := loadKey(dataDir)
	if err != nil {
		return err
	}
	tl, err := s.Thread(ctx, threadID)
	if err != nil {
		return err
	}
	a, err := protolog.NewAuthor(tl, priv)
	if err != nil {
		return err
	}
	e, err := a.Append(protolog.Draft{Kind: *kind, Lane: protolog.Lane(*lane), Body: body})
	if err != nil {
		return err
	}
	if err := s.AppendEntries(ctx, []*protolog.Entry{e}); err != nil {
		return err
	}
	drainEmbeds(ctx, s)
	fmt.Println(e.ID)
	return nil
}

func cmdRead(ctx context.Context, s store.Store, threadID string) error {
	entries, err := s.Entries(ctx, threadID,
		[]protolog.Lane{protolog.LaneControl, protolog.LaneSummary, protolog.LaneContent})
	if err != nil {
		return err
	}
	for _, e := range entries {
		var body struct {
			Text  string `json:"text"`
			Title string `json:"title"`
		}
		json.Unmarshal(e.Body, &body)
		txt := body.Text
		if txt == "" {
			txt = body.Title
		}
		fmt.Printf("%s  %-9s %-22s %s\n", e.TS, e.Lane, e.Kind, txt)
	}
	return nil
}

func cmdSearch(ctx context.Context, s store.Store, query string) error {
	req := store.SearchRequest{
		Query: query,
		Lanes: []protolog.Lane{protolog.LaneControl, protolog.LaneSummary, protolog.LaneContent},
	}
	if e := embedderFromEnv(); e != nil {
		drainEmbeds(ctx, s)
		vecs, err := e.Embed(ctx, []string{query})
		if err != nil {
			fmt.Fprintln(os.Stderr, "lamdis: embedding query failed, using FTS only:", err)
		} else {
			req.QueryVec = vecs[0]
		}
	}
	hits, err := s.Search(ctx, req)
	if err != nil {
		return err
	}
	if len(hits) == 0 {
		fmt.Println("no results")
		return nil
	}
	for _, h := range hits {
		fmt.Printf("%.4f  %s  [%s/%s] %s\n", h.Rank, h.Thread, h.Lane, h.Kind, h.Snippet)
	}
	return nil
}

func peerTransport(dataDir, name string) (*api.HTTPTransport, error) {
	priv, _, err := loadKey(dataDir)
	if err != nil {
		return nil, err
	}
	peers, err := loadPeers(dataDir)
	if err != nil {
		return nil, err
	}
	p, ok := peers[name]
	if !ok {
		return nil, fmt.Errorf("unknown peer %q (add with `lamdis peer add`)", name)
	}
	return api.NewHTTPTransport(p.URL, priv), nil
}

func cmdDiscover(ctx context.Context, dataDir, peer string) error {
	t, err := peerTransport(dataDir, peer)
	if err != nil {
		return err
	}
	threads, err := t.Discover(ctx)
	if err != nil {
		return err
	}
	if len(threads) == 0 {
		fmt.Println("nothing discoverable (they can mark threads with `thread new -discoverable`)")
		return nil
	}
	for _, th := range threads {
		fmt.Printf("%s  %s\n", th.ID, th.Title)
	}
	return nil
}

func cmdRequest(ctx context.Context, dataDir, peer, threadRef, scopesArg, reason string) error {
	t, err := peerTransport(dataDir, peer)
	if err != nil {
		return err
	}
	var scopes []string
	for _, sc := range strings.Split(scopesArg, ",") {
		sc = strings.TrimSpace(sc)
		if !perm.ValidScope(perm.Scope(sc)) {
			return fmt.Errorf("invalid scope %q (valid: contribute, read, summary, search)", sc)
		}
		scopes = append(scopes, sc)
	}
	threads, err := t.Discover(ctx)
	if err != nil {
		return err
	}
	var target string
	low := strings.ToLower(threadRef)
	for _, th := range threads {
		if th.ID == threadRef || strings.Contains(strings.ToLower(th.Title), low) {
			if target != "" {
				return fmt.Errorf("%q matches several of %s's threads — use the id from `lamdis discover %s`", threadRef, peer, peer)
			}
			target = th.ID
		}
	}
	if target == "" {
		return fmt.Errorf("no discoverable thread of %s matches %q (see `lamdis discover %s`)", peer, threadRef, peer)
	}
	priv, _, err := loadKey(dataDir)
	if err != nil {
		return err
	}
	e, err := syncp.BuildAccessRequest(priv, target, scopes, reason, nil)
	if err != nil {
		return err
	}
	if err := t.RequestAccess(ctx, e); err != nil {
		return err
	}
	fmt.Printf("✓ asked %s for %s on %s — they'll see it in `lamdis requests`\n", peer, scopesArg, threadRef)
	return nil
}

func cmdRequests(ctx context.Context, dataDir string, s store.Store) error {
	ids, err := s.Threads(ctx)
	if err != nil {
		return err
	}
	any := false
	for _, id := range ids {
		tl, err := s.Thread(ctx, id)
		if err != nil {
			continue
		}
		st := perm.Fold(id, tl.Entries())
		for _, r := range st.PendingRequests() {
			any = true
			who := r.Principal
			if name := peerName(dataDir, r.Principal); name != "" {
				who = name
			}
			reason := r.Reason
			if reason != "" {
				reason = " — \"" + reason + "\""
			}
			fmt.Printf("%s wants %s on %q%s\n  approve: lamdis approve %q %s\n",
				who, strings.Join(r.Scopes, ","), st.Title, reason, st.Title, who)
		}
	}
	if !any {
		fmt.Println("no pending requests")
	}
	return nil
}

func cmdApprove(ctx context.Context, dataDir string, s store.Store, rest []string) error {
	thread, err := resolveThread(ctx, s, rest[0])
	if err != nil {
		return err
	}
	who, err := resolveWho(dataDir, rest[1])
	if err != nil {
		return err
	}
	tl, err := s.Thread(ctx, thread)
	if err != nil {
		return err
	}
	st := perm.Fold(thread, tl.Entries())
	var scopes []string
	var requestID string
	for _, r := range st.PendingRequests() {
		if r.Principal == who {
			scopes, requestID = r.Scopes, r.EntryID
		}
	}
	if len(rest) == 3 { // explicit scopes override the asked-for ones
		scopes = nil
		for _, sc := range strings.Split(rest[2], ",") {
			sc = strings.TrimSpace(sc)
			if !perm.ValidScope(perm.Scope(sc)) {
				return fmt.Errorf("invalid scope %q", sc)
			}
			scopes = append(scopes, sc)
		}
	}
	if len(scopes) == 0 {
		return fmt.Errorf("no pending request from them on this thread — use `lamdis grant` to share unprompted")
	}
	body := map[string]any{"principal": who, "scopes": scopes}
	if requestID != "" {
		body["request"] = requestID
	}
	if err := cmdGrantWrite(ctx, dataDir, s, protolog.KindGrant, thread, body); err != nil {
		return err
	}
	fmt.Printf("✓ approved — they receive it on their next sync\n")
	return nil
}

func cmdShare(ctx context.Context, dataDir string, s store.Store, threadRef, peer string) error {
	thread, err := resolveThread(ctx, s, threadRef)
	if err != nil {
		return err
	}
	t, err := peerTransport(dataDir, peer)
	if err != nil {
		return err
	}
	_, pid, err := loadKey(dataDir)
	if err != nil {
		return err
	}
	client := &syncp.Client{Store: s, Peer: t, Self: pid}
	n, err := client.ShareThread(ctx, thread)
	if err != nil {
		return err
	}
	fmt.Printf("✓ %s now hosts the thread (%d entries seeded) — keep `lamdis sync -watch` running and grants decide who can pull it there\n", peer, n)
	return nil
}

// loadDotEnv reads KEY=VALUE lines from <dataDir>/.env into the process
// environment, without overriding anything already set.
//
// Deliberately small: no interpolation, no export keyword, no multi-line
// values. A configuration format that can surprise you is the wrong thing to
// put a credential in.
func loadDotEnv(dataDir string) {
	f, err := os.Open(filepath.Join(dataDir, ".env"))
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		// Quotes are stripped because every example on the internet has them.
		if len(val) >= 2 && (val[0] == '"' && val[len(val)-1] == '"' ||
			val[0] == '\'' && val[len(val)-1] == '\'') {
			val = val[1 : len(val)-1]
		}
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		os.Setenv(key, val)
	}
}

// openBrowser asks the desktop to show a URL, and does nothing loudly if it
// cannot: a headless box is not an error.
func openBrowser(url string) {
	time.Sleep(400 * time.Millisecond)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return
	}
	_ = cmd.Start()
}
