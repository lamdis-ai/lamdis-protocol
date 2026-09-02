// Package exchange drives the outcome lifecycle: quoting, escrow, routing,
// verification, and settlement.
//
// This is the only impure component in the design. Everything it orchestrates
// — the terms evaluator, the fold, the aggregation — is a pure function that
// has already been proven on its own, so what is left here is sequencing and
// the two places we talk to the outside world: the payment rail and the
// verifier.
package exchange

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"strings"
	"time"

	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/outcome"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/payment"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/verify"
)

// Order is what a buying agent asks for.
type Order struct {
	Predicate string
	Category  string
	Currency  string

	RequiredConfidenceBP int64
	RequiredTier         string

	BaseFeeMinor      int64
	SuccessBonusMinor int64
	FeeBP             int64

	EvidenceDeadline     time.Duration
	VerificationDeadline time.Duration
	DisputeWindow        time.Duration
	ArbitrationDeadline  time.Duration
}

// Defaults fills in the parts of an order a caller did not specify.
func (o Order) Defaults() Order {
	if o.Currency == "" {
		o.Currency = "USD"
	}
	if o.RequiredConfidenceBP == 0 {
		o.RequiredConfidenceBP = 9000
	}
	if o.RequiredTier == "" {
		o.RequiredTier = string(verify.TierV2)
	}
	if o.BaseFeeMinor == 0 {
		o.BaseFeeMinor = 500
	}
	if o.SuccessBonusMinor == 0 {
		o.SuccessBonusMinor = 1800
	}
	if o.FeeBP == 0 {
		// The fee the terms quote is the fee settlement charges. Two numbers
		// named FeeBP — this one defaulting to 500 while settle.go charged 0 —
		// meant every quote overstated the cut by five percent.
		o.FeeBP = FeeBP
	}
	if o.EvidenceDeadline == 0 {
		o.EvidenceDeadline = 4 * time.Hour
	}
	if o.VerificationDeadline == 0 {
		o.VerificationDeadline = 30 * time.Minute
	}
	if o.DisputeWindow == 0 {
		o.DisputeWindow = 2 * time.Hour
	}
	if o.ArbitrationDeadline == 0 {
		o.ArbitrationDeadline = 24 * time.Hour
	}
	return o
}

// Step is one narrated thing the exchange did, for the CLI and the dashboard.
type Step struct {
	Act    string
	Detail string
	Entry  string
}

// Run is one outcome from request to settlement.
type Run struct {
	Log      *protolog.ThreadLog
	State    *outcome.OutcomeState
	Steps    []Step
	Nonce    string
	Verify   verify.Result
	Provider string
	// TruthWas is what was actually true at the address. Only the simulation
	// knows this; it is what lets the demo say whether the exchange got it
	// right, which a real deployment can never do.
	TruthWas bool
}

// Delivery is what a provider handed over, and what verification made of it.
// It is the seam between the money lifecycle and the perception layer: the
// exchange does not care whether the evidence came from a simulated provider
// or a phone, only what the verifier concluded about it.
type Delivery struct {
	Evidence    verify.Evidence
	Result      verify.Result
	SubmittedAt time.Time
	// Detail is a human-readable note for the narrated trail.
	Detail string
}

// Executor performs the work and returns evidence with a verdict. Both the
// simulated providers and the real-photo path implement it.
type Executor interface {
	Name() string
	// Bid is what it charges and how long it expects to take.
	Bid(job string) (priceMinor int64, eta time.Duration)
	// Execute does the work. nonce is the challenge code the provider was told
	// to include in the shot; required is the tier the buyer paid for.
	Execute(ctx context.Context, job, nonce string, required verify.Tier,
		predicate string, start time.Time, deadline time.Duration) (Delivery, error)
}

// Exchange is the market operator. In v0 it holds every service role, which
// the terms disclose explicitly rather than hide.
type Exchange struct {
	Key    ed25519.PrivateKey
	PID    string
	Rail   payment.Adapter
	Corpus *verify.Corpus
	Params verify.Params

	now time.Time
}

