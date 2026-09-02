package api

// workerJS is the browser half of worker identity, shared by every page that
// needs one.
//
// This file is deliberately not named worker_js.go: Go reads a _js suffix as a
// GOOS constraint and would build it only for js/wasm, silently excluding it
// everywhere else.
//
// It tries to give the person a real key and degrades honestly when it cannot.
// A generated Ed25519 key makes them a principal: their submissions carry their
// own signature and they can be paid. WebCrypto only offers Ed25519 in a secure
// context, so a page served over plain HTTP on a local network always lands on
// the bearer-secret path — which works, counts as work, and cannot be paid.
// The page says which one happened rather than pretending they are the same.
const workerJS = `
"use strict";

// SHA-256 by hand, because crypto.subtle does not exist on plain HTTP and that
// is exactly where this page runs during a pilot.
function sha256Bytes(bytes) {
  var K=[0x428a2f98,0x71374491,0xb5c0fbcf,0xe9b5dba5,0x3956c25b,0x59f111f1,0x923f82a4,0xab1c5ed5,
         0xd807aa98,0x12835b01,0x243185be,0x550c7dc3,0x72be5d74,0x80deb1fe,0x9bdc06a7,0xc19bf174,
         0xe49b69c1,0xefbe4786,0x0fc19dc6,0x240ca1cc,0x2de92c6f,0x4a7484aa,0x5cb0a9dc,0x76f988da,
         0x983e5152,0xa831c66d,0xb00327c8,0xbf597fc7,0xc6e00bf3,0xd5a79147,0x06ca6351,0x14292967,
         0x27b70a85,0x2e1b2138,0x4d2c6dfc,0x53380d13,0x650a7354,0x766a0abb,0x81c2c92e,0x92722c85,
         0xa2bfe8a1,0xa81a664b,0xc24b8b70,0xc76c51a3,0xd192e819,0xd6990624,0xf40e3585,0x106aa070,
         0x19a4c116,0x1e376c08,0x2748774c,0x34b0bcb5,0x391c0cb3,0x4ed8aa4a,0x5b9cca4f,0x682e6ff3,
         0x748f82ee,0x78a5636f,0x84c87814,0x8cc70208,0x90befffa,0xa4506ceb,0xbef9a3f7,0xc67178f2];
  var H=[0x6a09e667,0xbb67ae85,0x3c6ef372,0xa54ff53a,0x510e527f,0x9b05688c,0x1f83d9ab,0x5be0cd19];
  var l=bytes.length, padded=new Uint8Array(Math.ceil((l+9)/64)*64);
  padded.set(bytes); padded[l]=0x80;
  var dv=new DataView(padded.buffer), bits=l*8;
  dv.setUint32(padded.length-4, bits>>>0);
  dv.setUint32(padded.length-8, Math.floor(bits/4294967296));
  var w=new Uint32Array(64);
  function rr(x,n){return (x>>>n)|(x<<(32-n));}
  for (var i=0;i<padded.length;i+=64) {
    for (var t=0;t<16;t++){w[t]=dv.getUint32(i+t*4);}
    for (t=16;t<64;t++){
      var s0=rr(w[t-15],7)^rr(w[t-15],18)^(w[t-15]>>>3);
      var s1=rr(w[t-2],17)^rr(w[t-2],19)^(w[t-2]>>>10);
      w[t]=(w[t-16]+s0+w[t-7]+s1)>>>0;
    }
    var a=H[0],b=H[1],c=H[2],d=H[3],e=H[4],f=H[5],g=H[6],h=H[7];
    for (t=0;t<64;t++){
      var S1=rr(e,6)^rr(e,11)^rr(e,25), ch=(e&f)^(~e&g);
      var t1=(h+S1+ch+K[t]+w[t])>>>0;
      var S0=rr(a,2)^rr(a,13)^rr(a,22), mj=(a&b)^(a&c)^(b&c);
      var t2=(S0+mj)>>>0;
      h=g;g=f;f=e;e=(d+t1)>>>0;d=c;c=b;b=a;a=(t1+t2)>>>0;
    }
    H[0]=(H[0]+a)>>>0;H[1]=(H[1]+b)>>>0;H[2]=(H[2]+c)>>>0;H[3]=(H[3]+d)>>>0;
    H[4]=(H[4]+e)>>>0;H[5]=(H[5]+f)>>>0;H[6]=(H[6]+g)>>>0;H[7]=(H[7]+h)>>>0;
  }
  var out=new Uint8Array(32), odv=new DataView(out.buffer);
  for (var j=0;j<8;j++){odv.setUint32(j*4,H[j]);}
  return out;
}
function toHex(b){var s="";for(var i=0;i<b.length;i++){s+=("0"+b[i].toString(16)).slice(-2);}return s;}
function sha256Hex(b){return toHex(sha256Bytes(b));}
function hmacHex(keyStr,msgStr){
  var enc=new TextEncoder(), key=enc.encode(keyStr);
  if (key.length>64){key=sha256Bytes(key);}
  var k=new Uint8Array(64); k.set(key);
  var ipad=new Uint8Array(64), opad=new Uint8Array(64);
  for(var i=0;i<64;i++){ipad[i]=k[i]^0x36;opad[i]=k[i]^0x5c;}
  var msg=enc.encode(msgStr);
  var inner=new Uint8Array(64+msg.length); inner.set(ipad); inner.set(msg,64);
  var ih=sha256Bytes(inner);
  var outer=new Uint8Array(96); outer.set(opad); outer.set(ih,64);
  return sha256Hex(outer);
}

// Crockford base32, matching how the protocol names a principal.
function crockford32(bytes){
  var A="0123456789ABCDEFGHJKMNPQRSTVWXYZ", out="", bits=0, value=0;
  for (var i=0;i<bytes.length;i++){
    value=(value<<8)|bytes[i]; bits+=8;
    while (bits>=5){out+=A[(value>>>(bits-5))&31]; bits-=5;}
  }
  if (bits>0){out+=A[(value<<(5-bits))&31];}
  return out;
}

function stamp(){return new Date().toISOString().replace(/\.\d+Z$/,"Z");}

var WORKER = null; // {id, verified} once signed in
var TOKEN = null;

function loadToken() {
  try { return localStorage.getItem("lamdis.token"); } catch (e) { return null; }
}
function loadWorker() {
  try {
    var raw = localStorage.getItem("lamdis.worker");
    if (raw) { return JSON.parse(raw); }
  } catch (e) {}
  return null;
}

// saveWorker persists who this is, so the /v1/me round trip below happens once
// rather than on every page.
//
// This was called and never defined. The consequence was not a missing cache:
// session() calls it inside a promise chain whose .catch clears the session, so
// the ReferenceError was swallowed and read as "this token is no good". Anybody
// holding a valid token with no cached worker id — a second tab, a fresh
// device, cleared site data, or simply the first navigation after signing in —
// was silently signed out and bounced to /signin. The comment on session()
// describes fixing exactly this. The fix was written; the function it depended
// on never was.
function saveWorker(w) {
  try { localStorage.setItem("lamdis.worker", JSON.stringify(w)); } catch (e) {}
}
function clearSession() {
  try {
    localStorage.removeItem("lamdis.token");
    localStorage.removeItem("lamdis.worker");
  } catch (e) {}
  TOKEN = null; WORKER = null;
}

// signedIn is the only question the pages ask. There is no guest tier: work
// nobody can be paid for is work nobody has a reason to do, and an identity
// anybody can mint for free bounds none of the abuse rules that depend on it.
function signedIn() { return !!(TOKEN && WORKER && WORKER.id); }

function goSignIn() {
  window.location.href = "/signin?next=" + encodeURIComponent(location.pathname);
}

// session restores who this is, if anyone. It never creates an identity.
//
// A token with no worker id attached is recoverable, not fatal: the token is
// the credential, and the id is only a label the server can hand back. Treating
// that state as signed-out was how somebody with a perfectly good session got
// sent to the sign-in page by every button on the site.
function session() {
  TOKEN = loadToken();
  WORKER = loadWorker();
  if (!TOKEN) { clearSession(); return Promise.resolve(null); }
  if (WORKER && WORKER.id) { return Promise.resolve(WORKER); }
  return fetch("/v1/me", {headers: {"Authorization": "Bearer " + TOKEN}})
    .then(function (r) { return r.ok ? r.json() : null; })
    .then(function (me) {
      if (!me || !me.worker) { clearSession(); return null; }
      WORKER = {id: me.worker, verified: !!me.verified, enrolled: !!me.enrolled};
      saveWorker(WORKER);
      return WORKER;
    })
    .catch(function () { clearSession(); return null; });
}

// workerHeaders authenticates a request as the signed-in account.
function workerHeaders(method, path) {
  if (!TOKEN) { return Promise.reject(new Error("not signed in")); }
  return Promise.resolve({"Authorization": "Bearer " + TOKEN});
}

// An expired token looks exactly like being signed out, because it is. Pages
// call this on a 401 so somebody mid-task is sent to sign in again rather than
// left staring at an error.
function handleAuthFailure(status) {
  if (status === 401) { clearSession(); goSignIn(); return true; }
  return false;
}
`

