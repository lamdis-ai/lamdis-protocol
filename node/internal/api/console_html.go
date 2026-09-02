package api

// The console: where a person finds out where they stand with the exchange,
// on whichever side of it they are.
//
// It used to be one page, two thousand lines of script and a rail of anchors.
// An operator looking for their statement scrolled past the buyer's post-a-job
// form; a buyer checking a receipt loaded the radar, the capacity sliders and
// the paving demonstration. Every visit fetched twelve routes to show one.
//
// Now it is a set of pages under one shell. Each route renders one thing,
// fetches only what that thing needs, and carries only the script that draws
// it. What the pages share — money formatting, the authenticated fetch, the
// pills, the side switch, the sample data — lives once in consoleJS.
//
// Two sides. An operator wants to know what they have earned, what is holding
// it up, what they hold, and how work reaches them. A buyer wants to know what
// their money is doing, what came back, and the keys that spend it. The rail
// lists both sides as real links; the switch in the header jumps between the
// two landing pages and remembers which side somebody was last on.
//
// Everything here is wired to a real endpoint. A settings page whose controls
// do not reach the dispatcher is worse than no settings page.

// consoleCSS is what is particular to these pages. The instrument-panel
// pieces every page shares — HUD tiles, glass, radar, pills, the mode switch —
// live in theme.go.
const consoleCSS = `
/* The cockpit header: the title, the sign-in and payout state as pills, and
   the switch between the two sides of the exchange. */
.cockpit { display: flex; align-items: flex-start; gap: 1rem; flex-wrap: wrap; margin: 0 0 1.3rem; }
.cockpit h1 { margin: 0 0 .25rem; }
.cockpit .lead { margin: 0; max-width: 40rem; }
.cockpit-r { margin-left: auto; display: flex; align-items: center; gap: .55rem; flex-wrap: wrap; }
.pill.ok { color: var(--green); border-color: #1C4530; }
.pill.warn { color: var(--warn); border-color: #3A2510; }
.pill .beacon.warn { background: var(--warn); animation: none; }
.pill .beacon.gold { background: var(--gold); animation: none; }
.mode button:focus-visible { outline: 2px solid var(--gold); outline-offset: -2px; }
.mode-grp { display: contents; }

/* Section heads: the eyebrow, a live count beside it, one line of why. */
.sh { display: flex; align-items: baseline; gap: .7rem; flex-wrap: wrap; margin: 1.9rem 0 .7rem; }
.sh h2 { margin: 0; }
.sh .n { font: 600 .64rem/1 var(--mono); color: var(--gold); letter-spacing: .1em; font-variant-numeric: tabular-nums; }
.sh .n.quiet { color: var(--ink-3); }
.sh .sub { font: 500 .68rem/1.4 var(--mono); color: var(--ink-3); }
.main .lead { font-size: .86rem; margin: 0 0 .9rem; }

/* Panels: every settings group is a glass tile over the ground. */
.panes { display: grid; gap: .7rem; }
@media (min-width: 50rem) { .panes { grid-template-columns: 1fr 1fr; } }
.pane { background: var(--glass); -webkit-backdrop-filter: blur(12px); backdrop-filter: blur(12px);
  border: 1px solid var(--rule); border-radius: 6px; padding: 1.05rem 1.1rem;
  box-shadow: inset 0 1px 0 rgba(255,255,255,.04); min-width: 0; }
.pane h3 { margin: 0 0 .3rem; font: 600 .62rem/1 var(--mono); letter-spacing: .15em;
  text-transform: uppercase; color: var(--ink-2); }
.pane p.why { margin: 0 0 .85rem; color: var(--ink-3); font-size: .82rem; }
.hud .t { min-width: 0; }
.hud .v { white-space: nowrap; }
.hud .v.n { font-family: var(--mono); font-weight: 600; letter-spacing: 0; }
.hud .s a { color: var(--gold); text-decoration: none; }
.radar-wrap canvas { height: 17rem; }
.radar-wrap .hint a { color: var(--gold); }
.rows .r { font-variant-numeric: tabular-nums; }
.rows .r .acts { margin-top: .45rem; }
.empty { padding: 1.1rem 1rem; text-align: left; font: 500 .78rem/1.5 var(--mono); color: var(--ink-3); }
.empty a { color: var(--ink-2); }
.demo { margin: 0 0 1.2rem; }

.ctl { display: flex; align-items: center; gap: .7rem; margin-bottom: .5rem; }
.ctl input[type=range] { flex: 1; accent-color: var(--gold); }
.ctl .v { font: 600 .92rem/1 var(--mono); min-width: 3.6rem; text-align: right; font-variant-numeric: tabular-nums; }

.toggle { display: flex; align-items: center; justify-content: space-between;
          gap: 1rem; padding: .6rem 0; border-top: 1px solid var(--rule); }
.toggle .tx { font-size: .87rem; }
.toggle .sx { margin-top: .12rem; font-size: .78rem; color: var(--ink-3); }
.sw { position: relative; width: 2.3rem; height: 1.3rem; flex: none; cursor: pointer;
      border-radius: 999px; border: 1px solid var(--rule-2); background: var(--panel-2); }
.sw::after { content: ""; position: absolute; top: 2px; left: 2px;
  width: .95rem; height: .95rem; border-radius: 50%; background: var(--ink-3);
  transition: transform .16s, background .16s; }
.sw[aria-pressed="true"] { background: #133020; border-color: #1C4530; }
.sw[aria-pressed="true"]::after { transform: translateX(.98rem); background: var(--green); }
.sw:focus-visible { outline: 2px solid var(--gold); outline-offset: 2px; }

.kinds { display: flex; flex-wrap: wrap; gap: .4rem; }
.kind { padding: .3rem .6rem; border-radius: 2px; cursor: pointer; font-size: .8rem;
        border: 1px solid var(--rule-2); background: none; color: var(--ink-3); }
.kind[aria-pressed="true"] { color: var(--ink); border-color: var(--gold); background: #1A1408; }

.keyline { display: flex; align-items: center; gap: .6rem; padding: .6rem .8rem;
  border: 1px solid var(--rule); border-radius: 3px; background: var(--panel);
  font: 500 .82rem/1 var(--mono); color: var(--ink-2); }
.setup { margin: .7rem 0 0; padding: .85rem .9rem; border: 1px solid var(--rule);
  border-radius: 4px; background: var(--panel); }
.setup .bar { height: 2px; background: var(--rule); border-radius: 2px;
  overflow: hidden; margin-bottom: .7rem; }
.setup .bar span { display: block; height: 100%; width: 35%; background: var(--gold);
  animation: slide 1.4s ease-in-out infinite; }
@keyframes slide {
  0%   { transform: translateX(-100%); }
  100% { transform: translateX(340%); }
}
@media (prefers-reduced-motion: reduce) {
  .setup .bar span { animation: none; width: 100%; opacity: .5; }
}
.setup-step { font: 600 .9rem/1.35 var(--sans); color: var(--ink); }
.setup-note { margin-top: .2rem; font-size: .82rem; color: var(--ink-3); }
.setup-clock { margin-top: .45rem; font: 500 .74rem/1 var(--mono); color: var(--ink-3);
  font-variant-numeric: tabular-nums; }
.ask { display: block; margin: .1rem 0 .35rem; font: 600 .82rem/1.3 var(--sans); }
textarea { width: 100%; box-sizing: border-box; padding: .55rem .6rem;
  border: 1px solid var(--rule-2); border-radius: 4px; background: var(--bg);
  color: var(--ink); font: inherit; font-size: .9rem; resize: vertical; }
textarea:focus-visible, .amt-in:focus-visible { outline: 2px solid var(--gold);
  outline-offset: 1px; }
.ask-acts { display: flex; gap: .5rem; margin-top: .5rem; }
.cur { color: var(--ink-3); font: 600 .95rem/1 var(--mono); }
.amt-in { width: 6rem; padding: .4rem .5rem; border: 1px solid var(--rule-2);
  border-radius: 4px; background: var(--bg); color: var(--ink); font: inherit;
  font-variant-numeric: tabular-nums; }
.secret { display: block; margin: .4rem 0 0; padding: .5rem .6rem;
  border: 1px solid var(--rule-2); border-radius: 6px; background: var(--panel-2);
  font: 600 .8rem/1.4 var(--mono); word-break: break-all; }
.keyline .k { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; }
.reveal { margin-top: .6rem; padding: .75rem .85rem; border-radius: 3px;
  border: 1px solid #4A3410; background: rgba(23,17,6,.6); }
.reveal .k { display: block; margin: .35rem 0; font: 600 .9rem/1.4 var(--mono);
  color: var(--gold); word-break: break-all; }
.reveal p { margin: 0; font-size: .78rem; color: var(--ink-3); }

/* Forms. The post-a-job form is the first real form on the console, so the
   field styles live here rather than in the theme. */
.field { margin: 0 0 .85rem; }
.field label { display: block; font: 600 .8rem/1.3 var(--sans); margin-bottom: .3rem; }
.field .hint { font-size: .76rem; color: var(--ink-3); margin-top: .25rem; }
.field input[type=text], .field input[type=number], .field input[type=date],
.field input[type=month], .field select, .field textarea {
  width: 100%; padding: .55rem .65rem; border: 1px solid var(--rule-2); border-radius: 3px;
  background: var(--panel); color: var(--ink); font-size: .9rem; }
.field input[type=number] { font-family: var(--mono); font-variant-numeric: tabular-nums; }
.field select { appearance: auto; }
.field input:focus, .field select:focus { outline: none; border-color: var(--gold); }
.two { display: grid; gap: .8rem; grid-template-columns: 1fr 1fr; }
.three { display: grid; gap: .8rem; grid-template-columns: 1fr 1fr 1fr; }
.seg { display: inline-flex; gap: .35rem; flex-wrap: wrap; }
.seg .kind { font-size: .84rem; padding: .4rem .75rem; }
.fund { margin: .6rem 0 .9rem; padding: .65rem .8rem; border: 1px solid var(--rule);
  border-left: 2px solid var(--green); background: var(--panel); font-size: .84rem;
  color: var(--ink-2); border-radius: 0 3px 3px 0; }
.fund.short { border-left-color: var(--warn); }
.fund b { color: var(--ink); font-family: var(--mono); font-variant-numeric: tabular-nums; }

/* Post a job: the fields, and beside them the row the board will show. The
   preview follows every keystroke, so what the operator sees is never a
   surprise the buyer discovers after posting. */
.post-grid { display: grid; gap: .8rem; }
@media (min-width: 62rem) {
  .post-grid { grid-template-columns: minmax(0, 1.05fr) minmax(0, .95fr); align-items: start; }
  .pv-wrap { position: sticky; top: 4rem; }
}
.post-grid .panes { grid-template-columns: 1fr; }
.pv { border: 1px solid var(--rule-2); border-radius: 8px; background: var(--glass-2);
  -webkit-backdrop-filter: blur(12px); backdrop-filter: blur(12px); overflow: hidden;
  box-shadow: 0 30px 80px rgba(0,0,0,.45), inset 0 1px 0 rgba(255,255,255,.04); }
.pv-top { display: flex; align-items: center; justify-content: space-between; gap: 1rem;
  padding: .65rem .95rem; border-bottom: 1px solid var(--rule);
  font: 600 .62rem/1 var(--mono); letter-spacing: .14em; text-transform: uppercase; color: var(--ink-3); }
.pv-top .live { display: flex; align-items: center; gap: .45rem; color: var(--green); }
.pv-top .live .beacon { width: .38rem; height: .38rem; }
.pv .jrow { cursor: default; border-left-color: var(--rule-2); }
.pv .jrow:hover { background: none; }
.pv.obs .jrow { border-left-color: var(--blue); }
.pv.do .jrow { border-left-color: var(--gold); }
.pv .jt.ph, .pv .dv.ph { color: var(--ink-3); font-weight: 400; }
.pv .meta { font: 500 .72rem/1.6 var(--mono); color: var(--ink-3); }
.pv .checks { padding: .75rem .95rem .9rem; border-top: 1px solid var(--rule); }
.pv .checks .k { font: 600 .6rem/1 var(--mono); letter-spacing: .15em; text-transform: uppercase; color: var(--ink-3); }
.pv .checks ul { list-style: none; margin: .5rem 0 0; padding: 0; display: grid; gap: .32rem; }
.pv .checks li { display: flex; gap: .55rem; align-items: center; color: var(--ink-3); font-size: .82rem; transition: color .3s; }
.pv .checks li i { width: 13px; height: 13px; border-radius: 50%; border: 1px solid var(--rule-2); flex: none;
  display: inline-grid; place-items: center; font-style: normal; font-size: 8px; transition: all .3s; }
.pv .checks li.on { color: var(--ink-2); }
.pv .checks li.on i { background: var(--green); border-color: var(--green); color: #04160A; }
.pv .checks li.on i::before { content: "\2713"; }
.pv .hold { display: flex; justify-content: space-between; align-items: center; gap: 1rem;
  padding: .8rem .95rem; border-top: 1px solid var(--rule);
  background: linear-gradient(90deg, rgba(255,182,39,.10), transparent); }
.pv .hold .amt { font: 700 1.3rem/1 var(--sans); color: var(--gold); letter-spacing: -.02em; font-variant-numeric: tabular-nums; }
.pv .hold .who { display: block; margin-top: .25rem; font: 500 .68rem/1.4 var(--mono); color: var(--ink-3); }
.pv .hold .st { font: 600 .62rem/1 var(--mono); letter-spacing: .14em; text-transform: uppercase; color: var(--ink-3); text-align: right; }
.pv .hold .st.ok { color: var(--green); }
.pv .hold .st.short { color: var(--warn); }
.pv .priv { padding: .6rem .95rem; border-top: 1px dashed var(--rule); font: 500 .7rem/1.5 var(--mono); color: var(--ink-3); }

.tablewrap { overflow-x: auto; border: 1px solid var(--rule); border-radius: 6px; background: var(--glass);
  -webkit-backdrop-filter: blur(12px); backdrop-filter: blur(12px); }
table.stmt { width: 100%; border-collapse: collapse; font-size: .84rem; }
table.stmt th, table.stmt td { padding: .5rem .6rem; border-bottom: 1px solid var(--rule);
  text-align: left; white-space: nowrap; }
table.stmt th { font: 600 .62rem/1 var(--mono); letter-spacing: .12em;
  text-transform: uppercase; color: var(--ink-3); }
table.stmt td.n, table.stmt th.n { text-align: right; font-family: var(--mono);
  font-variant-numeric: tabular-nums; }
table.stmt tr.tot td { border-top: 1px solid var(--rule-2); font-weight: 600; }
table.stmt tr.tot td.n { color: var(--gold); }
table.stmt tr:last-child td { border-bottom: 0; }

/* The receipt and the evidence behind it, read in the page rather than as
   JSON in a tab that a browser session could not even open. */
.jx { margin-top: .5rem; }
.receipt { border: 1px solid var(--rule); border-radius: 6px; padding: .9rem 1rem;
  margin-top: .6rem; background: var(--glass-2); font-size: .86rem; }
.receipt h4 { margin: .9rem 0 .35rem; font: 600 .6rem/1 var(--mono);
  text-transform: uppercase; letter-spacing: .15em; color: var(--ink-3); }
.receipt h4:first-child { margin-top: 0; }
.receipt ul { margin: 0; padding-left: 1.1rem; color: var(--ink-2); }
.receipt li { margin-bottom: .25rem; }
.receipt .ceil { font: 600 1.5rem/1 var(--mono); color: var(--gold); font-variant-numeric: tabular-nums; }
.receipt .fn { margin-top: .3rem; }
.receipt details { margin-top: .8rem; }
.receipt summary { cursor: pointer; font-size: .8rem; color: var(--ink-3); }
.receipt pre { margin: .5rem 0 0; padding: .6rem .7rem; background: var(--bg);
  border: 1px solid var(--rule); border-radius: 3px; overflow-x: auto;
  font: 500 .72rem/1.5 var(--mono); color: var(--ink-2); white-space: pre-wrap;
  word-break: break-all; }
.ev { display: flex; flex-wrap: wrap; gap: .6rem; margin-top: .6rem; }
.ev figure { margin: 0; width: 13rem; }
.ev img, .ev video { display: block; width: 100%; border: 1px solid var(--rule-2);
  border-radius: 3px; background: var(--panel); }
.ev figcaption { font: 500 .68rem/1.4 var(--mono); color: var(--ink-3);
  margin-top: .25rem; word-break: break-all; }
.ev figcaption b { color: var(--ink-2); font-weight: 500; }
.ev .flag { color: var(--warn); }
.bid { display: flex; gap: .8rem; align-items: flex-start; padding: .6rem 0;
  border-top: 1px solid var(--rule); font-size: .85rem; }
.bid .amt { margin-left: auto; font: 600 .95rem/1 var(--mono); color: var(--gold); white-space: nowrap; }
.acts { display: flex; gap: .4rem; flex-wrap: wrap; }

/* Supplier profile: licences are rows, because a business has several. */
.lic { display: grid; grid-template-columns: 1.3fr 1fr .6fr 1fr auto; gap: .4rem;
  margin-bottom: .4rem; align-items: center; }
.lic input, .lic select { width: 100%; padding: .45rem .5rem; border: 1px solid var(--rule-2);
  border-radius: 3px; background: var(--panel); color: var(--ink); font-size: .84rem; }
.lic .chip { margin: 0; }
.member { display: flex; align-items: center; gap: .6rem; padding: .45rem 0;
  border-top: 1px solid var(--rule); font: 500 .82rem/1 var(--mono); }
.member span { flex: 1; overflow: hidden; text-overflow: ellipsis; }
.refs { display: flex; flex-wrap: wrap; gap: .4rem; margin-top: .4rem; }
.refs img { width: 4.5rem; height: 3.4rem; object-fit: cover; border: 1px solid var(--rule-2);
  border-radius: 3px; }
.attn { margin: .5rem 0 0; padding-left: 1.1rem; font-size: .82rem; color: var(--warn); }
/* One page per thing, so the rail is long enough to need tightening. */
.rail a { padding: .42rem .65rem; }
.rail .grp { padding: .75rem .65rem .3rem; }
.cockpit .eyebrow { margin-bottom: .3rem; }
.more { margin: .7rem 0 0; font-size: .84rem; }
.more a { color: var(--gold); text-decoration: none; }
.more a:hover { text-decoration: underline; }
.nf { padding: 3rem 0; }
.nf ul { list-style: none; padding: 0; margin: 1rem 0 0; display: grid; gap: .3rem; grid-template-columns: repeat(auto-fit, minmax(12rem, 1fr)); }
.nf a { color: var(--gold); text-decoration: none; font-size: .9rem; }
`