// New builds an exchange from a seed key and a rail.
func New(key ed25519.PrivateKey, rail payment.Adapter, start time.Time) (*Exchange, error) {
	pid, err := protolog.PrincipalID(key.Public().(ed25519.PublicKey))
	if err != nil {
		return nil, err
	}
	return &Exchange{
		Key: key, PID: pid, Rail: rail,
		Corpus: verify.NewCorpus(), Params: verify.DefaultParams(),
		now: start,
	}, nil
}

func (x *Exchange) Now() time.Time          { return x.now }
func (x *Exchange) Advance(d time.Duration) { x.now = x.now.Add(d) }

// Purchase runs one outcome end to end against a simulated provider.
//
// truth is what is actually the case at the address. The exchange never sees
// it — it only sees what the provider submits — which is exactly the position
// a real exchange is in.
func (x *Exchange) Purchase(
	ctx context.Context,
	buyerKey, providerKey ed25519.PrivateKey,
	p Executor,
	o Order,
	truth bool,
) (*Run, error) {
	o = o.Defaults()

	log, _, err := protolog.NewThreadWith(x.Key, o.Predicate, false, x.Now)
	if err != nil {
		return nil, fmt.Errorf("create outcome thread: %w", err)
	}
	run := &Run{Log: log, TruthWas: truth}

	buyer, err := x.authorFor(log, buyerKey)
	if err != nil {
		return nil, err
	}
	provider, err := x.authorFor(log, providerKey)
	if err != nil {
		return nil, err
	}
	exch, err := x.authorFor(log, x.Key)
	if err != nil {
		return nil, err
	}
	run.Provider = provider.Principal()
	run.Nonce = challengeNonce(log.Thread)

	post := func(a *protolog.Author, kind string, body any) (*protolog.Entry, error) {
		x.Advance(time.Second)
		lane, _ := outcome.Lane(kind)
		return a.Append(protolog.Draft{Kind: kind, Lane: lane, Body: body})
	}
	note := func(act, detail string, e *protolog.Entry) {
		s := Step{Act: act, Detail: detail}
		if e != nil {
			s.Entry = e.ID
		}
		run.Steps = append(run.Steps, s)
	}

	// --- the ask ----------------------------------------------------------
	req, err := post(buyer, outcome.KindRequest, outcome.RequestBody{
		Text: o.Predicate, Category: o.Category, Currency: o.Currency,
		RequiredTier: o.RequiredTier, RequiredConfidence: o.RequiredConfidenceBP,
	})
	if err != nil {
		return nil, err
	}
	note("request", o.Predicate, req)

	// --- the bid ----------------------------------------------------------
	price, eta := p.Bid(log.Thread)
	bid, err := post(provider, outcome.KindBid, outcome.BidBody{
		PriceMinor: price, ETASeconds: int64(eta / time.Second),
	})
	if err != nil {
		return nil, err
	}
	note("bid", fmt.Sprintf("%s bids %s, eta %s", p.Name(), money(price, o.Currency), eta.Round(time.Minute)), bid)

	// --- the quote --------------------------------------------------------
	terms, err := outcome.ObservationSpec{
		Currency: o.Currency, Scale: 2,
		Verifier: x.PID, Arbiter: x.PID, EscrowAgent: x.PID, Timekeeper: x.PID,
		PredicateDefinition:  o.Predicate,
		BaseFeeMinor:         o.BaseFeeMinor,
		SuccessBonusMinor:    o.SuccessBonusMinor,
		FeeBP:                o.FeeBP,
		EvidenceDeadline:     o.EvidenceDeadline,
		VerificationDeadline: o.VerificationDeadline,
		DisputeWindow:        o.DisputeWindow,
		ArbitrationDeadline:  o.ArbitrationDeadline,
	}.Build()
	if err != nil {
		return nil, fmt.Errorf("build terms: %w", err)
	}
	termsHash, err := terms.Hash()
	if err != nil {
		return nil, err
	}
	escrow, err := outcome.MaxPayout(terms)
	if err != nil {
		return nil, err
	}
	quote, err := post(exch, outcome.KindQuote, outcome.QuoteBody{
		Terms: terms, TermsHash: termsHash, EscrowMinor: escrow,
		ETASeconds: int64(eta / time.Second), Tier: o.RequiredTier,
		ConfidenceBP: o.RequiredConfidenceBP,
		ExpiresAt:    x.Now().Add(10 * time.Minute).Format(time.RFC3339),
	})
	if err != nil {
		return nil, err
	}
	note("quote", fmt.Sprintf("%s to hold, %s base + %s bonus, %d bp fee",
		money(escrow, o.Currency), money(o.BaseFeeMinor, o.Currency),
		money(o.SuccessBonusMinor, o.Currency), o.FeeBP), quote)

	// --- acceptance -------------------------------------------------------
	quoteHash, err := quote.Hash()
	if err != nil {
		return nil, err
	}
	acc, err := post(buyer, outcome.KindAccept, outcome.AcceptBody{
		QuoteEntry: quote.ID, QuoteHash: quoteHash, TermsHash: termsHash,
	})
	if err != nil {
		return nil, err
	}
	note("accept", "terms frozen by hash", acc)

	// --- escrow: intent, then rail, then attested receipt ------------------
	intent, err := post(buyer, outcome.KindEscrowIntent, outcome.EscrowIntentBody{
		AmountMinor: escrow, Currency: o.Currency, Rail: x.Rail.Rail(),
		IdempotencyKey: string(payment.DeriveKey("hold", quoteHash)), TermsHash: termsHash,
	})
	if err != nil {
		return nil, err
	}
	intentHash, err := intent.Hash()
	if err != nil {
		return nil, err
	}
	holdKey := payment.DeriveKey("hold", intentHash)
	holdRes, holdErr := x.Rail.Hold(ctx, payment.Request{
		Key: holdKey, Outcome: log.Thread, Instruction: intent.ID,
		AmountMinor: escrow, Currency: o.Currency,
		Source: buyer.Principal(),
	})
	state := outcome.EscrowHeld
	if holdErr != nil || holdRes.State != payment.StateSucceeded {
		state = outcome.EscrowFailed
	}
	rec, err := post(exch, outcome.KindEscrowReceipt, outcome.EscrowReceiptBody{
		IntentEntry: intent.ID, State: state, Rail: x.Rail.Rail(),
		RailRef: holdRes.Ref, ObservedAt: x.Now().Format(time.RFC3339),
	})
	if err != nil {
		return nil, err
	}
	note("escrow", fmt.Sprintf("%s %s (%s)", money(escrow, o.Currency), state, holdRes.Ref), rec)
	if state == outcome.EscrowFailed {
		run.State = outcome.Fold(log.Thread, log.Entries())
		return run, nil
	}

	// --- award ------------------------------------------------------------
	aw, err := post(exch, outcome.KindAward, outcome.AwardBody{
		BidEntry: bid.ID, Provider: provider.Principal(), TermsHash: termsHash,
	})
	if err != nil {
		return nil, err
	}
	note("award", fmt.Sprintf("%s, challenge code %s", p.Name(), run.Nonce), aw)

	// --- execution --------------------------------------------------------
	del, err := p.Execute(ctx, log.Thread, run.Nonce, verify.Tier(o.RequiredTier),
		o.Predicate, x.Now(), o.EvidenceDeadline)
	if err != nil {
		return nil, fmt.Errorf("execute: %w", err)
	}
	x.now = del.SubmittedAt
	ev, err := post(provider, outcome.KindEvidence, outcome.EvidenceBody{
		Kind: "image",
		Blob: &outcome.BlobRef{
			SHA256: del.Evidence.SHA256, MediaType: del.Evidence.MediaType, Bytes: del.Evidence.Bytes,
		},
		AttestedBy:  del.Evidence.AttestedBy,
		CollectedAt: del.SubmittedAt.Format(time.RFC3339),
		Capture:     captureOf(del.Evidence),
		Supports:    []string{outcome.MeasureSatisfied},
	})
	if err != nil {
		return nil, err
	}
	elapsed := "unknown"
	if awardedAt, perr := time.Parse(time.RFC3339, aw.TS); perr == nil {
		elapsed = del.SubmittedAt.Sub(awardedAt).Round(time.Minute).String()
	}
	detail := fmt.Sprintf("%s… submitted %s after award", del.Evidence.SHA256[:12], elapsed)
	if del.Detail != "" {
		detail = del.Detail + ", " + detail
	}
	note("evidence", detail, ev)

	// --- verification -----------------------------------------------------
	del.Evidence.EntryID = ev.ID
	res := del.Result
	run.Verify = res
	x.Corpus.Add(del.Evidence)

	verdictResult := outcome.VerdictFail
	satisfied := res.Satisfied(o.RequiredConfidenceBP)
	if satisfied {
		verdictResult = outcome.VerdictPass
	}
	vd, err := post(exch, outcome.KindVerdict, outcome.VerdictBody{
		Result:       verdictResult,
		Measures:     outcome.Observed(res.Admissible, satisfied),
		ConfidenceBP: res.ConfidenceBP(),
		Tier:         string(res.Tier),
		Method:       "deterministic+blind_describe_then_adjudicate",
		MethodDetail: signalSummary(res),
		Provenance:   map[string][]string{outcome.MeasureSatisfied: {ev.ID}},
		TermsHash:    termsHash, AggregateHash: res.AggregateHash,
	})
	if err != nil {
		return nil, err
	}
	note("verdict", res.Explain(o.RequiredConfidenceBP), vd)

	// --- the dispute window has to actually elapse ------------------------
	st := outcome.Fold(log.Thread, log.Entries())
	if d := st.Deadlines[outcome.DeadlineDispute]; d != nil && d.AnchorEntry != "" {
		x.Advance(o.DisputeWindow + time.Minute)
		to, err := post(exch, outcome.KindTimeout, outcome.TimeoutBody{
			DeadlineID: outcome.DeadlineDispute, AnchorEntry: d.AnchorEntry,
			FiredAt: x.Now().Format(time.RFC3339),
		})
		if err != nil {
			return nil, err
		}
		note("dispute window", "closed with no dispute", to)
	}

	// --- settlement -------------------------------------------------------
	st = outcome.Fold(log.Thread, log.Entries())
	measures, _, _ := st.Basis()
	want, err := outcome.Evaluate(terms, measures, escrow)
	if err != nil {
		return nil, fmt.Errorf("settlement arithmetic: %w", err)
	}
	inst, err := post(exch, outcome.KindSettleInstruction, outcome.SettleInstructionBody{
		BasisEntry: vd.ID, TermsHash: termsHash, EscrowMinor: escrow,
		Payouts:  []outcome.Payout{{Principal: provider.Principal(), AmountMinor: want.ProviderNet}},
		FeeMinor: want.FeeMinor, RefundMinor: want.RefundMinor,
		IdempotencyKey: string(payment.DeriveKey("settle", termsHash)),
	})
	if err != nil {
		return nil, err
	}
	note("settle", fmt.Sprintf("instruct: %s to provider, %s fee, %s refunded",
		money(want.ProviderNet, o.Currency), money(want.FeeMinor, o.Currency),
		money(want.RefundMinor, o.Currency)), inst)

	ops, err := x.executeSettlement(ctx, log.Thread, inst, want,
		buyer.Principal(), provider.Principal(), o.Currency)
	if err != nil {
		return nil, err
	}
	rcpt, err := post(exch, outcome.KindSettleReceipt, outcome.SettleReceiptBody{
		InstructionEntry: inst.ID, Ops: ops, ObservedAt: x.Now().Format(time.RFC3339),
	})
	if err != nil {
		return nil, err
	}
	note("receipt", "rail confirmed; money has actually moved", rcpt)

	run.State = outcome.Fold(log.Thread, log.Entries())
	return run, nil
}

