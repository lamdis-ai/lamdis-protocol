package exchange

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
)

// Telling both sides what happened when nothing did.
//
// An empty marketplace fails quietly on both sides and the silence is the
// failure. A worker sees a blank board and leaves, with nothing able to reach
// them when work appears. A buyer's job sits unclaimed, their money locked,
// and the only feedback is a string on a page they have to think to visit —
// after which it refunds with no explanation of why or whether to try again.
//
// Neither of those is a missing feature so much as a missing sentence.

// Watch is somebody asking to be told when work appears.
type Watch struct {
	Person string `json:"person"`
	Email  string `json:"-"`
	// Quiet stops the alerts without losing the settings.
	Quiet bool `json:"quiet"`
	// LastSent bounds how often somebody hears from us. A board that fills up
	// must not turn into a mailbox that does.
	LastSent time.Time `json:"last_sent,omitempty"`
}

// Watches records who wants telling.
type Watches struct {
	mu  sync.Mutex
	by  map[string]*Watch
	Now func() time.Time
}

// NewWatches builds an empty store.
func NewWatches() *Watches { return &Watches{by: map[string]*Watch{}} }

func (w *Watches) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now()
}

// Set records that somebody wants to hear about new work.
func (w *Watches) Set(person, email string, quiet bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	x, ok := w.by[person]
	if !ok {
		x = &Watch{Person: person}
		w.by[person] = x
	}
	x.Email, x.Quiet = email, quiet
}

// Get returns somebody's setting.
func (w *Watches) Get(person string) (Watch, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	x, ok := w.by[person]
	if !ok {
		return Watch{}, false
	}
	return *x, true
}

// MinBetweenAlerts is how rarely one person hears from us.
//
// Generous on purpose. The failure this exists to fix is somebody never
// hearing that work appeared; the failure it must not cause is somebody
// muting us because a busy Tuesday sent nine emails.
const MinBetweenAlerts = 6 * time.Hour

// Due lists who should be told about a job, respecting their settings and how
// recently they last heard from us.
func (w *Watches) Due(eligible []string) []Watch {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := w.now()
	var out []Watch
	for _, p := range eligible {
		x, ok := w.by[p]
		if !ok || x.Quiet || x.Email == "" {
			continue
		}
		if now.Sub(x.LastSent) < MinBetweenAlerts {
			continue
		}
		x.LastSent = now
		out = append(out, *x)
	}
	return out
}

// AlertNewWork tells the people who could take a job that it exists.
//
// Called from the same hook that dispatches to webhooks, so a person with a
// phone gets the same signal a fleet with an endpoint does. Best effort: a
// failure to send must never affect whether the job lists.
func (s *Server) AlertNewWork(l *api.Listing) {
	if s.Mail == nil || s.Watches == nil || l == nil || !api.IsWork(l.Kind) {
		return
	}
	if l.Practice {
		return // nobody needs an email about a practice run
	}
	// Only people who could actually take it. An alert about work somebody is
	// not qualified for or cannot reach is worse than no alert.
	var eligible []string
	for worker, cap := range s.Capacities.All() {
		if !cap.Accepting || !cap.Takes(l.Kind) {
			continue
		}
		if !api.MeetsSkills(l.Skills, cap.Skills) {
			continue
		}
		if !api.InRange(l.LatE7, l.LonE7, cap.LatE7, cap.LonE7, cap.RangeMiles) {
			continue
		}
		eligible = append(eligible, worker)
	}
	due := s.Watches.Due(eligible)
	// The register is the other half of who could take this. Somebody who
	// said where they work but has not signed in is exactly the person this
	// job was posted near, and telling only the account holders would leave
	// them out of the one thing they were promised.
	if len(due) == 0 && (s.Coverage == nil || s.Coverage.Count() == 0) {
		return
	}
	base := trimSlash(s.BaseURL)
	subject := "Work near you: " + l.Title
	body := fmt.Sprintf(
		"%s\n\n%s\n\nPays %s. %s\n\nTake it: %s/board\n\n"+
			"To stop these, turn off alerts in your console: %s/console\n",
		l.Title, orDash(l.Area), money(l.PayMinor, l.Currency),
		orDash(l.Window()), base, base)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		sent := 0
		for _, x := range due {
			if err := s.Mail.Send(ctx, x.Email, subject, body); err != nil {
				log.Printf("alert: could not reach %s: %v", x.Person, err)
				continue
			}
			sent++
		}
		if sent > 0 {
			log.Printf("alert: told %d operator(s) about %s", sent, l.Job)
		}
		s.AlertCoverage(ctx, l)
	}()
}

