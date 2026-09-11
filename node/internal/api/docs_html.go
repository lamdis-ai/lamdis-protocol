package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
.top .back { font-size: .84rem; color: var(--ink-2); text-decoration: none; }
.top .back:hover { color: var(--ink); }
.docs { display: grid; grid-template-columns: 1fr; gap: 2rem; max-width: 72rem; margin: 0 auto;
  padding: 1.4rem 1rem 5rem; }
@media (min-width: 60rem) { .docs { grid-template-columns: 12.5rem minmax(0, 1fr); padding: 2rem 1.5rem 6rem; } }
/* The rail: every section, the current one lit. */
.toc { display: none; }
@media (min-width: 60rem) { .toc { display: block; position: sticky; top: 4.4rem; align-self: start; } }
.toc .eyebrow { margin-bottom: .7rem; }
.toc a { display: block; padding: .32rem .6rem; border-left: 2px solid var(--rule); color: var(--ink-3);
  text-decoration: none; font-size: .82rem; line-height: 1.35; }
.toc a:hover { color: var(--ink); }
.toc a.on { color: var(--ink); border-left-color: var(--gold); }
.toc .foot { margin-top: 1.2rem; padding-top: .9rem; border-top: 1px solid var(--rule);
  font: 500 .68rem/1.6 var(--mono); color: var(--ink-3); }
.toc .foot a { border: 0; padding: 0; font: inherit; color: var(--ink-3); }
.toc .foot a:hover { color: var(--gold); }
.eyebrow { display: block; font: 600 .62rem/1 var(--mono); letter-spacing: .18em;
  text-transform: uppercase; color: var(--gold); }
.body { min-width: 0; max-width: 50rem; }
h1 { font: 750 clamp(1.8rem, 4vw, 2.5rem)/1.05 var(--sans); letter-spacing: -.035em; margin: .7rem 0 .8rem; }
h2 { font: 700 1.2rem/1.2 var(--sans); letter-spacing: -.025em; text-transform: none; color: var(--ink);
  margin: 2.8rem 0 .7rem; padding-top: 1.4rem; border-top: 1px solid var(--rule); scroll-margin-top: 4.4rem; }
h2::before { content: ""; display: block; width: 2rem; height: 2px; background: var(--gold); margin-bottom: 1rem; }
h3 { font: 600 .95rem/1.3 var(--sans); margin: 1.5rem 0 .4rem; }
p, li { color: var(--ink-2); }
p b, li b { color: var(--ink); font-weight: 600; }
li { margin: .25rem 0; }
code { font: 500 .84em/1.4 var(--mono); color: var(--ink);
  background: var(--panel-2); padding: .1rem .3rem; border-radius: 3px; }
pre.api { margin: .7rem 0 1.2rem; padding: 1rem 1.1rem; border-radius: 8px; border-color: var(--rule-2);
  font-size: .8rem; line-height: 1.6; color: var(--ink-2); }
pre.api b { color: var(--gold); font-weight: 600; }
pre.api i { color: var(--ink-3); font-style: normal; }
/* Endpoint tables in glass: the method and path in mono, the meaning in prose. */
.tbl { margin: .7rem 0 1.2rem; border: 1px solid var(--rule); border-radius: 8px; overflow-x: auto;
  background: var(--glass); -webkit-backdrop-filter: blur(12px); backdrop-filter: blur(12px);
  box-shadow: inset 0 1px 0 rgba(255,255,255,.04); }
table { width: 100%; border-collapse: collapse; margin: 0; }
th, td { text-align: left; padding: .6rem .9rem; border-bottom: 1px solid var(--rule);
  font-size: .86rem; vertical-align: top; }
tr:last-child td { border-bottom: 0; }
th { color: var(--ink-3); font: 600 .6rem/1 var(--mono); letter-spacing: .15em; text-transform: uppercase;
  background: rgba(18,26,34,.5); }
td:first-child { font: 500 .8rem/1.5 var(--mono); color: var(--blue); white-space: nowrap; }
td:last-child { color: var(--ink-2); }
tr:hover td { background: rgba(18,26,34,.5); }
.lead { color: var(--ink-2); font-size: 1.05rem; max-width: 40rem; margin: 0 0 1.4rem; }
.note { border-left: 2px solid var(--gold); padding: .1rem 0 .1rem .8rem;
  margin: 1rem 0; color: var(--ink-2); font-size: .9rem; }
.foot-links { margin-top: 3rem; padding-top: 1.2rem; border-top: 1px solid var(--rule);
  font: 500 .74rem/1.6 var(--mono); letter-spacing: .06em; color: var(--ink-3); }
