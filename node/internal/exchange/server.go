package exchange

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ncruces/go-sqlite3/driver"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/account"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/anchor"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/evidence"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/ledger"
	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/media"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/outcome"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/payment"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/verify"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/vision"
)

// Server is the exchange as a long-running service.
//
// It carries two surfaces that are deliberately kept apart. Operators and
// agents authenticate as principals with Ed25519 request signatures; reviewers
// and providers hold capability links and never become principals. The two
// live under different path prefixes with different middleware, so one cannot
// be reached with the other's credential.
type Server struct {
	// Key is the exchange's own principal, used to sign what it issues.
	Key ed25519.PrivateKey
	PID string

	// BaseURL is the public origin, used to build reviewer links. It must be
	// the address a reviewer's phone can actually reach, which is why it is
	// configuration rather than something derived from the request.
	BaseURL string

	// Agentic checkout: selling outcomes inside somebody else's assistant.
	// Absent keys switch it off entirely rather than leaving it open. See acp.go.
	ACP *ACPSessions
	// Charges authorises and captures cards for agentic checkout.
	Charges   ChargeRail
	ACPKey    string
	ACPSecret string

	Caps    *api.Capabilities
	Workers *api.Workers
	Reviews *api.ReviewStore
	Replay  *api.ReplayGuard
	// Board is the marketplace: open work anyone can find and claim.
	Board *api.Board
	// Capacities is what each operator agreed to take.
	Capacities *api.Capacities
	// Suppliers is the businesses providing work, when the provider is a
	// company rather than a person.
	Suppliers *api.Suppliers
	// Projects are budget envelopes several jobs share, so an orchestrating
	// agent can ask what a plan has cost and what is left.
	Projects *api.Projects
	// Book is the buy side of a company that already has vendors: who they
	// have approved, at what rates, and where their locations are.
	Book *api.Book
	// Reservations are outstanding requests for quotes. They move no money and
	// stop a buyer soliciting more in quotes than they could ever honour.
	Reservations *Reservations
	// Mail reaches somebody who is not looking at the page. Nil where it is
	// not configured, which disables alerts rather than failing.
	Mail Mailer
	// Watches records which operators want telling when work appears.
	Watches *Watches
	// staleTold remembers which unfilled jobs their buyer has been warned
	// about, so a slow week does not become a daily email.
	staleTold map[string]bool
	// notify remembers which lifecycle events have been sent. See notify.go.
	notify notifier
	// Dispatch offers new work to operators who published an endpoint. This is
	// what makes an API operator a real participant rather than someone
	// refreshing a web page.
	Dispatch *api.Dispatcher
	// Accounts holds agent keys. Nil disables the agent surface entirely,
	// which is what a node with no database does.
	Accounts *account.Store
	agents   *api.AgentKeys
	// Ledger holds the money. When set, work is escrowed before it is listed
	// and cannot be listed at all if the buyer's balance will not cover it.
	Ledger *ledger.Ledger
	// Deposit starts a payment that will fund a balance. It returns a
	// reference and somewhere to pay. Nil means this exchange cannot take
	// money, which is said plainly rather than shown as a dead link.
	Deposit func(ctx context.Context, person string, amountMinor int64, currency string) (ref, payAt string, err error)
	// Authorize opens a hosted checkout that authorises a card for a job's
	// ceiling without capturing it; Authorized asks the rail whether one
	// completed. Both nil when there is no rail, which the pay page says.
	Authorize  func(ctx context.Context, job string, amountMinor int64, currency, successURL, cancelURL string) (session, payAt string, err error)
	Authorized func(ctx context.Context, session string) (ok bool, intent string, amountMinor int64, email string, err error)
	// Payout sends accumulated earnings out. Nil means nothing can leave.
	Payout func(ctx context.Context, person string, amountMinor int64, currency string) (ref string, err error)
	// Rail is the payout side of the payment provider: opening a payee
	// account, checking whether it can receive money, and sending it. Nil on
	// an exchange with no payment provider configured, which the console
	// reports as unavailable rather than blaming the worker.
	Rail PayoutRail
	// PayoutAccounts maps a person to their account at the rail.
	PayoutAccounts *PayoutAccounts
	// USDC is the stablecoin rail: jobs funded by a transfer to the
	// exchange's address, earnings routed to an operator's own. Watch-only;
	// nil when LAMDIS_USDC_ADDRESS and LAMDIS_BASE_RPC are not both set.
	USDC *USDCRail
	// Holdbacks is money earned but not yet clear to send, because the buyer
	// still has time to object.
	Holdbacks *Holdbacks
	// DisputeWindow is how long that is. Zero means the default.
	DisputeWindow time.Duration
	// PayoutAccount reports how far through payout setup somebody is, and
	// where to finish it.
	PayoutAccount func(person string) api.PayoutState
	// Verify decides whether a submission is what it claims to be. It must
	// confirm the challenge code appears in the artifact; without that a
	// photograph proves nothing about when or where it was taken. Nil means
	// nothing is verified and nothing becomes payable.
	Verify func(api.Submission, func(string) ([]byte, bool)) (api.Submission, error)
	// OnAccepted is told about every submission that passed verification,
	// with the listing it answered. Nil means nobody is listening. It is how
	// the findings store learns what the house jobs found without the
	// settlement path knowing that a findings store exists.
	OnAccepted func(l *api.Listing, sub api.Submission)
	// Anchors ties receipt hashes to Bitcoin through OpenTimestamps, so a
	// receipt can be proven to have existed unchanged without trusting this
	// exchange. Nil means receipts are signed but not anchored. See anchor_api.go.
	Anchors *anchor.Anchorer

	mu sync.Mutex
	// store is the durable half of everything below it: the evidence bytes as
	// files, the rest as JSON snapshots on the data disk. Nil means memory
	// only, which is what a test and an exchange with no -data expect. See
	// store.go.
	store *stateStore
	// blobs holds evidence bytes by content hash, for an exchange with no
	// store. With one, the bytes live under blobs/ and are read back on
	// demand rather than held: a photograph is megabytes and there is no
	// reason for the process to carry it.
	blobs map[string][]byte
	// secrets maps a job to the capability secrets issued for it.
	secrets map[string][]string
	// submissions holds what workers have uploaded, by job.
	submissions map[string][]api.Submission
	// blobMime records what each stored artifact actually is.
	blobMime map[string]string
	// buyers remembers who funded each job, because a refund has to go back to
	// the person it came from and the listing does not carry that.
	buyers map[string]string
	// pending holds jobs posted without an account, until their card is
	// authorised. Never on the board; dropped after PendingTTL.
	pending map[string]*pendingJob
	// guestSeen is when each address last parked unpaid jobs, for the limit.
	guestSeen map[string][]time.Time
	// started is when this process came up, for the status page.
	started time.Time

	Now func() time.Time
}

// NewServer builds an exchange service.
// Options are the things a real deployment needs and a test usually does not.
type Options struct {
	// DataDir is where the ledger and accounts live. Empty means in-memory,
	// which is right for a test and wrong for anything else.
	DataDir string
	// Vision reads images. Nil means submissions are stored and never become
	// payable, which is stated rather than hidden.
	Vision vision.Model
	// Media decomposes video. Nil means video is refused with an explanation.
	Media media.Extractor
	// Transcriber reads soundtracks.
	Transcriber media.Transcriber
}

// NewServer builds an exchange with nothing attached.
//
// Kept for tests that want a bare server. Anything that serves real traffic
// wants Open, because a server without a ledger cannot escrow, without
// accounts cannot mount the agent surface, and without a verifier cannot ever
// pay anybody — and every one of those failures is silent.
func NewServer(key ed25519.PrivateKey, baseURL string) (*Server, error) {
	return Open(key, baseURL, Options{})
}

