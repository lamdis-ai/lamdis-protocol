package api

import (
	"encoding/json"
	"html/template"
	"strings"
)

// The hosted page is the same application, with a sign-in step in front of
// it. The script proves who you are against the user pool, then hands the
// resulting token to every call the application makes. The application below
// is unchanged and does not know a front door exists.

// faviconSVG is the mark on a cream tile, generated from brand/mark.py.
const faviconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="7" fill="#FFF9EF"/><g transform="translate(9.6 1.2) scale(0.95) translate(-1 -4.6)" fill="none" stroke-linecap="round" stroke-linejoin="round"><path d="M6 5.6L10 8L6 10.4L2 8Z M10 23.9L14 26.3L10 28.7L6 26.3Z" fill="#FFB22E"/><path d="M2 8L2 13.3 M2 8L6 5.6 M2 8L6 10.4 M2 13.3L2 18.6 M2 13.3L6 15.7 M2 18.6L2 23.9 M2 18.6L6 21 M2 23.9L2 29.2 M2 23.9L6 26.3 M2 29.2L6 31.6 M6 5.6L10 8 M6 10.4L6 15.7 M6 10.4L10 8 M6 15.7L6 21 M6 15.7L10 13.3 M6 21L6 26.3 M6 21L10 18.6 M6 26.3L6 31.6 M6 26.3L10 23.9 M6 26.3L10 28.7 M6 31.6L10 34 M10 8L10 13.3 M10 13.3L10 18.6 M10 18.6L10 23.9 M10 23.9L14 26.3 M10 28.7L10 34 M10 28.7L14 26.3 M10 34L14 31.6 M14 26.3L14 31.6" stroke="#1C1A17" stroke-width="1.3"/></g></svg>`

const hostMark = `<svg viewBox="1 4.6 14 30.4" fill="none" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" style="width:24px;height:52px;color:var(--ink,#1C1A17)"><path d="M6 5.6L10 8L6 10.4L2 8Z M10 23.9L14 26.3L10 28.7L6 26.3Z" fill="#FFB22E"/><path d="M2 8L2 13.3 M2 8L6 5.6 M2 8L6 10.4 M2 13.3L2 18.6 M2 13.3L6 15.7 M2 18.6L2 23.9 M2 18.6L6 21 M2 23.9L2 29.2 M2 23.9L6 26.3 M2 29.2L6 31.6 M6 5.6L10 8 M6 10.4L6 15.7 M6 10.4L10 8 M6 15.7L6 21 M6 15.7L10 13.3 M6 21L6 26.3 M6 21L10 18.6 M6 26.3L6 31.6 M6 26.3L10 23.9 M6 26.3L10 28.7 M6 31.6L10 34 M10 8L10 13.3 M10 13.3L10 18.6 M10 18.6L10 23.9 M10 23.9L14 26.3 M10 28.7L10 34 M10 28.7L14 26.3 M10 34L14 31.6 M14 26.3L14 31.6" stroke="currentColor" stroke-width="1"/></svg>`

const hostCSS = `
/* A class that sets display beats the hidden attribute, so say it once. */
[hidden]{display:none!important}
.gate{position:fixed;inset:0;z-index:60;display:grid;place-items:center;background:var(--bg);padding:1.5rem}
.gate .box{max-width:23rem;text-align:center;animation:rise .4s both}
@media(max-width:520px){.gate{padding:1.1rem}.gate h1{font-size:1.25rem}}
.gate .glyphwrap{display:flex;justify-content:center;margin-bottom:1.4rem}
.gate h1{font-family:var(--display);font-size:2.4rem;font-weight:800;letter-spacing:-.035em;line-height:1.05;margin-bottom:.6rem}
.gate p{color:var(--ink3);font-size:.95rem;line-height:1.6;margin-bottom:1.5rem}
.gate .btn{width:100%;justify-content:center;padding:.7rem 1rem}
.gate .err{color:var(--red);font-size:.84rem;margin-top:.9rem;min-height:1.2rem}
.gate .gform{display:flex;flex-direction:column;gap:.5rem;margin-bottom:.5rem}
.gate .gform input{font:inherit;font-size:1rem;padding:.7rem .85rem;border:1px solid var(--line);border-radius:12px;background:var(--card,#fff);color:var(--ink);text-align:center}
.gate .gform input:focus{outline:2px solid var(--gold);border-color:transparent}
.gate #code,.gate #dev{letter-spacing:.3em;font-weight:600}
.gate #g-main>.btn{margin-top:.5rem}
.gate .or{display:flex;align-items:center;gap:.7rem;color:var(--ink4);font-size:.78rem;margin:.9rem 0 .2rem}
.gate .or:before,.gate .or:after{content:"";flex:1;height:1px;background:var(--line)}
.gate .lnk{background:none;border:none;color:var(--ink3);font:inherit;font-size:.84rem;cursor:pointer;text-decoration:underline}
.gate .sent{margin:0 0 .3rem;font-size:.88rem}
.gate .paircode{font-family:ui-monospace,Menlo,monospace;font-size:2.2rem;font-weight:700;letter-spacing:.12em;padding:1rem;border-radius:16px;background:var(--gold-glow);color:var(--ink);margin-bottom:1rem}
.gate .pairlink{font-family:ui-monospace,Menlo,monospace;font-size:.78rem;word-break:break-all;color:var(--ink2)}
.gate .fine{color:var(--ink4);font-size:.78rem;margin-top:1.6rem;line-height:1.6}
.booting{position:fixed;inset:0;z-index:60;display:grid;place-items:center;background:var(--bg);color:var(--ink4);font:.8rem var(--mono)}
.who{border-top:1px solid var(--line);padding:.6rem .4rem 0;font-size:.76rem;color:var(--ink4);display:flex;gap:.6rem;align-items:center}
.who b{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-weight:500}
.who button{color:var(--gold-text);font-size:.76rem;font-weight:500}
.who button:hover{color:var(--ink2)}
`

// hostedAppHTML renders the signed-in application shell.
func hostedAppHTML(cfg SignIn) string {
	c, _ := json.Marshal(map[string]string{
		"domain":   strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(cfg.Domain, "https://"), "http://"), "/"),
		"clientId": cfg.ClientID,
		"redirect": cfg.Redirect,
	})
	return `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><meta name="referrer" content="no-referrer"><meta name="color-scheme" content="light">
<title>Lamdis</title><link rel="icon" href="/favicon.svg" type="image/svg+xml">
<link rel="preconnect" href="https://fonts.googleapis.com"><link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Bricolage+Grotesque:opsz,wght@12..96,600;12..96,800&family=Figtree:wght@400;500;600;700&display=swap" rel="stylesheet"><style>` + appCSS + hostCSS + `</style></head><body>

<div class="gate" id="gate" hidden>
  <div class="box">
    <div class="glyphwrap">` + hostMark + `</div>
    <h1 id="gate-h">Use Lamdis on every device</h1>
    <p id="gate-p">Add your email and sign in anywhere with a code we send you. No password. Your channels stay exactly where they are.</p>
    <div id="g-main">
      <form id="em-form" class="gform"><input id="em" type="email" placeholder="you@example.com" autocomplete="email" required><button class="btn solid">Continue with email</button></form>
      <form id="code-form" class="gform" hidden><p class="sent" id="sent"></p><input id="code" inputmode="numeric" autocomplete="one-time-code" placeholder="6-digit code" maxlength="6" required><button class="btn solid" id="code-go">Sign in</button><button type="button" class="lnk" id="em-back">Use a different address</button></form>
      <div class="or"><span>or</span></div>
      <button class="btn" id="pk-login">Sign in with a passkey</button>
      <button class="btn" id="pk-save">Save this account with a passkey</button>
      <button class="btn" id="dev-open">I have a code from my other device</button>
      <form id="dev-form" class="gform" hidden><input id="dev" placeholder="ABCD-2345" autocapitalize="characters" autocomplete="off" required><button class="btn solid">Sign in on this device</button></form>
    </div>
    <div id="g-pair" hidden>
      <div class="paircode" id="pair-code">····-····</div>
      <p class="sent">Or open this link on the other device:<br><span class="pairlink" id="pair-link"></span></p>
    </div>
    <button class="btn ghost" id="back" style="border:none">Not now</button>
    <div class="err" id="err"></div>
    <div class="fine" id="gate-fine">Every device you sign in on is the same you: the same channels, agents and invitations.</div>
  </div>
</div>
<div class="booting" id="booting">opening your record…</div>

` + strings.Replace(appShell(`<div class="who"><b id="who-email"></b><button id="pair" hidden>Add a device</button><button id="signout">Sign out</button></div>`), `<div class="shell" id="shell">`, `<div class="shell" id="shell" hidden>`, 1) + `

<script>
"use strict";
var CFG = ` + string(c) + `;
var KEY = "lamdis.session";
var sess = null;
var retriedStart = false;

function saveSession(s){ sess = s; try{ localStorage.setItem(KEY, JSON.stringify(s)) }catch(e){} }
function loadSession(){ try{ return JSON.parse(localStorage.getItem(KEY) || "null") }catch(e){ return null } }
function clearSession(){ sess = null; try{ localStorage.removeItem(KEY) }catch(e){} }

function b64url(buf){
  var bin = "", b = new Uint8Array(buf);
  for (var i = 0; i < b.length; i++) bin += String.fromCharCode(b[i]);
  return btoa(bin).replace(/\+/g,"-").replace(/\//g,"_").replace(/=+$/,"");
}
function randHex(n){
  var a = new Uint8Array(n); crypto.getRandomValues(a);
  return Array.prototype.map.call(a, function(x){ return ("0"+x.toString(16)).slice(-2) }).join("");
}
function challenge(v){
  return crypto.subtle.digest("SHA-256", new TextEncoder().encode(v)).then(b64url);
}

function signIn(){
  if(!CFG.domain || !CFG.clientId){ document.getElementById("err").textContent = "This host has no sign-in configured yet."; return }
  var v = randHex(48);
  try{ sessionStorage.setItem("lamdis.pkce", v) }catch(e){}
  challenge(v).then(function(c){
    location.href = "https://" + CFG.domain + "/oauth2/authorize"
      + "?response_type=code&client_id=" + encodeURIComponent(CFG.clientId)
      + "&redirect_uri=" + encodeURIComponent(CFG.redirect)
      + "&scope=" + encodeURIComponent("openid email")
      + "&code_challenge=" + c + "&code_challenge_method=S256";
  });
}
function signOut(){
  clearSession();
  if(CFG.domain && CFG.clientId){
    location.href = "https://" + CFG.domain + "/logout?client_id=" + encodeURIComponent(CFG.clientId)
      + "&logout_uri=" + encodeURIComponent(CFG.redirect);
    return;
  }
  location.reload();
}

function tokenCall(body){
  return fetch("https://" + CFG.domain + "/oauth2/token", {
    method: "POST", headers: {"content-type": "application/x-www-form-urlencoded"}, body: new URLSearchParams(body)
  }).then(function(r){ return r.json().then(function(d){ if(!r.ok) throw new Error(d.error_description || d.error || ("HTTP "+r.status)); return d }) });
}
function store(d){
  var claims = {};
  try{ claims = JSON.parse(atob(d.id_token.split(".")[1].replace(/-/g,"+").replace(/_/g,"/"))) }catch(e){}
  saveSession({ id: d.id_token, refresh: d.refresh_token || (sess && sess.refresh),
    exp: Date.now() + ((d.expires_in || 3600) - 60) * 1000, email: claims.email || "" });
}
function exchangeCode(code){
  var v = "";
  try{ v = sessionStorage.getItem("lamdis.pkce") || "" }catch(e){}
  return tokenCall({ grant_type: "authorization_code", client_id: CFG.clientId, code: code,
    redirect_uri: CFG.redirect, code_verifier: v }).then(store);
}
function refresh(){
  if(!sess || !sess.refresh) return Promise.reject(new Error("no session"));
  return tokenCall({ grant_type: "refresh_token", client_id: CFG.clientId, refresh_token: sess.refresh }).then(store);
}
function fresh(){
  if(sess && Date.now() < sess.exp) return Promise.resolve();
  return refresh();
}

// Every call the application makes carries whoever this is. It does not know.
var rawFetch = window.fetch.bind(window);
window.fetch = function(p, o){
  if(typeof p !== "string" || p.indexOf("/app/api") !== 0) return rawFetch(p, o);
  var send = function(){
    var opts = Object.assign({}, o || {});
    var h = Object.assign({}, opts.headers || {});
    h.Authorization = "Bearer " + bearer();
    var g = guestToken();
    if(g) h["X-Lamdis-Guest"] = g;
    opts.headers = h;
    return rawFetch(p, opts);
  };
  // A visitor token the server no longer recognises should start a new
  // node rather than leave somebody staring at a broken page. It happens
  // if the host is restored from a backup, or its signing key is rotated.
  if(!sess || sess.guest){
    return send().then(function(r){
      if(r.status !== 401 || retriedStart) return r;
      retriedStart = true;
      return startAsGuest().then(function(){ return send() }, function(){ return r });
    });
  }
  return fresh().then(send, send).then(function(r){
    if(r.status !== 401) return r;
    return refresh().then(send, function(){ return r });
  });
};
function bearer(){ return sess ? (sess.id || sess.guest || "") : "" }
function guestToken(){ try{ return localStorage.getItem("lamdis.guest") || "" }catch(e){ return "" } }

function show(what){
  document.getElementById("booting").hidden = what !== "booting";
  document.getElementById("gate").hidden = what !== "gate";
  document.getElementById("shell").hidden = what !== "app";
}

// Starting is the whole point: no question is asked before the first one
// the person wants to ask.
function startAsGuest(){
  try{ localStorage.removeItem("lamdis.guest") }catch(e){}
  return rawFetch("/app/api/start", {method:"POST"}).then(function(r){ return r.json() }).then(function(d){
    if(d.error) throw new Error(d.error);
    try{ localStorage.setItem("lamdis.guest", d.token) }catch(e){}
    saveSession({ guest: d.token });
  });
}

function boot(){
  show("app");
  var signedIn = sess && sess.id;
  var who = document.getElementById("who-email"), b = document.getElementById("signout");
  who.textContent = signedIn ? (sess.email || "signed in") : "Not saved yet";
  b.textContent = signedIn ? "Sign out" : "Keep this";
  b.onclick = signedIn ? signOut : function(){ gateMode("keep") };
  // Say whose account this is and how it is kept, and offer the other ways.
  if(!signedIn) whoami().then(function(d){
    if(!d || !d.kept) return;
    who.textContent = "@" + d.handle + (d.email ? " · " + d.email : " · passkey");
    b.textContent = "Sign out"; b.onclick = passkeySignOut;
    var pb = document.getElementById("pair");
    if(pb){ pb.hidden = false; pb.onclick = pairDevice }
  });
  if(!window._appLoaded){
    window._appLoaded = true;
    var el = document.createElement("script");
    el.src = "/app/app.js";
    document.body.appendChild(el);
  }
}


/* ---- passkeys: WebAuthn wants bytes, the server speaks base64url ---- */
function b64u(s){ s = s.replace(/-/g,"+").replace(/_/g,"/"); while(s.length % 4) s += "="; var b = atob(s), a = new Uint8Array(b.length); for(var i=0;i<b.length;i++) a[i] = b.charCodeAt(i); return a.buffer }
function u64(buf){ var b = new Uint8Array(buf), s = ""; for(var i=0;i<b.length;i++) s += String.fromCharCode(b[i]); return btoa(s).replace(/\+/g,"-").replace(/\//g,"_").replace(/=+$/,"") }
function creationOptions(o){ var p = o.publicKey; p.challenge = b64u(p.challenge); p.user.id = b64u(p.user.id);
  (p.excludeCredentials||[]).forEach(function(c){ c.id = b64u(c.id) }); return { publicKey: p } }
function requestOptions(o){ var p = o.publicKey; p.challenge = b64u(p.challenge);
  (p.allowCredentials||[]).forEach(function(c){ c.id = b64u(c.id) }); return { publicKey: p } }
function credJSON(c){
  var r = c.response, out = { id: c.id, rawId: u64(c.rawId), type: c.type, response: { clientDataJSON: u64(r.clientDataJSON) } };
  if(r.attestationObject){ out.response.attestationObject = u64(r.attestationObject); if(r.getTransports) out.response.transports = r.getTransports() }
  if(r.authenticatorData){ out.response.authenticatorData = u64(r.authenticatorData); out.response.signature = u64(r.signature); if(r.userHandle) out.response.userHandle = u64(r.userHandle) }
  if(c.authenticatorAttachment) out.authenticatorAttachment = c.authenticatorAttachment;
  out.clientExtensionResults = c.getClientExtensionResults ? c.getClientExtensionResults() : {};
  return out }
function gateErr(t){ document.getElementById("err").textContent = t || "" }
function noPasskeys(){ if(!window.PublicKeyCredential){ gateErr("This browser cannot use passkeys. Try Safari, Chrome, Edge or Firefox on a recent device."); return true } return false }

function passkeySave(){
  if(noPasskeys()) return; gateErr("");
  fetch("/app/api/passkey/register/begin", {method:"POST"}).then(function(r){ return r.json() }).then(function(d){
    if(d.error) throw new Error(d.error);
    return navigator.credentials.create(creationOptions(d.options)).then(function(c){
      return fetch("/app/api/passkey/register/finish?ceremony=" + encodeURIComponent(d.ceremony), {method:"POST", headers:{"content-type":"application/json"}, body: JSON.stringify(credJSON(c))})
    })
  }).then(function(r){ return r.json() }).then(function(d){
    if(d.error) throw new Error(d.error);
    boot();
  }).catch(function(e){ gateErr(e && e.name === "NotAllowedError" ? "Cancelled. Press the button again when you're ready." : (e.message || "That did not work")) });
}

function passkeyLogin(){
  if(noPasskeys()) return; gateErr("");
  rawFetch("/app/api/passkey/login/begin", {method:"POST"}).then(function(r){ return r.json() }).then(function(d){
    if(d.error) throw new Error(d.error);
    return navigator.credentials.get(requestOptions(d.options)).then(function(c){
      return rawFetch("/app/api/passkey/login/finish?ceremony=" + encodeURIComponent(d.ceremony), {method:"POST", headers:{"content-type":"application/json"}, body: JSON.stringify(credJSON(c))})
    })
  }).then(function(r){ return r.json() }).then(function(d){
    if(d.error) throw new Error(d.error);
    try{ localStorage.setItem("lamdis.guest", d.token) }catch(e){}
    saveSession({ guest: d.token });
    location.href = location.pathname + "#today"; location.reload();
  }).catch(function(e){ gateErr(e && e.name === "NotAllowedError" ? "Cancelled, or no passkey for this site on this device." : (e.message || "That did not work")) });
}

function passkeySignOut(){
  clearSession(); try{ localStorage.removeItem("lamdis.guest") }catch(e){}
  location.reload();
}
/* ---- one person, every device ---- */
function whoami(){ return fetch("/app/api/whoami").then(function(r){ return r.ok ? r.json() : null }).catch(function(){ return null }) }
var emailAddr = "";
// useToken signs this device in as the account behind tok and reloads,
// keeping an invitation in the address so it is accepted by that account.
function useToken(tok){
  try{ localStorage.setItem("lamdis.guest", tok) }catch(e){}
  saveSession({ guest: tok });
  var q = new URLSearchParams(location.search); q.delete("device");
  var qs = q.toString();
  location.replace(location.pathname + (qs ? "?" + qs : "") + (location.hash || "#today"));
  if(qs || location.hash) location.reload();
}
function gateMode(mode){
  var h = document.getElementById("gate-h"), p = document.getElementById("gate-p"), back = document.getElementById("back");
  document.getElementById("g-main").hidden = mode === "pair";
  document.getElementById("g-pair").hidden = mode !== "pair";
  document.getElementById("pk-save").hidden = mode !== "keep";
  document.getElementById("gate-fine").hidden = mode === "pair";
  document.getElementById("em-form").hidden = false; document.getElementById("code-form").hidden = true;
  document.getElementById("dev-form").hidden = true; gateErr("");
  if(mode === "join"){
    h.textContent = "You're invited";
    p.textContent = "Sign in first, so the channel lands in your account and follows you to every device. New here? Your email is all it takes.";
    back.textContent = "Join as a guest on this device";
  } else if(mode === "pair"){
    h.textContent = "Open Lamdis on another device";
    p.textContent = "On your phone or other computer, open the Lamdis app or app.lamdis.ai, choose Sign in, then \u201cI have a code from my other device\u201d, and type:";
    back.textContent = "Done";
  } else {
    h.textContent = "Use Lamdis on every device";
    p.textContent = "Add your email and sign in anywhere with a code we send you. No password. Your channels stay exactly where they are.";
    back.textContent = "Not now";
  }
  show("gate");
}
function pairDevice(){
  gateMode("pair");
  document.getElementById("pair-code").textContent = "····-····";
  fetch("/app/api/device/code", {method:"POST"}).then(function(r){ return r.json() }).then(function(d){
    if(d.error) throw new Error(d.error);
    document.getElementById("pair-code").textContent = d.code;
    document.getElementById("pair-link").textContent = d.url;
  }).catch(function(e){ gateErr(e.message) });
}
document.getElementById("em-form").onsubmit = function(ev){
  ev.preventDefault(); gateErr("");
  emailAddr = document.getElementById("em").value.trim();
  fetch("/app/api/signin/email", {method:"POST", headers:{"content-type":"application/json"}, body: JSON.stringify({email: emailAddr})})
    .then(function(r){ return r.json() }).then(function(d){
      if(d.error) throw new Error(d.error);
      if(d.already){ boot(); return }
      document.getElementById("em-form").hidden = true; document.getElementById("code-form").hidden = false;
      document.getElementById("sent").textContent = (d.existing ? "Welcome back. " : "") + "We sent a code to " + d.sent_to + ".";
      document.getElementById("code-go").textContent = d.existing ? "Sign in" : "Confirm";
      document.getElementById("code").focus();
    }).catch(function(e){ gateErr(e.message) });
};
document.getElementById("em-back").onclick = function(){ document.getElementById("em-form").hidden = false; document.getElementById("code-form").hidden = true };
document.getElementById("code-form").onsubmit = function(ev){
  ev.preventDefault(); gateErr("");
  fetch("/app/api/signin/email/confirm", {method:"POST", headers:{"content-type":"application/json"}, body: JSON.stringify({email: emailAddr, code: document.getElementById("code").value})})
    .then(function(r){ return r.json() }).then(function(d){
      if(d.error) throw new Error(d.error);
      if(d.token){ useToken(d.token); return }
      // The address is now on this account: carry on where they were.
      var q = new URLSearchParams(location.search);
      if(q.get("join")) location.reload(); else boot();
    }).catch(function(e){ gateErr(e.message) });
};
document.getElementById("dev-open").onclick = function(){ document.getElementById("dev-form").hidden = false; document.getElementById("dev").focus() };
document.getElementById("dev-form").onsubmit = function(ev){
  ev.preventDefault(); redeemDevice(document.getElementById("dev").value);
};
function redeemDevice(code){
  gateErr("");
  return rawFetch("/app/api/device/redeem", {method:"POST", headers:{"content-type":"application/json"}, body: JSON.stringify({code: code})})
    .then(function(r){ return r.json() }).then(function(d){
      if(d.error) throw new Error(d.error);
      useToken(d.token);
    }).catch(function(e){ gateMode("signin"); gateErr(e.message) });
}

document.getElementById("pk-save").onclick = passkeySave;
document.getElementById("pk-login").onclick = passkeyLogin;
document.getElementById("back").onclick = function(){ boot() };

sess = loadSession();
(function start(){
  var q = new URLSearchParams(location.search);
  if(q.get("error")){ show("gate"); document.getElementById("err").textContent = q.get("error_description") || q.get("error"); return }
  if(q.get("code")){
    exchangeCode(q.get("code")).then(function(){
      history.replaceState({}, "", location.pathname);
      boot();
    }).catch(function(e){ show("gate"); document.getElementById("err").textContent = e.message });
    return;
  }
  if(q.get("device")){ redeemDevice(q.get("device")); return }
  // An invitation opened on a device that is not signed in: ask who this
  // is first, so the channel joins their real account, not a fresh guest.
  var arrive = function(){
    if(!q.get("join")){ boot(); return }
    whoami().then(function(d){ if(d && d.kept) boot(); else gateMode("join") });
  };
  if(sess && sess.id){ fresh().then(boot, function(){ clearSession(); startAsGuest().then(boot, failed) }); return }
  if(sess && sess.guest){ arrive(); return }
  var g = guestToken();
  if(g){ saveSession({ guest: g }); arrive(); return }
  startAsGuest().then(arrive, failed);
})();

function failed(e){
  document.getElementById("booting").textContent =
    (e && e.message) ? e.message : "could not start; try again in a moment";
}
</script></body></html>`
}

// hostNotice is the plain page for a host that is not configured yet.
func hostNotice(title, body string) string {
	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="light">
<title>` + template.HTMLEscapeString(title) + `</title><style>` + appCSS + `</style></head>
<body><div class="notice"><h1>` + template.HTMLEscapeString(title) + `</h1><p>` + template.HTMLEscapeString(body) + `</p></div></body></html>`
}
