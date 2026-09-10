package api

import (
	"fmt"
	"net/http"
)

// The pages a person looks for before trusting a marketplace with their money,
// their address, or their afternoon.
//
// There were none. No terms, no privacy statement, no way to reach anybody —
// on a platform that holds funds, pays individuals, and sends strangers to
// private homes. Meanwhile the genuinely strong parts of the design (escrow
// before work starts, per-job challenge codes, signed receipts anyone can
// verify) lived only in developer documentation, where nobody worried about
// letting a stranger through their gate would ever find them.
//
// Written plainly and kept short. A trust page nobody reads is the same as no
// trust page, and length is the main reason nobody reads them.
//
// Six routes share one body, because the answers overlap almost entirely and
// six pages that drift is worse than one that is true. Each route gets its own
// title and opening, so /terms reads as terms and /privacy as privacy, and
// /how-it-works carries the explanation the others only summarise: the four
// verbs, and the money rules with the numbers the code actually enforces.

// trustRoute is what differs between the six paths.
type trustRoute struct {
	Path, Eyebrow, Title, Lead string
	// Desc is the sentence a search engine shows under the link. Written per
	// page: six pages sharing one description is six pages a crawler treats
	// as one.
	Desc string
}

var trustRoutes = []trustRoute{
	{"/how-it-works", "How this works", "Route. Execute. Prove. Settle.",
		"Software hires people to do things in the physical world, and pays only " +
			"against evidence that the thing was done. Four verbs, one set of money " +
			"rules, and nothing that depends on trusting us.",
		"How the Lamdis exchange works: an agent posts what should become true, " +
			"the money is escrowed before anyone is dispatched, a person does the work " +
			"and proves it with a coded photograph, and settlement follows the proof."},
	{"/terms", "Terms", "What each side agrees to",
		"Plain words rather than a contract you will not read. This is what you are " +
			"agreeing to when you post work or take it, and what we can honestly " +
			"claim in return.",
		"The terms of the Lamdis exchange in plain words: what a buyer agrees to " +
			"when they post work, what a worker agrees to when they take it, and what " +
			"Lamdis does and does not promise about either."},
	{"/privacy", "Privacy", "What we keep, and what we never see",
		"An email address, what you took and submitted, and where the evidence says " +
			"it was taken. No card numbers, no bank accounts, no address on the open " +
			"board. The detail is below.",
		"What the Lamdis exchange stores and what it never sees: an email address " +
			"and evidence hashes, no card numbers or bank details, and never a private " +
			"address on the public board."},
	{"/about", "About", "An exchange for work in the physical world",
		"Lamdis is early software run by a small team. Agents post what should " +
			"become true; people make it true and prove it; money moves on the proof. " +
			"Here is what that means for you, and what it does not.",
		"About Lamdis: an exchange where software hires people for work in the " +
			"physical world and pays against verified evidence. Early software, run by " +
			"a small team, with the limits stated rather than hidden."},
	{"/support", "Support", "A person reads it",
		"If work was not done, was done badly, or something happened on site, write " +
			"to support@lamdis.ai. There is no automated appeals process and no " +
			"arbitration clause. The rules you are asking about are below.",
		"Support for the Lamdis exchange. Write to support@lamdis.ai about a job, a " +
			"payment or an account and a person reads it: no automated appeals process " +
			"and no arbitration clause."},
	{"/contact", "Contact", "How to reach us",
		"support@lamdis.ai for anything about a job, a payment or an account. " +
			"security@lamdis.ai for a security problem. What follows is what we can " +
			"promise, so you know what to ask for.",
		"How to reach Lamdis: support@lamdis.ai for a job, a payment or an account, " +
			"and security@lamdis.ai to report a security problem."},
}

