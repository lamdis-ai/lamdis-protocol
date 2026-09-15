package main

// `lamdis` with no subcommand is the agent, in the directory you are in.
//
//	lamdis                                  interactive
//	lamdis "add a zero guard to div() and verify it"
//
// Every task is a signed entry in a thread named after the repository, and
// every run is recorded there: what it read, what it changed, what it ran,
// what it cost. The same thread is visible in the app and to any MCP client,
// so what the agent did at the terminal is part of the same record as
// everything else.

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/agent"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/perm"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/store"
)

func cmdCode(ctx context.Context, dataDir string, s store.Store, args []string) error {
	fs := flag.NewFlagSet("lamdis", flag.ContinueOnError)
	model := fs.String("model", "", "model id (default: Settings or LAMDIS_MODEL)")
	modelURL := fs.String("url", "", "OpenAI-compatible base URL for a local model, e.g. http://localhost:11434/v1")
	dir := fs.String("dir", "", "workspace directory (default: the git root of the current directory)")
	quiet := fs.Bool("q", false, "print only the answer")
	if err := fs.Parse(args); err != nil {
		return err
	}
	task := strings.TrimSpace(strings.Join(fs.Args(), " "))

	root := *dir
	if root == "" {
		root = repoRoot()
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
	if *model != "" {
		cfg.Model = *model
	}
	if *modelURL != "" {
		cfg.ModelURL = *modelURL
	}
	if *model != "" || *modelURL != "" {
		agent.SaveConfig(dataDir, cfg)
	}
	envModel := os.Getenv("LAMDIS_MODEL")
	var base agent.Model
	if or := agent.NewOpenRouter(os.Getenv("LAMDIS_OPENROUTER_KEY"), envModel); or != nil {
		base = or
	}
	names := func(principal string) string { return peerName(dataDir, principal) }
	runner := &agent.Runner{Store: s, PersonKey: priv, Person: pid, AgentKey: agentKey, Agent: agentPID,
		Model: base, ModelName: envModel, DataDir: dataDir, Names: names, Embedder: embedderFromEnv(),
		State: agent.LoadState(dataDir), Workspace: &agent.Workspace{Root: root}}
	if !runner.Ready() {
		return fmt.Errorf("no model is configured.\n  OpenRouter:  put LAMDIS_OPENROUTER_KEY=sk-or-... in %s/.env (keys at openrouter.ai/keys)\n  local model: lamdis -url http://localhost:11434/v1 -model qwen3.5:4b", dataDir)
	}
	if !*quiet {
		runner.OnTool = func(name string, args map[string]any, out string, took time.Duration) {
			sum := strings.ReplaceAll(toolArgSummary(name, args), root+"/", "")
			status := ""
			if strings.HasPrefix(out, "error:") {
				status = " \033[31m" + trunc(strings.TrimPrefix(out, "error: "), 60) + "\033[0m"
			}
			fmt.Fprintf(os.Stderr, "  \033[2m→ %s %s (%s)\033[0m%s\n", name, sum, took.Round(100*time.Millisecond), status)
		}
	}

	thread, err := codeThread(ctx, s, priv, pid, root)
	if err != nil {
		return err
	}
	_, mname := runner.ModelFor(cfg)
	if !*quiet {
		fmt.Fprint(os.Stderr, banner(filepath.Base(root), mname))
	}

	ask := func(text string) error {
		q, err := personAppendCLI(ctx, s, priv, thread, protolog.Draft{Kind: agent.KindQuestion, Lane: protolog.LaneContent,
			Body: map[string]any{"text": text, "workspace": root}})
		if err != nil {
			return err
		}
		res := runner.Run(ctx, agent.Trigger{Kind: agent.TriggerCode, Thread: thread, Entry: q.ID})
		for res.Outcome == "waiting" {
			// The agent asked something. Answer here, in the terminal.
			reply, err := promptDecision(ctx, s, thread, res)
			if err != nil {
				return err
			}
			r, err := personAppendCLI(ctx, s, priv, thread, protolog.Draft{Kind: agent.KindDecisionReply, Lane: protolog.LaneContent,
				Refs: &protolog.Refs{RepliesTo: res.Outputs[len(res.Outputs)-1]},
				Body: map[string]any{"choice": reply.choice, "text": reply.text}})
			if err != nil {
				return err
			}
			res = runner.Run(ctx, agent.Trigger{Kind: agent.TriggerDecision, Thread: thread, Entry: r.ID})
		}
		if res.Outcome == "error" {
			return fmt.Errorf("%s", res.Error)
		}
		fmt.Println(res.Answer)
		return nil
	}

	if task != "" {
		return ask(task)
	}
	// Interactive.
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 1<<20)
	fmt.Fprintf(os.Stderr, "\033[2mType a task. Empty line or Ctrl-D to quit.\033[0m\n")
	for {
		fmt.Fprint(os.Stderr, "\n\033[1m› \033[0m")
		if !in.Scan() {
			return nil
		}
		line := strings.TrimSpace(in.Text())
		if line == "" || line == "/quit" || line == "/q" {
			return nil
		}
		if err := ask(line); err != nil {
			fmt.Fprintf(os.Stderr, "\033[31m%v\033[0m\n", err)
		}
	}
}

type decisionReply struct{ choice, text string }

