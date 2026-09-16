package main

// `lamdis trust` is where you say once how much of this machine the agent
// may work in, instead of answering the same question all week.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/agent"
)

func cmdTrust(ctx context.Context, dataDir string, args []string) error {
	cfg, _ := agent.LoadConfig(dataDir)
	if len(args) == 0 {
		return showTrust(dataDir, cfg)
	}
	switch args[0] {
	case "project", "home", "all":
		cfg.Trust = args[0]
		if err := agent.SaveConfig(dataDir, cfg); err != nil {
			return err
		}
		fmt.Printf("The agent may now work %s.\n", agent.TrustSays(cfg.Trust))
		if cfg.Trust == "all" && !cfg.Unguarded {
			fmt.Printf("Credential stores are still refused. `lamdis trust -unguarded` if you really mean everything.\n")
		}
		return nil
	case "-unguarded", "--unguarded":
		cfg.Unguarded = true
		if err := agent.SaveConfig(dataDir, cfg); err != nil {
			return err
		}
		fmt.Printf("The guard is off. Your ssh keys, cloud credentials and browser profiles\nare now readable by an agent you can direct from your phone.\nTurn it back on with: lamdis trust -guarded\n")
		return nil
	case "-guarded", "--guarded":
		cfg.Unguarded = false
		if err := agent.SaveConfig(dataDir, cfg); err != nil {
			return err
		}
		fmt.Println("Credential stores are refused again.")
		return nil
	case "forget":
		if len(args) < 2 {
			cfg.AllowPaths = nil
			if err := agent.SaveConfig(dataDir, cfg); err != nil {
				return err
			}
			fmt.Println("Forgot every directory you had allowed.")
			return nil
		}
		abs, _ := filepath.Abs(args[1])
		kept := cfg.AllowPaths[:0]
		for _, p := range cfg.AllowPaths {
			if p != abs {
				kept = append(kept, p)
			}
		}
		cfg.AllowPaths = kept
		if err := agent.SaveConfig(dataDir, cfg); err != nil {
			return err
		}
		fmt.Printf("Forgot %s.\n", abs)
		return nil
	}
	// Anything else is a directory to allow from now on.
	for _, a := range args {
		abs, err := filepath.Abs(a)
		if err != nil {
			return err
		}
		if st, err := os.Stat(abs); err != nil || !st.IsDir() {
			return fmt.Errorf("%s is not a directory that exists", a)
		}
		if bad, what := agent.Guarded(abs); bad && !cfg.Unguarded {
			return fmt.Errorf("%s holds credentials, so it is not something to hand over. `lamdis trust -unguarded` first if you mean it", what)
		}
		already := false
		for _, p := range cfg.AllowPaths {
			if p == abs {
				already = true
			}
		}
		if !already {
			cfg.AllowPaths = append(cfg.AllowPaths, abs)
		}
		fmt.Printf("The agent may work in %s, and below it.\n", abs)
	}
	return agent.SaveConfig(dataDir, cfg)
}

func showTrust(dataDir string, cfg agent.Config) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "reach\t%s\t%s\n", cfg.Trust, agent.TrustSays(cfg.Trust))
	guard := "on"
	if cfg.Unguarded {
		guard = "OFF"
	}
	fmt.Fprintf(w, "guard\t%s\t%s\n", guard,
		map[bool]string{true: "credential stores are readable", false: "ssh keys, cloud credentials and browser profiles are refused"}[cfg.Unguarded])
	if len(cfg.AllowPaths) > 0 {
		fmt.Fprintf(w, "also\t%s\t%s\n", fmt.Sprint(len(cfg.AllowPaths)), strings.Join(cfg.AllowPaths, ", "))
	}
	w.Flush()
	fmt.Printf(`
  lamdis trust project     %s
  lamdis trust home        %s
  lamdis trust all         %s
  lamdis trust ~/work      allow one place, permanently
  lamdis trust forget      take it all back
`, agent.TrustSays("project"), agent.TrustSays("home"), agent.TrustSays("all"))
	return nil
}