// Open builds a complete exchange.
func Open(key ed25519.PrivateKey, baseURL string, opt Options) (*Server, error) {
	pid, err := protolog.PrincipalID(key.Public().(ed25519.PublicKey))
	if err != nil {
		return nil, err
	}
	caps := api.NewCapabilities()
	board := api.NewBoard(caps)
	capacities := api.NewCapacities()
	board.Capacities = capacities
	workers := api.NewWorkers()
	// Hosted identity, if this deployment has one. Empty configuration leaves
	// it disabled, which is what lets the exchange run locally with no AWS
	// account — and, deliberately, with nobody payable.
	workers.Cognito = api.NewCognito(
		os.Getenv("AWS_REGION"),
		os.Getenv("LAMDIS_COGNITO_POOL"),
		os.Getenv("LAMDIS_COGNITO_CLIENT"),
	)
	srv := &Server{
		Key: key, PID: pid, BaseURL: baseURL,
		Board: board, Capacities: capacities,
		Caps: caps, Workers: workers, Reviews: api.NewReviewStore(),
		Replay:      api.NewReplayGuard(10 * time.Minute),
		blobs:       map[string][]byte{},
		secrets:     map[string][]string{},
		submissions: map[string][]api.Submission{},
		blobMime:    map[string]string{},
		buyers:      map[string]string{},
		started:     time.Now(),
		Now:         time.Now,
	}

	// The money. A file when there is somewhere to put it, memory otherwise,
	// but never absent: a nil ledger silently disables escrow and every job
	// lists unfunded.
	ledgerPath, accountsPath := ":memory:", ":memory:"
	if opt.DataDir != "" {
		if err := os.MkdirAll(opt.DataDir, 0o750); err != nil {
			return nil, err
		}
		ledgerPath = filepath.Join(opt.DataDir, "ledger.db")
		accountsPath = filepath.Join(opt.DataDir, "accounts.db")

		// The open work, the seats somebody is holding, the links they were
		// issued, and the evidence they uploaded. All of it used to die with
		// the process while the escrow behind it stayed in the ledger below.
		st, err := openState(opt.DataDir)
		if err != nil {
			return nil, err
		}
		srv.store = st
		srv.loadState()
		board.Persist(opt.DataDir)
		caps.Persist(opt.DataDir)
		capacities.Persist(opt.DataDir)
	}
	l, err := ledger.Open(ledgerPath)
	if err != nil {
		return nil, fmt.Errorf("opening the ledger: %w", err)
	}
	srv.Ledger = l

	adb, err := driver.Open("file:" + accountsPath +
		"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("opening accounts: %w", err)
	}
	accounts, err := account.New(adb)
	if err != nil {
		return nil, fmt.Errorf("preparing accounts: %w", err)
	}
	srv.Accounts = accounts

	// The payment rail, if this deployment has one.
	//
	// Both directions are wired here or neither is. Until this block existed
	// the hooks were declared and never assigned, so top-ups refused and every
	// worker was told "payouts are not switched on yet" — permanently, with no
	// way to discover that the exchange itself was the thing missing.
	// A threshold nobody measured must not be able to refuse somebody's work.
	// Checked here so it fails at startup rather than at somebody's payout.
	if err := CheckCalibrations(map[string]bool{
		"SyntheticThreshold": true,
	}); err != nil {
		return nil, err
	}
	srv.Suppliers = api.NewSuppliers()
	srv.Projects = api.NewProjects()
	srv.Book = api.NewBook()
	srv.Reservations = NewReservations()
	srv.Watches = NewWatches()
	if m := NewSES(); m != nil {
		srv.Mail = m
	}
	srv.Board.Suppliers = srv.Suppliers
	// Agentic checkout. Both halves or neither: a key with no signing secret
	// would mean accepting unsigned requests, which is worse than being off.
	if k, sec := os.Getenv("LAMDIS_ACP_KEY"), os.Getenv("LAMDIS_ACP_SECRET"); k != "" && sec != "" {
		srv.ACP, srv.ACPKey, srv.ACPSecret = NewACPSessions(), k, sec
	}
	srv.PayoutAccounts = NewPayoutAccounts(opt.DataDir)
	srv.Holdbacks = NewHoldbacks(opt.DataDir)
	// The stablecoin rail, when its address and an RPC are configured.
	if usdc, err := NewUSDCRailFromEnv(opt.DataDir); err != nil {
		return nil, err
	} else if usdc != nil {
		srv.USDC = usdc
		log.Printf("usdc       watching %s for transfers to %s", usdc.Net.Name, usdc.Address())
	}
	if rail, err := payment.NewStripe(); err == nil {
		srv.Rail = rail
		srv.Charges = rail
		srv.PayoutAccount = srv.payoutAccountFor
		srv.Payout = func(ctx context.Context, person string, amountMinor int64, currency string) (string, error) {
			return srv.payOut(ctx, person, amountMinor, currency)
		}
		base := strings.TrimSuffix(baseURL, "/")
		srv.Authorize = rail.AuthorizeCheckout
		srv.Authorized = rail.CheckoutAuthorization
		srv.Deposit = func(ctx context.Context, person string, amountMinor int64, currency string) (string, string, error) {
			return rail.Checkout(ctx, person, amountMinor, currency,
				base+"/console/funds?topup=done&session={CHECKOUT_SESSION_ID}",
				base+"/console/funds?topup=cancelled")
		}
		log.Printf("payments   stripe (%s)", modeOf(rail))
	} else {
		log.Printf("payments   unavailable — %v", err)
	}

	// The verifier. Wired even with no vision model, because it still catches
	// reuse and geofence violations, and because a nil hook means submissions
	// are accepted without anyone ever looking at them.
	// The reuse corpus is on disk when there is a disk. In memory it forgot
	// every photograph at each deploy, and the same picture earned again the
	// morning after a restart.
	corpus := verify.NewCorpus()
	if opt.DataDir != "" {
		c, err := verify.OpenCorpus(filepath.Join(opt.DataDir, "corpus.jsonl"))
		if err != nil {
			return nil, fmt.Errorf("opening the evidence corpus: %w", err)
		}
		corpus = c
	}
	srv.Verify = (&SubmissionVerifier{
		Vision: opt.Vision, Corpus: corpus,
		Media: opt.Media, Transcriber: opt.Transcriber,
		StoreFrame: srv.storeFrame,
		StageDeliverable: func(job string, stage int) (string, bool) {
			l, ok := srv.Board.Get(job)
			if !ok || !l.Staged() {
				return "", false
			}
			st, ok := l.StageAt(stage)
			if !ok {
				return "", false
			}
			return st.Deliverable, true
		},
		Predicate: func(job string) (string, string, bool) {
			l, ok := srv.Board.Get(job)
			if !ok {
				return "", "", false
			}
			return l.Title, l.Kind, true
		},
	}).Verify

	// Wired here rather than where the routes are built: a check that only
	// takes effect once Handler has been called is a check that anything
	// running before Handler quietly escapes.
	board.Funded = srv.checkFunded

	// Said last, because it needs the ledger and the board both. It reports
	// and never repairs: see reconcile.
	srv.reconcile(context.Background())
	return srv, nil
}