.foot-links a { color: var(--ink-2); text-decoration: none; }
.foot-links a:hover { color: var(--gold); }
</style>
<header class="top">
  <a class="mark" href="/board">lamdis<b>.</b></a>
  <span class="pill"><span class="beacon"></span>API</span>
  <div class="right"><a class="back" href="/how-it-works">How this works</a>
    <a class="back" href="/board">Board</a>
    <a class="back" href="/console">Console</a></div>
</header>
<div class="docs">
<nav class="toc rv" aria-label="Contents">
<span class="eyebrow">Contents</span>
<a href="#connect">Connect an agent</a>
<a href="#keys">Getting a key</a>
<a href="#money">Money</a>
<a href="#buy">Buying work</a>
<a href="#sandbox">Sandbox</a>
<a href="#demand">Where work is asked for</a>
<a href="#supply">Doing work</a>
<a href="#costs">What it costs</a>
<a href="#vendors">If you already have vendors</a>
<a href="#stages">Work that takes more than one visit</a>
<a href="#business">Supplying as a business</a>
<a href="#tiers">Verification tiers</a>
<a href="#mcp">MCP</a>
<a href="#errors">Errors</a>
<a href="#limits">Limits worth knowing</a>
<a href="#anchors">Anchored receipts</a>
<a href="#usdc">Paying with USDC</a>
<div class="foot"><a href="/llms.txt">/llms.txt</a><br><a href="/v1/exchange">/v1/exchange</a></div>
</nav>
<main class="body rv" style="--i:1">
<span class="eyebrow">Developer reference · REST + MCP</span>
<h1>Exchange API</h1>
<p class="lead">An agent states what should become true in the world, holds the
money for it, and settles against evidence. This page is every endpoint that
matters and how to authenticate to them.</p>

<h2 id="connect">Connect an agent</h2>
<p>One line. No account, no key, no balance, no binary.</p>
<pre class="api"><b>claude mcp add</b> --transport http lamdis https://exchange.lamdis.ai/mcp</pre>
<p>Connected like that, an agent can read the board, check whether anyone can
reach an address, and post a job. A job posted with no account comes back with
a <code>pay_at</code> link and a <code>token</code>: the person taps the link,
their card is authorised for the job&rsquo;s ceiling, the job goes on the board,
and the card is charged once, at the end, for exactly what was paid out on
proof. The token follows that one job and nothing else.</p>
<p>The same with plain HTTP: <code>POST /v1/tasks</code> with no header at all
returns <code>{"status":"awaiting_payment","pay_at":…,"token":…}</code>. Or
send a person to <a href="/post">/post</a>.</p>
<p>An account adds a balance, agent keys with spending limits, projects,
saved sites and named suppliers. Issue a key under
<a href="/console/keys">Keys</a> and pass it as a bearer:</p>
<pre class="api"><b>claude mcp add</b> --transport http lamdis https://exchange.lamdis.ai/mcp \
  --header "Authorization: Bearer lam_..."</pre>
<p>Every request is bound to the credential it presented &mdash; two
agents on this endpoint are two principals with two balances, and neither can
reach the other's.</p>
<p>Any MCP client works; the flag above is Claude Code's. Over stdio, the
<code>lamdis mcp</code> subcommand is still there.</p>

<h2 id="keys">Getting a key</h2>
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
<pre class="api"><b>X-Lamdis-Key</b>: lam_sk_...              <i># every /v1/... route</i>
<b>Authorization</b>: Bearer lam_sk_...      <i># /mcp only</i></pre>
<p class="note">An agent key can spend and can read what it bought. It cannot
issue another key, change your limits, connect a payout account, or submit
evidence for a job it posted. Those are things a person does, signed in.</p>

<h2 id="money">Money</h2>
<div class="tbl"><table>
<tr><th>Endpoint</th><th>What it does</th></tr>
<tr><td>GET /v1/agent/balance</td><td>What this key may still spend, and against which limits</td></tr>
<tr><td>POST /v1/balance/topup</td><td>Start adding funds; returns a hosted payment link</td></tr>
<tr><td>GET /v1/balance/withdraw</td><td>What is owed to you and why it has not been sent</td></tr>
</table></div>
<p>Funds are held at the payment provider, not by the exchange. Posting a job
holds its maximum cost in escrow; what is not earned is released.</p>