// trustPage renders one of the six. Unknown paths fall back to the first.
func trustPage(path string) string {
	r := trustRoutes[0]
	for _, c := range trustRoutes {
		if c.Path == path {
			r = c
		}
	}
	explain := ""
	if r.Path == "/how-it-works" {
		explain = howItWorksHTML
	}
	return trustPageTop + `<title>Lamdis Exchange — ` + r.Eyebrow + `</title>
<style>` + themeCSS + trustCSS + `</style>
<header class="top">
  <a class="mark" href="/board">lamdis<b>.</b></a>
  <span class="pill"><span class="beacon"></span>` + r.Eyebrow + `</span>
  <div class="right"><a class="back" href="/board">Board</a>
    <a class="back" href="/docs">API</a></div>
</header>
<main>
<div class="hero rv">
<span class="eyebrow">` + r.Eyebrow + `</span>
<h1>` + r.Title + `</h1>
<p class="lead">` + r.Lead + `</p>
<nav class="crumbs">` + trustNav(r.Path) + `</nav>
</div>
` + explain + trustBodyHTML + `</main>
`
}

// trustNav is the strip of sibling pages, the current one lit.
func trustNav(current string) string {
	out := ""
	for _, c := range trustRoutes {
		cur := ""
		if c.Path == current {
			cur = ` aria-current="page"`
		}
		out += `<a href="` + c.Path + `"` + cur + `>` + c.Eyebrow + `</a>`
	}
	return out
}

const trustPageTop = `<!doctype html>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
`

