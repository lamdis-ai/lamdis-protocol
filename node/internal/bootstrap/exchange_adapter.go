package bootstrap

import (
	"context"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/exchange"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/ledger"
)

// exchangeMarket is the real exchange seen through the Market interface.
type exchangeMarket struct{ s *exchange.Server }

// Attach binds the loop to a running exchange: the market it posts to, and
// the hook that tells it what house jobs found.
func (l *Loop) Attach(s *exchange.Server) {
	l.cfg.Market = &exchangeMarket{s: s}
	s.OnAccepted = l.RecordAccepted
}

func (m *exchangeMarket) Operators() map[string]api.Capacity { return m.s.Capacities.All() }
func (m *exchangeMarket) Open() []*api.Listing               { return m.s.Board.Listings() }
func (m *exchangeMarket) Get(job string) (*api.Listing, bool) {
	return m.s.Board.Get(job)
}
func (m *exchangeMarket) PostFunded(ctx context.Context, buyer string, l *api.Listing) error {
	return m.s.PostFunded(ctx, buyer, l)
}
func (m *exchangeMarket) Balance(ctx context.Context, principal string) (int64, error) {
	if m.s.Ledger == nil {
		return 0, nil
	}
	return m.s.Ledger.Balance(ctx, ledger.BalanceOf(principal), "USD")
}
func (m *exchangeMarket) Topup(ctx context.Context, key, principal string, amountMinor int64) error {
	if m.s.Ledger == nil {
		return nil
	}
	_, err := m.s.Ledger.Topup(ctx, key, principal, amountMinor, "USD", "house budget")
	return err
}
func (m *exchangeMarket) Widen(job string, radiusM int64) bool { return m.s.Board.Widen(job, radiusM) }
func (m *exchangeMarket) Alert(l *api.Listing)                 { m.s.AlertNewWork(l) }
func (m *exchangeMarket) Interested() []api.Capacity {
	if m.s.Coverage == nil {
		return nil
	}
	return m.s.Coverage.Positioned()
}