// consoleJS is what every console page needs and nothing any single page
// owns: formatting, the authenticated fetch, the masthead and pills, the side
// switch, the anchor redirects, the boot sequence and the sample data.
//
// A page declares PAGE and SIDE before this runs, then calls boot() with the
// routes it needs and a function that draws from the result.
const consoleJS = `
// panelJS reaches the browser's animation and event globals unqualified. The
// page audit in pagescript_test only credits names a page defines, so they are
// bound here to window, which is where they live anyway. The event pair is
// taken from the prototype: a top-level var shadows an inherited property
// before this line runs, so window.addEventListener would already be undefined.
var requestAnimationFrame = window.requestAnimationFrame.bind(window);
var cancelAnimationFrame = window.cancelAnimationFrame.bind(window);
var addEventListener = EventTarget.prototype.addEventListener.bind(window);
var removeEventListener = EventTarget.prototype.removeEventListener.bind(window);

// Set to true by the server for ?demo=1 on an exchange with no identity
// provider. Never on a real one; see Console.handlePage.
var DEMO_ALLOWED = false;
var DEMO = false;
// The demonstration scope, named once so the page and the example calls it
// prints can never drift apart.
var DEMO_PROJECT = "proj-demo-paving";

// What the pages read. Declared here rather than per page so the sample data
// can fill any of them and a page only assigns the ones it fetched.
var ME = null, CAP = null, KEYS = [], SUP = null, STMT = null, ALERTS = null, PAYOUT = null;
var HOLDING = [], BOARD = null, SKILLS = [], SPEND = null, SCOPE = null;
var LOADER = null;

function esc(s) {
  return String(s == null ? "" : s).replace(/[&<>"']/g, function (c) {
    return {"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c];
  });
}
function money(m, cur) {
  var sign = m < 0 ? "-" : "", v = Math.abs(Math.round(m || 0));
  var sym = (cur || "USD") === "USD" ? "$" : cur + " ";
  var whole = String(Math.floor(v / 100)).replace(/(\d)(?=(\d{3})+$)/g, "$1,");
  return sign + sym + whole + "." + String(v % 100).padStart(2, "0");
}
// fmt is the count-up formatter for tiles that count things rather than money.
function fmt(n) { return String(Math.round(n)); }
function when(iso) {
  var d = new Date(iso);
  return isNaN(d) ? "" : d.toLocaleDateString(undefined, {month: "short", day: "numeric"});
}
function whenAt(iso) {
  var d = new Date(iso);
  return isNaN(d) ? "" : d.toLocaleDateString(undefined, {month: "short", day: "numeric"}) +
    " " + d.toLocaleTimeString(undefined, {hour: "numeric", minute: "2-digit"});
}
// Dollars typed into a field, as minor units. Anything unreadable is zero.
function minorOf(id) {
  var el = document.getElementById(id);
  var v = el ? parseFloat(el.value) : 0;
  return v > 0 ? Math.round(v * 100) : 0;
}
// A date input's value from a server timestamp. Go serialises an unset time as
// the year one, which is not a date anybody set.
function dateOf(iso) {
  if (!iso || iso.indexOf("0001-") === 0) { return ""; }
  return String(iso).slice(0, 10);
}
function val(id) {
  var el = document.getElementById(id);
  return el ? el.value.trim() : "";
}
function bodyEl() { return document.getElementById("body"); }

// api sends one authenticated request and hands back what came back, parsed
// when it is JSON. A 401 is a session that has ended, not an error to print.
function api(method, path, body, rawType) {
  return workerHeaders(method, path).then(function (h) {
    var opts = {method: method, headers: h};
    if (body != null) {
      if (rawType) { h["Content-Type"] = rawType; opts.body = body; }
      else { h["Content-Type"] = "application/json"; opts.body = JSON.stringify(body); }
    }
    return fetch(path, opts);
  }).then(function (r) {
    if (handleAuthFailure(r.status)) { throw new Error("signed out"); }
    return r.text().then(function (t) {
      var j = null;
      try { j = t ? JSON.parse(t) : null; } catch (e) {}
      return {ok: r.ok, status: r.status, body: j, text: t};
    });
  });
}
function errorOf(res, fallback) {
  return (res.body && res.body.error) || fallback;
}
// One authenticated GET that must not take the page down with it: anything
// that fails resolves to a null body rather than rejecting.
function softGet(path) {
  return workerHeaders("GET", path)
    .then(function (h) { return fetch(path, {headers: h}); })
    .catch(function () { return {ok: false, status: 0, json: function () { return null; }}; });
}

// --- arrival -----------------------------------------------------------------
//
// Rows and panels rise into place in the order they are read. The stagger is
// capped so a long list does not keep somebody waiting on its tail.

var RV = 0;
function rv(extra) { return ' class="rv ' + (extra || "") + '" style="--i:' + Math.min(RV++, 14) + '"'; }
function rvi(i, extra) { return ' class="rv ' + (extra || "") + '" style="--i:' + Math.min(i, 12) + '"'; }

// A section head: the h2 as markup, a live count beside it, one line of why.
function head(h2, count, sub) {
  return '<div class="sh">' + h2 +
    (count != null ? '<span class="n' + (count ? "" : " quiet") + '">' + esc(count) + '</span>' : '') +
    (sub ? '<span class="sub">' + sub + '</span>' : '') + '</div>';
}
function demoStrip() {
  if (!DEMO) { return ""; }
  return '<div class="strip warn demo"><span class="d"></span><span><b>Sample data.</b> ' +
    'Every figure on this page is invented to show the layout. Nothing here is an account, ' +
    'and nothing can be saved.</span></div>';
}

// --- the two sides -----------------------------------------------------------
//
// The rail lists both sides. The switch in the header jumps between the two
// landing pages, and whichever side a page belongs to is remembered, so a bare
// /console reached from outside — an email, a bookmark, the sign-in return —
// opens where the person last was.

function loadMode() {
  try {
    var m = localStorage.getItem("lamdis.console.mode");
    if (m === "operator" || m === "buyer") { return m; }
  } catch (e) {}
  return "";
}
function rememberMode(m) {
  try { localStorage.setItem("lamdis.console.mode", m); } catch (e) {}
}
function setMode(m) {
  rememberMode(m);
  location.href = (m === "buyer" ? "/console/buy" : "/console") + location.search;
}
function wireMode() {
  document.getElementById("mode-operator").addEventListener("click", function () { setMode("operator"); });
  document.getElementById("mode-buyer").addEventListener("click", function () { setMode("buyer"); });
}
// The anchors the one-page console had, and the pages they became. Links from
// mail, the docs and older pages still name them.
var ANCHORS = {
  "flight": "/console/work", "sec-operator": "/console",
  "earnings": "/console/earnings", "payout": "/console/earnings", "statement": "/console/earnings",
  "capacity": "/console/capacity", "standing": "/console/capacity",
  "supplier": "/console/business", "alerts": "/console/alerts", "alerts-buyer": "/console/alerts",
  "integration": "/console/keys", "keys": "/console/keys", "larger": "/console/larger",
  "sec-buyer": "/console/buy", "post": "/console/buy",
  "spending": "/console/spending", "funds": "/console/funds"
};
function hashRedirect() {
  var id = location.hash ? location.hash.slice(1) : "";
  var to = id && ANCHORS[id];
  if (!to || to === location.pathname) { return false; }
  location.replace(to + location.search);
  return true;
}
// Only the bare overview bounces, and only when this tab arrived from outside
// the console: once somebody is inside, a rail click on Overview means it.
function sideRedirect() {
  if (PAGE !== "") { return false; }
  var here = false;
  try { here = sessionStorage.getItem("lamdis.console.here") === "1"; } catch (e) {}
  if (here || loadMode() !== "buyer") { return false; }
  location.replace("/console/buy" + location.search);
  return true;
}

// --- masthead ----------------------------------------------------------------

function renderHealth() {
  var el = document.getElementById("h-text");
  var b = document.getElementById("h-beacon");
  var a = document.getElementById("h-auth");
  if (signedIn()) {
    el.textContent = "Signed in";
    b.classList.remove("off");
    a.textContent = "Sign out";
    a.href = "#";
    a.onclick = function (e) { e.preventDefault(); clearSession(); location.href = "/board"; };
  } else {
    el.textContent = "Not signed in";
    b.classList.add("off");
    a.textContent = "Sign in";
    a.href = "/signin?next=" + encodeURIComponent(location.pathname);
    a.onclick = null;
  }
}
// The masthead's reading: how much is open and what it adds up to. Filled
// from a board the page already fetched, or from one public read.
function fillLive(board) {
  var n = document.getElementById("live-n"), s = document.getElementById("live-sum");
  var draw = function (b) {
    var work = (b && b.work) || [];
    if (n) { n.textContent = String(work.length); }
    if (s) { s.textContent = money(work.reduce(function (a, w) { return a + (w.pay_minor || 0); }, 0), "USD"); }
    if (!b) { document.getElementById("live-beacon").classList.add("off"); }
  };
  if (board) { draw(board); return; }
  fetch("/v1/board").then(function (r) { return r.ok ? r.json() : null; })
    .then(draw).catch(function () { draw(null); });
}
// The pills: signed in or not, and whether money can reach this person.
function renderPills() {
  var s = document.getElementById("pill-session"), st = document.getElementById("pill-session-t");
  var p = document.getElementById("pill-payout"), pt = document.getElementById("pill-payout-t");
  if (DEMO) {
    s.className = "pill warn"; s.firstChild.className = "beacon warn"; st.textContent = "Sample data";
  } else if (ME) {
    s.className = "pill ok"; s.firstChild.className = "beacon";
    st.textContent = ME.verified ? "Signed in · verified" : "Signed in";
  }
  if (!ME || SIDE === "buyer") { p.hidden = true; return; }
  var po = (PAYOUT && PAYOUT.payout) || ME.payout || {};
  if (po.unavailable) { p.className = "pill"; p.firstChild.className = "beacon off"; pt.textContent = "Payouts off"; }
  else if (po.ready) { p.className = "pill ok"; p.firstChild.className = "beacon"; pt.textContent = "Payouts connected"; }
  else if (po.connected) { p.className = "pill warn"; p.firstChild.className = "beacon warn"; pt.textContent = "Payouts checking"; }
  else { p.className = "pill warn"; p.firstChild.className = "beacon warn"; pt.textContent = "Payouts not set up"; }
  p.hidden = false;
}

// --- boot --------------------------------------------------------------------
//
// paths are the routes this page needs beyond /v1/me, which every page reads:
// it is the sign-in check, and it carries the pills. show() is handed the
// parsed bodies in the same order, or null when drawing from sample data.

function boot(paths, show) {
  if (hashRedirect() || sideRedirect()) { return; }
  try { sessionStorage.setItem("lamdis.console.here", "1"); } catch (e) {}
  rememberMode(SIDE);
  wireMode();
  if (DEMO_ALLOWED && new URLSearchParams(location.search).get("demo") === "1") {
    DEMO = true;
    workerHeaders = function () { return Promise.reject(new Error("Sample data — nothing is saved.")); };
    document.getElementById("h-text").textContent = "Sample data";
    demoData();
    RV = 0; show(null); renderPills(); fillLive(BOARD);
    return;
  }
  LOADER = function () {
    Promise.all([
      workerHeaders("GET", "/v1/me").then(function (h) { return fetch("/v1/me", {headers: h}); })
    ].concat(paths.map(softGet))).then(function (rs) {
      if (handleAuthFailure(rs[0].status)) { return null; }
      return Promise.all(rs.map(function (r) {
        return r.ok ? r.json().catch(function () { return null; }) : null;
      }));
    }).then(function (out) {
      if (!out) { return; }
      ME = out[0];
      RV = 0; show(out.slice(1)); renderPills();
    }).catch(function () {
      bodyEl().innerHTML = '<div class="empty">Could not reach the exchange.</div>';
    });
  };
  session().then(function () {
    if (!signedIn()) { goSignIn(); return; }
    renderHealth();
    if (paths.indexOf("/v1/board") < 0) { fillLive(null); }
    LOADER();
  });
}
function reload() { if (LOADER) { LOADER(); } }

// --- sample data -------------------------------------------------------------
//
// Only reachable through ?demo=1 on an exchange with no identity provider,
// which the server decides. Every figure is invented and the page says so at
// the top; nothing can be saved because every request is refused before it
// leaves the browser. One set, so the pages agree with each other.

function demoData() {
  var now = Date.now(), h = 3600000;
  var iso = function (t) { return new Date(t).toISOString(); };
  ME = {worker: "cognito:sample-operator", verified: true, enrolled: true, currency: "USD",
    earned_minor: 184500, paid_minor: 121000, pending_minor: 63500, held_minor: 12000, clear_minor: 41500,
    ceiling_minor: 30000, exposure_minor: 9000, room_minor: 21000, tier: "proven", payout_threshold: 2000,
    blocked: "", can_connect_payout: false, payout: {connected: true, ready: true}, tax: null,
    history: [
      {job: "j-3c1d", kind: "do", title: "Replace the porch light and photograph it lit", at: iso(now - 26 * h), status: "accepted", amount_minor: 12000, currency: "USD"},
      {job: "j-88e2", kind: "observe", title: "Is the loading dock gate locked after 8pm", at: iso(now - 50 * h), status: "paid", amount_minor: 2300, currency: "USD"},
      {job: "j-4a90", kind: "do", title: "Move the bins to the kerb before 7am", at: iso(now - 74 * h), status: "paid", amount_minor: 1800, currency: "USD"},
      {job: "j-19bf", kind: "observe", title: "Photograph the storefront with the new sign", at: iso(now - 120 * h), status: "rejected", amount_minor: 0, currency: "USD", why: "the house number was not legible"}
    ],
    bids: [{job: "b-77", title: "Pressure-wash the driveway and both walks", amount_minor: 18500, currency: "USD", status: "Sealed. Offers close tomorrow; the buyer has not chosen.", won: false}]};
  CAP = {capacity: {max_concurrent: 2, range_miles: 18, accepting: true, auto_accept: false,
    lat_e7: 423314000, lon_e7: -830458000, kinds: [], skills: []},
    ceiling: 4, standing: {completed: 7, abandoned: 0, allowance: 4}};
  HOLDING = [
    {job: "j-7f3a", kind: "do", title: "Clear both gutter runs on the north side", where: "Dearborn, MI",
      expires: iso(now + 2 * h), resume: "/board#holding", pay_minor: 9000, currency: "USD", next_stage: 1,
      stages: [{name: "Ladder up, before photos", pay_minor: 2000}, {name: "Clear both runs", pay_minor: 5000}, {name: "After photos, downpipe", pay_minor: 2000}],
      stage_done: [true, false, false]},
    {job: "j-a1c4", kind: "observe", title: "Is the pop-up still trading at the corner of Michigan and Schaefer", where: "Dearborn, MI",
      expires: iso(now + 0.75 * h), resume: "/board#holding", pay_minor: 2500, currency: "USD", next_stage: -1},
    {job: "j-c2e8", kind: "do", title: "Replace the battery in the lockbox at unit 9", where: "Detroit, MI",
      expires: iso(now + 5 * h), resume: "/board#holding", pay_minor: 3500, currency: "USD", next_stage: -1, blocked_by: ["j-7f3a"],
      project: {position: 2, jobs: 3}},
    {job: "j-d904", kind: "observe", title: "Count the cars in the lot at noon", where: "Dearborn, MI",
      expires: iso(now + 3 * h), resume: "/board#holding", pay_minor: 2000, currency: "USD", next_stage: -1}
  ];
  BOARD = {personalized: true, reviews_waiting: 2, work: [
    {job: "w-1", kind: "observe", title: "Is the sign still up on the corner", distance_miles: 3.2, pay_minor: 2300, currency: "USD"},
    {job: "w-2", kind: "do", title: "Deliver the keys to the tenant at unit 4", distance_miles: 6.8, pay_minor: 4000, currency: "USD"},
    {job: "w-3", kind: "do", title: "Photograph the meter and send the reading", distance_miles: 1.4, pay_minor: 1500, currency: "USD"},
    {job: "w-4", kind: "observe", title: "Count the cars in the lot at noon", distance_miles: 11.9, pay_minor: 2000, currency: "USD"},
    {job: "w-5", kind: "do", title: "Take the parcel from the porch inside", distance_miles: 9.1, pay_minor: 2200, currency: "USD"},
    {job: "w-6", kind: "observe", title: "Is the crane still on the Fort Street site", distance_miles: 15.5, pay_minor: 3100, currency: "USD"},
    {job: "w-7", kind: "do", title: "Replace the battery in the lockbox", distance_miles: 4.6, pay_minor: 3500, currency: "USD"}
  ]};
  PAYOUT = {payout: {connected: true, ready: true}, owed_minor: 63500, clear_minor: 41500, threshold_minor: 2000, currency: "USD",
    waiting: [
      {job: "j-3c1d", amount_minor: 12000, status: "waiting out the buyer's review window", clears: iso(now + 31 * h)},
      {job: "j-0e77", amount_minor: 10000, status: "the buyer has raised a problem", reason: "the downpipe is still blocked"}
    ]};
  SPEND = {currency: "USD", balance_minor: 92000, held_minor: 31500, committed_minor: 143800, awaiting_review: 1, jobs: [
    {job: "s-51", kind: "do", title: "Fit the new house number plate by the door", posted: iso(now - 4 * h), where: "Detroit, MI",
      committed_minor: 6500, status: "checking what came back", submissions: 1, worker: "op-4c2e", worker_completed: 12,
      review: {awaiting_release_minor: 6500, hours_left: 18}, evidence: "/v1/jobs/s-51/evidence", receipt: "/v1/jobs/s-51/receipt"},
    {job: "s-48", kind: "observe", title: "Is the pharmacy on Woodward open on Sunday", posted: iso(now - 30 * h), where: "Detroit, MI",
      committed_minor: 2500, status: "somebody is working on it", submissions: 0, worker: "op-91aa", worker_completed: 3, agent: "dispatch bot"},
    {job: "s-44", kind: "do", title: "Pressure-wash the driveway and both walks", posted: iso(now - 40 * h), where: "Dearborn, MI",
      committed_minor: 22500, status: "collecting offers", submissions: 0},
    {job: "s-39", kind: "do", title: "Replace the porch light and photograph it lit", posted: iso(now - 20 * h), where: "Detroit, MI",
      committed_minor: 12000, status: "done", submissions: 1, accepted: 1, worker: "op-4c2e", worker_completed: 12,
      evidence: "/v1/jobs/s-39/evidence", receipt: "/v1/jobs/s-39/receipt"},
    {job: "s-31", kind: "observe", title: "Photograph the frontage at 3 Marlow", posted: iso(now - 200 * h), where: "Dearborn, MI",
      committed_minor: 2300, status: "nobody took it — refunding", submissions: 0}
  ]};
  KEYS = [{id: "k1", label: "dispatch bot", last4: "a8f1", last_used: iso(now - 5 * h), max_per_job_minor: 10000},
          {id: "k0", label: "first try", last4: "0c3d", revoked: true}];
  SKILLS = []; SUP = null; SCOPE = null; ALERTS = {alerts_on: true};
  STMT = {from: new Date().toISOString().slice(0, 7) + "-01", totals: {jobs: 2, gross_minor: 14300, fee_minor: 1430, expense_minor: 0, net_minor: 12870, currency: "USD"},
    lines: [
      {done: iso(now - 26 * h), job: "j-3c1d", title: "Replace the porch light and photograph it lit", by: "you", gross_minor: 12000, fee_minor: 1200, net_minor: 10800},
      {done: iso(now - 50 * h), job: "j-88e2", title: "Is the loading dock gate locked after 8pm", by: "you", gross_minor: 2300, fee_minor: 230, net_minor: 2070}
    ]};
}
`

// consoleTilesJS draws HUD tiles and counts them up. Included with panelJS by
// the pages that have numbers worth a tile.
const consoleTilesJS = `
// A HUD tile: the label, the number (counted up after render), one line under.
function tile(cls, label, target, kind, sub, cur) {
  var v = kind === "money" ? money(0, cur) : "0";
  return '<div' + rv("t " + (cls || "")) + '><div class="k">' + label + '</div>' +
    '<div class="v' + (kind === "money" ? "" : " n") + '" data-target="' + (target || 0) +
    '" data-kind="' + kind + '" data-cur="' + esc(cur || "USD") + '">' + v + '</div>' +
    (sub ? '<div class="s">' + sub + '</div>' : '') + '</div>';
}
function animateTiles() {
  document.querySelectorAll(".hud .v[data-target]").forEach(function (el) {
    var target = parseFloat(el.getAttribute("data-target")) || 0;
    var cur = el.getAttribute("data-cur");
    countUp(el, target, el.getAttribute("data-kind") === "money"
      ? function (x) { return money(x, cur); } : fmt);
    el.removeAttribute("data-target");
  });
}
`

