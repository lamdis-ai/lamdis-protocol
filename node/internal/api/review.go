package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// ReviewPanel is one escalated question put to human reviewers.
type ReviewPanel struct {
	// Job is the child outcome's thread id; it is what a capability is scoped to.
	Job string
	// Parent is the outcome whose fate depends on this panel.
	Parent   string
	Question string
	// Context is what the reviewer should know, in plain language.
	Context string
	// EvidenceSHA are the artifacts to look at, content-addressed.
	EvidenceSHA []string
	Reviewers   int
	Agreement   int
	FeeMinor    int64
	BonusMinor  int64
	// Practice marks the demonstration panel: a real page, a real flow, and
	// nothing behind it.
	Practice bool
	Currency string
	Expires  time.Time
}

// Review is one person's answer.
type Review struct {
	// Reviewer identifies who answered: a device principal where the phone
	// could hold a key, otherwise the capability that was used.
	Reviewer string `json:"reviewer"`
	// Finding is the answer to the panel's question.
	Finding bool `json:"finding"`
	// Confident reports whether they felt able to tell at all. An honest "I
	// could not tell" is a real answer and is paid for; it simply does not
	// count toward agreement.
	Confident bool `json:"confident"`
	// Reason is why. A review with no reason is not admissible: it is what
	// separates someone who looked from someone who clicked.
	Reason string `json:"reason"`
	// AttestedBy records how they authenticated.
	AttestedBy string    `json:"attested_by"`
	At         time.Time `json:"at"`
	// Worker is the account the seat was assigned to, which is who the fee is
	// credited to. Reviewer above is how the answer is deduplicated; this is
	// how it is paid. Empty for a pre-issued link nobody claimed from the
	// board, which has no account behind it and cannot be paid.
	Worker string `json:"worker,omitempty"`
}

// Admissible reports whether a review counts as work done.
//
// The bar cannot be raised by making it depend on the answer. Forfeiting pay
// for disagreeing with the majority would look like a fix and is worse than
// the problem: reviewers would start predicting each other instead of looking
// at the picture, and a panel of people guessing the consensus is a beauty
// contest, not a verification. The base fee stays unbiased.
//
// So the bar is on effort rather than on conclusion: a reason with enough
// distinct content that it cannot be produced by holding down a key. This
// stops the laziest extraction and nothing more — it is not, and cannot be,
// the defence against somebody determined to farm reviews. That defence is
// that payment requires an identity able to receive money, and accumulates to
// a threshold before it moves. See Payable.
func (r Review) Admissible() bool {
	reason := strings.TrimSpace(r.Reason)
	if len(reason) < 20 {
		return false
	}
	// Degenerate input — "aaaaaaaa", "........" — has almost no distinct
	// characters. Real sentences have many.
	distinct := map[rune]bool{}
	for _, c := range strings.ToLower(reason) {
		distinct[c] = true
	}
	if len(distinct) < 8 {
		return false
	}
	// And it must contain actual words, not one token repeated.
	words := map[string]bool{}
	for _, w := range strings.Fields(strings.ToLower(reason)) {
		words[w] = true
	}
	return len(words) >= 4
}

// Payable reports whether an admissible review may be paid yet.
//
// Admissibility is about the work; this is about the worker. An anonymous
// reviewer can produce endless admissible reviews at no cost, so money does
// not leave against one until somebody has vouched for an address they
// control — which is checked at the worker level, not here, because the same
// person may hold several capabilities.
//
// This deliberately does not require a device key. The key proves which
// browser produced a submission, which is a statement about evidence; being
// payable is a statement about the person, and requiring the key here would
// pay an enrolled guest while refusing a verified person on an old phone.
func (r Review) Payable(workerVerified bool) bool {
	return r.Admissible() && workerVerified
}

// Tally is the panel's state.
type Tally struct {
	Reviews    []Review `json:"reviews"`
	Admissible int      `json:"admissible"`
	Yes        int      `json:"yes"`
	No         int      `json:"no"`
	Unsure     int      `json:"unsure"`
	// Agreeing is the size of the largest group that answered the same way,
	// counting only reviewers who felt able to tell.
	Agreeing int  `json:"agreeing"`
	Finding  bool `json:"finding"`
	Complete bool `json:"complete"`
	Decided  bool `json:"decided"`
}

// ReviewStore holds panels and the answers they collect.
type ReviewStore struct {
	mu      sync.Mutex
	panels  map[string]*ReviewPanel
	reviews map[string][]Review
	Now     func() time.Time
}

func NewReviewStore() *ReviewStore {
	return &ReviewStore{
		panels: map[string]*ReviewPanel{}, reviews: map[string][]Review{},
		Now: time.Now,
	}
}

