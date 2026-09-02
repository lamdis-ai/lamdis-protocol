package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/anchor"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/bootstrap"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/exchange"
	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/media"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/vision"
)

// cmdServeExchange runs the exchange as a service.
//
// Two things are configuration rather than derived, both for the same reason:
// the exchange's identity must survive a restart, and a reviewer link must
// point at an address a phone can actually reach. Deriving either from the
// request or generating it at boot would break the moment there is more than
// one task, or the moment a task is replaced.
func cmdServeExchange(args []string) error {
	fs := flag.NewFlagSet("exchange", flag.ContinueOnError)
	addr := fs.String("addr", envOr("LAMDIS_ADDR", ":8080"), "address to listen on")
	baseURL := fs.String("base-url", envOr("LAMDIS_BASE_URL", ""),
		"public origin reviewer links point at, e.g. https://exchange.lamdis.ai")
	keyHex := fs.String("key", envOr("LAMDIS_EXCHANGE_KEY", ""),
		"hex-encoded ed25519 seed for the exchange's principal")
	seed := fs.Bool("seed-board", false,
		"put sample work on the board, so the marketplace has something to show")
	data := fs.String("data", envOr("LAMDIS_DATA", ""),
		"directory for the ledger and accounts; empty keeps them in memory and loses them on restart")
	novision := fs.Bool("no-vision", false,
		"run without a vision model; submissions are stored and never become payable")
	usdcEvery := fs.Duration("usdc-interval", 30*time.Second,
		"how often to scan the chain for USDC transfers, when LAMDIS_USDC_ADDRESS and LAMDIS_BASE_RPC are set")
	// The bootstrap loop: the exchange as its own first buyer. Off unless
	// asked for, and it cannot be asked for without a budget to cap it.
	boot := fs.Bool("bootstrap", envOr("LAMDIS_BOOTSTRAP", "") == "1" || envOr("LAMDIS_BOOTSTRAP", "") == "true",
		"post real observe jobs near signed-in operators from a capped house budget "+
			"(needs LAMDIS_HOUSE_BUDGET_MINOR and -data)")
	bootEvery := fs.Duration("bootstrap-every", envDuration("LAMDIS_BOOTSTRAP_EVERY", bootstrap.DefaultEvery),
		"how often the bootstrap loop runs")
	anchorOn := fs.Bool("anchor", true,
		"anchor receipt hashes to Bitcoin through OpenTimestamps; needs -data, costs nothing")
	anchorEvery := fs.Duration("anchor-every", time.Hour,
		"how often unanchored receipts are batched into one root and submitted")
	anchorCalendars := fs.String("anchor-calendars", envOr("LAMDIS_ANCHOR_CALENDARS",
		strings.Join(anchor.DefaultCalendars, ",")),
		"comma-separated OpenTimestamps calendar URLs to submit roots to")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *baseURL == "" {
		return fmt.Errorf("exchange: -base-url (or LAMDIS_BASE_URL) is required; " +
			"reviewer links must point somewhere a phone can reach")
	}

	key, generated, err := loadOrMakeKey(*keyHex)
	if err != nil {
		return err
	}

	opt := exchange.Options{DataDir: *data}
	if !*novision {
		// Nil is a working configuration — evidence is stored and stays
		// unpayable — so a missing model degrades the exchange rather than
		// stopping it.
		opt.Vision = vision.NewBedrock(
			os.Getenv("AWS_PROFILE"),
			envOr("AWS_REGION", "us-east-1"),
			os.Getenv("LAMDIS_VISION_MODEL"))
	}
	// Video needs ffmpeg. Without it, video uploads are refused with a reason
	// rather than silently accepted and never checked.
	if ff := media.NewFFmpeg(); ff.Available() {
		opt.Media = ff
	}

	srv, err := exchange.Open(key, *baseURL, opt)
	if err != nil {
		return err
	}

	// The house budget is the only money the loop can move, and it is read
	// from the environment rather than a flag so it cannot be raised by
	// editing a service file's arguments without also meaning to.
	var houseBudget int64
	if v := os.Getenv("LAMDIS_HOUSE_BUDGET_MINOR"); v != "" {
		houseBudget, err = strconv.ParseInt(v, 10, 64)
		if err != nil || houseBudget <= 0 {
			return fmt.Errorf("exchange: LAMDIS_HOUSE_BUDGET_MINOR must be a positive integer of minor units, got %q", v)
		}
	}
	loop, err := bootstrap.New(bootstrap.Config{
		Enabled: *boot, BudgetMinor: houseBudget, DataDir: *data, Every: *bootEvery,
		Places: bootstrap.NewOverpass(os.Getenv("LAMDIS_OVERPASS_URL")),
	})
	if err != nil {
		return err
	}
	loop.Attach(srv)

	if *seed {
		if err := seedPanel(srv); err != nil {
			return err
		}
		if err := seedBoard(srv); err != nil {
			return err
		}
		// A worked example of multi-part work, for the side that would do it.
		if err := seedDemoProject(srv); err != nil {
			return err
		}
	}

	fmt.Printf("lamdis exchange\n")
	fmt.Printf("  principal  %s\n", srv.PID)
	fmt.Printf("  base url   %s\n", *baseURL)
	fmt.Printf("  listening  %s\n", *addr)
	if os.Getenv("LAMDIS_ACP_KEY") != "" && os.Getenv("LAMDIS_ACP_SECRET") != "" {
		fmt.Printf("  checkout   agentic checkout is ON at %s/acp\n", *baseURL)
	} else {
		fmt.Printf("  checkout   agentic checkout is off " +
			"(set LAMDIS_ACP_KEY and LAMDIS_ACP_SECRET)\n")
	}
	// State the shape of the deployment, because every one of these being
	// absent is a silent failure that looks like a working service.
	fmt.Printf("  storage    %s\n", orMemory(*data))
	if srv.USDC != nil {
		fmt.Printf("  usdc       ON — watching %s for transfers to %s (no key held)\n",
			srv.USDC.Net.Name, srv.USDC.Address())
	} else {
		fmt.Printf("  usdc       off (set LAMDIS_USDC_ADDRESS and LAMDIS_BASE_RPC)\n")
	}
	fmt.Printf("  verifier   %s\n", yesNo(opt.Vision != nil, "vision model", "NONE — nothing can be paid"))
	fmt.Printf("  video      %s\n", yesNo(opt.Media != nil, "ffmpeg", "unavailable — video will be refused"))
	fmt.Printf("  accounts   %s\n", yesNo(srv.Workers.Cognito.Enabled(), "cognito", "NONE — nobody can sign in"))
	fmt.Printf("  board      %s/board\n", *baseURL)
	// Anchoring needs somewhere to keep the log and the proofs; an in-memory
	// exchange would lose every root it ever submitted. Off is a choice,
	// not a silent default.
	anchorDesc := "off (-anchor=false)"
	if *anchorOn && *data == "" {
		anchorDesc = "off (no -data directory to keep proofs in)"
	} else if *anchorOn {
		cals := strings.Split(*anchorCalendars, ",")
		for i := range cals {
			cals[i] = strings.TrimSpace(cals[i])
		}
		a, err := anchor.Open(filepath.Join(*data, "anchor"), anchor.Options{
			Calendars: cals, Every: *anchorEvery,
		})
		if err != nil {
			return fmt.Errorf("opening the anchor store: %w", err)
		}
		defer a.Close()
		srv.Anchors = a
		anchorDesc = fmt.Sprintf("opentimestamps every %s via %s", *anchorEvery, strings.Join(cals, ", "))
	}
	fmt.Printf("  anchoring  %s\n", anchorDesc)
	if loop.Enabled() {
		fmt.Printf("  bootstrap  ON — house budget %d minor, every %s; status at %s/v1/bootstrap\n",
			houseBudget, *bootEvery, *baseURL)
	} else {
		fmt.Printf("  bootstrap  off (set -bootstrap and LAMDIS_HOUSE_BUDGET_MINOR)\n")
	}
	if generated {
		// A generated key means every restart is a different exchange, and
		// anything it signed before becomes unverifiable. Say so loudly.
		fmt.Printf("\n  WARNING: no key supplied, so one was generated for this process.\n")
		fmt.Printf("  Attestations signed now cannot be verified after a restart.\n")
		fmt.Printf("  Set LAMDIS_EXCHANGE_KEY to a stable seed in any real deployment.\n\n")
	}

	// Nothing else in the service ever looks at a listing again once it stops
	// being claimable, so without this an expired job holds its buyer's money
	// forever.
	sweepCtx, stopSweep := context.WithCancel(context.Background())
	defer stopSweep()
	srv.StartSweeper(sweepCtx, 5*time.Minute)
	// Earned money should not wait on somebody remembering to press a button —
	// least of all the person who is owed it. Hourly, because payouts are
	// batched to a threshold anyway and a tighter loop only adds rail calls.
	srv.StartPayoutSweeper(sweepCtx, time.Hour)
	// Transfers to the exchange's address fund jobs; nobody else will notice
	// them arriving.
	srv.StartChainWatcher(sweepCtx, *usdcEvery)
	// Rebuild the payout mapping now rather than on somebody's first click.
	srv.WarmPayoutAccounts(sweepCtx)
	// Tell buyers when their work is going unfilled, while they can still act.
	srv.StartAlerts(sweepCtx, 30*time.Minute)
	// Batch receipt hashes and get the roots onto the chain. Network work
	// lives in this loop only; serving a receipt never waits on a calendar.
	if srv.Anchors != nil {
		srv.Anchors.Start(sweepCtx)
	}
	// The exchange as its own first buyer, if asked. See internal/bootstrap.
	loop.Start(sweepCtx)

	mux := srv.Handler()
	// Public, read-only: what the loop is doing and what it has found.
	loop.Register(mux)
	server := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return server.ListenAndServe()
}