// executeSettlement performs the rail operations the instruction calls for.
// The idempotency key of each is derived from the instruction entry's hash, so
// a retry after a crash — on this node or another — computes the same key and
// the rail deduplicates rather than paying twice.
func (x *Exchange) executeSettlement(
	ctx context.Context, thread string, inst *protolog.Entry,
	want outcome.Result, payer, payee, currency string,
) ([]outcome.SettleOp, error) {
	instHash, err := inst.Hash()
	if err != nil {
		return nil, err
	}
	var ops []outcome.SettleOp

	if want.GrossMinor > 0 {
		res, err := x.Rail.Capture(ctx, payment.Request{
			Key: payment.DeriveKey("capture", instHash), Outcome: thread, Instruction: inst.ID,
			AmountMinor: want.GrossMinor, Currency: currency,
			Source: payer, Destination: payee, FeeMinor: want.FeeMinor,
		})
		ops = append(ops, outcome.SettleOp{
			Op: "capture", AmountMinor: want.GrossMinor,
			State: stateOf(res, err), RailRef: res.Ref,
		})
	}
	if want.RefundMinor > 0 {
		res, err := x.Rail.Release(ctx, payment.Request{
			Key: payment.DeriveKey("release", instHash), Outcome: thread, Instruction: inst.ID,
			AmountMinor: want.RefundMinor, Currency: currency,
			Source: payer,
		})
		ops = append(ops, outcome.SettleOp{
			Op: "release", AmountMinor: want.RefundMinor,
			State: stateOf(res, err), RailRef: res.Ref,
		})
	}
	return ops, nil
}

