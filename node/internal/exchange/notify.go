package exchange

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
)

// Telling people what happened to their money.
//
// The two alerts that existed covered the empty board: work appeared, or work
// was not being taken. Everything after that was silent. A worker's evidence
// was accepted, or refused with a reason they never saw because they had
// already walked away from the page; a buyer's money was held for a day so
// they could look at what came back, and nothing told them to look; an
// objection froze somebody's earnings and they found out when the payout did
// not arrive. Every one of those is a sentence, and the mail path already
// existed to carry it.
//
// Same rules as alerts.go. Nothing is sent when mail is not configured. Nobody
// is written to who has no address or who asked for quiet. A failure to send
// never changes what happened to the job, and no event is sent twice.

// notifier remembers what has been said, so a retry, a second sweep or a
// second click does not say it again.
type notifier struct {
	seen sync.Map
	// wg lets a test wait for the messages a hook sent in the background.
	wg sync.WaitGroup
}

// tell sends one message to one person, once per key. Best effort and off the
// request path: whatever called it has already done the thing being reported.
func (s *Server) tell(key, person, subject, body string) {
	if s.Mail == nil {
		return
	}
	email, ok := s.reach(person)
	if !ok {
		return
	}
	if _, dup := s.notify.seen.LoadOrStore(key, true); dup {
		return
	}
	s.notify.wg.Add(1)
	go func() {
		defer s.notify.wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := s.Mail.Send(ctx, email, subject, body); err != nil {
			// Forget it, so the next lifecycle pass can try again.
			s.notify.seen.Delete(key)
			log.Printf("notify: could not reach %s about %s: %v", person, key, err)
		}
	}()
}

// reach finds somebody's address, honouring the switch they were given.
//
// Alerts are turned off in one place, and "off" has to mean every email — a
// person who said stop and then hears from us about something else has not
// been listened to.
func (s *Server) reach(person string) (string, bool) {
	if person == "" {
		return "", false
	}
	if s.Watches != nil {
		if x, ok := s.Watches.Get(person); ok {
			if x.Quiet {
				return "", false
			}
			if x.Email != "" {
				return x.Email, true
			}
		}
	}
	return s.emailFor(person)
}

// base is the public origin with no trailing slash.
func (s *Server) base() string { return trimSlash(s.BaseURL) }

// when renders a time the way it would be said.
func when(t time.Time) string { return t.Format("Mon 2 Jan at 3:04pm") }

// short is a stable, compact tag for a sentence, so a second refusal for a
// different reason is a different event and the same reason is not.
func short(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:4])
}

// stageKey distinguishes the pieces of a staged job.
func stageKey(sub api.Submission) string {
	return fmt.Sprintf("%s:%s#%d", sub.Job, sub.Holder, sub.Stage)
}

// whatFor names the piece of work a submission is about.
func whatFor(l *api.Listing, sub api.Submission) string {
	if sub.StageName != "" {
		return fmt.Sprintf("%s (%s)", l.Title, sub.StageName)
	}
	return l.Title
}

// resumeFor is the worker's own page for a job they still hold, or "".
func (s *Server) resumeFor(job, worker string) string {
	if s.Board == nil || worker == "" {
		return ""
	}
	for _, h := range s.Board.HeldBy(worker) {
		if h.Job == job {
			return s.base() + h.Resume
		}
	}
	return ""
}

// notifySettled tells both sides that evidence was accepted and money moved.
//
// Called from settle once the credit is in the ledger, with the net amount
// the worker earned.
func (s *Server) notifySettled(l *api.Listing, sub api.Submission, worker string, netMinor int64) {
	if l == nil || netMinor <= 0 || l.Practice {
		return
	}
	base := s.base()
	what := whatFor(l, sub)
	amount := money(netMinor, l.Currency)

	// The worker.
	var state string
	if s.Holdbacks != nil {
		clears := s.now().Add(s.disputeWindow())
		state = fmt.Sprintf(
			"It is held until %s so the buyer can look at what came back. If they "+
				"release it sooner it clears sooner; if they object you will hear from us.",
			when(clears))
	} else {
		state = "It is clear to send now."
	}
	s.tell("settled:"+stageKey(sub), worker, "Accepted: "+l.Title, fmt.Sprintf(
		"Your evidence for %s was accepted and %s has been credited to you.\n\n"+
			"%s\n\nWhere it stands: %s/console\n",
		what, amount, state, base))

	// The buyer.
	if l.Owner == "" {
		return
	}
	came := "It passed verification."
	switch {
	case sub.Attempted:
		came = "It passed verification as an attempt: the worker went and could not finish."
		if sub.Why != "" {
			came += " In their words: " + sub.Why
		}
	case l.Kind == api.KindObserve && sub.Finding:
		came = "It passed verification, and the answer came back yes."
	case l.Kind == api.KindObserve:
		came = "It passed verification, and the answer came back no."
	case sub.StageName != "":
		came = "It passed verification and shows this stage finished."
	default:
		came = "It passed verification and shows the work finished."
	}
	var money2 string
	if s.Holdbacks != nil {
		clears := s.now().Add(s.disputeWindow())
		money2 = fmt.Sprintf(
			"%s is held for the worker until %s. If you are happy with it, release "+
				"it now and they are paid sooner. If something is wrong with the work "+
				"against what was agreed, object before then. After %s it goes to the "+
				"worker on its own.\n\nLook, release or object: %s/console",
			amount, when(clears), when(clears), base)
	} else {
		money2 = fmt.Sprintf("%s has been paid to the worker.\n\nStatus: %s/console", amount, base)
	}
	s.tell("settled:buyer:"+stageKey(sub), l.Owner, "Evidence in: "+l.Title, fmt.Sprintf(
		"Evidence arrived for %s.\n\n%s\n\n%s\n\nReceipt: %s/v1/jobs/%s/receipt\n",
		what, came, money2, base, l.Job))
}

