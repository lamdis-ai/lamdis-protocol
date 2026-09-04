package api

import (
	"fmt"
	"strings"
)

// The block that asks somebody where they work.
//
// The board used to open with five practice jobs and nothing else, which is
// the page telling a visitor that the exchange is a rehearsal. It is worse
// than an empty board, because it wastes the one visit: the person leaves and
// nothing can reach them when work appears.
//
// So when there is no paid work a viewer could take, the page leads with the
// only useful thing it can ask for — where they are, what they can do, how far
// they will go, and an address to reach them on. Practice moves below it and
// says plainly what it is.
//
// Written once and included by both the queue and /coverage, because the
// honest sentence and the form under it must not drift apart.

// coverTravelMiles is the ladder of distances the form offers. The last is the
// same ceiling exchange.MaxCoverageRangeMiles and api.Capacities.Set clamp to,
// so the form cannot offer a promise the server would quietly shorten.
var coverTravelMiles = []int{5, 12, 25, 40, 60}

// coverDefaultMiles matches DefaultCapacity, so somebody who registers here
// and signs in later does not find their range silently changed.
var coverDefaultMiles = DefaultCapacity().RangeMiles

// coverCSS styles the block in the instrument-panel language: a glass panel
// with a gold edge, mono labels, and the same chips the capacity page uses.
const coverCSS = `
.cover { position: relative; margin: 0 0 1.5rem; padding: 1.2rem 1.3rem;
  border: 1px solid var(--rule); border-left: 2px solid var(--gold); border-radius: 6px;
  background: var(--glass); -webkit-backdrop-filter: blur(12px); backdrop-filter: blur(12px); }
.cover h2 { margin: .1rem 0 .5rem; font: 600 1.28rem/1.25 var(--sans);
  letter-spacing: -.02em; text-transform: none; color: var(--ink); }
.cover .say { margin: 0 0 1.1rem; max-width: 44rem; color: var(--ink-2); font-size: .92rem; line-height: 1.55; }
.cover .say b { color: var(--ink); font-weight: 600; }
.cover-grid { display: grid; gap: 1.2rem 2rem; grid-template-columns: 1fr; align-items: start; }
@media (min-width: 52rem) { .cover-grid { grid-template-columns: 1fr 1fr; } }
.cover-grid .col { display: flex; flex-direction: column; gap: 1.2rem; }
.cover .fld .label { display: block; margin-bottom: .45rem; }
.cover .row { display: flex; gap: .5rem; align-items: center; flex-wrap: wrap; }
.cover .row input { flex: 1 1 11rem; }
.cover .fine { margin: .45rem 0 0; font: 400 .78rem/1.5 var(--sans); color: var(--ink-3); }
.cover select { padding: .55rem .7rem; border: 1px solid var(--rule-2); border-radius: 3px;
  background: var(--panel); color: var(--ink); font: inherit; font-size: .9rem; }
.cover select:focus { outline: none; border-color: var(--gold); }
.cover .kinds { display: flex; flex-wrap: wrap; gap: .4rem; }
.cover .kind { padding: .3rem .6rem; border-radius: 2px; cursor: pointer; font-size: .8rem;
  border: 1px solid var(--rule-2); background: none; color: var(--ink-3); }
.cover .kind[aria-pressed="true"] { color: var(--ink); border-color: var(--gold); background: #1A1408; }
.cover .send { margin-top: 1.1rem; }
.cover .tally { margin: 1.1rem 0 0; padding-top: .85rem; border-top: 1px solid var(--rule);
  font: 400 .78rem/1.5 var(--mono); color: var(--ink-3); }
.cover .tally b { color: var(--ink-2); font-weight: 500; }
/* The rehearsal, framed as one. A heading over the practice jobs so nobody
   reads unpaid drill as an empty market. */
.rehearse { margin: 0 0 .9rem; padding: .7rem .9rem; border: 1px dashed var(--rule-2);
  border-radius: 4px; font-size: .84rem; line-height: 1.5; color: var(--ink-3); }
.rehearse b { color: var(--ink-2); font-weight: 600; }
`

// coverBlock is the markup. Hidden until the page knows whether there is real
// work in reach, so it never flashes over a board that has some.
var coverBlock = `<section class="cover" id="cover" hidden>
      <p class="eyebrow">Supply</p>
      <h2 id="cover-h">There is no paid work in your area yet</h2>
      <p class="say" id="cover-say"></p>
      <div id="cover-account" hidden>
        <a class="btn go" href="/console/capacity">Set your capacity</a>
      </div>
      <div id="cover-form" hidden>
        <div class="cover-grid">
          <div class="col">
          <div class="fld">
            <span class="label">Where you work</span>
            <div class="row">
              <button class="btn sm" type="button" id="cv-locate">Use my location</button>
              <input type="text" id="cv-place" maxlength="80" placeholder="Town or postcode">
            </div>
            <p class="fine" id="cv-where">A location is rounded to about a kilometre before it
              is stored, and nothing finer is ever kept. A town or postcode on its own is
              recorded but cannot be measured from, so nothing can be posted near it.</p>
          </div>
          <div class="fld">
            <span class="label">How far you will travel</span>
            <div class="row"><select id="cv-range" aria-label="How far you will travel">` +
	coverRangeOptions() + `</select></div>
          </div>
          <div class="fld">
            <span class="label">Where to reach you</span>
            <input type="email" id="cv-email" placeholder="you@example.com" autocomplete="email">
            <p class="fine">Used to tell you when work appears that you could take, and for
              nothing else. Every message carries a link that takes you off the register.</p>
          </div>
          </div>
          <div class="col">
          <div class="fld">
            <span class="label">What you can do</span>
            <div class="kinds" id="cv-skills"></div>
            <p class="fine">Licensed trades are marked. Claiming one you do not hold is fraud,
              and the work would carry your name.</p>
          </div>
          </div>
        </div>
        <div class="send"><button class="btn go" type="button" id="cv-send">Put me on the register</button></div>
        <div class="err" id="cv-err"></div>
      </div>
      <p class="tally" id="cover-tally"></p>
    </section>`