func stateOf(r payment.Result, err error) string {
	if err != nil && r.State == "" {
		return payment.StateUnknown
	}
	if r.State == "" {
		return payment.StateUnknown
	}
	return r.State
}

func (x *Exchange) authorFor(l *protolog.ThreadLog, k ed25519.PrivateKey) (*protolog.Author, error) {
	a, err := protolog.NewAuthor(l, k)
	if err != nil {
		return nil, err
	}
	a.Now = x.Now
	return a, nil
}

func captureOf(e verify.Evidence) *outcome.Capture {
	if e.CapturedAt.IsZero() {
		return nil
	}
	return &outcome.Capture{CapturedAt: e.CapturedAt.Format(time.RFC3339)}
}

// challengeNonce derives the per-job code the provider must include in the
// shot. Deriving it from the thread id keeps demos reproducible; a real
// deployment draws it from crypto/rand.
func challengeNonce(thread string) string {
	sum := sha256.Sum256([]byte("lamdis-challenge:" + thread))
	enc := base32.NewEncoding("0123456789ABCDEFGHJKMNPQRSTVWXYZ").WithPadding(base32.NoPadding)
	return enc.EncodeToString(sum[:4])[:6]
}

func signalSummary(r verify.Result) string {
	var parts []string
	for _, s := range r.Signals {
		mark := ""
		if s.Fatal {
			mark = " (fatal)"
		}
		parts = append(parts, fmt.Sprintf("%s=%s%s", s.Feature, s.Value, mark))
	}
	return strings.Join(parts, ", ")
}

func money(minor int64, currency string) string {
	sign := ""
	if minor < 0 {
		sign, minor = "-", -minor
	}
	symbol := "$"
	if currency != "USD" {
		symbol = currency + " "
	}
	return fmt.Sprintf("%s%s%d.%02d", sign, symbol, minor/100, minor%100)
}

// Money is exported for the CLI's use.
func Money(minor int64, currency string) string { return money(minor, currency) }