// boardJS is the part of the queue that a single-job page needs too: how a
// figure is shown, how a job is taken, how a bid is placed. Shared so /j/{job}
// and /board cannot drift apart on what "take" or "$45.00" means.
//
// post() refreshes through load(), which each page defines for itself.
const boardJS = `
// TERMS is what the exchange keeps and when it pays out, from /v1/board.
var TERMS = null;

function esc(s) {
  return String(s == null ? "" : s).replace(/[&<>"']/g, function (c) {
    return {"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c];
  });
}
function money(m, cur) {
  var sign = m < 0 ? "-" : "", v = Math.abs(m || 0);
  var sym = (cur || "USD") === "USD" ? "$" : cur + " ";
  return sign + sym + Math.floor(v / 100) + "." + String(v % 100).padStart(2, "0");
}
function toMinor(t) {
  var c = String(t || "").replace(/[^0-9.]/g, "");
  if (!c) { return 0; }
  var n = Math.round(parseFloat(c) * 100);
  return isFinite(n) ? n : 0;
}
function left(iso) {
  var ms = new Date(iso) - new Date();
  if (isNaN(ms) || ms <= 0) { return "closed"; }
  var m = Math.round(ms / 60000);
  if (m < 60) { return m + "m"; }
  var h = Math.round(m / 60);
  return h < 48 ? h + "h" : Math.round(h / 24) + "d";
}
function kindLabel(k) {
  return k === "do" ? "Act" : (k === "review" ? "Verify" : "Check");
}

// What a listed figure actually becomes in your account.
//
// The board advertised a number and settlement paid a smaller one, with the
// difference explained nowhere. Showing the take-home next to the headline is
// the least a marketplace can do before somebody spends an afternoon earning
// it.
function takeHome(minor) {
  if (!TERMS || !TERMS.fee_bp || !minor) { return null; }
  return minor - Math.floor(minor * TERMS.fee_bp / 10000);
}

function post(button, errEl, path, body) {
  if (!signedIn()) { goSignIn(); return; }
  button.disabled = true;
  var was = button.textContent;
  button.textContent = "Working…";
  if (errEl) { errEl.textContent = ""; errEl.className = "err"; }

  workerHeaders("POST", path).then(function (h) {
    if (body) { h["Content-Type"] = "application/json"; }
    return fetch(path, {method: "POST", headers: h, body: body ? JSON.stringify(body) : undefined});
  }).then(function (r) {
    return r.json().then(function (j) { return {ok: r.ok, status: r.status, body: j}; });
  }).then(function (res) {
    if (handleAuthFailure(res.status)) { return; }
    if (!res.ok) { throw new Error(res.body && res.body.error || "that did not work"); }
    if (res.body.url) { window.location.href = res.body.url; return; }
    button.textContent = "Done";
    load();
  }).catch(function (e) {
    if (errEl) { errEl.textContent = e.message; }
    button.disabled = false;
    button.textContent = was;
  });
}

function takeJob(button, job) {
  post(button, document.getElementById("e-" + job), "/v1/workers/claim/" + encodeURIComponent(job));
}

// askUnknowns renders a field per thing the buyer said they do not know.
//
// A price on a job whose dimensions nobody has established is a guess, and the
// argument about it happens on site. Asking here costs one line each and makes
// the offer mean something.
// siteShots shows what the buyer supplied so this can be priced.
//
// The complaint that produced this: a job could describe four stages of paving
// and give nothing to price them against — no photograph of the ground, no
// access, no way to tell on arrival that you are at the right property. The
// shot marked as the identifier is called out, because that is the one
// somebody opens standing at the kerb.
function siteShots(w) {
  var refs = w.references || [];
  if (!refs.length) { return ""; }
  return '<div class="shots">' + refs.map(function (r) {
    var src = "/v1/jobs/" + encodeURIComponent(w.job) +
      "/references/" + encodeURIComponent(r.sha256);
    return '<figure class="ref' + (r.identifies ? " id" : "") + '">' +
      '<a href="' + src + '" target="_blank" rel="noopener">' +
        '<img src="' + src + '" alt="' + esc(r.caption || "the site") + '" loading="lazy">' +
      '</a>' +
      '<figcaption>' + esc(r.caption || "") +
        (r.identifies ? '<b>check you are here</b>' : "") + '</figcaption>' +
    '</figure>';
  }).join("") + '</div>';
}

function askUnknowns(w) {
  var us = w.unknowns || [];
  if (!us.length) { return ""; }
  return '<div class="unk">' +
    '<p class="unk-h">The buyer does not know these. Say what you priced on.</p>' +
    us.map(function (u, i) {
      return '<label class="unk-r">' +
        '<span>' + esc(u.name) + (u.unit ? ' <i>(' + esc(u.unit) + ')</i>' : "") + '</span>' +
        (u.note ? '<span class="unk-n">' + esc(u.note) + '</span>' : "") +
        '<input type="text" maxlength="60" placeholder="what you assumed" ' +
          'data-unk="' + esc(w.job) + '" data-unk-i="' + i + '" ' +
          'data-unk-name="' + esc(u.name) + '">' +
        '<label class="unk-f"><input type="checkbox" data-unkfirm="' + esc(w.job) +
          '" data-unk-i="' + i + '" checked> price holds at this figure</label>' +
      '</label>';
    }).join("") +
    '</div>';
}

function readAssumptions(job) {
  var out = [];
  document.querySelectorAll('[data-unk="' + job + '"]').forEach(function (el) {
    var i = el.getAttribute("data-unk-i");
    var firm = document.querySelector(
      '[data-unkfirm="' + job + '"][data-unk-i="' + i + '"]');
    out.push({
      name: el.getAttribute("data-unk-name"),
      value: el.value.trim(),
      firm: !!(firm && firm.checked)
    });
  });
  return out;
}

function placeBid(button, job) {
  var amount = toMinor(document.querySelector('[data-bid="' + job + '"]').value);
  var note = (document.querySelector('[data-note="' + job + '"]') || {}).value || "";
  var err = document.getElementById("e-" + job);
  if (amount <= 0) { err.textContent = "Enter what you would charge."; return; }
  var assumptions = readAssumptions(job);
  // Caught here as well as on the server, because being told what is missing
  // while the form is still in front of you is the difference between a fix
  // and a re-entry.
  var blank = assumptions.filter(function (a) { return !a.value; });
  if (blank.length) {
    err.textContent = "Say what you priced on for: " +
      blank.map(function (a) { return a.name; }).join(", ") + ".";
    return;
  }
  post(button, err, "/v1/workers/bid/" + encodeURIComponent(job),
       {amount_minor: amount, note: note, assumptions: assumptions});
}
`

