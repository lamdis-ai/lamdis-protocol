package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"
)

// The console is where a worker finds out where they stand.
//
// Until now a person could do work and then have no way to learn what it
// earned, whether it passed, or why they had not been paid. That is the state
// most gig platforms leave people in, and it is the fastest way to lose the
// supply side: somebody who cannot see their own money assumes there isn't any.
//
// Three questions, answered plainly: what have I earned, what is stopping it
// reaching me, and what did I actually do.
type Console struct {
	Workers *Workers
	Board   *Board
	// Earnings reports what a worker has been credited and paid, from the
	// ledger. Nil leaves the figures at zero rather than inventing them.
	Earnings func(worker string) (earned, paid int64, currency string)
	// TaxStatus reports where somebody stands against the calendar-year
	// reporting threshold. Shown before they cross it, because collecting
	// details afterwards means holding their money while they find a document.
	TaxStatus func(worker string) any
	// PayoutStatus reports how far through payout setup this worker is.
	PayoutStatus func(worker string) PayoutState
	// History lists what they submitted.
	History func(worker string) []WorkRecord
	// Bids lists their open offers, so a bid does not disappear once placed.
	Bids func(worker string) []OpenBid
	// Frozen reports earnings a buyer has objected to and the date each must
	// be decided by. Nil means the exchange keeps no holdbacks, and the figure
	// is reported as zero rather than guessed at.
	Frozen func(worker string) (heldMinor, clearMinor int64)
	// PayoutThresholdMinor is what must accumulate before a transfer is made.
	PayoutThresholdMinor int64
	Now                  func() time.Time
}

// PayoutState is the honest answer to "when do I get paid".
type PayoutState struct {
	// Unavailable means this exchange has no payment rail at all, so there is
	// nothing for the worker to go and set up. It is distinct from "not
	// connected", which is their move to make.
	Unavailable bool `json:"unavailable,omitempty"`
	// Connected is whether they have an account at the payment rail at all.
	Connected bool `json:"connected"`
	// Ready is whether that account can actually receive money. The two differ
	// for days while identity checks run, and saying so is better than a
	// spinner.
	Ready bool `json:"ready"`
	// Needs lists what the rail is still waiting for, in its own words.
	Needs []string `json:"needs,omitempty"`
	// OnboardingURL sends them to finish it.
	OnboardingURL string `json:"onboarding_url,omitempty"`
}

// OpenBid is an offer this worker has out, and where it stands.
type OpenBid struct {
	Job         string    `json:"job"`
	Title       string    `json:"title"`
	Where       string    `json:"where,omitempty"`
	AmountMinor int64     `json:"amount_minor"`
	Currency    string    `json:"currency"`
	Note        string    `json:"note,omitempty"`
	Placed      time.Time `json:"placed"`
	ClosesAt    time.Time `json:"closes_at,omitempty"`
	Won         bool      `json:"won,omitempty"`
	// Status is the sentence shown to the worker, not a code they have to
	// look up.
	Status string `json:"status"`
}

// WorkRecord is one thing a worker did.
type WorkRecord struct {
	Job         string    `json:"job"`
	Kind        string    `json:"kind"`
	Title       string    `json:"title"`
	At          time.Time `json:"at"`
	Status      string    `json:"status"`
	AmountMinor int64     `json:"amount_minor"`
	Currency    string    `json:"currency"`
	// Why explains a rejection, so somebody can do better next time rather
	// than guessing.
	Why string `json:"why,omitempty"`
}

func (c *Console) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Console) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /console", c.handlePage)
	mux.HandleFunc("GET /v1/me", c.handleMe)
}

func (c *Console) handlePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Referrer-Policy", "no-referrer")
	page := consolePageHTML
	// ?demo=1 renders the cockpit with labelled sample figures so the layout
	// can be looked at without an account. Only on an exchange with no
	// identity provider configured — a development box — and never where
	// real people sign in, because a page of invented money next to a real
	// sign-in is a page somebody will one day believe.
	if r.URL.Query().Get("demo") == "1" && os.Getenv("LAMDIS_COGNITO_POOL") == "" {
		page = strings.Replace(page, "var DEMO_ALLOWED = false;", "var DEMO_ALLOWED = true;", 1)
	}
	w.Write([]byte(page))
}

