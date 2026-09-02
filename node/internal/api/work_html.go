package api

// workPageHTML is what a person sees after claiming a task.
//
// It has to work on a phone standing in a street, so it does the minimum: what
// to look at, the code to get in frame, and one button that opens the camera.
// The file input is a plain multipart upload rather than anything clever,
// because the original bytes are the evidence — a canvas re-encode would strip
// the EXIF that lets the verifier tell a photograph taken here today from one
// taken somewhere else last year.
//
// The layout is the instrument panel the rest of the exchange uses: glass over
// the gridded ground, mono eyebrows, the pay in gold. The one thing on this
// page that must not be missed — the challenge code — is set huge, in mono,
// with a gold glow, because it is the thing the photograph has to contain.
const workPageHTML = `<!doctype html>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Lamdis — your task</title>
<style>` + themeCSS + `
main { padding: 1.2rem 1rem 4rem; max-width: 40rem; margin: 0 auto; }
@media (min-width: 40rem) { main { padding: 2rem 1.25rem 5rem; } }
.top .back { font-size: .84rem; color: var(--ink-2); text-decoration: none; }
.top .back:hover { color: var(--ink); }
.eyebrow { display: block; font: 600 .62rem/1 var(--mono); letter-spacing: .18em;
  text-transform: uppercase; color: var(--ink-3); }
.eyebrow.gold { color: var(--gold); }
.hdr { margin: 0 0 1.1rem; }
.hdr h1 { font-size: 1.6rem; margin: .55rem 0 .3rem; }
.hdr .lead { margin: 0; font-size: .95rem; }
.hdr .lead b { color: var(--ink); font-weight: 600; }
.hud { --cols: 2; margin-bottom: 1rem; }
.hud .v.txt { font: 600 .98rem/1.3 var(--sans); letter-spacing: 0; color: var(--ink); }
/* The code. Set as large as the viewport allows, glowing, on its own panel:
   a person glancing at a phone in a street must be able to copy it onto a
   piece of paper without zooming in. */
.code-card { position: relative; overflow: hidden; margin: 0 0 1rem;
  padding: 1.3rem 1rem 1.25rem; text-align: center;
  border: 1px solid rgba(255,182,39,.38); border-radius: 8px; background: var(--glass);
  -webkit-backdrop-filter: blur(12px); backdrop-filter: blur(12px);
  box-shadow: inset 0 1px 0 rgba(255,255,255,.04), 0 0 48px rgba(255,182,39,.10); }
.code-card::after { content: ""; position: absolute; left: 0; right: 0; top: 0; height: 2px;
  background: linear-gradient(90deg, var(--gold), transparent); }
.code-card .big { display: block; margin: .75rem 0 .65rem;
  font: 700 clamp(2.6rem, 11vw, 4.4rem)/1 var(--mono); letter-spacing: .2em; text-indent: .2em;
  color: var(--gold); font-variant-numeric: tabular-nums;
  text-shadow: 0 0 18px rgba(255,182,39,.55), 0 0 56px rgba(255,182,39,.22); }
.code-card p { margin: 0; font-size: .84rem; color: var(--ink-2); }
/* The drop zone: one obvious target, gold when it is ready to take a file. */
.drop { display: grid; place-items: center; gap: .3rem; min-height: 8.5rem;
  padding: 1.2rem; cursor: pointer; text-align: center;
  border: 1px dashed var(--rule-2); border-radius: 8px; background: var(--glass);
  -webkit-backdrop-filter: blur(12px); backdrop-filter: blur(12px); color: var(--ink-2);
  transition: border-color .15s, box-shadow .15s; }
.drop:hover, .drop:focus-within, .drop.over { border-color: var(--gold);
  box-shadow: 0 0 0 1px rgba(255,182,39,.25), 0 0 28px rgba(255,182,39,.12); }
.drop svg { width: 2rem; height: 2rem; stroke: var(--ink-3); fill: none; stroke-width: 1.5;
  stroke-linecap: round; stroke-linejoin: round; margin-bottom: .2rem; }
.drop:hover svg, .drop.over svg { stroke: var(--gold); }
.drop .big { font-weight: 600; color: var(--ink); font-size: 1rem; }
.drop .sm { font: 500 .68rem/1.4 var(--mono); letter-spacing: .1em; text-transform: uppercase;
  color: var(--ink-3); }
input[type=file] { position: absolute; width: 1px; height: 1px; opacity: 0; }
/* The preview is bounded in both directions.
   It had width:100% and no height rule at all, so a photograph straight off a
   phone — 3024 x 4032, which is most of them — rendered about 45rem tall and
   pushed the submit button and everything below it off the screen. The person
   had done the work, taken the photo, and could no longer reach the button
   that pays them.
   object-fit keeps the aspect ratio inside the box, and max-width rather than
   width stops a small image being blown up into a soft mess. */
.shot { display: none; margin-bottom: .8rem; padding: .6rem; border: 1px solid var(--rule);
  border-radius: 8px; background: var(--glass);
  -webkit-backdrop-filter: blur(12px); backdrop-filter: blur(12px); }
.shot.on { display: block; }
img#preview { display: block; max-width: 100%; max-height: 42vh; height: auto;
  margin: 0 auto; border-radius: 4px; border: 1px solid var(--rule);
  background: var(--bg); object-fit: contain; }
.shot .cap { margin: .5rem 0 0; font: 500 .7rem/1.3 var(--mono); letter-spacing: .08em;
  text-transform: uppercase; color: var(--ink-3); text-align: center; }
.shot .cap b { color: var(--blue); font-weight: 500; }
.send { margin: .8rem 0 0; }
.send .btn.go { height: 2.7rem; font-size: .95rem; }
.next { color: var(--gold); font-weight: 600; }
.ask { display: block; margin: .1rem 0 .35rem; font: 600 .82rem/1.3 var(--sans); }
textarea { width: 100%; box-sizing: border-box; padding: .55rem .6rem;
  border: 1px solid var(--rule-2); border-radius: 4px; background: var(--bg);
  color: var(--ink); font: inherit; font-size: .9rem; resize: vertical; }
textarea:focus-visible { outline: 2px solid var(--gold); outline-offset: 1px; }
.ask-acts { display: flex; gap: .5rem; margin-top: .5rem; }
/* The report: a small table the job wants filled in, one card per row. */
.report { margin: 0 0 1rem; }
.report .row { padding: .8rem .9rem .9rem; margin: 0 0 .6rem; border: 1px solid var(--rule);
  border-radius: 8px; background: var(--glass); }
.report .row .rn { font: 600 .62rem/1 var(--mono); letter-spacing: .18em; text-transform: uppercase;
  color: var(--ink-3); margin-bottom: .5rem; display: flex; justify-content: space-between; }
.report .row .rn button { font: inherit; letter-spacing: inherit; background: none; border: 0;
  color: var(--ink-3); cursor: pointer; padding: 0; }
.report label { display: block; margin: .35rem 0 .2rem; font: 600 .82rem/1.3 var(--sans); }
.report label small { font-weight: 400; color: var(--ink-3); }
.report input, .report select { width: 100%; box-sizing: border-box; padding: .5rem .6rem;
  border: 1px solid var(--rule-2); border-radius: 4px; background: var(--bg); color: var(--ink);
  font: inherit; font-size: .9rem; }
.report input:focus-visible { outline: 2px solid var(--gold); outline-offset: 1px; }
.report .add { margin-top: .2rem; }
/* Stage progress as what it is: money-weighted, not a schedule. */
.stage { margin: 1rem 0 0; position: relative; overflow: hidden; }
.stage::after { content: ""; position: absolute; left: 0; right: 0; top: 0; height: 2px;
  background: linear-gradient(90deg, var(--gold), transparent); }
.stage h2 { margin: .35rem 0 .3rem; font: 700 1.1rem/1.25 var(--sans); letter-spacing: -.02em;
  text-transform: none; color: var(--ink); }
.stage .stagebar { margin-top: .75rem; }
.stage .stagebar .seg { flex: var(--w, 1) 1 0; height: 6px; }
.stage .note { margin: .6rem 0 0; }
.attempt { margin: 1rem 0 0; border-style: dashed; }
.attempt .eyebrow { margin-bottom: .6rem; }
/* The outcome, as one HUD tile: green for accepted, amber for refused, gold
   while the money is still being decided. */
.outcome { --cols: 1; margin: 0 0 1rem; }
.outcome .t { padding: 1.4rem 1.3rem 1.3rem; }
.outcome .v { font-size: 2.1rem; }
.outcome .lead { margin: .7rem 0 1rem; font-size: .93rem; }
.outcome .acts { display: flex; gap: .6rem; flex-wrap: wrap; }
.sha { margin-top: .9rem; font: 400 .66rem/1.5 var(--mono); color: var(--ink-3);
  word-break: break-all; letter-spacing: .04em; }
.sha b { color: var(--ink-3); font-weight: 500; letter-spacing: .12em; text-transform: uppercase; }
.fine { margin: .9rem 0 0; font-size: .82rem; color: var(--ink-3); }
.fine b { color: var(--ink-2); }
</style>
<header class="top">
  <a class="mark" href="/board">lamdis<b>.</b></a>
  <span class="pill" id="pill"><span class="beacon"></span>Work order</span>
  <div class="right"><a class="back" href="/how-it-works">How this works</a>
    <a class="back" href="/board">&larr; Board</a></div>
</header>
<main id="app"><p class="eyebrow" style="padding:2rem 0">Loading the brief&hellip;</p></main>
<script>
"use strict";

// The secret arrived in the fragment, which browsers never send to a server.
// Read it, then remove it from the address bar so a screenshot or a shared URL
// does not carry it.
var JOB = location.pathname.split("/").filter(Boolean).pop();
var SECRET = location.hash.slice(1);

// Stripping the code from the address bar keeps it out of screenshots and
// shared URLs. On its own it also made a reload or a back-button press into a
// dead end: the code was gone, and the page told the worker to open a link
// they no longer had.
//
// Holding it in sessionStorage survives both, and dies with the tab. It is a
// real widening — anything running in this origin can read it — accepted
// because the alternative was losing a job halfway through, standing outside,
// with no way back in.
var STASH = "lamdis.work." + JOB;
try {
  if (SECRET) { sessionStorage.setItem(STASH, SECRET); }
  else { SECRET = sessionStorage.getItem(STASH) || ""; }
} catch (e) { /* private mode: the link still works, a reload will not */ }
if (location.hash) { history.replaceState(null, "", location.pathname); }

function esc(s) {
  return String(s == null ? "" : s).replace(/[&<>"']/g, function (c) {
    return {"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c];
  });
}

// tile draws one outcome as a HUD tile. variant is ok, wait or money; the
// colour is the verdict, read before a word is.
function tile(variant, eyebrow, big, lead, actions, extra) {
  return '<div class="hud outcome rv"><div class="t ' + variant + '">' +
    '<div class="k">' + eyebrow + '</div>' +
    '<div class="v">' + big + '</div>' +
    '<p class="lead">' + lead + '</p>' +
    '<div class="acts">' + actions + '</div>' +
    (extra || "") +
  '</div></div>';
}

function showOutcome(b, sha) {
  try { sessionStorage.removeItem(STASH); } catch (e) {}
  var paid = b.amount_minor ? money(b.amount_minor, b.currency || "usd") : null;
  var accepted = b.status === "accepted" || b.status === "attempt recorded";
  var practice = !!b.practice;
  var pending = !accepted && !practice && !b.why;
  var variant = accepted ? "ok" : (pending || practice ? "money" : "wait");
  var eyebrow = practice ? "Practice run"
          : accepted ? "Accepted" : (pending ? "Submitted · being checked" : "Not accepted");
  var big = practice ? (b.why ? "This one did not pass" : "Practice run recorded")
          : accepted ? (paid ? paid + " earned" : "Accepted")
          : (pending ? "Sent for checking" : "This one did not pass");
  // A practice run says what it is. It used to read "payment is still
  // settling" over an escrow of nothing.
  var lead = practice
    ? (b.why ? esc(b.why) + " " : "") + esc(b.note || "This was a practice run. Nothing is paid for it and it does not count toward your record.")
    : accepted
    ? (b.reached === "V0"
        ? "Your report is in and the fee is credited to your account. No photograph was checked, so the receipt records it as a signed claim."
        : "It is in your account. Payouts follow your payout setting.")
    : (pending
        ? "You will be paid once the evidence is accepted. Your account shows the outcome."
        : esc(b.why) + " You are still here — you can take the job again and reshoot.");
  var acts = '<a class="btn go" href="/board">' +
    (accepted ? "Take another job" : "Back to the board") + '</a>' +
    '<a class="btn" href="/console">Earnings</a>';
  var extra = sha ? '<div class="sha"><b>Evidence sha-256</b><br>' + esc(sha) + '</div>' : "";
  var pill = document.getElementById("pill");
  if (pill) { pill.innerHTML = '<span class="beacon' + (accepted ? "" : " off") + '"></span>' + eyebrow; }
  document.getElementById("app").innerHTML = tile(variant, eyebrow, big, lead, acts, extra);
}

function money(m, cur) {
  var sign = m < 0 ? "-" : "", v = Math.abs(m || 0);
  var sym = (cur || "USD") === "USD" ? "$" : cur + " ";
  return sign + sym + Math.floor(v / 100) + "." + String(v % 100).padStart(2, "0");
}

// SHA-256 by hand: crypto.subtle is unavailable on plain HTTP, which is
// exactly how this page gets served on a local network.
function sha256Hex(bytes) {
  var K=[0x428a2f98,0x71374491,0xb5c0fbcf,0xe9b5dba5,0x3956c25b,0x59f111f1,0x923f82a4,0xab1c5ed5,
         0xd807aa98,0x12835b01,0x243185be,0x550c7dc3,0x72be5d74,0x80deb1fe,0x9bdc06a7,0xc19bf174,
         0xe49b69c1,0xefbe4786,0x0fc19dc6,0x240ca1cc,0x2de92c6f,0x4a7484aa,0x5cb0a9dc,0x76f988da,
         0x983e5152,0xa831c66d,0xb00327c8,0xbf597fc7,0xc6e00bf3,0xd5a79147,0x06ca6351,0x14292967,
         0x27b70a85,0x2e1b2138,0x4d2c6dfc,0x53380d13,0x650a7354,0x766a0abb,0x81c2c92e,0x92722c85,
         0xa2bfe8a1,0xa81a664b,0xc24b8b70,0xc76c51a3,0xd192e819,0xd6990624,0xf40e3585,0x106aa070,
         0x19a4c116,0x1e376c08,0x2748774c,0x34b0bcb5,0x391c0cb3,0x4ed8aa4a,0x5b9cca4f,0x682e6ff3,
         0x748f82ee,0x78a5636f,0x84c87814,0x8cc70208,0x90befffa,0xa4506ceb,0xbef9a3f7,0xc67178f2];
  var H=[0x6a09e667,0xbb67ae85,0x3c6ef372,0xa54ff53a,0x510e527f,0x9b05688c,0x1f83d9ab,0x5be0cd19];
  var l = bytes.length, withOne = l + 1;
  var padded = new Uint8Array(Math.ceil((withOne + 8) / 64) * 64);
  padded.set(bytes); padded[l] = 0x80;
  var bits = l * 8;
  var dv = new DataView(padded.buffer);
  dv.setUint32(padded.length - 4, bits >>> 0);
  dv.setUint32(padded.length - 8, Math.floor(bits / 4294967296));
  var w = new Uint32Array(64);
  function rr(x, n) { return (x >>> n) | (x << (32 - n)); }
  for (var i = 0; i < padded.length; i += 64) {
    for (var t = 0; t < 16; t++) { w[t] = dv.getUint32(i + t * 4); }
    for (t = 16; t < 64; t++) {
      var s0 = rr(w[t-15],7) ^ rr(w[t-15],18) ^ (w[t-15] >>> 3);
      var s1 = rr(w[t-2],17) ^ rr(w[t-2],19) ^ (w[t-2] >>> 10);
      w[t] = (w[t-16] + s0 + w[t-7] + s1) >>> 0;
    }
    var a=H[0],b=H[1],c=H[2],d=H[3],e=H[4],f=H[5],g=H[6],h=H[7];
    for (t = 0; t < 64; t++) {
      var S1 = rr(e,6) ^ rr(e,11) ^ rr(e,25);
      var ch = (e & f) ^ (~e & g);
      var t1 = (h + S1 + ch + K[t] + w[t]) >>> 0;
      var S0 = rr(a,2) ^ rr(a,13) ^ rr(a,22);
      var mj = (a & b) ^ (a & c) ^ (b & c);
      var t2 = (S0 + mj) >>> 0;
      h=g; g=f; f=e; e=(d+t1)>>>0; d=c; c=b; b=a; a=(t1+t2)>>>0;
    }
    H[0]=(H[0]+a)>>>0; H[1]=(H[1]+b)>>>0; H[2]=(H[2]+c)>>>0; H[3]=(H[3]+d)>>>0;
    H[4]=(H[4]+e)>>>0; H[5]=(H[5]+f)>>>0; H[6]=(H[6]+g)>>>0; H[7]=(H[7]+h)>>>0;
  }
  return H.map(function (x) { return ("00000000" + x.toString(16)).slice(-8); }).join("");
}

function hmacHex(keyStr, msgStr) {
  var enc = new TextEncoder();
  var key = enc.encode(keyStr);
  if (key.length > 64) { key = hexToBytes(sha256Hex(key)); }
  var k = new Uint8Array(64); k.set(key);
  var ipad = new Uint8Array(64), opad = new Uint8Array(64);
  for (var i = 0; i < 64; i++) { ipad[i] = k[i] ^ 0x36; opad[i] = k[i] ^ 0x5c; }
  var msg = enc.encode(msgStr);
  var inner = new Uint8Array(64 + msg.length);
  inner.set(ipad); inner.set(msg, 64);
  var innerHash = hexToBytes(sha256Hex(inner));
  var outer = new Uint8Array(64 + 32);
  outer.set(opad); outer.set(innerHash, 64);
  return sha256Hex(outer);
}
function hexToBytes(hex) {
  var out = new Uint8Array(hex.length / 2);
  for (var i = 0; i < out.length; i++) { out[i] = parseInt(hex.substr(i * 2, 2), 16); }
  return out;
}

// The same signing input the Ed25519 scheme uses, so this path inherits its
// replay and tamper properties rather than inventing weaker ones. The headers
// match the reviewer surface exactly.
function authHeaders(method, path, bodyBytes) {
  var ts = new Date().toISOString().replace(/\.\d+Z$/, "Z");
  var bodyHash = sha256Hex(bodyBytes || new Uint8Array(0));
  var mac = hmacHex(SECRET, method + "\n" + path + "\n" + ts + "\n" + bodyHash);
  return { "X-Lamdis-Timestamp": ts, "X-Lamdis-Capability": JOB + "." + mac };
}

var BRIEF = null;

function load() {
  if (!SECRET) {
    fail("This page is missing its access code — that happens if the link was " +
      "retyped or opened in a new tab. The job is still yours: open it again from " +
      "the board.", "Access code missing");
    return;
  }
  var path = "/v1/work/" + encodeURIComponent(JOB);
  fetch(path, { headers: authHeaders("GET", path, null) })
    .then(function (r) { if (!r.ok) { throw new Error("This link is not valid any more."); } return r.json(); })
    .then(function (b) { BRIEF = b; render(); })
    .catch(function (e) { fail(e.message, "Link not valid"); });
}

function fail(msg, eyebrow) {
  var pill = document.getElementById("pill");
  if (pill) { pill.innerHTML = '<span class="beacon off"></span>' + eyebrow; }
  document.getElementById("app").innerHTML = tile("wait", eyebrow,
    "This link cannot open", esc(msg),
    '<a class="btn go" href="/board">Back to the queue</a>' +
    '<a class="btn" href="/how-it-works">How this works</a>',
    '<p class="fine">Work links end in a <b>#</b> and a code. The part after the # ' +
      'never reaches the server, which is why it cannot be recovered from here.</p>');
}

// stageBar draws the run of stages weighted by money. Only this stage's pay
// is known on this page, so it is drawn to scale against the whole job and
// the other stages share what remains: the honest picture without inventing
// figures for stages the brief did not send.
function stageBar(b) {
  var m = /^(\d+) of (\d+)$/.exec(b.stage_of || "");
  if (!m) { return ""; }
  var at = parseInt(m[1], 10), of = parseInt(m[2], 10);
  var share = b.pay_minor > 0 ? b.stage_pay_minor / b.pay_minor : 1 / of;
  share = Math.max(.06, Math.min(.9, share));
  var rest = of > 1 ? (1 - share) / (of - 1) : 0;
  var segs = "";
  for (var i = 1; i <= of; i++) {
    var cls = i < at ? " paid" : (i === at ? " now" : "");
    segs += '<span class="seg' + cls + '" style="--w:' + (i === at ? share : rest).toFixed(3) + '"></span>';
  }
  return '<div class="stagebar" role="img" aria-label="' + esc("stage " + at + " of " + of) + '">' + segs + '</div>' +
    '<div class="stagekey">' +
      (at > 1 ? '<span class="paid"><b>' + (at - 1) + '</b> done</span>' : "") +
      '<span class="now"><b>' + money(b.stage_pay_minor, b.currency) + '</b> this stage</span>' +
      '<span><b>' + money(b.pay_minor, b.currency) + '</b> whole job</span>' +
    '</div>';
}

// reportForm draws the table a job wants filled in: one card per row, a
// field per input, "add another" when the fields repeat.
function reportField(f, i) {
  var type = f.kind === "url" ? "url" : f.kind === "phone" ? "tel" : f.kind === "date" ? "date" : "text";
  var hint = f.kind === "money" ? " <small>(amount, e.g. 1200.00)</small>" : "";
  if (f.kind === "bool") {
    return '<label for="rf-' + i + '-' + esc(f.name) + '">' + esc(f.label || f.name) +
      (f.required ? '' : ' <small>optional</small>') + hint + '</label>' +
      '<select id="rf-' + i + '-' + esc(f.name) + '" data-field="' + esc(f.name) + '">' +
        '<option value="">&mdash;</option><option value="yes">Yes</option><option value="no">No</option></select>';
  }
  return '<label for="rf-' + i + '-' + esc(f.name) + '">' + esc(f.label || f.name) +
    (f.required ? '' : ' <small>optional</small>') + hint + '</label>' +
    '<input id="rf-' + i + '-' + esc(f.name) + '" type="' + type + '" data-field="' + esc(f.name) + '"' +
    (f.kind === "money" ? ' inputmode="decimal"' : '') + '>';
}
function reportRow(fields, i, repeats) {
  var body = fields.filter(function (f) { return i === 0 || f.repeats; })
    .map(function (f) { return reportField(f, i); }).join("");
  if (!body) { return ""; }
  return '<div class="row" data-row="' + i + '"><div class="rn"><span>' +
    (repeats ? "Result " + (i + 1) : "Your answer") + '</span>' +
    (i > 0 ? '<button type="button" class="rm">remove</button>' : '') + '</div>' + body + '</div>';
}
function readReport(fields) {
  var rows = [];
  Array.prototype.forEach.call(document.querySelectorAll("#report .row"), function (row) {
    var r = {}, any = false;
    Array.prototype.forEach.call(row.querySelectorAll("[data-field]"), function (el) {
      var v = (el.value || "").trim();
      if (v) { r[el.dataset.field] = v; any = true; }
    });
    if (any) { rows.push(r); }
  });
  return rows;
}
function reportComplete(fields) {
  var rows = readReport(fields);
  if (!rows.length) { return false; }
  return rows.every(function (r, i) {
    return fields.every(function (f) {
      if (!f.required) { return true; }
      if (!f.repeats && i > 0) { return true; }
      return !!r[f.name];
    });
  });
}

function render() {
  var b = BRIEF;
  var isDo = b.kind === "do";
  var fields = b.report || [];
  var isReport = fields.length > 0;
  var repeats = fields.some(function (f) { return f.repeats; });
  var pill = document.getElementById("pill");
  if (pill) { pill.innerHTML = '<span class="beacon"></span>' + (isDo ? "Do job" : "Observe job") + (b.tier ? ' · ' + esc(b.tier) : ""); }
  document.getElementById("app").innerHTML = '' +
    '<div class="hdr rv" style="--i:0">' +
      '<span class="chip ' + (isDo ? "do" : "obs") + '">' + (isDo ? "Act" : "Check") + '</span>' +
      (b.tier ? '<span class="chip">' + esc(b.tier) + '</span>' : "") +
      '<h1>' + esc(b.title) + '</h1>' +
      (b.where ? '<p class="lead"><b>Where:</b> ' + esc(b.where) + '</p>' : '') +
    '</div>' +
    '<dl class="hud rv" style="--i:1">' +
      '<div class="t money"><dt class="k">You get paid</dt><dd class="v" style="margin-left:0">' + money(b.pay_minor, b.currency) + '</dd>' +
        '<div class="s">held before you started</div></div>' +
      (b.bonus_minor
        ? '<div class="t soft"><dt class="k">Bonus if the answer is yes</dt><dd class="v" style="margin-left:0">+' + money(b.bonus_minor, b.currency) + '</dd>' +
          '<div class="s">paid either way for a usable photo</div></div>'
        : '<div class="t soft"><dt class="k">Bring back</dt><dd class="v txt" style="margin-left:0">' +
            esc(b.deliverable || "a clear photo") + '</dd><div class="s">as the camera saved it</div></div>') +
    '</dl>' +
    (b.practice
      ? '<p class="fine rv" style="--i:1"><b>Practice run.</b> Nothing is paid for this and it ' +
        'does not count toward your record either way. It is here so you can see how the flow works.</p>'
      : "") +
    (isReport
      ? '<div class="report rv" id="report" style="--i:2">' +
          '<span class="eyebrow gold" style="margin-bottom:.5rem">Write down what you found</span>' +
          reportRow(fields, 0, repeats) +
          (repeats ? '<p class="add"><button type="button" class="btn" id="addrow">Add another</button></p>' : "") +
          '<p class="fine" style="margin:.4rem 0 0">A photo is optional on this job. Without one, the ' +
            'report is recorded as your signed answer and nothing ties it to a time or place.</p>' +
        '</div>'
      : "") +
    '<div class="code-card rv" style="--i:2">' +
      '<span class="eyebrow gold">' + (isReport ? "If you add a photo, write this where the camera can see it" : "Write this where the camera can see it") + '</span>' +
      '<span class="big">' + esc(b.challenge) + '</span>' +
      '<p>Paper, a phone screen, anything. It proves the photo was taken now, for ' +
        'this job.</p>' +
    '</div>' +
    '<div class="shot" id="shot"><img id="preview" alt="The photograph you are about to send">' +
      '<p class="cap" id="shotcap"></p></div>' +
    '<label class="drop rv" for="f" id="drop" style="--i:3">' +
      '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 8h3l2-3h6l2 3h3v11H4z"/><circle cx="12" cy="13" r="3.5"/></svg>' +
      '<span class="big" id="droptext">' + (isReport ? "Add a photo (optional)" : "Take a photo") + '</span>' +
      '<span class="sm">or drop one here · camera roll works too</span>' +
    '</label>' +
    '<input id="f" type="file" accept="image/*,video/mp4" capture="environment">' +
    '<p class="send rv" style="--i:4"><button class="btn go wide" id="send" disabled>Submit</button></p>' +
    '<div class="err" id="err"></div>' +
    (BRIEF.stage
      ? '<div class="glass stage rv" style="--i:5">' +
          '<span class="eyebrow gold">Stage ' + esc(BRIEF.stage_of) + '</span>' +
          '<h2>' + esc(BRIEF.stage) + '</h2>' +
          '<p class="note" style="margin:0">Photograph ' + esc(BRIEF.stage_proves) +
            '. This stage pays ' + money(BRIEF.stage_pay_minor, BRIEF.currency) +
            ' on its own — you do not wait for the whole job.</p>' +
          stageBar(BRIEF) +
        '</div>'
      : "") +
    '<div class="glass attempt rv" style="--i:6">' +
      '<span class="eyebrow">If it cannot be done</span>' +
      '<button type="button" class="btn" id="cant">I went, but this cannot be done</button>' +
      '<div id="cant-box" hidden>' +
        '<label class="ask" for="cant-why">What stopped you?</label>' +
        '<textarea id="cant-why" rows="2" placeholder="The gate was padlocked and nobody answered."></textarea>' +
        '<div class="ask-acts">' +
          '<button type="button" class="btn go" id="cant-ok">Mark as an attempt</button>' +
          '<button type="button" class="btn" id="cant-no">Cancel</button>' +
        '</div>' +
      '</div>' +
      '<p class="note" style="margin:.5rem 0 0">Photograph what stopped you, with the ' +
        'code in frame. You are paid the attempt fee rather than the full amount.</p>' +
      '<p class="note next" id="cant-next" hidden></p>' +
    '</div>' +
    (BRIEF.tier === "V2" || BRIEF.tier === "V3"
      ? '<p class="fine rv" style="--i:7"><b>Location must be on.</b> This job needs photographs ' +
        'that record where and when they were taken. Take them in the camera app ' +
        'with location enabled — a picture sent through a messaging app has ' +
        'that stripped and will be refused.</p>'
      : "") +
    '<p class="fine rv" style="--i:8">Your photo uploads exactly as your camera saved it. You are paid ' +
      'for a usable submission &mdash; the answer does not have to be the one anyone ' +
      'hoped for.</p>';

  var cant = document.getElementById("cant");
  var cantBox = document.getElementById("cant-box");
  if (cant && cantBox) {
    cant.addEventListener("click", function () {
      cantBox.hidden = false;
      cant.hidden = true;
      document.getElementById("cant-why").focus();
    });
    document.getElementById("cant-no").addEventListener("click", function () {
      cantBox.hidden = true;
      cant.hidden = false;
    });
    document.getElementById("cant-ok").addEventListener("click", function () {
      var why = document.getElementById("cant-why").value.trim();
      if (!why) { document.getElementById("cant-why").focus(); return; }
      window.__lamdisAttempt = true;
      window.__lamdisAttemptWhy = why;
      cantBox.hidden = true;
      cant.hidden = false;
      cant.textContent = "Attempt: " + (why.length > 40 ? why.slice(0, 40) + "…" : why);
      cant.disabled = true;

      // Marking an attempt is not submitting one. It still needs a photograph
      // of whatever stopped you, and saying so here is the difference between
      // a flow that continues and a page that appears to have done nothing.
      var send = document.getElementById("send");
      if (send) { send.textContent = "Submit attempt"; }
      var note = document.getElementById("cant-next");
      if (note) {
        note.hidden = false;
        note.textContent = "Now photograph what stopped you, with the code in " +
          "frame, and send it. Without a photo nothing is submitted.";
      }
      var input = document.getElementById("f");
      if (input) { input.click(); }
    });
  }
  var input = document.getElementById("f");
  var send = document.getElementById("send");
  var drop = document.getElementById("drop");
  var chosen = null;

  // The report enables the button on its own; a photo is a bonus.
  var reportBox = document.getElementById("report");
  function ready() { send.disabled = !(chosen || (isReport && reportComplete(fields))); }
  if (reportBox) {
    reportBox.addEventListener("input", ready);
    reportBox.addEventListener("change", ready);
    reportBox.addEventListener("click", function (ev) {
      var t = ev.target;
      if (t && t.classList && t.classList.contains("rm")) {
        var row = t.closest(".row");
        if (row) { row.parentNode.removeChild(row); ready(); }
      }
    });
    var addrow = document.getElementById("addrow");
    if (addrow) {
      addrow.addEventListener("click", function () {
        var n = document.querySelectorAll("#report .row").length;
        var wrap = document.createElement("div");
        wrap.innerHTML = reportRow(fields, n, repeats);
        addrow.parentNode.parentNode.insertBefore(wrap.firstChild, addrow.parentNode);
        ready();
      });
    }
  }

  function took(file) {
    chosen = file;
    if (!chosen) { return; }
    var img = document.getElementById("preview");
    img.src = URL.createObjectURL(chosen);
    document.getElementById("shot").classList.add("on");
    // Say what is being sent. Somebody about to upload a 12MB photo over a
    // phone signal at the side of a road is entitled to know that before it
    // starts, not after it fails.
    img.onload = function () {
      var mb = (chosen.size / (1024 * 1024)).toFixed(1);
      document.getElementById("shotcap").innerHTML =
        '<b>' + img.naturalWidth + " × " + img.naturalHeight + '</b> · ' + mb + " MB · " +
        esc(chosen.type || "file") + " · sent as saved";
    };
    ready();
    document.getElementById("droptext").textContent = "Choose a different photo";
  }

  input.addEventListener("change", function () {
    took(input.files && input.files[0]);
  });
  // A real drop target on a desktop, so the dashed zone does what it says.
  if (drop) {
    drop.addEventListener("dragover", function (ev) { ev.preventDefault(); drop.classList.add("over"); });
    drop.addEventListener("dragleave", function () { drop.classList.remove("over"); });
    drop.addEventListener("drop", function (ev) {
      ev.preventDefault(); drop.classList.remove("over");
      var f = ev.dataTransfer && ev.dataTransfer.files && ev.dataTransfer.files[0];
      if (f) { took(f); }
    });
  }

  // Uploading is not submitting. Without the second call the file sat on
  // the server unclaimed forever: the worker saw a success screen, no
  // submission was ever created, nothing was verified, and nobody was ever
  // paid. A report job may skip the upload and go straight to submitting.
  function finalize(sha) {
    send.textContent = "Checking…";
    var attempted = !!window.__lamdisAttempt;
    var claim = {};
    if (attempted) { claim.attempted = true; claim.why = window.__lamdisAttemptWhy || ""; }
    if (isReport) { claim.report = readReport(fields); }
    var body = (attempted || isReport) ? JSON.stringify(claim) : null;
    var sub = "/v1/work/" + encodeURIComponent(JOB) + "/submit";
    var payload = body ? new TextEncoder().encode(body) : new Uint8Array(0);
    var hs = authHeaders("POST", sub, payload);
    if (body) { hs["Content-Type"] = "application/json"; }
    return fetch(sub, { method: "POST", headers: hs, body: body })
      .then(function (r) {
        return r.json().then(function (j) { return { ok: r.ok, body: j, sha: sha }; });
      })
      .then(function (res) {
        if (!res.ok) { throw new Error(res.body && res.body.error || "could not submit"); }
        showOutcome(res.body, res.sha);
      });
  }
  function failed(e) {
    document.getElementById("err").textContent = e.message;
    send.disabled = false;
    send.textContent = "Submit";
  }

  send.addEventListener("click", function () {
    if (!chosen && !(isReport && reportComplete(fields))) { return; }
    send.disabled = true;
    document.getElementById("err").textContent = "";
    if (!chosen) {
      finalize("").catch(failed);
      return;
    }
    send.textContent = "Uploading…";

    // Read the file into memory so the exact bytes can be hashed into the
    // signature and sent unchanged. No canvas, no re-encode: the EXIF is
    // evidence.
    var reader = new FileReader();
    reader.onload = function () {
      var bytes = new Uint8Array(reader.result);
      var path = "/v1/work/" + encodeURIComponent(JOB) + "/evidence";
      var h = authHeaders("POST", path, bytes);
      h["Content-Type"] = "application/octet-stream";
      fetch(path, { method: "POST", headers: h, body: bytes })
        .then(function (r) { return r.json().then(function (j) { return { ok: r.ok, body: j }; }); })
        .then(function (res) {
          if (!res.ok) { throw new Error(res.body && res.body.error || "upload failed"); }
          return finalize(res.body.sha256);
        })
        .catch(failed);
    };
    reader.onerror = function () {
      failed(new Error("Could not read that file."));
    };
    reader.readAsArrayBuffer(chosen);
  });
}

load();
</script>
`
