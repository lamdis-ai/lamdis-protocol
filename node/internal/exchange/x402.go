package exchange

// Paying inline with x402.
//
// The stablecoin rail in chain_funding.go lets an agent fund a job by sending
// USDC to an address and waiting for the watcher to match the amount. That is
// a wallet, a transfer, and twelve confirmations: minutes, and a human-shaped
// loop for something that has no human in it. x402 is the HTTP-402 protocol
// for paying for a request inline: the server answers 402 with what it wants,
// the client retries with an X-PAYMENT header carrying a signed USDC
// authorisation (EIP-3009 transferWithAuthorization), a facilitator verifies
// the signature and submits the transfer, and the server fulfils. One round
// trip, no card, no human, no waiting on the chain.
//
// Opt-in per request: POST /v1/tasks with no credential and "x402": true in
// the body. The pay-link path is unchanged for everybody else. The exchange
// still holds no key and signs nothing; the facilitator does the chain work,
// and the settle transaction hash is recorded exactly as a watched transfer
// would be. The amount asked for is the same exact figure pay_usdc quotes —
// the job's ceiling at par plus its dust — so the chain watcher is a second
// pair of eyes: a settlement whose reply was lost still lists the job when
// the transfer confirms, and FundFromChain is idempotent on the hash.
//
// Verified against the x402 v1 specification (coinbase/x402, specs/
// x402-specification-v1.md and transports-v1/http.md): the 402 body is
// {x402Version, error, accepts: [PaymentRequirements]}; X-PAYMENT is base64
// JSON {x402Version, scheme, network, payload}; the facilitator takes
// {x402Version, paymentPayload, paymentRequirements} on /verify (answering
// {isValid, invalidReason, payer}) and /settle (answering {success,
// errorReason, transaction, network, payer}); X-PAYMENT-RESPONSE is the
// base64 JSON settle response.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/chain"
)

const (
	// x402Version is the protocol version this exchange speaks. v1 names
	// networks "base" and "base-sepolia", which is what chain.Network uses.
	x402Version = 1
	// x402Scheme is the only scheme here: an exact amount, transferred once.
	x402Scheme = "exact"
	// x402Timeout is how long a signed authorisation stays valid: the client
	// sets validBefore from it, and settlement has to land inside it.
	x402Timeout = 300
	// x402FundingKind is what a listing funded this way says in Funding.Kind.
	x402FundingKind = "x402"
)

// X402 is the facilitator the exchange asks to verify and settle. Nil on the
// server means inline payment is off.
type X402 struct {
	// Facilitator is the base URL; /verify and /settle are appended.
	Facilitator string
	// Auth, when set, is sent as the Authorization header on every
	// facilitator call. The public x402.org facilitator wants none; a
	// production one does.
	Auth string
	HTTP *http.Client

	mu sync.Mutex
	// settled remembers each payment nonce that reached a terminal outcome,
	// so a replayed request returns the same answer and never settles twice.
	settled map[string]*x402Outcome
	// inflight is every nonce currently between verify and settle.
	inflight map[string]bool
}

// x402Outcome is what a replayed payment gets back.
type x402Outcome struct {
	Job    string
	Status int
	Body   map[string]any
	Header string
}

// NewX402FromEnv switches inline payment on when LAMDIS_X402_FACILITATOR is
// set; the caller also requires the USDC rail, since that is where the money
// goes. LAMDIS_X402_FACILITATOR_AUTH is an optional Authorization value.
func NewX402FromEnv() (*X402, error) {
	u := strings.TrimSpace(os.Getenv("LAMDIS_X402_FACILITATOR"))
	if u == "" {
		return nil, nil
	}
	if !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
		return nil, fmt.Errorf("x402: LAMDIS_X402_FACILITATOR must be an http(s) URL, got %q", u)
	}
	x := NewX402(u)
	x.Auth = strings.TrimSpace(os.Getenv("LAMDIS_X402_FACILITATOR_AUTH"))
	return x, nil
}

// NewX402 builds the client for one facilitator.
func NewX402(facilitator string) *X402 {
	return &X402{
		Facilitator: strings.TrimSuffix(facilitator, "/"),
		HTTP:        &http.Client{Timeout: 60 * time.Second},
		settled:     map[string]*x402Outcome{},
		inflight:    map[string]bool{},
	}
}