// consoleFlightJS is one held job as a row, shared by the overview's short
// list and the in-flight page's full one.
const consoleFlightJS = `
// stageBar draws the plan as segments weighted by what each pays. Progress,
// not a schedule: what somebody needs is which piece is in front of them and
// what it is worth.
function stageBar(h) {
  if (!h.stages || !h.stages.length) { return ""; }
  var done = h.stage_done || [];
  var segs = h.stages.map(function (st, i) {
    var cls = done[i] ? "paid" : (i === h.next_stage ? "now" : "");
    return '<div class="seg ' + cls + '" style="flex:' + Math.max(1, st.pay_minor || 1) + '"></div>';
  }).join("");
  var keys = h.stages.map(function (st, i) {
    var state = done[i] ? "paid" : (i === h.next_stage ? "now" : "");
    return '<span class="' + state + '"><b>' + esc(st.name) + '</b> ' +
      money(st.pay_minor || 0, h.currency || "USD") + (state ? " " + state : "") + '</span>';
  }).join("");
  return '<div class="stagebar">' + segs + '</div><div class="stagekey">' + keys + '</div>';
}
// flightRow is one held job. acts is extra markup for the action row — the
// in-flight page adds give back; the overview only opens the work.
function flightRow(h, i, acts) {
  var blocked = (h.blocked_by || []).length > 0;
  var mins = Math.max(0, Math.round((new Date(h.expires) - new Date()) / 60000));
  var staged = h.stages && h.stages.length;
  var chip = blocked ? '<span class="chip wait">Waiting on other work</span>'
    : staged ? '<span class="chip ok">Stage ' + (h.next_stage + 1) + ' of ' + h.stages.length + '</span>'
    : '<span class="chip ok">Yours for ' + mins + ' min</span>';
  return '<div' + rvi(i, "r") + ' style="align-items:flex-start;--i:' + Math.min(i, 12) + '"><div class="grow">' +
    '<div class="t">' + chip + '<span class="chip ' + (h.kind === "observe" ? "obs" : "do") + '">' +
      (h.kind === "observe" ? "find out" : h.kind === "review" ? "review" : "make it true") + '</span>' +
      esc(h.title) + '</div>' +
    '<div class="m">' + (h.where ? esc(h.where) + ' &middot; ' : '') + 'lease ends ' + esc(whenAt(h.expires)) +
      (h.project ? ' &middot; piece ' + esc(h.project.position) + ' of ' + esc(h.project.jobs) : '') + '</div>' +
    stageBar(h) +
    '<div class="acts"><a class="btn sm" href="' + esc(h.resume) + '">Open the work</a>' + (acts || "") + '</div>' +
    '</div><span class="amt">' + money(h.pay_minor || 0, h.currency) + '</span></div>';
}
`

// consoleCapJS saves capacity from whichever controls the page has. The
// capacity page has the sliders and switches; the dispatch page has only the
// endpoint. Both must send the whole record, because PUT replaces it, so the
// fields a page does not show are carried over from what was read.
const consoleCapJS = `
var KINDS = [["observe", "Checks"], ["do", "Errands & jobs"], ["review", "Verification"]];
function capacityBody() {
  var c = (CAP && CAP.capacity) || {};
  var b = {
    max_concurrent: c.max_concurrent || 1, range_miles: c.range_miles || 12,
    kinds: c.kinds || [], skills: c.skills || [],
    lat_e7: c.lat_e7 || 0, lon_e7: c.lon_e7 || 0,
    accepting: !!c.accepting, auto_accept: !!c.auto_accept, webhook: c.webhook || ""
  };
  var conc = document.getElementById("c-conc"), range = document.getElementById("c-range");
  if (conc) { b.max_concurrent = parseInt(conc.value, 10); }
  if (range) { b.range_miles = parseInt(range.value, 10); }
  if (document.querySelector("[data-kind]")) {
    var kinds = [];
    document.querySelectorAll("[data-kind]").forEach(function (k) {
      if (k.getAttribute("aria-pressed") === "true") { kinds.push(k.dataset.kind); }
    });
    b.kinds = kinds.length === KINDS.length ? [] : kinds;   // all of them means no filter
  }
  if (document.querySelector("[data-skill]")) {
    var skills = [];
    document.querySelectorAll("[data-skill]").forEach(function (k) {
      if (k.getAttribute("aria-pressed") === "true") { skills.push(k.dataset.skill); }
    });
    b.skills = skills;
  }
  var acc = document.getElementById("c-accept"), auto = document.getElementById("c-auto");
  var hook = document.getElementById("c-hook");
  if (acc) { b.accepting = acc.getAttribute("aria-pressed") === "true"; }
  if (auto) { b.auto_accept = auto.getAttribute("aria-pressed") === "true"; }
  if (hook) { b.webhook = hook.value.trim(); }
  return b;
}
function saveCapacity(after) {
  var err = document.getElementById("cap-err");
  api("PUT", "/v1/capacity", capacityBody()).then(function (res) {
    if (!res.ok) { throw new Error(errorOf(res, "could not save that")); }
    CAP = {capacity: res.body.capacity, standing: CAP && CAP.standing, ceiling: res.body.ceiling};
    if (err) { err.className = "err ok"; err.textContent = res.body.note || "Saved."; }
    if (after) { after(); }
  }).catch(function (e) {
    if (err) { err.className = "err"; err.textContent = e.message; }
  });
}
`

// consoleBuyerHudJS is the buyer's five numbers, shown over the post form,
// the spending list and the funds page so the balance is never a click away.
const consoleBuyerHudJS = `
function thisMonth(iso) {
  var d = new Date(iso), n = new Date();
  return !isNaN(d) && d.getFullYear() === n.getFullYear() && d.getMonth() === n.getMonth();
}
function buyerHud() {
  var sp = SPEND || {}, cur = sp.currency || "USD", jobs = sp.jobs || [];
  var open = 0, spentMonth = 0, release = 0;
  jobs.forEach(function (j) {
    var done = j.status === "done", back = String(j.status).indexOf("refunding") > -1;
    if (!done && !back) { open++; }
    if (done && thisMonth(j.posted)) { spentMonth += j.committed_minor || 0; }
    if (j.review && j.review.awaiting_release_minor) { release += j.review.awaiting_release_minor; }
  });
  var need = sp.awaiting_review || 0;
  return '<div class="hud" style="--cols:5">' +
    tile("money", "Balance", sp.balance_minor || 0, "money", '<a href="/console/funds">add funds</a>', cur) +
    tile("", "Committed to open jobs", sp.held_minor || 0, "money", "back if nobody takes them", cur) +
    tile("", "Spent this month", spentMonth, "money", "on jobs posted this month, done", cur) +
    tile("soft", "Jobs open", open, "n", jobs.length ? "of " + jobs.length + " ever posted" : "nothing posted yet") +
    tile(need ? "wait" : "", "Awaiting your release", need, "n", need
      ? money(release, cur) + ' goes out unless you object &middot; <a href="/console/spending">review</a>'
      : "nothing waiting on you") +
  '</div>';
}
`

// --- operator pages ----------------------------------------------------------

// consoleOverviewJS: the five numbers that decide what to do next, the radar,
// and the first three things in flight.
const consoleOverviewJS = `
// The most useful sentence on the page is why the money has not arrived.
function blockedStrip() {
  if (!ME.blocked) { return ""; }
  return '<div' + rv("strip warn") + '><span class="d"></span><span>' +
    esc(ME.blocked) + '</span>' +
    (ME.can_connect_payout ? '<a href="/console/earnings" style="margin-left:auto">Payouts &rarr;</a>' : "") +
    '</div>';
}

// Five numbers: what is about to be paid, what is stuck and until when, how
// much more work this account may carry, what it is carrying, and whether
// there is verification to do.
function operatorHud() {
  var cur = ME.currency, p = PAYOUT || {};
  var clear = p.clear_minor != null ? p.clear_minor : (ME.clear_minor || 0);
  var held = ME.held_minor || 0;
  var earliest = null;
  (p.waiting || []).forEach(function (h) {
    if (h.clears && (!earliest || new Date(h.clears) < new Date(earliest))) { earliest = h.clears; }
  });
  if (!held && (p.waiting || []).length) {
    held = p.waiting.reduce(function (a, h) { return a + (h.amount_minor || 0); }, 0);
  }
  var threshold = p.threshold_minor || ME.payout_threshold || 0;
  var clearSub = threshold && clear < threshold
    ? "sent at " + money(threshold, cur)
    : (clear > 0 ? "on the next run" : "past the review window");
  var heldSub = held
    ? (earliest ? "first clears " + esc(whenAt(earliest)) : "a buyer has raised a problem")
    : "nothing waiting";
  var room = ME.room_minor || 0;
  var roomSub = (ME.tier ? esc(ME.tier) + " · " : "") + "ceiling " + money(ME.ceiling_minor || 0, cur);
  var allowance = (CAP && CAP.ceiling) || 1;
  var reviews = (BOARD && BOARD.reviews_waiting) || 0;
  return '<div class="hud" style="--cols:5">' +
    tile("money", "Clear to send", clear, "money", clearSub + ' &middot; <a href="/console/earnings">earnings</a>', cur) +
    tile("wait", "Held", held, "money", heldSub, cur) +
    tile(room > 0 ? "ok" : "wait", "Room left", room, "money", roomSub, cur) +
    tile("soft", "Jobs in flight", HOLDING.length, "n", "of " + allowance + " you may hold") +
    tile("", "Reviews waiting", reviews, "n", reviews ? '<a href="/board">open a panel</a>' : "none open to you") +
  '</div>';
}

// Where the work is, from where the operator stands. Holdings first; failing
// those, the open board within range. The board never publishes a job's
// coordinates — a street address to seven decimal places is a street address —
// but it does say how far each is from the reader, so every dot sits on a ring
// at its true distance, on a bearing this page picks and says it picked.
function radarPanel() {
  return '<div' + rv("radar-wrap") + ' id="radar-wrap">' +
    '<canvas id="radar" aria-label="Work around you"></canvas>' +
    '<div class="cap"><span class="beacon"></span>Your range</div>' +
    '<div class="legend"><span class="you"><i></i>You</span><span class="obs"><i></i>Find out</span><span class="do"><i></i>Make it true</span></div>' +
    '<div class="hint" id="radar-hint"></div>' +
  '</div>';
}
// bearingOf spreads ids that differ by one character across the whole
// circle; a plain polynomial hash put "w-1" and "w-2" a degree apart.
function bearingOf(id) {
  var h = 0x811C9DC5, s = String(id || "");
  for (var i = 0; i < s.length; i++) { h = Math.imul(h ^ s.charCodeAt(i), 0x01000193) >>> 0; }
  h ^= h >>> 13; h = Math.imul(h, 0x5BD1E995) >>> 0; h ^= h >>> 15;
  return (h % 3600) / 10 * Math.PI / 180;
}
function placed(you, d, id) {
  var a = bearingOf(id), ky = 69.17, kx = 69.17 * Math.cos(you.lat * Math.PI / 180) || 1;
  return {lat: you.lat + d * Math.cos(a) / ky, lon: you.lon + d * Math.sin(a) / kx};
}
var RADAR_STOP = null;
function drawConsoleRadar() {
  var canvas = document.getElementById("radar"), hint = document.getElementById("radar-hint");
  if (!canvas || !hint) { return; }
  if (RADAR_STOP) { RADAR_STOP(); RADAR_STOP = null; }
  var c = (CAP && CAP.capacity) || {};
  var you = c.lat_e7 ? {lat: c.lat_e7 / 1e7, lon: c.lon_e7 / 1e7} : null;
  var range = parseInt(c.range_miles || 12, 10) || 12;
  var jobs = [], source = "";
  var mine = HOLDING.filter(function (h) { return h.lat_e7 || h.lat; });
  if (mine.length && you) {
    source = "your work";
    jobs = mine.map(function (h) {
      return {lat: h.lat || h.lat_e7 / 1e7, lon: h.lon || h.lon_e7 / 1e7, kind: h.kind, title: h.title,
        job: h.job, pay: money(h.pay_minor || 0, h.currency), resume: h.resume};
    });
  } else if (you && BOARD && (BOARD.work || []).length) {
    source = "the board";
    jobs = BOARD.work.filter(function (w) { return isFinite(w.distance_miles) && w.distance_miles > 0 && w.distance_miles <= range; })
      .map(function (w) {
        var p = placed(you, w.distance_miles, w.job);
        return {lat: p.lat, lon: p.lon, kind: w.kind, title: w.title, job: w.job,
          pay: w.pay_minor ? money(w.pay_minor, w.currency) : "", miles: w.distance_miles};
      });
  }
  if (!you) {
    hint.innerHTML = 'No location set. <a href="/console/capacity">Set where you work from</a> and the board is drawn around you.';
  } else if (source === "the board") {
    hint.innerHTML = jobs.length + ' open job' + (jobs.length === 1 ? '' : 's') + ' within ' + range +
      ' mi &middot; distance is real, bearing is not published &middot; <a href="/console/capacity">change range</a>';
  } else if (source === "your work") {
    hint.innerHTML = jobs.length + ' job' + (jobs.length === 1 ? '' : 's') + ' you hold.';
  } else {
    hint.innerHTML = 'Nothing on the board inside ' + range + ' mi of you right now.';
  }
  RADAR_STOP = drawRadar(canvas, {
    jobs: jobs, you: you, rangeMiles: range,
    empty: you ? "nothing within range" : "no location set",
    onPick: function (j) {
      location.href = j.resume ? j.resume : "/j/" + encodeURIComponent(j.job);
    }
  });
}

// The first three things in flight, and the way to the rest.
function flightPreview() {
  var rows = HOLDING.slice(0, 3).map(function (h, i) { return flightRow(h, i, ""); }).join("");
  return head('<h2 id="flight">In flight</h2>', HOLDING.length, "what you hold right now") +
    (rows ? '<div class="rows">' + rows + '</div>'
          : '<div' + rv("rows") + '><div class="empty">Nothing on at the moment. <a href="/board">Open work is on the board.</a></div></div>') +
    (HOLDING.length > 3
      ? '<p class="more"><a href="/console/work">All ' + HOLDING.length + ' in flight, with stages and give back &rarr;</a></p>'
      : (HOLDING.length ? '<p class="more"><a href="/console/work">Stages and give back &rarr;</a></p>' : ''));
}

function show(out) {
  if (out) {
    CAP = out[0] || {capacity: {max_concurrent: 1, range_miles: 12, accepting: true}, ceiling: 1};
    PAYOUT = out[1] || null;
    HOLDING = (out[2] && out[2].holding) || [];
    BOARD = out[3] || null;
    fillLive(BOARD || {});
  }
  bodyEl().innerHTML = demoStrip() + blockedStrip() + operatorHud() + radarPanel() + flightPreview();
  animateTiles();
  drawConsoleRadar();
}
boot(["/v1/capacity", "/v1/payout", "/v1/workers/holdings", "/v1/board"], show);
`

// consoleWorkJS: everything held, with stages, and the way to hand one back.
const consoleWorkJS = `
function giveBack(btn, job) {
  var err = document.getElementById("g-" + job);
  btn.disabled = true; btn.textContent = "Giving back…"; err.textContent = "";
  api("POST", "/v1/workers/giveback/" + encodeURIComponent(job)).then(function (res) {
    if (!res.ok) { throw new Error(errorOf(res, "could not give that back")); }
    err.className = "err ok"; err.textContent = res.body.note || "Back on the board.";
    reload();
  }).catch(function (e) {
    err.className = "err"; err.textContent = e.message;
    btn.disabled = false; btn.textContent = "Give it back";
  });
}
function inFlight() {
  var rows = HOLDING.map(function (h, i) {
    return flightRow(h, i,
      '<button class="btn sm" data-give="' + esc(h.job) + '">Give it back</button>') +
      '<div class="err" id="g-' + esc(h.job) + '" style="padding:0 .95rem"></div>';
  }).join("");
  return head('<h2 id="flight">In flight</h2>', HOLDING.length, "what you hold right now, and what each stage pays") +
    (rows ? '<div class="rows">' + rows + '</div>'
          : '<div' + rv("rows") + '><div class="empty">Nothing on at the moment. <a href="/board">Open work is on the board.</a></div></div>') +
    '<p class="note">Giving a job back puts it on the board for somebody else with no mark against you. ' +
      'Letting the lease lapse instead counts as abandoned.</p>';
}
function show(out) {
  if (out) { HOLDING = (out[0] && out[0].holding) || []; }
  bodyEl().innerHTML = demoStrip() + inFlight();
  document.querySelectorAll("[data-give]").forEach(function (b) {
    b.addEventListener("click", function () { giveBack(b, b.dataset.give); });
  });
}
boot(["/v1/workers/holdings"], show);
`