// notifyRejected tells both sides that evidence was not accepted, and why.
//
// again says whether the worker can add a better file and finish again — true
// when the verifier refused the submission outright, false when it was taken
// and simply earned nothing.
func (s *Server) notifyRejected(sub api.Submission, why string, again bool) {
	if s.Board == nil {
		return
	}
	l, ok := s.Board.Get(sub.Job)
	if !ok || l.Practice {
		return
	}
	why = strings.TrimSpace(why)
	if why == "" {
		why = "the evidence did not establish what the job asked for"
	}
	base := s.base()
	what := whatFor(l, sub)
	key := stageKey(sub) + ":" + short(why)

	worker, _ := s.Board.WorkerFor(sub.Holder)
	var next string
	if again {
		page := s.resumeFor(sub.Job, worker)
		if page == "" {
			page = base + "/console"
		}
		next = fmt.Sprintf(
			"Nothing is spent. Add a better photo or video and finish again from the "+
				"same page: %s\n\nIf you cannot, give the seat back from your console so "+
				"somebody else can take it, with no mark against you: %s/console",
			page, base)
	} else {
		next = fmt.Sprintf(
			"That submission has been used and nothing was credited for it. If you "+
				"think this is wrong, write to support@lamdis.ai with the job id %s.\n\n"+
				"Where you stand: %s/console", sub.Job, base)
	}
	s.tell("rejected:"+key, worker, "Not accepted: "+l.Title, fmt.Sprintf(
		"Your evidence for %s was not accepted.\n\n%s\n\n%s\n", what, why, next))

	if l.Owner == "" {
		return
	}
	then := "The seat is used, and whatever was not earned returns to your balance " +
		"when the job finishes."
	if again {
		then = "The worker can try again with better evidence while they hold the job."
	}
	s.tell("rejected:buyer:"+key, l.Owner, "Evidence refused: "+l.Title, fmt.Sprintf(
		"Evidence arrived for %s and did not pass.\n\n%s\n\nNothing has been paid "+
			"for it. %s\n\nStatus: %s/console\n", what, why, then, base))
}

// notifyAwarded tells a bidder they won an open job.
func (s *Server) notifyAwarded(l *api.Listing, worker string, amountMinor int64, currency string) {
	if l == nil || worker == "" {
		return
	}
	base := s.base()
	s.tell("awarded:"+l.Job+":"+worker, worker, "Yours: "+l.Title, fmt.Sprintf(
		"Your offer on %s was accepted at %s.\n\nThe job is yours. Take it from the "+
			"board to get the address and your code: %s/board\n\nIt expires %s.\n",
		l.Title, money(amountMinor, currency), base, when(l.Expires)))
}

// notifyScopeAwarded tells a bidder they won a whole project.
func (s *Server) notifyScopeAwarded(project string, won *api.ProjectBid) {
	if won == nil || won.Worker == "" {
		return
	}
	base := s.base()
	title := project
	if s.Board != nil {
		for _, ln := range won.Lines {
			if l, ok := s.Board.Get(ln.Job); ok && l.ProjectTitle != "" {
				title = l.ProjectTitle
				break
			}
		}
	}
	var lines []string
	for _, ln := range won.Lines {
		name := ln.Job
		if l, ok := s.Board.Get(ln.Job); ok {
			name = l.Title
		}
		lines = append(lines, fmt.Sprintf("  %s: %s", name, money(ln.AmountMinor, won.Currency)))
	}
	s.tell("awarded:project:"+project+":"+won.Worker, won.Worker, "Yours: "+title, fmt.Sprintf(
		"Your offer on %s was accepted at %s, every piece to you.\n\n%s\n\nEach piece is "+
			"escrowed at its line amount and paid when its evidence is accepted. Take "+
			"them from the board: %s/board\n",
		title, money(won.TotalMinor, won.Currency), strings.Join(lines, "\n"), base))
}

