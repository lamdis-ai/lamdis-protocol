package api

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
)

// The page for one job.
//
// A dispatch email, a pushed offer and a shared link all name a job id, and
// until now none of them had anywhere to send somebody: the queue is a list,
// and a list is the wrong thing to land on when you were told about one job.
// This is what /v1/board/{job} returns, rendered, with the same take and bid
// controls the queue has — the JavaScript is shared, so the two cannot drift.
//
// It is public for the same reason the board is: a listing exists so that
// strangers can decide whether to price it. Somebody who is not signed in sees
// the job and is told what signing in gets them.

// jobPage renders the shell for one listing. The title is written into the
// page so a link preview, a crawler, or a browser with scripts off still says
// what the job is; everything else is drawn from /v1/board/{job} so the page
// shows what the API shows and not a second, drifting copy.
func jobPage(l *Listing) string {
	id, _ := json.Marshal(l.Job)
	title := html.EscapeString(l.Title)
	return `<!doctype html>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>` + title + ` — Lamdis</title>
<style>` + themeCSS + jobCSS + `</style>
` + shellTop("queue", "") + `
    <p class="crumb"><a href="/board">&larr; Queue</a></p>
    <div class="strip" id="strip" hidden></div>
    <p class="eyebrow" id="eyebrow">Open job</p>
    <h1 id="title">` + title + `</h1>
    <div class="m" id="facts"></div>
    <div class="hud" id="jhud" style="--cols:5" hidden></div>
    <div id="job"><div class="empty">Loading&hellip;</div></div>
    <div class="err" id="e-` + html.EscapeString(l.Job) + `"></div>
` + shellBottom + `
<script>
"use strict";
var JOB = ` + string(id) + `;
` + workerJS + boardJS + panelJS + jobScript + `
</script>
`
}

const jobCSS = `
.crumb { margin: 0 0 .8rem; font: 500 .72rem/1 var(--mono); letter-spacing: .1em; text-transform: uppercase; }
.crumb a { color: var(--ink-3); text-decoration: none; }
.crumb a:hover { color: var(--ink); }
h1 { font-size: 1.75rem; max-width: 46rem; }
#facts { margin: .3rem 0 1.2rem; font: 400 .76rem/1.7 var(--mono); color: var(--ink-3); }
#facts b { color: var(--ink-2); font-weight: 500; }
.hud .t.do .v { color: var(--gold); }
.hud .t.do::after { background: linear-gradient(90deg, var(--gold), transparent); }
.hud .v .pill { vertical-align: .3em; }
.jd { padding: 0; overflow: hidden; }
.jd .jgrid { padding: 1.1rem 1.25rem; border-top: 0; gap: 1.2rem 1.6rem; }
.jd .jgrid h4 { font: 600 .6rem/1 var(--mono); letter-spacing: .15em; }
.jd .fx { font-size: .9rem; color: var(--ink); }
.jd .fx b { font-weight: 600; }
.jd .shots { margin: 0; padding: 0 1.25rem 1.1rem; }
.jd .stages { margin: 0; padding: 0; list-style: none; }
.jd .stages li { display: flex; justify-content: space-between; gap: 1rem;
  padding: .4rem 0; border-bottom: 1px solid var(--rule); font-size: .86rem; }
.jd .stages li:last-child { border-bottom: 0; }
.jd .stages b { color: var(--ink); font-weight: 600; }
.jd .stages span { color: var(--gold); font: 600 .84rem var(--mono); white-space: nowrap;
  font-variant-numeric: tabular-nums; }
.jd .stages small { display: block; color: var(--ink-3); font-weight: 400; }
.jd .cta { display: flex; align-items: center; gap: 1rem; flex-wrap: wrap;
  padding: .95rem 1.25rem; border-top: 1px solid var(--rule); background: rgba(18,26,34,.5); }
.jd .cta .who { font: 500 .72rem/1.5 var(--mono); color: var(--ink-3); flex: 1; min-width: 12rem; }
.jd .cta .btn.go { height: 2.6rem; padding: 0 1.4rem; font-size: .92rem; }
.jd .acts { display: contents; }
.bid { display: grid; gap: .5rem; margin: 0; padding: 0 1.25rem 1.1rem; }
.bid-row { display: flex; gap: .45rem; align-items: stretch; }
.bid-row .cur {
  display: grid; place-items: center; width: 2.1rem; flex: none;
  border: 1px solid var(--rule-2); border-radius: 3px;
  background: var(--panel); color: var(--ink-3); font: 500 .9rem var(--mono);
}
.bid-row input { flex: 1; min-width: 0; font-family: var(--mono); }
.hint { margin: 0; font-size: .78rem; color: var(--ink-3); }
.ref { width: 8.5rem; }
`