// promptDecision shows the agent's question and reads the answer.
func promptDecision(ctx context.Context, s store.Store, thread string, res agent.Result) (decisionReply, error) {
	tl, err := s.Thread(ctx, thread)
	if err != nil {
		return decisionReply{}, err
	}
	var q string
	var opts []string
	if len(res.Outputs) > 0 {
		if e := tl.Get(res.Outputs[len(res.Outputs)-1]); e != nil {
			var b struct {
				Text    string   `json:"text"`
				Options []string `json:"options"`
			}
			json.Unmarshal(e.Body, &b)
			q, opts = b.Text, b.Options
		}
	}
	fmt.Fprintf(os.Stderr, "\n\033[36m? %s\033[0m\n", q)
	for i, o := range opts {
		fmt.Fprintf(os.Stderr, "  %d) %s\n", i+1, o)
	}
	fmt.Fprint(os.Stderr, "\033[1m› \033[0m")
	in := bufio.NewScanner(os.Stdin)
	if !in.Scan() {
		return decisionReply{}, fmt.Errorf("no answer")
	}
	ans := strings.TrimSpace(in.Text())
	for i, o := range opts {
		if ans == fmt.Sprint(i+1) || strings.EqualFold(ans, o) {
			return decisionReply{choice: o}, nil
		}
	}
	return decisionReply{text: ans}, nil
}

// codeThread finds or creates the thread for a workspace. Threads are
// matched by title, "code: <directory name>", among the ones you steward.
func codeThread(ctx context.Context, s store.Store, priv ed25519.PrivateKey, pid, root string) (string, error) {
	title := "code: " + filepath.Base(root)
	ids, err := s.Threads(ctx)
	if err != nil {
		return "", err
	}
	for _, id := range ids {
		tl, err := s.Thread(ctx, id)
		if err != nil {
			continue
		}
		st := perm.Fold(id, tl.Entries())
		if st.Title == title && st.Stewards[pid] {
			return id, nil
		}
	}
	_, genesis, err := protolog.NewThreadWith(priv, title, false, nil)
	if err != nil {
		return "", err
	}
	if err := s.AppendEntries(ctx, []*protolog.Entry{genesis}); err != nil {
		return "", err
	}
	if _, err := personAppendCLI(ctx, s, priv, genesis.ID, protolog.Draft{Kind: protolog.KindMessage, Lane: protolog.LaneContent,
		Body: map[string]any{"text": "Workspace: " + root}}); err != nil {
		return "", err
	}
	return genesis.ID, nil
}

func personAppendCLI(ctx context.Context, s store.Store, key ed25519.PrivateKey, thread string, d protolog.Draft) (*protolog.Entry, error) {
	tl, err := s.Thread(ctx, thread)
	if err != nil {
		return nil, err
	}
	a, err := protolog.NewAuthor(tl, key)
	if err != nil {
		return nil, err
	}
	e, err := a.Append(d)
	if err != nil {
		return nil, err
	}
	if err := s.AppendEntries(ctx, []*protolog.Entry{e}); err != nil {
		return nil, err
	}
	return e, nil
}

func toolArgSummary(name string, args map[string]any) string {
	get := func(k string) string {
		v, _ := args[k].(string)
		return v
	}
	switch name {
	case "read_file", "edit_file", "write_file":
		return get("path")
	case "run":
		return trunc(get("command"), 80)
	case "search_files":
		return "/" + trunc(get("pattern"), 40) + "/"
	case "list_files":
		return get("path")
	case "search_context", "fetch_url":
		return trunc(get("query")+get("url"), 60)
	}
	return ""
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// repoRoot is the git root of the current directory, or the directory.
func repoRoot() string {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err == nil {
		if p := strings.TrimSpace(string(out)); p != "" {
			return p
		}
	}
	wd, _ := os.Getwd()
	return wd
}

// banner is the block mark: three blocks in an L, the same shape as the
// site's cube logo, in the site's gold.
func banner(repo, model string) string {
	g := "\033[38;5;214m"
	d := "\033[2m"
	z := "\033[0m"
	// An L, the right way up: a stem, then a foot to the right.
	return g + "  ▛▀▜" + z + "\n" +
		g + "  ▙▄▟" + z + "         \033[1mlamdis\033[0m " + d + "· " + repo + " · " + model + z + "\n" +
		g + "  ▛▀▜▛▀▜" + z + "\n" +
		g + "  ▙▄▟▙▄▟" + z + "      " + d + "everything here is recorded · `lamdis app` to read it" + z + "\n"
}

// cmdApp opens the interface in a browser, starting the node if it is not
// already running.
//
// "Where is the web app" should never be answered with "find the URL the
// server printed, and the token in it". One word opens it.
func cmdApp(ctx context.Context, dataDir string, s store.Store, args []string) error {
	fs := flag.NewFlagSet("app", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:8420", "address the node listens on")
	if err := fs.Parse(args); err != nil {
		return err
	}
	tokenPath := filepath.Join(dataDir, "portal.token")
	raw, err := os.ReadFile(tokenPath)
	if err != nil {
		tok, err := api.NewPortalToken()
		if err != nil {
			return err
		}
		if err := os.WriteFile(tokenPath, []byte(tok), 0o600); err != nil {
			return err
		}
		raw = []byte(tok)
	}
	token := strings.TrimSpace(string(raw))
	host := *addr
	if strings.HasPrefix(host, ":") {
		host = "127.0.0.1" + host
	}
	url := "http://" + host + "/app?token=" + token

	// Already running? Then just open it.
	client := &http.Client{Timeout: 700 * time.Millisecond}
	if resp, err := client.Get("http://" + host + "/v1/node"); err == nil {
		resp.Body.Close()
		fmt.Fprintf(os.Stderr, "opening %s\n", "http://"+host+"/app")
		if os.Getenv("LAMDIS_NO_OPEN") == "" {
			openBrowser(url)
		}
		return nil
	}
	// Not running: start it here. serve opens the browser itself.
	fmt.Fprintf(os.Stderr, "starting the node…\n")
	return cmdServe(dataDir, s, []string{"-addr", *addr})
}
