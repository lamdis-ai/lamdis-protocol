package exchange

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Hearing from the payment provider directly.
//
// A deposit was credited only when the buyer came back to the page. Close the
// tab after paying and the money arrived at the provider and never reached the
// balance — and a bank debit, which settles days later and returns nobody to
// any page, could not work at all. That is the payment method the economics
// here depend on, because a flat card fee eats a small top-up.
//
// CreditDeposit already carried a comment saying it is "called from the rail's
// webhook, never from a request the payer controls". Nothing called it that
// way. This is the caller that comment describes.

// StripeWebhook receives events from the payment provider.
type StripeWebhook struct {
	Server *Server
	// Secret is the endpoint's signing secret, from the provider. Without it
	// nothing is accepted: an unauthenticated endpoint that credits balances
	// is a way to mint money by POSTing JSON.
	Secret string
	// Tolerance bounds how old a signed event may be, so a captured delivery
	// cannot be replayed indefinitely.
	Tolerance time.Duration
	Now       func() time.Time
}

// NewStripeWebhook builds the receiver from the environment, or returns nil
// when no signing secret is configured.
//
// Nil disables the endpoint rather than mounting one that trusts anybody.
func NewStripeWebhook(s *Server) *StripeWebhook {
	secret := strings.TrimSpace(os.Getenv("STRIPE_WEBHOOK_SECRET"))
	if secret == "" {
		return nil
	}
	return &StripeWebhook{
		Server: s, Secret: secret,
		Tolerance: 5 * time.Minute,
		Now:       time.Now,
	}
}

func (h *StripeWebhook) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

// Register mounts the endpoint.
func (h *StripeWebhook) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/stripe/webhook", h.handle)
}

func (h *StripeWebhook) handle(w http.ResponseWriter, r *http.Request) {
	// Bounded read: this endpoint is public, and an unbounded one is a way to
	// spend the exchange's memory from the outside.
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "unreadable", http.StatusBadRequest)
		return
	}
	if err := h.verify(r.Header.Get("Stripe-Signature"), body); err != nil {
		// Deliberately terse. An endpoint that explains why a forgery failed
		// is a tool for producing one that does not.
		http.Error(w, "rejected", http.StatusBadRequest)
		return
	}

	var ev struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Data struct {
			Object json.RawMessage `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &ev); err != nil {
		http.Error(w, "unparseable", http.StatusBadRequest)
		return
	}

	if err := h.apply(r, ev.ID, ev.Type, ev.Data.Object); err != nil {
		// A 500 asks the provider to redeliver, which is what we want when the
		// failure is ours. Anything already applied is caught by the ledger's
		// idempotency on the event id, so a redelivery credits once.
		log.Printf("webhook: %s (%s): %v", ev.Type, ev.ID, err)
		http.Error(w, "retry", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"received":true}`))
}

// apply acts on one event.
//
// Unknown types are acknowledged rather than refused: the provider sends what
// the account is subscribed to, and answering an unrecognised event with an
// error makes it retry forever.
func (h *StripeWebhook) apply(r *http.Request, eventID, kind string, obj json.RawMessage) error {
	switch kind {
	case "checkout.session.completed", "checkout.session.async_payment_succeeded":
		// A session opened for a job rather than a balance carries the job
		// in its metadata: that is an authorisation to list, not a deposit.
		var card struct {
			ID            string `json:"id"`
			PaymentIntent string `json:"payment_intent"`
			AmountTotal   int64  `json:"amount_total"`
			Metadata      struct {
				Job string `json:"lamdis_job"`
			} `json:"metadata"`
			CustomerDetails struct {
				Email string `json:"email"`
			} `json:"customer_details"`
		}
		if err := json.Unmarshal(obj, &card); err == nil && card.Metadata.Job != "" {
			return h.Server.FundFromCard(r.Context(), card.Metadata.Job, card.ID,
				card.PaymentIntent, card.AmountTotal, card.CustomerDetails.Email)
		}
		return h.creditCheckout(r, eventID, obj)
	case "checkout.session.async_payment_failed":
		// Nothing to undo: nothing was credited until it succeeded. Logged so
		// a buyer asking why their balance did not move can be answered.
		log.Printf("webhook: a delayed payment failed (%s)", eventID)
		return nil
	default:
		return nil
	}
}

// creditCheckout puts a cleared payment into the buyer's balance.
func (h *StripeWebhook) creditCheckout(r *http.Request, eventID string, obj json.RawMessage) error {
	var sess struct {
		ID                string `json:"id"`
		PaymentStatus     string `json:"payment_status"`
		AmountTotal       int64  `json:"amount_total"`
		Currency          string `json:"currency"`
		ClientReferenceID string `json:"client_reference_id"`
		Metadata          struct {
			Person string `json:"lamdis_person"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(obj, &sess); err != nil {
		return fmt.Errorf("unparseable session: %w", err)
	}
	// Only money that actually cleared. A completed session is not a paid one
	// when the method settles later.
	if sess.PaymentStatus != "paid" {
		return nil
	}
	person := sess.ClientReferenceID
	if person == "" {
		person = sess.Metadata.Person
	}
	if person == "" || sess.AmountTotal <= 0 {
		// Nothing to credit and nobody to credit it to. Acknowledged so it is
		// not retried forever; logged because it should not happen.
		log.Printf("webhook: paid session %s names no account", sess.ID)
		return nil
	}
	currency := strings.ToUpper(sess.Currency)
	if currency == "" {
		currency = "USD"
	}
	// Keyed on the provider's event id, so a redelivery credits once.
	return h.Server.CreditDeposit(r.Context(), eventID, person,
		sess.AmountTotal, currency, sess.ID)
}

// verify checks the provider actually sent this.
//
// The scheme signs the timestamp and the raw body together, so the body cannot
// be edited and an old delivery cannot be replayed once it falls outside the
// tolerance. Compared in constant time, because a comparison that returns
// early leaks how much of a forged signature was right.
func (h *StripeWebhook) verify(header string, body []byte) error {
	if h.Secret == "" {
		return fmt.Errorf("no signing secret")
	}
	var ts string
	var sigs []string
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			ts = v
		case "v1":
			sigs = append(sigs, v)
		}
	}
	if ts == "" || len(sigs) == 0 {
		return fmt.Errorf("malformed signature header")
	}
	secs, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return fmt.Errorf("malformed timestamp")
	}
	age := h.now().Sub(time.Unix(secs, 0))
	if age < 0 {
		age = -age
	}
	tolerance := h.Tolerance
	if tolerance <= 0 {
		tolerance = 5 * time.Minute
	}
	if age > tolerance {
		return fmt.Errorf("outside the tolerance")
	}

	mac := hmac.New(sha256.New, []byte(h.Secret))
	mac.Write([]byte(ts))
	mac.Write([]byte("."))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))

	// Several signatures may be present while a secret is being rotated; any
	// one matching is the provider.
	for _, got := range sigs {
		if hmac.Equal([]byte(got), []byte(want)) {
			return nil
		}
	}
	return fmt.Errorf("no signature matched")
}