// storeFrame keeps a still pulled from a video so a reviewer can see what the
// model saw.
func (s *Server) storeFrame(job, sha string, jpeg []byte) error {
	s.putBlob(sha, "image/jpeg", jpeg)
	return nil
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Handler mounts every surface.
func (s *Server) Handler() *http.ServeMux {
	mux := http.NewServeMux()

	// Health is unauthenticated by necessity: the orchestrator has no
	// principal. It deliberately reveals nothing beyond liveness.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})

	// Public identity, so a counterparty can learn the exchange's principal
	// before trusting anything it signed. This is the same reasoning the node
	// applies to /v1/node.
	mux.HandleFunc("GET /v1/exchange", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		panels := len(s.secrets)
		s.mu.Unlock()
		writeJSONResponse(w, map[string]any{
			"principal": s.PID,
			"uptime_s":  int(s.now().Sub(s.started).Seconds()),
			"panels":    panels,
			// How to sign in, so a client learns it from the exchange rather
			// than from documentation that can drift.
			"identity": map[string]any{
				"accounts_required": true,
				"sign_in":           "/signin",
				"configured":        s.Workers.Cognito.Enabled(),
			},
			"tiers": map[string]string{
				"V0": "a signed claim, no artifact",
				"V1": "an artifact with deterministic checks",
				"V2": "artifact, challenge code, and adjudication",
				"V3": "two independent sources that agree",
			},
		})
	})

	// The reviewer surface: capability-authenticated, its own prefix.
	rs := &api.ReviewServer{
		Caps: s.Caps, Reviews: s.Reviews, Replay: s.Replay,
		Secrets: s.secretsFor,
		Blob:    s.blob,
		// A recorded review frees its seat and is paid from the panel's
		// escrow. Without both, every review lapsed into an abandonment and
		// credited nobody.
		Board: s.Board, Settled: s.settleReview,
	}
	rs.Register(mux)

	// Nothing reaches the board without the money to pay for it.
	// The settlement constant, which is the one a worker actually feels.
	s.Board.FeeBP = FeeBP
	s.Board.PayoutThresholdMinor = PayoutThresholdMinor
	s.Board.Funded = s.checkFunded
	s.Board.Workers = s.Workers
	s.Dispatch = &api.Dispatcher{
		Board: s.Board, Capacities: s.Capacities,
		BaseURL: strings.TrimSuffix(s.BaseURL, "/"), Now: s.now,
	}
	s.Board.Announce = func(l *api.Listing) {
		if n := s.Dispatch.Announce(context.Background(), l); n > 0 {
			log.Printf("dispatch: %s offered to %d operator(s)", l.Job, n)
		}
		// A fleet gets a webhook; a person with a phone gets an email. Same
		// signal, and until now only the fleet had one.
		s.AlertNewWork(l)
	}
	s.Board.Register(mux)
	api.RegisterSkills(mux)
	api.RegisterDocs(mux)
	// The provider's own callback. Nil when no signing secret is configured,
	// which leaves the endpoint unmounted rather than mounting one that would
	// credit a balance for anybody who POSTs JSON at it.
	if wh := NewStripeWebhook(s); wh != nil {
		wh.Register(mux)
		log.Printf("payments   webhook mounted at /v1/stripe/webhook")
	} else {
		log.Printf("payments   NO webhook — deposits only credit if the buyer " +
			"returns to the page, and bank debits cannot work at all")
	}
	api.RegisterTrust(mux)
	(&PayoutServer{Server: s, Workers: s.Workers, BaseURL: s.BaseURL}).Register(mux)
	(&SpendServer{Server: s, Workers: s.Workers}).Register(mux)
	(&AlertServer{Server: s, Workers: s.Workers}).Register(mux)
	(&StatementServer{Server: s, Workers: s.Workers}).Register(mux)
	(&api.SupplierServer{Suppliers: s.Suppliers, Workers: s.Workers,
		Board: s.Board, Now: s.now}).Register(mux)
	// Capacity: what an operator will take, gating dispatch rather than
	// decorating a settings page.
	s.Board.Capacities = s.Capacities
	(&api.CapacityServer{
		Workers: s.Workers, Capacities: s.Capacities, Board: s.Board,
	}).Register(mux)

	// Where a worker sees their own standing.
	console := &api.Console{
		Workers: s.Workers, Board: s.Board,
		Earnings:     s.earningsFor,
		PayoutStatus: s.payoutStatusFor,
		TaxStatus: func(worker string) any {
			return s.TaxStatusFor(context.Background(), worker)
		},
		History:              s.historyFor,
		Bids:                 s.bidsFor,
		Frozen:               s.frozenFor,
		PayoutThresholdMinor: PayoutThresholdMinor,
	}
	console.Register(mux)

	// Agent credentials, issued by a signed-in person for their own agent.
	if s.Accounts != nil {
		s.agents = &api.AgentKeys{Accounts: s.Accounts, Workers: s.Workers}
		s.agents.Register(mux)
		// The buyer's own view of a job, its bids, its evidence and its
		// receipt take any credential that spends this account's money —
		// the person's session included. They were agent-key only, which
		// meant the console linked a signed-in buyer to a page that answered
		// 401 and bounced them to sign in again. None of these handlers
		// reads the key; handleAgentBalance does, and stays agent-only.
		mux.HandleFunc("GET /v1/jobs/{job}", s.withBuyer(s.handleJobStatus))
		mux.HandleFunc("GET /v1/agent/balance", s.withAgent(s.handleAgentBalance))
		mux.HandleFunc("GET /v1/jobs/{job}/bids", s.withBuyer(s.handleListBids))
		mux.HandleFunc("POST /v1/jobs/{job}/award", s.withBuyer(s.handleAwardBid))
		mux.HandleFunc("GET /v1/jobs/{job}/receipt", s.withBuyer(s.handleJobReceipt))
		mux.HandleFunc("GET /v1/jobs/{job}/receipt/anchor", s.withBuyer(s.handleReceiptAnchor))
		s.registerReview(mux)
		s.registerQuote(mux)
		s.registerProjects(mux)
		s.registerScopeBuyer(mux)
		s.registerReferences(mux)
		// The front door: MCP over HTTP, no binary to build.
		s.registerMCP(mux)
		// The gateless way in: pages a poster with no account lands on.
		s.registerGuest(mux)
		// Agentic checkout, when it is configured.
		if s.ACP != nil {
			s.registerACP(mux)
		}
		s.registerBook(mux)
		mux.HandleFunc("GET /v1/jobs/{job}/evidence", s.withBuyer(s.handleJobEvidence))
		mux.HandleFunc("GET /v1/jobs/{job}/evidence/{sha}", s.withBuyer(s.handleEvidenceFile))
		// Funding and withdrawal are things a person does from the console with
		// their session, as well as things an agent does with its key. The
		// console has always sent the session token here; withAgent only ever
		// read X-Lamdis-Key, so "Add funds" answered 401 to every signed-in
		// buyer and bounced them to sign in again. Neither handler uses the key.
		mux.HandleFunc("POST /v1/balance/topup", s.withBuyer(s.handleTopupIntent))
		mux.HandleFunc("GET /v1/balance/withdraw", s.withBuyer(s.handleWithdraw))
	}

	// Sign-in. Everything that takes work requires an account.
	auth := api.NewAuth(os.Getenv("AWS_REGION"), os.Getenv("LAMDIS_COGNITO_POOL"),
		os.Getenv("LAMDIS_COGNITO_CLIENT"), s.Workers)
	auth.Register(mux)

	// Assignment and claiming.
	wk := &api.WorkerServer{
		Workers: s.Workers, Board: s.Board, Replay: s.Replay,
	}
	wk.Register(mux)

	ws := &api.WorkServer{
		Caps: s.Caps, Board: s.Board,
		Replay: s.Replay,
		Submit: s.acceptEvidence,
		Store:  s.storeArtifact,
	}
	ws.Register(mux)

	// Operator surface: creating a panel is a principal-authenticated act.
	// A capability holder must never be able to mint more capabilities.
	node := &api.Server{Principal: s.PID}
	mux.HandleFunc("POST /v1/panels", node.WithAuth(s.handleCreatePanel))
	// The stablecoin rail's operator and admin routes, and the public list
	// of which rails are on.
	s.registerUSDC(mux, node)
	// One route, two credentials. A person's own key signs; their agent
	// presents a key that person issued. Registering it twice — once per
	// scheme — panics the mux at boot, which is how this was found.
	mux.HandleFunc("POST /v1/tasks", s.postJobAsAnyone(node))

	// The chain of receipt roots, readable by anyone.
	s.registerAnchors(mux)

	mux.HandleFunc("GET /", s.handleIndex)
	return mux
}

// secretsFor supplies the candidate secrets for a job. Capabilities reach a
// job by two routes — pushed as a link, or claimed from the board — and
// authentication has to consider both.
func (s *Server) secretsFor(job string) []string {
	s.mu.Lock()
	pushed := append([]string(nil), s.secrets[job]...)
	s.mu.Unlock()
	if s.Board != nil {
		pushed = append(pushed, s.Board.Secrets(job)...)
	}
	return pushed
}

// storeArtifact keeps one uploaded file under its content hash.
//
// The bytes are written exactly as received: a re-encoded copy would mean the
// hash in the signed trail did not describe the file the worker actually took.
func (s *Server) storeArtifact(job string, a api.Artifact, data []byte) error {
	s.putBlob(a.SHA256, a.Mime, data)
	return nil
}

// acceptEvidence records a finished submission and has it verified.
//
// Verification does not happen at upload time because a submission is not one
// file: a video and the still that supports it are judged together, and a
// worker adding a second angle has not yet said they are finished.
//
// If no verifier is wired the submission is kept with Verified false and stays
// unpayable, which is the honest state for evidence nobody has looked at.
// Marking it accepted would make the marketplace appear to work while paying
// for photographs of nothing.
func (s *Server) acceptEvidence(sub api.Submission) (api.Submission, error) {
	s.mu.Lock()
	s.submissions[sub.Job] = append(s.submissions[sub.Job], sub)
	idx := len(s.submissions[sub.Job]) - 1
	// Written before verification, not after: evidence that arrived and was
	// never judged is still the worker's proof that they went.
	s.saveSubmissionsLocked()
	s.mu.Unlock()

	if s.Verify == nil {
		return sub, nil
	}
	verified, err := s.Verify(sub, s.blobFor)
	if err != nil {
		s.notifyRejected(sub, err.Error(), true)
		return sub, err
	}
	s.mu.Lock()
	if idx < len(s.submissions[sub.Job]) {
		s.submissions[sub.Job][idx] = verified
		s.saveSubmissionsLocked()
	}
	s.mu.Unlock()

	// Money moves here or it never moves. Everything upstream of this line is
	// a claim about evidence; this is the part the worker came for.
	if worker, ok := s.Board.WorkerFor(verified.Holder); ok {
		if l, ok := s.Board.Get(verified.Job); ok && l.Practice {
			// A rehearsal holds nothing and pays nothing. Settling it produced
			// "nothing is held" and the message below, which told a newcomer
			// their first $0 was "still settling".
		} else if err := s.settle(context.Background(), verified.Job, verified, worker); err != nil {
			// The evidence stands even if settlement did not. Losing the
			// submission because the ledger was busy would be the worse of the
			// two failures, and the reconciler is what resolves the other.
			verified.Why = "accepted; payment is still settling"
		}
	}
	if s.OnAccepted != nil && verified.Verified {
		if l, ok := s.Board.Get(verified.Job); ok {
			s.OnAccepted(l, verified)
		}
	}
	return verified, nil
}

// blobFor lets the verifier read back the bytes it was handed hashes for.
func (s *Server) blobFor(sha string) ([]byte, bool) {
	s.mu.Lock()
	b, ok := s.blobs[sha]
	st := s.store
	s.mu.Unlock()
	if ok || st == nil {
		return b, ok
	}
	data, err := st.readBlob(sha)
	if err != nil {
		return nil, false
	}
	return data, true
}

// AddPanel registers a review panel together with the artifact reviewers must
// look at, and lists it on the board.
//
// A panel without its evidence is not a panel: the page has nothing to show
// and every capability issued against it looks broken. Keeping the two in one
// call is what stops them being registered separately and drifting apart.
func (s *Server) AddPanel(p *api.ReviewPanel, img []byte, mime string) error {
	art, err := evidence.Analyze(img, mime)
	if err != nil {
		return fmt.Errorf("panel evidence: %w", err)
	}
	p.EvidenceSHA = []string{art.SHA256}
	s.putBlob(art.SHA256, mime, img)
	s.Reviews.Add(p)
	return s.Board.Post(&api.Listing{
		Job: p.Job, Parent: p.Parent, Kind: api.KindReview,
		Title: p.Question, Detail: p.Context,
		PayMinor: p.FeeMinor, BonusMinor: p.BonusMinor, Currency: p.Currency,
		Slots: p.Reviewers, Expires: p.Expires, Posted: s.now(),
	})
}