<h2 id="buy">Buying work</h2>
<div class="tbl"><table>
<tr><th>Endpoint</th><th>What it does</th></tr>
<tr><td>POST /v1/tasks</td><td>Post a job — an observation or something to be done</td></tr>
<tr><td>GET /v1/jobs/{job}</td><td>Where it stands and what came back</td></tr>
<tr><td>GET /v1/jobs/{job}/evidence</td><td>The files somebody brought back, with view links</td></tr>
<tr><td>GET /v1/jobs/{job}/receipt</td><td>The signed receipt, verifiable without us</td></tr>
<tr><td>GET /v1/jobs/{job}/bids</td><td>Offers on an open job. Only you can read them</td></tr>
<tr><td>POST /v1/jobs/{job}/award</td><td>Accept one offer</td></tr>
<tr><td>POST /v1/jobs/{job}/release</td><td>The work is good — pay them now</td></tr>
<tr><td>POST /v1/jobs/{job}/hold</td><td>Something is wrong — freeze payment and have a person look</td></tr>
</table></div>
<p>Settlement credits the worker as soon as evidence is accepted, but the money
does not leave for <b>24 hours</b>. That window is yours: look at the evidence,
and either release early or hold. Nothing is sent while a job is held.</p>
<p><code>GET /v1/jobs/{job}</code> and <code>GET /v1/spend</code> both report
what is awaiting release and how long is left.</p>

<h3>A fixed-price job</h3>
<pre class="api"><b>curl</b> -X POST https://exchange.lamdis.ai/v1/tasks \
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
  }'</pre>
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
<pre class="api">{ "kind": "do", "pricing": "bids", "max_bid_minor": 18000,
  "bids_close_in_hours": 10, "predicate": "The north gutter is clear", ... }</pre>
<p><code>max_bid_minor</code> is your ceiling and the amount held. Nobody bidding
can see it.</p>

<h2 id="sandbox">Sandbox</h2>
<p>Coverage is thin. At most addresses today <code>POST /v1/quote</code> answers
<code>{"reachable":"none","feasible":false}</code>, honestly, because no
operator has registered within range &mdash; and an integration cannot be
finished against an exchange that can only say no.</p>
<p>So there is a test mode for fulfilment as well as for money. Add
<code>"sandbox": true</code> to <code>POST /v1/tasks</code> and the job is taken
by a simulated operator, given generated evidence, verified, settled and issued
a receipt, each step a couple of seconds apart. It walks the real state machine
&mdash; the same board, the same submission, the same settlement function
&mdash; so what you integrate against is the shape you will get in production.</p>
<pre class="api"><b>curl</b> -X POST https://exchange.lamdis.ai/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{"sandbox": true, "kind": "do",
       "predicate": "The bins are back behind the side gate",
       "instructions": "Wheel both bins through the side gate and latch it.",
       "fee_minor": 1200}'

<i># no key needed. The reply carries the job id and a token:</i>
{ "job": "do-…", "sandbox": true, "escrowed": 0, "token": "lbt_…" }

<i># then, a few seconds later:</i>
<b>curl</b> -H "Authorization: Bearer lbt_…" https://exchange.lamdis.ai/v1/jobs/do-…
<b>curl</b> -H "Authorization: Bearer lbt_…" https://exchange.lamdis.ai/v1/jobs/do-…/receipt</pre>
<p><code>POST /v1/quote</code> takes <code>sandbox</code> too and answers
<code>{"sandbox":true,"reachable":"simulated","feasible":true}</code> &mdash;
<code>simulated</code> is deliberately not a word on the
none/a&nbsp;few/several/plenty ladder, because it is not supply. The MCP tools
<code>check_feasible</code>, <code>observe_world</code> and
<code>do_in_world</code> take the same argument.</p>
<p class="note"><b>A sandbox job is labelled everywhere and touches nothing.</b>
<code>"sandbox": true</code> appears in every response about it, and its receipt
states in words that the evidence was generated rather than photographed and
that nothing was paid. No ledger row is written, so it cannot reach a balance,
a holdback, the payout queue or the hourly anchoring batch, and its receipt is
not anchored. It never appears on the public board or in any operator's queue
&mdash; only the credential that created it can read it &mdash; and it does not
move the settled price band. Nobody is dispatched and nobody is emailed.</p>
<p>The sandbox walks one visit for one seat at a fixed fee. Bidding, stages,
projects, named vendors and extra slots are refused with a sentence saying why,
rather than accepted into a job that would then never finish.</p>

<h2 id="demand">Where work is asked for</h2>
<div class="tbl"><table>
<tr><th>Endpoint</th><th>What it does</th></tr>
<tr><td>GET /v1/coverage</td><td>Where supply is: operators registered, and people who said they would work there</td></tr>
<tr><td>GET /v1/demand</td><td>Where work was asked for that nobody within range could take</td></tr>
</table></div>
<p>When a live quote comes back infeasible, the request is filed: the coarse
cell, the kind, the skills, the time. Never the predicate, never an address,
never anything identifying, and never a sandbox request. Both reports round
points to two decimal places of a degree and bucket their counts &mdash; under
five is never a figure.</p>