// panelJS is the instrument panel's shared script: the live radar canvas and
// the count-up used by HUD tiles. Included by any page that draws either.
//
// drawRadar(canvas, opts) draws jobs around a centre: opts.jobs is a list of
// {lat, lon, kind, title, job, pay}, opts.you is {lat, lon} or null, and
// opts.rangeMiles the ring to draw. Without coordinates it still draws the
// ground and says so, rather than a blank rectangle. It returns a stop().
const panelJS = `
function countUp(el, target, fmt) {
  if (!el) { return; }
  var t0 = null, from = parseFloat(el.getAttribute("data-from") || "0") || 0;
  if (matchMedia("(prefers-reduced-motion: reduce)").matches) { el.textContent = fmt(target); return; }
  function step(ts) {
    if (!t0) { t0 = ts; }
    var p = Math.min(1, (ts - t0) / 700); p = 1 - Math.pow(1 - p, 3);
    el.textContent = fmt(from + (target - from) * p);
    if (p < 1) { requestAnimationFrame(step); } else { el.setAttribute("data-from", String(target)); }
  }
  requestAnimationFrame(step);
}

function drawRadar(canvas, opts) {
  if (!canvas || !canvas.getContext) { return function () {}; }
  var ctx = canvas.getContext("2d"), raf = 0, t0 = performance.now(), hover = -1;
  var jobs = (opts.jobs || []).filter(function (j) { return isFinite(j.lat) && isFinite(j.lon) && (j.lat || j.lon); });
  var you = opts.you && isFinite(opts.you.lat) ? opts.you : null;
  var range = opts.rangeMiles || 20;
  var cx = you ? you.lat : (jobs.length ? jobs.reduce(function (a, j) { return a + j.lat; }, 0) / jobs.length : 0);
  var cz = you ? you.lon : (jobs.length ? jobs.reduce(function (a, j) { return a + j.lon; }, 0) / jobs.length : 0);
  var dpr = Math.min(devicePixelRatio || 1, 2);
  function size() {
    var r = canvas.getBoundingClientRect();
    canvas.width = Math.max(1, r.width * dpr); canvas.height = Math.max(1, r.height * dpr);
  }
  function project(lat, lon) {
    var W = canvas.width, H = canvas.height, R = Math.min(W, H) * 0.42;
    var kx = 69.17 * Math.cos(cx * Math.PI / 180), ky = 69.17;
    var dx = (lon - cz) * kx / range, dy = (lat - cx) * ky / range;
    return [W / 2 + dx * R, H / 2 - dy * R];
  }
  function frame(now) {
    var W = canvas.width, H = canvas.height, R = Math.min(W, H) * 0.42, s = (now - t0) / 1000;
    ctx.clearRect(0, 0, W, H);
    ctx.save(); ctx.scale(1, 1);
    // grid
    ctx.strokeStyle = "rgba(43,57,69,.45)"; ctx.lineWidth = 1;
    var g = 28 * dpr;
    for (var x = (W / 2) % g; x < W; x += g) { ctx.beginPath(); ctx.moveTo(x, 0); ctx.lineTo(x, H); ctx.stroke(); }
    for (var y = (H / 2) % g; y < H; y += g) { ctx.beginPath(); ctx.moveTo(0, y); ctx.lineTo(W, y); ctx.stroke(); }
    // range rings
    [0.33, 0.66, 1].forEach(function (f, i) {
      ctx.beginPath(); ctx.arc(W / 2, H / 2, R * f, 0, Math.PI * 2);
      ctx.strokeStyle = i === 2 ? "rgba(95,176,255,.35)" : "rgba(95,176,255,.16)"; ctx.stroke();
    });
    ctx.fillStyle = "rgba(147,164,179,.7)"; ctx.font = (10 * dpr) + "px ui-monospace, Menlo, monospace";
    ctx.fillText(Math.round(range) + " mi", W / 2 + R + 4 * dpr, H / 2 - 4 * dpr);
    // sweep
    var a = (s * 0.6) % (Math.PI * 2);
    var grad = ctx.createConicGradient ? ctx.createConicGradient(a, W / 2, H / 2) : null;
    if (grad) {
      grad.addColorStop(0, "rgba(95,176,255,.18)"); grad.addColorStop(0.12, "rgba(95,176,255,0)"); grad.addColorStop(1, "rgba(95,176,255,0)");
      ctx.beginPath(); ctx.moveTo(W / 2, H / 2); ctx.arc(W / 2, H / 2, R, 0, Math.PI * 2); ctx.closePath();
      ctx.fillStyle = grad; ctx.fill();
    }
    // you
    if (you) {
      ctx.beginPath(); ctx.arc(W / 2, H / 2, 4 * dpr, 0, Math.PI * 2);
      ctx.fillStyle = "#3FCF71"; ctx.shadowColor = "#3FCF71"; ctx.shadowBlur = 12 * dpr; ctx.fill(); ctx.shadowBlur = 0;
    }
    // jobs
    jobs.forEach(function (j, i) {
      var p = project(j.lat, j.lon), col = j.kind === "observe" ? "#5FB0FF" : "#FFB627";
      var pulse = (s * 0.9 + i * 0.37) % 1;
      ctx.beginPath(); ctx.arc(p[0], p[1], (4 + pulse * 14) * dpr, 0, Math.PI * 2);
      ctx.strokeStyle = col; ctx.globalAlpha = (1 - pulse) * 0.5; ctx.stroke(); ctx.globalAlpha = 1;
      ctx.beginPath(); ctx.arc(p[0], p[1], (i === hover ? 5 : 3.5) * dpr, 0, Math.PI * 2);
      ctx.fillStyle = col; ctx.shadowColor = col; ctx.shadowBlur = 10 * dpr; ctx.fill(); ctx.shadowBlur = 0;
      if (i === hover && j.title) {
        ctx.fillStyle = "rgba(237,241,245,.95)"; ctx.font = (11 * dpr) + "px ui-monospace, Menlo, monospace";
        ctx.fillText((j.pay ? j.pay + "  " : "") + j.title.slice(0, 48), p[0] + 9 * dpr, p[1] - 8 * dpr);
      }
    });
    if (!jobs.length) {
      ctx.fillStyle = "rgba(93,110,124,.9)"; ctx.font = (11 * dpr) + "px ui-monospace, Menlo, monospace";
      ctx.textAlign = "center"; ctx.fillText(opts.empty || "nothing on the board carries a location yet", W / 2, H / 2 + 4 * dpr); ctx.textAlign = "left";
    }
    ctx.restore();
    raf = requestAnimationFrame(frame);
  }
  function onMove(e) {
    var r = canvas.getBoundingClientRect(), mx = (e.clientX - r.left) * dpr, my = (e.clientY - r.top) * dpr, best = -1, bd = 14 * dpr;
    jobs.forEach(function (j, i) { var p = project(j.lat, j.lon), d = Math.hypot(p[0] - mx, p[1] - my); if (d < bd) { bd = d; best = i; } });
    hover = best; canvas.style.cursor = best >= 0 ? "pointer" : "default";
  }
  function onClick() { if (hover >= 0 && opts.onPick) { opts.onPick(jobs[hover]); } }
  size(); addEventListener("resize", size, {passive: true});
  canvas.addEventListener("mousemove", onMove); canvas.addEventListener("click", onClick);
  if (matchMedia("(prefers-reduced-motion: reduce)").matches) { frame(t0); cancelAnimationFrame(raf); }
  else { raf = requestAnimationFrame(frame); }
  return function () { cancelAnimationFrame(raf); removeEventListener("resize", size); };
}
`