const jobScript = `
var WORK = null;

function renderHealth() {
  var el = document.getElementById("h-text");
  var b = document.getElementById("h-beacon");
  var a = document.getElementById("h-auth");
  if (signedIn()) {
    el.textContent = "Taking work";
    b.classList.remove("off");
    a.textContent = "Console";
    a.href = "/console";
  } else {
    el.textContent = "Not signed in";
    b.classList.add("off");
    a.textContent = "Sign in";
    a.href = "/signin?next=" + encodeURIComponent(location.pathname);
  }
}

function renderStrip() {
  var el = document.getElementById("strip");
  if (!signedIn()) {
    el.hidden = false;
    el.className = "strip warn";
    el.innerHTML = '<span class="d"></span><span><b>You are not signed in.</b> ' +
      'You can read this job; sign in to take it or bid on it &mdash; one email ' +
      'and a code. <a href="/signin?next=' + encodeURIComponent(location.pathname) +
      '">Sign in</a></span>';
    return;
  }
  el.hidden = true;
  el.innerHTML = "";
}

// renderLive fills the masthead's reading from the same /v1/board call that
// fetches the terms.
function renderLive(b) {
  var work = (b && b.work) || [];
  var n = document.getElementById("live-n"), s = document.getElementById("live-sum");
  if (n) { n.textContent = String(work.length); }
  if (s) {
    s.textContent = money(work.reduce(function (a, w) { return a + (w.pay_minor || 0); }, 0), "USD");
  }
}

// renderHud is the header: the five readings that decide whether this job is
// worth a closer look, in the same tiles the queue uses.
function renderHud(w) {
  var host = document.getElementById("jhud");
  var bidding = w.pricing === "bids";
  var net = takeHome(w.pay_minor);
  var tile = function (cls, k, v, s, i) {
    return '<div class="t' + (DREW ? " " : " rv ") + cls + '" style="--i:' + i + '"><div class="k">' + k + '</div>' +
      '<div class="v">' + v + '</div><div class="s">' + s + '</div></div>';
  };
  var payV = bidding ? "Your bid" : '<span id="hud-pay">' + money(w.pay_minor, w.currency) + '</span>';
  var payS = bidding
    ? "nobody has told you a budget"
    : (w.practice ? "practice run, pays nothing"
      : (net !== null && net !== w.pay_minor ? money(net, w.currency) + " to you" : "on completion"));
  if (!bidding && w.bonus_minor) { payS += " · +" + money(w.bonus_minor, w.currency) + " if yes"; }
  host.hidden = false;
  host.innerHTML =
    tile(bidding || !w.pay_minor ? "" : "money", "Pays", payV, payS, 0) +
    tile(w.kind === "do" ? "do" : "soft", "Kind", kindLabel(w.kind),
      w.kind === "do" ? "make something true" : "find something out", 1) +
    tile("", "Where", w.area ? esc(w.area) : "&mdash;",
      w.area ? "exact address once you take it" : "the buyer gave no area", 2) +
    tile(w.distance_miles ? "ok" : "", "Distance",
      w.distance_miles ? w.distance_miles + '<small>mi</small>' : "&mdash;",
      w.distance_miles ? "from your location" : (signedIn() ? "set your location in Capacity" : "sign in to see"), 3) +
    tile("wait", bidding ? "Bids close" : "Closes",
      left(bidding && w.bids_close_at ? w.bids_close_at : w.expires),
      bidding ? "then the buyer picks one" : "unless somebody takes it first", 4);
  if (!bidding) {
    countUp(document.getElementById("hud-pay"), w.pay_minor || 0, function (v) {
      return money(Math.round(v), w.currency);
    });
  }
}

// facts is the one-line summary under the title: the same items, in the same
// words, that the queue row shows.
function facts(w) {
  var f = ['<span class="chip hot">' + kindLabel(w.kind) + '</span>'];
  if (w.practice) { f.push('<b>practice run — not real work, pays nothing</b>'); }
  if (w.distance_miles) { f.push('<b>' + w.distance_miles + ' mi</b>'); }
  if (w.area) { f.push(esc(w.area)); }
  if (w.skills && w.skills.length) { f.push("needs " + w.skills.map(esc).join(", ")); }
  if (w.posted_by_agent) { f.push("posted by an agent"); }
  if (w.stages && w.stages.length) { f.push("<b>" + w.stages.length + " stages</b>, paid as you go"); }
  if (w.work_hours >= 24) { f.push(Math.round(w.work_hours / 24) + " day job"); }
  else if (w.work_hours) { f.push(w.work_hours + "h job"); }
  if (w.pricing === "bids") {
    f.push("you name the price");
    if (w.bids_close_at) { f.push("bids close in " + left(w.bids_close_at)); }
  }
  if (w.bonus_minor) { f.push("+" + money(w.bonus_minor, w.currency) + " if the answer is yes"); }
  if (w.attempt_minor) { f.push(money(w.attempt_minor, w.currency) + " if it is impossible"); }
  if (w.expense_cap_minor) { f.push("expenses to " + money(w.expense_cap_minor, w.currency)); }
  if (w.expires) { f.push("open for " + left(w.expires)); }
  return f.join(" &middot; ");
}

function section(h, body) {
  return body ? '<div><h4>' + h + '</h4>' + body + '</div>' : "";
}

function stagesList(w) {
  if (!w.stages || !w.stages.length) { return ""; }
  return '<ul class="stages">' + w.stages.map(function (st, i) {
    return '<li><div><b>' + (i + 1) + '. ' + esc(st.name) + '</b>' +
      (st.deliverable ? '<small>' + esc(st.deliverable) + '</small>' : "") + '</div>' +
      '<span>' + money(st.pay_minor || 0, w.currency) +
      (st.materials ? " materials" : "") + '</span></li>';
  }).join("") + '</ul>';
}

function unknownsList(w) {
  var us = w.unknowns || [];
  if (!us.length) { return ""; }
  return us.map(function (u) {
    return '<p class="fx"><b>' + esc(u.name) + '</b>' +
      (u.unit ? ' <i>(' + esc(u.unit) + ')</i>' : "") +
      (u.note ? ' — ' + esc(u.note) : "") + '</p>';
  }).join("") + '<p class="fn">A bid says what it priced these on.</p>';
}

function projectLine(w) {
  var p = w.project;
  if (!p) { return ""; }
  return '<p class="fx">' + (p.title ? '<b>' + esc(p.title) + '</b> ' : "") +
    'piece ' + esc(p.position || "?") + ' of ' + esc(p.jobs) +
    (p.one_visit ? ", all at one address" : "") +
    (p.bids_as_one ? " &middot; one offer can cover the whole scope" : "") + '</p>';
}

function actions(w) {
  if (w.pricing === "bids") {
    return '<div class="cta"><span class="who">You are naming your own price. Nobody has told you ' +
        'a budget, and other bids are not shown.</span>' +
        '<button class="btn go" id="open-bid">' + (signedIn() ? "Bid on this job" : "Sign in to bid") + '</button></div>' +
      '<div class="bid" id="bid-' + esc(w.job) + '" hidden>' +
        '<div class="bid-row"><span class="cur">$</span>' +
          '<input type="text" inputmode="decimal" placeholder="45.00" data-bid="' + esc(w.job) + '">' +
          '<button class="btn go sm" data-place="' + esc(w.job) + '">Place bid</button></div>' +
        '<input type="text" maxlength="140" placeholder="How you would do it (optional)" data-note="' + esc(w.job) + '">' +
        askUnknowns(w) +
        '<p class="hint">You are naming your own price. Nobody has told you a budget, ' +
          'and other bids are not shown.</p>' +
      '</div>';
  }
  var who = w.practice
    ? "A practice run: the buttons are real, the money is not."
    : (w.posted_by_agent ? "Posted by an agent. Paid on proof, released on acceptance."
      : "Paid on proof, released on acceptance.");
  return '<div class="cta"><span class="who">' + who + '</span>' +
    '<button class="btn go" data-job="' + esc(w.job) + '">' +
    (signedIn() ? "Take this job" : "Sign in to take this job") + '</button></div>';
}

var DREW = false;

function render() {
  var w = WORK, host = document.getElementById("job");
  // The page renders once for the job and again when the terms arrive; the
  // arrival animation belongs to the first.
  var rv = DREW ? "" : " rv";
  DREW = true;
  document.getElementById("facts").innerHTML = facts(w);
  document.getElementById("eyebrow").textContent = w.practice ? "Practice job"
    : (w.pricing === "bids" ? "Open for bids" : "Open job");
  renderHud(w);
  var blocked = (w.blocked_by || []).length > 0;
  host.innerHTML = '<div class="jd glass' + rv + '" style="--i:5">' +
    '<div class="jgrid">' +
      section("What proves it", w.deliverable ? '<p class="fx">' + esc(w.deliverable) + '</p>' : "") +
      section("The job", w.brief ? '<p class="fx">' + esc(w.brief) + '</p>' : "") +
      section("The buyer does not know", unknownsList(w)) +
      section("Stages", stagesList(w)) +
      section("Part of a larger scope", projectLine(w)) +
      (blocked
        ? section("Cannot start yet", '<p class="fx">' + esc(w.blocked_by.join(", ")) +
            ' has to be finished and accepted first.</p>')
        : "") +
      (w.withheld ? section("Withheld", '<p class="fx">' + esc(w.withheld) + '</p>') : "") +
    '</div>' +
    siteShots(w) +
    actions(w) +
  '</div>';

  host.querySelectorAll("button[data-job]").forEach(function (b) {
    b.addEventListener("click", function () { takeJob(b, b.getAttribute("data-job")); });
  });
  var ob = document.getElementById("open-bid");
  if (ob) {
    ob.addEventListener("click", function () {
      if (!signedIn()) { goSignIn(); return; }
      var box = document.getElementById("bid-" + w.job);
      box.hidden = !box.hidden;
      if (!box.hidden) { box.querySelector("input").focus(); }
    });
  }
  host.querySelectorAll("button[data-place]").forEach(function (b) {
    b.addEventListener("click", function () { placeBid(b, b.getAttribute("data-place")); });
  });
}

function load() {
  var path = "/v1/board/" + encodeURIComponent(JOB);
  // Signed in, the same request also says how far away the job is.
  (signedIn()
    ? workerHeaders("GET", path).then(function (h) { return fetch(path, {headers: h}); })
    : fetch(path))
    .then(function (r) {
      if (r.status === 404) { throw new Error("gone"); }
      return r.json();
    })
    .then(function (w) { WORK = w; render(); })
    .catch(function (e) {
      document.getElementById("job").innerHTML = '<div class="empty">' +
        (e.message === "gone"
          ? 'This job is no longer listed. <a href="/board">See what is open</a>.'
          : 'Could not reach the exchange.') + '</div>';
    });
  // The fee and payout terms, so the take-home reads the same as on the queue.
  fetch("/v1/board").then(function (r) { return r.json(); }).then(function (b) {
    TERMS = (b && b.terms) || null;
    renderLive(b);
    if (WORK) { render(); }
  }).catch(function () {});
}

session().then(function () {
  renderHealth();
  renderStrip();
  load();
});
`

// handleJobPage serves the page for one listing.
//
// The same refusal as /v1/board/{job}: directed work is not on the open board
// and has no public page, and saying "no such job" rather than "not yours"
// keeps the page from confirming what a buyer sends to whom.
func (s *WorkerServer) handleJobPage(w http.ResponseWriter, r *http.Request) {
	l, ok := s.Board.Get(r.PathValue("job"))
	if !ok || l.Directed() {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `<!doctype html><meta charset="utf-8"><title>No such job — Lamdis</title>`+
			`<style>`+themeCSS+`</style>`+shellTop("queue", "")+
			`<h1>No such job</h1><p class="lead">Nothing is listed under that id. `+
			`<a href="/board">See what is open</a>.</p>`+shellBottom)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Referrer-Policy", "no-referrer")
	fmt.Fprint(w, jobPage(l))
}