// loadOrMakeKey reads a stable seed, or generates an ephemeral one and reports
// that it did.
func loadOrMakeKey(seedHex string) (ed25519.PrivateKey, bool, error) {
	if seedHex == "" {
		_, priv, err := ed25519.GenerateKey(nil)
		return priv, true, err
	}
	seed, err := hex.DecodeString(seedHex)
	if err != nil {
		return nil, false, fmt.Errorf("exchange key is not valid hex: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, false, fmt.Errorf("exchange key must be %d bytes of hex, got %d",
			ed25519.SeedSize, len(seed))
	}
	return ed25519.NewKeyFromSeed(seed), false, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// envDuration reads a duration, falling back on anything unset or unreadable.
func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return fallback
}

var _ = protolog.GenesisPrev

// seedBoard puts sample work on the marketplace.
//
// Only for looking at the thing. Real listings arrive over /v1/tasks and
// /v1/panels from an authenticated principal, and nothing here is escrowed, so
// none of it would actually pay.
func seedBoard(srv *exchange.Server) error {
	now := time.Now()

	// Practice runs, not fake work.
	//
	// Shown only while an operator's board is otherwise empty: once real work
	// is in range — a buyer's, or the bootstrap loop's — the board hides
	// these. See Board.ForOperator.
	//
	// Seeded listings used to be indistinguishable from real ones, so somebody
	// could claim one, drive to an address and find nothing there — which
	// teaches them the board is fake and is a worse first experience than an
	// empty board. These say what they are, pay nothing, and can be done from
	// a kitchen table. Their job is to let an operator learn the flow — claim,
	// photograph the code, submit, see the verdict — before any of it matters.
	//
	// Coordinates are Detroit, because a marketplace is a place before it is a
	// product: ten jobs in one city is liquidity and ten across ten states is
	// nothing. They are on the listings so the board's range filter treats a
	// practice run like real work: somebody in Detroit sees it, somebody who
	// set a fifty-mile range in Phoenix does not. They used to be computed and
	// thrown away, and every practice job showed to everybody everywhere.
	const (
		detroitLatE7 = 423314000 // Campus Martius, downtown
		detroitLonE7 = -830458000
	)
	samples := []*api.Listing{
		{
			Job: "practice-1", Kind: api.KindTask,
			Title:    "Practice: photograph anything with the code in frame",
			Detail:   "A practice run. Nothing here is real work and nobody is paying for it.",
			Practice: true,
			Instructions: "Write the code you are given on a piece of paper, " +
				"photograph it next to anything at all, and submit. You are " +
				"learning where the buttons are.",
			Deliverable: "One photo with the code legible.",
			Area:        "Detroit, MI",
			LatE7:       detroitLatE7, LonE7: detroitLonE7,
			Tier:     "V1",
			PayMinor: 0, Currency: "USD",
			Slots: 50, Expires: now.Add(365 * 24 * time.Hour), Posted: now,
		},
		{
			Job: "practice-2", Kind: api.KindDo,
			Title:    "Practice: show something moved",
			Detail:   "A practice run for a do-job, where the photograph has to show the work finished rather than just show you were there.",
			Practice: true,
			Instructions: "Move any object somewhere else and photograph it in " +
				"its new place with the code in frame. This is the difference " +
				"between proving you turned up and proving you did something.",
			Deliverable: "One photo of the object in its new place, code legible.",
			Area:        "Detroit, MI",
			LatE7:       detroitLatE7, LonE7: detroitLonE7,
			Tier:     "V1",
			PayMinor: 0, Currency: "USD",
			Slots: 50, Expires: now.Add(365 * 24 * time.Hour), Posted: now,
		},
	}

	// Practice work is unpaid, so there is nothing to escrow and nothing to
	// fund. The board refuses unfunded paid work, which is the check doing its
	// job — these are exempt because they cost nothing.
	for _, l := range samples {
		if err := srv.Board.Post(l); err != nil {
			return err
		}
	}
	return nil
}

