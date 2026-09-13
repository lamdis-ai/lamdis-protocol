package main

import (
	"context"
	"log"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/exchange"
)

// seededJobs are the listings -seed-board used to create on every boot. They
// persisted on disk, so a deployment that merely stopped seeding still showed
// them; removing them is an explicit act.
var seededJobs = []string{
	"practice-1", "practice-2",
	"demo-barn-slab", "demo-front-drive", "demo-back-drive",
}

// unseedBoard withdraws the practice runs and the demonstration project and
// returns the escrow the seeded review panel held against seed money.
//
// Idempotent: a listing already cancelled or absent is not an error, and the
// ledger's idempotency key makes the release a no-op the second time.
func unseedBoard(srv *exchange.Server) {
	removed := 0
	for _, job := range seededJobs {
		if _, ok := srv.Board.Get(job); !ok {
			continue
		}
		if err := srv.Board.Cancel(job); err != nil {
			log.Printf("unseed: %s: %v", job, err)
			continue
		}
		removed++
	}
	// The seeded panel's hold: $7.50 of seed money against a job that no
	// longer exists, which the reconciler flagged on every boot.
	if srv.Ledger != nil {
		if _, err := srv.Ledger.Release(context.Background(), "unseed-panel-demo-1",
			"panel-demo-1", "demo-buyer", 750, "USD"); err != nil {
			log.Printf("unseed: panel-demo-1 escrow: %v", err)
		}
	}
	if removed > 0 {
		log.Printf("unseed: withdrew %d seeded listing(s)", removed)
	}
}