// coverRangeOptions builds the distance list from coverTravelMiles, so the
// figures on the page are the figures in the code.
func coverRangeOptions() string {
	var b strings.Builder
	for _, m := range coverTravelMiles {
		sel := ""
		if m == coverDefaultMiles {
			sel = " selected"
		}
		fmt.Fprintf(&b, `<option value="%d"%s>%d miles</option>`, m, sel, m)
	}
	return b.String()
}

// coverJS fills the block in, wires it, and posts it.
//
// renderCover is called by whichever page includes this with whether the
// viewer has real, paid work in reach. Everything it says in the negative case
// is true of an exchange with no supply: there is no work, this is not a job,
// and the only promise is an email.
const coverJS = `
var CV_SKILLS = [], CV_PICKED = {}, CV_LAT = 0, CV_LON = 0, CV_DONE = false;

// renderCover shows the block when there is nothing paid to show instead.
function renderCover(hasPaidWork) {
  var el = document.getElementById("cover");
  if (!el) { return; }
  if (hasPaidWork) { el.hidden = true; return; }
  el.hidden = false;
  var inAcct = signedIn();
  var h = document.getElementById("cover-h");
  var say = document.getElementById("cover-say");
  document.getElementById("cover-account").hidden = !inAcct;
  document.getElementById("cover-form").hidden = inAcct || CV_DONE;
  if (inAcct) {
    h.textContent = "There is no paid work in range of your capacity";
    say.innerHTML = "Nobody has posted paid work you could take here. " +
      "<b>Your capacity is the record the exchange dispatches against</b> — where you " +
      "work from, how far you will travel, and what you are qualified for. Check it, " +
      "and you are in the queue the moment work is posted near you.";
    return;
  }
  h.textContent = "There is no paid work in your area yet";
  say.innerHTML = "Nobody has posted paid work you could take here, and this page will not " +
    "pretend otherwise. <b>Say where you are and what you can do, and you are the first " +
    "told when there is.</b> No account, no fee, and nothing owed either way — " +
    "this is a register of who could work where, not a job.";
}

// coverTally reports, coarsely, how many people are already on the register.
// Buckets rather than figures, because the server will not publish a number
// under five and this must not invent one.
function coverTally() {
  var el = document.getElementById("cover-tally");
  if (!el) { return; }
  fetch("/v1/coverage").then(function (r) { return r.json(); }).then(function (j) {
    var areas = (j && j.areas) || [];
    if (!areas.length) {
      el.textContent = "Nobody is on the register yet. You would be the first.";
      return;
    }
    el.innerHTML = "On the register so far: <b>" + esc(j.interested) + "</b> registered, <b>" +
      esc(j.operators) + "</b> with a capacity set, across <b>" + areas.length +
      (areas.length === 1 ? "</b> area." : "</b> areas.") +
      " Counts are buckets and points are rounded to about a kilometre; nobody is named.";
  }).catch(function () { el.textContent = ""; });
}

// wireCover loads the skill catalogue and hooks the controls up. The
// catalogue is the same one a capacity uses, so what somebody claims here
// means the same thing there.
function wireCover() {
  var host = document.getElementById("cv-skills");
  if (!host) { return; }
  fetch("/v1/skills").then(function (r) { return r.json(); }).then(function (j) {
    CV_SKILLS = (j && j.skills) || [];
    host.innerHTML = CV_SKILLS.map(function (k) {
      return '<button type="button" class="kind' + (k.licensed ? " lic" : "") +
        '" data-cv-skill="' + esc(k.skill) + '" aria-pressed="false" title="' +
        esc(k.note || "") + '">' + esc(k.label) + "</button>";
    }).join("");
    host.querySelectorAll("[data-cv-skill]").forEach(function (b) {
      b.addEventListener("click", function () {
        var s = b.getAttribute("data-cv-skill");
        var on = b.getAttribute("aria-pressed") !== "true";
        b.setAttribute("aria-pressed", String(on));
        if (on) { CV_PICKED[s] = true; } else { delete CV_PICKED[s]; }
      });
    });
  }).catch(function () {
    host.innerHTML = '<span class="fine">Could not load the list of skills.</span>';
  });

  var locate = document.getElementById("cv-locate");
  locate.addEventListener("click", function () {
    var out = document.getElementById("cv-where");
    if (!navigator.geolocation) {
      out.textContent = "This browser will not share a location. Type a town or postcode instead.";
      return;
    }
    out.textContent = "Asking…";
    navigator.geolocation.getCurrentPosition(function (pos) {
      CV_LAT = Math.round(pos.coords.latitude * 1e7);
      CV_LON = Math.round(pos.coords.longitude * 1e7);
      out.textContent = "Got it. It is rounded to about a kilometre before it is stored, " +
        "and the exact point is not kept.";
    }, function () {
      out.textContent = "Location refused. A town or postcode still counts, but nothing " +
        "can measure a distance from it.";
    });
  });

  document.getElementById("cv-send").addEventListener("click", sendCover);
}

// sendCover posts the registration and reports back exactly what the server
// said it kept.
function sendCover() {
  var btn = document.getElementById("cv-send");
  var err = document.getElementById("cv-err");
  var email = (document.getElementById("cv-email").value || "").trim();
  var place = (document.getElementById("cv-place").value || "").trim();
  var miles = parseInt(document.getElementById("cv-range").value, 10) || 0;
  err.className = "err";
  if (!email) { err.textContent = "An email address is how we reach you."; return; }
  if (!place && !CV_LAT && !CV_LON) {
    err.textContent = "Say roughly where you work: share your location, or type a town or postcode.";
    return;
  }
  btn.disabled = true;
  err.textContent = "";
  fetch("/v1/coverage", {
    method: "POST", headers: {"Content-Type": "application/json"},
    body: JSON.stringify({
      email: email, place: place, lat_e7: CV_LAT, lon_e7: CV_LON,
      range_miles: miles, skills: Object.keys(CV_PICKED)
    })
  }).then(function (r) {
    return r.json().then(function (j) { return {ok: r.ok, body: j}; });
  }).then(function (res) {
    if (!res.ok) {
      btn.disabled = false;
      err.textContent = (res.body && res.body.error) || "Could not save that.";
      return;
    }
    CV_DONE = true;
    document.getElementById("cover-form").hidden = true;
    document.getElementById("cover-h").textContent = "You are on the register";
    document.getElementById("cover-say").textContent = res.body.note || "";
    coverTally();
  }).catch(function () {
    btn.disabled = false;
    err.textContent = "Could not reach the exchange.";
  });
}
`

