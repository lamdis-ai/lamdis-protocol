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
	guests := fs.Bool("guests", os.Getenv("LAMDIS_NO_GUESTS") == "", "let people start without signing up")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// A user pool is how somebody keeps a node across browsers. It is not
	// needed to start one, so a host without it still works; people simply
	// cannot carry their threads to another machine yet.
	cog := api.NewCognito(os.Getenv("LAMDIS_COGNITO_REGION"), os.Getenv("LAMDIS_COGNITO_POOL"), os.Getenv("LAMDIS_COGNITO_CLIENT"))
	if !cog.Enabled() && !*guests {
		return fmt.Errorf("nothing lets anybody in: either allow guests, or set\n" +
			"  LAMDIS_COGNITO_REGION, LAMDIS_COGNITO_POOL, LAMDIS_COGNITO_CLIENT,\n" +
			"  LAMDIS_SIGNIN_DOMAIN and LAMDIS_SIGNIN_REDIRECT")
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
		PublicBase:  strings.TrimSuffix(envOr("LAMDIS_SIGNIN_REDIRECT", ""), "/app"),
		Mail:        api.MailerFromEnv(),
		Guests:      *guests,
		// Somebody spending a credential they did not supply gets a short
		// menu of cheap models and a small daily allowance. Bring your own
		// key and both restrictions fall away.
		AllowedModels:        strings.Fields(envOr("LAMDIS_ALLOWED_MODELS", model+" openai/gpt-5.6-mini")),
		AccountRunsPerDay:    envInt("LAMDIS_ACCOUNT_RUNS", 25),
		AccountTokensPerDay:  envInt("LAMDIS_ACCOUNT_TOKENS", 60000),
		AccountFetchesPerDay: envInt("LAMDIS_ACCOUNT_FETCHES", 20),
		SharedKey:            strings.TrimSpace(os.Getenv("LAMDIS_HOST_MODEL_KEY")),
		StarterCap:           *starterCap,
		KeyCeiling:           ceiling(),
		Logf:                 func(f string, a ...any) { fmt.Fprintf(os.Stderr, f+"\n", a...) },
	}
	// Starter keys are minted only if a management key is present, and only
	// while the total committed stays under the ceiling.
	if mk := strings.TrimSpace(os.Getenv("LAMDIS_OPENROUTER_MANAGEMENT_KEY")); mk != "" && *starterCap > 0 {
		h.Starter = &agent.Provisioner{ManagementKey: mk}
	}

	if mk := strings.TrimSpace(os.Getenv("LAMDIS_TRY_KEY")); mk != "" {
		t := &api.Try{ModelName: model, Origin: envOr("LAMDIS_TRY_ORIGIN", "https://lamdis.ai"),
			Logf: func(f string, a ...any) { fmt.Fprintf(os.Stderr, f+"\n", a...) }}
		if m := agent.NewOpenRouter(mk, model); m != nil {
			t.Model = m
		}
		h.Try = t
	}
	if err := h.Start(ctx); err != nil {
		return err
	}
	shown := *addr
	if strings.HasPrefix(shown, ":") {
		shown = "localhost" + shown
	}
	fmt.Printf("host       http://%s/app\n", shown)
	fmt.Printf("accounts   %s · %d now · limit %d\n", *root, h.Count(), *maxAccounts)
	fmt.Printf("model      %s\n", model)
	switch {
	case h.Starter != nil:
		fmt.Printf("starter    $%.2f each, stopping at a $%.2f ceiling\n", *starterCap, ceiling())
	case os.Getenv("LAMDIS_HOST_MODEL_KEY") != "":
		fmt.Printf("model key  one shared key for every account; cap it\n")
		fmt.Printf("per account %d runs, %d tokens, %d fetches a day, and only %s\n",
			envInt("LAMDIS_ACCOUNT_RUNS", 25), envInt("LAMDIS_ACCOUNT_TOKENS", 60000),
			envInt("LAMDIS_ACCOUNT_FETCHES", 20), envOr("LAMDIS_ALLOWED_MODELS", model+", openai/gpt-5.6-mini"))
	default:
		fmt.Printf("starter    off; new accounts bring their own key or write in\n")
	}
	if *guests {
		fmt.Printf("starting   anybody can begin without signing up\n")
	}
	if cog.Enabled() {
		fmt.Printf("sign-in    %s\n", h.SignIn.Domain)
	} else {
		fmt.Printf("sign-in    off; nobody can carry a node to another browser yet\n")
	}

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
