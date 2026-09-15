package main

// `lamdis try-server` is the thing behind the box on the website: somebody
// types a question and gets a real answer before they have installed
// anything. It holds nothing, writes nothing to disk, and forgets every
// session within the hour.

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/agent"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
)

func cmdTryServer(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("try-server", flag.ContinueOnError)
	addr := fs.String("addr", envOr("LAMDIS_ADDR", ":8090"), "listen address")
	origin := fs.String("origin", envOr("LAMDIS_TRY_ORIGIN", "https://lamdis.ai"), "site allowed to call this")
	perDay := fs.Int("per-visitor", envInt("LAMDIS_TRY_PER_VISITOR", 8), "questions per visitor per day")
	global := fs.Int("per-day", envInt("LAMDIS_TRY_PER_DAY", 400), "questions in total per day")
	if err := fs.Parse(args); err != nil {
		return err
	}
	model := envOr("LAMDIS_MODEL", agent.DefaultModel)
	// The demo gets its own key, capped, so the worst case is a number you
	// chose rather than whatever the internet decides to do.
	key := strings.TrimSpace(os.Getenv("LAMDIS_TRY_KEY"))
	if key == "" {
		key = strings.TrimSpace(os.Getenv("LAMDIS_OPENROUTER_KEY"))
	}
	t := &api.Try{
		ModelName:        model,
		Origin:           *origin,
		PerVisitorPerDay: *perDay,
		GlobalPerDay:     *global,
		Logf:             func(f string, a ...any) { fmt.Fprintf(os.Stderr, f+"\n", a...) },
	}
	if m := agent.NewOpenRouter(key, model); m != nil {
		t.Model = m
	}
	mux := http.NewServeMux()
	mux.Handle("/v1/try", t.Handler())
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })

	fmt.Printf("try        http://localhost%s/v1/try\n", *addr)
	fmt.Printf("origin     %s\n", *origin)
	fmt.Printf("limits     %d per visitor per day, %d per day in total\n", *perDay, *global)
	if t.Model == nil {
		fmt.Printf("model      OFF - set LAMDIS_TRY_KEY to a capped OpenRouter key (lamdis keys mint demo -limit 5)\n")
	} else {
		fmt.Printf("model      %s\n", model)
	}
	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	return srv.ListenAndServe()
}