// withAgent authenticates an agent key and hands the handler the person it
// acts for, plus the key's own limits.
//
// The callback takes both deliberately. Attribution is to the person — an
// agent is an instrument, not a party — but the limits belong to the key, so a
// compromised key is bounded without touching its siblings.
func (s *Server) withAgent(
	next func(w http.ResponseWriter, r *http.Request, key *account.Key, person string, body []byte),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		if err != nil {
			writeError(w, http.StatusBadRequest, "could not read the request")
			return
		}
		key, person, ok := s.agents.AuthenticateAgent(r)
		if !ok {
			writeError(w, http.StatusUnauthorized,
				"present an agent key issued from your account")
			return
		}
		next(w, r, key, person.ID, body)
	}
}

// handleCreateTaskAsAgent posts a job on behalf of the agent's person, after
// checking the key is allowed to spend that much.
func (s *Server) handleCreateTaskAsAgent(w http.ResponseWriter, r *http.Request, key *account.Key, person string, body []byte) {
	var peek CreateTaskRequest
	if err := json.Unmarshal(body, &peek); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request")
		return
	}
	slots := peek.Slots
	if slots == 0 {
		slots = 1
	}
	commit := (peek.FeeMinor + peek.BonusMinor + peek.ExpenseCapMinor) * int64(slots)
	job := fmt.Sprintf("pending-%d", s.now().UnixNano())
	if err := s.Accounts.Commit(r.Context(), key, job, commit, orDefault(peek.Currency, "USD")); err != nil {
		// An agent over its limit is told exactly which limit, because the
		// person who set it is the one who has to decide whether to raise it.
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	s.handleCreateTask(w, r, person, body)
}

// handleJobStatus reports where a job has got to.
// ownedBy fetches a job only if this account is the one paying for it.
//
// Every buyer-side route goes through here. Without it, any authenticated
// account could read another buyer's sealed bids and award their job to a
// colluding bidder — spending money that was never theirs. The bids being
// sealed on the board counts for nothing if the API hands them to whoever
// asks.
//
// The refusal is a 404 rather than a 403: telling a stranger that a job exists
// but is not theirs is still telling them it exists.
func (s *Server) ownedBy(w http.ResponseWriter, job, person string) (*api.Listing, bool) {
	l, ok := s.Board.Get(job)
	if !ok || l.Owner == "" || l.Owner != person {
		writeError(w, http.StatusNotFound, "no such job")
		return nil, false
	}
	return l, true
}

func (s *Server) handleJobStatus(w http.ResponseWriter, r *http.Request, key *account.Key, person string, _ []byte) {
	job := r.PathValue("job")
	if p, ok := s.pendingFor(job); ok && person == guestOwner(job) {
		writeJSONResponse(w, map[string]any{
			"job": job, "kind": p.L.Kind, "predicate": p.L.Title,
			"status": "awaiting_payment", "pay_at": p.PayAt,
			"amount_minor": p.Amount, "currency": p.L.Currency,
			"expires_at": p.Created.Add(PendingTTL).Format(time.RFC3339),
		})
		return
	}
	l, ok := s.ownedBy(w, job, person)
	if !ok {
		return
	}
	subs := s.Submissions(job)
	out := map[string]any{
		"job": job, "kind": l.Kind, "predicate": l.Title,
		"slots": l.Slots, "taken": l.Taken,
		"expires":     l.Expires.Format(time.RFC3339),
		"submissions": len(subs),
	}
	if s.Ledger != nil {
		held, _ := s.Ledger.Held(r.Context(), job, l.Currency)
		out["escrow_minor"] = held
	}
	// What actually came back, and why it was or was not accepted.
	var results []map[string]any
	for _, sub := range subs {
		res := map[string]any{
			"at": sub.At.Format(time.RFC3339), "verified": sub.Verified,
			"files": len(sub.Artifacts),
		}
		if sub.Why != "" {
			res["why"] = sub.Why
		}
		var kinds []string
		for _, a := range sub.Artifacts {
			kinds = append(kinds, a.Kind)
			if a.Transcript != "" {
				res["transcript"] = a.Transcript
			}
		}
		res["media"] = kinds
		results = append(results, res)
	}
	out["results"] = results
	writeJSONResponse(w, out)
}

// postJob accepts a job from either an agent or a principal.
//
// Agent keys are tried first because they carry limits, and a person who has
// bothered to issue one wants those limits applied rather than bypassed by
// also holding the signing key.
func (s *Server) postJob(node *api.Server) http.HandlerFunc {
	signed := node.WithAuth(s.handleCreateTask)
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Lamdis-Key") != "" && s.agents != nil {
			s.withAgent(s.handleCreateTaskAsAgent)(w, r)
			return
		}
		signed(w, r)
	}
}

// postJobAsAnyone adds the third credential: a signed-in person.
//
// A buyer could sign in, add funds and issue keys from the console, and then
// could not post a job from it — the route took an agent key or a signed
// principal, and a browser session is neither. The person's account is the
// buyer; there is no key, so no key limits apply, and the funding check in
// handleCreateTask still does.
//
// Order matters. An agent key first, because it carries limits the person
// chose. Then the session, which must be Verified: an enrolled device key is
// not an identity. Anything else falls through to the signed-principal path
// exactly as before, with the body handed back untouched.
func (s *Server) postJobAsAnyone(node *api.Server) http.HandlerFunc {
	signed := s.postJob(node)
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Lamdis-Key") != "" || s.Workers == nil {
			signed(w, r)
			return
		}
		bearer := strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ")
		body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		if err != nil {
			writeError(w, http.StatusBadRequest, "could not read the request")
			return
		}
		if !bearer && r.Header.Get("X-Lamdis-Principal") == "" {
			if r.Header.Get("X-Lamdis-Signature") == "" {
				// Nobody at all. The gateless path: the job is created, a
				// pay link comes back, and the card stands behind it. What
				// may be posted does not change; only who may post it.
				s.handleCreateTask(w, r, guestSentinel, body)
				return
			}
			r.Body = io.NopCloser(strings.NewReader(string(body)))
			signed(w, r)
			return
		}
		worker, err := s.Workers.Authenticate(r, body, s.now())
		if err == nil && worker.Verified {
			s.handleCreateTask(w, r, worker.ID, body)
			return
		}
		if bearer {
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]any{
					"error": "sign in to post a job", "signin": "/signin",
				})
				return
			}
			writeError(w, http.StatusForbidden,
				"verify your account before posting work; an unverified account "+
					"cannot be charged for it")
			return
		}
		// A signed principal that is not a verified person: the integration
		// path, unchanged.
		r.Body = io.NopCloser(strings.NewReader(string(body)))
		signed(w, r)
	}
}

// handleJobReceipt returns the signed record of a finished job.
//
// This is the artefact the whole system exists to produce: what was asked, what
// arrived, what was concluded, and what moved — in a form somebody who does not
// trust this exchange can check for themselves.
func (s *Server) handleJobReceipt(w http.ResponseWriter, r *http.Request, key *account.Key, person string, _ []byte) {
	job := r.PathValue("job")
	l, ok := s.ownedBy(w, job, person)
	if !ok {
		return
	}
	subs := s.Submissions(job)
	if len(subs) == 0 {
		writeError(w, http.StatusConflict, "nothing has been submitted for this job yet")
		return
	}

	var evidence []map[string]any
	settled := false
	for _, sub := range subs {
		files := make([]map[string]any, 0, len(sub.Artifacts))
		for _, a := range sub.Artifacts {
			f := map[string]any{"sha256": a.SHA256, "kind": a.Kind, "bytes": a.Bytes}
			if a.ChallengeSeen != "" {
				f["challenge_found_in"] = a.ChallengeSeen
			}
			if a.HasGeo {
				f["lat"] = float64(a.LatE7) / 1e7
				f["lon"] = float64(a.LonE7) / 1e7
			}
			if a.Transcript != "" {
				f["transcript"] = a.Transcript
			}
			files = append(files, f)
		}
		e := map[string]any{
			"at": sub.At.Format(time.RFC3339), "accepted": sub.Verified,
			"attested_by": sub.AttestedBy, "files": files,
		}
		if sub.Why != "" {
			e["why"] = sub.Why
		}
		evidence = append(evidence, e)
		settled = settled || sub.Verified
	}

	out := map[string]any{
		"job": job, "kind": l.Kind, "predicate": l.Title,
		"issued_by": s.PID, "issued_at": s.now().Format(time.RFC3339),
		"evidence": evidence, "accepted": settled,
		"verification": verificationBlock(l, subs),
	}
	// The buyer's own identifiers, carried untouched and signed with the rest.
	//
	// A receipt that cannot be matched to a purchase order cannot be paid by a
	// company with an accounts department, and a sweep across four hundred
	// stores is unreadable without knowing which store each one was.
	if l.Reference != "" {
		out["reference"] = l.Reference
	}
	if l.SiteID != "" {
		out["site"] = l.SiteID
	}
	if l.Directed() {
		out["directed"] = true
	}
	if s.Ledger != nil {
		held, _ := s.Ledger.Held(r.Context(), job, l.Currency)
		out["escrow_remaining_minor"] = held
	}
	// Anchored, so it can be shown to have existed without this server.
	s.anchorReceipt(job, out)
	// Signed, so the receipt stands on its own away from this server.
	if body, err := json.Marshal(out); err == nil {
		out["signature"] = hex.EncodeToString(ed25519.Sign(s.Key, body))
	}
	writeJSONResponse(w, out)
}