func (s *ReviewStore) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *ReviewStore) Add(p *ReviewPanel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.panels[p.Job] = p
}

func (s *ReviewStore) Panel(job string) (*ReviewPanel, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.panels[job]
	return p, ok
}

// Submit records one review. A reviewer may only answer once, and may not
// review a panel they are already counted in.
func (s *ReviewStore) Submit(job string, r Review) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.panels[job]
	if !ok {
		return fmt.Errorf("no such panel")
	}
	if s.now().After(p.Expires) {
		return fmt.Errorf("this panel has closed")
	}
	for _, prior := range s.reviews[job] {
		if prior.Reviewer == r.Reviewer {
			return fmt.Errorf("you have already reviewed this")
		}
	}
	if len(s.reviews[job]) >= p.Reviewers {
		return fmt.Errorf("this panel is already full")
	}
	if !r.Admissible() {
		return fmt.Errorf("please say briefly why — a review without a reason cannot be paid")
	}
	r.At = s.now()
	s.reviews[job] = append(s.reviews[job], r)
	return nil
}

// Tally computes the panel's standing.
func (s *ReviewStore) Tally(job string) Tally {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.panels[job]
	rs := append([]Review(nil), s.reviews[job]...)
	sort.Slice(rs, func(i, j int) bool { return rs[i].At.Before(rs[j].At) })

	t := Tally{Reviews: rs}
	for _, r := range rs {
		if !r.Admissible() {
			continue
		}
		t.Admissible++
		switch {
		case !r.Confident:
			t.Unsure++
		case r.Finding:
			t.Yes++
		default:
			t.No++
		}
	}
	// Agreement counts only reviewers who felt able to tell. Someone who could
	// not tell has not joined either side.
	if t.Yes >= t.No {
		t.Agreeing, t.Finding = t.Yes, true
	} else {
		t.Agreeing, t.Finding = t.No, false
	}
	if p != nil {
		t.Complete = t.Admissible >= p.Reviewers
		t.Decided = t.Agreeing >= p.Agreement
	}
	return t
}

// ReviewServer is the human-facing surface for escalated outcomes.
//
// Every route here is capability-authenticated and lives under its own path
// prefix. The handler signature takes a *Capability rather than a principal
// id, which is what makes it impossible to reach a principal-authenticated
// handler with a capability by mistake.
type ReviewServer struct {
	Caps    *Capabilities
	Reviews *ReviewStore
	Replay  *ReplayGuard
	// Blob returns the bytes of an artifact by content hash.
	Blob func(sha string) ([]byte, string, bool)
	// Secrets returns the candidate secrets for a job. In a real deployment
	// this is backed by the issued-capability table.
	Secrets func(job string) []string
	// Board is where a seat on a panel was assigned, so a recorded review can
	// free it. Without this every review lapsed into an abandonment: the
	// reviewer did the work, the lease ran out, and their standing paid for it.
	Board *Board
	// Settled credits the reviewer for a recorded review. It returns what was
	// credited, in minor units, so the page can say something true. Nil means
	// nobody is paid, which the page then says instead.
	Settled func(job, worker string, r Review) (paidMinor int64, err error)
	Now     func() time.Time
}

func (s *ReviewServer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Register mounts the reviewer surface.
func (s *ReviewServer) Register(mux *http.ServeMux) {
	// The page itself is unauthenticated: the secret is in the URL fragment,
	// which the browser never sends. The page reads it and authenticates the
	// API calls it makes.
	mux.HandleFunc("GET /r/{job}", s.handlePage)

	mux.HandleFunc("GET /v1/claims/{job}", s.withCapability(ActionView, s.handleBrief))
	mux.HandleFunc("POST /v1/claims/{job}/review", s.withCapability(ActionReview, s.handleReview))
	mux.HandleFunc("GET /v1/claims/{job}/evidence/{sha}", s.withCapability(ActionView, s.handleEvidence))
}

// withCapability authenticates a capability request. Note the callback's
// signature: it receives a *Capability, never a principal id, so a capability
// cannot be threaded into a handler that expects a real principal.
func (s *ReviewServer) withCapability(
	action string,
	next func(w http.ResponseWriter, r *http.Request, c *Capability, body []byte),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := readBody(r)
		if err != nil {
			refuse(w)
			return
		}
		// Replay protection applies here exactly as it does to signed
		// requests: a captured review submission must not count twice.
		if s.Replay != nil && !s.Replay.Check(r.Header.Get(hdrCapability)) {
			refuse(w)
			return
		}
		c, err := s.Caps.authenticate(r, body, s.now(), s.Secrets)
		if err != nil || c.Job != r.PathValue("job") || !c.Can(action) {
			// One generic refusal for every failure: no oracle telling a
			// prober which part they got wrong.
			refuse(w)
			return
		}
		next(w, r, c, body)
	}
}

