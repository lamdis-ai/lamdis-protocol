package exchange

// The stablecoin rail.
//
// An agent with a wallet has no card and no human to hand a pay link to; an
// operator in a country the card rail does not serve has no way to receive.
// This file gives both a path that touches no bank: a job can be funded by
// sending USDC to the exchange's address, and earnings can be routed to an
// address instead of a connected account.
//
// Watch-only, all of it. The exchange holds no private key and signs no
// transaction. Money in is observed on the chain and matched to a job by its
// exact amount. Money out is a queue that a person works through with their
// own wallet, on a schedule, and then marks as sent. The ledger records both
// sides the same way it records a card, so everything between — escrow,
// settlement, holdbacks, receipts — is untouched.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/chain"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/ledger"
)

// USDCLifetimeCapMinor is how much an operator can be paid to an address
// before the exchange asks them to connect a payout account. A wallet is not
// an identity; up to this much, the exchange accepts that and above it, it
// does not.
const USDCLifetimeCapMinor = 60000

// USDCRail is everything the stablecoin path needs. Nil on the server means
// the rail is off.
type USDCRail struct {
	Net       chain.Network
	Client    *chain.Client
	Watcher   *chain.Watcher
	Addresses *USDCAddresses
	Queue     *USDCQueue
}

// NewUSDCRailFromEnv switches the rail on when both LAMDIS_USDC_ADDRESS and
// LAMDIS_BASE_RPC are set, and leaves it off otherwise.
func NewUSDCRailFromEnv(dir string) (*USDCRail, error) {
	addr, rpc := os.Getenv("LAMDIS_USDC_ADDRESS"), os.Getenv("LAMDIS_BASE_RPC")
	if addr == "" || rpc == "" {
		return nil, nil
	}
	net, ok := chain.NetworkNamed(os.Getenv("LAMDIS_USDC_NETWORK"))
	if !ok {
		return nil, fmt.Errorf("usdc: LAMDIS_USDC_NETWORK %q is not base or base-sepolia",
			os.Getenv("LAMDIS_USDC_NETWORK"))
	}
	return NewUSDCRail(net, rpc, addr, dir)
}

// NewUSDCRail builds the rail against one network.
func NewUSDCRail(net chain.Network, rpc, addr, dir string) (*USDCRail, error) {
	c, err := chain.New(rpc, net.USDC, addr)
	if err != nil {
		return nil, err
	}
	return &USDCRail{
		Net: net, Client: c,
		Watcher:   chain.NewWatcher(c, dir, chain.Confirmations),
		Addresses: NewUSDCAddresses(dir),
		Queue:     NewUSDCQueue(dir),
	}, nil
}

// Address is where a payer sends.
func (u *USDCRail) Address() string { return u.Client.Recipient }

// USDCAddresses remembers where each operator wants to be paid.
type USDCAddresses struct {
	mu   sync.Mutex
	by   map[string]string
	path string
}

func NewUSDCAddresses(dir string) *USDCAddresses {
	a := &USDCAddresses{by: map[string]string{}}
	if dir != "" {
		a.path = filepath.Join(dir, "payout-usdc.json")
		if b, err := os.ReadFile(a.path); err == nil {
			_ = json.Unmarshal(b, &a.by)
		}
	}
	return a
}

func (a *USDCAddresses) Get(person string) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.by[person]
	return s, ok
}

// Put sets or clears a person's address. Unlike a connected account, an
// address is the person's own statement and they may change it; the route
// that calls this is authenticated as them.
func (a *USDCAddresses) Put(person, addr string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if addr == "" {
		delete(a.by, person)
	} else {
		a.by[person] = addr
	}
	return writeAtomic(a.path, a.by)
}

