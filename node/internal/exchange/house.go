package exchange

import (
	"context"
	"fmt"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
)

// Work the exchange posts on its own account.
//
// The bootstrap loop puts real observe jobs on the board from a house budget.
// It must reach the board by the same gate as a stranger's agent — screening,
// the mass-low-value check, the geofence rule for V2, escrow before listing —
// so this is that gate, lifted out of the HTTP handler where a caller with no
// request could not reach it. Nothing here is a shortcut: a house job that
// fails screening is refused exactly as a buyer's would be.

// PostFunded escrows a listing from the buyer's balance and puts it on the
// board, applying every check a job posted over the API gets.
func (s *Server) PostFunded(ctx context.Context, buyer string, l *api.Listing) error {
	if l == nil || buyer == "" {
		return fmt.Errorf("post: a job needs a listing and a buyer")
	}
	if !api.IsWork(l.Kind) {
		return fmt.Errorf("post: kind must be observe or do")
	}
	if l.PayMinor <= 0 {
		return fmt.Errorf("post: a job must pay for the work it asks for")
	}
	if l.Currency == "" {
		l.Currency = "USD"
	}
	if l.Slots <= 0 {
		l.Slots = 1
	}
	if l.Tier == "" {
		l.Tier = "V2"
	}
	l.Owner = buyer
	if ref := api.Screen(l.Title, l.Detail, l.Instructions, l.Deliverable); ref != nil {
		return ref
	}
	if ref := api.MassLowValue(l.Slots, l.PayMinor); ref != nil {
		return ref
	}
	if l.Where != "" && l.RadiusM <= 0 && (l.Tier == "V2" || l.Tier == "V3") {
		return fmt.Errorf("post: a %s job with an address needs lat, lon and radius_m", l.Tier)
	}
	need := MaxPayoutFor(l)
	if s.Ledger != nil {
		if _, err := s.Ledger.Hold(ctx, "hold-"+l.Job, l.Job, buyer, need, l.Currency); err != nil {
			return err
		}
	}
	if err := s.Board.Post(l); err != nil {
		// The money was committed to a job that never listed. Put it back now
		// rather than leaving it for a sweep that only looks at the board.
		if s.Ledger != nil {
			_, _ = s.Ledger.Release(ctx, "release:"+l.Job, l.Job, buyer, need, l.Currency)
		}
		return err
	}
	s.setBuyer(l.Job, buyer)
	return nil
}
