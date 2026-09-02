package api

import "html"

// Pages for a poster who has no account: the public post form, the page a
// buyer follows their job from with only a token, and the small notices the
// payment return can land on. Same instrument panel as everything else.

// GuestNotice is a one-card page with a title and a sentence.
func GuestNotice(title, body string) string {
	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>` +
		html.EscapeString(title) + ` — Lamdis</title><style>` + themeCSS + `
.one { min-height: 100vh; display: grid; place-items: center; padding: 2rem; }
.one .glass { max-width: 30rem; }
.one h1 { margin-bottom: .6rem; }
.one p { color: var(--ink-2); margin: 0 0 1.2rem; }
</style></head><body><div class="one"><div class="glass">
<a class="mark" href="/board">lamdis<b>.</b></a>
<h1 style="margin-top:1rem">` + html.EscapeString(title) + `</h1>
<p>` + html.EscapeString(body) + `</p>
<a class="btn" href="/board">Open the board</a>
</div></div></body></html>`
}

// MyJobPage follows one job with a token carried in the URL.
func MyJobPage(job string) string {
	j := html.EscapeString(job)
	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="referrer" content="no-referrer"><title>Job ` + j + ` — Lamdis</title><style>` + themeCSS + `
.wrap { max-width: 52rem; margin: 0 auto; padding: 2rem 1.25rem 4rem; }
.hd { display: flex; align-items: center; gap: 1rem; margin-bottom: 1.4rem; }
.hd .mark { margin-right: auto; }
.res { margin-top: 1rem; }
.res .r { border: 1px solid var(--rule); border-radius: 6px; margin-bottom: .6rem; align-items: flex-start; }
.res .r .m { white-space: pre-wrap; }
.keep { margin-top: 1.4rem; font: 500 .74rem/1.5 var(--mono); color: var(--ink-3); }
img.ev { max-width: 100%; border-radius: 6px; border: 1px solid var(--rule-2); margin-top: .5rem; }
</style></head><body><div class="wrap">
<div class="hd"><a class="mark" href="/board">lamdis<b>.</b></a><span class="pill"><span class="beacon" id="beacon"></span><span id="state">Loading</span></span></div>
<p class="eyebrow" style="margin:0 0 .4rem;font:600 .62rem/1 var(--mono);letter-spacing:.15em;text-transform:uppercase;color:var(--gold)">Your job</p>
<h1 id="title">` + j + `</h1>
<p class="lead" id="lead">Reading the job with the token in this link.</p>
<div class="hud" style="--cols:4" id="hud"></div>
<div class="glass" id="body"><div class="empty">Nothing yet.</div></div>
<div class="res" id="res"></div>
<p class="keep">This link carries the only key to this job. Anyone with it can read the receipt and release or object to payment. Keep it.</p>
</div>
<script>
(function () {
  var job = ` + "`" + j + "`" + `;
  var t = new URLSearchParams(location.search).get("t") || "";
  var H = { "Authorization": "Bearer " + t };
  function esc(s) { return String(s == null ? "" : s).replace(/[&<>"]/g, function (c) { return {"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;"}[c]; }); }
  function money(m, c) { return (c || "USD") === "USD" ? "$" + (m / 100).toFixed(2) : (m / 100).toFixed(2) + " " + c; }
  function tile(k, v, s, cls) { return '<div class="t ' + (cls || "") + '"><div class="k">' + esc(k) + '</div><div class="v">' + esc(v) + '</div><div class="s">' + esc(s || "") + '</div></div>'; }
  function set(id, html) { document.getElementById(id).innerHTML = html; }
  function state(txt, on) { document.getElementById("state").textContent = txt; document.getElementById("beacon").className = "beacon" + (on ? "" : " off"); }
  function load() {
    fetch("/v1/jobs/" + encodeURIComponent(job), { headers: H }).then(function (r) { return r.json().then(function (j) { return { ok: r.ok, j: j }; }); }).then(function (res) {
      var j = res.j;
      if (!res.ok) { state("Not readable", false); set("lead", "<span class=\"err\">" + esc(j.error || "This link does not open this job.") + "</span>"); return; }
      if (j.status === "awaiting_payment") {
        state("Awaiting payment", false);
        set("lead", "The job is saved but not on the board. It goes live the moment the card is authorised.");
        set("hud", tile("Ceiling", money(j.amount_minor, j.currency), "authorised, not charged", "money") + tile("Expires", (j.expires_at || "").slice(0, 16).replace("T", " "), "if unpaid"));
        set("body", '<a class="btn go" href="' + esc(j.pay_at) + '">Authorise the card</a>');
        return;
      }
      state(j.taken > 0 ? "Taken" : "On the board", true);
      document.getElementById("title").textContent = j.predicate || job;
      set("lead", (j.kind === "observe" ? "Find out: paid for going, whichever way the answer turns out." : "Make it true: paid when the proof is accepted."));
      set("hud", tile("Escrow", money(j.escrow_minor || 0), "held for this job", "money") + tile("Taken", (j.taken || 0) + " / " + (j.slots || 1), "seats") + tile("Submissions", j.submissions || 0, "photos back", (j.submissions ? "ok" : "")) + tile("Expires", (j.expires || "").slice(0, 16).replace("T", " "), ""));
      var rows = (j.results || []).map(function (r) {
        return '<div class="r"><div class="grow"><div class="t">' + (r.verified ? '<span class="chip ok">passed</span>' : '<span class="chip warn">not accepted</span>') + esc(r.at) + '</div><div class="m">' + esc(r.why || (r.verified ? "Proof accepted; payment settles on the release window." : "")) + (r.transcript ? "\n" + esc(r.transcript) : "") + '</div></div></div>';
      }).join("");
      set("res", rows);
      set("body", '<a class="btn" href="/v1/jobs/' + encodeURIComponent(job) + '/receipt?t=' + encodeURIComponent(t) + '">Receipt</a> <a class="btn" href="/v1/jobs/' + encodeURIComponent(job) + '/evidence?t=' + encodeURIComponent(t) + '">Evidence</a> <a class="btn" href="/j/' + encodeURIComponent(job) + '">Public listing</a>');
    }).catch(function () { state("Offline", false); });
  }
  load(); setInterval(function () { if (!document.hidden) { load(); } }, 20000);
})();
</script></body></html>`
}