func (s *ReviewServer) handleBrief(w http.ResponseWriter, r *http.Request, c *Capability, _ []byte) {
	p, ok := s.Reviews.Panel(c.Job)
	if !ok {
		refuse(w)
		return
	}
	t := s.Reviews.Tally(c.Job)
	writeJSON(w, map[string]any{
		"question":    p.Question,
		"context":     p.Context,
		"evidence":    p.EvidenceSHA,
		"reviewers":   p.Reviewers,
		"agreement":   p.Agreement,
		"fee_minor":   p.FeeMinor,
		"bonus_minor": p.BonusMinor,
		"currency":    p.Currency,
		"received":    t.Admissible,
		// Whether this is the demonstration panel. A page that looks identical
		// to a real one, judging a drawing rather than a place, teaches
		// somebody that the whole exchange is theatre.
		"practice":    p.Practice,
		"attested_by": c.Attestation(),
		"expires":     p.Expires.Format(time.RFC3339),
	})
}

func (s *ReviewServer) handleReview(w http.ResponseWriter, r *http.Request, c *Capability, body []byte) {
	var in struct {
		Finding   bool   `json:"finding"`
		Confident bool   `json:"confident"`
		Reason    string `json:"reason"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		refuse(w)
		return
	}
	reviewer := c.DevicePrincipal
	if reviewer == "" {
		reviewer = "cap:" + c.Holder[:16]
	}
	// The account behind the seat, when the seat came from the board.
	var worker string
	if s.Board != nil {
		worker, _ = s.Board.WorkerFor(c.Holder)
	}
	review := Review{
		Reviewer: reviewer, Finding: in.Finding, Confident: in.Confident,
		Reason: in.Reason, AttestedBy: c.Attestation(), Worker: worker,
	}
	err := s.Reviews.Submit(c.Job, review)
	if err != nil {
		// The reviewer is a person being asked to do something, so this is the
		// one place a specific message is worth more than a generic refusal.
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}
	p, _ := s.Reviews.Panel(c.Job)
	practice := p != nil && p.Practice

	// The review is the work. Recording it and leaving the seat to lapse
	// charged the reviewer an abandonment for finishing: forty-five minutes
	// later they were in cooldown, and after three reviews their allowance
	// was one forever. Free the seat the same way a submitted job does.
	if s.Board != nil && worker != "" {
		if practice {
			s.Board.DoneUnscored(c.Job, worker)
		} else {
			s.Board.Done(c.Job, worker)
		}
	}
	// And pay for it. The page promised money with the next payout; until this
	// existed nothing credited anybody for a review.
	var paid int64
	if s.Settled != nil && worker != "" && !practice {
		if n, err := s.Settled(c.Job, worker, review); err != nil {
			log.Printf("review: %s by %s recorded but not settled: %v", c.Job, worker, err)
		} else {
			paid = n
		}
	}
	t := s.Reviews.Tally(c.Job)
	writeJSON(w, map[string]any{
		"recorded": true, "received": t.Admissible, "complete": t.Complete,
		// What actually happened to the money, so the page does not have to
		// guess. Zero on a real panel means the fee is owed and not yet
		// credited — the earnings page is the source of truth.
		"paid_minor": paid, "practice": practice,
		"payable": worker != "" && !practice,
	})
}

func (s *ReviewServer) handleEvidence(w http.ResponseWriter, r *http.Request, c *Capability, _ []byte) {
	p, ok := s.Reviews.Panel(c.Job)
	if !ok {
		refuse(w)
		return
	}
	sha := r.PathValue("sha")
	permitted := false
	for _, s := range p.EvidenceSHA {
		if s == sha {
			permitted = true
			break
		}
	}
	if !permitted || s.Blob == nil {
		refuse(w)
		return
	}
	data, mediaType, ok := s.Blob(sha)
	if !ok {
		refuse(w)
		return
	}
	// A hostile upload must never execute in this origin.
	switch mediaType {
	case "image/jpeg", "image/png", "image/webp":
	default:
		refuse(w)
		return
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", sha[:12]+".jpg"))
	w.Write(data)
}

func (s *ReviewServer) handlePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Referrer-Policy", "no-referrer")
	fmt.Fprint(w, reviewPageHTML)
}

func refuse(w http.ResponseWriter) {
	// The same answer for not-found and not-authorized, matching the node's
	// existing discipline: an error must not tell a prober what exists.
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`{"error":"rejected"}`))
}

func readBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	defer r.Body.Close()
	return readAllLimited(r.Body, maxBody)
}