// AlertStaleJobs tells buyers that their work is not being taken, and why.
//
// A job that fails silently and then refunds teaches a buyer that the exchange
// does not work. A job that says "nobody within twenty-five miles takes this
// kind of work" is a problem they can do something about.
func (s *Server) AlertStaleJobs(ctx context.Context) int {
	if s.Mail == nil {
		return 0
	}
	now := s.now()
	told := 0
	for _, l := range s.Board.All() {
		if !api.IsWork(l.Kind) || l.Practice || l.Sandbox || l.Cancelled || l.Owner == "" {
			continue
		}
		if l.Taken > 0 || l.Expires.Before(now) {
			continue
		}
		// A third of the way to expiry with nobody on it is when a buyer can
		// still act — earlier is noise, later is a post-mortem.
		life := l.Expires.Sub(l.Posted)
		if life <= 0 || now.Sub(l.Posted) < life/3 {
			continue
		}
		if s.staleTold == nil {
			s.staleTold = map[string]bool{}
		}
		if s.staleTold[l.Job] {
			continue
		}
		email, ok := s.emailFor(l.Owner)
		if !ok {
			continue
		}
		reach, advice := s.reachFor(QuoteRequest{
			Kind: l.Kind, Skills: l.Skills,
			Lat: api.Deg(l.LatE7), Lon: api.Deg(l.LonE7), Slots: l.Slots,
		})
		why := "Nobody has taken it yet."
		if reach == 0 {
			why = "Nobody set up for this work is currently within range of it."
			if len(advice) > 0 {
				why += " " + advice[0]
			}
		}
		base := trimSlash(s.BaseURL)
		body := fmt.Sprintf(
			"Your job is still open.\n\n%s\n\n%s\n\nIt expires %s, and anything "+
				"not earned is returned to your balance automatically.\n\n"+
				"You can cancel it now and get the money back straight away, or "+
				"change what it asks for: %s/v1/jobs/%s\n",
			l.Title, why, l.Expires.Format("Mon 2 Jan at 3:04pm"), base, l.Job)
		if err := s.Mail.Send(ctx, email, "Still waiting: "+l.Title, body); err != nil {
			continue
		}
		s.staleTold[l.Job] = true
		told++
	}
	return told
}

// StartAlerts checks for unfilled work on a timer.
func (s *Server) StartAlerts(ctx context.Context, every time.Duration) {
	if s.Mail == nil {
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
				if n := s.AlertStaleJobs(ctx); n > 0 {
					log.Printf("alert: told %d buyer(s) their work is unfilled", n)
				}
			}
		}
	}()
}

// AlertServer lets somebody turn alerts on and off.
type AlertServer struct {
	Server  *Server
	Workers *api.Workers
}

func (as *AlertServer) Register(mux *http.ServeMux) {
	mux.HandleFunc("PUT /v1/alerts", as.handleSet)
	mux.HandleFunc("GET /v1/alerts", as.handleGet)
}

func (as *AlertServer) handleGet(w http.ResponseWriter, r *http.Request) {
	worker, ok := as.who(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "sign in first")
		return
	}
	x, _ := as.Server.Watches.Get(worker.ID)
	writeJSONResponse(w, map[string]any{
		"alerts_on": !x.Quiet && x.Email != "",
		"available": as.Server.Mail != nil,
		"note": "we will tell you when work appears that you could actually " +
			"take, at most once every few hours.",
	})
}

func (as *AlertServer) handleSet(w http.ResponseWriter, r *http.Request) {
	worker, ok := as.who(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "sign in first")
		return
	}
	on := r.URL.Query().Get("on") != "false"
	as.Server.Watches.Set(worker.ID, worker.Email, !on)
	writeJSONResponse(w, map[string]any{"alerts_on": on})
}

func (as *AlertServer) who(r *http.Request) (*api.Worker, bool) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	x, err := as.Workers.Authenticate(r, body, as.Server.now())
	if err != nil {
		return nil, false
	}
	return x, true
}

func orDash(s string) string {
	if s == "" {
		return ""
	}
	return s
}

func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

// emailFor finds a buyer's address so their unfilled job can be explained to
// them. Absent is not an error: an account without a reachable address simply
// does not get told.
func (s *Server) emailFor(person string) (string, bool) {
	// A job posted without an account: the only address is the one the
	// payment rail reported for the card.
	if isGuest(person) {
		if l, ok := s.Board.Get(guestJob(person)); ok && l.Funding != nil && l.Funding.Email != "" {
			return l.Funding.Email, true
		}
		return "", false
	}
	if s.Workers == nil {
		return "", false
	}
	w, ok := s.Workers.Get(person)
	if !ok || w.Email == "" {
		return "", false
	}
	return w.Email, true
}