// PostPage is the public form: post a job, get a pay link. No account.
func PostPage() string {
	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Post a job — Lamdis</title><style>` + themeCSS + `
.wrap { max-width: 44rem; margin: 0 auto; padding: 2rem 1.25rem 4rem; }
.hd { display: flex; align-items: center; gap: 1rem; margin-bottom: 1.4rem; }
.hd .mark { margin-right: auto; }
label { display: block; margin: .9rem 0 .3rem; font: 600 .62rem/1 var(--mono); letter-spacing: .15em; text-transform: uppercase; color: var(--ink-3); }
.two { display: grid; grid-template-columns: 1fr 1fr; gap: .8rem; }
.kinds { display: flex; gap: .5rem; }
.kinds button { flex: 1; padding: .8rem; border-radius: 6px; border: 1px solid var(--rule-2); background: none; color: var(--ink-2); cursor: pointer; text-align: left; }
.kinds button[aria-pressed="true"] { border-color: var(--gold); color: var(--ink); }
.kinds b { display: block; font-size: .95rem; }
.kinds span { font: 400 .78rem/1.4 var(--sans); color: var(--ink-3); }
input[type=number] { width: 100%; padding: .6rem .75rem; border: 1px solid var(--rule-2); border-radius: 3px; background: var(--panel); color: var(--ink); }
.foot { display: flex; align-items: center; gap: 1rem; margin-top: 1.4rem; }
.foot .fine { font: 500 .74rem/1.5 var(--mono); color: var(--ink-3); }
@media (max-width: 40rem) { .two { grid-template-columns: 1fr; } }
</style></head><body><div class="wrap">
<div class="hd"><a class="mark" href="/board">lamdis<b>.</b></a><a class="btn sm" href="/signin">Sign in</a></div>
<p style="margin:0 0 .4rem;font:600 .62rem/1 var(--mono);letter-spacing:.15em;text-transform:uppercase;color:var(--gold)">No account needed</p>
<h1>Post a job in the world.</h1>
<p class="lead">Say what should be true and what proves it. You get a link to authorise a card for the ceiling; nothing is charged until the proof is accepted.</p>
<form class="glass" id="f">
  <div class="kinds">
    <button type="button" data-kind="observe" aria-pressed="true"><b>Find out</b><span>Someone goes and looks. Paid for going, whatever they find.</span></button>
    <button type="button" data-kind="do" aria-pressed="false"><b>Make it true</b><span>Someone does it. Paid when the proof is accepted.</span></button>
  </div>
  <label for="predicate">What should be true</label>
  <input type="text" id="predicate" placeholder="the FOR LEASE sign is up at the front of the building" required>
  <div id="instr-row" hidden><label for="instructions">What to do</label><textarea id="instructions" rows="2" placeholder="Hang the sign on the front railing, centred, and photograph it."></textarea></div>
  <label for="deliverable">What proves it</label>
  <input type="text" id="deliverable" placeholder="one photo with the sign, the house number and the code in frame">
  <div class="two">
    <div><label for="where">Exact address (only whoever takes it sees this)</label><input type="text" id="where" placeholder="742 Evergreen Rd, Detroit, MI"></div>
    <div><label for="area">Area shown on the board</label><input type="text" id="area" placeholder="Detroit, MI"></div>
  </div>
  <div class="two">
    <div><label for="lat">Latitude</label><input type="number" id="lat" step="0.000001" placeholder="42.3314"></div>
    <div><label for="lon">Longitude</label><input type="number" id="lon" step="0.000001" placeholder="-83.0458"></div>
  </div>
  <div class="two">
    <div><label for="fee">Pay ($)</label><input type="number" id="fee" min="1" step="1" value="25"></div>
    <div id="attempt-row" hidden><label for="attempt">Wasted-trip pay ($, optional)</label><input type="number" id="attempt" min="0" step="1" value="0"></div>
  </div>
  <div class="foot"><button class="btn go" type="submit" id="go">Get the pay link</button><span class="fine" id="msg">Ceiling is pay plus wasted-trip pay. Authorised, not charged.</span></div>
  <div class="err" id="err"></div>
</form>
</div>
<script>
(function () {
  var kind = "observe";
  var ks = document.querySelectorAll(".kinds button");
  ks.forEach(function (b) { b.addEventListener("click", function () { kind = b.dataset.kind; ks.forEach(function (o) { o.setAttribute("aria-pressed", String(o === b)); }); document.getElementById("instr-row").hidden = kind !== "do";
    // An observation pays in full for going, so it has no attempt fee and
    // the exchange refuses one.
    document.getElementById("attempt-row").hidden = kind !== "do"; }); });
  function v(id) { return document.getElementById(id).value.trim(); }
  document.getElementById("f").addEventListener("submit", function (e) {
    e.preventDefault();
    var go = document.getElementById("go"), err = document.getElementById("err");
    err.textContent = ""; go.disabled = true; go.textContent = "Saving…";
    var lat = parseFloat(v("lat")), lon = parseFloat(v("lon"));
    var body = { kind: kind, predicate: v("predicate"), instructions: v("instructions"), deliverable: v("deliverable"), where: v("where"), area: v("area"),
      fee_minor: Math.round(parseFloat(v("fee") || "0") * 100) };
    if (kind === "do") { body.attempt_minor = Math.round(parseFloat(v("attempt") || "0") * 100); }
    if (isFinite(lat) && isFinite(lon)) { body.lat = lat; body.lon = lon; body.radius_m = 150; }
    fetch("/v1/tasks", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) })
      .then(function (r) { return r.json().then(function (j) { return { ok: r.ok, j: j }; }); })
      .then(function (res) {
        if (!res.ok || !res.j.pay_at) { throw new Error(res.j.error || "could not save the job"); }
        location.href = res.j.watch || res.j.pay_at;
      }).catch(function (e) { err.textContent = e.message; go.disabled = false; go.textContent = "Get the pay link"; });
  });
})();
</script></body></html>`
}