// x402On is whether a job can be paid for inline: the rail to receive on and
// a facilitator to settle through.
func (s *Server) x402On() bool { return s.USDC != nil && s.X402 != nil }

// usdcDomainName is the EIP-712 domain name the USDC contract signs under,
// which the client needs to produce a signature the facilitator can recover.
// Circle's mainnet contract is named "USD Coin"; the Base Sepolia one "USDC".
func usdcDomainName(net chain.Network) string {
	if net.ChainID == chain.BaseSepolia.ChainID {
		return "USDC"
	}
	return "USD Coin"
}

// x402Advert is the line in a reply that says inline payment is possible.
func (s *Server) x402Advert() map[string]any {
	if !s.x402On() {
		return nil
	}
	return map[string]any{
		"supported": true,
		"version":   x402Version,
		"scheme":    x402Scheme,
		"network":   s.USDC.Net.Name,
		"chain_id":  s.USDC.Net.ChainID,
		"asset":     s.USDC.Net.USDC,
		"note": "Post again with \"x402\": true and no credential to get HTTP 402 " +
			"with the payment requirements; retry with a signed X-PAYMENT header " +
			"and the job lists in the same round trip. No card, no human, no " +
			"waiting on confirmations.",
	}
}

// x402Requirements is the one entry in `accepts` for a parked job.
func (s *Server) x402Requirements(p *pendingJob) map[string]any {
	l := p.L
	return map[string]any{
		"scheme":            x402Scheme,
		"network":           s.USDC.Net.Name,
		"maxAmountRequired": strconv.FormatInt(chain.UnitsFor(l.Job, p.Amount), 10),
		"resource":          strings.TrimSuffix(s.BaseURL, "/") + "/v1/tasks",
		"description":       "Lamdis exchange job " + l.Job + ": " + l.Title,
		"mimeType":          "application/json",
		"payTo":             s.USDC.Address(),
		"maxTimeoutSeconds": x402Timeout,
		"asset":             s.USDC.Net.USDC,
		"extra": map[string]any{
			"name":    usdcDomainName(s.USDC.Net),
			"version": "2",
		},
	}
}

// parkX402 holds a job until its payment arrives. The card path's
// stagePending opens a checkout session; this parks the same pendingJob with
// no session, because the thing that pays is the next request.
func (s *Server) parkX402(l *api.Listing) *pendingJob {
	p := &pendingJob{L: l, Amount: MaxPayoutFor(l), Created: s.now(),
		PayAt: strings.TrimSuffix(s.BaseURL, "/") + "/pay/" + l.Job}
	s.mu.Lock()
	if s.pending == nil {
		s.pending = map[string]*pendingJob{}
	}
	s.pending[l.Job] = p
	s.mu.Unlock()
	return p
}

// x402Required writes the 402. The body is what the specification asks for
// plus the same fields the pay-link reply carries, so the token and status
// URL come back with the price rather than after it.
func (s *Server) x402Required(w http.ResponseWriter, p *pendingJob, reason string) {
	l := p.L
	base := strings.TrimSuffix(s.BaseURL, "/")
	token := s.BuyerToken(l.Job)
	out := map[string]any{
		"x402Version":  x402Version,
		"error":        reason,
		"accepts":      []map[string]any{s.x402Requirements(p)},
		"job":          l.Job,
		"kind":         l.Kind,
		"status":       "awaiting_payment",
		"token":        token,
		"amount_minor": p.Amount,
		"currency":     l.Currency,
		"expires_at":   p.Created.Add(PendingTTL).Format(time.RFC3339),
		"status_url":   base + "/v1/jobs/" + l.Job,
		"watch":        base + "/my/" + l.Job + "?t=" + token,
		"note": "Sign an EIP-3009 authorisation for exactly maxAmountRequired of " +
			"asset to payTo and retry this same request with it base64-encoded " +
			"in X-PAYMENT. The facilitator settles it and the job lists in the " +
			"reply; X-PAYMENT-RESPONSE carries the transaction. Whatever the job " +
			"does not pay out on proof is owed back to the paying address. Keep " +
			"the token: it is how you follow this job.",
	}
	if q := s.usdcQuote(l.Job, p.Amount); q != nil {
		out["pay_usdc"] = q
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusPaymentRequired)
	json.NewEncoder(w).Encode(out)
}

// x402Payment is the decoded X-PAYMENT header.
type x402Payment struct {
	X402Version int             `json:"x402Version"`
	Scheme      string          `json:"scheme"`
	Network     string          `json:"network"`
	Payload     json.RawMessage `json:"payload"`
}