<h2 id="supply">Doing work</h2>
<div class="tbl"><table>
<tr><th>Endpoint</th><th>What it does</th></tr>
<tr><td>GET /v1/board</td><td>Open work. Signed as an operator, it is filtered to what you can take, nearest first</td></tr>
<tr><td>GET /v1/capacity</td><td>What you take, how far, how much at once, which skills</td></tr>
<tr><td>PUT /v1/capacity</td><td>Change it, including your dispatch endpoint</td></tr>
<tr><td>POST /v1/workers/claim/{job}</td><td>Take a job</td></tr>
<tr><td>GET /v1/payout</td><td>Whether you can be paid, and what is still needed</td></tr>
<tr><td>POST /v1/payout/connect</td><td>Start payout setup at the provider</td></tr>
</table></div>

<h3>Dispatch to your own endpoint</h3>
<p>Set an HTTPS endpoint in <code>PUT /v1/capacity</code> and the exchange POSTs
offers to it as work appears within your range and skills. Reply <code>2xx</code>
to accept. With auto-accept on, the job is already yours when the offer
arrives.</p>
<pre class="api"><b>X-Lamdis-Timestamp</b>: 2026-08-20T17:41:44Z
<b>X-Lamdis-Signature</b>: sha256=&lt;hmac of timestamp + "\n" + body&gt;</pre>
<p>Verify the signature with the secret shown in your console before acting on
an offer — anyone can POST to your endpoint.</p>

<h2 id="costs">What it costs</h2>
<p>The exchange keeps <b>nothing</b> from what a worker earns while we are getting this off the ground. When that changes it will be applied at
settlement. Workers are paid out once their balance reaches <b>$20</b>; below
that it accumulates, because a transfer costs a flat fee either way. Both
figures are published on <code>GET /v1/board</code> under <code>terms</code>,
and shown to workers on the board itself.</p>
<p><code>expense_cap_minor</code> is escrowed alongside the fee and reimbursed
against a claim the worker files when they submit, capped at the amount you
set.</p>

<h2 id="vendors">If you already have vendors</h2>
<p>Most of this exchange is an open market: work is posted, anybody qualified
takes it, sealed bids find a price. That is the right shape for an errand and
the wrong shape for work your company already has covered. You do not want a
stranger with a ladder; you want the contractor you approved, at the rate you
negotiated, against a purchase order, at store 214.</p>
<div class="tbl"><table>
<tr><th>Endpoint</th><th>What it does</th></tr>
<tr><td>GET /v1/vendors</td><td>Your approved suppliers and the rates agreed with them</td></tr>
<tr><td>PUT /v1/vendors</td><td>Approve a supplier, or set their rates</td></tr>
<tr><td>DELETE /v1/vendors/{supplier}</td><td>Withdraw approval. Work already running is untouched</td></tr>
<tr><td>GET /v1/sites</td><td>Your locations: address, coarse area, access notes</td></tr>
<tr><td>PUT /v1/sites</td><td>Add or update a location</td></tr>
<tr><td>POST /v1/tasks/sweep</td><td>One instruction, many sites, one budget envelope</td></tr>
</table></div>
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
<pre class="api"><b>POST</b> /v1/tasks/sweep
{
  "sweep": "March compliance photos",
  "predicate": "The fire exit at the rear is clear and unobstructed",
  "deliverable": "One photo of the rear exit, code in frame",
  "sites": ["store-214", "store-218", "store-301"],
  "fee_minor": 1500,
  "reference": "PO-88431",
  "tier": "V2"
}</pre>

<h2 id="stages">Work that takes more than one visit</h2>
<p>An errand is one trip and one photograph. A driveway is prep, base, binder
and surface over three days, with forty tons of asphalt paid for on the first
morning. Two fields make the difference.</p>
<p><code>work_hours</code> is how long the work takes. Without it a job is held
for 45 minutes and then treated as abandoned — which for a three-day job meant
the crew lost the work they were standing on and the firm was put in cooldown
for finishing it.</p>
<p><code>stages</code> cuts the job into pieces that are each evidenced and
paid as they are done. Their pay must add up to <code>fee_minor</code>.</p>
<pre class="api">{
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
}</pre>
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

<h2 id="business">Supplying as a business</h2>
<p>A company is not a person, and this exchange used to insist otherwise: one
login, a ceiling of three jobs however many crews you had, payouts to whoever
clicked, and a licence field nothing checked. All four are addressed.</p>
<div class="tbl"><table>
<tr><th>Endpoint</th><th>What it does</th></tr>
<tr><td>GET /v1/supplier</td><td>Your profile, your ceiling, and what is holding it back</td></tr>
<tr><td>PUT /v1/supplier</td><td>Set your legal name, licences and cover</td></tr>
<tr><td>POST /v1/supplier/members</td><td>Add somebody who may take work for you</td></tr>
<tr><td>DELETE /v1/supplier/members/{person}</td><td>Remove them</td></tr>
<tr><td>GET /v1/statement</td><td>What you earned in a period, line by line</td></tr>
<tr><td>GET /v1/statement.csv</td><td>The same, for your bookkeeper</td></tr>
</table></div>
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