// consoleEarningsJS: what was earned, where the money goes and whether it
// can yet, and the statement a bookkeeper reconciles.
const consoleEarningsJS = `
// Returning from the provider proves nothing on its own; the state is re-read
// from them rather than assumed from the redirect, so the query is dropped
// and the page loads as it always does.
if (new URLSearchParams(location.search).get("payout")) {
  history.replaceState(null, "", location.pathname);
}

function blockedStrip() {
  if (!ME.blocked) { return ""; }
  return '<div' + rv("strip warn") + '><span class="d"></span><span>' +
    esc(ME.blocked) + '</span>' +
    (ME.can_connect_payout ? '<a href="#payout" style="margin-left:auto">Payouts &rarr;</a>' : "") +
    '</div>';
}

function earnings() {
  var cur = ME.currency;
  var rows = (ME.history || []).map(function (h, i) {
    var chip = h.status === "accepted" || h.status === "paid" ? "ok"
             : (h.status === "rejected" ? "bad" : "");
    return '<div' + rvi(i, "r") + '><div class="grow">' +
      '<div class="t"><span class="chip ' + chip + '">' + esc(h.status) + '</span>' +
        esc(h.title || h.job) + '</div>' +
      '<div class="m">' + esc(when(h.at)) + (h.why ? ' &middot; ' + esc(h.why) : '') + '</div>' +
      '</div><span class="amt' + (h.amount_minor ? '' : ' none') + '">' +
      (h.amount_minor ? money(h.amount_minor, cur) : "&mdash;") + '</span></div>';
  }).join("");

  var bids = (ME.bids || []).map(function (b, i) {
    return '<div' + rvi(i, "r") + '><div class="grow">' +
      '<div class="t"><span class="chip ' + (b.won ? "ok" : "hot") + '">' +
        (b.won ? "Won" : "Bid") + '</span>' + esc(b.title) + '</div>' +
      '<div class="m">' + esc(b.status) + '</div></div>' +
      '<span class="amt">' + money(b.amount_minor, b.currency) + '</span></div>';
  }).join("");

  var tax = ME.tax;
  var taxNote = "";
  if (tax && (tax.reportable || tax.approaching)) {
    taxNote = '<div' + rv("strip") + '><span class="d"></span><span>' +
      (tax.reportable
        ? "You have earned " + money(tax.earned_minor, cur) + " here in " + tax.year +
          ", which is above the " + money(tax.threshold_minor, cur) +
          " US reporting threshold. The payment provider will have collected " +
          "your tax details during setup."
        : "You are approaching the " + money(tax.threshold_minor, cur) +
          " US reporting threshold for " + tax.year + " (" +
          money(tax.earned_minor, cur) + " so far). Finishing payout setup now " +
          "means nothing is held up later.") +
      '</span></div>';
  }

  return head('<h2 id="earnings">Earnings</h2>', null,
      money(ME.earned_minor, cur) + " earned &middot; " + money(ME.paid_minor, cur) + " paid out &middot; " +
      money(ME.pending_minor, cur) + " waiting") +
    blockedStrip() + taxNote +
    (ME.pending_minor > 0
      ? '<div' + rv("pane") + ' style="margin-bottom:.7rem"><div class="ctl" style="margin:0">' +
          '<button class="btn" id="cash-now">Send what I am owed now</button>' +
          '<span class="why" style="margin:0;font-size:.8rem;color:var(--ink-3)">Below the threshold the ' +
            'provider’s transfer fee comes out of it — your call, not ours.' +
          '</span></div><div class="err" id="cash-err"></div></div>'
      : "") +
    (bids ? '<h2 style="margin-top:1rem">Offers you have out</h2><div class="rows">' + bids + '</div>' : "") +
    '<h2 style="margin-top:1rem">What you have done</h2>' +
    (rows ? '<div class="rows">' + rows + '</div>'
          : '<div' + rv("rows") + '><div class="empty">Nothing yet. <a href="/board">Find work.</a></div></div>');
}

// Where the money goes, and whether it can yet. Read from /v1/payout rather
// than inferred from the earnings figures, because "connected" and "able to
// receive money" differ for days while the provider runs its checks.
function payoutPanel() {
  var p = PAYOUT || {};
  var st = p.payout || ME.payout || {};
  var cur = p.currency || ME.currency || "USD";
  var threshold = p.threshold_minor || ME.payout_threshold || 2000;
  var owed = p.owed_minor || ME.pending_minor || 0;
  var clear = p.clear_minor != null ? p.clear_minor : ME.clear_minor;
  var state, chip;
  if (st.unavailable) {
    state = "Payouts are not switched on for this exchange yet. Earnings are recorded " +
      "and will be sent once they are."; chip = "";
  } else if (!st.connected) {
    state = "No payout account yet. Setting one up takes about two minutes on the " +
      "payment provider’s own pages; we never see your bank details."; chip = "warn";
  } else if (!st.ready) {
    state = "Account created. The provider is still checking it" +
      ((st.needs || []).length ? " and needs " + esc(st.needs.join(", ")) : "") + "."; chip = "warn";
  } else {
    state = "Connected and able to receive money."; chip = "ok";
  }
  var next;
  if (st.unavailable || !st.ready) { next = "once the account above can receive money"; }
  else if ((clear || 0) >= threshold) { next = "on the next automatic run"; }
  else { next = "when what is clear to send reaches " + money(threshold, cur); }

  var waiting = (p.waiting || []).map(function (h, i) {
    return '<div' + rvi(i, "r") + '><div class="grow"><div class="t">' + esc(h.job) + '</div>' +
      '<div class="m">' + esc(h.status) +
      (h.clears ? ' &middot; clears ' + esc(whenAt(h.clears)) : '') +
      (h.reason ? ' &middot; ' + esc(h.reason) : '') + '</div></div>' +
      '<span class="amt">' + money(h.amount_minor, cur) + '</span></div>';
  }).join("");

  return head('<h2 id="payout">Payouts</h2>', null, "next payout " + next) +
    '<div' + rv("strip" + (chip === "warn" ? " warn" : "")) + '><span class="d"></span>' +
      '<span><span class="chip ' + chip + '">' +
        (st.unavailable ? "off" : st.ready ? "connected" : st.connected ? "checking" : "not set up") +
      '</span>' + state + '</span>' +
      (ME.can_connect_payout
        ? '<button class="btn sm go" id="pay-connect" style="margin-left:auto">' +
          (st.connected ? "Finish setup" : "Set up payouts") + '</button>'
        : "") +
    '</div><div class="err" id="pay-err"></div>' +
    '<div class="hud" style="--cols:3">' +
      tile("money", "Owed to you", owed, "money", "credited, not yet sent", cur) +
      tile("money", "Clear to send", clear || 0, "money", "past the buyer’s review window", cur) +
      tile("", "Threshold", threshold, "money", "sent once reached", cur) +
    '</div>' +
    (waiting ? '<h2 style="margin-top:.2rem">Still waiting</h2><div class="rows">' + waiting + '</div>' : "");
}

// Opening the rail's hosted onboarding.
//
// The link is minted per click rather than rendered into the page: it is
// single-use at the provider, so a stale one in the HTML fails with no
// explanation at the worst possible moment.
function connectPayout(btn) {
  var host = document.getElementById("pay-err");
  btn.disabled = true;
  btn.textContent = "Setting up…";

  // A staged panel rather than a spinner. This waits on the payment provider,
  // which takes seconds rather than milliseconds. A bar that pretends to know
  // how far along it is would be inventing progress it cannot see; naming the
  // step actually in flight, and showing real elapsed time, is true.
  host.innerHTML =
    '<div class="setup" role="status" aria-live="polite">' +
      '<div class="bar"><span></span></div>' +
      '<div class="setup-step" id="pay-step">Opening your account with Stripe</div>' +
      '<div class="setup-note" id="pay-note">' +
        'Stripe holds your bank details, not us. You will finish on their pages.' +
      '</div>' +
      '<div class="setup-clock" id="pay-clock">0s</div>' +
    '</div>';

  var t0 = Date.now();
  var tick = setInterval(function () {
    var s = Math.round((Date.now() - t0) / 1000);
    var clock = document.getElementById("pay-clock");
    if (clock) { clock.textContent = s + "s"; }
    var step = document.getElementById("pay-step");
    var note = document.getElementById("pay-note");
    if (!step) { return; }
    // Only claims that stay true however long it takes.
    if (s >= 20) {
      step.textContent = "Still waiting on Stripe";
      note.textContent = "Longer than usual. Nothing is lost — leaving this " +
        "page and trying again later is safe.";
    } else if (s >= 8) {
      step.textContent = "Preparing your verification link";
      note.textContent = "Almost there.";
    }
  }, 250);
  var stop = function () { clearInterval(tick); };

  api("POST", "/v1/payout/connect").then(function (res) {
    if (!res.ok || !res.body.url) {
      throw new Error(errorOf(res, "could not start setup"));
    }
    stop();
    var step = document.getElementById("pay-step");
    if (step) { step.textContent = "Taking you to Stripe…"; }
    location.href = res.body.url;
  }).catch(function (e) {
    stop();
    host.innerHTML = '<div class="err">' + esc(e.message) + '</div>';
    btn.disabled = false;
    btn.textContent = "Set up payouts";
  });
}

// --- statement ---------------------------------------------------------------
//
// The record a business gives its bookkeeper. Fetched with the session, so it
// is rendered here rather than linked: a bare link to /v1/statement carries no
// credential and answers 401.

function monthOf(iso) { return String(iso || "").slice(0, 7); }

function statement() {
  var st = STMT || {}, lines = st.lines || [], t = st.totals || {};
  var cur = t.currency || "USD";
  var month = monthOf(st.from) || new Date().toISOString().slice(0, 7);
  var rows = lines.map(function (l) {
    return '<tr><td>' + esc(when(l.done)) + '</td>' +
      '<td>' + esc(l.title) + '<div class="m" style="font:.7rem var(--mono);color:var(--ink-3)">' +
        esc(l.job) + (l.reference ? ' &middot; ' + esc(l.reference) : '') +
        (l.site ? ' &middot; ' + esc(l.site) : '') + '</div></td>' +
      '<td>' + esc(l.by || "") + '</td>' +
      '<td class="n">' + money(l.gross_minor, cur) + '</td>' +
      '<td class="n">' + money(-l.fee_minor, cur) + '</td>' +
      '<td class="n">' + (l.expense_minor ? money(l.expense_minor, cur) : '&mdash;') + '</td>' +
      '<td class="n">' + money(l.net_minor, cur) + '</td></tr>';
  }).join("");
  var q = "?from=" + esc(st.from || "") + "&to=" + esc(st.to || "");
  return head('<h2 id="statement">Statement</h2>', lines.length ? lines.length + " lines" : null,
      "job by job, in a form a bookkeeper can reconcile against the bank") +
  '<div' + rv("pane") + ' style="margin-bottom:.7rem"><div class="ctl" style="gap:.6rem;flex-wrap:wrap;margin:0">' +
    '<input type="month" id="stmt-month" value="' + esc(month) + '" aria-label="Month" class="amt-in" style="width:auto">' +
    '<button class="btn sm" id="stmt-csv">Download CSV</button>' +
    '<button class="btn sm" id="stmt-json">Show JSON</button>' +
    '<span class="why" style="margin:0;font-size:.72rem;color:var(--ink-3);font-family:var(--mono)">' +
      'GET /v1/statement' + q + ' &middot; GET /v1/statement.csv' + q + ', signed with your session</span>' +
  '</div></div>' +
  (st.reconciles === false
    ? '<div' + rv("strip warn") + '><span class="d"></span><span>' + esc(st.reconciliation_note) + '</span></div>'
    : '') +
  (lines.length
    ? '<div' + rv("tablewrap") + '><table class="stmt"><thead><tr>' +
        '<th>Date</th><th>Job</th><th>By</th><th class="n">Gross</th><th class="n">Fee</th>' +
        '<th class="n">Expenses</th><th class="n">Net</th></tr></thead><tbody>' + rows +
        '<tr class="tot"><td colspan="3">' + esc(t.jobs) + ' job' + (t.jobs === 1 ? '' : 's') + '</td>' +
        '<td class="n">' + money(t.gross_minor, cur) + '</td>' +
        '<td class="n">' + money(-(t.fee_minor || 0), cur) + '</td>' +
        '<td class="n">' + money(t.expense_minor || 0, cur) + '</td>' +
        '<td class="n">' + money(t.net_minor, cur) + '</td></tr>' +
      '</tbody></table></div>'
    : '<div' + rv("rows") + '><div class="empty">Nothing earned in ' + esc(month) + '.</div></div>') +
  '<div class="err" id="stmt-err"></div>' +
  '<pre class="api" id="stmt-raw" hidden>' + esc(JSON.stringify(st, null, 2)) + '</pre>';
}

function loadStatement(month) {
  var y = parseInt(month.slice(0, 4), 10), m = parseInt(month.slice(5, 7), 10);
  if (!(y > 0 && m > 0)) { return; }
  var last = new Date(y, m, 0).getDate();
  var path = "/v1/statement?from=" + month + "-01&to=" + month + "-" + String(last).padStart(2, "0");
  api("GET", path).then(function (res) {
    if (!res.ok) { throw new Error(errorOf(res, "could not load the statement")); }
    STMT = res.body;
    var host = document.getElementById("statement-wrap");
    if (host) { RV = 0; host.innerHTML = statement(); wireStatement(); }
  }).catch(function (e) {
    var err = document.getElementById("stmt-err");
    if (err) { err.textContent = e.message; }
  });
}

// The browser cannot follow a link with a bearer token on it, so the file is
// fetched with the session and handed to the download manager from here.
function downloadCSV() {
  var path = (STMT && STMT.csv) || "/v1/statement.csv";
  var err = document.getElementById("stmt-err");
  workerHeaders("GET", path).then(function (h) {
    return fetch(path, {headers: h});
  }).then(function (r) {
    if (handleAuthFailure(r.status)) { return null; }
    if (!r.ok) { throw new Error("could not fetch the CSV"); }
    return r.blob();
  }).then(function (b) {
    if (!b) { return; }
    var a = document.createElement("a");
    a.href = URL.createObjectURL(b);
    a.download = "lamdis-" + (monthOf(STMT && STMT.from) || "statement") + ".csv";
    document.body.appendChild(a);
    a.click();
    a.remove();
  }).catch(function (e) { err.textContent = e.message; });
}

function wireStatement() {
  var m = document.getElementById("stmt-month");
  if (m) { m.addEventListener("change", function () { loadStatement(this.value); }); }
  var csv = document.getElementById("stmt-csv");
  if (csv) { csv.addEventListener("click", downloadCSV); }
  var raw = document.getElementById("stmt-json");
  if (raw) {
    raw.addEventListener("click", function () {
      var pre = document.getElementById("stmt-raw");
      pre.hidden = !pre.hidden;
      raw.textContent = pre.hidden ? "Show JSON" : "Hide JSON";
    });
  }
}

function wireEarnings() {
  var cash = document.getElementById("cash-now");
  if (cash) {
    cash.addEventListener("click", function () {
      var btn = this, err = document.getElementById("cash-err");
      btn.disabled = true; btn.textContent = "Sending…"; err.textContent = "";
      api("POST", "/v1/payout/now").then(function (res) {
        if (!res.ok) { throw new Error(errorOf(res, "could not send")); }
        err.className = "err ok";
        err.textContent = res.body.sent
          ? "Sent " + money(res.body.amount_minor, ME.currency) + "."
          : res.body.status;
        reload();
      }).catch(function (e) {
        err.textContent = e.message;
        btn.disabled = false; btn.textContent = "Send what I am owed now";
      });
    });
  }
  var pc = document.getElementById("pay-connect");
  if (pc) { pc.addEventListener("click", function () { connectPayout(this); }); }
  wireStatement();
}

function show(out) {
  if (out) { PAYOUT = out[0] || null; STMT = out[1] || null; }
  bodyEl().innerHTML = demoStrip() + earnings() + payoutPanel() +
    '<div id="statement-wrap">' + statement() + '</div>';
  wireEarnings();
  animateTiles();
}
boot(["/v1/payout", "/v1/statement"], show);
`

// consoleCapacityJS: the limits the exchange dispatches inside.
const consoleCapacityJS = `
function capacity() {
  var c = CAP.capacity, st = CAP.standing || {};
  return head('<h2 id="capacity">Capacity</h2>', null, "the exchange only dispatches inside these limits") +
  '<div class="panes">' +
    '<div' + rv("pane") + '>' +
      '<h3>Concurrency</h3>' +
      '<p class="why">How many jobs you will hold at the same time. You can hold up to ' +
        '<b>' + CAP.ceiling + '</b> right now &mdash; finishing jobs raises it.</p>' +
      '<div class="ctl">' +
        '<input type="range" min="1" max="' + (CAP.ceiling || 1) +
          '" value="' + Math.min(c.max_concurrent, CAP.ceiling || 1) +
          '" id="c-conc" aria-label="Jobs at once">' +
        '<span class="v" id="c-conc-v">' + Math.min(c.max_concurrent, CAP.ceiling || 1) +
          '</span>' +
      '</div>' +
      '<div class="toggle"><div>' +
        '<div class="tx">Taking work</div>' +
        '<div class="sx">Turn off to finish what you hold and stop.</div>' +
      '</div><button class="sw" id="c-accept" aria-pressed="' + !!c.accepting + '" aria-label="Taking work"></button></div>' +
      '<p class="why" style="margin:.8rem 0 0">' +
        (CAP.ceiling >= 12
          ? 'Your ceiling is ' + CAP.ceiling + ', because a reviewer has checked ' +
            'your licences and cover.'
          : 'Your ceiling is ' + (CAP.ceiling || 1) + ' for now. It rises as you ' +
            'finish work, and rises a lot once a reviewer has checked a ' +
            '<a href="/console/business">business profile</a> — a business ' +
            'with crews should not be throttled like a stranger.') +
      '</p>' +
    '</div>' +

    '<div' + rv("pane") + '>' +
      '<h3>Range</h3>' +
      '<p class="why">How far from you a job can be. The <a href="/console">overview radar</a> is drawn to this range.</p>' +
      '<div class="ctl">' +
        '<input type="range" min="1" max="60" value="' + c.range_miles + '" id="c-range" aria-label="Range in miles">' +
        '<span class="v" id="c-range-v">' + c.range_miles + ' mi</span>' +
      '</div>' +
      '<div class="toggle"><div>' +
        '<div class="tx">Where you work from</div>' +
        '<div class="sx" id="c-loc">' +
          (c.lat_e7 ? "Set &mdash; jobs are sorted by distance from here."
                    : "Not set. Until it is, range does nothing and you see every job in the country.") +
        '</div>' +
      '</div><button class="sw2" id="c-locate">' +
        (c.lat_e7 ? "Update" : "Set") + '</button></div>' +
      '<div class="toggle"><div>' +
        '<div class="tx">Auto-accept</div>' +
        '<div class="sx">Take matching work without asking. Needs a <a href="/console/dispatch">dispatch endpoint</a>.</div>' +
      '</div><button class="sw" id="c-auto" aria-pressed="' + !!c.auto_accept + '" aria-label="Auto-accept"></button></div>' +
    '</div>' +

    '<div' + rv("pane") + '>' +
      '<h3>What you take</h3>' +
      '<p class="why">Only these kinds of job reach you.</p>' +
      '<div class="kinds">' + KINDS.map(function (k) {
        var on = !c.kinds || !c.kinds.length || c.kinds.indexOf(k[0]) > -1;
        return '<button class="kind" data-kind="' + k[0] + '" aria-pressed="' + on + '">' + k[1] + '</button>';
      }).join("") + '</div>' +
      '<h3 style="margin-top:1.1rem">What you are qualified for</h3>' +
      '<p class="why">Licensed trades only reach people who claim the credential. ' +
        'Claiming one you do not hold is fraud, and the job carries your name.</p>' +
      '<div class="kinds">' + SKILLS.map(function (k) {
        var on = (c.skills || []).indexOf(k.skill) > -1;
        return '<button class="kind' + (k.licensed ? " lic" : "") + '" data-skill="' + k.skill +
          '" aria-pressed="' + on + '" title="' + esc(k.note || "") + '">' +
          esc(k.label) + '</button>';
      }).join("") + '</div>' +
    '</div>' +

    '<div' + rv("pane") + '>' +
      '<h3>Standing</h3>' +
      '<p class="why">Earned by finishing what you take.</p>' +
      '<div class="hud" style="--cols:3;margin:0">' +
        tile("ok", "Done", st.completed || 0, "n", "settled and paid") +
        tile(st.abandoned ? "wait" : "", "Abandoned", st.abandoned || 0, "n", "taken and let lapse") +
        tile("soft", "At once", st.allowance || 1, "n", "jobs you may hold") +
      '</div>' +
    '</div>' +
  '</div>' +
  '<div class="err" id="cap-err"></div>';
}

function wireCapacity() {
  var conc = document.getElementById("c-conc"), range = document.getElementById("c-range");
  if (conc) {
    conc.addEventListener("input", function () {
      document.getElementById("c-conc-v").textContent = this.value;
    });
    conc.addEventListener("change", function () { saveCapacity(); });
  }
  if (range) {
    range.addEventListener("input", function () {
      document.getElementById("c-range-v").textContent = this.value + " mi";
    });
    range.addEventListener("change", function () { saveCapacity(); });
  }
  document.querySelectorAll(".sw[id^=c-], [data-kind], [data-skill]").forEach(function (el) {
    el.addEventListener("click", function () {
      el.setAttribute("aria-pressed", el.getAttribute("aria-pressed") === "true" ? "false" : "true");
      saveCapacity();
    });
  });
  var locate = document.getElementById("c-locate");
  if (locate) {
    locate.addEventListener("click", function () {
      var out = document.getElementById("c-loc");
      if (!navigator.geolocation) {
        out.textContent = "This browser will not share a location.";
        return;
      }
      out.textContent = "Asking…";
      navigator.geolocation.getCurrentPosition(function (pos) {
        CAP.capacity = CAP.capacity || {};
        CAP.capacity.lat_e7 = Math.round(pos.coords.latitude * 1e7);
        CAP.capacity.lon_e7 = Math.round(pos.coords.longitude * 1e7);
        out.textContent = "Set — jobs are sorted by distance from here.";
        saveCapacity();
      }, function () {
        out.textContent = "Location refused. Range stays off until you allow it.";
      });
    });
  }
}

function show(out) {
  if (out) {
    CAP = out[0] || {capacity: {max_concurrent: 1, range_miles: 12, accepting: true}, ceiling: 1};
    SKILLS = (out[1] && out[1].skills) || [];
  }
  bodyEl().innerHTML = demoStrip() + capacity();
  wireCapacity();
  animateTiles();
}
boot(["/v1/capacity", "/v1/skills"], show);
`

