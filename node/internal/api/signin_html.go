package api

// signInPageHTML is one field at a time: an address, then a code.
const signInPageHTML = `<!doctype html>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Sign in — Lamdis</title>
<style>` + themeCSS + `
body { display: flex; align-items: center; justify-content: center; min-height: 100vh; }
main { width: 100%; max-width: 24rem; padding: 2rem 1.25rem; }
.mark { display: flex; align-items: center; justify-content: center; gap: .5rem;
  margin-bottom: 1.3rem; font-size: 1.15rem; }
.mark i { width: .55rem; height: .55rem; border-radius: 1px; background: var(--gold);
  box-shadow: 0 0 12px rgba(255,182,39,.6); transform: rotate(45deg); }
.card { padding: 1.6rem 1.5rem 1.4rem; border-color: var(--rule-2);
  box-shadow: 0 30px 80px rgba(0,0,0,.5), inset 0 1px 0 rgba(255,255,255,.04); }
.card .eyebrow { margin-bottom: .6rem; }
h1 { font-size: 1.45rem; margin-bottom: .3rem; }
.lead { margin-bottom: 1.1rem; font-size: .9rem; }
label { display: block; margin-bottom: .4rem; }
input#code {
  font: 700 1.35rem/1 var(--mono); letter-spacing: .28em;
  text-align: center; padding: .85rem 1rem;
}
button.go { width: 100%; height: 2.8rem; margin-top: .9rem; }
.muted { margin: 1rem 0 0; text-align: center; font-size: .82rem; color: var(--ink-3); }
.back { display: block; width: 100%; margin-top: .6rem; padding: .5rem;
  background: none; border: 0; color: var(--ink-3); font-size: .84rem; cursor: pointer; }
.back:hover { color: var(--ink); }
.foot { margin: 1.1rem 0 0; text-align: center; font: 600 .62rem/1.5 var(--mono);
  letter-spacing: .14em; text-transform: uppercase; color: var(--ink-3); }
</style>
<main>
  <a class="mark" href="/board"><i></i><span>lamdis<b style="color:var(--gold)">.</b></span></a>
  <div class="glass card">
    <div id="step1">
      <p class="eyebrow">Operator sign-in</p>
      <h1>Sign in</h1>
      <p class="lead">We will email you a code.</p>
      <label class="label" for="email">Email address</label>
      <input id="email" type="email" autocomplete="email" inputmode="email"
             autocapitalize="off" autocorrect="off" placeholder="you@example.com">
      <button class="btn go" id="send">Send me a code</button>
      <div class="err" id="err1"></div>
      <p class="muted">Signing in creates an account if you do not have one.</p>
    </div>

    <div id="step2" hidden>
      <p class="eyebrow ok" style="color:var(--green)">Code sent</p>
      <h1>Check your email</h1>
      <p class="lead">We sent a code to <b id="sentto"></b>. It is good for fifteen minutes.</p>
      <label class="label" for="code">Code from the email</label>
      <input id="code" type="text" inputmode="numeric" autocomplete="one-time-code"
             maxlength="8" placeholder="00000000">
      <button class="btn go" id="verify" disabled>Sign in</button>
      <div class="err" id="err2"></div>
      <button class="back" id="back">Use a different address</button>
    </div>
  </div>
  <p class="foot">No password to choose or forget.</p>
</main>
<script>
"use strict";
var TICKET = null;
// Where to go afterwards. Only same-origin paths, so this cannot be used to
// bounce somebody to another site after they sign in.
var NEXT = (function () {
  var n = new URLSearchParams(location.search).get("next") || "/board";
  return (n.charAt(0) === "/" && n.charAt(1) !== "/") ? n : "/board";
})();

function show(step) {
  document.getElementById("step1").hidden = step !== 1;
  document.getElementById("step2").hidden = step !== 2;
}

function post(path, body) {
  return fetch(path, {
    method: "POST",
    headers: {"Content-Type": "application/json"},
    body: JSON.stringify(body)
  }).then(function (r) {
    return r.json().then(function (j) { return {ok: r.ok, body: j}; });
  });
}

var sendBtn = document.getElementById("send");
var emailEl = document.getElementById("email");
var codeEl = document.getElementById("code");

function sendCode() {
  var email = emailEl.value.trim();
  var err = document.getElementById("err1");
  err.textContent = "";
  if (!email || email.indexOf("@") < 1) {
    err.textContent = "Enter an email address.";
    return;
  }
  sendBtn.disabled = true;
  sendBtn.textContent = "Sending…";
  post("/v1/auth/start", {email: email}).then(function (res) {
    if (!res.ok) { throw new Error(res.body && res.body.error || "could not send a code"); }
    TICKET = res.body.ticket;
    document.getElementById("sentto").textContent = email;
    show(2);
    codeEl.focus();
  }).catch(function (e) {
    err.textContent = e.message;
  }).then(function () {
    sendBtn.disabled = false;
    sendBtn.textContent = "Send me a code";
  });
}

function verifyCode() {
  var code = codeEl.value.trim();
  var err = document.getElementById("err2");
  var btn = document.getElementById("verify");
  err.textContent = "";
  if (code.length < 6) { err.textContent = "Enter the code from the email."; return; }
  btn.disabled = true;
  btn.textContent = "Signing in…";
  post("/v1/auth/verify", {ticket: TICKET, code: code}).then(function (res) {
    if (!res.ok) { throw new Error(res.body && res.body.error || "that did not work"); }
    // The token is what every later request carries. It expires on its own.
    try {
      localStorage.setItem("lamdis.token", res.body.id_token);
      localStorage.setItem("lamdis.worker", JSON.stringify(
        {id: res.body.worker, verified: true, enrolled: false}));
    } catch (e) {}
    window.location.href = NEXT;
  }).catch(function (e) {
    err.textContent = e.message;
    btn.disabled = false;
    btn.textContent = "Sign in";
  });
}

sendBtn.addEventListener("click", sendCode);
document.getElementById("verify").addEventListener("click", verifyCode);
document.getElementById("back").addEventListener("click", function () {
  TICKET = null; codeEl.value = ""; show(1); emailEl.focus();
});
emailEl.addEventListener("keydown", function (e) { if (e.key === "Enter") sendCode(); });
codeEl.addEventListener("keydown", function (e) { if (e.key === "Enter") verifyCode(); });
// Codes are not one fixed length: Cognito sends 8 characters for a sign-in
// challenge and 6 for a first-time confirmation. Truncating to 6 silently
// mangled every 8-character code and reported it as wrong, which is the worst
// possible failure — the person is holding the correct code and being told it
// is not.
//
// So: keep up to 8, never cut a code short, and only submit on its own when it
// is unambiguously complete.
codeEl.addEventListener("input", function () {
  var cleaned = codeEl.value.replace(/\D/g, "").slice(0, 8);
  if (cleaned !== codeEl.value) { codeEl.value = cleaned; }
  document.getElementById("verify").disabled = cleaned.length < 6;
  if (cleaned.length === 8) { verifyCode(); }
});
emailEl.focus();
</script>
`