func (a *USDCAddresses) People() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]string, 0, len(a.by))
	for p := range a.by {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// USDCPayout is one send a person has to make.
type USDCPayout struct {
	ID          string    `json:"id"`
	Person      string    `json:"person"`
	Address     string    `json:"address"`
	AmountMinor int64     `json:"amount_minor"`
	Currency    string    `json:"currency"`
	Reason      string    `json:"reason"`
	LedgerKey   string    `json:"ledger_key,omitempty"`
	Created     time.Time `json:"created"`
	SentTx      string    `json:"sent_tx,omitempty"`
	SentAt      time.Time `json:"sent_at,omitempty"`
}

// USDCQueue is the file a person works through.
type USDCQueue struct {
	mu    sync.Mutex
	path  string
	state struct {
		Next  int           `json:"next"`
		Items []*USDCPayout `json:"items"`
	}
}

func NewUSDCQueue(dir string) *USDCQueue {
	q := &USDCQueue{}
	if dir != "" {
		q.path = filepath.Join(dir, "usdc-queue.json")
		if b, err := os.ReadFile(q.path); err == nil {
			_ = json.Unmarshal(b, &q.state)
		}
	}
	return q
}

// Add appends an item and assigns its id.
func (q *USDCQueue) Add(p USDCPayout) (*USDCPayout, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.state.Next++
	p.ID = fmt.Sprintf("q%d", q.state.Next)
	item := &p
	q.state.Items = append(q.state.Items, item)
	if err := writeAtomic(q.path, q.state); err != nil {
		q.state.Items = q.state.Items[:len(q.state.Items)-1]
		return nil, err
	}
	out := *item
	return &out, nil
}

// Remove drops an item that was never recorded anywhere else.
func (q *USDCQueue) Remove(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, it := range q.state.Items {
		if it.ID == id {
			q.state.Items = append(q.state.Items[:i], q.state.Items[i+1:]...)
			_ = writeAtomic(q.path, q.state)
			return
		}
	}
}

// Has reports whether an item exists for a reason, so a refund is queued once.
func (q *USDCQueue) Has(reason string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, it := range q.state.Items {
		if it.Reason == reason {
			return true
		}
	}
	return false
}

// List returns every item, unsent first.
func (q *USDCQueue) List() []USDCPayout {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]USDCPayout, 0, len(q.state.Items))
	for _, it := range q.state.Items {
		out = append(out, *it)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].SentTx == "" && out[j].SentTx != ""
	})
	return out
}

// Unsent counts what is still waiting on a person.
func (q *USDCQueue) Unsent() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := 0
	for _, it := range q.state.Items {
		if it.SentTx == "" {
			n++
		}
	}
	return n
}

// MarkSent records the on-chain reference for an item.
func (q *USDCQueue) MarkSent(id, tx string, now time.Time) (*USDCPayout, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, it := range q.state.Items {
		if it.ID != id {
			continue
		}
		if it.SentTx != "" && it.SentTx != tx {
			return nil, fmt.Errorf("%s was already sent as %s", id, it.SentTx)
		}
		it.SentTx, it.SentAt = tx, now
		if err := writeAtomic(q.path, q.state); err != nil {
			return nil, err
		}
		out := *it
		return &out, nil
	}
	return nil, fmt.Errorf("no queue item %s", id)
}

// PaidTo is everything queued as earnings for a person, sent or not: once it
// is queued it is theirs, and the cap counts it.
func (q *USDCQueue) PaidTo(person string) int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	var total int64
	for _, it := range q.state.Items {
		if it.Person == person && it.Reason == "earnings" {
			total += it.AmountMinor
		}
	}
	return total
}