// x402Authorization is the exact-scheme EVM payload the facilitator checks.
type x402Authorization struct {
	Signature     string `json:"signature"`
	Authorization struct {
		From        string `json:"from"`
		To          string `json:"to"`
		Value       string `json:"value"`
		ValidAfter  string `json:"validAfter"`
		ValidBefore string `json:"validBefore"`
		Nonce       string `json:"nonce"`
	} `json:"authorization"`
}

// decodeX402Payment reads the header. Clients encode with standard base64;
// the unpadded form is accepted too because some do.
func decodeX402Payment(h string) (raw map[string]any, p x402Payment, a x402Authorization, err error) {
	h = strings.TrimSpace(h)
	b, err := base64.StdEncoding.DecodeString(h)
	if err != nil {
		if b, err = base64.RawStdEncoding.DecodeString(h); err != nil {
			return nil, p, a, fmt.Errorf("X-PAYMENT is not base64")
		}
	}
	if err = json.Unmarshal(b, &raw); err != nil {
		return nil, p, a, fmt.Errorf("X-PAYMENT is not JSON")
	}
	if err = json.Unmarshal(b, &p); err != nil {
		return nil, p, a, fmt.Errorf("X-PAYMENT is not a payment payload")
	}
	if len(p.Payload) > 0 {
		if err = json.Unmarshal(p.Payload, &a); err != nil {
			return nil, p, a, fmt.Errorf("X-PAYMENT payload is not an exact-scheme authorisation")
		}
	}
	if a.Authorization.Nonce == "" || a.Signature == "" {
		return nil, p, a, fmt.Errorf("X-PAYMENT payload needs a signature and an authorization with a nonce")
	}
	return raw, p, a, nil
}

