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
    <div class="strip" id="strip"></div>
    <h1 id="title">` + title + `</h1>
    <div class="m" id="facts"></div>
    <div id="job"><div class="empty">Loading&hellip;</div></div>
    <div class="err" id="e-` + html.EscapeString(l.Job) + `"></div>
` + shellBottom + `
<script>
"use strict";
var JOB = ` + string(id) + `;
` + workerJS + boardJS + jobScript + `
</script>
`
}

const jobCSS = `
.crumb { margin: 0 0 .8rem; font: 500 .78rem/1 var(--mono); }
.crumb a { color: var(--ink-3); text-decoration: none; }
.crumb a:hover { color: var(--ink); }
#facts { margin: .3rem 0 1rem; font: 400 .8rem/1.5 var(--mono); color: var(--ink-3); }
.jd { border: 1px solid var(--rule); border-radius: 3px; padding: 1rem 1.1rem; }
.jd .pay { display: flex; align-items: baseline; gap: .6rem; margin: 0 0 .9rem; }
.jd .pay .n { font: 600 1.4rem/1 var(--mono); font-variant-numeric: tabular-nums; }
.jd .pay .n.quiet { color: var(--ink-3); font-weight: 500; }
.jd .pay .s { font-size: .78rem; color: var(--ink-3); }
.jd .pay .net { font: 500 .78rem/1 var(--mono); color: var(--ink-3); }
.jd .acts { display: flex; gap: .5rem; margin-top: 1rem; flex-wrap: wrap; }
.jd .stages { margin: 0; padding: 0; list-style: none; }
.jd .stages li { display: flex; justify-content: space-between; gap: 1rem;
  padding: .35rem 0; border-bottom: 1px solid var(--rule); font-size: .84rem; }
.jd .stages li:last-child { border-bottom: 0; }
.jd .stages b { color: var(--ink); font-weight: 600; }
.jd .stages span { color: var(--ink-3); font: 500 .8rem var(--mono); white-space: nowrap; }
.jd .stages small { display: block; color: var(--ink-3); font-weight: 400; }
.bid { display: grid; gap: .5rem; margin-top: .8rem; }
.bid-row { display: flex; gap: .45rem; align-items: stretch; }
.bid-row .cur {
  display: grid; place-items: center; width: 2.1rem; flex: none;
  border: 1px solid var(--rule-2); border-radius: 3px;
  background: var(--panel); color: var(--ink-3); font: 500 .9rem var(--mono);
}
.bid-row input { flex: 1; min-width: 0; font-family: var(--mono); }
.hint { margin: 0; font-size: .78rem; color: var(--ink-3); }
`

const jobScript = `
var WORK = null;

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
      'You can read this job; sign in to take it or bid on it &mdash; one email ' +
      'and a code. <a href="/signin?next=' + encodeURIComponent(location.pathname) +
      '">Sign in</a></span>';
    return;
  }
  el.className = "strip";
  el.innerHTML = '<span class="d"></span><span>Taking work. ' +
    '<a href="/board">Queue</a> &middot; <a href="/console">Your earnings</a></span>';
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

function payLine(w) {
  if (w.pricing === "bids") {
    return '<div class="pay"><span class="n quiet">Your bid</span>' +
      '<span class="s">nobody has told you a budget, and other bids are not shown</span></div>';
  }
  var net = takeHome(w.pay_minor);
  return '<div class="pay"><span class="n">' + money(w.pay_minor, w.currency) + '</span>' +
    '<span class="s">on completion</span>' +
    (net !== null && net !== w.pay_minor
      ? '<span class="net">' + money(net, w.currency) + ' to you</span>' : "") +
    '</div>';
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
    return '<div class="acts"><button class="btn go" id="open-bid">Bid</button></div>' +
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
  return '<div class="acts"><button class="btn go" data-job="' + esc(w.job) + '">' +
    (signedIn() ? "Take this job" : "Sign in to take this job") + '</button></div>';
}

function render() {
  var w = WORK, host = document.getElementById("job");
  document.getElementById("facts").innerHTML = facts(w);
  var blocked = (w.blocked_by || []).length > 0;
  host.innerHTML = '<div class="jd">' + payLine(w) +
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