const trustCSS = `
main { padding: 1.4rem 1rem 5rem; max-width: 52rem; margin: 0 auto; }
@media (min-width: 40rem) { main { padding: 2.2rem 1.5rem 6rem; } }
.top .back { font-size: .84rem; color: var(--ink-2); text-decoration: none; }
.top .back:hover { color: var(--ink); }
.eyebrow { display: block; font: 600 .62rem/1 var(--mono); letter-spacing: .18em;
  text-transform: uppercase; color: var(--gold); }
.eyebrow.blue { color: var(--blue); }
.eyebrow.green { color: var(--green); }
.hero { margin: 0 0 1.6rem; }
h1 { font: 750 clamp(1.9rem, 4.6vw, 2.8rem)/1.05 var(--sans); letter-spacing: -.035em; margin: .7rem 0 .8rem; }
.lead { color: var(--ink-2); font-size: 1.08rem; max-width: 40rem; margin: 0; }
.crumbs { display: flex; flex-wrap: wrap; gap: .4rem; margin-top: 1.3rem; }
.crumbs a { padding: .35rem .65rem; border: 1px solid var(--rule-2); border-radius: 999px;
  font: 600 .62rem/1 var(--mono); letter-spacing: .12em; text-transform: uppercase;
  color: var(--ink-3); text-decoration: none; }
.crumbs a:hover { color: var(--ink); border-color: var(--ink-3); }
.crumbs a[aria-current="page"] { color: var(--gold); border-color: var(--gold); }
h2 { font: 700 1.2rem/1.2 var(--sans); letter-spacing: -.025em; text-transform: none; color: var(--ink);
  margin: 2.6rem 0 .8rem; padding-top: 1.3rem; border-top: 1px solid var(--rule); }
h2::before { content: ""; display: block; width: 2rem; height: 2px; background: var(--gold); margin-bottom: 1rem; }
p, li { color: var(--ink-2); line-height: 1.6; }
p b { color: var(--ink); font-weight: 600; }
li { margin: .3rem 0; }
a { color: var(--gold); }
.plain { border-left: 2px solid var(--gold); padding: .1rem 0 .1rem .9rem;
  margin: 1rem 0; color: var(--ink); font-size: .93rem; }
.plain p { color: var(--ink); }
/* Q&A as glass cells, two up. */
dl.qa { display: grid; grid-template-columns: 1fr; gap: .7rem; margin: 0; }
@media (min-width: 46rem) { dl.qa { grid-template-columns: 1fr 1fr; } }
dl.qa > div { border: 1px solid var(--rule); border-radius: 8px; padding: 1rem 1.1rem;
  background: var(--glass); -webkit-backdrop-filter: blur(12px); backdrop-filter: blur(12px);
  box-shadow: inset 0 1px 0 rgba(255,255,255,.04); }
dt { font-weight: 600; color: var(--ink); font-size: .93rem; margin: 0; }
dd { margin: .4rem 0 0; color: var(--ink-2); font-size: .88rem; line-height: 1.55; }
/* The four verbs. Blue routes, gold executes, green proves, gold settles. */
.verbs { display: grid; grid-template-columns: 1fr; gap: .8rem; margin: 0 0 1.4rem; position: relative; }
@media (min-width: 46rem) { .verbs { grid-template-columns: repeat(4, 1fr); } }
.verb { position: relative; overflow: hidden; border: 1px solid var(--rule); border-radius: 8px;
  padding: 1.1rem 1.1rem 1.2rem; background: var(--glass);
  -webkit-backdrop-filter: blur(12px); backdrop-filter: blur(12px);
  box-shadow: inset 0 1px 0 rgba(255,255,255,.04); }
.verb::after { content: ""; position: absolute; left: 0; right: 0; top: 0; height: 2px;
  background: linear-gradient(90deg, var(--c, var(--rule-2)), transparent); }
.verb .n { font: 600 .62rem/1 var(--mono); letter-spacing: .18em; text-transform: uppercase; color: var(--c); }
.verb h3 { margin: .6rem 0 .4rem; font: 750 1.35rem/1 var(--sans); letter-spacing: -.03em; color: var(--ink); }
.verb p { margin: 0; font-size: .86rem; }
.verb.route { --c: var(--blue); } .verb.exec { --c: var(--gold); }
.verb.prove { --c: var(--green); } .verb.settle { --c: var(--gold); }
/* The ladder: what an account may hold at once, earned rung by rung. */
.ladder { display: grid; grid-template-columns: repeat(2, 1fr); gap: .7rem; margin: 1rem 0 1.4rem; }
@media (min-width: 46rem) { .ladder { grid-template-columns: repeat(5, 1fr); } }
.rung { border: 1px solid var(--rule); border-radius: 8px; padding: .9rem 1rem; background: var(--glass);
  -webkit-backdrop-filter: blur(12px); backdrop-filter: blur(12px); }
.rung .amt { display: block; font: 750 1.5rem/1 var(--sans); letter-spacing: -.03em; color: var(--gold);
  font-variant-numeric: tabular-nums; }
.rung .lbl { display: block; margin-top: .45rem; font: 600 .6rem/1.4 var(--mono); letter-spacing: .14em;
  text-transform: uppercase; color: var(--ink-3); }
.rung .why { display: block; margin-top: .3rem; font-size: .78rem; color: var(--ink-2); }
.rung.shaken .amt { color: var(--warn); }
/* Money rules: the numbers the code enforces, each on its own tile. */
.rules { display: grid; grid-template-columns: 1fr; gap: .7rem; margin: 1rem 0 1.4rem; }
@media (min-width: 46rem) { .rules { grid-template-columns: 1fr 1fr; } }
.rule { display: grid; grid-template-columns: 5.4rem 1fr; gap: .9rem; align-items: start;
  border: 1px solid var(--rule); border-radius: 8px; padding: .95rem 1rem; background: var(--glass);
  -webkit-backdrop-filter: blur(12px); backdrop-filter: blur(12px); }
.rule .num { font: 750 1.25rem/1.1 var(--sans); letter-spacing: -.03em; color: var(--gold);
  font-variant-numeric: tabular-nums; }
.rule .num.mono { font: 700 1.05rem/1.2 var(--mono); letter-spacing: .08em; }
.rule .num.green { color: var(--green); }
.rule .num.blue { color: var(--blue); }
.rule .num.warn { color: var(--warn); }
.rule b { display: block; color: var(--ink); font-size: .9rem; margin-bottom: .2rem; }
.rule p { margin: 0; font-size: .82rem; }
.grounds { display: flex; flex-wrap: wrap; gap: .4rem; margin: .6rem 0 0; padding: 0; list-style: none; }
.grounds li { margin: 0; padding: .3rem .55rem; border: 1px solid #3A2510; border-radius: 3px;
  font: 500 .72rem/1.3 var(--mono); color: var(--warn); }
.foot-links { margin-top: 3rem; padding-top: 1.2rem; border-top: 1px solid var(--rule);
  font: 500 .74rem/1.6 var(--mono); letter-spacing: .06em; color: var(--ink-3); }
.foot-links a { color: var(--ink-2); text-decoration: none; }
.foot-links a:hover { color: var(--gold); }
`

