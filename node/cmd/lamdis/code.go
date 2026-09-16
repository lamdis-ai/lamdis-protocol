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
	inProject := true
	if root == "" {
		root, inProject = workspaceHere()
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
	// Outside a project there is nothing sensible to read or edit, and
	// handing an agent somebody's whole home directory is worse than
	// useless. It still answers from the record, which is most of the point.
	// The same reach the listening agent uses, so the setting is chosen
	// once and means the same thing wherever you are.
	var ws *agent.Workspace
	extra, mayAsk := agent.Reach(cfg, root)
	if inProject || cfg.Trust != agent.TrustProject || len(extra) > 0 {
		ws = &agent.Workspace{Root: root, Guard: !cfg.Unguarded}
		for _, d := range extra {
			ws.Allow(d)
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
		if mayAsk {
			ws.Ask = func(ctx context.Context, path, why string) (bool, error) {
				return askAtTerminal(path, why)
			}
		}
	}
	runner := &agent.Runner{Store: s, PersonKey: priv, Person: pid, AgentKey: agentKey, Agent: agentPID,
		Model: base, ModelName: envModel, DataDir: dataDir, Names: names, Embedder: embedderFromEnv(),
		State: agent.LoadState(dataDir), Workspace: ws}
	if !runner.Ready() {
		return fmt.Errorf("no model is configured.\n  OpenRouter:  put LAMDIS_OPENROUTER_KEY=sk-or-... in %s/.env (keys at openrouter.ai/keys)\n  local model: lamdis -url http://localhost:11434/v1 -model qwen3.5:4b", dataDir)
	}
	spin := newSpinner(os.Stderr)
	if !*quiet {
		// What it just did goes above the line; what it is doing now stays
		// on it. So a wait always says something, and the transcript
		// afterwards reads as though nothing was ever spinning.
		runner.OnTool = func(name string, args map[string]any, out string, took time.Duration) {
			sum := strings.ReplaceAll(toolArgSummary(name, args), root+"/", "")
			status := ""
			if strings.HasPrefix(out, "error:") {
				status = " \033[31m" + trunc(strings.TrimPrefix(out, "error: "), 60) + "\033[0m"
			}
			spin.Note("  \033[2m→ %s %s (%s)\033[0m%s\n", name, sum, took.Round(100*time.Millisecond), status)
			spin.Say("thinking")
		}
		runner.OnStep = func(what string, args map[string]any) {
			spin.Say(doing(what, args))
		}
	}

	thread, err := codeThread(ctx, s, priv, pid, root, inProject)
	if err != nil {
		return err
	}
	_, mname := runner.ModelFor(cfg)
	where := filepath.Base(root)
	if !inProject {
		where = "no project here"
		if ws != nil && cfg.Trust != agent.TrustProject {
			where = agent.TrustSays(cfg.Trust)
		}
	}
	if !*quiet {
		fmt.Fprint(os.Stderr, banner(where, mname))
	}

	// What somebody typed is not thrown away because the network hiccuped.
	// It goes back in the box, so Enter tries it again.
	var retry string
	ask := func(text string) error {
		q, err := personAppendCLI(ctx, s, priv, thread, protolog.Draft{Kind: agent.KindQuestion, Lane: protolog.LaneContent,
			Body: map[string]any{"text": text, "workspace": root}})
		if err != nil {
			return err
		}
		retry = text
		spin.Start("thinking")
		res := runner.Run(ctx, agent.Trigger{Kind: agent.TriggerCode, Thread: thread, Entry: q.ID})
		spin.Stop()
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
			spin.Start("thinking")
			res = runner.Run(ctx, agent.Trigger{Kind: agent.TriggerDecision, Thread: thread, Entry: r.ID})
			spin.Stop()
		}
		if res.Outcome == "error" {
			return fmt.Errorf("%s", res.Error)
		}
		retry = ""
		fmt.Println(res.Answer)
		return nil
	}

	if task != "" {
		return ask(task)
	}
	// Interactive.
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 1<<20)
	switch {
	case inProject:
		fmt.Fprintf(os.Stderr, "\033[2mAsk it anything about this project, or tell it what to change. /help for more.\033[0m\n")
	case ws != nil:
		fmt.Fprintf(os.Stderr, "\033[2mNo project here, but it may work %s. /help for more.\033[0m\n", agent.TrustSays(cfg.Trust))
	default:
		fmt.Fprintf(os.Stderr, "\033[2mNo project here, so it answers from your record. `lamdis trust home` lets it work on files anywhere under your home directory. /help for more.\033[0m\n")
	}
	for {
		fmt.Fprint(os.Stderr, "\n\033[1m› \033[0m")
		if !in.Scan() {
			fmt.Fprintln(os.Stderr)
			return nil
		}
		line := strings.TrimSpace(in.Text())
		if line == "" {
			// Enter on an empty line retries whatever did not get through.
			if retry == "" {
				continue
			}
			line, retry = retry, ""
			fmt.Fprintf(os.Stderr, "\033[2m%s\033[0m\n", line)
		}
		switch line {
		case "/quit", "/q", "exit", "quit":
			return nil
		case "/help", "help", "?":
			fmt.Fprint(os.Stderr, cliHelp(ws != nil, thread))
			continue
		case "/trust":
			if err := cmdTrust(ctx, dataDir, nil); err != nil {
				fmt.Fprintf(os.Stderr, "\033[31m%v\033[0m\n", err)
			}
			continue
		case "/threads":
			if err := cmdThreads(ctx, s); err != nil {
				fmt.Fprintf(os.Stderr, "\033[31m%v\033[0m\n", err)
			}
			continue
		case "/app":
			fmt.Fprintf(os.Stderr, "\033[2mopening your record…\033[0m\n")
			go cmdApp(ctx, dataDir, s, nil)
			continue
		case "/here":
			fmt.Fprintf(os.Stderr, "\033[2m%s · thread %s · %s\033[0m\n", root, thread[:12], mname)
			continue
		}
		if err := ask(line); err != nil {
			fmt.Fprintf(os.Stderr, "\033[31m%v\033[0m\n", err)
			if retry != "" {
				fmt.Fprintf(os.Stderr, "\033[2mYour question is still here. Press Enter to try it again.\033[0m\n")
			}
		}
	}
}

