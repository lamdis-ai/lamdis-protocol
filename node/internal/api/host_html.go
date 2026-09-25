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

const hostMark = `<svg viewBox="0 0 20 22" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round" aria-hidden="true" style="width:26px;height:28px;color:var(--gold-text)"><path d="M2 5.5 7 3l5 2.5-5 2.5z M2 5.5v4.6l5 2.5V8 M12 5.5v4.6L7 12.6"/><path d="M2 10.1v4.6l5 2.5v-4.6 M12 10.1v4.6l-5 2.5"/><path d="M7 17.2l5-2.5 5 2.5-5 2.5z M12 19.7v2 M17 17.2v2l-5 2.5 M2 14.7l5 2.5"/></svg>`

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
<title>Lamdis</title><link rel="icon" href="/favicon.ico">
<link rel="preconnect" href="https://fonts.googleapis.com"><link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Bricolage+Grotesque:opsz,wght@12..96,600;12..96,800&family=Figtree:wght@400;500;600;700&display=swap" rel="stylesheet"><style>` + appCSS + hostCSS + `</style></head><body>

<div class="gate" id="gate" hidden>
  <div class="box">
    <div class="glyphwrap">` + hostMark + `</div>
    <h1 id="gate-h">Keep your account</h1>
    <p id="gate-p">Save it with a passkey: Face ID, Touch ID, your phone or a security key. Then sign in on any device with the same passkey. No password, and nothing here that could leak one.</p>
    <button class="btn solid" id="pk-save">Save with a passkey</button>
    <button class="btn" id="pk-login" style="width:100%;justify-content:center;margin-top:.5rem">I already have an account: sign in</button>
    <button class="btn" id="go" style="width:100%;justify-content:center;margin-top:.5rem" hidden>Continue with email</button>
    <button class="btn ghost" id="back" style="width:100%;justify-content:center;margin-top:.5rem;border:none">Not now</button>
    <div class="err" id="err"></div>
    <div class="fine">Your channels stay exactly where they are. The passkey stays on your device; this site keeps only its public half.</div>
  </div>
</div>
<div class="booting" id="booting">opening your record…</div>

` + strings.Replace(appShell(`<div class="who"><b id="who-email"></b><button id="signout">Sign out</button></div>`), `<div class="shell" id="shell">`, `<div class="shell" id="shell" hidden>`, 1) + `

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
  b.onclick = signedIn ? signOut : function(){ show("gate") };
  // Kept with a passkey: say whose account this is, and offer sign-out.
  if(!signedIn){
    fetch("/app/api/passkey").then(function(r){ return r.json() }).then(function(d){
      if(d && d.passkeys > 0){ who.textContent = "@" + d.handle + " · passkey"; b.textContent = "Sign out"; b.onclick = passkeySignOut }
    }).catch(function(){});
  }
  if(!window._appLoaded){
    window._appLoaded = true;
    var el = document.createElement("script");
    el.src = "/app/app.js";
    document.body.appendChild(el);
  }
}

document.getElementById("go").onclick = signIn;
document.getElementById("go").hidden = !(CFG.domain && CFG.clientId);

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
document.getElementById("pk-save").onclick = passkeySave;
document.getElementById("pk-login").onclick = passkeyLogin;
document.getElementById("back").onclick = function(){ show("app") };

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
  if(sess && sess.id){ fresh().then(boot, function(){ clearSession(); startAsGuest().then(boot, failed) }); return }
  if(sess && sess.guest){ boot(); return }
  var g = guestToken();
  if(g){ saveSession({ guest: g }); boot(); return }
  startAsGuest().then(boot, failed);
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