func writeAtomic(path string, v any) error {
	if path == "" {
		return nil
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// usdcQuote is what a pending job's reply says about paying in USDC. Nil when
// the rail is off.
func (s *Server) usdcQuote(job string, amountMinor int64) map[string]any {
	if s.USDC == nil {
		return nil
	}
	units := chain.UnitsFor(job, amountMinor)
	return map[string]any{
		"address":     s.USDC.Address(),
		"amount_usdc": chain.Format(units),
		"chain":       s.USDC.Net.Name,
		"chain_id":    s.USDC.Net.ChainID,
		"contract":    s.USDC.Net.USDC,
		"note": "Send exactly amount_usdc of USDC on " + s.USDC.Net.Name + " to address. " +
			"The last digits are this job's own; a different amount cannot be matched " +
			"and is returned by hand. The job lists after " +
			fmt.Sprint(chain.Confirmations) + " confirmations. Whatever the job does " +
			"not pay out is sent back to the sending address by a person, on a schedule.",
	}
}

// FundFromChain turns a confirmed transfer into escrow and lists the job.
//
// The mirror of FundFromCard, and idempotent on the transaction hash: the
// watcher can offer the same transfer twice across a restart and the job is
// listed once. amountMinor is the transfer in USD minor units at par, dust
// dropped; from is the sending address, which is where any remainder is owed.
func (s *Server) FundFromChain(ctx context.Context, job, txHash string, amountMinor int64, from string) error {
	p, ok := s.pendingFor(job)
	if !ok {
		if l, live := s.Board.Get(job); live && l.Funding != nil && l.Funding.Intent == txHash {
			return nil
		}
		return fmt.Errorf("no job is waiting for payment under %s", job)
	}
	if amountMinor < p.Amount {
		return fmt.Errorf("the transfer covers %d of the %d this job could pay out", amountMinor, p.Amount)
	}
	owner := guestOwner(job)
	l := p.L
	l.Owner = owner
	l.Funding = &api.Funding{Kind: "usdc", Intent: txHash, AuthorizedMinor: p.Amount, Payer: from}
	if s.Ledger != nil {
		key := "usdc:" + txHash
		if done, err := s.Ledger.Applied(ctx, key); err != nil {
			return err
		} else if !done {
			if _, err := s.Ledger.Topup(ctx, key, owner, p.Amount, l.Currency, txHash); err != nil {
				return err
			}
		}
		if done, _ := s.Ledger.Applied(ctx, "hold-"+job); !done {
			if _, err := s.Ledger.Hold(ctx, "hold-"+job, job, owner, p.Amount, l.Currency); err != nil {
				return err
			}
		}
	}
	if err := s.Board.Post(l); err != nil {
		return err
	}
	s.mu.Lock()
	s.buyers[job] = owner
	delete(s.pending, job)
	s.mu.Unlock()
	return nil
}

// settleChain closes the USDC side once a job can no longer be worked.
//
// A transfer is prepaid, so there is nothing to capture: what the job paid
// out stays, and the remainder the ledger released into the guest principal
// is withdrawn again and owed back to the sender through the queue.
func (s *Server) settleChain(ctx context.Context, l *api.Listing, remainder int64) {
	f := l.Funding
	if f == nil || f.Kind != "usdc" || f.Settled {
		return
	}
	f.Settled = true
	if remainder <= 0 {
		return
	}
	if s.Ledger != nil {
		if _, err := s.Ledger.Withdraw(ctx, "usdc-void:"+l.Job, guestOwner(l.Job),
			remainder, l.Currency, f.Intent); err != nil {
			log.Printf("usdc       could not void the remainder for %s: %v", l.Job, err)
		}
	}
	if s.USDC == nil {
		log.Printf("usdc       %s owes %d back to %s and the rail is off", l.Job, remainder, f.Payer)
		return
	}
	reason := "refund:" + l.Job
	if s.USDC.Queue.Has(reason) {
		return
	}
	if _, err := s.USDC.Queue.Add(USDCPayout{
		Person: guestOwner(l.Job), Address: f.Payer, AmountMinor: remainder,
		Currency: l.Currency, Reason: reason, Created: s.now(),
	}); err != nil {
		log.Printf("usdc       could not queue the refund for %s: %v", l.Job, err)
	}
}

// ScanChain reads new confirmed blocks and funds whatever they paid for.
func (s *Server) ScanChain(ctx context.Context) (int, error) {
	if s.USDC == nil {
		return 0, nil
	}
	return s.USDC.Watcher.Scan(ctx, func(ctx context.Context, t chain.Transfer) (string, bool, error) {
		job, ok := s.pendingByUnits(t.Units)
		if !ok {
			log.Printf("usdc       %s USDC arrived in %s for no job; a person must return it to %s",
				chain.Format(t.Units), t.TxHash, t.From)
			return "", false, nil
		}
		if err := s.FundFromChain(ctx, job, t.TxHash, chain.MinorOf(t.Units), t.From); err != nil {
			return "", false, err
		}
		log.Printf("usdc       %s funded by %s (%s USDC from %s)", job, t.TxHash, chain.Format(t.Units), t.From)
		return job, true, nil
	})
}

// pendingByUnits finds the one pending job that asked for exactly this amount.
func (s *Server) pendingByUnits(units int64) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for job, p := range s.pending {
		if chain.UnitsFor(job, p.Amount) == units {
			return job, true
		}
	}
	return "", false
}

// StartChainWatcher scans on a timer.
func (s *Server) StartChainWatcher(ctx context.Context, every time.Duration) {
	if s.USDC == nil {
		return
	}
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if _, err := s.ScanChain(ctx); err != nil {
					log.Printf("usdc       scan: %v", err)
				}
			}
		}
	}()
}

// usdcRoute says whether a person's earnings go to an address.
func (s *Server) usdcRoute(person string) (string, bool) {
	if s.USDC == nil {
		return "", false
	}
	return s.USDC.Addresses.Get(person)
}

