package api

import (
	"fmt"
	"html/template"
	"strings"
)

// The interface. One file, no build step, no dependencies: a node you can run
// from a single binary should not need a toolchain to show you anything.

const appCSS = `
:root{--bg:#0B0D10;--panel:#11151A;--panel2:#161B22;--ink:#E8EDF2;--ink2:#9AA7B4;
--ink3:#5F6B77;--rule:#1E252E;--rule2:#2B3541;--gold:#FFB627;--green:#3FCF71;
--sans:system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;
--mono:ui-monospace,"SF Mono",Menlo,Consolas,monospace;color-scheme:dark}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--ink);font:400 15px/1.55 var(--sans);
-webkit-font-smoothing:antialiased}
a{color:var(--gold)}
button,input,textarea,select{font:inherit}
.wrap{display:grid;grid-template-columns:288px 1fr;height:100vh}
@media(max-width:820px){.wrap{grid-template-columns:1fr}.side{display:none}}
.side{border-right:1px solid var(--rule);background:var(--panel);display:flex;
flex-direction:column;min-height:0}
.brand{padding:1rem 1.1rem;border-bottom:1px solid var(--rule);font-weight:650;
letter-spacing:.02em;display:flex;align-items:center;gap:.5rem}
.dot{width:7px;height:7px;border-radius:50%;background:var(--green)}
.tlist{overflow:auto;flex:1;padding:.5rem}
.t{padding:.6rem .7rem;border-radius:7px;cursor:pointer;border:1px solid transparent}
.t:hover{background:var(--panel2)}
.t[aria-selected=true]{background:var(--panel2);border-color:var(--rule2)}
.t h3{margin:0 0 .15rem;font-size:.93rem;font-weight:600;line-height:1.3}
.t p{margin:0;font-size:.75rem;color:var(--ink3);font-family:var(--mono)}
.badge{display:inline-block;margin-left:.35rem;padding:0 .35rem;border-radius:4px;
background:#1D2A1F;color:var(--green);font-size:.68rem;font-family:var(--mono)}
.main{display:flex;flex-direction:column;min-width:0;min-height:0}
.top{padding:.85rem 1.2rem;border-bottom:1px solid var(--rule);display:flex;
align-items:center;gap:.8rem;flex-wrap:wrap}
.top h2{margin:0;font-size:1rem;font-weight:620;flex:1;min-width:10rem}
.btn{background:none;border:1px solid var(--rule2);color:var(--ink);border-radius:7px;
padding:.36rem .75rem;font-size:.84rem;cursor:pointer}
.btn:hover{border-color:var(--gold)}
.btn.go{background:var(--gold);border-color:var(--gold);color:#140E00;font-weight:600}
.feed{flex:1;overflow:auto;padding:1.1rem 1.2rem;min-height:0}
.e{margin:0 0 1rem;padding-left:.8rem;border-left:2px solid var(--rule2);max-width:56rem}
.e.summary{border-left-color:var(--gold)}
.e.control{border-left-color:var(--rule);opacity:.65}
.e .meta{font:.72rem/1.4 var(--mono);color:var(--ink3);margin-bottom:.2rem}
.e .meta b{color:var(--ink2);font-weight:500}
.e .body{white-space:pre-wrap;word-wrap:break-word}
.lane{display:inline-block;padding:0 .3rem;border-radius:3px;background:var(--panel2);
color:var(--ink3);font-size:.68rem}
.composer{border-top:1px solid var(--rule);padding:.8rem 1.2rem;background:var(--panel)}
.composer textarea{width:100%;min-height:3.4rem;resize:vertical;background:var(--bg);
color:var(--ink);border:1px solid var(--rule2);border-radius:8px;padding:.6rem .7rem}
.row{display:flex;gap:.55rem;align-items:center;margin-top:.55rem;flex-wrap:wrap}
.row select{background:var(--bg);color:var(--ink);border:1px solid var(--rule2);
border-radius:7px;padding:.34rem .5rem;font-size:.84rem}
.hint{color:var(--ink3);font-size:.78rem}
.answer{margin:0 0 1rem;padding:.8rem .9rem;border:1px solid var(--rule2);
border-radius:9px;background:var(--panel2);max-width:56rem}
.answer .meta{font:.72rem/1.4 var(--mono);color:var(--ink3);margin-bottom:.35rem}
.empty{color:var(--ink3);padding:2.5rem 1.2rem;max-width:34rem}
.empty h3{color:var(--ink2);font-weight:600;margin:0 0 .5rem}
code{font-family:var(--mono);font-size:.86em;background:var(--panel2);padding:.1rem .3rem;
border-radius:4px}
.notice{max-width:34rem;margin:14vh auto;padding:0 1.5rem}
.notice h1{font-size:1.35rem;margin:0 0 .6rem}
.notice p{color:var(--ink2)}
`

