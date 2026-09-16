package main

// `lamdis agent` leaves an agent running on this machine.
//
// It is the same agent that answers you at the terminal, with one
// difference: nobody has to be at the terminal. It watches a thread, and a
// thread is the same thing whether you write into it from this laptop, from
// app.lamdis.ai on your phone, or from a colleague's node that you granted
// access. So "have a look at the logs on my machine" becomes something you
// can say from a train.
//
// What keeps that from being alarming is that it is the same agent with the
// same rules: it works where you launched it, asks before reaching anywhere
// else, and writes down every run. The permission for the machine is the
// directory you started it in; the permission for the thread is the one you
// signed.

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/agent"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/perm"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/store"
	syncp "github.com/lamdis-ai/lamdis-protocol/node/internal/sync"
)

func cmdListen(ctx context.Context, dataDir string, s store.Store, args []string) error {
	fs := flag.NewFlagSet("agent", flag.ContinueOnError)
	threadRef := fs.String("thread", "", "thread to work in (default: this machine's own)")
	dir := fs.String("dir", "", "where it may work (default: here)")
	allow := fs.String("allow", "", "more directories it may work in, comma separated")
	trust := fs.String("trust", "", "project, home or all: how much of this machine it may use (remembered)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	root := *dir
	if root == "" {
		root, _ = os.Getwd()
	}
	root, _ = filepath.Abs(root)

	priv, pid, err := loadKey(dataDir)
	if err != nil {
		return err
	}
	agentKey, agentPID, err := agent.LoadOrMintAgentKey(dataDir)
	if err != nil {
		return err
	}
	cfg, _ := agent.LoadConfig(dataDir)
	names := func(p string) string { return peerName(dataDir, p) }

	thread, err := machineThread(ctx, s, priv, pid, *threadRef)
	if err != nil {
		return err
	}

	// What it may reach is a setting, not a question asked over and over.
	if *trust != "" {
		cfg.Trust = *trust
		agent.SaveConfig(dataDir, cfg)
	}
	extra, mayAsk := agent.Reach(cfg, root)
	ws := &agent.Workspace{Root: root, Guard: !cfg.Unguarded}
	for _, d := range extra {
		ws.Allow(d)
	}
	for _, d := range strings.Split(*allow, ",") {
		if d = strings.TrimSpace(d); d != "" {
			ws.Allow(d)
		}
	}
	ws.Remember = func(p string) {
		c, _ := agent.LoadConfig(dataDir)
		for _, q := range c.AllowPaths {
			if q == p {
				return
			}
		}
		c.AllowPaths = append(c.AllowPaths, p)
		agent.SaveConfig(dataDir, c)
	}

	var base agent.Model
	if or := agent.NewOpenRouter(os.Getenv("LAMDIS_OPENROUTER_KEY"), os.Getenv("LAMDIS_MODEL")); or != nil {
		base = or
	}
	state := agent.LoadState(dataDir)
	runner := &agent.Runner{Store: s, PersonKey: priv, Person: pid, AgentKey: agentKey, Agent: agentPID,
		Model: base, ModelName: os.Getenv("LAMDIS_MODEL"), DataDir: dataDir, Names: names,
		Embedder: embedderFromEnv(), State: state, Workspace: ws,
		Logf: func(f string, a ...any) { fmt.Fprintf(os.Stderr, "\033[2m"+f+"\033[0m\n", a...) },
		OnTool: func(name string, args map[string]any, out string, took time.Duration) {
			fmt.Fprintf(os.Stderr, "  \033[2m→ %s %s (%s)\033[0m\n", name,
				strings.ReplaceAll(toolArgSummary(name, args), root+"/", ""), took.Round(100*time.Millisecond))
		}}
	if !runner.Ready() {
		return fmt.Errorf("no model is configured. Put LAMDIS_OPENROUTER_KEY=sk-or-... in %s/.env, or run\n  lamdis -url http://localhost:11434/v1 -model qwen3.5:4b", dataDir)
	}

	syncNow := func(ctx context.Context) error {
		peers, err := loadPeers(dataDir)
		if err != nil || len(peers) == 0 {
			return nil
		}
		var first error
		for name, p := range peers {
			c := &syncp.Client{Store: s, Peer: api.NewHTTPTransport(p.URL, priv), Self: pid,
				SelfKeys: map[string]bool{agentPID: true}}
			if _, err := c.SyncAll(ctx); err != nil && first == nil {
				first = fmt.Errorf("%s: %w", name, err)
			}
		}
		return first
	}

	// Asking for a directory is a decision like any other, so it arrives
	// wherever the person is: the terminal, the browser, their phone.
	//
	// It carries its own syncing, because the run that is asking holds the
	// agent still: whatever would otherwise deliver the question is itself
	// waiting on the answer to it.
	if mayAsk {
		ws.Ask = func(ctx context.Context, path, why string) (bool, error) {
			return askForPath(ctx, s, priv, agentKey, pid, thread, path, why, syncNow)
		}
	}

	// Somebody is on the other end of this, so check often enough that it
	// feels like talking rather than posting.
	sched := &agent.Scheduler{Runner: runner, State: state, SyncEvery: 10 * time.Second,
		Logf: func(f string, a ...any) { fmt.Fprintf(os.Stderr, "\033[2m"+f+"\033[0m\n", a...) }}
	sched.Sync = syncNow

	_, mname := runner.ModelFor(cfg)
	title, _ := threadTitle(ctx, s, thread)
	fmt.Fprint(os.Stderr, banner(filepath.Base(root), mname))
	fmt.Fprintf(os.Stderr, "\033[2mlistening on \033[0m%s\033[2m (%s)\033[0m\n", title, thread[:12])
	peers, _ := loadPeers(dataDir)
	if len(peers) > 0 {
		names := []string{}
		for n := range peers {
			names = append(names, n)
		}
		fmt.Fprintf(os.Stderr, "\033[2msyncing with %s, so anything written there arrives here\033[0m\n", strings.Join(names, ", "))
	} else {
		fmt.Fprintf(os.Stderr, "\033[2mnot paired with anything yet; `lamdis link` connects this machine to app.lamdis.ai\033[0m\n")
	}
	reach := root
	if cfg.Trust != agent.TrustProject {
		reach = agent.TrustSays(cfg.Trust)
	}
	guard := ""
	if !cfg.Unguarded {
		guard = " · keys and credentials refused"
	}
	fmt.Fprintf(os.Stderr, "\033[2mworking in %s%s%s · Ctrl-C to stop\033[0m\n\n", reach,
		map[bool]string{true: " · asks before going further", false: ""}[mayAsk], guard)

	// Standing instructions, so the scheduler acts on what arrives. The
	// person can change them later from anywhere; this only sets them up
	// the first time.
	if err := ensureListenBrief(ctx, s, priv, pid, thread); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	go sched.Start(ctx)
	<-ctx.Done()
	fmt.Fprintf(os.Stderr, "\n\033[2mstopped. What it did is in %s\033[0m\n", title)
	return nil
}

// machineThread is where this machine takes its direction: the one you
// named, or one of its own named after the host.
func machineThread(ctx context.Context, s store.Store, priv ed25519.PrivateKey, pid, ref string) (string, error) {
	if ref != "" {
		return resolveThread(ctx, s, ref)
	}
	host, _ := os.Hostname()
	if host == "" {
		host = "this machine"
	}
	title := "machine: " + host
	ids, err := s.Threads(ctx)
	if err != nil {
		return "", err
	}
	for _, id := range ids {
		tl, err := s.Thread(ctx, id)
		if err != nil {
			continue
		}
		if st := perm.Fold(id, tl.Entries()); st.Title == title {
			return id, nil
		}
	}
	tl, genesis, err := protolog.NewThreadWith(priv, title, false, nil)
	if err != nil {
		return "", err
	}
	_ = tl
	if err := s.AppendEntries(ctx, []*protolog.Entry{genesis}); err != nil {
		return "", err
	}
	return genesis.ID, nil
}

func threadTitle(ctx context.Context, s store.Store, id string) (string, error) {
	tl, err := s.Thread(ctx, id)
	if err != nil {
		return id, err
	}
	t := perm.Fold(id, tl.Entries()).Title
	if t == "" {
		t = id
	}
	return t, nil
}

// askForPath puts the request into the thread and waits for an answer.
//
// This is the same decision card the agent uses for anything else it will
// not decide alone, which means the answer can come from the terminal, the
// browser, or a phone, and the question and the answer both stay in the
// record. An unanswered request expires rather than blocking forever.
func askForPath(ctx context.Context, s store.Store, priv, agentKey ed25519.PrivateKey, person, thread, path, why string, syncNow func(context.Context) error) (bool, error) {
	q := "May I work in " + path + "?"
	if strings.TrimSpace(why) != "" {
		q += " " + strings.TrimSpace(why)
	}
	tl, err := s.Thread(ctx, thread)
	if err != nil {
		return false, err
	}
	author, err := protolog.NewAuthor(tl, agentKey)
	if err != nil {
		return false, err
	}
	e, err := author.Append(protolog.Draft{Kind: agent.KindDecision, Lane: protolog.LaneContent,
		OnBehalfOf: person,
		Body: map[string]any{"text": q, "options": []string{"allow", "no"},
			"path": path, "trigger": "path"}})
	if err != nil {
		return false, err
	}
	if err := s.AppendEntries(ctx, []*protolog.Entry{e}); err != nil {
		return false, err
	}
	fmt.Fprintf(os.Stderr, "\n\033[36m? %s\033[0m\n\033[2m  answer here, or from app.lamdis.ai\033[0m\n", q)
	// Send the question out before waiting on it.
	if syncNow != nil {
		if err := syncNow(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "\033[2m  (could not reach the other side: %v)\033[0m\n", err)
		}
	}

	// Wait for a reply to that exact question, wherever it comes from.
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(3 * time.Second):
		}
		// An answer may have been written anywhere, so go and look.
		if syncNow != nil {
			syncNow(ctx)
		}
		tl, err := s.Thread(ctx, thread)
		if err != nil {
			continue
		}
		for _, r := range tl.Entries() {
			if r.Kind != agent.KindDecisionReply || r.Refs == nil || r.Refs.RepliesTo != e.ID {
				continue
			}
			var b struct {
				Choice string `json:"choice"`
				Text   string `json:"text"`
			}
			json.Unmarshal(r.Body, &b)
			answer := strings.ToLower(strings.TrimSpace(b.Choice + " " + b.Text))
			ok := strings.Contains(answer, "allow") || strings.HasPrefix(answer, "yes") || strings.Contains(answer, "ok")
			if ok {
				fmt.Fprintf(os.Stderr, "\033[2m  allowed\033[0m\n")
			} else {
				fmt.Fprintf(os.Stderr, "\033[2m  refused\033[0m\n")
			}
			return ok, nil
		}
	}
	return false, fmt.Errorf("nobody answered within ten minutes")
}

// ensureListenBrief gives the thread standing instructions the first time,
// so that something written into it is treated as something to do.
func ensureListenBrief(ctx context.Context, s store.Store, priv ed25519.PrivateKey, person, thread string) error {
	tl, err := s.Thread(ctx, thread)
	if err != nil {
		return err
	}
	if _, has := agent.LoadBrief(tl, person); has {
		return nil
	}
	author, err := protolog.NewAuthor(tl, priv)
	if err != nil {
		return err
	}
	e, err := author.Append(protolog.Draft{Kind: agent.KindBrief, Lane: protolog.LaneContent,
		Body: map[string]any{
			"text": "You are running on this machine and taking direction from this thread. " +
				"When somebody writes something here, treat it as a request: do it where you are allowed to work, " +
				"and say what you did and what you found. Ask with open_path before reaching a directory you do not have. " +
				"If what they wrote is not a request, answer it. If a choice is theirs to make, ask and wait.",
			"on_new_entry": "all",
			"web":          false,
		}})
	if err != nil {
		return err
	}
	return s.AppendEntries(ctx, []*protolog.Entry{e})
}