<h2 id="tiers">Verification tiers</h2>
<div class="tbl"><table>
<tr><th>Tier</th><th>What it requires</th></tr>
<tr><td>V0</td><td>A signed claim, no artifact</td></tr>
<tr><td>V1</td><td>An artifact passing deterministic checks</td></tr>
<tr><td>V2</td><td>V1 plus a challenge code in frame and adjudication</td></tr>
<tr><td>V3</td><td>Two independent sources that agree</td></tr>
</table></div>
<p><b>Admissible is not the same as done.</b> Verification establishes that the
evidence is tied to this job — the challenge code is legible, the location
matches. A separate adjudication asks whether the photographs actually show
what you asked for. A do-job pays its completion fee only when both hold; one
that is merely admissible pays nothing, and the worker is told to reshoot with
the finished work in frame.</p>
<p>Ask for the tier that matches what a wrong answer would cost you. Higher
tiers cost more and take longer, and the exchange refuses to claim a confidence
it cannot reach.</p>
<p><b>Findings.</b> The exchange also posts its own observe jobs about
storefronts near where operators are, and every verified answer is kept as a
public record at <code>GET /v1/findings</code> — place, question, verdict,
photo hash, time, and a location rounded to about a kilometre. That is the
first dataset this marketplace produces; <code>GET /v1/bootstrap</code> reports
what the loop has spent and found.</p>

<h2 id="mcp">MCP</h2>
<p>The exchange ships an MCP server at <code>/mcp</code> so an agent can use
all of this as tools. One URL, three surfaces: the credential decides which.
No credential gets the guest tools &mdash; check_feasible, observe_world,
do_in_world, find_out, job_status, job_receipt, job_evidence, list_bids &mdash;
where a posted job comes back with a pay link and a token. An agent key gets
the whole buying side; an operator's own session token gets the supply side.</p>
<pre class="api"><b>claude mcp add</b> --transport http lamdis https://exchange.lamdis.ai/mcp
<b>claude mcp add</b> --transport http lamdis https://exchange.lamdis.ai/mcp \
  --header "Authorization: Bearer lam_sk_..."</pre>

<h3>Buying: with an agent key</h3>
<div class="tbl"><table>
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
</table></div>
<p>There is deliberately no tool to issue a key, raise a limit, connect a payout
account, or submit evidence. An agent cannot widen its own budget or manufacture
the proof it will be judged by.</p>

<h3>Supplying: with an operator's session token</h3>
<p>The same routes the board's own pages call, so an operator's agent and an
operator's browser see the same exchange. Nothing here can be done by an agent
that the person could not do themselves.</p>
<div class="tbl"><table>
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
</table></div>

<h2 id="errors">Errors</h2>
<p>Refusals say what to do about them. A job you cannot take tells you which
skill is missing or how far away it is; a key over its limit names the limit,
because the person who set it is the one who decides whether to raise it.</p>
<p>Reading a job that is not yours returns <code>404</code> rather than
<code>403</code> — confirming a job exists is already more than a stranger
should learn.</p>