// handleListBids shows the offers on an open job.
func (s *Server) handleListBids(w http.ResponseWriter, r *http.Request, key *account.Key, person string, _ []byte) {
	job := r.PathValue("job")
	// Sealed means sealed from everyone except the buyer. A competitor who
	// can read this list can undercut it by a dollar, which is the auction
	// with extra steps.
	l, ok := s.ownedBy(w, job, person)
	if !ok {
		return
	}
	writeJSONResponse(w, map[string]any{
		"job": job, "pricing": l.Pricing, "ceiling_minor": l.MaxBidMinor,
		"closes":  l.BidsCloseAt.Format(time.RFC3339),
		"awarded": l.Awarded, "bids": s.Board.Bids(job),
	})
}

// handleAwardBid accepts one offer, escrowing the winning amount.
//
// The funding check runs here rather than only at posting because until a bid
// is accepted nobody knows what the job costs. An open job is held against the
// ceiling the buyer set; awarding above it is refused even if the bid is good.
func (s *Server) handleAwardBid(w http.ResponseWriter, r *http.Request, key *account.Key, person string, body []byte) {
	var in struct {
		Bid string `json:"bid"`
	}
	if err := json.Unmarshal(body, &in); err != nil || in.Bid == "" {
		writeError(w, http.StatusBadRequest, "name the bid to accept")
		return
	}
	job := r.PathValue("job")
	if _, ok := s.ownedBy(w, job, person); !ok {
		return
	}
	// Escrow at the agreed price, which is the first moment there is one.
	//
	// Accepting a bid is where the money becomes real: before this the job
	// carried only a reservation, because nobody knew the amount. Held before
	// the award is recorded, so a failure here leaves the job open rather than
	// awarded and unfunded.
	l, _ := s.Board.Get(job)
	bid, ok := s.Board.Bid(job, in.Bid)
	if !ok {
		writeError(w, http.StatusNotFound, "no such bid on this job")
		return
	}
	if s.Ledger != nil && l != nil && l.Pricing == api.PriceBids {
		if _, err := s.Ledger.Hold(r.Context(), "hold-"+job, job, person,
			bid.AmountMinor, bid.Currency); err != nil {
			writeError(w, http.StatusPaymentRequired, err.Error())
			return
		}
	}
	won, err := s.Board.Award(job, in.Bid, s.checkFunded)
	if err != nil {
		// The hold is idempotent on its key, so a retry does not double it;
		// the sweeper returns it if the job never runs.
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	// The request is settled either way, so it stops counting against what
	// this buyer may ask for elsewhere.
	s.Reservations.Release(job)
	s.notifyAwarded(l, won.Worker, won.AmountMinor, won.Currency)

	writeJSONResponse(w, map[string]any{
		"job": job, "awarded_to": won.Worker,
		"amount_minor": won.AmountMinor, "currency": won.Currency,
		"escrowed_minor": won.AmountMinor,
	})
}

// handleAgentBalance says what this agent may still spend.
func (s *Server) handleAgentBalance(w http.ResponseWriter, r *http.Request, key *account.Key, person string, _ []byte) {
	out := map[string]any{"account": person, "key": key.ID, "limits": key.Limits}
	if s.Ledger != nil {
		bal, _ := s.Ledger.Balance(r.Context(), ledger.BalanceOf(person), "USD")
		out["balance_minor"] = bal
	}
	if total, open, err := s.Accounts.Committed(r.Context(), key.ID); err == nil {
		out["committed_minor"] = total
		out["in_flight_minor"] = open
		if key.Limits.MaxTotalMinor > 0 {
			out["remaining_minor"] = key.Limits.MaxTotalMinor - total
		}
	}
	writeJSONResponse(w, out)
}

// earningsFor reads a worker's position from the ledger.
//
// The ledger is the only source: a second tally kept for display would be a
// second source of truth, and the one people read would be the one that drifts.
func (s *Server) earningsFor(worker string) (earned, paid int64, currency string) {
	currency = "USD"
	if s.Ledger == nil {
		return 0, 0, currency
	}
	ctx := context.Background()
	// What is still owed sits in their payable account; what has left is the
	// difference between everything credited and that balance.
	pending, _ := s.Ledger.Balance(ctx, ledger.PayableOf(worker), currency)
	credited, _ := s.Ledger.Credited(ctx, ledger.PayableOf(worker), currency)
	return credited, credited - pending, currency
}

// payoutStatusFor reports how far through payout setup a worker is.
//
// When no payment rail is attached this returns Unavailable rather than
// "not connected". The difference matters to the person reading it: "not
// connected" invites them to go and connect something, and there is nothing
// there to connect to. Telling somebody to act and giving them no way to is
// worse than telling them to wait.
func (s *Server) payoutStatusFor(worker string) api.PayoutState {
	if s.Payout == nil {
		return api.PayoutState{Unavailable: true}
	}
	if s.PayoutAccount == nil {
		return api.PayoutState{}
	}
	return s.PayoutAccount(worker)
}

// bidsFor lists this worker's own open offers, so a bid does not vanish the
// moment it is placed.
func (s *Server) bidsFor(worker string) []api.OpenBid {
	var out []api.OpenBid
	for _, l := range s.Board.All() {
		if l.Pricing != api.PriceBids {
			continue
		}
		for _, b := range s.Board.Bids(l.Job) {
			if b.Worker != worker {
				continue
			}
			ob := api.OpenBid{
				Job: l.Job, Title: l.Title, Where: l.Where,
				AmountMinor: b.AmountMinor, Currency: b.Currency,
				Note: b.Note, Placed: b.Placed,
				ClosesAt: l.BidsCloseAt, Won: b.Won,
			}
			switch {
			case b.Won:
				ob.Status = "won — the job is yours"
			case l.Awarded != "":
				ob.Status = "not chosen this time"
			case !l.BidsCloseAt.IsZero() && s.now().After(l.BidsCloseAt):
				ob.Status = "bidding closed, waiting on a decision"
			default:
				ob.Status = "open — you can still change it"
			}
			out = append(out, ob)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Placed.After(out[j].Placed) })
	return out
}

// historyFor lists what this worker submitted.
func (s *Server) historyFor(worker string) []api.WorkRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []api.WorkRecord
	for job, subs := range s.submissions {
		for _, sub := range subs {
			holderWorker, _ := s.Board.WorkerFor(sub.Holder)
			if holderWorker != worker {
				continue
			}
			l, _ := s.Board.Get(job)
			rec := api.WorkRecord{
				Job: job, At: sub.At, Why: sub.Why,
				Status: "checking", Currency: "USD",
			}
			if l != nil {
				rec.Kind, rec.Title, rec.Currency = l.Kind, l.Title, l.Currency
				rec.AmountMinor = l.PayMinor
			}
			if sub.Verified {
				rec.Status = "accepted"
			} else if sub.Why != "" {
				rec.Status, rec.AmountMinor = "rejected", 0
			}
			out = append(out, rec)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}

// Submissions returns what has been uploaded for a job.
func (s *Server) Submissions(job string) []api.Submission {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]api.Submission, len(s.submissions[job]))
	copy(out, s.submissions[job])
	return out
}

// blob serves stored evidence.
//
// The type is what was recorded when the bytes arrived, not a guess. Serving a
// HEIC or an MP4 as image/jpeg would have the reviewer's browser refuse to
// render evidence that is perfectly valid.
func (s *Server) blob(sha string) ([]byte, string, bool) {
	b, ok := s.blobFor(sha)
	if !ok {
		return nil, "", false
	}
	mime := s.mimeFor(sha)
	if mime == "" {
		mime = "image/jpeg"
	}
	return b, mime, true
}

// CreatePanelRequest is what an operator posts to open a review panel.
type CreatePanelRequest struct {
	Question string `json:"question"`
	Context  string `json:"context,omitempty"`
	// ImageBase64 is the artifact reviewers must look at.
	ImageBase64 string `json:"image_base64"`
	Reviewers   int    `json:"reviewers"`
	Agreement   int    `json:"agreement"`
	FeeMinor    int64  `json:"fee_minor"`
	BonusMinor  int64  `json:"bonus_minor"`
	Currency    string `json:"currency,omitempty"`
	TTLSeconds  int64  `json:"ttl_seconds,omitempty"`
	// Parent is the outcome whose fate this panel decides, if any.
	Parent string `json:"parent,omitempty"`
	// Publish lists the panel on the open board instead of pre-issuing links.
	// The two are exclusive: pre-issued links and open seats would each fill
	// the same panel, and between them seat twice as many reviewers as asked
	// for.
	Publish bool `json:"publish,omitempty"`
}