// appHTML is the owner's view.
func appHTML(model string, canAsk bool) string {
	askNote := "No model configured — set LAMDIS_OPENROUTER_KEY to enable asking"
	if canAsk {
		askNote = "Answers come from " + model + ", grounded only in this thread"
	}
	return `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="referrer" content="no-referrer">
<title>Lamdis</title><style>` + appCSS + `</style></head><body>
<div class="wrap">
  <aside class="side">
    <div class="brand"><span class="dot"></span> lamdis</div>
    <div class="tlist" id="tlist"></div>
  </aside>
  <main class="main">
    <div class="top">
      <h2 id="title">Pick a thread</h2>
      <button class="btn" id="share-summary" hidden>Share summary</button>
      <button class="btn" id="share-full" hidden>Share everything</button>
    </div>
    <div class="feed" id="feed">
      <div class="empty">
        <h3>Nothing selected</h3>
        <p>Threads are on the left. Anything you or your agents post lands here,
        signed, in the order it happened.</p>
      </div>
    </div>
    <div class="composer">
      <textarea id="text" placeholder="Write something, or ask a question of this thread…"></textarea>
      <div class="row">
        <select id="lane">
          <option value="content">content — the detail</option>
          <option value="summary">summary — what a counterparty may see</option>
        </select>
        <button class="btn go" id="post">Post</button>
        <button class="btn" id="ask">Ask</button>
        <span class="hint" id="hint">` + template.HTMLEscapeString(askNote) + `</span>
      </div>
    </div>
  </main>
</div>
<script>
"use strict";
var cur = null, self = null;
var $ = function (id) { return document.getElementById(id); };

function esc(s) {
  return String(s == null ? "" : s).replace(/[&<>"]/g, function (c) {
    return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c];
  });
}
function when(ts) {
  if (!ts) { return ""; }
  var d = new Date(ts);
  return isNaN(d) ? ts : d.toLocaleString([], { month: "short", day: "numeric",
    hour: "2-digit", minute: "2-digit" });
}
function api(path, opts) {
  return fetch(path, opts).then(function (r) {
    if (!r.ok) { throw new Error("HTTP " + r.status); }
    return r.json();
  });
}

function threads() {
  return api("/app/api/threads").then(function (d) {
    self = d.self;
    var el = $("tlist");
    if (!d.threads.length) {
      el.innerHTML = '<div class="empty"><h3>No threads yet</h3><p>Make one from an ' +
        'agent, or on the command line with <code>lamdis thread new "title"</code>.</p></div>';
      return;
    }
    el.innerHTML = d.threads.map(function (t) {
      var bits = [t.entries + (t.entries === 1 ? " entry" : " entries")];
      if (t.last) { bits.push(when(t.last)); }
      var badge = t.shared ? '<span class="badge">shared ' + t.shared + "</span>" : "";
      var pend = t.pending ? '<span class="badge">' + t.pending + " waiting</span>" : "";
      return '<div class="t" data-id="' + esc(t.id) + '" role="option" aria-selected="' +
        (t.id === cur) + '"><h3>' + esc(t.title) + badge + pend + "</h3><p>" +
        esc(bits.join(" · ")) + "</p></div>";
    }).join("");
    Array.prototype.forEach.call(el.querySelectorAll(".t"), function (n) {
      n.onclick = function () { open(n.getAttribute("data-id")); };
    });
  });
}

function open(id) {
  cur = id;
  $("share-summary").hidden = false;
  $("share-full").hidden = false;
  threads();
  return api("/app/api/thread/" + encodeURIComponent(id)).then(function (d) {
    $("title").textContent = d.title || "(untitled)";
    if (!d.entries.length) {
      $("feed").innerHTML = '<div class="empty"><h3>Empty thread</h3>' +
        "<p>Post the first thing into it below.</p></div>";
      return;
    }
    $("feed").innerHTML = d.entries.map(function (e) {
      return '<div class="e ' + esc(e.lane) + '"><div class="meta"><b>' + esc(e.who) +
        "</b> · " + esc(when(e.ts)) + ' · <span class="lane">' + esc(e.lane) +
        '</span></div><div class="body">' + esc(e.text) + "</div></div>";
    }).join("");
    $("feed").scrollTop = $("feed").scrollHeight;
  });
}

$("post").onclick = function () {
  var text = $("text").value.trim();
  if (!cur || !text) { return; }
  api("/app/api/post", {
    method: "POST", headers: { "content-type": "application/json" },
    body: JSON.stringify({ thread: cur, text: text, lane: $("lane").value })
  }).then(function () { $("text").value = ""; open(cur); });
};

$("ask").onclick = function () {
  var q = $("text").value.trim();
  if (!cur || !q) { return; }
  $("hint").textContent = "Thinking…";
  api("/app/api/ask", {
    method: "POST", headers: { "content-type": "application/json" },
    body: JSON.stringify({ thread: cur, question: q })
  }).then(function (d) {
    $("hint").textContent = d.model
      ? "Answered from " + d.grounded_in + " entries by " + d.model
      : "";
    var box = document.createElement("div");
    box.className = "answer";
    box.innerHTML = '<div class="meta">' + esc(q) + "</div><div>" +
      esc(d.answer || d.error) + "</div>";
    $("feed").appendChild(box);
    $("feed").scrollTop = $("feed").scrollHeight;
  }).catch(function (e) { $("hint").textContent = String(e.message); });
};

function share(scope) {
  if (!cur) { return; }
  api("/app/api/share", {
    method: "POST", headers: { "content-type": "application/json" },
    body: JSON.stringify({ thread: cur, scope: scope, days: 30 })
  }).then(function (d) {
    var url = location.origin + d.path;
    if (navigator.clipboard) { navigator.clipboard.writeText(url); }
    $("hint").textContent = "Link copied — " + d.lanes.join(" + ") +
      " only, expires in " + d.days + " days";
    window.prompt("Send them this link. They need no account.", url);
  });
}
$("share-summary").onclick = function () { share("summary"); };
$("share-full").onclick = function () { share("read"); };

threads();
setInterval(function () { if (cur) { open(cur); } else { threads(); } }, 15000);
</script></body></html>`
}