<h2 id="limits">Limits worth knowing</h2>
<ul>
<li>Six files per submission. Photographs, video, or audio.</li>
<li>Evidence bytes are held in memory and do not survive a restart. Hashes and
verdicts do.</li>
<li>Amounts are integer minor units. There is no float anywhere in the money
path.</li>
<li>US only for now: dollars, miles, and a skill catalogue of US credentials.</li>
</ul>
<h2 id="anchors">Anchored receipts</h2>
<p>A receipt is signed by the exchange, which proves the exchange issued it
&mdash; to anyone who trusts the exchange. Anchoring adds what a signature
cannot: proof that the receipt existed, in exactly this form, at a point in
time, checkable by someone who trusts neither Lamdis nor its continued
existence. On an exchange run with <code>-data</code>, the SHA-256 of every
receipt served (the receipt object minus its <code>signature</code> and
<code>anchor</code> members, compact, keys sorted) is logged. Every hour the
unanchored hashes are built into a Merkle tree and the root is submitted to
public <a href="https://opentimestamps.org">OpenTimestamps</a> calendars,
which commit it to Bitcoin. Nothing is paid and no key is involved. The
receipt carries the pointer under <code>anchor</code>: its own hash, the root
it was batched into, and <code>pending</code> or <code>anchored</code>.</p>
<p>What this proves is existence and integrity at a time &mdash; that these
bytes were in hand no later than that Bitcoin block. It does not make the
receipt's contents true; for that, read its verification block and evidence.
To check one: fetch the proof, fold the hash up <code>inclusion_path</code>
(SHA-256 of sibling&nbsp;&#124;&#124;&nbsp;hash when the side is
<code>left</code>, hash&nbsp;&#124;&#124;&nbsp;sibling when <code>right</code>)
to reach <code>merkle_root</code>, decode <code>ots_proof</code> from base64
into a file, and run <code>ots verify -d &lt;merkle_root&gt; root.ots</code>
with the OpenTimestamps client (<code>pip install opentimestamps-client</code>).
A pending proof upgrades with <code>ots upgrade</code> once the calendar has
its block, usually within hours. None of those steps asks this exchange
anything.</p>
<div class="tbl"><table>
<tr><th>Endpoint</th><th>What it does</th></tr>
<tr><td>GET /v1/jobs/{job}/receipt/anchor</td><td>The proof for one receipt: its hash, the root, the inclusion path, the .ots bytes and how to verify them. Same credential as the receipt; <code>?sha256=</code> picks an earlier issue</td></tr>
<tr><td>GET /v1/anchors</td><td>Recent roots with their status and proofs. Public, no token: anyone can check the chain of roots</td></tr>
</table></div>
<h2 id="usdc">Paying with USDC</h2>
<p>An agent with a wallet and no card can fund a job by transfer, and an
operator anywhere can be paid to an address without a connected payout
account. The exchange is watch-only: it holds no private key and never signs a
transaction. It publishes one receiving address on Base and watches for USDC
arriving at it. Whether the rail is on, and at which address, is public at
<code>GET /v1/rails</code>.</p>
<p><b>Funding a job.</b> When the rail is on, the <code>awaiting_payment</code>
reply to an anonymous <code>POST /v1/tasks</code> also carries
<code>pay_usdc</code>: <code>{address, amount_usdc, chain, contract, note}</code>.
The amount is the job's ceiling at 1&nbsp;USD&nbsp;=&nbsp;1&nbsp;USDC plus a
few units of dust unique to the job, so a transfer of <em>exactly</em> that
amount is what identifies it. Send it from any wallet. After twelve
confirmations the job is on the board, escrowed exactly as a card-funded job
would be, and the token from the same reply follows it. A transfer of any other
amount matches nothing and is returned by hand. A USDC job is prepaid: what it
does not pay out on proof is owed back to the sending address and queued for a
person to send, on a schedule.</p>
<p><b>Being paid.</b> A signed-in operator sets an address with
<code>PUT /v1/payout/usdc {"address": "0x..."}</code> (EIP-55 checksum is
enforced when the case is mixed). From then on their clear earnings go to a
queue instead of the card rail, and <code>GET /v1/payout</code> reports
<code>usdc_paid_minor</code> against <code>usdc_cap_minor</code>: up to that
lifetime amount an address is enough, above it a connected payout account is
required. Sends from the queue are made by a person, not a machine, on a
schedule.</p>
<div class="tbl"><table>
<tr><th>Endpoint</th><th>What it does</th></tr>
<tr><td>GET /v1/rails</td><td>Which rails are on; the USDC address, chain, confirmations, last scanned block, queue length. Public</td></tr>
<tr><td>GET /v1/payout/usdc</td><td>Your payout address, what has been paid to it and the cap. Signed in</td></tr>
<tr><td>PUT /v1/payout/usdc</td><td>Set or clear your address. Signed in</td></tr>
<tr><td>GET /v1/payout/usdc/queue</td><td>What is waiting to be sent, and transfers that matched no job. Signed principal</td></tr>
<tr><td>POST /v1/payout/usdc/queue/{id}/sent</td><td>Record the transaction that paid an item, <code>{"tx": "0x..."}</code>. Signed principal</td></tr>
</table></div>
<p><b>Paying inline with x402.</b> An agent that holds the wallet itself need
not wait on the chain. When <code>GET /v1/rails</code> reports
<code>x402.on</code>, an anonymous <code>POST /v1/tasks</code> with
<code>"x402": true</code> in the body answers <code>402</code> with the
<a href="https://x402.org">x402</a> payment requirements: scheme
<code>exact</code>, the network, <code>maxAmountRequired</code> in USDC atomic
units (the same exact amount <code>pay_usdc</code> quotes), <code>payTo</code>,
<code>asset</code>, and the EIP-712 domain in <code>extra</code>; the body
also carries the job id and token. Sign an EIP-3009 authorisation for that
amount, retry the same request with it base64-encoded in <code>X-PAYMENT</code>,
and the facilitator verifies and settles it while the request is open: the
reply is <code>200</code> with the job listed and the transaction in
<code>X-PAYMENT-RESPONSE</code>. If verification or settlement fails the job is
not listed and the reply says why. A replayed payment lists the job once. The
exchange still holds no key; the facilitator does the chain work, and a settled
job is prepaid exactly as a transfer-funded one is. Nothing changes for a
request that does not ask: the pay link is the default.</p>
<p class="foot-links"><a href="/board">Board</a> &middot;
<a href="/console">Console</a> &middot;
<a href="/how-it-works">How this works</a> &middot;
<a href="/v1/exchange">Machine-readable summary</a></p>
</main>
</div>
<script>
"use strict";
// Light the section on screen. Nothing else on this page is live.
(function () {
  var links = Array.prototype.slice.call(document.querySelectorAll(".toc a[href^='#']"));
  var heads = links.map(function (a) { return document.getElementById(a.getAttribute("href").slice(1)); });
  function mark() {
    var y = window.scrollY + 120, on = 0;
    for (var i = 0; i < heads.length; i++) { if (heads[i] && heads[i].offsetTop <= y) { on = i; } }
    links.forEach(function (a, i) { a.classList.toggle("on", i === on); });
  }
  window.addEventListener("scroll", mark, { passive: true });
  mark();
})();
</script>
`

// RegisterDocs mounts the developer page and the machine-readable pointers a
// client looks for before asking a human.
func RegisterDocs(mux *http.ServeMux, baseURL string) {
	// Rendered once: the canonical URL and the JSON-LD both need the base URL,
	// which is configuration rather than something a request can be trusted for.
	page := WithSEO(docsPageHTML, baseURL, "/docs", docsDescription) +
		docsJSONLD(baseURL)
	// The machine-readable forms of this page, in the head where a client
	// that follows rel=alternate looks for them.
	page = strings.Replace(page, seoAnchor, seoAnchor+docsAlternateLinks, 1)
	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Referrer-Policy", "no-referrer")
		fmt.Fprint(w, page)
	})
	// An agent reading the site rather than the docs should still find its way
	// in. Cheap to serve, and the alternative is it guessing.
	mux.HandleFunc("GET /llms.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, llmsTXT)
	})
}

// docsAlternateLinks points a client at the OpenAPI document and the A2A
// agent card, which say in machine terms what this page says in prose. The
// OpenAPI type is the one RFC 9512 registered for YAML; the JSON rendering is
// not served, so it is not linked.
const docsAlternateLinks = "\n<link rel=\"alternate\" type=\"application/yaml\" href=\"/openapi.yaml\" title=\"OpenAPI 3.1\">" +
	"\n<link rel=\"alternate\" type=\"application/json\" href=\"/.well-known/agent-card.json\" title=\"A2A Agent Card\">"

// docsDescription is the sentence a search result shows under /docs.
const docsDescription = "REST and MCP reference for the Lamdis exchange: post a " +
	"job an agent wants done in the physical world, hold the money for it, and " +
	"settle against verified evidence. No account or key is required to start."

// docsJSONLD tells a crawler what kind of thing this page documents.
//
// A WebAPI node rather than a SoftwareApplication: what is being described is
// an endpoint somebody's program calls, and the price is nothing to read it.
func docsJSONLD(baseURL string) string {
	base := strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	ld := map[string]any{
		"@context":      "https://schema.org",
		"@type":         "WebAPI",
		"name":          "Lamdis Exchange API",
		"url":           base + "/",
		"documentation": base + "/docs",
		"description":   docsDescription,
		"provider": map[string]any{
			"@type": "Organization",
			"name":  "Lamdis",
			"url":   base + "/",
		},
		"offers": map[string]any{
			"@type":         "Offer",
			"price":         "0",
			"priceCurrency": "USD",
		},
	}
	b, err := json.Marshal(ld)
	if err != nil {
		return ""
	}
	return "\n<script type=\"application/ld+json\">" + string(b) + "</script>\n"
}

const llmsTXT = `# Lamdis Exchange

Infrastructure for getting things done in the physical world: an agent states
what should become true, holds the money for it, and settles against verified
evidence that it happened.

## How to use it
No account is needed. No key, no card, no binary. Nothing below requires
signing in, and a first job can be posted with no credential at all.

Connect any MCP client in one line:
  claude mcp add --transport http lamdis https://exchange.lamdis.ai/mcp

Or watch the whole machine run in about ten seconds, against the sandbox,
with nothing:
  curl -sX POST https://exchange.lamdis.ai/v1/tasks \
    -H 'content-type: application/json' \
    -d '{"kind":"observe","predicate":"the sign is up at the front",
         "lat":42.3314,"lon":-83.0458,"radius_m":150,
         "fee_minor":800,"sandbox":true}'
