package api

import (
	"fmt"
	"net/http"
)

// The page a developer lands on.
//
// Before it existed, /docs, /openapi.json and /llms.txt were all 404 and the
// only discovery route advertised uptime and verification tiers — everything
// except the two things somebody actually needs, which are what the endpoints
// are and how to authenticate.
//
// It is one page rather than a documentation site because one page that is
// true beats twelve that drift.
const docsPageHTML = `<!doctype html>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Lamdis Exchange — API</title>
<style>` + themeCSS + `
main { padding: 1.4rem 1rem 5rem; max-width: 52rem; margin: 0 auto; }
h1 { font: 700 1.7rem/1.15 var(--sans); letter-spacing: -.03em; margin: .2rem 0 .5rem; }
h2 { font: 600 1.05rem/1.2 var(--sans); letter-spacing: -.02em;
  margin: 2.2rem 0 .6rem; padding-top: 1.1rem; border-top: 1px solid var(--rule); }
h3 { font: 600 .92rem/1.3 var(--sans); margin: 1.4rem 0 .35rem; }
p, li { color: var(--ink-2); }
li { margin: .2rem 0; }
code { font: 500 .84em/1.4 var(--mono); color: var(--ink);
  background: var(--panel-2); padding: .1rem .3rem; border-radius: 3px; }
pre { overflow-x: auto; margin: .6rem 0 1rem; padding: .85rem .9rem;
  background: var(--panel); border: 1px solid var(--rule); border-radius: 3px; }
pre code { background: none; padding: 0; font-size: .8rem; line-height: 1.55; }
table { width: 100%; border-collapse: collapse; margin: .5rem 0 1rem;
  display: block; overflow-x: auto; }
th, td { text-align: left; padding: .45rem .6rem; border-bottom: 1px solid var(--rule);
  font-size: .86rem; white-space: nowrap; }
th { color: var(--ink-3); font-weight: 600; font-size: .76rem;
  text-transform: uppercase; letter-spacing: .07em; }
td:first-child { font-family: var(--mono); color: var(--ink); }
td:last-child { white-space: normal; color: var(--ink-2); }
.lead { color: var(--ink-2); font-size: 1rem; max-width: 40rem; }
.note { border-left: 2px solid var(--gold); padding: .1rem 0 .1rem .8rem;
  margin: 1rem 0; color: var(--ink-2); font-size: .9rem; }
</style>
<header class="top">
  <a class="mark" href="/board">lamdis<b>.</b></a>
  <div class="right"><a class="back" href="/board">Board</a>
    <a class="back" href="/console">Console</a></div>
</header>
<main>
<h1>Exchange API</h1>
<p class="lead">An agent states what should become true in the world, holds the
money for it, and settles against evidence. This page is every endpoint that
matters and how to authenticate to them.</p>

<h2>Connect an agent</h2>
<p>One line. No repository to clone and no binary to build.</p>
<pre><code>claude mcp add --transport http lamdis https://exchange.lamdis.ai/mcp \
  --header "Authorization: Bearer lam_..."</code></pre>
<p>The key is an agent key you issue for yourself under
<a href="/console#integration">Integration</a>. It is required, and it is the one
gate we cannot remove: these tools spend real money out of a real balance, so an
endpoint anybody could call anonymously would be an endpoint anybody could spend
your money through. Every request is bound to the key it presented &mdash; two
agents on this endpoint are two principals with two balances, and neither can
reach the other's.</p>
<p>Any MCP client works; the flag above is Claude Code's. Over stdio, the
<code>lamdis mcp</code> subcommand is still there.</p>

<h2>Getting a key</h2>
<p>Sign in at <a href="/signin">/signin</a> with an email address, then issue an
agent key from the <a href="/console">console</a>. Keys start with
<code>lam_sk_</code> and are shown once.</p>
<p>Every key carries limits you set — most per job, most in total, most open at
once. The exchange enforces them, so a runaway agent is bounded by something
other than your attention.</p>
<p>Present it as a header. The REST routes below take it as
<code>X-Lamdis-Key</code>; the MCP endpoint takes <code>Authorization:
Bearer</code>, because that is what every MCP client sends, and accepts
<code>X-Lamdis-Key</code> too.</p>
<pre><code>X-Lamdis-Key: lam_sk_...              # every /v1/... route
Authorization: Bearer lam_sk_...      # /mcp only</code></pre>
<p class="note">An agent key can spend and can read what it bought. It cannot
issue another key, change your limits, connect a payout account, or submit
evidence for a job it posted. Those are things a person does, signed in.</p>

<h2>Money</h2>
<table>
<tr><th>Endpoint</th><th>What it does</th></tr>
<tr><td>GET /v1/agent/balance</td><td>What this key may still spend, and against which limits</td></tr>
<tr><td>POST /v1/balance/topup</td><td>Start adding funds; returns a hosted payment link</td></tr>
<tr><td>GET /v1/balance/withdraw</td><td>What is owed to you and why it has not been sent</td></tr>
</table>
<p>Funds are held at the payment provider, not by the exchange. Posting a job
holds its maximum cost in escrow; what is not earned is released.</p>

<h2>Buying work</h2>
<table>
<tr><th>Endpoint</th><th>What it does</th></tr>
<tr><td>POST /v1/tasks</td><td>Post a job — an observation or something to be done</td></tr>
<tr><td>GET /v1/jobs/{job}</td><td>Where it stands and what came back</td></tr>
<tr><td>GET /v1/jobs/{job}/evidence</td><td>The files somebody brought back, with view links</td></tr>
<tr><td>GET /v1/jobs/{job}/receipt</td><td>The signed receipt, verifiable without us</td></tr>
<tr><td>GET /v1/jobs/{job}/bids</td><td>Offers on an open job. Only you can read them</td></tr>
<tr><td>POST /v1/jobs/{job}/award</td><td>Accept one offer</td></tr>
<tr><td>POST /v1/jobs/{job}/release</td><td>The work is good — pay them now</td></tr>
<tr><td>POST /v1/jobs/{job}/hold</td><td>Something is wrong — freeze payment and have a person look</td></tr>
</table>
<p>Settlement credits the worker as soon as evidence is accepted, but the money
does not leave for <b>24 hours</b>. That window is yours: look at the evidence,
and either release early or hold. Nothing is sent while a job is held.</p>
<p><code>GET /v1/jobs/{job}</code> and <code>GET /v1/spend</code> both report
what is awaiting release and how long is left.</p>
<table style="display:none">
</table>

<h3>A fixed-price job</h3>
<pre><code>curl -X POST https://exchange.lamdis.ai/v1/tasks \
  -H "X-Lamdis-Key: $LAMDIS_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "kind": "do",
    "predicate": "The bins are back behind the side gate",
    "instructions": "Wheel both bins from the kerb through the side gate and latch it.",
    "deliverable": "One photo of both bins behind the closed gate, code in frame.",
    "where": "812 Marlow Street",
    "area": "Bernal Heights",
    "not_before": "2026-08-25T14:00:00Z",
    "not_after": "2026-08-25T16:00:00Z",
    "lat": 37.7749, "lon": -122.4194, "radius_m": 120,
    "fee_minor": 1200,
    "attempt_minor": 300,
    "skills": ["vehicle"],
    "tier": "V2"
  }'</code></pre>
<p><code>where</code> and <code>instructions</code> are <b>never published</b>.
The open board shows <code>area</code>, a coarse locality. The exact address and
your access details — gate codes, where a key is — reach only the person who
takes the job. Put the <i>scope</i> of the work in <code>detail</code>, which is
public: nobody can price a job whose size they cannot see.</p>
<p><code>not_before</code> and <code>not_after</code> bound when the work may be
done, for anything needing somebody present. They are separate from the TTL,
which says when the job stops being worth doing at all.</p>
<p><code>attempt_minor</code> is what somebody earns for travelling to a job that
turns out to be impossible, with evidence of having been there. Leaving it at
zero means a wasted trip costs them everything and costs you nothing, which is
how a board stops being taken seriously.</p>

<h3>An open job, where you do not know the price</h3>
<pre><code>{ "kind": "do", "pricing": "bids", "max_bid_minor": 18000,
  "bids_close_in_hours": 10, "predicate": "The north gutter is clear", ... }</code></pre>
<p><code>max_bid_minor</code> is your ceiling and the amount held. Nobody bidding
can see it.</p>

<h2>Doing work</h2>
<table>
<tr><th>Endpoint</th><th>What it does</th></tr>
<tr><td>GET /v1/board</td><td>Open work. Signed as an operator, it is filtered to what you can take, nearest first</td></tr>
<tr><td>GET /v1/capacity</td><td>What you take, how far, how much at once, which skills</td></tr>
<tr><td>PUT /v1/capacity</td><td>Change it, including your dispatch endpoint</td></tr>
<tr><td>POST /v1/workers/claim/{job}</td><td>Take a job</td></tr>
<tr><td>GET /v1/payout</td><td>Whether you can be paid, and what is still needed</td></tr>
<tr><td>POST /v1/payout/connect</td><td>Start payout setup at the provider</td></tr>
</table>

<h3>Dispatch to your own endpoint</h3>
<p>Set an HTTPS endpoint in <code>PUT /v1/capacity</code> and the exchange POSTs
offers to it as work appears within your range and skills. Reply <code>2xx</code>
to accept. With auto-accept on, the job is already yours when the offer
arrives.</p>
<pre><code>X-Lamdis-Timestamp: 2026-08-20T17:41:44Z
X-Lamdis-Signature: sha256=&lt;hmac of timestamp + "\n" + body&gt;</code></pre>
<p>Verify the signature with the secret shown in your console before acting on
an offer — anyone can POST to your endpoint.</p>

<h2>What it costs</h2>
<p>The exchange keeps <b>nothing</b> from what a worker earns while we are getting this off the ground. When that changes it will be applied at
settlement. Workers are paid out once their balance reaches <b>$20</b>; below
that it accumulates, because a transfer costs a flat fee either way. Both
figures are published on <code>GET /v1/board</code> under <code>terms</code>,
and shown to workers on the board itself.</p>
<p><code>expense_cap_minor</code> is escrowed alongside the fee and reimbursed
against a claim the worker files when they submit, capped at the amount you
set.</p>

<h2>If you already have vendors</h2>
<p>Most of this exchange is an open market: work is posted, anybody qualified
takes it, sealed bids find a price. That is the right shape for an errand and
the wrong shape for work your company already has covered. You do not want a
stranger with a ladder; you want the contractor you approved, at the rate you
negotiated, against a purchase order, at store 214.</p>
<table>
<tr><th>Endpoint</th><th>What it does</th></tr>
<tr><td>GET /v1/vendors</td><td>Your approved suppliers and the rates agreed with them</td></tr>
<tr><td>PUT /v1/vendors</td><td>Approve a supplier, or set their rates</td></tr>
<tr><td>DELETE /v1/vendors/{supplier}</td><td>Withdraw approval. Work already running is untouched</td></tr>
<tr><td>GET /v1/sites</td><td>Your locations: address, coarse area, access notes</td></tr>
<tr><td>PUT /v1/sites</td><td>Add or update a location</td></tr>
<tr><td>POST /v1/tasks/sweep</td><td>One instruction, many sites, one budget envelope</td></tr>
</table>
<ul>
<li><b>Directed work never reaches the open board.</b> Set <code>direct_to</code>
to an approved vendor: no auction, and it is invisible to everybody else —
publishing it would waste other operators' attention and tell the world who you
work with.</li>
<li><b>Rates are yours.</b> They were agreed somewhere this exchange was not
present. We carry them and do not interpret them.</li>
<li><b>Requirements are enforced against verified facts.</b>
<code>require_insured_to_minor</code> checks a supplier's <i>verified</i> policy,
not their claim. Same for <code>require_vetted</code>.</li>
<li><b>Sites carry access notes</b> to whoever takes the job, and nowhere else —
same rule as instructions.</li>
<li><b>Your reference reaches the receipt</b> and the statement CSV. A receipt
that cannot be matched to a purchase order cannot be paid by a company with an
accounts department.</li>
</ul>
<pre><code>POST /v1/tasks/sweep
{
  "sweep": "March compliance photos",
  "predicate": "The fire exit at the rear is clear and unobstructed",
  "deliverable": "One photo of the rear exit, code in frame",
  "sites": ["store-214", "store-218", "store-301"],
  "fee_minor": 1500,
  "reference": "PO-88431",
  "tier": "V2"
}</code></pre>

<h2>Work that takes more than one visit</h2>
<p>An errand is one trip and one photograph. A driveway is prep, base, binder
and surface over three days, with forty tons of asphalt paid for on the first
morning. Two fields make the difference.</p>
<p><code>work_hours</code> is how long the work takes. Without it a job is held
for 45 minutes and then treated as abandoned — which for a three-day job meant
the crew lost the work they were standing on and the firm was put in cooldown
for finishing it.</p>
<p><code>stages</code> cuts the job into pieces that are each evidenced and
paid as they are done. Their pay must add up to <code>fee_minor</code>.</p>
<pre><code>{
  "kind": "do",
  "predicate": "The driveway is paved and open to traffic",
  "work_hours": 72,
  "fee_minor": 1200000,
  "stages": [
    {"name": "Materials",   "deliverable": "the delivery ticket for the asphalt",
     "pay_minor": 400000, "materials": true},
    {"name": "Prep",        "deliverable": "old surface up and the base graded",
     "pay_minor": 300000},
    {"name": "Base course", "deliverable": "base course laid and rolled",
     "pay_minor": 250000},
    {"name": "Surface",     "deliverable": "the finished surface, rolled and edged",
     "pay_minor": 250000}
  ]
}</code></pre>
<ul>
<li><b>Stages run in order.</b> Nobody surfaces a driveway before the base is
in, and letting the last stage be claimed first would accept a photograph of a
result with nothing underneath it.</li>
<li><b>Each stage is judged against its own deliverable</b>, not the job's
headline. "The driveway is paved" is not true when the base is down, and
refusing honest work for that would be our mistake.</li>
<li><b>A <code>materials</code> stage pays against a receipt</b> rather than
against finished work, so nobody carries your supply costs for the length of
the job.</li>
<li><b>Reporting a stage extends the lease.</b> Somebody working is the
opposite of somebody who walked away.</li>
<li><b>The seat is held until the last stage.</b> A half-paved driveway does not
go back on the board.</li>
</ul>

<h2>Supplying as a business</h2>
<p>A company is not a person, and this exchange used to insist otherwise: one
login, a ceiling of three jobs however many crews you had, payouts to whoever
clicked, and a licence field nothing checked. All four are addressed.</p>
<table>
<tr><th>Endpoint</th><th>What it does</th></tr>
<tr><td>GET /v1/supplier</td><td>Your profile, your ceiling, and what is holding it back</td></tr>
<tr><td>PUT /v1/supplier</td><td>Set your legal name, licences and cover</td></tr>
<tr><td>POST /v1/supplier/members</td><td>Add somebody who may take work for you</td></tr>
<tr><td>DELETE /v1/supplier/members/{person}</td><td>Remove them</td></tr>
<tr><td>GET /v1/statement</td><td>What you earned in a period, line by line</td></tr>
<tr><td>GET /v1/statement.csv</td><td>The same, for your bookkeeper</td></tr>
</table>
<ul>
<li><b>Your crews claim against you.</b> Concurrency, cooldown and standing
belong to the business; the seat and the evidence belong to the technician, so
a buyer still knows which crew came.</li>
<li><b>Licensed trades need a licence we checked.</b> Claiming HVAC is not
enough — a person looks the number up on the issuing register. Editing a
licence clears its verification.</li>
<li><b>Vetting lifts the ceiling</b> from three to twelve, and to forty once you
have a record. It is not self-service.</li>
<li><b>Companies are paid as companies.</b> Set <code>kind: "company"</code> and
the payment provider asks you for an EIN rather than asking an employee for a
social security number.</li>
</ul>

<h2>Verification tiers</h2>
<table>
<tr><th>Tier</th><th>What it requires</th></tr>
<tr><td>V0</td><td>A signed claim, no artifact</td></tr>
<tr><td>V1</td><td>An artifact passing deterministic checks</td></tr>
<tr><td>V2</td><td>V1 plus a challenge code in frame and adjudication</td></tr>
<tr><td>V3</td><td>Two independent sources that agree</td></tr>
</table>
<p><b>Admissible is not the same as done.</b> Verification establishes that the
evidence is tied to this job — the challenge code is legible, the location
matches. A separate adjudication asks whether the photographs actually show
what you asked for. A do-job pays its completion fee only when both hold; one
that is merely admissible pays nothing, and the worker is told to reshoot with
the finished work in frame.</p>
<p>Ask for the tier that matches what a wrong answer would cost you. Higher
tiers cost more and take longer, and the exchange refuses to claim a confidence
it cannot reach.</p>

<h2>MCP</h2>
<p>The exchange ships an MCP server at <code>/mcp</code> so an agent can use
all of this as tools. One URL, two surfaces: the credential decides which. An
agent key gets the buying side; an operator's own session token gets the
supply side.</p>
<pre><code>claude mcp add --transport http lamdis https://exchange.lamdis.ai/mcp \
  --header "Authorization: Bearer lam_sk_..."</code></pre>

<h3>Buying: with an agent key</h3>
<table>
<tr><th>Tool</th><th>What it does</th></tr>
<tr><td>observe_world</td><td>Find out whether something is actually true in the physical world; somebody photographs it, the evidence is checked</td></tr>
<tr><td>do_in_world</td><td>Have something in the physical world made true, by whoever can do it, with proof it happened</td></tr>
<tr><td>find_out</td><td>Pay somebody to go and find something out, and get a structured answer back rather than a photograph</td></tr>
<tr><td>check_feasible</td><td>Whether supply is reachable for a job, before promising anybody it can be done. Costs and holds nothing</td></tr>
<tr><td>request_quotes</td><td>Post a job you do not know the price of and collect offers</td></tr>
<tr><td>list_bids</td><td>The offers on an open job: price, when, and how</td></tr>
<tr><td>accept_bid</td><td>Accept one offer; the amount becomes the price and the work begins</td></tr>
<tr><td>job_status</td><td>Where a job has got to: taken, submitted, checked, paid</td></tr>
<tr><td>job_evidence</td><td>The files somebody brought back, with where each says it was taken</td></tr>
<tr><td>job_receipt</td><td>The signed receipt for a finished job, verifiable without trusting the exchange</td></tr>
<tr><td>cancel_job</td><td>Withdraw a job nobody has taken yet and release its escrow</td></tr>
<tr><td>open_project</td><td>Start a budget envelope several jobs share</td></tr>
<tr><td>project_status</td><td>What a project has cost so far and what is left, job by job</td></tr>
<tr><td>list_project_bids</td><td>Offers covering a whole project at once, priced per piece</td></tr>
<tr><td>accept_project_bid</td><td>Accept one offer covering several jobs, awarded together or not at all</td></tr>
<tr><td>read_stage_plan</td><td>The stage breakdown a supplier proposed for a job whose winner writes the schedule</td></tr>
<tr><td>decide_stage_plan</td><td>Accept a supplier's stage breakdown, or send it back with a reason</td></tr>
<tr><td>sweep_sites</td><td>Describe work once and post it at many of your locations under one budget</td></tr>
<tr><td>list_sites</td><td>This account's locations, with the ids sweep_sites and do_in_world take</td></tr>
<tr><td>list_vendors</td><td>The suppliers this account has approved, with any agreed rates</td></tr>
<tr><td>exchange_balance</td><td>What this agent's account holds, what is committed, and what remains spendable</td></tr>
</table>
<p>There is deliberately no tool to issue a key, raise a limit, connect a payout
account, or submit evidence. An agent cannot widen its own budget or manufacture
the proof it will be judged by.</p>

<h3>Supplying: with an operator's session token</h3>
<p>The same routes the board's own pages call, so an operator's agent and an
operator's browser see the same exchange. Nothing here can be done by an agent
that the person could not do themselves.</p>
<table>
<tr><th>Tool</th><th>What it does</th></tr>
<tr><td>find_work</td><td>What is open right now that this operator could actually take, filtered to their range and qualifications</td></tr>
<tr><td>read_job</td><td>One job in full: what it asks for, what counts as proof, the buyer's photographs, what is blocking it</td></tr>
<tr><td>take_job</td><td>Take a fixed-price job; it is theirs from this moment and the clock starts</td></tr>
<tr><td>place_bid</td><td>Offer a price on an open job, priced from what the operator has said about their rates</td></tr>
<tr><td>read_scope</td><td>A multi-part job in full: every piece, in order, and what is waiting on what</td></tr>
<tr><td>bid_whole_scope</td><td>One offer covering every piece of a multi-part job, awarded together or not at all</td></tr>
<tr><td>propose_stages</td><td>On a job whose winner writes the schedule, propose how it breaks down and what each piece is worth</td></tr>
<tr><td>my_work</td><td>What this operator is holding: which stage each job is on, what is next, what is blocked</td></tr>
<tr><td>my_earnings</td><td>What this operator is owed, what is clear to send, what was objected to, and the bids still out</td></tr>
<tr><td>set_capacity</td><td>Record what this operator will take, how much at once, how far they will go, and where to push offers</td></tr>
<tr><td>give_back</td><td>Hand a job back that this operator cannot do after all, rather than letting it lapse</td></tr>
</table>

<h2>Errors</h2>
<p>Refusals say what to do about them. A job you cannot take tells you which
skill is missing or how far away it is; a key over its limit names the limit,
because the person who set it is the one who decides whether to raise it.</p>
<p>Reading a job that is not yours returns <code>404</code> rather than
<code>403</code> — confirming a job exists is already more than a stranger
should learn.</p>

<h2>Limits worth knowing</h2>
<ul>
<li>Six files per submission. Photographs, video, or audio.</li>
<li>Evidence bytes are held in memory and do not survive a restart. Hashes and
verdicts do.</li>
<li>Amounts are integer minor units. There is no float anywhere in the money
path.</li>
<li>US only for now: dollars, miles, and a skill catalogue of US credentials.</li>
</ul>
<p style="margin-top:2rem"><a href="/board">Board</a> &middot;
<a href="/console">Console</a> &middot;
<a href="/v1/exchange">Machine-readable summary</a></p>
</main>
`

