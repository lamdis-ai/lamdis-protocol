package api

// boardPageHTML is the operator's queue: work dispatched by agents that this
// person, fleet or business is eligible for.
//
// It reads as an instrument panel. The numbers that decide what to do next sit
// at the top, the radar shows where the open work is from where the operator
// stands, and the queue itself is a stack of glass cards whose stripe says the
// kind of work before a word is read. What somebody is already holding still
// comes first, because a person with a job out cannot take another and the
// first question they have is "where is the thing I already took".
// The coverage block rides on this page: its styles before the queue's, its
// markup at the top of the body, and its script beside the queue's own.
var boardPageHTML = boardTop + themeCSS + coverCSS + boardMid + boardBody +
	workerJS + boardJS + panelJS + coverJS + boardScript

const boardTop = `<!doctype html>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Queue — Lamdis</title>
<style>`

const boardMid = `
.bhead { display: flex; align-items: flex-end; justify-content: space-between; gap: 1rem;
  flex-wrap: wrap; margin: 0 0 1.2rem; }
.bhead h1 { font-size: 1.75rem; margin-bottom: .2rem; }
.bhead .lead { margin: 0; }
.terms { margin: 0 0 .9rem; font-size: .82rem; line-height: 1.5; color: var(--ink-3); }
.holding {
  border: 1px solid #1C4530; border-radius: 6px; margin-bottom: 1.4rem;
  background: linear-gradient(180deg, #0A1711, var(--bg));
  padding: 1.05rem 1.1rem;
}
.holding h3 { margin: .5rem 0 .25rem; font: 600 1.05rem/1.25 var(--sans); letter-spacing: -.02em; }
.holding .clock { margin: 0 0 .9rem; color: var(--ink-2); font: 400 .82rem/1.4 var(--mono); }
.holding .acts { display: flex; gap: .5rem; }
#holding .rows { border-radius: 6px; }

.bid { display: grid; gap: .5rem; margin-top: .6rem; }
.bid-row { display: flex; gap: .45rem; align-items: stretch; }
.bid-row .cur {
  display: grid; place-items: center; width: 2.1rem; flex: none;
  border: 1px solid var(--rule-2); border-radius: 3px;
  background: var(--panel); color: var(--ink-3); font: 500 .9rem var(--mono);
}
.bid-row input { flex: 1; min-width: 0; font-family: var(--mono); }
.hint { margin: 0; font-size: .78rem; color: var(--ink-3); }

/* The queue: one glass card per job. The stripe is the kind — blue for finding
   out, gold for making it true — and it goes grey when this account cannot
   carry the job yet. Money is big, right-aligned and tabular. */
.cards { display: grid; gap: .6rem; }
.cards .empty { border: 1px dashed var(--rule-2); border-radius: 6px; background: var(--glass); }
.qc { position: relative; display: grid; grid-template-columns: minmax(0, 1fr) auto;
  gap: .4rem 1.2rem; padding: .95rem 1.1rem .95rem 1.3rem; overflow: hidden;
  border: 1px solid var(--rule); border-radius: 6px; background: var(--glass);
  -webkit-backdrop-filter: blur(12px); backdrop-filter: blur(12px);
  box-shadow: inset 0 1px 0 rgba(255,255,255,.03); transition: border-color .15s, box-shadow .25s; }
.qc::before { content: ""; position: absolute; left: 0; top: 0; bottom: 0; width: 3px; background: var(--rule-2); }
.qc.obs::before { background: var(--blue); box-shadow: 0 0 14px rgba(95,176,255,.55); }
.qc.do::before { background: var(--gold); box-shadow: 0 0 14px rgba(255,182,39,.55); }
.qc.wait::before { background: var(--warn); box-shadow: none; }
.qc.shut { opacity: .55; }
.qc.shut::before { background: var(--rule-2); box-shadow: none; }
.qc:hover { border-color: var(--rule-2); }
.qc.lit { border-color: var(--gold); box-shadow: 0 0 0 1px var(--gold), 0 0 28px rgba(255,182,39,.25); }
.qc .t { font-size: .98rem; font-weight: 600; line-height: 1.35; }
.qc .t a.jl { color: inherit; text-decoration: none; }
.qc .t a.jl:hover { text-decoration: underline; }
.qc .t .pill { margin-left: .5rem; vertical-align: .15em; }
.qc .m { margin-top: .28rem; font: 400 .74rem/1.6 var(--mono); color: var(--ink-3); }
.qc .m b { color: var(--ink-2); font-weight: 500; }
.qc .m .chip { margin-right: .3rem; }
.qc .side { display: flex; flex-direction: column; align-items: flex-end; gap: .3rem;
  min-width: 7.5rem; text-align: right; }
.qc .pay { font: 700 1.3rem/1 var(--sans); letter-spacing: -.03em; color: var(--gold);
  font-variant-numeric: tabular-nums; }
.qc .pay.quiet { color: var(--ink-2); font-size: .95rem; font-weight: 600; }
.qc .net, .qc .when { font: 500 .62rem/1.4 var(--mono); color: var(--ink-3);
  letter-spacing: .1em; text-transform: uppercase; }
.qc .side .btn { margin-top: .35rem; }
.qc .err { grid-column: 1 / -1; margin: 0; min-height: 0; }
.qc .shots { margin-top: .6rem; }
@media (max-width: 40rem) {
  .qc { grid-template-columns: 1fr; }
  .qc .side { flex-direction: row; align-items: center; justify-content: space-between; text-align: left; }
}
/* The radar caption carries the freshness reading. */
.radar-wrap .cap .upd { color: var(--ink-3); letter-spacing: .08em; text-transform: none; }
</style>`