// sharedHTML is what a counterparty sees. No composer, no thread list, no way
// to reach anything the link did not name.
func sharedHTML(full bool) string {
	scope := "You are seeing the summary lane of this thread."
	if full {
		scope = "You are seeing the full detail of this thread."
	}
	return `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="referrer" content="no-referrer">
<title>Shared thread — Lamdis</title><style>` + appCSS + `
.wrap{grid-template-columns:1fr}
</style></head><body>
<div class="wrap"><main class="main">
  <div class="top"><h2 id="title">Loading…</h2></div>
  <div class="feed" id="feed"></div>
  <div class="composer"><span class="hint" id="scope">` +
		template.HTMLEscapeString(scope) + ` Read only, and nothing you do here is recorded.</span></div>
</main></div>
<script>
"use strict";
function esc(s){return String(s==null?"":s).replace(/[&<>"]/g,function(c){
  return {"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;"}[c];});}
function when(ts){var d=new Date(ts);return isNaN(d)?ts:
  d.toLocaleString([],{month:"short",day:"numeric",hour:"2-digit",minute:"2-digit"});}
fetch(location.pathname.replace(/\/$/,"")+"/api/thread").then(function(r){
  if(!r.ok){throw new Error("This link is no longer valid.");}
  return r.json();
}).then(function(d){
  document.getElementById("title").textContent=d.title||"Shared thread";
  var rows=d.entries.filter(function(e){return e.lane!=="control";});
  document.getElementById("feed").innerHTML = rows.length ? rows.map(function(e){
    return '<div class="e '+esc(e.lane)+'"><div class="meta"><b>'+esc(e.who)+
      "</b> · "+esc(when(e.ts))+'</div><div class="body">'+esc(e.text)+"</div></div>";
  }).join("") : '<div class="empty"><h3>Nothing shared yet</h3><p>The owner has not '+
    "posted anything into the lanes this link covers.</p></div>";
}).catch(function(e){
  document.getElementById("title").textContent="Not available";
  document.getElementById("feed").innerHTML='<div class="empty"><p>'+esc(e.message)+"</p></div>";
});
</script></body></html>`
}

func appNotice(title, body string) string {
	return `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>` + template.HTMLEscapeString(title) + `</title><style>` + appCSS + `</style></head>
<body><div class="notice"><h1>` + template.HTMLEscapeString(title) + `</h1><p>` +
		template.HTMLEscapeString(body) + `</p></div></body></html>`
}

var appNoToken = appNotice("This node is not yours to open",
	"The web interface is authenticated by a token held in the node's data directory. "+
		"Run the node yourself and it prints the address to open.")

// appTokenLine is what the node prints on start.
func appTokenLine(host, token string) string {
	return fmt.Sprintf("open       http://%s/app?token=%s", strings.TrimPrefix(host, "http://"), token)
}
