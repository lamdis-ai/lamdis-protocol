package ledger

import (
	"context"
	"fmt"
	"strings"
)

// The movements the exchange actually makes. Callers use these rather than
// assembling postings, so the shape of each movement is stated once and the
// double-entry structure cannot be got wrong at a call site.
//
// Two of them cross the boundary to the outside world — Topup and Payout — and
// those are the only two that ever cost a card or bank fee. Everything between
// them is internal and free, which is the whole reason balances exist.

// Topup credits a buyer after money has arrived from outside.
//
// It is called after the rail confirms, never before: crediting on intent is
// how an exchange funds a buyer whose payment later fails.
func (l *Ledger) Topup(ctx context.Context, key, principal string, amountMinor int64, currency, ref string) (Receipt, error) {
	if amountMinor <= 0 {
		return Receipt{}, fmt.Errorf("ledger: a top-up must be positive, got %d", amountMinor)
	}
	r, err := l.Post(ctx, key, "topup", principal, currency, []Posting{
		{Account: AccountExternal, AmountMinor: -amountMinor},
		{Account: BalanceOf(principal), AmountMinor: amountMinor},
	})
	if err == nil && ref != "" {
		_, _ = l.db.ExecContext(ctx, `UPDATE ledger_ops SET ref = ? WHERE key = ?`, ref, key)
	}
	return r, err
}

// Hold commits a buyer's funds to one outcome.
//
// The amount is MaxPayout(terms), not the expected payout: a contingent payout
// that cannot reach its own maximum is not funded, it is a hope.
func (l *Ledger) Hold(ctx context.Context, key, outcome, buyer string, amountMinor int64, currency string) (Receipt, error) {
	if amountMinor <= 0 {
		return Receipt{}, fmt.Errorf("ledger: a hold must be positive, got %d", amountMinor)
	}
	return l.Post(ctx, key, "hold", outcome, currency, []Posting{
		{Account: BalanceOf(buyer), AmountMinor: -amountMinor},
		{Account: EscrowOf(outcome), AmountMinor: amountMinor},
	})
}

// Capture settles the earned part of an outcome out of its escrow.
//
// grossMinor is what the terms evaluated to and feeMinor is the exchange's cut
// of it. The provider is credited to a payable account rather than paid: a
// $1.47 transfer costs more to make than it is worth, so earnings accumulate
// and leave in one payout.
func (l *Ledger) Capture(ctx context.Context, key, outcome, provider string, grossMinor, feeMinor int64, currency string) (Receipt, error) {
	if grossMinor < 0 || feeMinor < 0 {
		return Receipt{}, fmt.Errorf("ledger: capture amounts must be non-negative")
	}
	if feeMinor > grossMinor {
		return Receipt{}, fmt.Errorf("ledger: fee %d exceeds the captured %d", feeMinor, grossMinor)
	}
	if grossMinor == 0 {
		return Receipt{}, fmt.Errorf("ledger: nothing to capture")
	}
	postings := []Posting{
		{Account: EscrowOf(outcome), AmountMinor: -grossMinor},
		{Account: PayableOf(provider), AmountMinor: grossMinor - feeMinor},
	}
	if feeMinor > 0 {
		postings = append(postings, Posting{Account: AccountFees, AmountMinor: feeMinor})
	}
	return l.Post(ctx, key, "capture", outcome, currency, postings)
}

// Release returns the unearned remainder of an escrow to the buyer.
//
// This is the movement the product exists for. An exchange that only ever pays
// out is a payment processor; returning money when the thing was not true is
// what makes the verdict worth anything.
func (l *Ledger) Release(ctx context.Context, key, outcome, buyer string, amountMinor int64, currency string) (Receipt, error) {
	if amountMinor <= 0 {
		return Receipt{}, fmt.Errorf("ledger: a release must be positive, got %d", amountMinor)
	}
	return l.Post(ctx, key, "release", outcome, currency, []Posting{
		{Account: EscrowOf(outcome), AmountMinor: -amountMinor},
		{Account: BalanceOf(buyer), AmountMinor: amountMinor},
	})
}

// Payout sends accumulated earnings out to the rail.
func (l *Ledger) Payout(ctx context.Context, key, provider string, amountMinor int64, currency, ref string) (Receipt, error) {
	if amountMinor <= 0 {
		return Receipt{}, fmt.Errorf("ledger: a payout must be positive, got %d", amountMinor)
	}
	r, err := l.Post(ctx, key, "payout", provider, currency, []Posting{
		{Account: PayableOf(provider), AmountMinor: -amountMinor},
		{Account: AccountExternal, AmountMinor: amountMinor},
	})
	if err == nil && ref != "" {
		_, _ = l.db.ExecContext(ctx, `UPDATE ledger_ops SET ref = ? WHERE key = ?`, ref, key)
	}
	return r, err
}

// Withdraw returns a buyer's unspent balance to them.
func (l *Ledger) Withdraw(ctx context.Context, key, principal string, amountMinor int64, currency, ref string) (Receipt, error) {
	if amountMinor <= 0 {
		return Receipt{}, fmt.Errorf("ledger: a withdrawal must be positive, got %d", amountMinor)
	}
	r, err := l.Post(ctx, key, "withdraw", principal, currency, []Posting{
		{Account: BalanceOf(principal), AmountMinor: -amountMinor},
		{Account: AccountExternal, AmountMinor: amountMinor},
	})
	if err == nil && ref != "" {
		_, _ = l.db.ExecContext(ctx, `UPDATE ledger_ops SET ref = ? WHERE key = ?`, ref, key)
	}
	return r, err
}

// Due lists providers whose accumulated earnings justify a payout.
//
// The threshold exists because a payout has a fixed cost. Below it, paying
// costs more than it moves; the earnings are not lost, they wait.
func (l *Ledger) Due(ctx context.Context, currency string, thresholdMinor int64) (map[string]int64, error) {
	rows, err := l.db.QueryContext(ctx,
		`SELECT account, SUM(amount) AS bal FROM ledger_postings
		 WHERE currency = ? AND account LIKE ?
		 GROUP BY account HAVING bal >= ? ORDER BY bal DESC`,
		strings.ToUpper(currency), prefixPayable+"%", thresholdMinor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var a string
		var v int64
		if err := rows.Scan(&a, &v); err != nil {
			return nil, err
		}
		out[strings.TrimPrefix(a, prefixPayable)] = v
	}
	return out, rows.Err()
}

// Held reports what is still committed to an outcome.
func (l *Ledger) Held(ctx context.Context, outcome, currency string) (int64, error) {
	return l.Balance(ctx, EscrowOf(outcome), currency)
}

// SetRef replaces the outside reference on an operation that has already
// been posted: a queued payout that a person later sends, for instance, whose
// on-chain hash did not exist when the ledger entry was made.
func (l *Ledger) SetRef(ctx context.Context, key, ref string) error {
	if key == "" || ref == "" {
		return fmt.Errorf("ledger: a reference needs a key and a value")
	}
	res, err := l.db.ExecContext(ctx, `UPDATE ledger_ops SET ref = ? WHERE key = ?`, ref, key)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("ledger: no operation under %s", key)
	}
	return nil
}

// Ref reads the outside reference recorded on an operation.
func (l *Ledger) Ref(ctx context.Context, key string) (string, error) {
	var ref string
	err := l.db.QueryRowContext(ctx, `SELECT ref FROM ledger_ops WHERE key = ?`, key).Scan(&ref)
	return ref, err
}