// handleMe answers every question the console asks, in one call.
func (c *Console) handleMe(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		refuse(w)
		return
	}
	worker, err := c.Workers.Authenticate(r, body, c.now())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{
			"error": "sign in to see your account", "signin": "/signin",
		})
		return
	}

	var earned, paid int64
	currency := "USD"
	if c.Earnings != nil {
		earned, paid, currency = c.Earnings(worker.ID)
	}
	var payout PayoutState
	if c.PayoutStatus != nil {
		payout = c.PayoutStatus(worker.ID)
	}
	var history []WorkRecord
	if c.History != nil {
		history = c.History(worker.ID)
	}
	var bids []OpenBid
	if c.Bids != nil {
		bids = c.Bids(worker.ID)
	}

	pending := earned - paid
	// Pending is one number covering three different situations, and the
	// difference is the whole question somebody has when they look at this
	// page: is my money coming, is it waiting out a window, or has somebody
	// objected to it. Split so the page can say which.
	var held, clear int64
	if c.Frozen != nil {
		held, clear = c.Frozen(worker.ID)
	}
	// What this account may still take on, which is the other half of the
	// answer to "what can I do next". See assurance.go.
	var ceiling, exposure int64
	tier := ""
	if c.Board != nil {
		standing := c.Board.StandingFor(worker.ID)
		ceiling = ValueCeiling(standing)
		exposure = c.Board.ExposureOf(worker.ID)
		tier = tierName(standing)
	}
	room := ceiling - exposure
	if room < 0 {
		room = 0
	}
	// The single most useful sentence on the page: why the money has not
	// arrived. Every branch here is a real state somebody will be in.
	var blocked string
	switch {
	case !worker.Verified:
		blocked = "Your account is not verified yet."
	case payout.Unavailable:
		// Nothing to act on, so do not tell them to act.
		blocked = "Payouts are not switched on yet. Your earnings are recorded and " +
			"will be paid once they are."
	case !payout.Connected:
		blocked = "Add a payout account to receive what you have earned. " +
			"It takes about two minutes and happens on the payment provider's " +
			"own pages — we never see your bank details."
	case !payout.Ready && len(payout.Needs) > 0:
		blocked = "The payment provider still needs " + joinWords(payout.Needs) + "."
	case !payout.Ready:
		blocked = "Your payout account is still being checked by the payment provider."
	case pending <= 0:
		blocked = ""
	case c.PayoutThresholdMinor > 0 && pending < c.PayoutThresholdMinor:
		blocked = "Earnings are paid out once they reach " +
			minor(c.PayoutThresholdMinor, currency) + "."
	}

	writeWork(w, http.StatusOK, map[string]any{
		"can_connect_payout": !payout.Unavailable && !payout.Ready,
		"tax": func() any {
			if c.TaxStatus == nil {
				return nil
			}
			return c.TaxStatus(worker.ID)
		}(),
		"worker":           worker.ID,
		"verified":         worker.Verified,
		"enrolled":         worker.Enrolled,
		"earned_minor":     earned,
		"paid_minor":       paid,
		"pending_minor":    pending,
		"held_minor":       held,
		"clear_minor":      clear,
		"ceiling_minor":    ceiling,
		"exposure_minor":   exposure,
		"room_minor":       room,
		"tier":             tier,
		"currency":         currency,
		"payout":           payout,
		"payout_threshold": c.PayoutThresholdMinor,
		"blocked":          blocked,
		"history":          history,
		"bids":             bids,
	})
}

// tierName is the word for where an account's ceiling comes from, so the
// console can say "proven" next to the figure rather than leaving somebody to
// work out from a dollar amount what the exchange thinks of them. It follows
// ValueCeiling exactly; a name that disagreed with the number would be worse
// than no name.
func tierName(s Standing) string {
	switch ValueCeiling(s) {
	case ShakenCeilingMinor:
		return "shaken"
	case VettedCeilingMinor:
		return "vetted"
	case EstablishedCeilingMinor:
		return "established"
	case ProvenCeilingMinor:
		return "proven"
	default:
		return "new"
	}
}

// joinWords renders a list the way a person would say it.
func joinWords(ss []string) string {
	switch len(ss) {
	case 0:
		return ""
	case 1:
		return ss[0]
	case 2:
		return ss[0] + " and " + ss[1]
	default:
		return strings.Join(ss[:len(ss)-1], ", ") + ", and " + ss[len(ss)-1]
	}
}