// consoleBusinessJS: a business describing itself — what it is called, what
// it is licensed to do, what cover it carries, and who works for it. Claims,
// until a person at the exchange checks them, which is deliberately not a
// button on this page.
const consoleBusinessJS = `
function licenceRow(l) {
  l = l || {};
  var opts = SKILLS.slice().sort(function (a, b) {
    return (b.licensed ? 1 : 0) - (a.licensed ? 1 : 0);
  }).map(function (k) {
    return '<option value="' + esc(k.skill) + '"' + (k.skill === l.skill ? " selected" : "") + '>' +
      esc(k.label) + (k.licensed ? "" : " (no licence)") + '</option>';
  }).join("");
  return '<div class="lic">' +
    (opts ? '<select class="l-skill" aria-label="Trade">' + opts + '</select>'
          : '<input type="text" class="l-skill" placeholder="trade" value="' + esc(l.skill || "") + '">') +
    '<input type="text" class="l-num" placeholder="Licence number" value="' + esc(l.number || "") + '">' +
    '<input type="text" class="l-state" placeholder="State" maxlength="2" value="' + esc(l.state || "") + '">' +
    '<input type="date" class="l-exp" aria-label="Expires" value="' + esc(dateOf(l.expires)) + '">' +
    '<span>' + (l.verified ? '<span class="chip ok">checked</span>' : '') +
      '<button class="btn sm l-del" type="button" aria-label="Remove licence">&times;</button></span>' +
  '</div>';
}

function supplier() {
  var d = SUP || {}, sup = d.supplier || null, ins = (sup && sup.insurance) || {};
  var lics = (sup && sup.licences || []).map(licenceRow).join("");
  var members = (sup && sup.members || []).map(function (m) {
    return '<div class="member"><span>' + esc(m) + '</span>' +
      '<button class="btn sm" data-member-del="' + esc(m) + '">Remove</button></div>';
  }).join("");
  var attention = (d.attention || []).map(function (a) { return '<li>' + esc(a) + '</li>'; }).join("");
  var mail = "mailto:support@lamdis.ai?subject=" +
    encodeURIComponent("Vetting request: " + ((sup && (sup.legal_name || sup.trading_name)) || ME.worker));

  return head('<h2 id="supplier">Business</h2>', null, sup && sup.vetted ? "vetted" : "if you work as a company, say so here") +
  '<p class="lead">Buyers see the name, the checked licences and the cover; your crews claim ' +
    'against the company’s ceiling rather than each starting as a stranger.</p>' +
  (sup && sup.vetted
    ? '<div' + rv("strip") + '><span class="d"></span><span><span class="chip ok">vetted</span>' +
      'A reviewer has checked this business' + (sup.vetted_at ? ' (' + esc(when(sup.vetted_at)) + ')' : '') +
      '. You can hold up to <b>' + esc(d.ceiling) + '</b> jobs at once.</span></div>'
    : '<div' + rv("note-box") + '><b>Vetting is done by a person, not a button.</b> Fill this in, ' +
      'then <a href="' + mail + '">write to support@lamdis.ai</a> with the job id of ' +
      'something you have finished here. A reviewer checks the licence numbers against ' +
      'the issuing register and the cover with the carrier, and raises your ceiling from ' +
      esc(d.ceiling || 1) + ' toward ' + esc(d.vetted_ceiling || "the vetted maximum") +
      '. Changing a licence or policy afterwards clears its check.</div>') +
  (attention ? '<ul class="attn">' + attention + '</ul>' : '') +
  '<div class="panes" style="margin-top:1rem">' +
    '<div' + rv("pane") + '>' +
      '<h3>Who you are</h3>' +
      '<div class="field"><label for="s-kind">Working as</label>' +
        '<select id="s-kind">' +
          '<option value="individual"' + (!sup || sup.kind !== "company" ? " selected" : "") + '>An individual</option>' +
          '<option value="company"' + (sup && sup.kind === "company" ? " selected" : "") + '>A company</option>' +
        '</select></div>' +
      '<div class="field"><label for="s-legal">Registered name</label>' +
        '<input type="text" id="s-legal" maxlength="120" value="' + esc(sup && sup.legal_name) + '">' +
        '<div class="hint">Required for a company. Passed to the payment provider as-is.</div></div>' +
      '<div class="field"><label for="s-trading">Trading as</label>' +
        '<input type="text" id="s-trading" maxlength="120" value="' + esc(sup && sup.trading_name) + '">' +
        '<div class="hint">Only if it differs. This is what buyers see.</div></div>' +
      '<h3 style="margin-top:.6rem">Insurance</h3>' +
      '<div class="two">' +
        '<div class="field"><label for="s-carrier">Carrier</label>' +
          '<input type="text" id="s-carrier" value="' + esc(ins.carrier) + '"></div>' +
        '<div class="field"><label for="s-policy">Policy number</label>' +
          '<input type="text" id="s-policy" value="' + esc(ins.policy_number) + '"></div>' +
        '<div class="field"><label for="s-cover">Cover, $</label>' +
          '<input type="number" id="s-cover" min="0" step="1000" value="' +
            (ins.coverage_minor ? Math.round(ins.coverage_minor / 100) : "") + '"></div>' +
        '<div class="field"><label for="s-ins-exp">Expires</label>' +
          '<input type="date" id="s-ins-exp" value="' + esc(dateOf(ins.expires)) + '"></div>' +
      '</div>' +
      (ins.carrier ? '<p class="why">' + (ins.verified ? '<span class="chip ok">checked</span>' :
        '<span class="chip">not checked yet</span>') + '</p>' : '') +
    '</div>' +
    '<div' + rv("pane") + '>' +
      '<h3>Licences</h3>' +
      '<p class="why">One row per credential, with the number a reviewer can look up.</p>' +
      '<div id="lic-list">' + lics + '</div>' +
      '<button class="btn sm" id="lic-add" type="button">Add a licence</button>' +
      '<div style="margin-top:1rem"><button class="btn go" id="s-save">Save profile</button></div>' +
      '<div class="err" id="s-err"></div>' +
    '</div>' +
    '<div' + rv("pane") + '>' +
      '<h3>Crew</h3>' +
      '<p class="why">People who take work on the company’s behalf. Their claims count ' +
        'against the company’s ceiling and their earnings are attributed to it.</p>' +
      (members || '<div class="empty" style="padding:.2rem 0 .6rem">Nobody yet.</div>') +
      '<div class="ctl" style="margin-top:.8rem;gap:.5rem">' +
        '<input type="text" id="m-add" placeholder="Their account id, e.g. cognito:&hellip;" style="flex:1">' +
        '<button class="btn sm" id="m-add-btn">Add</button></div>' +
      '<div class="hint" style="font-size:.76rem;color:var(--ink-3)">They can read their ' +
        'account id under <a href="/console/keys">Agent keys</a> in their own console.</div>' +
      '<div class="err" id="m-err"></div>' +
    '</div>' +
  '</div>';
}

function saveSupplier() {
  var btn = document.getElementById("s-save"), err = document.getElementById("s-err");
  var lics = [];
  document.querySelectorAll("#lic-list .lic").forEach(function (row) {
    var skill = row.querySelector(".l-skill").value.trim();
    var num = row.querySelector(".l-num").value.trim();
    if (!skill && !num) { return; }
    var l = {skill: skill, number: num, state: row.querySelector(".l-state").value.trim()};
    var exp = row.querySelector(".l-exp").value;
    if (exp) { l.expires = exp + "T00:00:00Z"; }
    lics.push(l);
  });
  var body = {
    kind: val("s-kind") || "individual",
    legal_name: val("s-legal"), trading_name: val("s-trading"),
    licences: lics
  };
  if (val("s-carrier") || val("s-policy")) {
    body.insurance = {
      carrier: val("s-carrier"), policy_number: val("s-policy"),
      coverage_minor: minorOf("s-cover"), currency: "USD"
    };
    if (val("s-ins-exp")) { body.insurance.expires = val("s-ins-exp") + "T00:00:00Z"; }
  }
  btn.disabled = true; err.className = "err"; err.textContent = "";
  api("PUT", "/v1/supplier", body).then(function (res) {
    if (!res.ok) { throw new Error(errorOf(res, "could not save that")); }
    err.className = "err ok";
    err.textContent = res.body.note || "Saved.";
    return refreshSupplier();
  }).catch(function (e) { err.textContent = e.message; })
    .then(function () { btn.disabled = false; });
}

function refreshSupplier() {
  return api("GET", "/v1/supplier").then(function (res) {
    if (res.ok) { SUP = res.body; }
    var host = document.getElementById("supplier-wrap");
    if (host) { RV = 0; host.innerHTML = supplier(); wireSupplier(); }
  }).catch(function () {});
}

function wireSupplier() {
  var add = document.getElementById("lic-add");
  if (add) {
    add.addEventListener("click", function () {
      document.getElementById("lic-list").insertAdjacentHTML("beforeend", licenceRow(null));
      wireLicenceRows();
    });
  }
  wireLicenceRows();
  var save = document.getElementById("s-save");
  if (save) { save.addEventListener("click", saveSupplier); }
  var madd = document.getElementById("m-add-btn");
  if (madd) {
    madd.addEventListener("click", function () {
      var who = val("m-add"), err = document.getElementById("m-err");
      if (!who) { document.getElementById("m-add").focus(); return; }
      api("POST", "/v1/supplier/members", {person: who}).then(function (res) {
        if (!res.ok) { throw new Error(errorOf(res, "could not add them")); }
        return refreshSupplier();
      }).catch(function (e) { err.textContent = e.message; });
    });
  }
  document.querySelectorAll("[data-member-del]").forEach(function (b) {
    b.addEventListener("click", function () {
      var err = document.getElementById("m-err");
      b.disabled = true;
      api("DELETE", "/v1/supplier/members/" + encodeURIComponent(b.dataset.memberDel))
        .then(function (res) {
          if (!res.ok) { throw new Error(errorOf(res, "could not remove them")); }
          return refreshSupplier();
        }).catch(function (e) { err.textContent = e.message; b.disabled = false; });
    });
  });
}
function wireLicenceRows() {
  document.querySelectorAll("#lic-list .l-del").forEach(function (b) {
    if (b.dataset.wired) { return; }
    b.dataset.wired = "1";
    b.addEventListener("click", function () { b.closest(".lic").remove(); });
  });
}

function show(out) {
  if (out) { SUP = out[0] || null; SKILLS = (out[1] && out[1].skills) || []; }
  bodyEl().innerHTML = demoStrip() + '<div id="supplier-wrap">' + supplier() + '</div>';
  wireSupplier();
}
boot(["/v1/supplier", "/v1/skills"], show);
`

// consoleAlertsJS: what the exchange will email about, on either side.
const consoleAlertsJS = `
function alertsOperator() {
  var a = ALERTS || {};
  return head('<h2 id="alerts">Alerts</h2>', null, a.alerts_on ? "on" : "off") +
  '<div class="panes"><div' + rv("pane") + '>' +
    '<h3>As an operator</h3>' +
    '<div class="toggle" style="border-top:0;padding-top:0"><div>' +
      '<div class="tx">Email me when work appears that I could take</div>' +
      '<div class="sx">' + esc(a.note || "At most once every few hours, and only for jobs inside your capacity settings.") + '</div>' +
    '</div><button class="sw" id="al-work" aria-pressed="' + !!a.alerts_on + '" aria-label="Email me about new work"></button></div>' +
    (a.available === false
      ? '<p class="why" style="margin:.5rem 0 0">This exchange has no email configured yet, so nothing ' +
        'is sent. Your choice is kept and takes effect once it does.</p>'
      : '') +
    (!ME.verified ? '<p class="why" style="margin:.5rem 0 0">Alerts need a verified account with an email address.</p>' : '') +
    '<div class="err" id="al-err"></div>' +
  '</div>' + alertsBuyer() + '</div>';
}

function alertsBuyer() {
  var a = ALERTS || {};
  return '<div' + rv("pane") + ' id="alerts-buyer">' +
    '<h3>As a buyer</h3>' +
    '<div class="toggle" style="border-top:0;padding-top:0"><div>' +
      '<div class="tx">Email me when a job of mine is not being taken</div>' +
      '<div class="sx">Sent once, when a job has sat unfilled for a third of its life, with the ' +
        'reason and what you could change. Goes to the address on your account; ' +
        'it is always on and cannot be switched off here yet.</div>' +
    '</div><span class="chip ' + (a.available === false ? '' : 'ok') + '">' +
      (a.available === false ? 'email not configured' : 'on') + '</span></div>' +
  '</div>';
}

function setAlerts(on) {
  var err = document.getElementById("al-err");
  api("PUT", "/v1/alerts?on=" + (on ? "true" : "false")).then(function (res) {
    if (!res.ok) { throw new Error(errorOf(res, "could not save that")); }
    ALERTS = ALERTS || {};
    ALERTS.alerts_on = !!res.body.alerts_on;
    err.className = "err ok"; err.textContent = on ? "You will be told." : "Off.";
  }).catch(function (e) {
    err.className = "err"; err.textContent = e.message;
    var sw = document.getElementById("al-work");
    if (sw) { sw.setAttribute("aria-pressed", String(!on)); }
  });
}

function show(out) {
  if (out) { ALERTS = out[0] || null; }
  bodyEl().innerHTML = demoStrip() + alertsOperator();
  var al = document.getElementById("al-work");
  if (al) {
    al.addEventListener("click", function () {
      var on = al.getAttribute("aria-pressed") !== "true";
      al.setAttribute("aria-pressed", String(on));
      setAlerts(on);
    });
  }
}
boot(["/v1/alerts"], show);
`

// consoleDispatchJS: where offers are sent, for fleets that take work over
// the API.
const consoleDispatchJS = `
function dispatchPane() {
  var c = CAP.capacity || {}, hook = c.webhook || "";
  return head('<h2 id="integration">Dispatch endpoint</h2>', null, hook ? "set" : "for fleets that take work over the API") +
  '<div class="panes">' +
    '<div' + rv("pane") + '>' +
      '<h3>Dispatch endpoint</h3>' +
      '<p class="why">We post offers here; reply 202 to accept.</p>' +
      '<input type="text" id="c-hook" placeholder="https://your.host/lamdis/dispatch" ' +
        'value="' + esc(hook) + '">' +
      '<p class="why" style="margin:.6rem 0 0">HTTPS only. Saved when you leave the field. ' +
        '<a href="/console/capacity">Auto-accept</a> stays off until this is set' +
        (c.auto_accept ? '; it is on.' : '.') + '</p>' +
      '<div class="err" id="cap-err"></div>' +
    '</div>' +
    '<div' + rv("pane") + '>' +
      (c.webhook_secret
        ? '<h3>Signing secret</h3>' +
          '<p class="why">Every offer carries <code>X-Lamdis-Signature</code>, an ' +
            'HMAC-SHA256 over the timestamp, a newline, and the body. Check it before ' +
            'acting on an offer &mdash; anyone can POST to your endpoint.</p>' +
          '<code class="secret">' + esc(c.webhook_secret) + '</code>'
        : '<h3>Signing secret</h3><p class="why">Issued once an endpoint is set.</p>') +
    '</div>' +
  '</div>' +
  '<h2>What arrives</h2>' +
  '<pre class="api"><b>POST</b> ' + esc(hook || "https://your.host/lamdis/dispatch") + '\n' +
    'X-Lamdis-Timestamp: 2026-09-02T14:03:00Z\n' +
    'X-Lamdis-Signature: hmac-sha256(secret, timestamp + "\\n" + body)\n\n' +
    '{ "job": "…", "kind": "do", "title": "…", "pay_minor": 6000,\n' +
    '  "distance_miles": 4.2, "expires": "…", "claim": "/v1/workers/claim/{job}" }\n\n' +
    'reply <b>202</b> to take it; anything else and it is offered on</pre>';
}

function show(out) {
  if (out) { CAP = out[0] || {capacity: {}, ceiling: 1}; }
  bodyEl().innerHTML = demoStrip() + dispatchPane();
  var hook = document.getElementById("c-hook");
  if (hook) {
    hook.addEventListener("change", function () {
      saveCapacity(function () { RV = 0; show(null); });
    });
  }
}
boot(["/v1/capacity"], show);
`

// consoleLargerJS: multi-part work, explained where the person who needs it
// is standing.
//
// A paving contractor's agent that can bid on three separate listings is not
// the same thing as one that can price a scope. The difference is worth real
// money to them and the console said nothing about it. SCOPE is the
// demonstration project: a real listing on a real board with real rules,
// marked practice throughout, so a business can read exactly how a job too
// big for one visit behaves before deciding whether to wire anything up.
const consoleLargerJS = `
function largerJobs() {
  var sc = SCOPE, pieces = "", n = 0;
  if (sc && sc.jobs && sc.jobs.length) {
    pieces = sc.jobs.map(function (j) {
      n++;
      var blocked = (j.blocked_by || []).length > 0;
      // Generic. An earlier draft explained the paving demo's own reason
      // here — "the mixer crosses this ground" — which would have been
      // asserted over every blocked piece of every project, including ones
      // with nothing to do with concrete.
      var sub = blocked
        ? "Cannot start until piece " + pieceNumber(sc, j.blocked_by[0]) +
          " is finished and accepted"
        : "Can start as soon as it is awarded";
      return '<div class="piece' + (blocked ? " blocked" : "") + '">' +
        '<div class="num">' + n + '</div>' +
        '<div><div class="t">' + esc(j.title) + '</div>' +
        '<div class="s' + (blocked ? " warn" : "") + '">' + sub + '</div></div>' +
        '</div>';
    }).join("");
  }

  return head('<h2 id="larger">Larger jobs</h2>', null, "more than one visit, place or trade") +
  '<p class="lead">Everything below is live on the board right now as a ' +
    'demonstration you can read, price and bid against without a cent moving.</p>' +

  (pieces
    ? '<div' + rv("scope") + '>' +
        '<div class="hd"><b>' + esc((sc.project && sc.project.title) || "Demonstration scope") +
          '</b><span>' + esc(sc.jobs.length) + ' pieces &middot; ' +
          ((sc.project && sc.project.one_visit) ? "one address" : (sc.project.sites + " sites")) +
          '</span></div>' + pieces +
      '</div>'
    : '<div' + rv("note-box") + '>The demonstration scope is not on this board.</div>') +

  '<div' + rv("note-box") + '><b>Why the grouping matters.</b> Getting a crew and a ' +
    'paver to a site is most of the cost of a small job. Three jobs at one ' +
    'address is <b>one</b> mobilisation. Shown as three unrelated listings, you ' +
    'either price three of them and lose, or price one and are ruined if you win ' +
    'two. So the board says what a job is a piece of, and you can make one offer ' +
    'for the whole thing.</div>' +

  '<div class="panes">' +
    '<div' + rv("pane") + '>' +
      '<h3>One offer, all or nothing</h3>' +
      '<p class="why">Price each piece, send it as one bid. It is awarded ' +
        'together or not at all, so the piece carrying your mobilisation cannot ' +
        'be cherry-picked away from the pieces that pay for it.</p>' +
    '</div>' +
    '<div' + rv("pane") + '>' +
      '<h3>Order that is enforced</h3>' +
      '<p class="why">A piece that depends on another cannot be claimed until ' +
        'that one is finished <i>and accepted</i>. Nobody else can book the ' +
        'ground you need on the morning you need it.</p>' +
    '</div>' +
    '<div' + rv("pane") + '>' +
      '<h3>You write the stages</h3>' +
      '<p class="why">On these jobs the buyer says what they want and what they ' +
        'will pay. <b>You</b> say how it breaks down &mdash; prep, base, binder, ' +
        'surface &mdash; and what each is worth, and you are paid per stage as ' +
        'each is accepted. A homeowner does not know what a binder course is, ' +
        'and neither does their agent.</p>' +
    '</div>' +
    '<div' + rv("pane") + '>' +
      '<h3>Many sites, one buyer</h3>' +
      '<p class="why">A project can span locations as easily as trades. Each ' +
        'piece carries the buyer\'s own site reference, so four hundred stores ' +
        'stay distinguishable on the receipts your accounts department reads.</p>' +
    '</div>' +
  '</div>' +

  '<h2>What your agent calls</h2>' +
  '<pre class="api">' +
    '<b>GET</b>  /v1/scope/' + esc(DEMO_PROJECT) + '\n' +
    '     the whole scope, in the order it has to happen, with what blocks what\n\n' +
    '<b>POST</b> /v1/scope/' + esc(DEMO_PROJECT) + '/bid\n' +
    '     { "lines": [ {"job":"...","amount_minor":480000,\n' +
    '                   "note":"carries mobilisation for all three"}, ... ],\n' +
    '       "note": "slab first, cure over the weekend, both drives Mon-Tue",\n' +
    '       "all_or_nothing": true }\n\n' +
    '<b>POST</b> /v1/workers/plan/{job}\n' +
    '     { "stages": [ {"name":"Aggregate base",\n' +
    '                    "deliverable":"Base compacted to depth",\n' +
    '                    "pay_minor":150000}, ... ] }\n' +
    '     your breakdown; the stages must add up to the price you were awarded' +
  '</pre>' +
  '<p class="why" style="margin-bottom:1.4rem;font-size:.82rem;color:var(--ink-3)">Same signing as every other ' +
    'route. Full reference in <a href="/docs">the docs</a>.</p>';
}

// pieceNumber turns a job id into its position, so a dependency reads as
// "waits on piece 1" rather than as an identifier nobody recognises.
function pieceNumber(sc, job) {
  for (var i = 0; i < sc.jobs.length; i++) {
    if (sc.jobs[i].job === job) { return i + 1; }
  }
  return "?";
}

function show(out) {
  if (out) { SCOPE = out[0] || null; }
  bodyEl().innerHTML = demoStrip() + largerJobs();
}
boot(["/v1/scope/" + DEMO_PROJECT], show);
`