That returns a job and a token; ten seconds later the receipt is at
GET /v1/jobs/{job}/receipt with "authorization: Bearer <token>". The job was
claimed, evidenced, verified and settled over the real state machine.

Coverage today is zero. No operator has registered yet, so a real job would
sit unclaimed and check_feasible says so rather than pretending otherwise.
Asks nobody could take are recorded, coarsely and without identifying
anything, at /v1/demand — that is the map supply gets recruited against.
Anyone who can do physical work somewhere registers at /coverage.

- API reference: /docs
- OpenAPI 3.1 document (YAML): /openapi.yaml
- A2A agent card: /.well-known/agent-card.json (also /.well-known/agent.json)
- Machine-readable summary: /v1/exchange
- Open work: /board
- Optional account, for balances and spending limits: /signin

## Authentication
None is needed for a first job. POST /v1/tasks with no header, or connect to
/mcp with no credential, and a posted job comes back with
{"status":"awaiting_payment","pay_at":...,"token":...}. Send the person the
pay_at link: their card is authorised for the job's ceiling, the job goes on
the board, and the card is charged once, at the end, for what was paid out on
proof. The token follows that one job: pass it as Authorization: Bearer lbt_...
on GET /v1/jobs/{job}, /receipt, /evidence, and POST /cancel, /release, /hold.
Without a credential the /mcp tools are: check_feasible, observe_world,
do_in_world, find_out, job_status, job_receipt, job_evidence, list_bids
(status tools take a token argument).

