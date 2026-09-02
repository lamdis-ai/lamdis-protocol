package exchange

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/account"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/ledger"
)

// Money in, and money out.
//
// Neither of these existed. A buyer had no way to fund a balance and a worker
// no way to be paid, so the marketplace was a closed system that could only
// ever move zero — which is why every console read $0.00 and would have
// however much work was done.
//
// Both cross the boundary to the payment rail, and both are written so the
// ledger is only touched after the rail has confirmed. Crediting on intent is
// how an exchange funds a buyer whose payment later fails.

// handleTopupIntent starts a deposit.
//
// It returns what the caller needs to pay and records nothing: the balance
// moves when the rail says the money arrived, which for a card is seconds and
// for a bank transfer is days.
func (s *Server) handleTopupIntent(w http.ResponseWriter, r *http.Request, key *account.Key, person string, body []byte) {
	var in struct {
		AmountMinor int64  `json:"amount_minor"`
		Currency    string `json:"currency,omitempty"`
	}
	if err := json.Unmarshal(body, &in); err != nil || in.AmountMinor <= 0 {
		writeError(w, http.StatusBadRequest, "name an amount to add")
		return
	}
	if in.Currency == "" {
		in.Currency = "USD"
	}
	if s.Deposit == nil {
		// Said plainly rather than returning a dead payment link. An exchange
		// with no rail configured cannot take money, and pretending otherwise
		// would strand somebody mid-payment.
		writeError(w, http.StatusServiceUnavailable,
			"this exchange cannot take deposits yet")
		return
	}
	ref, url, err := s.Deposit(r.Context(), person, in.AmountMinor, in.Currency)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSONResponse(w, map[string]any{
		"reference": ref, "pay_at": url,
		"amount_minor": in.AmountMinor, "currency": in.Currency,
		"note": "your balance moves once the payment clears, not when you start it",
	})
}

// CreditDeposit moves a confirmed deposit into a balance.
//
// Called from the rail's webhook, never from a request the payer controls, and
// keyed on the rail's own event id so a redelivered webhook credits once.
func (s *Server) CreditDeposit(ctx context.Context, eventID, person string, amountMinor int64, currency, ref string) error {
	_, err := s.Ledger.Topup(ctx, "deposit:"+eventID, person, amountMinor, currency, ref)
	return err
}

// handleWithdraw asks for accumulated earnings to be sent out.
//
// It does not send them. A payout is made by the reconciler once the amount
// clears the threshold that makes a transfer worth its fee, and once the
// dispute window on the work behind it has closed — which is the reason the
// delay exists at all, since a refund cannot claw back money already gone.
func (s *Server) handleWithdraw(w http.ResponseWriter, r *http.Request, key *account.Key, person string, _ []byte) {
	if s.Ledger == nil {
		writeError(w, http.StatusServiceUnavailable, "no ledger")
		return
	}
	owed, err := s.Ledger.Balance(r.Context(), ledger.PayableOf(person), "USD")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not read your balance")
		return
	}
	out := map[string]any{
		"owed_minor": owed, "currency": "USD",
		"threshold_minor": PayoutThresholdMinor,
	}
	switch {
	case owed <= 0:
		out["status"] = "nothing owed yet"
	case owed < PayoutThresholdMinor:
		out["status"] = "waiting to reach the payout threshold"
		out["why"] = "a transfer costs a fixed fee, so small amounts are held until they are worth sending"
	case func() bool { _, ok := s.usdcRoute(person); return ok }():
		out["status"] = "queued for a person to send in USDC on the next run"
	case s.Rail == nil:
		// Not the worker's fault and not the worker's fix. Saying "no payout
		// account connected" here sent people to look for a setting that does
		// not exist on an exchange with no payment rail at all.
		out["status"] = "payouts are not switched on for this exchange"
		out["why"] = "your earnings are recorded and will be paid once they are"
	case !s.payoutAccountFor(person).Connected:
		out["status"] = "no payout account connected"
		out["connect_at"] = "/console"
	default:
		out["status"] = "queued for the next payout run"
	}
	writeJSONResponse(w, out)
}

// PayoutThresholdMinor is what must accumulate before a transfer is made.
//
// Measured against the payment rail: a card movement costs roughly 2.9% plus a
// fixed 30c, so paying out $1.50 the moment it is earned spends a fifth of it
// on the transfer.
const PayoutThresholdMinor = 2000
