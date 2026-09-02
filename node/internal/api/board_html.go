package api

// boardPageHTML is the operator's queue: work dispatched by agents that this
// person, fleet or business is eligible for.
//
// It opens with what they are already holding, because somebody with a job out
// cannot take another and the first question they have is "where is the thing
// I already took". Everything else is secondary to that.
var boardPageHTML = boardTop + themeCSS + boardMid + boardBody + workerJS + boardJS + boardScript

const boardTop = `<!doctype html>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Queue — Lamdis</title>
<style>`

const boardMid = `
.terms { margin: 0 0 .9rem; font-size: .82rem; line-height: 1.5; color: var(--ink-3); }
.amt .net { display: block; margin-top: .1rem; font: 500 .74rem/1 var(--mono);
  color: var(--ink-3); font-variant-numeric: tabular-nums; }
.holding {
  border: 1px solid #1C4530; border-radius: 3px; margin-bottom: 1.4rem;
  background: linear-gradient(180deg, #0A1711, var(--bg));
  padding: 1.05rem 1.1rem;
}
.holding h3 { margin: .5rem 0 .25rem; font: 600 1.05rem/1.25 var(--sans); letter-spacing: -.02em; }
.holding .clock { margin: 0 0 .9rem; color: var(--ink-2); font: 400 .82rem/1.4 var(--mono); }
.holding .acts { display: flex; gap: .5rem; }

.bid { display: grid; gap: .5rem; margin-top: .2rem; }
.bid-row { display: flex; gap: .45rem; align-items: stretch; }
.bid-row .cur {
  display: grid; place-items: center; width: 2.1rem; flex: none;
  border: 1px solid var(--rule-2); border-radius: 3px;
  background: var(--panel); color: var(--ink-3); font: 500 .9rem var(--mono);
}
.bid-row input { flex: 1; min-width: 0; font-family: var(--mono); }
.hint { margin: 0; font-size: .78rem; color: var(--ink-3); }
.t a.jl { color: inherit; text-decoration: none; }
.t a.jl:hover { text-decoration: underline; }
</style>`

const boardBody = `
<header class="top">
  <a class="mark" href="/board">lamdis<b>.</b></a>
  <div class="right">
    <span class="health"><span class="beacon off"></span><span id="h-text">&hellip;</span></span>
  </div>
</header>
<div class="shell">
  <nav class="rail">
    <span class="label grp">Work</span>
    <a href="/board" aria-current="page">Queue <span class="n hot" id="n-queue"></span></a>
    <a href="#holding">In flight <span class="n" id="n-flight"></span></a>
    <span class="label grp">Operation</span>
    <a href="/console">Earnings</a>
    <a href="/console#capacity">Capacity</a>
    <a href="/console#larger">Larger jobs</a>
    <a href="/console#integration">Integration</a>
  </nav>
  <main class="main">
    <h1>Queue</h1>
    <p class="lead">What is out there, what it pays, and what you have on.</p>
    <div class="strip" id="strip"></div>

    <!-- Summary before detail: the four numbers that decide what to do next. -->
    <dl class="glance" id="glance"></dl>

    <div id="terms-line"></div>

    <nav class="tabs" role="tablist" id="tabs">
      <button class="tab" role="tab" aria-selected="true" data-view="mine">
        My work<span class="tn" id="t-mine"></span></button>
      <button class="tab" role="tab" aria-selected="false" data-view="open">
        Open work<span class="tn" id="t-open"></span></button>
    </nav>

    <div id="holding"></div>
    <div class="rows" id="rows" hidden><div class="empty">Loading&hellip;</div></div>
    <div id="verify"></div>
    <div class="err" id="verify-err"></div>
  </main>
</div>
<script>
"use strict";
`