// --- buyer pages -------------------------------------------------------------

// consoleBuyJS: the buyer's own hand on the board.
//
// Until this existed a person could sign in, add funds and issue keys, and
// then had to hand the actual buying to an agent or to curl. The form speaks
// the board's language: what should be true, what to do about it, what proof
// looks like, where, and what it pays. Beside it, the row the board will
// show, redrawn on every keystroke.
const consoleBuyJS = `
var PKIND = "do", PPRICING = "fixed";

function postForm() {
  return head('<h2 id="post">Post a job</h2>', null, "ask somebody to find out whether something is true, or to make it true") +
  (!ME.verified
    ? '<div' + rv("strip warn") + '><span class="d"></span><span>Posting needs a verified account. ' +
      '<a href="/signin">Sign in with your email</a> to verify.</span></div>'
    : '') +
  '<div class="post-grid"><div id="post-form"><div class="panes">' +
    '<div' + rv("pane") + '>' +
      '<h3>What</h3>' +
      '<div class="field"><label>Kind</label><div class="seg">' +
        '<button class="kind" data-pkind="observe" aria-pressed="' + (PKIND === "observe") + '">Find out</button>' +
        '<button class="kind" data-pkind="do" aria-pressed="' + (PKIND === "do") + '">Make it true</button>' +
      '</div><div class="hint" id="p-kind-hint"></div></div>' +
      '<div class="field"><label for="p-pred">What should be true</label>' +
        '<textarea id="p-pred" rows="2" placeholder="The gutters on the north side are clear."></textarea>' +
        '<div class="hint">Shown on the open board. Keep the street address out of it.</div></div>' +
      '<div class="field" id="p-instr-f"><label for="p-instr">What to do</label>' +
        '<textarea id="p-instr" rows="3" placeholder="Ladder is in the garage; side gate code 4471. Clear both runs and the downpipe."></textarea>' +
        '<div class="hint">Only the person who takes the job sees this.</div></div>' +
      '<div class="field"><label for="p-deliv">What proof looks like</label>' +
        '<input type="text" id="p-deliv" placeholder="A photo along each gutter run, from the ladder, after.">' +
        '<div class="hint">This is what the evidence is checked against, and what a hold has to be about.</div></div>' +
      '<div class="field"><label for="p-refs">Reference photos</label>' +
        '<input type="file" id="p-refs" accept="image/*" multiple>' +
        '<div class="hint">Up to six: the site, the access, the number on the door. Published with the job so people can price it.</div>' +
        '<div class="refs" id="p-refs-preview"></div></div>' +
    '</div>' +
    '<div' + rv("pane") + '>' +
      '<h3>Where</h3>' +
      '<div class="field"><label for="p-where">Address</label>' +
        '<input type="text" id="p-where" placeholder="812 Marlow Street, Dearborn MI">' +
        '<div class="hint">Reaches the person who takes it, never the board. The board sees the area below.</div></div>' +
      '<div class="field"><label for="p-area">Area shown on the board</label>' +
        '<input type="text" id="p-area" placeholder="Dearborn, MI"></div>' +
      '<div class="three">' +
        '<div class="field"><label for="p-lat">Latitude</label><input type="number" id="p-lat" step="any" placeholder="42.3314"></div>' +
        '<div class="field"><label for="p-lon">Longitude</label><input type="number" id="p-lon" step="any" placeholder="-83.0458"></div>' +
        '<div class="field"><label for="p-radius">Radius, m</label><input type="number" id="p-radius" min="10" step="10" value="150"></div>' +
      '</div>' +
      '<div class="ctl" style="gap:.5rem"><button class="btn sm" id="p-locate">Use my location</button>' +
        '<span class="why" style="margin:0;font-size:.76rem;color:var(--ink-3)" id="p-loc-note">Photos are checked against this spot.</span></div>' +
      '<h3 style="margin-top:.8rem">When</h3>' +
      '<div class="field"><label for="p-ttl">Open for, hours</label>' +
        '<input type="number" id="p-ttl" min="1" max="720" value="24">' +
        '<div class="hint">After this the job expires and the money comes back.</div></div>' +
    '</div>' +
    '<div' + rv("pane") + '>' +
      '<h3>Pay</h3>' +
      '<div class="field"><label>Pricing</label><div class="seg">' +
        '<button class="kind" data-ppricing="fixed" aria-pressed="' + (PPRICING === "fixed") + '">Fixed price</button>' +
        '<button class="kind" data-ppricing="bids" aria-pressed="' + (PPRICING === "bids") + '">Take bids</button>' +
      '</div></div>' +
      '<div class="two">' +
        '<div class="field" id="p-fee-f"><label for="p-fee">Pay on completion, $</label>' +
          '<input type="number" id="p-fee" min="1" step="1" placeholder="60"></div>' +
        '<div class="field" id="p-max-f" hidden><label for="p-max">Most you will pay, $</label>' +
          '<input type="number" id="p-max" min="1" step="1" placeholder="400">' +
          '<div class="hint">Nothing is held until you accept an offer, but you must have this available.</div></div>' +
        '<div class="field" id="p-close-f" hidden><label for="p-close">Offers close in, hours</label>' +
          '<input type="number" id="p-close" min="1" max="168" value="24"></div>' +
        '<div class="field" id="p-attempt-f"><label for="p-attempt">Wasted trip, $</label>' +
          '<input type="number" id="p-attempt" min="0" step="1" placeholder="0">' +
          '<div class="hint">Paid for a documented failed attempt: locked gate, nobody home.</div></div>' +
        '<div class="field"><label for="p-expense">Expense cap, $</label>' +
          '<input type="number" id="p-expense" min="0" step="1" placeholder="0">' +
          '<div class="hint">Most they may lay out and reclaim against a receipt.</div></div>' +
      '</div>' +
      '<div class="fund" id="p-fund"></div>' +
      '<div class="acts">' +
        '<button class="btn" id="p-quote">Check feasibility</button>' +
        '<button class="btn go" id="p-post"' + (ME.verified ? '' : ' disabled') + '>Post this job</button>' +
      '</div>' +
      '<div id="p-quote-out"></div>' +
      '<div class="err" id="p-err"></div>' +
      '<div id="p-out"></div>' +
    '</div>' +
  '</div></div>' +
  '<div class="pv-wrap"><div' + rv("pv") + ' id="p-preview"></div></div>' +
  '</div>';
}

// What posting will hold, against what the balance can cover. Recomputed on
// every keystroke so the shortfall is known before the button is pressed
// rather than as a 402 after it.
function escrowNeed() {
  if (PPRICING === "bids") { return minorOf("p-max"); }
  var per = minorOf("p-fee"), attempt = minorOf("p-attempt");
  if (attempt > per) { per = attempt; }
  return per + minorOf("p-expense");
}
function fundingNote() {
  var out = document.getElementById("p-fund");
  if (!out) { return; }
  var sp = SPEND || {};
  var balance = sp.balance_minor || 0, held = sp.held_minor || 0;
  var avail = balance - held;
  var need = escrowNeed(), sentence;
  if (PPRICING === "bids") {
    sentence = "Asking for bids up to <b>" + money(need) + "</b> holds nothing yet, but you need " +
      "that much available to ask.";
  } else {
    sentence = "Posting holds <b>" + money(need) + "</b> in escrow until the proof is accepted or the job expires.";
  }
  var short = need > avail;
  out.className = "fund" + (short ? " short" : "");
  out.innerHTML = sentence + " Available: <b>" + money(avail) + "</b>" +
    (held ? " (" + money(balance) + " less " + money(held) + " held for open jobs)" : "") + "." +
    (short ? " Short by <b>" + money(need - avail) + "</b> — <a href=\"/console/funds\">add funds</a> first." : "");
  updatePreview();
}

// The row the board will show, as the fields stand. Everything the board
// publishes is here and nothing it withholds: the address and the what-to-do
// are listed as private so the buyer sees the line the exchange draws.
function updatePreview() {
  var host = document.getElementById("p-preview");
  if (!host) { return; }
  var pred = val("p-pred"), deliv = val("p-deliv"), area = val("p-area"), instr = val("p-instr");
  var lat = parseFloat(val("p-lat")), lon = parseFloat(val("p-lon")), radius = parseInt(val("p-radius") || "0", 10);
  var ttl = Math.max(1, parseInt(val("p-ttl") || "24", 10));
  var located = !isNaN(lat) && !isNaN(lon) && radius > 0;
  var fee = minorOf("p-fee"), max = minorOf("p-max"), attempt = minorOf("p-attempt"), expense = minorOf("p-expense");
  var refs = ((document.getElementById("p-refs") || {}).files || []).length;
  var sp = SPEND || {}, avail = (sp.balance_minor || 0) - (sp.held_minor || 0), need = escrowNeed();
  var bids = PPRICING === "bids";
  var amt = bids ? (max ? "up to " + money(max) : "&mdash;") : (fee ? money(fee) : "&mdash;");
  var kindChip = '<span class="chip ' + (PKIND === "observe" ? "obs" : "do") + '">' +
    (PKIND === "observe" ? "find out" : "make it true") + '</span>';
  var checks = [
    [!!deliv, deliv ? "Proof matches: " + esc(deliv) : "Proof: say what it looks like"],
    [true, "Code in frame, taken at the time"],
    [located, located ? "Within " + radius + " m of the address" : "No location: photos are not tied to a place"]
  ];
  if (PKIND === "do") { checks.push([attempt > 0, attempt > 0 ? "Wasted trip pays " + money(attempt) : "Nothing paid for a wasted trip"]); }
  if (expense > 0) { checks.push([true, "Expenses reclaimable up to " + money(expense)]); }
  host.className = "pv rv " + (PKIND === "observe" ? "obs" : "do");
  host.innerHTML =
    '<div class="pv-top"><span>What the operator sees</span><span class="live"><span class="beacon"></span>As typed</span></div>' +
    '<div class="job"><div class="jrow"><div class="jbody">' +
      '<div class="jt' + (pred ? '' : ' ph') + '">' + kindChip + (pred ? esc(pred) : "What should be true") + '</div>' +
      '<div class="meta">' + (area ? esc(area) : "anywhere") + ' &middot; open ' + ttl + ' h' +
        (bids ? ' &middot; sealed bids' : '') + (refs ? ' &middot; ' + refs + ' photo' + (refs === 1 ? '' : 's') : '') + '</div>' +
      '<div class="dv' + (deliv ? '' : ' ph') + '">' + (deliv ? esc(deliv) : "What proof looks like") + '</div>' +
    '</div><div class="amt"><div class="n' + ((bids ? max : fee) ? '' : ' quiet') + '">' + amt + '</div>' +
      '<div class="s">' + (bids ? "best offer wins" : "on completion") + '</div></div></div></div>' +
    '<div class="checks"><span class="k">Checked before a cent moves</span><ul>' + checks.map(function (c) {
      return '<li class="' + (c[0] ? "on" : "") + '"><i></i>' + c[1] + '</li>';
    }).join("") + '</ul></div>' +
    '<div class="hold"><div><span class="amt">' + money(need) + '</span><span class="who">' +
      (bids ? "must be available to ask; held when you accept an offer" : "held when you post; back if nobody takes it") +
      '</span></div><span class="st ' + (need ? (need > avail ? "short" : "ok") : "") + '">' +
      (need ? (need > avail ? "short " + money(need - avail) : "covered") : "&mdash;") + '</span></div>' +
    '<div class="priv">' + (PKIND === "do"
      ? (instr ? "What to do, and the address, go only to the person who takes it." : "The address, and what to do, go only to the person who takes it.")
      : "The address goes only to the person who takes it.") + '</div>';
}

function setPKind(k) {
  PKIND = k;
  document.querySelectorAll("[data-pkind]").forEach(function (b) {
    b.setAttribute("aria-pressed", String(b.dataset.pkind === k));
  });
  document.getElementById("p-instr-f").hidden = k !== "do";
  document.getElementById("p-attempt-f").hidden = k !== "do";
  document.getElementById("p-kind-hint").textContent = k === "do"
    ? "Somebody goes and does it. Paid on completion, or the wasted-trip amount if they could not."
    : "Somebody goes and looks. Paid for admissible evidence whichever way the answer turns out.";
  fundingNote();
}
function setPPricing(p) {
  PPRICING = p;
  document.querySelectorAll("[data-ppricing]").forEach(function (b) {
    b.setAttribute("aria-pressed", String(b.dataset.ppricing === p));
  });
  document.getElementById("p-fee-f").hidden = p !== "fixed";
  document.getElementById("p-max-f").hidden = p !== "bids";
  document.getElementById("p-close-f").hidden = p !== "bids";
  fundingNote();
}

function jobBody() {
  var body = {
    kind: PKIND, predicate: val("p-pred"), deliverable: val("p-deliv"),
    where: val("p-where"), area: val("p-area"), currency: "USD",
    ttl_seconds: Math.max(1, parseInt(val("p-ttl") || "24", 10)) * 3600
  };
  if (PKIND === "do") {
    body.instructions = val("p-instr");
    body.attempt_minor = minorOf("p-attempt");
  }
  var lat = parseFloat(val("p-lat")), lon = parseFloat(val("p-lon"));
  if (!isNaN(lat) && !isNaN(lon)) {
    body.lat = lat; body.lon = lon;
    body.radius_m = Math.max(0, parseInt(val("p-radius") || "0", 10));
  }
  body.expense_cap_minor = minorOf("p-expense");
  if (PPRICING === "bids") {
    body.pricing = "bids";
    body.max_bid_minor = minorOf("p-max");
    // The route insists a job pays something; for an open job the ceiling is
    // that something until an offer is accepted.
    body.fee_minor = body.max_bid_minor;
    body.bids_close_in_hours = Math.max(1, parseInt(val("p-close") || "24", 10));
  } else {
    body.fee_minor = minorOf("p-fee");
  }
  return body;
}

// The same checks the route makes, made here first so the message arrives
// next to the field rather than as a status code.
function validateJob(b) {
  if (!b.predicate) { return "Say what should be true."; }
  if (b.kind === "do" && !b.instructions) { return "A job that asks somebody to do something must say what to do."; }
  if (!(b.fee_minor > 0)) { return PPRICING === "bids" ? "Say the most you will pay." : "Say what it pays."; }
  if (b.where && !(b.radius_m > 0)) {
    return "An address needs a latitude, longitude and radius, or there is nothing to check the photographs against. Use your location, or clear the address.";
  }
  return "";
}

function checkFeasibility() {
  var b = jobBody(), out = document.getElementById("p-quote-out"), btn = document.getElementById("p-quote");
  if (!b.predicate) { document.getElementById("p-err").textContent = "Say what should be true first."; return; }
  document.getElementById("p-err").textContent = "";
  btn.disabled = true;
  api("POST", "/v1/quote", {
    kind: b.kind, predicate: b.predicate, instructions: b.instructions || "",
    detail: b.deliverable || "", lat: b.lat || 0, lon: b.lon || 0, slots: 1, tier: "V2"
  }).then(function (res) {
    if (!res.ok) { throw new Error(errorOf(res, "could not check that")); }
    var q = res.body, s = q.settled_here;
    out.innerHTML = '<div class="receipt rv" style="margin-top:.6rem">' +
      '<div><span class="chip ' + (q.refused ? "bad" : q.feasible ? "ok" : "warn") + '">' +
        (q.refused ? "would be refused" : q.feasible ? "somebody could take this" : "nobody in range") + '</span>' +
        esc(q.reachable ? q.reachable + " operators reachable" : "") + '</div>' +
      (q.why ? '<div class="fn">' + esc(q.why) + '</div>' : '') +
      (q.refused_why ? '<div class="fn">' + esc(q.refused_why) + '</div>' : '') +
      (s ? '<h4>What work like this has settled at here</h4><div class="fn">' +
        money(s.low_minor, s.currency) + ' &ndash; ' + money(s.high_minor, s.currency) +
        ', median ' + money(s.median_minor, s.currency) + ', from ' + esc(s.based_on) + ' jobs</div>' : '') +
      ((q.advice || []).length ? '<h4>Advice</h4><ul>' + q.advice.map(function (a) {
        return '<li>' + esc(a) + '</li>'; }).join("") + '</ul>' : '') +
    '</div>';
  }).catch(function (e) { out.innerHTML = '<div class="err">' + esc(e.message) + '</div>'; })
    .then(function () { btn.disabled = false; });
}

// A phone photograph is several megabytes; the reference route reads one.
// Scaled in the browser, which also strips whatever the camera wrote into it.
function shrinkImage(file) {
  return new Promise(function (resolve) {
    if (!file.type || file.type.indexOf("image/") !== 0 || file.size < 900000) { resolve(file); return; }
    var url = URL.createObjectURL(file), img = document.createElement("img");
    img.onload = function () {
      var max = 1600, w = img.naturalWidth, h = img.naturalHeight;
      var k = Math.min(1, max / Math.max(w, h, 1));
      var c = document.createElement("canvas");
      c.width = Math.round(w * k); c.height = Math.round(h * k);
      c.getContext("2d").drawImage(img, 0, 0, c.width, c.height);
      URL.revokeObjectURL(url);
      c.toBlob(function (b) { resolve(b || file); }, "image/jpeg", 0.85);
    };
    img.onerror = function () { resolve(file); };
    img.src = url;
  });
}

function uploadReferences(job, files, progress) {
  var chain = Promise.resolve(0), n = 0;
  var say = progress;
  files.slice(0, 6).forEach(function (file, i) {
    chain = chain.then(function () {
      say("Attaching photo " + (i + 1) + " of " + Math.min(files.length, 6) + "…");
      return shrinkImage(file);
    }).then(function (blob) {
      var path = "/v1/jobs/" + encodeURIComponent(job) + "/references" +
        (i === 0 ? "?identifies=true" : "");
      return api("POST", path, blob, blob.type || "image/jpeg");
    }).then(function (res) {
      if (res.ok) { n++; }
      return n;
    });
  });
  return chain;
}

function submitJob() {
  var btn = document.getElementById("p-post"), err = document.getElementById("p-err");
  var out = document.getElementById("p-out");
  var b = jobBody();
  var problem = validateJob(b);
  err.textContent = problem;
  if (problem) { return; }
  var files = Array.prototype.slice.call((document.getElementById("p-refs") || {}).files || []);
  btn.disabled = true; btn.textContent = "Posting…"; out.innerHTML = "";
  var posted = null;
  api("POST", "/v1/tasks", b).then(function (res) {
    if (!res.ok) { throw new Error(errorOf(res, "could not post that")); }
    posted = res.body;
    return uploadReferences(posted.job, files, function (msg) { btn.textContent = msg; });
  }).then(function (attached) {
    var job = posted.job;
    out.innerHTML = '<div class="receipt rv">' +
      '<div><span class="chip ok">posted</span><b>' + esc(job) + '</b></div>' +
      '<div class="fn">' + money(posted.escrowed || 0) + ' held &middot; expires ' + esc(when(posted.expires)) +
        (attached ? ' &middot; ' + attached + ' photo' + (attached === 1 ? '' : 's') + ' attached' : '') + '</div>' +
      (posted.warning ? '<div class="fn" style="color:var(--warn)">' + esc(posted.warning) + '</div>' : '') +
      '<div class="acts" style="margin-top:.7rem">' +
        '<a class="btn sm" href="/j/' + encodeURIComponent(job) + '">Public page</a>' +
        '<a class="btn sm" href="/console/spending">Status, under Spending</a>' +
        '<a class="btn sm" href="/board">See it on the board</a>' +
      '</div>' +
      '<div class="fn"><code>GET /v1/jobs/' + esc(job) + '</code> is the status your agent would read.</div>' +
    '</div>';
    document.getElementById("p-pred").value = "";
    document.getElementById("p-instr").value = "";
    return refreshSpending();
  }).catch(function (e) { err.textContent = e.message; })
    .then(function () { btn.disabled = false; btn.textContent = "Post this job"; });
}

// The balance moved, so the tiles and the funding line are re-read. The form
// itself is left alone: somebody may be halfway through the next job.
function refreshSpending() {
  return api("GET", "/v1/spend").then(function (res) {
    if (res.ok) { SPEND = res.body; }
    var host = document.getElementById("hud-wrap");
    if (host) { RV = 0; host.innerHTML = buyerHud(); animateTiles(); }
    fundingNote();
  }).catch(function () {});
}

function wirePost() {
  document.querySelectorAll("[data-pkind]").forEach(function (b) {
    b.addEventListener("click", function () { setPKind(b.dataset.pkind); });
  });
  document.querySelectorAll("[data-ppricing]").forEach(function (b) {
    b.addEventListener("click", function () { setPPricing(b.dataset.ppricing); });
  });
  var form = document.getElementById("post-form");
  if (form) {
    form.addEventListener("input", function () { fundingNote(); });
    form.addEventListener("change", function () { fundingNote(); });
  }
  var locate = document.getElementById("p-locate");
  if (locate) {
    locate.addEventListener("click", function () {
      var note = document.getElementById("p-loc-note");
      if (!navigator.geolocation) { note.textContent = "This browser will not share a location."; return; }
      note.textContent = "Asking…";
      navigator.geolocation.getCurrentPosition(function (pos) {
        document.getElementById("p-lat").value = pos.coords.latitude.toFixed(6);
        document.getElementById("p-lon").value = pos.coords.longitude.toFixed(6);
        note.textContent = "Set from this device. Adjust if the job is somewhere else.";
        fundingNote();
      }, function () {
        note.textContent = "Location refused. Type the coordinates instead.";
      });
    });
  }
  var refs = document.getElementById("p-refs");
  if (refs) {
    refs.addEventListener("change", function () {
      var host = document.getElementById("p-refs-preview");
      var files = Array.prototype.slice.call(refs.files || []);
      host.innerHTML = files.slice(0, 6).map(function (f) {
        return '<img alt="" src="' + URL.createObjectURL(f) + '">';
      }).join("") + (files.length > 6 ? '<div class="hint">Only the first six are attached.</div>' : '');
      fundingNote();
    });
  }
  var quote = document.getElementById("p-quote");
  if (quote) { quote.addEventListener("click", checkFeasibility); }
  var post = document.getElementById("p-post");
  if (post) { post.addEventListener("click", submitJob); }
  if (document.getElementById("p-instr-f")) {
    setPKind(PKIND);
    setPPricing(PPRICING);
  }
}

function show(out) {
  if (out) { SPEND = out[0] || null; }
  bodyEl().innerHTML = demoStrip() + '<div id="hud-wrap">' + buyerHud() + '</div>' + postForm();
  wirePost();
  animateTiles();
}
boot(["/v1/spend"], show);
`