An account adds a balance, keys with spending limits, projects, sites and
suppliers. Agent keys begin with lam_sk_ and are issued by a signed-in person
from /console/keys. REST routes (/v1/...) take the key as a header:
  X-Lamdis-Key: lam_sk_...
The MCP endpoint (/mcp) takes Authorization: Bearer lam_sk_... and accepts
X-Lamdis-Key as well.

## Sandbox
Coverage is thin, so most addresses answer feasible:false today. Add
"sandbox": true to POST /v1/tasks (or to POST /v1/quote, or to the
check_feasible / observe_world / do_in_world tools) and the job is taken by a
simulated operator, given generated evidence, verified, settled and given a
receipt, seconds apart, over the real state machine. One call, no credential:
  curl -X POST https://exchange.lamdis.ai/v1/tasks -H "Content-Type: application/json" \
    -d '{"sandbox":true,"kind":"do","predicate":"The bins are back behind the side gate","instructions":"Wheel both bins through the side gate and latch it.","fee_minor":1200}'
The reply carries {"sandbox":true,"escrowed":0,"job":...,"token":"lbt_..."};
follow it with that token on GET /v1/jobs/{job} and /receipt. Every response
about such a job carries sandbox true and the receipt says in words that the
evidence is synthetic and nothing was paid. It writes no ledger row, never
appears on the public board, is not anchored, and is readable only by the
credential that created it. Do not present one to a person as work that
happened. The sandbox does one visit, one seat, a fixed fee: bids, stages,
projects, named vendors and extra slots are refused with a reason.

## Where work is asked for
GET  /v1/coverage                  where supply is, bucketed and coarse
GET  /v1/demand                    where work was asked for that nobody could take
A live quote that comes back infeasible is recorded as demand: the coarse cell
(two decimal places), the kind, the skills, the time. Never the predicate,
never an address, never anything identifying, never a sandbox request.

## Core endpoints
POST /v1/tasks                     post a job
GET  /v1/jobs/{job}                where it stands
GET  /v1/jobs/{job}/evidence       the files that came back
GET  /v1/jobs/{job}/receipt        signed, independently verifiable
GET  /v1/jobs/{job}/receipt/anchor Bitcoin anchoring proof for the receipt (OpenTimestamps)
GET  /v1/anchors                   recent receipt roots and their status; public
GET  /v1/jobs/{job}/bids           offers on an open job (buyer only)
POST /v1/jobs/{job}/award          accept one
GET  /v1/agent/balance             what this key may still spend
POST /v1/balance/topup             add funds
GET  /v1/rails                     which rails are on; with USDC on, an anonymous job reply carries pay_usdc: send exactly that amount to that address and the job lists after 12 confirmations
x402: with rails.x402.on, an anonymous POST /v1/tasks with "x402": true answers 402 with x402 payment requirements (scheme exact, USDC on Base); retry with a signed X-PAYMENT header and the job lists in the same round trip, transaction in X-PAYMENT-RESPONSE

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

## Anchored receipts
Every receipt's SHA-256 (the receipt minus its signature and anchor members,
compact, keys sorted) is batched hourly into a Merkle root that is committed
to Bitcoin through OpenTimestamps. This proves a receipt existed unchanged at
a time, without trusting Lamdis; it does not prove its contents are true.
Verify with the ots tool: ots verify -d <merkle_root> root.ots.

## Notes
Money is in integer minor units. Amounts are USD, distances are miles.
Verification tiers V0-V3 are defined at /v1/exchange.
`