const boardScript = `

var WORK = [], WAITING = 0, HOLDING = [], ME = null;

function renderHealth() {
  var el = document.getElementById("h-text");
  var b = document.querySelector(".beacon");
  if (signedIn()) {
    el.textContent = "Taking work";
    b.classList.remove("off");
  } else {
    el.textContent = "Not signed in";
    b.classList.add("off");
  }
}

function renderStrip() {
  var el = document.getElementById("strip");
  if (!signedIn()) {
    el.className = "strip warn";
    el.innerHTML = '<span class="d"></span><span><b>You are not signed in.</b> ' +
      'Sign in to take work and get paid &mdash; one email and a code. ' +
      '<a href="/signin?next=/board">Sign in</a></span>';
    return;
  }
  el.className = "strip";
  el.innerHTML = '<span class="d"></span><span>Taking work. ' +
    '<a href="/console">Your earnings</a> &middot; ' +
    '<a href="#" id="signout">Sign out</a></span>';
  var so = document.getElementById("signout");
  if (so) {
    so.addEventListener("click", function (e) {
      e.preventDefault(); clearSession(); renderHealth(); renderStrip(); load();
    });
  }
}

// renderGlance is the four numbers somebody opens this page to see.
//
// Summary before detail. "Pending" used to be one figure covering three
// different situations — coming, waiting out a window, objected to — and which
// one you are in is the entire question. Room left is here because the other
// half of "what can I do next" is what you are still allowed to take on.
function renderGlance() {
  var host = document.getElementById("glance");
  if (!ME) { host.innerHTML = ""; return; }
  var cur = ME.currency || "USD";
  var cells = [
    {k: "Clear to send", v: money(ME.clear_minor || 0, cur), cls: "money"},
    {k: "Held", v: money(ME.held_minor || 0, cur), cls: (ME.held_minor > 0 ? "wait" : "")},
    {k: "In flight", v: String(HOLDING.length), sub: HOLDING.length === 1 ? "job" : "jobs"},
    {k: "Room left", v: money(ME.room_minor || 0, cur),
     sub: "of " + money(ME.ceiling_minor || 0, cur)}
  ];
  host.innerHTML = cells.map(function (c) {
    return '<div class="g ' + (c.cls || "") + '">' +
      '<dt>' + esc(c.k) + '</dt>' +
      '<dd>' + esc(c.v) + (c.sub ? '<small>' + esc(c.sub) + '</small>' : "") + '</dd>' +
    '</div>';
  }).join("");
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
    host.innerHTML = '<div class="empty">Nothing on at the moment. ' +
      'Open work is on the other tab.</div>';
    return;
  }
  host.innerHTML = '<div class="rows">' + HOLDING.map(function (h) {
    var blocked = (h.blocked_by || []).length > 0;
    var mins = Math.max(0, Math.round((new Date(h.expires) - new Date()) / 60000));
    var staged = h.stages && h.stages.length;
    var next = staged && h.next_stage >= 0 ? h.stages[h.next_stage] : null;

    var facts = [];
    if (blocked) { facts.push('<span class="chip wait">Waiting on other work</span>'); }
    else if (staged) {
      facts.push('<span class="chip go">Stage ' + (h.next_stage + 1) +
        ' of ' + h.stages.length + '</span>');
    } else {
      facts.push('<span class="chip go">Yours for ' + mins + ' min</span>');
    }
    if (h.where) { facts.push(esc(h.where)); }
    if (h.project) {
      facts.push(esc(h.project.position) + ' of ' + esc(h.project.jobs) +
        (h.project.one_visit ? " &middot; one address" : ""));
    }

    return '<div class="job ' + (blocked ? "wait" : "go") + '" data-job="' + esc(h.job) + '">' +
      '<button class="jrow" data-open="' + esc(h.job) + '">' +
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
      '<span class="chip ' + (a.firm ? "go" : "wait") + '">' +
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

var PERSONAL = false, HIDDEN = 0;

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
    money(TERMS.payout_threshold_minor, "usd") + ' \u2014 below that they stay ' +
    'in your account, because a transfer costs a flat fee either way.</p>';
}

function renderQueue() {
  var host = document.getElementById("rows");
  document.getElementById("n-queue").textContent = WORK.length || "";
  document.getElementById("t-open").textContent = WORK.length || "";
  var ready = signedIn();

  document.getElementById("terms-line").innerHTML = termsLine();

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
          : "Noted \u2014 email is not switched on yet, so this is recorded " +
            "rather than working.";
      }).catch(function () {
        btn.disabled = false;
        err.textContent = "Could not save that.";
      });
    });
  }
    return;
  }
  host.innerHTML = WORK.map(function (w) {
    var bidding = w.pricing === "bids";
    var facts = [];
    if (w.distance_miles) { facts.push('<b>' + w.distance_miles + ' mi</b>'); }
    // The exact address is released only once the job is taken.
    if (w.area) { facts.push(esc(w.area)); }
    if (w.skills && w.skills.length) { facts.push("needs " + w.skills.map(esc).join(", ")); }
    // A multi-day job has to look like one on the board, or somebody takes it
    // expecting an errand.
    // Say who is asking. A worker on the first marketplace of this shape had
    // no way to know whether an employer was a person or a pipeline.
    if (w.posted_by_agent) { facts.push("posted by an agent"); }
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

    if (w.practice) {
      facts.unshift('<b>practice run \u2014 not real work, pays nothing</b>');
    }
    var net = takeHome(w.pay_minor);
    var right = bidding
      ? '<span class="amt none">Your bid</span>'
      : '<span class="amt">' + money(w.pay_minor, w.currency) +
        (net !== null && net !== w.pay_minor
          ? '<span class="net">' + money(net, w.currency) + ' to you</span>'
          : "") + '</span>';

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
      : '<button class="btn sm" data-job="' + esc(w.job) + '"' +
        (ready && !overRoom ? "" : " disabled") + '>Take</button>';

    return '<div class="r' + (overRoom ? " shut" : "") + '">' +
      '<div class="grow">' +
        '<div class="t"><span class="chip hot">' + kindLabel(w.kind) + '</span>' +
          // The title is a link to the job's own page, which is what a
          // dispatch email or a shared link lands on.
          '<a class="jl" href="/j/' + encodeURIComponent(w.job) + '">' +
          esc(w.kind === "do" && w.instructions ? w.instructions : w.title) + '</a></div>' +
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
      right +
      '<span class="when">' + left(w.expires) + '</span>' +
      action +
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
  host.innerHTML = '<h2>Verification</h2><div class="rows"><div class="r">' +
    '<div class="grow"><div class="t"><span class="chip hot">' + WAITING + ' waiting</span>' +
      "Check another operator&rsquo;s evidence</div>" +
    '<div class="m">about a minute &middot; assigned, never chosen</div></div>' +
    '<button class="btn" id="verify-next">Verify next</button></div></div>';
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
      .then(function (m) { ME = m; renderGlance(); renderQueue(); })
      .catch(function () { ME = null; renderGlance(); });
    workerHeaders("GET", "/v1/workers/holdings")
      .then(function (h) { return fetch("/v1/workers/holdings", {headers: h}); })
      .then(function (r) { return r.ok ? r.json() : null; })
      .then(function (b) {
        HOLDING = (b && b.holding) || [];
        renderHolding(); renderGlance();
      })
      .catch(function () { HOLDING = []; renderHolding(); });
  } else {
    HOLDING = []; renderHolding(); renderGlance();
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
      renderQueue();
      renderVerify();
    })
    .catch(function () {
      document.getElementById("rows").innerHTML =
        '<div class="empty">Could not reach the exchange.</div>';
    });
}

session().then(function () {
  renderHealth();
  renderStrip();
  wireTabs();
  load();
});
// Re-armed after every refresh, because both panels are rebuilt from scratch
// and the listeners on the old nodes go with them.
setInterval(function () { load(); }, 20000);
</script>
`