// handleX402 is the guest branch of POST /v1/tasks when the body asks for
// inline payment. With no X-PAYMENT the job is parked and priced; with one,
// the payment is matched to the job it was signed for, verified, settled,
// and the job listed.
func (s *Server) handleX402(w http.ResponseWriter, r *http.Request, l *api.Listing) {
	header := r.Header.Get("X-PAYMENT")
	if header == "" {
		s.x402Required(w, s.parkX402(l), "Payment required to list this job")
		return
	}
	raw, pay, auth, err := decodeX402Payment(header)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	nonce := strings.ToLower(strings.TrimSpace(auth.Authorization.Nonce))

	// A replay gets the answer it got the first time, and nothing else runs.
	x := s.X402
	x.mu.Lock()
	if done, ok := x.settled[nonce]; ok {
		x.mu.Unlock()
		s.writeX402Outcome(w, done)
		return
	}
	if x.inflight[nonce] {
		x.mu.Unlock()
		writeError(w, http.StatusConflict, "that payment is being settled now; ask for the job's status")
		return
	}
	x.inflight[nonce] = true
	x.mu.Unlock()
	defer func() {
		x.mu.Lock()
		delete(x.inflight, nonce)
		x.mu.Unlock()
	}()

	// The payment names its job by its amount, exactly as a transfer does.
	units, perr := strconv.ParseInt(auth.Authorization.Value, 10, 64)
	job, ok := "", false
	if perr == nil {
		job, ok = s.pendingByUnits(units)
	}
	if !ok {
		// Nothing is waiting for that amount: it was never quoted, it was
		// paid already, or it waited longer than a day. This request is a
		// fresh one, and rate-limited as one.
		if err := s.guestAllowed(r); err != nil {
			writeError(w, http.StatusTooManyRequests, err.Error())
			return
		}
		s.x402Required(w, s.parkX402(l),
			"no unpaid job is waiting for that amount; this is a new one, priced below")
		return
	}
	p, _ := s.pendingFor(job)
	req := s.x402Requirements(p)
	if pay.X402Version != x402Version || pay.Scheme != x402Scheme ||
		!strings.EqualFold(pay.Network, s.USDC.Net.Name) ||
		!strings.EqualFold(auth.Authorization.To, s.USDC.Address()) {
		s.x402Required(w, p, "the payment does not match these requirements: "+
			"scheme, network and payTo must be exactly what accepts says")
		return
	}

	ctx := r.Context()
	verdict, err := x.call(ctx, "/verify", raw, req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "the facilitator could not be reached: "+err.Error())
		return
	}
	if valid, _ := verdict["isValid"].(bool); !valid {
		reason, _ := verdict["invalidReason"].(string)
		if reason == "" {
			reason = "the facilitator rejected the payment"
		}
		s.x402Required(w, p, reason)
		return
	}
	settled, err := x.call(ctx, "/settle", raw, req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "the facilitator could not be reached to settle: "+err.Error())
		return
	}
	tx, _ := settled["transaction"].(string)
	tx = strings.ToLower(strings.TrimSpace(tx))
	payer, _ := settled["payer"].(string)
	if payer == "" {
		payer = auth.Authorization.From
	}
	if success, _ := settled["success"].(bool); !success {
		reason, _ := settled["errorReason"].(string)
		if reason == "settlement_pending" && tx != "" {
			// Submitted, not yet final. The job stays parked; the watcher
			// lists it when the transfer confirms, because the amount is
			// this job's own. Nothing is said to be listed that is not.
			writeJSONResponse(w, map[string]any{
				"job": job, "status": "awaiting_payment", "settlement": "pending",
				"transaction": tx, "token": s.BuyerToken(job),
				"status_url": strings.TrimSuffix(s.BaseURL, "/") + "/v1/jobs/" + job,
				"error": "the facilitator submitted the payment but has not confirmed it; " +
					"the job is not listed yet and will be once the transfer confirms",
			})
			return
		}
		if reason == "" {
			reason = "settlement failed"
		}
		s.x402Required(w, p, "not listed: "+reason)
		return
	}
	if tx == "" {
		s.x402Required(w, p, "not listed: the facilitator reported success without a transaction")
		return
	}
	if err := s.fundFromChain(ctx, job, tx, chain.MinorOf(units), payer, x402FundingKind); err != nil {
		// Paid and not listed is the one state that must never be quiet.
		log.Printf("x402       %s settled as %s but could not be listed: %v", job, tx, err)
		writeJSONResponse(w, map[string]any{
			"job": job, "status": "awaiting_payment", "transaction": tx,
			"error": "the payment settled but the job could not be listed: " + err.Error(),
		})
		return
	}
	log.Printf("x402       %s funded by %s (%s USDC from %s)", job, tx, chain.Format(units), payer)

	resp := map[string]any{
		"success": true, "transaction": tx, "network": s.USDC.Net.Name, "payer": payer,
	}
	respJSON, _ := json.Marshal(resp)
	base := strings.TrimSuffix(s.BaseURL, "/")
	token := s.BuyerToken(job)
	out := &x402Outcome{
		Job: job, Status: http.StatusOK,
		Header: base64.StdEncoding.EncodeToString(respJSON),
		Body: map[string]any{
			"job": job, "kind": p.L.Kind, "status": "listed", "token": token,
			"amount_minor": p.Amount, "currency": p.L.Currency,
			"funding": map[string]any{"kind": x402FundingKind, "transaction": tx, "payer": payer,
				"network": s.USDC.Net.Name},
			"payment_response": resp,
			"status_url":       base + "/v1/jobs/" + job,
			"watch":            base + "/my/" + job + "?t=" + token,
			"note": "Paid and on the board. Nothing more is taken: what the job does " +
				"not pay out on proof is owed back to the paying address. Keep the " +
				"token; it is how you follow this job.",
		},
	}
	x.mu.Lock()
	x.settled[nonce] = out
	x.mu.Unlock()
	s.writeX402Outcome(w, out)
}

func (s *Server) writeX402Outcome(w http.ResponseWriter, o *x402Outcome) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-PAYMENT-RESPONSE", o.Header)
	w.WriteHeader(o.Status)
	json.NewEncoder(w).Encode(o.Body)
}

// call posts one facilitator request and decodes the answer.
func (x *X402) call(ctx context.Context, path string, payload map[string]any, req map[string]any) (map[string]any, error) {
	body, err := json.Marshal(map[string]any{
		"x402Version":         x402Version,
		"paymentPayload":      payload,
		"paymentRequirements": req,
	})
	if err != nil {
		return nil, err
	}
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, x.Facilitator+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	hr.Header.Set("Content-Type", "application/json")
	if x.Auth != "" {
		hr.Header.Set("Authorization", x.Auth)
	}
	resp, err := x.HTTP.Do(hr)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("%s answered %d with something unreadable", path, resp.StatusCode)
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("%s answered %d", path, resp.StatusCode)
	}
	return out, nil
}