// seedPanel gives the review surface something to show. Kept separate from the
// board seed because it needs real escrow.
func seedPanel(srv *exchange.Server) error {
	now := time.Now()
	ctx := context.Background()
	const demoBuyer = "demo-buyer"
	if _, err := srv.Ledger.Topup(ctx, "seed-topup", demoBuyer, 100000, "USD", "seed"); err != nil {
		return err
	}
	_ = now
	// A panel needs the artifact it is judging, or its page has nothing to
	// show and every seat assigned to it looks like a broken link.
	panelHold := int64((150 + 100) * 3)
	if _, err := srv.Ledger.Hold(ctx, "seed-hold-panel", "panel-demo-1",
		demoBuyer, panelHold, "USD"); err != nil {
		return err
	}
	return srv.AddPanel(&api.ReviewPanel{
		Job: "panel-demo-1", Parent: "practice-1",
		Question:  "Does this photograph show a FOR LEASE sign at the address given?",
		Context:   "Automated verification reached 73%, below the 90% the buyer asked for.",
		Reviewers: 3, Agreement: 2, Practice: true,
		FeeMinor: 150, BonusMinor: 100, Currency: "USD",
		Expires: now.Add(2 * time.Hour),
	}, samplePhoto(), "image/jpeg")
}

// samplePhoto draws something a reviewer can actually look at, so the demo
// does not depend on a file being present.
func samplePhoto() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 480, 320))
	for y := 0; y < 320; y++ {
		for x := 0; x < 480; x++ {
			switch {
			case y < 120:
				img.Set(x, y, color.RGBA{R: 120, G: 160, B: 210, A: 255}) // sky
			case x > 150 && x < 330 && y > 150 && y < 230:
				img.Set(x, y, color.RGBA{R: 200, G: 40, B: 40, A: 255}) // a red sign
			default:
				img.Set(x, y, color.RGBA{R: 90, G: 90, B: 95, A: 255})
			}
		}
	}
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, nil)
	return buf.Bytes()
}

func orMemory(dir string) string {
	if dir == "" {
		return "in memory (LOST ON RESTART — set -data)"
	}
	return dir
}

func yesNo(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}