// boardBody is a var rather than a const because the masthead is a function.
var boardBody = shellTop("queue", "") + `
    <div class="bhead">
      <div>
        <p class="eyebrow">Operator queue</p>
        <h1>Queue</h1>
        <p class="lead">What is out there, what it pays, and what you have on.</p>
      </div>
    </div>
    <div class="strip" id="strip" hidden></div>
` + coverBlock + `

    <!-- Summary before detail: the five numbers that decide what to do next. -->
    <div class="hud" id="glance" style="--cols:5">
      <div class="t rv" style="--i:0" id="tile-open"><div class="k">Open work</div>
        <div class="v" id="hud-open">0</div><div class="s" id="hud-open-s">on the board now</div></div>
      <div class="t money rv" style="--i:1"><div class="k">On the board</div>
        <div class="v" id="hud-sum">$0.00</div><div class="s" id="hud-sum-s">fixed prices, before fees</div></div>
      <div class="t rv" style="--i:2" id="tile-room"><div class="k">Your room</div>
        <div class="v" id="hud-room">&ndash;</div><div class="s" id="hud-room-s">sign in to see your ceiling</div></div>
      <div class="t rv" style="--i:3" id="tile-clear"><div class="k">Clear to send</div>
        <div class="v" id="hud-clear">&ndash;</div><div class="s" id="hud-clear-s">sign in to see your earnings</div></div>
      <div class="t wait rv" style="--i:4"><div class="k">Reviews waiting</div>
        <div class="v" id="hud-rev">0</div><div class="s">assigned, never chosen</div></div>
    </div>

    <!-- Where the work is, from where you stand. -->
    <div class="radar-wrap rv" style="--i:5">
      <canvas class="radar" id="radar" aria-label="Where the open work is"></canvas>
      <div class="cap"><span class="beacon" id="radar-beacon"></span><span>Where the work is</span>
        <span class="upd" id="radar-upd"></span></div>
      <div class="legend"><span class="obs"><i></i>Check</span><span class="do"><i></i>Act</span>
        <span class="you"><i></i>You</span></div>
      <div class="hint" id="radar-hint" hidden></div>
    </div>

    <div id="terms-line"></div>

    <nav class="tabs" role="tablist" id="tabs">
      <button class="tab" role="tab" aria-selected="true" data-view="mine">
        My work<span class="tn" id="t-mine"></span></button>
      <button class="tab" role="tab" aria-selected="false" data-view="open">
        Open work<span class="tn" id="t-open"></span></button>
    </nav>

    <div id="holding"></div>
    <div class="rehearse" id="rehearse" hidden></div>
    <div class="cards" id="rows" hidden><div class="empty">Loading&hellip;</div></div>
    <div id="verify"></div>
    <div class="err" id="verify-err"></div>
` + shellBottom + `
<script>
"use strict";
`