// coveragePageHTML is /coverage: the same block on a page of its own, for
// somebody arriving from a link that says "work on Lamdis" rather than from
// the queue. It says the same thing the queue says, because it is the same
// thing.
var coveragePageHTML = coverageTop + themeCSS + coverCSS + coveragePageBody +
	workerJS + coverageBoardShim + coverJS + coveragePageScript

const coverageTop = `<!doctype html>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Where you work — Lamdis</title>
<style>`

var coveragePageBody = `</style>` + shellTop("queue", "") + `
    <div class="bhead">
      <div>
        <p class="eyebrow">Operator register</p>
        <h1>Where you work</h1>
        <p class="lead">Who could take work where, so the first paid jobs are posted
          somewhere somebody can reach.</p>
      </div>
    </div>
    ` + coverBlock + `
    <p class="note">The open work, whatever there is of it, is on <a href="/board">the board</a>.</p>
` + shellBottom + `
<script>
"use strict";
`

// coverageBoardShim gives this page the two helpers the block borrows from the
// board without pulling the whole queue in: escaping, and the masthead.
const coverageBoardShim = `
function esc(s) {
  return String(s == null ? "" : s).replace(/[&<>"']/g, function (c) {
    return {"&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;"}[c];
  });
}
`

const coveragePageScript = `
session().then(function () {
  var a = document.getElementById("h-auth"), t = document.getElementById("h-text");
  var b = document.getElementById("h-beacon");
  if (signedIn()) {
    t.textContent = "Taking work"; b.classList.remove("off");
    a.textContent = "Sign out";
    a.href = "#";
    a.onclick = function (e) { e.preventDefault(); clearSession(); location.reload(); };
  }
  // Always shown here: this page exists to ask the question, so it asks it
  // whether or not the board has anything on it today.
  renderCover(false);
  wireCover();
  coverTally();
  liveReading();
});

// liveReading fills the masthead pill the shell carries: how much work is
// open and what it adds up to. Practice pays nothing, so the sum is honest
// about a board that is all rehearsal.
function liveReading() {
  fetch("/v1/board").then(function (r) { return r.json(); }).then(function (b) {
    var work = (b && b.work) || [];
    var sum = work.reduce(function (a, w) { return a + (w.pay_minor || 0); }, 0);
    document.getElementById("live-n").textContent = String(work.length);
    document.getElementById("live-sum").textContent =
      "$" + (sum / 100).toFixed(2);
  }).catch(function () {
    document.getElementById("live-beacon").classList.add("off");
  });
}
</script>
`

// CoveragePage is the standalone register page.
func CoveragePage() string { return coveragePageHTML }