// cliHelp is what you get for asking, which should never be a wall.
func cliHelp(canWork bool, thread string) string {
	d, z, b := "\033[2m", "\033[0m", "\033[1m"
	s := "\n" + b + "What you can say" + z + "\n"
	if canWork {
		s += "  " + d + "a task" + z + "        add a test for the login bug\n" +
			"  " + d + "a question" + z + "    why does auth fail on refresh?\n" +
			"  " + d + "it reads, edits and runs your tests, then reports back\n" + z
	} else {
		s += "  " + d + "a question" + z + "    what did we decide about the Acme rate?\n" +
			"  " + d + "a note" + z + "        anything you tell it is kept and searchable\n" +
			"  " + d + "`lamdis trust home` lets it work on files, anywhere under your home\n" + z
	}
	s += "\n" + b + "Commands" + z + "\n" +
		"  /threads     " + d + "everything in your record" + z + "\n" +
		"  /trust       " + d + "how much of this machine it may use" + z + "\n" +
		"  /app         " + d + "open it in a browser" + z + "\n" +
		"  /here        " + d + "where you are and what is answering" + z + "\n" +
		"  /quit        " + d + "or Ctrl-D" + z + "\n\n" +
		d + "This conversation is thread " + thread[:12] + ", kept with everything else.\n" + z
	return s
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
func codeThread(ctx context.Context, s store.Store, priv ed25519.PrivateKey, pid, root string, inProject bool) (string, error) {
	title := "code: " + filepath.Base(root)
	if !inProject {
		// Not a project, so this is just where terminal conversations go.
		title = "terminal"
	}
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
	if inProject {
		if _, err := personAppendCLI(ctx, s, priv, genesis.ID, protolog.Draft{Kind: protolog.KindMessage, Lane: protolog.LaneContent,
			Body: map[string]any{"text": "Workspace: " + root}}); err != nil {
			return "", err
		}
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

// workspaceHere decides whether the directory you are standing in is a
// project worth giving an agent, and says so.
//
// A git root obviously is. A directory with the marks of a project probably
// is. Your home directory is not, and handing an agent everything in it is
// worse than handing it nothing.
func workspaceHere() (string, bool) {
	if out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output(); err == nil {
		if p := strings.TrimSpace(string(out)); p != "" {
			return p, true
		}
	}
	wd, _ := os.Getwd()
	if home, err := os.UserHomeDir(); err == nil && filepath.Clean(wd) == filepath.Clean(home) {
		return wd, false
	}
	for _, mark := range []string{"go.mod", "package.json", "pyproject.toml", "Cargo.toml",
		"Gemfile", "pom.xml", "build.gradle", "composer.json", "Makefile", "requirements.txt"} {
		if _, err := os.Stat(filepath.Join(wd, mark)); err == nil {
			return wd, true
		}
	}
	return wd, false
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

// askAtTerminal is the version of the question for somebody who is right
// here: one line, one keystroke, and the answer is remembered.
func askAtTerminal(path, why string) (bool, error) {
	fmt.Fprintf(os.Stderr, "\n\033[36m? May I work in %s\033[0m", path)
	if strings.TrimSpace(why) != "" {
		fmt.Fprintf(os.Stderr, "\033[2m — %s\033[0m", strings.TrimSpace(why))
	}
	fmt.Fprintf(os.Stderr, "\n\033[2m  [y] yes, and remember it   [n] no\033[0m\n\033[1m› \033[0m")
	in := bufio.NewReader(os.Stdin)
	line, err := in.ReadString('\n')
	if err != nil {
		return false, nil
	}
	a := strings.ToLower(strings.TrimSpace(line))
	return a == "y" || a == "yes" || a == "allow", nil
}