const boardScript = `

var WORK = [], WAITING = 0, HOLDING = [], ME = null, CAP = null;
var PERSONAL = false, HIDDEN = 0;
// SIG is the last set of jobs drawn, so a poll that changes nothing does not
// redraw the queue; LAST_AT is when the board was last read.
var SIG = "", LAST_AT = 0;
var RADAR_STOP = null;

function renderHealth() {
  var el = document.getElementById("h-text");
  var b = document.getElementById("h-beacon");
  var a = document.getElementById("h-auth");
  if (signedIn()) {
    el.textContent = "Taking work";
    b.classList.remove("off");
    a.textContent = "Sign out";
    a.href = "#";
    a.onclick = function (e) {
      e.preventDefault(); clearSession(); renderHealth(); renderStrip(); load();
    };
  } else {
    el.textContent = "Not signed in";
    b.classList.add("off");
    a.textContent = "Sign in";
    a.href = "/signin?next=/board";
    a.onclick = null;
  }
}

// The strip only appears when there is something to say: signed out, you
// cannot take work. Signed in, the masthead already says so.
function renderStrip() {
  var el = document.getElementById("strip");
  if (!signedIn()) {
    el.hidden = false;
    el.className = "strip warn";
    el.innerHTML = '<span class="d"></span><span><b>You are not signed in.</b> ' +
      'Taking work and getting paid needs an account &mdash; one email and a code. ' +
      'Saying where you work does not. ' +
      '<a href="/signin?next=/board">Sign in</a></span>';
    return;
  }
  el.hidden = true;
  el.innerHTML = "";
}

function boardSum() {
  return WORK.reduce(function (a, w) { return a + (w.pay_minor || 0); }, 0);
}

// renderLive fills the masthead's reading: how much is open and what it adds
// up to. The same two numbers the HUD leads with, kept in the bar so they
// survive scrolling.
function renderLive() {
  var n = document.getElementById("live-n"), s = document.getElementById("live-sum");
  if (n) { n.textContent = String(WORK.length); }
  if (s) { s.textContent = money(boardSum(), "USD"); }
}

// renderGlance is the HUD: the five numbers somebody opens this page to see.
//
// Summary before detail. "Pending" used to be one figure covering three
// different situations — coming, waiting out a window, objected to — and which
// one you are in is the entire question. Room left is here because the other
// half of "what can I do next" is what you are still allowed to take on.
function renderGlance(flash) {
  var cur = (ME && ME.currency) || "USD";
  var asInt = function (v) { return String(Math.round(v)); };
  var asMoney = function (v) { return money(Math.round(v), cur); };

  var openEl = document.getElementById("hud-open");
  countUp(openEl, WORK.length, asInt);
  if (flash) {
    openEl.classList.remove("flash"); void openEl.offsetWidth; openEl.classList.add("flash");
  }
  document.getElementById("hud-open-s").textContent = HIDDEN
    ? HIDDEN + " more outside your range or skills"
    : (paidWork() ? (PERSONAL ? "within your range" : "on the board now")
                  : "practice only \u2014 none of it pays");

  countUp(document.getElementById("hud-sum"), boardSum(), asMoney);
  var bids = WORK.filter(function (w) { return w.pricing === "bids"; }).length;
  document.getElementById("hud-sum-s").textContent = !paidWork()
    ? "nothing here pays"
    : (bids ? "plus " + bids + " you price yourself" : "fixed prices, before fees");

  var room = document.getElementById("tile-room"), clear = document.getElementById("tile-clear");
  if (ME) {
    room.classList.add("money");
    countUp(document.getElementById("hud-room"), ME.room_minor || 0, asMoney);
    document.getElementById("hud-room-s").textContent =
      "of " + money(ME.ceiling_minor || 0, cur) + " ceiling, " +
      money(ME.exposure_minor || 0, cur) + " in flight";
    clear.classList.add("ok");
    countUp(document.getElementById("hud-clear"), ME.clear_minor || 0, asMoney);
    var held = ME.held_minor || 0;
    clear.classList.toggle("wait", held > 0);
    document.getElementById("hud-clear-s").textContent = held > 0
      ? money(held, cur) + " held while a buyer's objection is decided"
      : (HOLDING.length ? HOLDING.length + " in flight" : "nothing held");
  } else {
    room.classList.remove("money");
    document.getElementById("hud-room").textContent = "Sign in";
    document.getElementById("hud-room-s").textContent = "to see your ceiling";
    clear.classList.remove("ok", "wait");
    document.getElementById("hud-clear").textContent = "Sign in";
    document.getElementById("hud-clear-s").textContent = "to see your earnings";
  }

  countUp(document.getElementById("hud-rev"), WAITING, asInt);
}

// renderRadar draws the open work around the operator. Coordinates on the
// board are coarse — the server rounds them to about a kilometre — which is
// exactly the grain a map of "where is the work" needs and no finer.
function renderRadar() {
  var canvas = document.getElementById("radar");
  var hint = document.getElementById("radar-hint");
  if (RADAR_STOP) { RADAR_STOP(); RADAR_STOP = null; }
  // Coarse points coincide — five jobs in one part of town land on one dot —
  // so jobs at the same point are drawn once and labelled with the count.
  var at = {}, jobs = [];
  WORK.filter(function (w) { return w.area_lat || w.area_lon; }).forEach(function (w) {
    var k = w.area_lat + "," + w.area_lon;
    if (!at[k]) {
      at[k] = {lat: w.area_lat, lon: w.area_lon, kind: w.kind, title: w.title, job: w.job,
               pay: w.pay_minor ? money(w.pay_minor, w.currency) : "", n: 1, sum: w.pay_minor || 0};
      jobs.push(at[k]);
    } else {
      var g = at[k];
      g.n++; g.sum += (w.pay_minor || 0);
      if (w.kind === "do") { g.kind = "do"; }
      g.title = g.n + " jobs here" + (w.area ? " \u00b7 " + w.area : "");
      g.pay = g.sum ? money(g.sum, w.currency) : "";
    }
  });
  var you = (CAP && (CAP.lat_e7 || CAP.lon_e7)) ? {lat: CAP.lat_e7 / 1e7, lon: CAP.lon_e7 / 1e7} : null;
  var range = (CAP && CAP.range_miles) || 0;
  if (!range) {
    // No range set: fit the ring to the work, centred on the operator when
    // there is one and on the jobs otherwise.
    var cx = you ? you.lat : (jobs.reduce(function (a, j) { return a + j.lat; }, 0) / (jobs.length || 1));
    var cz = you ? you.lon : (jobs.reduce(function (a, j) { return a + j.lon; }, 0) / (jobs.length || 1));
    var far = jobs.reduce(function (m, j) {
      var dx = (j.lon - cz) * 69.17 * Math.cos(cx * Math.PI / 180), dy = (j.lat - cx) * 69.17;
      return Math.max(m, Math.sqrt(dx * dx + dy * dy));
    }, 0);
    range = Math.max(5, Math.ceil(far * 1.3));
  }
  RADAR_STOP = drawRadar(canvas, {
    jobs: jobs, you: you, rangeMiles: range,
    empty: WORK.length ? "the open work has no location yet" : "nothing is open",
    onPick: function (j) { openRow(j.job); }
  });
  if (!jobs.length) {
    hint.hidden = false;
    hint.textContent = WORK.length
      ? "Nothing open carries a location yet. The buyer described these jobs in words; see the queue below."
      : "Nothing is open right now. Jobs appear here as agents dispatch them.";
  } else {
    hint.hidden = !(!you && signedIn());
    hint.textContent = "Centred on the work. Set your location in Capacity to see it from where you are.";
  }
}

// openRow answers a click on a radar dot: the queue tab, the card lit, and
// the bid box open if the job takes bids.
function openRow(job) {
  var openTab = document.querySelector('#tabs .tab[data-view="open"]');
  if (openTab && openTab.getAttribute("aria-selected") !== "true") { openTab.click(); }
  var row = document.getElementById("row-" + job);
  if (!row) { return; }
  row.scrollIntoView({behavior: "smooth", block: "center"});
  row.classList.add("lit");
  setTimeout(function () { row.classList.remove("lit"); }, 1800);
  var box = document.getElementById("bid-" + job);
  if (box && signedIn()) { box.hidden = false; }
}

function tickUpdated() {
  var el = document.getElementById("radar-upd");
  if (!el) { return; }
  if (!LAST_AT) { el.textContent = ""; return; }
  var s = Math.max(0, Math.round((Date.now() - LAST_AT) / 1000));
  el.textContent = "· updated " + s + "s ago";
}

// stageRail draws the plan as segments weighted by what each pays.
//
// Progress, not a schedule. There are no dates on it and there is not going to
// be: what somebody needs from this is which piece is in front of them and
// what it is worth, and a timeline would be answering a question nobody asked.
function stageRail(h) {
  if (!h.stages || !h.stages.length) { return ""; }
  var done = h.stage_done || [];
  var segs = h.stages.map(function (st, i) {
    var cls = done[i] ? "paid" : (i === h.next_stage ? "now" : "");
    return '<div class="seg ' + cls + '" style="flex:' +
      Math.max(1, st.pay_minor || 1) + '"></div>';
  }).join("");
  var keys = h.stages.map(function (st, i) {
    var state = done[i] ? "paid" : (i === h.next_stage ? "now" : "");
    return '<span class="' + state + '"><b>' + esc(st.name) + '</b> ' +
      money(st.pay_minor || 0, h.currency || "USD") +
      (state ? " " + state : "") + '</span>';
  }).join("");
  return '<div class="stagebar">' + segs + '</div>' +
    '<div class="stagekey">' + keys + '</div>';
}

// renderHolding is "My work": what you are on, what stage, what is owed.
function renderHolding() {
  var host = document.getElementById("holding");
  document.getElementById("n-flight").textContent = HOLDING.length || "";
  document.getElementById("t-mine").textContent = HOLDING.length || "";
  if (!HOLDING.length) {
    host.innerHTML = '<div class="cards"><div class="empty">Nothing on at the moment. ' +
      'Open work is on the other tab.</div></div>';
    return;
  }
  host.innerHTML = '<div class="rows glass" style="padding:0">' + HOLDING.map(function (h, i) {
    var blocked = (h.blocked_by || []).length > 0;
    var mins = Math.max(0, Math.round((new Date(h.expires) - new Date()) / 60000));
    var staged = h.stages && h.stages.length;
    var next = staged && h.next_stage >= 0 ? h.stages[h.next_stage] : null;

    var facts = [];
    if (blocked) { facts.push('<span class="chip warn">Waiting on other work</span>'); }
    else if (staged) {
      facts.push('<span class="chip ok">Stage ' + (h.next_stage + 1) +
        ' of ' + h.stages.length + '</span>');
    } else {
      facts.push('<span class="chip ok">Yours for ' + mins + ' min</span>');
    }
    if (h.where) { facts.push(esc(h.where)); }
    if (h.project) {
      facts.push(esc(h.project.position) + ' of ' + esc(h.project.jobs) +
        (h.project.one_visit ? " &middot; one address" : ""));
    }

    return '<div class="job rv ' + (blocked ? "wait" : "go") + '" style="--i:' + i + '" data-job="' + esc(h.job) + '">' +
      '<button class="jrow" data-expand="' + esc(h.job) + '">' +
        '<div class="jbody">' +
          '<p class="jt">' + esc(h.title) + '</p>' +
          '<div class="m">' + facts.join(' <span class="dot">&middot;</span> ') + '</div>' +
          stageRail(h) +
        '</div>' +
        '<div class="amt">' +
          '<div class="n' + (next ? "" : " quiet") + '">' +
            money(next ? next.pay_minor : h.pay_minor, h.currency || "USD") + '</div>' +
          '<div class="s">' + (next ? "next stage" : "on completion") + '</div>' +
        '</div>' +
      '</button>' +
      '<div class="jopen" id="o-' + esc(h.job) + '">' +
        '<div class="jgrid">' +
          (next
            ? '<div><h4>What proves this stage</h4><p class="fx">' +
                esc(next.deliverable) + '</p></div>'
            : '') +
          (blocked
            ? '<div><h4>Why it cannot start</h4><p class="fx">' +
                esc(h.blocked_by.join(", ")) + ' has to be finished and accepted first. ' +
                'Nobody else can take this in the meantime &mdash; it is yours.</p></div>'
            : '') +
          agreedBlock(h) +
        '</div>' +
        '<div class="acts">' +
          (blocked ? '' : '<a class="btn go" href="' + esc(h.resume) + '">Carry on</a>') +
          '<button class="btn" data-give="' + esc(h.job) + '">Give it back</button>' +
        '</div>' +
        '<div class="err" id="g-' + esc(h.job) + '"></div>' +
      '</div>' +
    '</div>';
  }).join("") + '</div>';

  host.querySelectorAll("button[data-give]").forEach(function (b) {
    b.addEventListener("click", function (e) {
      e.stopPropagation();
      giveBack(b, b.getAttribute("data-give"));
    });
  });
  wireRows(host);
  // The panel was just rebuilt; keep whichever tab is selected showing.
  var sel = document.querySelector('#tabs .tab[aria-selected="true"]');
  if (sel) {
    host.hidden = sel.dataset.view !== "mine";
    document.getElementById("rows").hidden = sel.dataset.view === "mine";
  }
}

// agreedBlock shows the figures the work is judged against.
function agreedBlock(h) {
  if (!h.agreed || !h.agreed.length) { return ""; }
  return '<div><h4>Agreed when you bid</h4>' + h.agreed.map(function (a) {
    return '<p class="fx"><b>' + esc(a.name) + '</b> ' + esc(a.value) + ' ' +
      '<span class="chip ' + (a.firm ? "ok" : "warn") + '">' +
      (a.firm ? "firm" : "provisional") + '</span></p>';
  }).join("") +
  '<p class="fn">Provisional means you said you would measure and requote. ' +
    'Do that before the stage, not after.</p></div>';
}

// wireRows makes a row open in place. One at a time, so the page stays short
// enough to scan — which is the complaint this whole view answers.
function wireRows(host) {
  host.querySelectorAll("button[data-expand]").forEach(function (b) {
    b.addEventListener("click", function () {
      var job = b.parentElement, list = job.parentElement;
      var was = job.classList.contains("on");
      list.querySelectorAll(".job.on").forEach(function (o) { o.classList.remove("on"); });
      if (!was) { job.classList.add("on"); }
    });
  });
}

function termsLine() {
  if (!TERMS) { return ""; }
  var pct = (TERMS.fee_bp / 100);
  // Zero is worth saying properly, with the reason and the fact that it is
  // temporary. "The exchange keeps 0%" reads like a rounding error.
  if (!TERMS.fee_bp) {
    return '<p class="terms"><b>You keep everything you earn.</b> ' +
      'No fee while we are getting this off the ground &mdash; there is no ' +
      'supply here to take a cut from until somebody builds it. Earnings are ' +
      'paid out once they reach ' +
      money(TERMS.payout_threshold_minor, "usd") + ', because a transfer costs ' +
      'a flat fee either way.</p>';
  }
  return '<p class="terms">The exchange keeps ' + pct + '% of what you earn. ' +
    'Earnings are paid out once they reach ' +
    money(TERMS.payout_threshold_minor, "usd") + ' — below that they stay ' +
    'in your account, because a transfer costs a flat fee either way.</p>';
}

// paidWork reports whether anything on the board is real. A practice job is a
// rehearsal of the flow and pays nothing, so a queue of nothing but practice
// is an empty market and the page has to say so.
function paidWork() {
  return WORK.some(function (w) { return !w.practice; });
}

// renderRehearse frames the practice runs, when they are all there is.
function renderRehearse() {
  var el = document.getElementById("rehearse");
  var practice = WORK.filter(function (w) { return w.practice; }).length;
  if (!practice || paidWork()) { el.hidden = true; el.innerHTML = ""; return; }
  el.hidden = false;
  el.innerHTML = "<b>Below is a rehearsal, not work.</b> " +
    (practice === 1 ? "This job pays" : "These " + practice + " jobs pay") +
    " nothing and no money is escrowed against " + (practice === 1 ? "it" : "them") +
    ". They exist so you can walk the flow once \u2014 take one, photograph the code, " +
    "submit it \u2014 before there is anything paid to take.";
}

// renderQueue draws the open work. Cards rise in only when the set of jobs
// changed; a redraw for a new room figure keeps them where they are.
function renderQueue(animate) {
  var host = document.getElementById("rows");
  document.getElementById("n-queue").textContent = WORK.length || "";
  document.getElementById("t-open").textContent = WORK.length || "";
  var ready = signedIn();

  document.getElementById("terms-line").innerHTML = termsLine();
  renderRehearse();
  renderCover(paidWork());

  if (!WORK.length) {
    host.innerHTML = '<div class="empty">' + (HIDDEN
      ? HIDDEN + ' job' + (HIDDEN === 1 ? ' is' : 's are') + ' open, but ' +
        (HIDDEN === 1 ? 'it is' : 'none are') + ' within your range or ' +
        'qualifications. <a href="/console">Widen them</a>.'
      : 'No work in range right now. Jobs appear here as agents dispatch them.' +
        (signedIn()
          ? '<div style="margin-top:.8rem"><button class="btn go" id="want-alerts">' +
            'Email me when work appears</button>' +
            '<div class="err" id="alert-err"></div></div>'
          : "")) +
      '</div>';
    var wa = document.getElementById("want-alerts");
    if (wa) {
      wa.addEventListener("click", function () {
        var btn = this, err = document.getElementById("alert-err");
        btn.disabled = true;
        workerHeaders("PUT", "/v1/alerts").then(function (h) {
          return fetch("/v1/alerts?on=true", {method: "PUT", headers: h});
        }).then(function (r) { return r.json(); }).then(function (j) {
          err.className = "err ok";
          err.textContent = j.available
            ? "We will email you when work appears that you could take."
            : "Noted — email is not switched on yet, so this is recorded " +
              "rather than working.";
        }).catch(function () {
          btn.disabled = false;
          err.textContent = "Could not save that.";
        });
      });
    }
    return;
  }
  host.innerHTML = WORK.map(function (w, i) {
    var bidding = w.pricing === "bids";
    var blocked = (w.blocked_by || []).length > 0;
    var facts = [];
    if (w.distance_miles) { facts.push('<b>' + w.distance_miles + ' mi</b>'); }
    // The exact address is released only once the job is taken.
    if (w.area) { facts.push(esc(w.area)); }
    if (w.skills && w.skills.length) { facts.push("needs " + w.skills.map(esc).join(", ")); }
    // Say who is asking. A worker on the first marketplace of this shape had
    // no way to know whether an employer was a person or a pipeline.
    if (w.posted_by_agent) { facts.push("posted by an agent"); }
    // A multi-day job has to look like one on the board, or somebody takes it
    // expecting an errand.
    if (w.stages && w.stages.length) {
      facts.push("<b>" + w.stages.length + " stages</b>, paid as you go");
    }
    if (w.work_hours >= 24) {
      facts.push(Math.round(w.work_hours / 24) + " day job");
    } else if (w.work_hours) {
      facts.push(w.work_hours + "h job");
    }
    if (bidding) { facts.push("you name the price"); }
    if (w.bonus_minor) { facts.push("+" + money(w.bonus_minor, w.currency) + " if the answer is yes"); }
    if (w.attempt_minor) { facts.push(money(w.attempt_minor, w.currency) + " if it is impossible"); }
    if (w.expense_cap_minor) { facts.push("expenses to " + money(w.expense_cap_minor, w.currency)); }
    if (blocked) { facts.unshift('<span class="chip warn">Waits on ' + esc(w.blocked_by.join(", ")) + '</span>'); }
    if (w.practice) { facts.unshift("not real work, pays nothing"); }

    var net = takeHome(w.pay_minor);
    // Nothing is not money, so it is not gold: a practice job's figure reads
    // quiet rather than dressed as a payment.
    var pay = bidding
      ? '<div class="pay quiet">Your bid</div>'
      : '<div class="pay' + (w.pay_minor ? "" : " quiet") + '">' + money(w.pay_minor, w.currency) + '</div>' +
        (net !== null && net !== w.pay_minor
          ? '<div class="net">' + money(net, w.currency) + ' to you</div>'
          : "");

    // Work this account cannot carry yet, said with the number rather than
    // hidden. Somebody looking at a job they cannot take is owed the reason
    // and the figure, not a missing row.
    var atRisk = (w.stages && w.stages.length)
      ? w.stages.reduce(function (m, st) { return Math.max(m, st.pay_minor || 0); }, 0)
      : (w.pay_minor || w.max_bid_minor || 0);
    var overRoom = ME && ME.room_minor !== undefined && !w.practice &&
      atRisk > ME.room_minor;
    if (overRoom) {
      facts.unshift('<span class="chip">Above your room</span>');
    }

    var action = bidding
      ? '<button class="btn sm" data-open="' + esc(w.job) + '"' +
        (overRoom ? " disabled" : "") + '>Bid</button>'
      : '<button class="btn sm' + (ready && !overRoom ? " go" : "") + '" data-job="' + esc(w.job) + '"' +
        (ready && !overRoom ? "" : " disabled") + '>Take</button>';

    var cls = "qc " + (w.kind === "do" ? "do" : "obs") +
      (overRoom ? " shut" : "") + (blocked ? " wait" : "") +
      (animate ? " rv" : "");
    return '<div class="' + cls + '" id="row-' + esc(w.job) + '" style="--i:' + i + '">' +
      '<div class="grow">' +
        '<div class="t"><span class="chip ' + (w.kind === "do" ? "do" : "obs") + '">' + kindLabel(w.kind) + '</span>' +
          // The title is a link to the job's own page, which is what a
          // dispatch email or a shared link lands on.
          '<a class="jl" href="/j/' + encodeURIComponent(w.job) + '">' +
          esc(w.kind === "do" && w.instructions ? w.instructions : w.title) + '</a>' +
          (w.practice ? '<span class="pill quiet">Practice</span>' : "") + '</div>' +
        '<div class="m">' + facts.join(" &middot; ") + '</div>' +
        // What the work is and what would prove it. Both used to be withheld
        // from the board, which meant nobody could price the job they were
        // being asked to bid on.
        (w.deliverable ? '<div class="dv">Proof: ' + esc(w.deliverable) + '</div>' : "") +
        (w.brief ? '<div class="bf">' + esc(w.brief) + '</div>' : "") +
        siteShots(w) +
        (w.withheld ? '<div class="wh">' + esc(w.withheld) + '</div>' : "") +
        (overRoom
          ? '<div class="wh">This would put ' + money(atRisk, w.currency) +
            ' on unfinished work and you have ' + money(ME.room_minor, ME.currency || "USD") +
            ' of room. Finish something in flight, or take something smaller.</div>'
          : "") +
        '<div class="bid" id="bid-' + esc(w.job) + '" hidden>' +
          '<div class="bid-row">' +
            '<span class="cur">$</span>' +
            '<input type="text" inputmode="decimal" placeholder="45.00" data-bid="' + esc(w.job) + '">' +
            '<button class="btn go sm" data-place="' + esc(w.job) + '">Place bid</button>' +
          '</div>' +
          '<input type="text" maxlength="140" placeholder="How you would do it (optional)" data-note="' + esc(w.job) + '">' +
          askUnknowns(w) +
          '<p class="hint">You are naming your own price. Nobody has told you a budget, ' +
            'and other bids are not shown.</p>' +
        '</div>' +
      '</div>' +
      '<div class="side">' + pay +
        '<div class="when">' + left(w.expires) + ' left</div>' +
        action +
      '</div>' +
      '<div class="err" id="e-' + esc(w.job) + '"></div>' +
    '</div>';
  }).join("");

  host.querySelectorAll("button[data-job]").forEach(function (b) {
    b.addEventListener("click", function () { takeJob(b, b.getAttribute("data-job")); });
  });
  host.querySelectorAll("button[data-open]").forEach(function (b) {
    b.addEventListener("click", function () {
      if (!signedIn()) { goSignIn(); return; }
      var box = document.getElementById("bid-" + b.getAttribute("data-open"));
      box.hidden = !box.hidden;
      if (!box.hidden) { box.querySelector("input").focus(); }
    });
  });
  host.querySelectorAll("button[data-place]").forEach(function (b) {
    b.addEventListener("click", function () { placeBid(b, b.getAttribute("data-place")); });
  });
}

function renderVerify() {
  var host = document.getElementById("verify");
  if (!WAITING) { host.innerHTML = ""; return; }
  host.innerHTML = '<h2>Verification</h2><div class="cards"><div class="qc obs">' +
    '<div class="grow"><div class="t"><span class="chip obs">' + WAITING + ' waiting</span>' +
      "Check another operator&rsquo;s evidence</div>" +
    '<div class="m">about a minute &middot; assigned, never chosen</div></div>' +
    '<div class="side"><button class="btn" id="verify-next">Verify next</button></div></div></div>';
  document.getElementById("verify-next").addEventListener("click", function () {
    post(this, document.getElementById("verify-err"), "/v1/workers/assign");
  });
}

function giveBack(button, job) {
  post(button, document.getElementById("g-" + job),
       "/v1/workers/giveback/" + encodeURIComponent(job));
}

// The two views, switched without a round trip. Somebody checking whether
// they have been paid should not have to reload the board to see their work.
function wireTabs() {
  var mine = document.getElementById("holding");
  var open = document.getElementById("rows");
  document.querySelectorAll("#tabs .tab").forEach(function (tab) {
    tab.addEventListener("click", function () {
      document.querySelectorAll("#tabs .tab").forEach(function (t) {
        t.setAttribute("aria-selected", String(t === tab));
      });
      var wantMine = tab.dataset.view === "mine";
      mine.hidden = !wantMine;
      open.hidden = wantMine;
      document.getElementById("terms-line").hidden = wantMine;
    });
  });
  // Somebody arriving with nothing on wants the open board, not an empty
  // panel telling them so.
  if (!HOLDING.length) {
    var openTab = document.querySelector('#tabs .tab[data-view="open"]');
    if (openTab) { openTab.click(); }
  }
}

function load() {
  if (signedIn()) {
    // What this account is owed and what it may still take on. Both halves of
    // "what do I do next", and neither was on this page before.
    workerHeaders("GET", "/v1/me")
      .then(function (h) { return fetch("/v1/me", {headers: h}); })
      .then(function (r) { return r.ok ? r.json() : null; })
      .then(function (m) { ME = m; renderGlance(false); renderQueue(false); })
      .catch(function () { ME = null; renderGlance(false); });
    workerHeaders("GET", "/v1/workers/holdings")
      .then(function (h) { return fetch("/v1/workers/holdings", {headers: h}); })
      .then(function (r) { return r.ok ? r.json() : null; })
      .then(function (b) {
        HOLDING = (b && b.holding) || [];
        renderHolding(); renderGlance(false);
      })
      .catch(function () { HOLDING = []; renderHolding(); });
    // Where the operator stands, so the radar is drawn from there.
    workerHeaders("GET", "/v1/capacity")
      .then(function (h) { return fetch("/v1/capacity", {headers: h}); })
      .then(function (r) { return r.ok ? r.json() : null; })
      .then(function (c) {
        var was = JSON.stringify(CAP);
        CAP = (c && c.capacity) || null;
        if (JSON.stringify(CAP) !== was) { renderRadar(); }
      })
      .catch(function () {});
  } else {
    HOLDING = []; CAP = null; renderHolding(); renderGlance(false);
  }

  (signedIn()
    ? workerHeaders("GET", "/v1/board").then(function (h) { return fetch("/v1/board", {headers: h}); })
    : fetch("/v1/board"))
    .then(function (r) { return r.json(); })
    .then(function (b) {
      WORK = (b && b.work) || [];
      WAITING = (b && b.reviews_waiting) || 0;
      PERSONAL = !!(b && b.personalized);
      TERMS = (b && b.terms) || null;
      HIDDEN = (b && b.filtered_out) || 0;
      var sig = WORK.map(function (w) { return w.job + "/" + w.taken; }).join(",");
      var changed = sig !== SIG, first = !LAST_AT;
      SIG = sig; LAST_AT = Date.now(); tickUpdated();
      // The block and the rehearsal note do not wait on the queue redrawing:
      // an empty board never changes signature, and the whole point of this
      // page is that it says something useful when there is nothing on it.
      renderRehearse();
      renderCover(paidWork());
      // First pass draws even when nothing changed, or a board that is empty
      // from the start sits on "Loading" forever.
      if (changed || first) { renderQueue(true); renderRadar(); }
      renderLive();
      renderGlance(changed && !first);
      renderVerify();
    })
    .catch(function () {
      document.getElementById("rows").innerHTML =
        '<div class="empty">Could not reach the exchange.</div>';
      document.getElementById("live-beacon").classList.add("off");
    });
}

session().then(function () {
  renderHealth();
  renderStrip();
  wireTabs();
  wireCover();
  coverTally();
  load();
});
// Live: re-read the board every twenty seconds while the tab is showing, and
// the moment it is shown again. A hidden tab polling is a battery, not a
// dashboard.
setInterval(function () {
  if (document.visibilityState === "visible") { load(); }
}, 20000);
document.addEventListener("visibilitychange", function () {
  if (document.visibilityState === "visible") { load(); }
});
setInterval(tickUpdated, 1000);
</script>
`