// consoleSpendingJS: what the money bought, what came back, and the two
// buttons that decide whether it is paid.
const consoleSpendingJS = `
function spendRow(j, cur, i) {
  var review = j.review || {};
  var waiting = review.awaiting_release_minor;
  var chip = waiting ? "hot"
           : (j.status === "done" ? "ok"
           : (j.status.indexOf("refunding") > -1 ? "bad" : ""));
  var bidding = j.status === "collecting offers";
  return '<div' + rvi(i, "r") + ' style="align-items:flex-start;--i:' + Math.min(i, 12) + '"><div class="grow">' +
    '<div class="t"><span class="chip ' + chip + '">' +
      (waiting ? "needs you" : esc(j.status)) + '</span>' +
      (j.kind ? '<span class="chip ' + (j.kind === "observe" ? "obs" : "do") + '">' +
        (j.kind === "observe" ? "find out" : "make it true") + '</span>' : '') +
      esc(j.title) + '</div>' +
    '<div class="m">' + esc(when(j.posted)) +
      (j.where ? ' &middot; ' + esc(j.where) : '') +
      (j.worker ? ' &middot; taken by ' + esc(j.worker) +
        (j.worker_completed ? ' (' + j.worker_completed + ' done here)' : ' (first job here)') : '') +
      (j.supplier && (j.supplier.trading_name || j.supplier.legal_name)
        ? ' of ' + esc(j.supplier.trading_name || j.supplier.legal_name) : '') +
      (j.stages_total
        ? ' &middot; ' + j.stages_done + '/' + j.stages_total + ' stages'
        : (j.submissions ? ' &middot; ' + j.submissions + ' submitted' : '')) +
      (j.agent ? ' &middot; via key ' + esc(j.agent) : '') +
      ' &middot; <a href="/j/' + encodeURIComponent(j.job) + '">public page</a>' +
    '</div>' +
    '<div class="acts">' +
      (j.evidence ? '<button class="btn sm" data-evidence="' + esc(j.job) + '">See what came back</button>' : '') +
      (j.receipt ? '<button class="btn sm" data-receipt="' + esc(j.job) + '">Receipt</button>' : '') +
      (bidding ? '<button class="btn sm" data-bids="' + esc(j.job) + '">See offers</button>' : '') +
      '<button class="btn sm" data-status="' + esc(j.job) + '">Status</button>' +
    '</div>' +
    (waiting
      ? '<div class="m" style="margin-top:.45rem">' +
          money(waiting, cur) + ' goes to them ' +
          (review.hours_left > 1
            ? 'in about ' + Math.round(review.hours_left) + ' hours'
            : 'shortly') +
          ' unless you say otherwise.' +
        '</div>' +
        '<div class="acts" style="margin-top:.45rem">' +
          '<button class="btn sm go" data-release="' + esc(j.job) + '">Looks good, pay now</button>' +
          '<button class="btn sm" data-hold="' + esc(j.job) + '">Something is wrong</button>' +
        '</div><div class="err" id="rv-' + esc(j.job) + '"></div>'
      : (review.held_minor
          ? '<div class="m" style="margin-top:.35rem">' +
              money(review.held_minor, cur) + ' is on hold while this is looked at.</div>'
          : "")) +
    '<div class="jx" id="jx-' + esc(j.job) + '"></div>' +
    '</div>' +
    '<span class="amt">' + money(j.committed_minor, cur) + '</span></div>';
}

function spending() {
  var sp = SPEND || {jobs: []};
  var cur = sp.currency || "USD";
  var jobs = sp.jobs || [];
  var rows = jobs.map(function (j, i) { return spendRow(j, cur, i); }).join("");
  var need = sp.awaiting_review || 0;
  return head('<h2 id="spending">Spending</h2>', jobs.length, "by you from the post form, or by an agent holding one of your keys") +
    (need
      ? '<div' + rv("strip warn") + '><span class="d"></span><span>' + need +
        (need === 1 ? ' job is' : ' jobs are') + ' finished and waiting on you. ' +
        'Payment goes out when the review window closes.</span></div>'
      : "") +
    (rows ? '<div class="rows">' + rows + '</div>'
          : '<div' + rv("rows") + '><div class="empty">Nothing bought yet. <a href="/console/buy">Post a job.</a></div></div>');
}

function refreshSpending() { reload(); }

// Accepting the work pays a good worker straight away rather than making them
// wait out a window that exists for the buyer's benefit.
function reviewAction(job, action, reason) {
  var err = document.getElementById("rv-" + job);
  var path = "/v1/jobs/" + encodeURIComponent(job) + "/" + action;
  err.textContent = "";
  api("POST", path, reason ? {reason: reason} : null).then(function (res) {
    if (!res.ok) { throw new Error(errorOf(res, "could not do that")); }
    refreshSpending();
  }).catch(function (e) { err.textContent = e.message; });
}

// What the receipt says, rendered rather than dumped: what was established,
// what was not, how sure anybody may be and why that number.
function showReceipt(job) {
  var host = document.getElementById("jx-" + job);
  host.innerHTML = '<div class="empty" style="padding:1rem">Fetching the receipt…</div>';
  api("GET", "/v1/jobs/" + encodeURIComponent(job) + "/receipt").then(function (res) {
    if (!res.ok) { throw new Error(errorOf(res, "no receipt yet")); }
    var r = res.body, v = r.verification || {};
    var li = function (s) { return '<li>' + esc(s) + '</li>'; };
    var ev = (r.evidence || []).map(function (e) {
      var files = (e.files || []).map(function (f) {
        return '<li>' + esc(f.kind || "file") + ' <span style="font-family:var(--mono)">' +
          esc(String(f.sha256 || "").slice(0, 12)) + '</span>' +
          (f.challenge_found_in ? ' &middot; code seen in ' + esc(f.challenge_found_in) : '') +
          (f.lat != null ? ' &middot; ' + esc(f.lat.toFixed(5)) + ', ' + esc(f.lon.toFixed(5)) : '') +
          (f.transcript ? '<div class="fn">“' + esc(f.transcript) + '”</div>' : '') + '</li>';
      }).join("");
      return '<li><span class="chip ' + (e.accepted ? "ok" : "bad") + '">' +
        (e.accepted ? "accepted" : "not accepted") + '</span>' + esc(when(e.at)) +
        (e.attested_by ? ' &middot; attested by ' + esc(e.attested_by) : '') +
        (e.why ? '<div class="fn">' + esc(e.why) + '</div>' : '') +
        (files ? '<ul style="margin-top:.3rem">' + files + '</ul>' : '') + '</li>';
    }).join("");
    host.innerHTML = '<div class="receipt rv">' +
      '<div><span class="chip ' + (r.accepted ? "ok" : "warn") + '">' +
        (r.accepted ? "accepted" : "not accepted") + '</span><b>' + esc(r.predicate) + '</b>' +
        ' <span style="color:var(--ink-3)">&middot; ' + esc(r.kind) +
        (r.reference ? ' &middot; ref ' + esc(r.reference) : '') +
        (r.site ? ' &middot; site ' + esc(r.site) : '') + '</span></div>' +
      '<h4>How sure anyone can be</h4>' +
      '<div class="ceil">' + esc(v.confidence_ceiling != null ? v.confidence_ceiling : "—") + '</div>' +
      '<div class="fn">' + esc(v.ceiling_because || "") +
        (v.tier_requested ? ' &middot; you asked for ' + esc(v.tier_requested) : '') + '</div>' +
      '<h4>What was established</h4>' +
      ((v.established || []).length ? '<ul>' + v.established.map(li).join("") + '</ul>'
        : '<div class="fn">Nothing yet.</div>') +
      '<h4>What was not</h4>' +
      ((v.limits || []).length ? '<ul>' + v.limits.map(li).join("") + '</ul>' : '<div class="fn">&mdash;</div>') +
      (v.site_mark
        ? '<div class="fn">Property mark “' + esc(v.site_mark.text) + '” ' +
          (v.site_mark.seen ? 'was legible in the evidence' : 'was not legible in the evidence') +
          (v.site_mark.inferred ? ' (inferred from the address)' : '') + '.</div>'
        : '') +
      '<h4>Evidence</h4>' + (ev ? '<ul>' + ev + '</ul>' : '<div class="fn">Nothing submitted.</div>') +
      (r.escrow_remaining_minor != null
        ? '<h4>Money</h4><div class="fn">' + money(r.escrow_remaining_minor, "USD") + ' still in escrow for this job.</div>'
        : '') +
      '<h4>Signed</h4><div class="fn">by ' + esc(r.issued_by) + ' at ' + esc(r.issued_at) +
        '<br>signature ' + esc(String(r.signature || "").slice(0, 24)) + '…</div>' +
      '<details><summary>Raw receipt, as signed</summary><pre>' +
        esc(JSON.stringify(r, null, 2)) + '</pre></details>' +
      '<div style="margin-top:.7rem"><button class="btn sm" data-close="' + esc(job) + '">Close</button></div>' +
    '</div>';
    wireClose(host);
  }).catch(function (e) { host.innerHTML = '<div class="err">' + esc(e.message) + '</div>'; });
}

// The files themselves. Images and video are fetched with the session and
// shown inline; a bare <img src> cannot carry a bearer token.
function showEvidence(job) {
  var host = document.getElementById("jx-" + job);
  host.innerHTML = '<div class="empty" style="padding:1rem">Fetching what came back…</div>';
  api("GET", "/v1/jobs/" + encodeURIComponent(job) + "/evidence").then(function (res) {
    if (!res.ok) { throw new Error(errorOf(res, "could not load the evidence")); }
    var files = res.body.files || [];
    if (!files.length) {
      host.innerHTML = '<div class="fn">Nothing has come back yet.</div>';
      return;
    }
    host.innerHTML = '<div class="ev">' + files.map(function (f, i) {
      var flags = [];
      if (f.looks_generated) { flags.push("scores as generated"); }
      if (f.looks_like_a_screen_or_print) { flags.push("looks like a screen or print"); }
      if (f.text_aimed_at_the_checker) { flags.push("contains text aimed at the checker"); }
      return '<figure id="ev-' + i + '" class="rv" style="--i:' + Math.min(i, 12) + '"><div class="slot"></div><figcaption>' +
        '<b>' + esc(f.kind || f.mime) + '</b> &middot; <span class="chip ' + (f.verified ? "ok" : "") + '">' +
          (f.verified ? "accepted" : "submitted") + '</span><br>' +
        esc(when(f.captured_at || f.at)) +
        (f.lat != null ? ' &middot; ' + esc(f.lat.toFixed(4)) + ', ' + esc(f.lon.toFixed(4)) : '') +
        (f.challenge_seen ? '<br>code seen in ' + esc(f.challenge_seen) : '') +
        (flags.length ? '<br><span class="flag">' + esc(flags.join("; ")) + '</span>' : '') +
        (f.transcript ? '<br>“' + esc(f.transcript) + '”' : '') +
        '<br>' + esc(String(f.sha256 || "").slice(0, 16)) + '</figcaption></figure>';
    }).join("") + '</div>' +
    '<div style="margin-top:.6rem"><button class="btn sm" data-close="' + esc(job) + '">Close</button></div>';
    wireClose(host);
    files.forEach(function (f, i) {
      var slot = host.querySelector("#ev-" + i + " .slot");
      var mime = f.mime || "";
      if (mime.indexOf("image/") !== 0 && mime.indexOf("video/") !== 0) {
        slot.innerHTML = '<div class="fn">' + esc(mime || "file") + ', ' + esc(f.bytes) + ' bytes</div>';
        return;
      }
      var path = "/v1/jobs/" + encodeURIComponent(job) + "/evidence/" + encodeURIComponent(f.sha256);
      workerHeaders("GET", path).then(function (h) {
        return fetch(path, {headers: h});
      }).then(function (r) {
        if (!r.ok) { throw new Error(r.status === 410 ? "no longer stored; the hash and verdict remain" : "unavailable"); }
        return r.blob();
      }).then(function (b) {
        var url = URL.createObjectURL(b);
        slot.innerHTML = mime.indexOf("video/") === 0
          ? '<video controls playsinline></video>'
          : '<img alt="">';
        slot.firstChild.src = url;
      }).catch(function (e) {
        slot.innerHTML = '<div class="fn">' + esc(e.message) + '</div>';
      });
    });
  }).catch(function (e) { host.innerHTML = '<div class="err">' + esc(e.message) + '</div>'; });
}

function showStatus(job) {
  var host = document.getElementById("jx-" + job);
  host.innerHTML = '<div class="empty" style="padding:1rem">Fetching…</div>';
  api("GET", "/v1/jobs/" + encodeURIComponent(job)).then(function (res) {
    if (!res.ok) { throw new Error(errorOf(res, "could not read the job")); }
    var s = res.body;
    var results = (s.results || []).map(function (r) {
      return '<li><span class="chip ' + (r.verified ? "ok" : "") + '">' + (r.verified ? "accepted" : "checked") +
        '</span>' + esc(when(r.at)) + ' &middot; ' + esc(r.files) + ' file' + (r.files === 1 ? '' : 's') +
        (r.why ? ' &middot; ' + esc(r.why) : '') + '</li>';
    }).join("");
    host.innerHTML = '<div class="receipt rv">' +
      '<div><b>' + esc(s.predicate) + '</b> <span style="color:var(--ink-3)">&middot; ' + esc(s.kind) + '</span></div>' +
      '<div class="fn">' + esc(s.taken || 0) + ' of ' + esc(s.slots || 1) + ' seat' + (s.slots === 1 ? '' : 's') + ' taken &middot; ' +
        esc(s.submissions || 0) + ' submitted &middot; expires ' + esc(when(s.expires)) +
        (s.escrow_minor != null ? ' &middot; ' + money(s.escrow_minor, "USD") + ' in escrow' : '') + '</div>' +
      (results ? '<h4>Results</h4><ul>' + results + '</ul>' : '') +
      '<div class="fn"><code>GET /v1/jobs/' + esc(job) + '</code> &middot; <a href="/j/' + encodeURIComponent(job) + '">public page</a></div>' +
      '<div style="margin-top:.7rem"><button class="btn sm" data-close="' + esc(job) + '">Close</button></div>' +
    '</div>';
    wireClose(host);
  }).catch(function (e) { host.innerHTML = '<div class="err">' + esc(e.message) + '</div>'; });
}

// The sealed offers on an open job, and the button that accepts one. Escrow
// happens at that moment, so the funding note applies here as much as at
// posting.
function showBids(job) {
  var host = document.getElementById("jx-" + job);
  host.innerHTML = '<div class="empty" style="padding:1rem">Fetching offers…</div>';
  api("GET", "/v1/jobs/" + encodeURIComponent(job) + "/bids").then(function (res) {
    if (!res.ok) { throw new Error(errorOf(res, "could not read the offers")); }
    var b = res.body, bids = b.bids || [];
    host.innerHTML = '<div class="receipt rv">' +
      '<div class="fn">Ceiling ' + money(b.ceiling_minor, "USD") + ' &middot; offers close ' + esc(when(b.closes)) +
        (b.awarded ? ' &middot; awarded' : '') + '</div>' +
      (bids.length ? bids.map(function (x) {
        var asm = (x.assumptions || []).map(function (a) {
          return '<div class="fn">' + esc(a.question || a.about || "") + ': ' + esc(a.answer || a.assumes || "") + '</div>';
        }).join("");
        return '<div class="bid"><div><b>' + esc(String(x.worker || "").slice(0, 8)) + '</b>' +
          (x.note ? '<div class="fn">' + esc(x.note) + '</div>' : '') + asm +
          (x.available_from && dateOf(x.available_from) ? '<div class="fn">from ' + esc(when(x.available_from)) + '</div>' : '') +
          '</div><span class="amt">' + money(x.amount_minor, x.currency) + '</span>' +
          (b.awarded ? '' : '<button class="btn sm go" data-award="' + esc(x.id) + '">Accept</button>') + '</div>';
      }).join("") : '<div class="fn" style="margin-top:.5rem">No offers yet.</div>') +
      '<div class="err" id="bid-err-' + esc(job) + '"></div>' +
      '<div style="margin-top:.7rem"><button class="btn sm" data-close="' + esc(job) + '">Close</button></div>' +
    '</div>';
    wireClose(host);
    host.querySelectorAll("[data-award]").forEach(function (btn) {
      btn.addEventListener("click", function () {
        btn.disabled = true;
        api("POST", "/v1/jobs/" + encodeURIComponent(job) + "/award", {bid: btn.dataset.award}).then(function (r2) {
          if (!r2.ok) { throw new Error(errorOf(r2, "could not accept that")); }
          refreshSpending();
        }).catch(function (e) {
          document.getElementById("bid-err-" + job).textContent = e.message;
          btn.disabled = false;
        });
      });
    });
  }).catch(function (e) { host.innerHTML = '<div class="err">' + esc(e.message) + '</div>'; });
}

function wireClose(host) {
  host.querySelectorAll("[data-close]").forEach(function (b) {
    b.addEventListener("click", function () { host.innerHTML = ""; });
  });
}

function wireSpending() {
  document.querySelectorAll("[data-release]").forEach(function (b) {
    b.addEventListener("click", function () {
      reviewAction(this.dataset.release, "release", null);
    });
  });
  document.querySelectorAll("[data-hold]").forEach(function (b) {
    b.addEventListener("click", function () {
      var job = this.dataset.hold;
      var host = document.getElementById("rv-" + job);
      host.innerHTML =
        '<label class="ask" for="hw-' + job + '">What is wrong? A person reads this.</label>' +
        '<textarea id="hw-' + job + '" rows="2" placeholder="The gutter is still full at the north end."></textarea>' +
        '<div class="ask-acts">' +
          '<button class="btn go" id="hs-' + job + '">Hold payment</button>' +
          '<button class="btn" id="hc-' + job + '">Cancel</button>' +
        '</div>';
      document.getElementById("hw-" + job).focus();
      document.getElementById("hc-" + job).onclick = function () { host.innerHTML = ""; };
      document.getElementById("hs-" + job).onclick = function () {
        var why = document.getElementById("hw-" + job).value.trim();
        if (!why) { document.getElementById("hw-" + job).focus(); return; }
        reviewAction(job, "hold", why);
      };
    });
  });
  document.querySelectorAll("[data-evidence]").forEach(function (b) {
    b.addEventListener("click", function () { showEvidence(b.dataset.evidence); });
  });
  document.querySelectorAll("[data-receipt]").forEach(function (b) {
    b.addEventListener("click", function () { showReceipt(b.dataset.receipt); });
  });
  document.querySelectorAll("[data-bids]").forEach(function (b) {
    b.addEventListener("click", function () { showBids(b.dataset.bids); });
  });
  document.querySelectorAll("[data-status]").forEach(function (b) {
    b.addEventListener("click", function () { showStatus(b.dataset.status); });
  });
}

function show(out) {
  if (out) { SPEND = out[0] || null; }
  bodyEl().innerHTML = demoStrip() + buyerHud() + spending();
  wireSpending();
  animateTiles();
}
boot(["/v1/spend"], show);
`