func (s *Server) handleCreatePanel(w http.ResponseWriter, r *http.Request, principal string, body []byte) {
	var in CreatePanelRequest
	if err := json.Unmarshal(body, &in); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request")
		return
	}
	if in.Currency == "" {
		in.Currency = "USD"
	}
	if in.TTLSeconds == 0 {
		in.TTLSeconds = 7200
	}
	spec := outcome.Escalation{
		Parent: orDefault(in.Parent, "(none)"), Question: in.Question,
		Reviewers: in.Reviewers, Agreement: in.Agreement,
		FeeMinor: in.FeeMinor, BonusMinor: in.BonusMinor, Currency: in.Currency,
	}
	if err := spec.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	img, err := decodeBase64(in.ImageBase64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "image is not valid base64")
		return
	}
	art, err := evidence.Analyze(img, "image/jpeg")
	if err != nil {
		writeError(w, http.StatusBadRequest, "image could not be decoded")
		return
	}

	job := fmt.Sprintf("panel-%d", s.now().UnixNano())
	ttl := time.Duration(in.TTLSeconds) * time.Second

	panel := &api.ReviewPanel{
		Job: job, Parent: in.Parent,
		Question: in.Question, Context: in.Context,
		EvidenceSHA: []string{art.SHA256},
		Reviewers:   in.Reviewers, Agreement: in.Agreement,
		FeeMinor: in.FeeMinor, BonusMinor: in.BonusMinor, Currency: in.Currency,
		Expires: s.now().Add(ttl),
	}

	// Published panels hand out no links: reviewers take a seat themselves,
	// and the capability is minted at that moment.
	if in.Publish {
		s.putBlob(art.SHA256, "image/jpeg", img)
		s.Reviews.Add(panel)
		// Parent travels onto the listing so the board can refuse a seat to
		// whoever produced the evidence being judged.
		if err := s.Board.Post(&api.Listing{
			Job: job, Parent: in.Parent, Kind: api.KindReview,
			Title: in.Question, Detail: in.Context,
			PayMinor: in.FeeMinor, BonusMinor: in.BonusMinor, Currency: in.Currency,
			Slots: in.Reviewers, Expires: s.now().Add(ttl), Posted: s.now(),
		}); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSONResponse(w, map[string]any{
			"job": job, "listed": true, "board": s.BaseURL + "/board",
			"evidence_sha": art.SHA256,
			"expires":      s.now().Add(ttl).Format(time.RFC3339),
		})
		return
	}

	secrets := make([]string, 0, in.Reviewers)
	links := make([]string, 0, in.Reviewers)
	for i := 0; i < in.Reviewers; i++ {
		secret, _, err := s.Caps.Issue(job, in.Question,
			[]string{api.ActionView, api.ActionReview}, ttl)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not issue links")
			return
		}
		secrets = append(secrets, secret)
		// The secret goes in the fragment, so it never reaches a server log.
		links = append(links, fmt.Sprintf("%s/r/%s#%s", s.BaseURL, job, secret))
	}

	s.putBlob(art.SHA256, "image/jpeg", img)
	s.setSecrets(job, secrets)

	s.Reviews.Add(panel)

	writeJSONResponse(w, map[string]any{
		"job":          job,
		"links":        links,
		"evidence_sha": art.SHA256,
		"expires":      s.now().Add(ttl).Format(time.RFC3339),
	})
}

// MaxPayoutFor is what a listing could cost if every seat is filled and every
// submission earns its bonus. That is the amount which has to exist up front:
// escrowing the expected cost rather than the maximum means the last worker to
// finish is the one who does not get paid.
func MaxPayoutFor(l *api.Listing) int64 {
	// The attempt fee has to be escrowed or it cannot be paid.
	//
	// It was quoted to buyers in the documentation, charged to nobody, held
	// against nothing, and no code path could ever set the flag that triggers
	// it. Both sides were told a protection existed that did not.
	//
	// Only one of completion and attempt can happen per seat, so the ceiling
	// takes the larger rather than the sum.
	per := l.PayMinor + l.BonusMinor
	if l.AttemptMinor > per {
		per = l.AttemptMinor
	}
	// A staged job's stages sum to PayMinor, checked at posting, so the
	// ceiling is unchanged — every stage paid in full is the whole price.
	return (per + l.ExpenseCapMinor) * int64(l.Slots)
}

// checkFunded refuses to list work the escrow cannot cover.
func (s *Server) checkFunded(l *api.Listing) error {
	if s.Ledger == nil {
		// No ledger means no money anywhere, which is a development setup
		// rather than a funded one. Listing is allowed so the board can be
		// looked at, and nothing can be paid because no rail exists.
		return nil
	}
	need := MaxPayoutFor(l)
	held, err := s.Ledger.Held(context.Background(), l.Job, l.Currency)
	if err != nil {
		return err
	}
	if held < need {
		return fmt.Errorf("escrow holds %d of the %d this could pay out", held, need)
	}
	return nil
}