// stripeReady is whether the person has finished the card rail's identity
// checks, which is what lifts the address cap.
func (s *Server) stripeReady(ctx context.Context, person string) bool {
	if s.Rail == nil || s.PayoutAccounts == nil {
		return false
	}
	acct, ok := s.PayoutAccounts.Get(person)
	if !ok {
		return false
	}
	a, err := s.Rail.Account(ctx, acct)
	return err == nil && a.Ready()
}

// errUSDCCap is why a payout was not queued.
var errUSDCCap = fmt.Errorf("the address payout cap is reached; connect a payout account to keep being paid")

// queueUSDC records a payout as owed to an address and puts it in the queue.
//
// Queue first, ledger second: a queued item nobody recorded is caught by the
// removal below, whereas a ledger entry nobody queued is money that has
// left the books and will never be sent.
func (s *Server) queueUSDC(ctx context.Context, person, addr string, amountMinor int64) (*USDCPayout, error) {
	if s.USDC == nil {
		return nil, fmt.Errorf("the USDC rail is off")
	}
	if !s.stripeReady(ctx, person) && s.USDC.Queue.PaidTo(person)+amountMinor > USDCLifetimeCapMinor {
		return nil, errUSDCCap
	}
	var credited int64
	if s.Ledger != nil {
		var err error
		if credited, err = s.Ledger.Credited(ctx, ledger.PayableOf(person), "USD"); err != nil {
			return nil, err
		}
	}
	key := fmt.Sprintf("usdc-payout:%s:%d:%d", person, amountMinor, credited)
	if s.Ledger != nil {
		if done, _ := s.Ledger.Applied(ctx, key); done {
			return nil, fmt.Errorf("this payout is already recorded")
		}
	}
	item, err := s.USDC.Queue.Add(USDCPayout{
		Person: person, Address: addr, AmountMinor: amountMinor, Currency: "USD",
		Reason: "earnings", LedgerKey: key, Created: s.now(),
	})
	if err != nil {
		return nil, err
	}
	if s.Ledger != nil {
		if _, err := s.Ledger.Payout(ctx, key, person, amountMinor, "USD", "usdc-queued:"+item.ID); err != nil {
			s.USDC.Queue.Remove(item.ID)
			return nil, err
		}
	}
	return item, nil
}

// usdcStanding is what GET /v1/payout says about the address path.
func (s *Server) usdcStanding(ctx context.Context, person string) map[string]any {
	out := map[string]any{
		"usdc_on":         s.USDC != nil,
		"usdc_cap_minor":  int64(USDCLifetimeCapMinor),
		"usdc_paid_minor": int64(0),
	}
	if s.USDC == nil {
		return out
	}
	out["usdc_paid_minor"] = s.USDC.Queue.PaidTo(person)
	if addr, ok := s.USDC.Addresses.Get(person); ok {
		out["usdc_address"] = addr
		if s.stripeReady(ctx, person) {
			out["usdc_cap_lifted"] = true
		}
	}
	return out
}

// registerUSDC mounts the operator and admin routes. Mounted whether or not
// the rail is on, so the answer to "can I be paid in USDC here" is a reply
// rather than a 404.
func (s *Server) registerUSDC(mux *http.ServeMux, node *api.Server) {
	ps := &PayoutServer{Server: s, Workers: s.Workers, BaseURL: s.BaseURL}
	mux.HandleFunc("GET /v1/payout/usdc", ps.handleUSDCAddress)
	mux.HandleFunc("PUT /v1/payout/usdc", ps.handleUSDCAddress)
	mux.HandleFunc("GET /v1/payout/usdc/queue", node.WithAuth(s.handleUSDCQueue))
	mux.HandleFunc("POST /v1/payout/usdc/queue/{id}/sent", node.WithAuth(s.handleUSDCSent))
	mux.HandleFunc("GET /v1/rails", s.handleRails)
}

func (ps *PayoutServer) handleUSDCAddress(w http.ResponseWriter, r *http.Request) {
	worker, body, ok := ps.worker(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "sign in first")
		return
	}
	s := ps.Server
	if r.Method == http.MethodPut {
		if s.USDC == nil {
			writeError(w, http.StatusServiceUnavailable, "this exchange does not pay out in USDC")
			return
		}
		var in struct {
			Address string `json:"address"`
		}
		if err := json.Unmarshal(body, &in); err != nil {
			writeError(w, http.StatusBadRequest, "send {\"address\": \"0x...\"}, or an empty address to clear it")
			return
		}
		in.Address = strings.TrimSpace(in.Address)
		if in.Address != "" {
			if err := chain.ValidAddress(in.Address); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			in.Address = chain.Checksum(in.Address)
		}
		if err := s.USDC.Addresses.Put(worker.ID, in.Address); err != nil {
			writeError(w, http.StatusInternalServerError, "could not save the address")
			return
		}
	}
	out := s.usdcStanding(r.Context(), worker.ID)
	if s.USDC != nil {
		out["chain"] = s.USDC.Net.Name
		out["contract"] = s.USDC.Net.USDC
		out["note"] = "Earnings routed to this address are queued and sent by a person, on a " +
			"schedule, not by a machine. Up to usdc_cap_minor can be paid this way without " +
			"a connected payout account; beyond that, connect one."
	}
	writeJSONResponse(w, out)
}