// RegisterDocs mounts the developer page and the machine-readable pointers a
// client looks for before asking a human.
func RegisterDocs(mux *http.ServeMux) {
	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Referrer-Policy", "no-referrer")
		fmt.Fprint(w, docsPageHTML)
	})
	// An agent reading the site rather than the docs should still find its way
	// in. Cheap to serve, and the alternative is it guessing.
	mux.HandleFunc("GET /llms.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, llmsTXT)
	})
}

const llmsTXT = `# Lamdis Exchange

Infrastructure for getting things done in the physical world: an agent states
what should become true, holds the money for it, and settles against verified
evidence that it happened.

## How to use it
- API reference: /docs
- Machine-readable summary: /v1/exchange
- Sign in to get a key: /signin
- Open work: /board

## Authentication
Agent keys begin with lam_sk_ and are issued by a signed-in person from
/console. REST routes (/v1/...) take the key as a header:
  X-Lamdis-Key: lam_sk_...
The MCP endpoint (/mcp) takes Authorization: Bearer lam_sk_... and accepts
X-Lamdis-Key as well.

## Core endpoints
POST /v1/tasks                     post a job
GET  /v1/jobs/{job}                where it stands
GET  /v1/jobs/{job}/evidence       the files that came back
GET  /v1/jobs/{job}/receipt        signed, independently verifiable
GET  /v1/jobs/{job}/bids           offers on an open job (buyer only)
POST /v1/jobs/{job}/award          accept one
GET  /v1/agent/balance             what this key may still spend
POST /v1/balance/topup             add funds

## MCP
One endpoint, /mcp, two surfaces chosen by credential.
Buying, with an agent key: observe_world, do_in_world, find_out,
check_feasible, request_quotes, list_bids, accept_bid, job_status,
job_evidence, job_receipt, cancel_job, open_project, project_status,
list_project_bids, accept_project_bid, read_stage_plan, decide_stage_plan,
sweep_sites, list_sites, list_vendors, exchange_balance.
Supplying, with an operator's session token: find_work, read_job, take_job,
place_bid, read_scope, bid_whole_scope, propose_stages, my_work, my_earnings,
set_capacity, give_back.

There is no tool to issue a key, raise a spending limit, connect a payout
account, or submit evidence. An agent cannot widen its own budget or
manufacture the proof it will be judged by.

## Notes
Money is in integer minor units. Amounts are USD, distances are miles.
Verification tiers V0-V3 are defined at /v1/exchange.
`