// howItWorksHTML is the explanation only /how-it-works carries.
//
// Every figure here is read from the code that enforces it, and the comment
// on each says where. A number on a trust page that the code does not match
// is a broken promise with a byline.
const howItWorksHTML = `
<div class="verbs">
<div class="verb route rv" style="--i:1"><span class="n">01 · Software</span><h3>Route</h3>
<p>An agent says what should become true &mdash; a gate latched, a meter read &mdash;
and the full amount is held before anybody sees the job. It reaches the people who
can do it: nearest first, filtered to skill, range and standing.</p></div>
<div class="verb exec rv" style="--i:2"><span class="n">02 · People</span><h3>Execute</h3>
<p>Somebody takes it and goes. They get a one-time code issued privately for that job,
and, for anything tied to a place, the house number or mark that has to be in frame.</p></div>
<div class="verb prove rv" style="--i:3"><span class="n">03 · Evidence</span><h3>Prove</h3>
<p>Photographs, as the camera saved them: the code legible, the place marked, when and
where recorded, never submitted before. Software judges first; a paid panel of people
when it cannot be sure.</p></div>
<div class="verb settle rv" style="--i:4"><span class="n">04 · Money</span><h3>Settle</h3>
<p>Accepted evidence credits the worker at once. The buyer has a day to look and either
release or object on a named ground. Both sides get a signed receipt anyone can verify.</p></div>
</div>

<h2>The money rules</h2>
<p>These are not policy; they are constants in the code that moves the money. If one
changes, this page changes in the same commit.</p>
<div class="rules">
<div class="rule rv" style="--i:1"><span class="num green">Held</span><div><b>Funds first, always</b>
<p>A job cannot be listed until its full amount is set aside. It is not a promise from
the buyer; the money is there before you see the job.</p></div></div>
<div class="rule rv" style="--i:2"><span class="num mono">7K3-QM</span><div><b>One code per job, once</b>
<p>Issued privately when you take the job, and only good for that job. In frame, it
proves the photograph was taken now, for this.</p></div></div>
<div class="rule rv" style="--i:3"><span class="num mono">812</span><div><b>The house number in frame</b>
<p>For work at an address, something visible only at that property has to appear too.
Read as text, so it survives a different camera, angle or season.</p></div></div>
<div class="rule rv" style="--i:4"><span class="num">85% <small style="color:var(--ink-3);font-size:.7em">/ 72%</small></span><div><b>How sure we will ever claim to be</b>
<p>Until cameras attest their own captures, no verdict on a photograph claims more than
85% confidence with location and time recorded, or 72% without. The receipt says which.</p></div></div>
<div class="rule rv" style="--i:5"><span class="num">$500</span><div><b>Staged above this</b>
<p>No single photograph decides more than $500. A bigger job is cut into stages that
are each evidenced and paid on their own, so the most anyone can lose on one verdict
is one stage.</p></div></div>
<div class="rule rv" style="--i:6"><span class="num">0%</span><div><b>What the exchange keeps</b>
<p>Nothing, while it is getting off the ground. When that changes it is applied at
settlement and said here first.</p></div></div>
<div class="rule rv" style="--i:7"><span class="num">$20</span><div><b>Payout threshold</b>
<p>Earnings are sent once they reach $20; below that they wait, because a transfer
costs a flat fee that would eat a small one. Ask and we will send it early, fee out
of it.</p></div></div>
<div class="rule rv" style="--i:8"><span class="num">24h <small style="color:var(--ink-3);font-size:.7em">+ 7d</small></span><div><b>Look, then object on a ground</b>
<p>Accepted work credits the worker at once; the money leaves after 24 hours unless the
buyer releases sooner or objects. An objection must name one of five grounds, is decided
by somebody who is not the buyer, and lapses in the worker&rsquo;s favour after 7 days.</p>
<ul class="grounds"><li>not_done</li><li>wrong_place</li><li>fabricated</li><li>damage</li><li>unsafe</li></ul>
</div></div>
</div>

<h2>What an account may hold at once</h2>
<p>A ceiling on exposure, not on a single job: the most you may have riding on
unfinished work. It is earned, never granted &mdash; a new account cannot take a
thousand-dollar job, and a thief has to build a record worth more than the theft
before the theft is possible.</p>
<div class="ladder">
<div class="rung rv" style="--i:1"><span class="amt">$75</span><span class="lbl">New</span><span class="why">No record yet</span></div>
<div class="rung rv" style="--i:2"><span class="amt">$300</span><span class="lbl">Proven</span><span class="why">A few clean completions</span></div>
<div class="rung rv" style="--i:3"><span class="amt">$1,200</span><span class="lbl">Established</span><span class="why">A real run of them</span></div>
<div class="rung rv" style="--i:4"><span class="amt">$25,000</span><span class="lbl">Vetted</span><span class="why">Licences and cover a person checked</span></div>
<div class="rung shaken rv" style="--i:5"><span class="amt">$25</span><span class="lbl">Shaken</span><span class="why">After abandoning work</span></div>
</div>
`