// CreateTaskRequest is how an agent puts a job into the world.
//
// This is the product. An agent can already read the web; what it cannot do is
// find out whether a sign is actually up on a building this morning, or cause
// a parcel to arrive at a door. Both of those are this one call, and the only
// difference between them is whether the agent is asking about the world or
// asking somebody to change it.
type CreateTaskRequest struct {
	// NotBeforeRFC3339 and NotAfterRFC3339 bound when the work may be done —
	// "somebody has to be home between two and four on Tuesday". Distinct
	// from the TTL, which says when the job stops being worth doing.
	NotBeforeRFC3339 string `json:"not_before,omitempty"`
	NotAfterRFC3339  string `json:"not_after,omitempty"`
	// ProjectID attaches this job to a budget envelope shared with others.
	ProjectID string `json:"project_id,omitempty"`
	// DirectTo sends this job to one of your approved vendors instead of the
	// open board. No auction, no strangers, and it is invisible to everybody
	// else.
	DirectTo string `json:"direct_to,omitempty"`
	// SiteID uses a location from your site list rather than a typed address.
	SiteID string `json:"site_id,omitempty"`
	// Reference is your own purchase order, cost centre or work order. Carried
	// to the receipt untouched.
	Reference string `json:"reference,omitempty"`
	// RequireInsuredToMinor refuses anybody whose verified cover is below this.
	RequireInsuredToMinor int64 `json:"require_insured_to_minor,omitempty"`
	// RequireVetted refuses anybody the exchange has not checked.
	RequireVetted bool `json:"require_vetted,omitempty"`
	// Stages cut a long job into pieces that are each evidenced and paid for.
	// Their pay must add up to fee_minor.
	Stages []api.Stage `json:"stages,omitempty"`
	// WorkHours is how long the work takes, and therefore how long somebody
	// may hold it. An errand needs nothing here; a three-day job does.
	WorkHours int `json:"work_hours,omitempty"`
	// Area is the coarse locality published on the open board. Where is the
	// exact address and is never published — it reaches the claimant only.
	Area string `json:"area,omitempty"`
	// Skills are the qualifications required to take this job. Unknown tags
	// are dropped rather than honoured: a job requiring a skill the exchange
	// has never heard of matches nobody, and looks to the buyer like there is
	// no supply when in fact there is no such skill.
	Skills []api.Skill `json:"skills,omitempty"`
	// Kind is "observe" to find out whether something is true, or "do" to have
	// somebody make it true. Empty means observe, which is the safer default:
	// posting an instruction as an observation wastes money, posting an
	// observation as an instruction could have somebody act on it.
	Kind string `json:"kind,omitempty"`
	// Predicate is what must be true when the job is finished. For an
	// observation it is the question; for a do-job it is the goal.
	Predicate string `json:"predicate"`
	// Instructions is what the worker should actually do. Required for a
	// do-job and meaningless for an observation.
	Instructions string `json:"instructions,omitempty"`
	// Deliverable says what proof is expected back.
	Deliverable string `json:"deliverable,omitempty"`
	Where       string `json:"where,omitempty"`
	Detail      string `json:"detail,omitempty"`

	// FeeMinor is what finishing pays. For an observation it is paid for
	// admissible evidence whichever way the answer turns out, so that honest
	// evidence of "no" pays the same as "yes"; for a do-job it is paid on
	// completion, because there the worker controls the answer.
	FeeMinor int64 `json:"fee_minor"`
	// BonusMinor applies to observations only: the part that depends on the
	// finding.
	BonusMinor int64 `json:"bonus_minor"`
	// AttemptMinor applies to do-jobs only: what a documented failed attempt
	// pays. A worker who travels to a shut shop has still spent the afternoon.
	AttemptMinor int64 `json:"attempt_minor,omitempty"`
	// ExpenseCapMinor is how much they may lay out and reclaim against a
	// receipt. Nobody should be asked to front money with no stated ceiling.
	ExpenseCapMinor int64 `json:"expense_cap_minor,omitempty"`

	// Lat, Lon and RadiusM fence the evidence to a place. Sent as degrees;
	// stored as integers.
	Lat     float64 `json:"lat,omitempty"`
	Lon     float64 `json:"lon,omitempty"`
	RadiusM int64   `json:"radius_m,omitempty"`

	// Pricing is "fixed" (default) or "bids". An open job holds MaxBidMinor
	// and settles at whatever bid is accepted.
	Pricing          string `json:"pricing,omitempty"`
	MaxBidMinor      int64  `json:"max_bid_minor,omitempty"`
	BidsCloseInHours int64  `json:"bids_close_in_hours,omitempty"`
	// Report asks for a structured answer instead of, or alongside, photographs.
	Report []api.ReportField `json:"report,omitempty"`

	// Open text and open questions. See internal/api/brief.go.
	Brief    string        `json:"brief,omitempty"`
	SiteMark *api.SiteMark `json:"site_mark,omitempty"`
	Access   string        `json:"access,omitempty"`
	Unknowns []api.Unknown `json:"unknowns,omitempty"`

	// Multi-part work. See internal/api/scope.go for why a scope's shape has
	// to reach the supply side rather than living only in the agent's plan.
	DependsOn []string `json:"depends_on,omitempty"`
	BidsAsOne bool     `json:"bids_as_one,omitempty"`
	PlanBy    string   `json:"plan_by,omitempty"`

	Currency   string `json:"currency,omitempty"`
	Slots      int    `json:"slots,omitempty"`
	Tier       string `json:"tier,omitempty"`
	TTLSeconds int64  `json:"ttl_seconds,omitempty"`
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request, principal string, body []byte) {
	// An agent key means software wrote this. Workers are told.
	_, _, postedByAgent := s.agents.AuthenticateAgent(r)
	var in CreateTaskRequest
	if err := json.Unmarshal(body, &in); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request")
		return
	}
	if in.Predicate == "" {
		writeError(w, http.StatusBadRequest, "a job needs a predicate")
		return
	}
	if in.FeeMinor <= 0 {
		writeError(w, http.StatusBadRequest, "a job must pay for the work it asks for")
		return
	}
	kind := in.Kind
	if kind == "" {
		kind = api.KindObserve
	}
	if kind != api.KindObserve && kind != api.KindDo {
		writeError(w, http.StatusBadRequest, `kind must be "observe" or "do"`)
		return
	}
	if kind == api.KindDo && in.Instructions == "" {
		writeError(w, http.StatusBadRequest,
			"a job that asks somebody to do something must say what to do")
		return
	}
	// A wasted trip is a do-job idea, and settlement never reads it for an
	// observation. Accepting it here escrowed money against a line that could
	// not pay, and told whoever took the job they were covered for a trip
	// that turned up nothing — when in fact an observation already pays in
	// full for turning up, whichever way the answer goes.
	if kind == api.KindObserve && in.AttemptMinor > 0 {
		writeError(w, http.StatusBadRequest,
			"an observation has no attempt fee: it pays in full for admissible "+
				"evidence whichever way the answer turns out. Put the amount in "+
				"fee_minor, and bonus_minor for the part that depends on the finding")
		return
	}
	if in.Currency == "" {
		in.Currency = "USD"
	}
	if in.TTLSeconds == 0 {
		in.TTLSeconds = 86400
	}
	if in.Slots == 0 {
		in.Slots = 1
	}
	if in.Tier == "" {
		in.Tier = "V2"
	}
	job := fmt.Sprintf("%s-%d", kind, s.now().UnixNano())
	ttl := time.Duration(in.TTLSeconds) * time.Second
	// A poster with no account: the job belongs to a principal only its
	// token can act as, and its escrow arrives by card rather than balance.
	guest := principal == guestSentinel
	if guest {
		if err := s.guestAllowed(r); err != nil {
			writeError(w, http.StatusTooManyRequests, err.Error())
			return
		}
		principal = guestOwner(job)
		postedByAgent = r.Header.Get("X-Lamdis-Posted-By") == "agent"
		if in.Pricing == api.PriceBids {
			writeError(w, http.StatusBadRequest,
				"open bidding needs an account for now: there is no price to authorise a card for until somebody bids")
			return
		}
		if in.ProjectID != "" || in.DirectTo != "" || in.SiteID != "" {
			writeError(w, http.StatusBadRequest,
				"projects, named suppliers and saved sites belong to an account; post a plain job or sign in")
			return
		}
	}

	listing := &api.Listing{
		Job: job, Kind: kind,
		Title: in.Predicate, Where: in.Where, Area: in.Area, Detail: in.Detail,
		NotBefore: parseWhen(in.NotBeforeRFC3339), NotAfter: parseWhen(in.NotAfterRFC3339),
		Instructions: in.Instructions, Deliverable: in.Deliverable,
		PayMinor: in.FeeMinor, BonusMinor: in.BonusMinor,
		AttemptMinor: in.AttemptMinor, ExpenseCapMinor: in.ExpenseCapMinor,
		Currency: in.Currency, Slots: in.Slots, Tier: in.Tier,
		LatE7: int64(in.Lat * 1e7), LonE7: int64(in.Lon * 1e7), RadiusM: in.RadiusM,
		Pricing: in.Pricing, MaxBidMinor: in.MaxBidMinor, Report: in.Report,
		Skills: api.NormalizeSkills(in.Skills),
		Stages: in.Stages, WorkHours: in.WorkHours,
		Owner:     principal,
		ProjectID: in.ProjectID,
		SiteID:    in.SiteID,
		Reference: in.Reference,
		DependsOn: in.DependsOn,
		BidsAsOne: in.BidsAsOne,
		PlanBy:    in.PlanBy,
		Brief:     in.Brief,
		SiteMark:  in.SiteMark,
		Access:    in.Access,
		Unknowns:  in.Unknowns,
		// Recorded from the credential that posted it, so the claim is the
		// exchange's rather than the buyer's.
		PostedByAgent: postedByAgent,
		Expires:       s.now().Add(ttl), Posted: s.now(),
	}
	// Multi-part terms, checked before any money moves.
	//
	// A dependency on a job outside this project would silently never resolve,
	// leaving a listing nobody can ever claim; and both of these only mean
	// anything inside a project, so asking for them without one is a mistake
	// worth naming rather than ignoring.
	if in.PlanBy != "" && in.PlanBy != api.PlanByBuyer && in.PlanBy != api.PlanBySupplier {
		writeError(w, http.StatusBadRequest,
			`plan_by must be "buyer" or "supplier"`)
		return
	}
	if in.PlanBy == api.PlanBySupplier && len(in.Stages) > 0 {
		writeError(w, http.StatusBadRequest,
			"a job whose supplier writes the stages cannot be posted with stages already on it")
		return
	}
	if (len(in.DependsOn) > 0 || in.BidsAsOne) && in.ProjectID == "" {
		writeError(w, http.StatusBadRequest,
			"depends_on and bids_as_one describe a job's place in a project; open one first")
		return
	}
	for _, dep := range in.DependsOn {
		if dep == job {
			writeError(w, http.StatusBadRequest, "a job cannot depend on itself")
			return
		}
		if got, ok := s.Projects.ProjectOf(dep); !ok || got != in.ProjectID {
			writeError(w, http.StatusBadRequest,
				"a job can only depend on another job in the same project")
			return
		}
	}
	// Who, and where — before anything is held.
	//
	// These once ran after the escrow and after the job was already on the
	// board, so a mistyped vendor came back as "insufficient funds" and left
	// an escrow behind for a job that had already listed. Naming a vendor and
	// a site costs nothing; doing it late costs the buyer money and tells them
	// the wrong reason.
	if in.DirectTo != "" {
		if !s.Book.IsApproved(principal, in.DirectTo) {
			writeError(w, http.StatusBadRequest,
				"that supplier is not on your approved vendor list")
			return
		}
		listing.DirectedTo = []string{in.DirectTo}
	}
	if in.RequireInsuredToMinor > 0 || in.RequireVetted {
		listing.Requires = &api.Requirements{
			InsuredToMinor: in.RequireInsuredToMinor,
			Vetted:         in.RequireVetted,
		}
	}
	// A named site supplies the place, so the address is not retyped per job.
	if in.SiteID != "" {
		site, ok := s.Book.Site(principal, in.SiteID)
		if !ok || site.Retired {
			writeError(w, http.StatusBadRequest, "no such live site on your list")
			return
		}
		listing.Where, listing.Area = site.Where, site.Area
		listing.LatE7, listing.LonE7, listing.RadiusM = site.LatE7, site.LonE7, site.RadiusM
		if site.Access != "" {
			listing.Instructions = joinNonEmpty(listing.Instructions, site.Access)
		}
	}

	// What this exchange will not carry.
	//
	// Refused before the job is listed, not after somebody has done it: a
	// worker who completes an abusive task has already taken the risk, and
	// paying them does not undo it.
	if ref := api.Screen(listing.Title, listing.Detail, listing.Instructions,
		listing.Deliverable, listing.Brief, listing.Access); ref != nil {
		if ref.Review {
			writeError(w, http.StatusUnprocessableEntity, ref.Why)
			return
		}
		log.Printf("screen: refused %s job from %s (%s)", listing.Kind, principal, ref.Class)
		writeError(w, http.StatusUnprocessableEntity, ref.Why)
		return
	}
	if ref := api.MassLowValue(listing.Slots, listing.PayMinor); ref != nil {
		writeError(w, http.StatusUnprocessableEntity, ref.Why)
		return
	}

	if in.Pricing == api.PriceBids {
		if in.MaxBidMinor <= 0 {
			writeError(w, http.StatusBadRequest,
				"an open job needs a ceiling, since the price is not known yet")
			return
		}
		hours := in.BidsCloseInHours
		if hours == 0 {
			hours = 24
		}
		listing.BidsCloseAt = s.now().Add(time.Duration(hours) * time.Hour)
		listing.PayMinor = in.MaxBidMinor

		// Reserved, not escrowed.
		//
		// Holding the ceiling made asking a price cost the maximum: comparing
		// three approaches to one garden locked three ceilings. Nobody knows
		// what the work costs yet — that is the entire reason bids exist — so
		// there is nothing sensible to hold. What the buyer must show is that
		// they could pay it, because operators price a job with real effort
		// and soliciting quotes you cannot honour teaches them to ignore open
		// jobs.
		if err := s.canReserve(r.Context(), principal, in.MaxBidMinor,
			orDefault(in.Currency, "USD")); err != nil {
			writeError(w, http.StatusPaymentRequired, err.Error())
			return
		}
	}
	// The envelope, before the money moves.
	//
	// Refused with the number remaining rather than as a failed escrow
	// part-way through a plan: an agent that asks for too much should be told
	// what it has left, not handed an error it cannot act on.
	if in.ProjectID != "" {
		pr, ok := s.Projects.Get(in.ProjectID, principal)
		if !ok {
			writeError(w, http.StatusNotFound, "no such project")
			return
		}
		st := s.projectState(r, pr)
		want := MaxPayoutFor(listing)
		if pr.BudgetMinor > 0 && want > st.RemainingMinor {
			writeJSONResponse(w, map[string]any{
				"error":           "that would exceed the project budget",
				"remaining_minor": st.RemainingMinor,
				"wanted_minor":    want,
				"currency":        pr.Currency,
			})
			return
		}
	}

	// A tier that asks for a location check needs somewhere to check against.
	//
	// V2 is defined as tying evidence to a place. A V2 job with an address and
	// no coordinates asks for a standard it has given us no way to apply, and
	// the submission would pass on a photograph taken anywhere on earth.
	// Checked before any money moves and before the listing exists: it used to
	// run after both, so a refused job left an escrow and a listing behind.
	if listing.Where != "" && listing.RadiusM <= 0 &&
		(listing.Tier == "V2" || listing.Tier == "V3") {
		writeError(w, http.StatusBadRequest,
			"a "+listing.Tier+" job with an address needs lat, lon and radius_m, "+
				"or there is nothing to check the photographs against")
		return
	}
	if guest {
		out, err := s.stagePending(r.Context(), listing)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		if api.AddressInTitle(listing.Title, listing.Where) {
			out["warning"] = "the predicate repeats the street address, and the " +
				"predicate is shown on the open board; put the address in `where`"
		}
		writeJSONResponse(w, out)
		return
	}

	// Hold the money first. If this fails the listing never exists, which is
	// the right order: a worker who completes unfunded work has been defrauded
	// by the exchange, not by a counterparty.
	//
	// An open job is the exception, and only until it is awarded. There is no
	// amount to hold yet — that is what the bids are for — so it carries a
	// reservation instead, and the escrow happens the moment somebody's price
	// is accepted.
	if s.Ledger != nil && listing.Pricing != api.PriceBids {
		if _, err := s.Ledger.Hold(r.Context(),
			"hold-"+job, job, principal, MaxPayoutFor(listing), in.Currency); err != nil {
			writeError(w, http.StatusPaymentRequired, err.Error())
			return
		}
	}
	if listing.Pricing == api.PriceBids {
		s.Reservations.Add(job, principal, in.MaxBidMinor,
			orDefault(in.Currency, "USD"), listing.BidsCloseAt)
	}
	if in.ProjectID != "" {
		if err := s.Projects.Attach(in.ProjectID, job); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
	}
	if err := s.Board.Post(listing); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.setBuyer(job, principal)
	out := map[string]any{
		"job": job, "kind": kind, "board": s.BaseURL + "/board",
		"escrowed": MaxPayoutFor(listing),
		"expires":  s.now().Add(ttl).Format(time.RFC3339),
		"status":   s.BaseURL + "/v1/jobs/" + job,
	}
	if in.ProjectID != "" {
		if pr, ok := s.Projects.Get(in.ProjectID, principal); ok {
			st := s.projectState(r, pr)
			out["project"] = map[string]any{
				"id": pr.ID, "remaining_minor": st.RemainingMinor,
				"committed_minor": st.CommittedMinor, "spent_minor": st.SpentMinor,
			}
		}
	}
	// The title is public. Removing the address from the board counts for
	// nothing if the agent has also written it into the predicate, which is
	// the mistake agents actually make — the same string pasted into both
	// fields.
	//
	// Reported rather than refused: for a sign on a commercial street the
	// address is a public fact and belongs in the title. The caller knows
	// which case this is and we do not.

	if api.AddressInTitle(listing.Title, listing.Where) {
		out["warning"] = "the predicate repeats the street address, and the " +
			"predicate is shown on the open board. If this address is not " +
			"already public, reword it and put the address in `where`, which " +
			"only the person who takes the job can see."
	}
	writeJSONResponse(w, out)
}