// consoleKeysJS: the credentials that spend the balance, the limits on each,
// and the one line that connects an agent.
const consoleKeysJS = `
var NEW_KEY = "";

// The connect line for whatever exchange this page is on, with the key just
// issued in it while there is one: the only moment the key is visible is the
// moment somebody most wants to paste it.
function mcpLine(key) {
  return 'claude mcp add --transport http lamdis ' + location.origin + '/mcp \\\n' +
    '  --header "Authorization: Bearer ' + (key || "lam_…") + '"';
}

function keysPane() {
  var live = KEYS.filter(function (k) { return !k.revoked; }).length;
  var rows = KEYS.length ? KEYS.map(function (k, i) {
    return '<div' + rvi(i, "r") + '><div class="grow">' +
      '<div class="t"><span class="chip ' + (k.revoked ? "bad" : "ok") + '">' +
        (k.revoked ? "Revoked" : "Active") + '</span>' + esc(k.label || "agent") + '</div>' +
      '<div class="m">&bull;&bull;&bull;&bull;' + esc(k.last4) +
        (k.last_used ? ' &middot; last used ' + esc(when(k.last_used)) : ' &middot; never used') +
        (k.max_per_job_minor ? ' &middot; up to ' + money(k.max_per_job_minor) + ' a job' : '') + '</div>' +
      '</div>' +
      (k.revoked ? '' : '<button class="btn sm" data-revoke="' + esc(k.id) + '">Revoke</button>') +
    '</div>';
  }).join("") : '<div class="empty">No keys yet. Issue one so an agent can buy on your behalf.</div>';

  return head('<h2 id="keys">Agent keys</h2>', live, "keys spend your balance inside limits you set here") +
  '<p class="lead">Your account id is <code>' + esc(ME.worker) + '</code>.</p>' +
  '<div' + rv("rows") + '>' + rows + '</div>' +
  '<div id="new-key">' + NEW_KEY + '</div>' +
  '<div class="panes" style="margin-top:.7rem">' +
    '<div' + rv("pane") + '>' +
      '<h3>Issue a key</h3>' +
      '<p class="why">Shown once. Nothing here can show it to you again.</p>' +
      '<input type="text" id="k-label" placeholder="What is it for? e.g. dispatch bot" maxlength="60">' +
      '<div class="ctl" style="margin-top:.7rem">' +
        '<input type="range" min="10" max="1000" step="10" value="100" id="k-cap" aria-label="Per-job cap">' +
        '<span class="v" id="k-cap-v">$100</span>' +
      '</div>' +
      '<p class="why" style="margin:.2rem 0 .8rem">Most it may commit to any one job.</p>' +
      '<button class="btn go" id="k-make">Issue key</button>' +
      '<div class="err" id="k-err"></div>' +
    '</div>' +
    '<div' + rv("pane") + '>' +
      '<h3>What a key can do</h3>' +
      '<p class="why">Post jobs, read their status, evidence and receipts, and accept ' +
        'offers &mdash; all against your balance, never above the per-job cap. It cannot ' +
        'add funds, issue other keys, or take work. Full reference in ' +
        '<a href="/docs">the docs</a>.</p>' +
      '<h3 style="margin-top:.6rem">Webhook</h3>' +
      '<p class="why">A key does not call you back. Your agent polls <code>GET /v1/jobs/{job}</code>, ' +
        'or reads the signed receipt once the job is done.</p>' +
    '</div>' +
  '</div>' +
  '<h2 id="integration">Connect an agent</h2>' +
  '<p class="why" style="margin:0 0 .6rem;font-size:.84rem;color:var(--ink-2)">One line. Any MCP client works; the flag is Claude Code&rsquo;s.</p>' +
  '<pre class="api" id="mcp-line">' + esc(mcpLine(NEW_KEY_RAW)) + '</pre>';
}
var NEW_KEY_RAW = "";

function wireKeys() {
  var cap = document.getElementById("k-cap");
  if (cap) {
    cap.addEventListener("input", function () {
      document.getElementById("k-cap-v").textContent = "$" + this.value;
    });
  }
  var make = document.getElementById("k-make");
  if (make) { make.addEventListener("click", issueKey); }
  document.querySelectorAll("[data-revoke]").forEach(function (b) {
    b.addEventListener("click", function () { revokeKey(b, b.dataset.revoke); });
  });
}

function issueKey() {
  var btn = document.getElementById("k-make");
  var err = document.getElementById("k-err");
  var label = document.getElementById("k-label").value.trim() || "agent";
  var perJob = parseInt(document.getElementById("k-cap").value, 10) * 100;
  btn.disabled = true; btn.textContent = "Issuing…"; err.textContent = "";
  api("POST", "/v1/agent-keys", {label: label, max_per_job_minor: perJob}).then(function (res) {
    if (!res.ok) { throw new Error(errorOf(res, "could not issue a key")); }
    // Kept in a variable rather than only in the DOM: the key list re-renders
    // after issuing, and the one-time reveal must survive that.
    NEW_KEY_RAW = res.body.key;
    NEW_KEY = '<div class="reveal rv"><span class="label">Copy this now</span>' +
      '<span class="k">' + esc(res.body.key) + '</span>' +
      '<p>It will not be shown again. Anything using it spends your balance, up to $' +
      (perJob / 100) + ' a job. The connect line below has it filled in.</p></div>';
    reload();
  }).catch(function (e) {
    err.textContent = e.message;
  }).then(function () {
    btn.disabled = false; btn.textContent = "Issue key";
  });
}

function revokeKey(btn, id) {
  btn.disabled = true; btn.textContent = "Revoking…";
  api("DELETE", "/v1/agent-keys/" + encodeURIComponent(id))
    .then(function () { reload(); })
    .catch(function () { btn.disabled = false; btn.textContent = "Revoke"; });
}

function show(out) {
  if (out) { KEYS = (out[0] && out[0].keys) || []; }
  bodyEl().innerHTML = demoStrip() + keysPane();
  wireKeys();
}
boot(["/v1/agent-keys"], show);
`

// consoleFundsJS: the balance, and adding to it. That happens on the
// provider's hosted page: the exchange never sees a card number, and the
// balance moves only once they confirm the payment.
const consoleFundsJS = `
// Back from the provider. A person who lands on the success page has not
// necessarily paid, so the balance is credited only after the provider
// confirms it; the query is dropped so a reload does not confirm twice.
var Q = new URLSearchParams(location.search);
var TOPUP = Q.get("topup") || "", TOPUP_SESSION = Q.get("session") || "";
if (TOPUP) { history.replaceState(null, "", location.pathname); }

function confirmTopup(sess) {
  return api("POST", "/v1/balance/confirm?session=" + encodeURIComponent(sess))
    .then(function () { reload(); }).catch(function () { reload(); });
}

function fundsHud() {
  var sp = SPEND || {}, cur = sp.currency || "USD";
  var avail = (sp.balance_minor || 0) - (sp.held_minor || 0);
  return '<div class="hud" style="--cols:3">' +
    tile("money", "Balance", sp.balance_minor || 0, "money", "on the account", cur) +
    tile("", "Held for open jobs", sp.held_minor || 0, "money", '<a href="/console/spending">back if nobody takes them</a>', cur) +
    tile(avail > 0 ? "ok" : "wait", "Available to post", avail, "money", "what a new job can hold", cur) +
  '</div>';
}

function fundsPane() {
  var sp = SPEND || {}, cur = sp.currency || "USD";
  var note = TOPUP === "cancelled"
    ? '<div' + rv("strip warn") + '><span class="d"></span><span>Payment cancelled. Nothing was charged.</span></div>'
    : (TOPUP === "done"
      ? '<div' + rv("strip") + '><span class="d"></span><span>Payment received. The balance above includes it once the provider has confirmed.</span></div>'
      : '');
  return head('<h2 id="funds">Add funds</h2>', null, money(sp.balance_minor || 0, cur) + " on the account") + note +
    '<div class="panes"><div' + rv("pane") + '>' +
      '<div class="ctl" style="margin:0;gap:.5rem">' +
        '<span class="cur">$</span>' +
        '<input type="number" id="topup-amt" value="50" min="1" step="1" ' +
          'aria-label="Amount to add" class="amt-in">' +
        '<button class="btn go" id="s-topup">Add funds</button></div>' +
      '<div class="err" id="topup-err"></div>' +
      '<p class="why" style="margin:.4rem 0 0">Paid on Stripe’s own page; the balance moves once they confirm it. ' +
        'It is held when you post and paid when proof is accepted; if nobody takes a job, it comes back.</p>' +
    '</div>' +
    '<div' + rv("pane") + '>' +
      '<h3>Where it goes</h3>' +
      '<p class="why">Posting a job holds its price in escrow. Accepting the proof pays the operator; ' +
        'a hold keeps it until a person has looked; a job nobody takes returns it. ' +
        'Every movement is on <a href="/console/spending">Spending</a>.</p>' +
    '</div></div>';
}

function addFunds(btn) {
  var field = document.getElementById("topup-amt");
  if (!field) { return; }
  var minor = Math.round(parseFloat(field.value) * 100);
  if (!(minor > 0)) { field.focus(); return; }
  btn.disabled = true;
  btn.textContent = "Opening…";
  var slow = setTimeout(function () {
    btn.textContent = "Reaching Stripe…";
  }, 1200);
  api("POST", "/v1/balance/topup", {amount_minor: minor, currency: "USD"}).then(function (res) {
    if (!res.ok || !res.body.pay_at) {
      throw new Error(errorOf(res, "could not start payment"));
    }
    clearTimeout(slow);
    location.href = res.body.pay_at;
  }).catch(function (e) {
    clearTimeout(slow);
    btn.disabled = false;
    btn.textContent = "Add funds";
    // Shown in the page rather than an alert: a modal steals focus and says
    // nothing about where the failure was.
    var host = document.getElementById("topup-err");
    if (host) { host.textContent = e.message; }
  });
}

function show(out) {
  if (out) { SPEND = out[0] || null; }
  bodyEl().innerHTML = demoStrip() + fundsHud() + fundsPane();
  var st = document.getElementById("s-topup");
  if (st) { st.addEventListener("click", function () { addFunds(this); }); }
  animateTiles();
  if (TOPUP === "done" && TOPUP_SESSION && !DEMO) {
    var s = TOPUP_SESSION; TOPUP_SESSION = "";
    confirmTopup(s);
  }
}
boot(["/v1/spend"], show);
`

// --- assembly ----------------------------------------------------------------

// consolePageSpec is one route under /console.
type consolePageSpec struct {
	// ID is the path segment after /console; "" is the overview.
	ID string
	// Side is which half of the exchange the page belongs to.
	Side string
	// Nav is the rail label; Title and Lead head the page.
	Nav, Title, Lead string
	// Panel is whether the page needs panelJS (tiles, the radar).
	Panel bool
	// Scripts are the page's own script pieces, after consoleJS.
	Scripts []string
}

// consolePageSpecs is every console page, in rail order. The rail in theme.go
// is built from it, so a page cannot exist without a link or a link without
// a page.
var consolePageSpecs = []consolePageSpec{
	{ID: "", Side: "operator", Nav: "Overview", Title: "Your account",
		Lead:  "What is clear to send, what is held, and what you hold.",
		Panel: true, Scripts: []string{consoleTilesJS, consoleFlightJS, consoleOverviewJS}},
	{ID: "work", Side: "operator", Nav: "In flight", Title: "In flight",
		Lead:    "Everything you hold, the stage each is at, and the way to hand one back.",
		Scripts: []string{consoleFlightJS, consoleWorkJS}},
	{ID: "earnings", Side: "operator", Nav: "Earnings", Title: "Earnings",
		Lead:  "What you have earned, what is stopping it reaching you, and the statement your bookkeeper wants.",
		Panel: true, Scripts: []string{consoleTilesJS, consoleEarningsJS}},
	{ID: "capacity", Side: "operator", Nav: "Capacity", Title: "Capacity",
		Lead:  "How much you will hold, how far you will go, and what kinds of work reach you.",
		Panel: true, Scripts: []string{consoleTilesJS, consoleCapJS, consoleCapacityJS}},
	{ID: "business", Side: "operator", Nav: "Business", Title: "Business",
		Lead:    "If you work as a company, say so: the name buyers see, the licences, the cover, the crew.",
		Scripts: []string{consoleBusinessJS}},
	{ID: "alerts", Side: "operator", Nav: "Alerts", Title: "Alerts",
		Lead:    "What the exchange will email you about, on either side.",
		Scripts: []string{consoleAlertsJS}},
	{ID: "dispatch", Side: "operator", Nav: "Dispatch", Title: "Dispatch endpoint",
		Lead:    "For fleets that take work over the API: where offers are sent, and how to tell ours from anyone else's.",
		Scripts: []string{consoleCapJS, consoleDispatchJS}},
	{ID: "larger", Side: "operator", Nav: "Larger jobs", Title: "Larger jobs",
		Lead:    "More than one visit, place or trade: how a scope is priced, ordered and paid.",
		Scripts: []string{consoleLargerJS}},
	{ID: "buy", Side: "buyer", Nav: "Post a job", Title: "Post a job",
		Lead:  "Ask somebody to find out whether something is true, or to make it true. Beside the form, the row the board will show.",
		Panel: true, Scripts: []string{consoleTilesJS, consoleBuyerHudJS, consoleBuyJS}},
	{ID: "spending", Side: "buyer", Nav: "Spending", Title: "Spending",
		Lead:  "What your money is doing, what came back, and the two buttons that decide whether it is paid.",
		Panel: true, Scripts: []string{consoleTilesJS, consoleBuyerHudJS, consoleSpendingJS}},
	{ID: "keys", Side: "buyer", Nav: "Agent keys", Title: "Agent keys",
		Lead:    "The credentials that spend your balance, the limit on each, and the one line that connects an agent.",
		Scripts: []string{consoleKeysJS}},
	{ID: "funds", Side: "buyer", Nav: "Add funds", Title: "Add funds",
		Lead:  "The balance, and adding to it. Paid on the provider's page; we never see a card number.",
		Panel: true, Scripts: []string{consoleTilesJS, consoleFundsJS}},
}

func (p consolePageSpec) path() string {
	if p.ID == "" {
		return "/console"
	}
	return "/console/" + p.ID
}

// railID is the shellTop current-page id for this page.
func (p consolePageSpec) railID() string {
	if p.ID == "" {
		return "console"
	}
	return "console-" + p.ID
}

const consoleHead = `<!doctype html>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>`

// buildConsolePage assembles one route: the shared shell with this page
// current, the header with the side switch set to this page's side, and only
// the script this page needs.
func buildConsolePage(p consolePageSpec) string {
	eyebrow, opOn, buyOn := "Operator", "true", "false"
	if p.Side == "buyer" {
		eyebrow, opOn, buyOn = "Buyer", "false", "true"
	}
	script := workerJS
	if p.Panel {
		script += panelJS
	}
	script += "\n" + `var PAGE = "` + p.ID + `", SIDE = "` + p.Side + `";` + "\n" + consoleJS
	for _, s := range p.Scripts {
		script += s
	}
	return consoleHead + p.Title + ` — Lamdis</title>
<style>` + themeCSS + consoleCSS + `</style>
` + shellTop(p.railID(), "") + `
    <div class="cockpit">
      <div>
        <p class="eyebrow` + map[bool]string{true: " blue", false: ""}[p.Side == "buyer"] + `">` + eyebrow + `</p>
        <h1>` + p.Title + `</h1>
        <p class="lead" id="lead">` + p.Lead + `</p>
      </div>
      <div class="cockpit-r">
        <span class="pill" id="pill-session"><span class="beacon off"></span><span id="pill-session-t">Not signed in</span></span>
        <span class="pill" id="pill-payout" hidden><span class="beacon off"></span><span id="pill-payout-t">Payouts</span></span>
        <div class="mode" role="group" aria-label="Which side of the exchange">
          <button id="mode-operator" aria-pressed="` + opOn + `">Operator</button>
          <button id="mode-buyer" aria-pressed="` + buyOn + `">Buyer</button>
        </div>
      </div>
    </div>
    <div id="body"><div class="empty">Loading&hellip;</div></div>
` + shellBottom + `
<script>
` + script + `
</script>
`
}

// consolePages is every console route, assembled once, keyed by path segment.
var consolePages = func() map[string]string {
	m := map[string]string{}
	for _, p := range consolePageSpecs {
		m[p.ID] = buildConsolePage(p)
	}
	return m
}()

// consolePageHTML is the overview, for the tests that look at one console
// page as representative of the set.
var consolePageHTML = consolePages[""]

// consoleNotFoundHTML is what a /console/… that does not exist gets: the
// shell, with every page that does, rather than a bare 404 that reads as
// the whole console being broken.
var consoleNotFoundHTML = func() string {
	links := ""
	for _, p := range consolePageSpecs {
		links += `<li><a href="` + p.path() + `">` + p.Nav + `</a> <span class="label">` + p.Side + `</span></li>`
	}
	return consoleHead + `No such page — Lamdis</title>
<style>` + themeCSS + consoleCSS + `</style>
` + shellTop("", "") + `
    <div class="nf">
      <p class="eyebrow dim">Console</p>
      <h1>No such page</h1>
      <p class="lead">Nothing lives here. These do:</p>
      <ul>` + links + `</ul>
    </div>
` + shellBottom + "\n"
}()