// trustBodyHTML is what all six routes share.
const trustBodyHTML = `
<h2>If you are doing the work</h2>
<dl class="qa">
<div><dt>The money is already there before you start</dt>
<dd>A job cannot be listed unless the full amount is held. It is not a promise
from the buyer; it is set aside before you see the job.</dd></div>

<div><dt>The buyer has a day before your money is sent</dt>
<dd>Your balance is credited when the evidence is accepted, and the transfer
goes out once a 24-hour window closes. Most buyers release sooner. If somebody
objects you will see it, with their ground and reason, on your earnings page;
an objection nobody decides within 7 days releases the money to you.</dd></div>

<div><dt>If you cannot finish, say so and photograph why</dt>
<dd>Mark it as an attempt and take a picture of whatever stopped you with the
code in frame. That earns the attempt fee. Photographing the address and
claiming completion does not.</dd></div>

<div><dt>You are paid for admissible evidence, not for the answer</dt>
<dd>On a check, you are paid whether the answer turns out to be yes or no. That
is deliberate: paying only for one answer is how you get that answer.</dd></div>

<div><dt>What we keep</dt>
<dd>Nothing, for now. The exchange takes no cut of what you earn while it is
getting off the ground; when that changes it will be said here first and
applied at settlement. Payouts are sent once your balance reaches $20 &mdash;
below that it waits, because a transfer costs a flat fee that would eat a small
one. Both figures are shown on the board.</dd></div>

<div><dt>You can see whether a person or an agent posted the job</dt>
<dd>Jobs written by software are labelled. That comes from the credential used
to post, not from anything the buyer told us.</dd></div>

<div><dt>You can take your money out below the threshold</dt>
<dd>If you would rather have it now than wait for $20, ask and we will send it
&mdash; the transfer fee comes out of it. That trade is yours to make.</dd></div>

<div><dt>What we do not know about the buyer</dt>
<dd>Their account is verified by email, and their money is real and held. We do
not check who they are, and we do not vet the address. Treat an unfamiliar
address the way you would any other stranger's.</dd></div>

<div><dt>You can hand a job back</dt>
<dd>Before the claim expires, with no penalty beyond a short wait before taking
another. Letting a claim lapse silently is what costs you standing, because
somebody is waiting on it.</dd></div>
</dl>

<h2>If you are buying the work</h2>
<dl class="qa">
<div><dt>You pay against evidence, not against a promise</dt>
<dd>Photographs, video, and where and when they were taken. Money settles on
the verdict, and what is not earned is returned.</dd></div>

<div><dt>You get a day to look before the money leaves</dt>
<dd>Payment is worked out as soon as the evidence is accepted, but it is not
sent for 24 hours. In that time you can look at what came back and either pay
straight away or object. An objection has to name a ground &mdash; the work was
not done, it is not this property, the evidence is not genuine, something was
damaged, the work was left unsafe &mdash; and is decided by somebody who is not
you, within 7 days.</dd></div>

<div><dt>Being there is not the same as being done</dt>
<dd>We check that the evidence belongs to your job, and separately whether it
shows what you asked for. Somebody who turns up, photographs the front of the
property and leaves is not paid the completion fee.</dd></div>

<div><dt>Your address is not published</dt>
<dd>The open board shows a coarse area. The exact address and your access
instructions go only to the person who has taken the job.</dd></div>

<div><dt>What we do not check</dt>
<dd>We do not run background checks, and we are not insured on your behalf.
Accounts are verified by email and payouts by the payment provider's own
identity checks. That is a real limit and you should weigh it.</dd></div>

<div><dt>The receipt does not require trusting us</dt>
<dd>Every finished job produces a signed receipt. Anyone can verify it against
the evidence hashes without asking this exchange anything.</dd></div>
</dl>

<h2>Work we refuse</h2>
<p>We do not carry jobs that involve opening accounts in someone else's name,
one-time passcodes or two-factor codes, impersonating anybody, paid followers
or reviews, referral and signup-bonus farming, or passing login details between
people. These are refused before a job is listed rather than after you have
done it.</p>
<p>If you are ever asked to do one of these &mdash; here or anywhere &mdash; the person
whose name ends up on the account is you, not whoever paid. Say no and tell us
at <a href="mailto:support@lamdis.ai">support@lamdis.ai</a>.</p>

<h2>Money</h2>
<p>Money you add sits in our account at Stripe until it is paid out. We do hold
it in that sense, and we would rather say so plainly than use a form of words
that implies otherwise.</p>
<p>What we do not do: we run no wallet of our own, we hold nothing outside
Stripe, and we never see a card number or a bank account. Adding funds and
setting up payouts both happen on Stripe's own pages, and payouts go to an
account in your name that you control.</p>

<h2>What we are not</h2>
<div class="plain">
<p>We are not an employer. People here choose what to take, when, and at what
price on open jobs, and use their own equipment.</p>
<p>We are not a bank. Money held here earns you nothing, is not insured as a
deposit, and is meant to sit here only as long as a job takes.</p>
<p>We do not guarantee the quality of anyone's work. We verify evidence that
something was done; a verified photograph of a badly cleared gutter is still a
badly cleared gutter.</p>
<p>We check that a photograph carries a code we issued privately for that job,
that it records when and where it was taken, that it has not been submitted
before, and that it does not look generated. The last of those is a model's
judgement measured against real photographs and generated ones; it catches
ordinary image generation and would not stop somebody who set out to defeat it
specifically.</p>
</div>

<h2>Privacy</h2>
<p>We store your email address, what you took and submitted, and where the
evidence says it was taken. Evidence files are held only long enough to verify
them and are not retained afterwards; their content hashes and the verdict
are.</p>
<p>We do not sell anything to anybody. Your exact address is never published on
the open board.</p>

<h2>Disputes and getting hold of us</h2>
<p>If work was not done, or was done badly, or something happened on site,
write to <a href="mailto:support@lamdis.ai">support@lamdis.ai</a>. A person
reads it. There is no automated appeals process, and there is no arbitration
clause.</p>
<p>This is early software run by a small team. If that matters for what you
were about to use it for, it should.</p>

<h2>Reporting a security problem</h2>
<p><a href="mailto:security@lamdis.ai">security@lamdis.ai</a>. We will not
pursue anyone acting in good faith.</p>

<p class="foot-links"><a href="/board">Board</a> &middot;
<a href="/console">Console</a> &middot; <a href="/docs">API</a> &middot;
<a href="/how-it-works">How this works</a></p>
`

// RegisterTrust mounts the plain-language pages. Several paths reach the same
// body because people look for it under different names, and a 404 on /terms
// is its own kind of answer. Each is rendered once, at startup.
func RegisterTrust(mux *http.ServeMux, baseURL string) {
	for _, r := range trustRoutes {
		page := WithSEO(trustPage(r.Path), baseURL, r.Path, r.Desc)
		mux.HandleFunc("GET "+r.Path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Referrer-Policy", "no-referrer")
			fmt.Fprint(w, page)
		})
	}
}
