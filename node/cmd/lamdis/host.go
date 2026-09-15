package main

// `lamdis host` runs many people's nodes from one process.
//
// This is what makes "it keeps working while you are away" true. A node on a
// laptop stops when the lid closes; a hosted one does not. It is the same
// binary and the same on-disk layout, one directory per account, so anybody
// can take their directory and run it themselves.

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/agent"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
)

func cmdHost(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("host", flag.ContinueOnError)
	addr := fs.String("addr", envOr("LAMDIS_ADDR", ":8080"), "listen address")
	root := fs.String("root", envOr("LAMDIS_ACCOUNTS", "/data/accounts"), "directory holding one folder per account")
	maxAccounts := fs.Int("max-accounts", envInt("LAMDIS_MAX_ACCOUNTS", 25), "refuse new sign-ups past this many accounts")
	starterCap := fs.Float64("starter", envFloat("LAMDIS_STARTER_CAP", 0.5), "credit to give each new account, in dollars; 0 for none")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cog := api.NewCognito(os.Getenv("LAMDIS_COGNITO_REGION"), os.Getenv("LAMDIS_COGNITO_POOL"), os.Getenv("LAMDIS_COGNITO_CLIENT"))
	if !cog.Enabled() {
		return fmt.Errorf("no user pool configured.\n" +
			"  LAMDIS_COGNITO_REGION   e.g. us-east-1\n" +
			"  LAMDIS_COGNITO_POOL     the user pool id\n" +
			"  LAMDIS_COGNITO_CLIENT   the app client id\n" +
			"  LAMDIS_SIGNIN_DOMAIN    the pool's hosted domain, e.g. auth.lamdis.ai\n" +
			"  LAMDIS_SIGNIN_REDIRECT  where sign-in returns to, e.g. https://app.lamdis.ai/app")
	}

	model := strings.TrimSpace(os.Getenv("LAMDIS_MODEL"))
	if model == "" {
		model = agent.DefaultModel
	}
	h := &api.Host{
		Root:    *root,
		Cognito: cog,
		Model:   model,
		SignIn: api.SignIn{
			Domain:   os.Getenv("LAMDIS_SIGNIN_DOMAIN"),
			ClientID: os.Getenv("LAMDIS_COGNITO_CLIENT"),
			Redirect: envOr("LAMDIS_SIGNIN_REDIRECT", "http://localhost"+*addr+"/app"),
		},
		MaxAccounts: *maxAccounts,
		StarterCap:  *starterCap,
		KeyCeiling:  ceiling(),
		Logf:        func(f string, a ...any) { fmt.Fprintf(os.Stderr, f+"\n", a...) },
	}
	// Starter keys are minted only if a management key is present, and only
	// while the total committed stays under the ceiling.
	if mk := strings.TrimSpace(os.Getenv("LAMDIS_OPENROUTER_MANAGEMENT_KEY")); mk != "" && *starterCap > 0 {
		h.Starter = &agent.Provisioner{ManagementKey: mk}
	}

	if err := h.Start(ctx); err != nil {
		return err
	}
	fmt.Printf("host       http://localhost%s/app\n", *addr)
	fmt.Printf("accounts   %s · %d now · limit %d\n", *root, h.Count(), *maxAccounts)
	fmt.Printf("model      %s\n", model)
	if h.Starter != nil {
		fmt.Printf("starter    $%.2f each, stopping at a $%.2f ceiling\n", *starterCap, ceiling())
	} else {
		fmt.Printf("starter    off; new accounts bring their own key or write in\n")
	}
	fmt.Printf("sign-in    %s\n", h.SignIn.Domain)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           h.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return srv.ListenAndServe()
}

func envInt(k string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(k))); err == nil && v > 0 {
		return v
	}
	return def
}

func envFloat(k string, def float64) float64 {
	if v, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv(k)), 64); err == nil && v >= 0 {
		return v
	}
	return def
}