// usdcAdmin is who may read the payout queue and mark items sent: the
// exchange's own key, and nobody else. Any signed principal could otherwise
// read every payee's address and amount, or mark a rival's payout as paid.
func (s *Server) usdcAdmin(w http.ResponseWriter, principal string) bool {
	if principal == "" || principal != s.PID {
		writeError(w, http.StatusForbidden, "the payout queue is operated with the exchange's own key")
		return false
	}
	return true
}

func (s *Server) handleUSDCQueue(w http.ResponseWriter, r *http.Request, principal string, _ []byte) {
	if !s.usdcAdmin(w, principal) {
		return
	}
	if s.USDC == nil {
		writeError(w, http.StatusServiceUnavailable, "the USDC rail is off")
		return
	}
	items := s.USDC.Queue.List()
	writeJSONResponse(w, map[string]any{
		"queue": items, "unsent": s.USDC.Queue.Unsent(),
		"chain": s.USDC.Net.Name, "contract": s.USDC.Net.USDC,
		"unmatched": s.USDC.Watcher.Unmatched(),
		"note": "Every item here is sent by a person from their own wallet, on a schedule. " +
			"After sending, POST /v1/payout/usdc/queue/{id}/sent with {\"tx\": \"0x...\"}. " +
			"Unmatched transfers arrived for no job and are returned the same way.",
	})
}

func (s *Server) handleUSDCSent(w http.ResponseWriter, r *http.Request, principal string, body []byte) {
	if !s.usdcAdmin(w, principal) {
		return
	}
	if s.USDC == nil {
		writeError(w, http.StatusServiceUnavailable, "the USDC rail is off")
		return
	}
	var in struct {
		Tx string `json:"tx"`
	}
	if err := json.Unmarshal(body, &in); err != nil || len(in.Tx) != 66 || !strings.HasPrefix(in.Tx, "0x") {
		writeError(w, http.StatusBadRequest, "send {\"tx\": \"0x<64 hex>\"}, the transaction that paid it")
		return
	}
	item, err := s.USDC.Queue.MarkSent(r.PathValue("id"), strings.ToLower(in.Tx), s.now())
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if s.Ledger != nil && item.LedgerKey != "" {
		if err := s.Ledger.SetRef(r.Context(), item.LedgerKey, "usdc-sent:"+item.SentTx); err != nil {
			log.Printf("usdc       %s marked sent as %s but the ledger ref failed: %v", item.ID, item.SentTx, err)
		}
	}
	writeJSONResponse(w, map[string]any{"sent": true, "item": item})
}

// handleRails says, to anyone, which ways money can move here.
func (s *Server) handleRails(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{
		"card":           map[string]any{"on": s.Authorize != nil},
		"stripe_connect": map[string]any{"on": s.Rail != nil},
	}
	usdc := map[string]any{"on": s.USDC != nil}
	if s.USDC != nil {
		usdc["chain"] = s.USDC.Net.Name
		usdc["chain_id"] = s.USDC.Net.ChainID
		usdc["contract"] = s.USDC.Net.USDC
		usdc["address"] = s.USDC.Address()
		usdc["confirmations"] = chain.Confirmations
		usdc["last_scanned_block"] = s.USDC.Watcher.LastBlock()
		usdc["queue_length"] = s.USDC.Queue.Unsent()
		usdc["unmatched"] = len(s.USDC.Watcher.Unmatched())
		usdc["cap_minor"] = int64(USDCLifetimeCapMinor)
		usdc["note"] = "Watch-only: the exchange holds no key. Jobs are funded by a transfer " +
			"of the exact amount quoted in pay_usdc; payouts and refunds are sent by a " +
			"person, on a schedule."
	}
	out["usdc"] = usdc
	writeJSONResponse(w, out)
}