// PanelStatus reports a panel's standing to its operator.
func (s *Server) PanelStatus(job string) (api.Tally, bool) {
	if _, ok := s.Reviews.Panel(job); !ok {
		return api.Tally{}, false
	}
	return s.Reviews.Tally(job), true
}

// handleIndex sends people into the product.
//
// This used to serve a second landing page, with the same headline as the
// marketing site. Somebody who had just read the pitch and clicked "open the
// exchange" arrived at the pitch again and reasonably concluded the link was
// broken. The selling happens at lamdis.ai; this host is the thing itself, so
// its front door is the board.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/board", http.StatusFound)
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func writeJSONResponse(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func decodeBase64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

// parseWhen reads an optional timestamp, treating anything unparseable as
// absent.
//
// A malformed window must not become a job nobody can ever do: an unset window
// means "whenever", which is the safe reading of "we could not understand
// this".
func parseWhen(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// verificationBlock is what the receipt says about how sure anybody can be.
//
// The landing page said we "cap how sure any answer claims to be, and say so
// on the receipt". Half of that was true. The cap is real and stricter than
// the sentence implies — with capture unattested the whole tier ladder
// collapses to 0.85, and to 0.72 without capture metadata, so nothing here can
// claim more however good it looks. The receipt said none of it.
//
// What it did say was worse than nothing: a "tier" field carrying the tier the
// buyer *asked for*. A receipt reading V2 when the achievable ceiling was 0.72
// is not a cautious claim, it is a misleading one.
//
// So the receipt now states the ceiling, why it is that number, and — the part
// that matters most — what was and was not established. A reader who wants to
// know whether to believe it should not have to read our source.
func verificationBlock(l *api.Listing, subs []api.Submission) map[string]any {
	hasGeo, markSeen := false, false
	var mark *api.SiteMark
	for _, sub := range subs {
		if sub.SiteMark != nil {
			mark = sub.SiteMark
		}
		if sub.MarkSeen {
			markSeen = true
		}
		for _, a := range sub.Artifacts {
			if a.HasGeo {
				hasGeo = true
			}
		}
	}
	ceiling := verify.Tier(strings.ToUpper(l.Tier)).Ceiling(hasGeo)

	established := []string{
		"the evidence carries a code issued privately for this job, so it was " +
			"made after the job was taken and not before",
		"the evidence has not been submitted here before",
		"the evidence does not score as generated",
	}
	if l.RadiusM > 0 && hasGeo {
		established = append(established,
			"at least one file records a capture location inside the area the job named")
	}
	if mark != nil && markSeen {
		established = append(established,
			"something identifying this property was legible in the evidence: "+mark.Text)
	}

	// The limits, in the same detail as the assurances. A receipt that lists
	// only what it proved is an advertisement.
	limits := []string{
		"no camera signed these images at capture, so nothing here rules out a " +
			"sufficiently determined fabrication. That is why the ceiling is " +
			"below 1 and does not move with the tier",
		"verification answers whether something happened, not whether it was " +
			"done well",
	}
	if l.RadiusM > 0 && hasGeo {
		limits = append(limits,
			"capture location comes from file metadata, which whoever produced "+
				"the file chose what to write. It raises the cost of faking; it "+
				"does not settle location")
	}
	if l.RadiusM > 0 && !hasGeo {
		limits = append(limits, "no file recorded where it was taken")
	}
	if mark == nil && l.TiedToPlace() {
		limits = append(limits,
			"nothing identifies this property in the evidence, so it establishes "+
				"that the work was done somewhere, not that it was done here")
	}
	if mark != nil && !markSeen {
		limits = append(limits,
			"the property mark ("+mark.Text+") was not legible, so the evidence "+
				"is not tied to this address")
	}

	out := map[string]any{
		"confidence_ceiling": ceiling,
		"ceiling_because": "capture is not attested in hardware, so every tier " +
			"collapses to the same ceiling. See limits",
		"tier_requested": l.Tier,
		"established":    established,
		"limits":         limits,
	}
	if mark != nil {
		out["site_mark"] = map[string]any{"text": mark.Text, "seen": markSeen,
			"inferred": mark.Derived}
	}
	// A written report is a different kind of thing, and says so. See
	// receipt_report.go.
	annotateReportReceipt(out, subs)
	return out
}

// frozenFor splits what a worker is owed into money somebody has objected to
// and money that is simply waiting for the next sweep.
//
// One "pending" figure covers three different situations and a person looking
// at this page wants to know which of them they are in: is it coming, is it
// waiting out a window, or has somebody objected. Guessing between those is
// how a page ends up reassuring somebody whose money is frozen.
func (s *Server) frozenFor(worker string) (heldMinor, clearMinor int64) {
	if s.Holdbacks == nil {
		return 0, 0
	}
	now := s.now()
	clearMinor = s.Holdbacks.Available(worker, now)
	for _, h := range s.Holdbacks.Pending(worker, now) {
		if h.Held {
			heldMinor += h.AmountMinor
		}
	}
	return heldMinor, clearMinor
}