// notifyHeld tells a worker the buyer has objected, on what ground, and by
// when it will be decided.
func (s *Server) notifyHeld(job, reason string, until time.Time) {
	if s.Holdbacks == nil || s.Board == nil {
		return
	}
	l, ok := s.Board.Get(job)
	if !ok {
		return
	}
	base := s.base()
	for _, h := range s.Holdbacks.ForJob(job) {
		if h.Paid || !h.Held {
			continue
		}
		s.tell("held:"+job+":"+h.Person+":"+short(reason), h.Person, "Objection: "+l.Title, fmt.Sprintf(
			"The buyer has objected to your work on %s, and %s is frozen while it is "+
				"decided.\n\nTheir objection: %s\n\nIt will be decided by %s, by a panel "+
				"of people who are shown what was agreed and what you submitted, and who "+
				"are not the buyer and not us. If it is not decided by then, the money "+
				"goes to you.\n\nIf you want to add anything, write to support@lamdis.ai "+
				"with the job id %s.\n\nWhere it stands: %s/console\n",
			l.Title, money(h.AmountMinor, h.Currency), reason, when(until), job, base))
	}
}

// notifyReleased tells a worker the buyer accepted the work early.
func (s *Server) notifyReleased(job string) {
	if s.Holdbacks == nil || s.Board == nil {
		return
	}
	l, ok := s.Board.Get(job)
	if !ok {
		return
	}
	base := s.base()
	for _, h := range s.Holdbacks.ForJob(job) {
		if h.Paid || h.Held {
			continue
		}
		s.tell("released:"+job+":"+h.Person, h.Person, "Released: "+l.Title, fmt.Sprintf(
			"The buyer has accepted your work on %s. %s is clear to send and goes "+
				"out with the next payout run once you are over the threshold, or "+
				"sooner if you ask for it.\n\nWhere it stands: %s/console\n",
			l.Title, money(h.AmountMinor, h.Currency), base))
	}
}

// notifyHoldsLapsed tells workers that an objection went undecided and the
// money is theirs.
func (s *Server) notifyHoldsLapsed(jobs []string) {
	if s.Holdbacks == nil || s.Board == nil {
		return
	}
	base := s.base()
	for _, job := range jobs {
		l, ok := s.Board.Get(job)
		if !ok {
			continue
		}
		for _, h := range s.Holdbacks.ForJob(job) {
			if h.Paid || h.Held {
				continue
			}
			s.tell("lapsed:"+job+":"+h.Person, h.Person, "Cleared: "+l.Title, fmt.Sprintf(
				"The buyer's objection to your work on %s was not decided in time, so "+
					"it has lapsed in your favour. %s is clear to send and goes out with "+
					"the next payout run.\n\nWhere it stands: %s/console\n",
				l.Title, money(h.AmountMinor, h.Currency), base))
		}
	}
}

// notifyPaid tells a worker money has left for their account.
func (s *Server) notifyPaid(person string, amountMinor int64, currency, acct, ref string) {
	if amountMinor <= 0 {
		return
	}
	key := ref
	if key == "" {
		key = fmt.Sprintf("%s:%d:%s:%d", person, amountMinor, currency, s.now().Unix())
	}
	s.tell("paid:"+key, person, "Sent: "+money(amountMinor, currency), fmt.Sprintf(
		"We have sent %s to your payout account (%s).\n\nReference: %s\n\nIt usually "+
			"lands within a few business days, depending on your bank.\n\nYour record: %s/console\n",
		money(amountMinor, currency), acct, orDash(ref), s.base()))
}

// notifyExpired tells a buyer that nobody took their job and the money is
// back.
//
// AlertStaleJobs says "still waiting" while there is time to act; this is the
// end of it. Same voice, different sentence, and only for work nobody touched.
func (s *Server) notifyExpired(l *api.Listing, returnedMinor int64) {
	if l == nil || l.Owner == "" || l.Cancelled || l.Practice || !api.IsWork(l.Kind) {
		return
	}
	if l.Taken > 0 || s.now().Before(l.Expires) {
		return
	}
	reach, advice := s.reachFor(QuoteRequest{
		Kind: l.Kind, Skills: l.Skills,
		Lat: api.Deg(l.LatE7), Lon: api.Deg(l.LonE7), Slots: l.Slots,
	})
	why := "Nobody took it."
	if reach == 0 {
		why = "Nobody set up for this work is currently within range of it."
		if len(advice) > 0 {
			why += " " + advice[0]
		}
	}
	back := ""
	if returnedMinor > 0 {
		back = fmt.Sprintf(" %s has been returned to your balance.", money(returnedMinor, l.Currency))
	}
	s.tell("expired:"+l.Job, l.Owner, "Expired unfilled: "+l.Title, fmt.Sprintf(
		"Your job expired %s with nobody on it.\n\n%s\n\n%s%s\n\nIf you still want it "+
			"done, post it again with a different window, pay or area: %s/console\n",
		when(l.Expires), l.Title, why, back, s.base()))
}
