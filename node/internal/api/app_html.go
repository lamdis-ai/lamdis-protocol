package api

import (
	"html/template"
)

// The interface.
//
// One file, no build step, no dependencies: a node that runs from a single
// binary should not need a toolchain to show you anything.
//
// The design carries one idea, which is the product's whole idea — a thread has
// layers, and you decide which layer somebody else gets. So lanes are not a
// dropdown detail here. They are the visual language: what you keep is plain,
// what you are willing to share glows, and before you send a link you see the
// other person's view of your own thread, side by side with yours.

const appCSS = `
:root{
  --bg:#08090C; --bg2:#0C0E13; --panel:#111319; --panel2:#161922; --raise:#1B1F2A;
  --ink:#F2F5F9; --ink2:#A8B3C2; --ink3:#69748A; --ink4:#434C5E;
  --line:#191D26; --line2:#242A36;
  --gold:#FFC043; --gold-dim:#8A6413; --gold-glow:rgba(255,192,67,.13);
  --green:#4ADE80; --red:#F87171;
  --sans:'Inter var','Inter',-apple-system,BlinkMacSystemFont,'Segoe UI',system-ui,sans-serif;
  --mono:'SF Mono',ui-monospace,'JetBrains Mono',Menlo,monospace;
  --r:12px; --r2:16px;
  color-scheme:dark;
}
*{box-sizing:border-box;margin:0;padding:0}
html,body{height:100%}
body{background:var(--bg);color:var(--ink);font-family:var(--sans);
  font-size:15px;line-height:1.6;-webkit-font-smoothing:antialiased;
  text-rendering:optimizeLegibility}
button,input,textarea,select{font:inherit;color:inherit}
button{cursor:pointer;background:none;border:none}
::selection{background:var(--gold-glow);color:#fff}
::-webkit-scrollbar{width:10px;height:10px}
::-webkit-scrollbar-thumb{background:var(--line2);border-radius:99px;
  border:3px solid transparent;background-clip:content-box}
::-webkit-scrollbar-thumb:hover{background:var(--ink4);background-clip:content-box}

.shell{display:grid;grid-template-columns:320px minmax(0,1fr);height:100vh}
@media(max-width:860px){.shell{grid-template-columns:1fr}.rail{display:none}}

/* ---- rail ---- */
.rail{background:var(--bg2);border-right:1px solid var(--line);
  display:flex;flex-direction:column;min-height:0}
.mark{display:flex;align-items:center;gap:.6rem;padding:1.4rem 1.5rem 1.1rem;
  font-size:1.02rem;font-weight:640;letter-spacing:-.012em}
.mark .glyph{width:22px;height:22px;border-radius:7px;flex:none;
  background:linear-gradient(145deg,var(--gold),#C2820E);
  box-shadow:0 0 0 1px rgba(255,192,67,.25),0 4px 14px -4px rgba(255,192,67,.5)}
.mark small{margin-left:auto;font:500 .7rem/1 var(--mono);color:var(--ink4);
  letter-spacing:.06em;text-transform:uppercase}

.railhead{padding:0 1.5rem .7rem;font:600 .68rem/1 var(--mono);
  letter-spacing:.14em;text-transform:uppercase;color:var(--ink4)}
.threads{flex:1;overflow-y:auto;padding:0 .75rem 1rem;display:flex;
  flex-direction:column;gap:2px}
.thread{width:100%;text-align:left;padding:.75rem .8rem;border-radius:var(--r);
  border:1px solid transparent;transition:background .14s,border-color .14s;
  display:block;position:relative}
.thread:hover{background:var(--panel)}
.thread[aria-current=true]{background:var(--panel2);border-color:var(--line2)}
.thread[aria-current=true]:before{content:"";position:absolute;left:0;top:14px;
  bottom:14px;width:2px;border-radius:2px;background:var(--gold)}
.thread .name{font-size:.925rem;font-weight:560;letter-spacing:-.008em;
  line-height:1.35;margin-bottom:.22rem;
  display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden}
.thread .sub{font:.74rem/1.4 var(--mono);color:var(--ink4);
  display:flex;align-items:center;gap:.45rem;flex-wrap:wrap}
.pip{display:inline-flex;align-items:center;gap:.28rem;padding:.08rem .38rem;
  border-radius:5px;font-size:.68rem;font-weight:500;letter-spacing:.01em}
.pip.share{background:rgba(74,222,128,.1);color:var(--green)}
.pip.wait{background:var(--gold-glow);color:var(--gold)}

/* ---- main ---- */
.main{display:flex;flex-direction:column;min-width:0;min-height:0;background:var(--bg)}
.head{display:flex;align-items:center;gap:1rem;padding:1.15rem 2rem;
  border-bottom:1px solid var(--line);background:rgba(8,9,12,.82);
  backdrop-filter:saturate(1.6) blur(14px);position:sticky;top:0;z-index:5}
.head h1{font-size:1.06rem;font-weight:620;letter-spacing:-.016em;flex:1;
  min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.head .who{font:.72rem/1 var(--mono);color:var(--ink4)}

.btn{border:1px solid var(--line2);border-radius:9px;padding:.44rem .82rem;
  font-size:.85rem;font-weight:500;color:var(--ink2);
  transition:color .14s,border-color .14s,background .14s}
.btn:hover{color:var(--ink);border-color:var(--ink4);background:var(--panel)}
.btn.key{border-color:var(--gold-dim);color:var(--gold)}
.btn.key:hover{background:var(--gold-glow);border-color:var(--gold)}
.btn.solid{background:var(--gold);border-color:var(--gold);color:#120C00;font-weight:600}
.btn.solid:hover{background:#FFCF6B;border-color:#FFCF6B}
.btn[disabled]{opacity:.4;pointer-events:none}

.feed{flex:1;overflow-y:auto;padding:2rem 2rem 1rem;scroll-behavior:smooth}
.stream{max-width:46rem;margin:0 auto;display:flex;flex-direction:column;gap:1.5rem}

.entry{display:grid;grid-template-columns:auto minmax(0,1fr);gap:0 1rem;
  animation:rise .3s cubic-bezier(.2,.8,.3,1) both}
@keyframes rise{from{opacity:0;transform:translateY(6px)}to{opacity:1;transform:none}}
.entry .spine{width:2px;border-radius:2px;background:var(--line2);grid-row:1/3}
.entry.summary .spine{background:linear-gradient(var(--gold),var(--gold-dim));
  box-shadow:0 0 12px -2px var(--gold-glow)}
.entry.control{opacity:.5}
.entry.control .spine{background:var(--line)}
.entry .meta{display:flex;align-items:center;gap:.6rem;margin-bottom:.35rem;
  font:.75rem/1 var(--mono);color:var(--ink4)}
.entry .meta .auth{color:var(--ink2);font-weight:500}
.entry .lane{padding:.1rem .4rem;border-radius:5px;background:var(--panel2);
  font-size:.67rem;letter-spacing:.04em;text-transform:uppercase}
.entry.summary .lane{background:var(--gold-glow);color:var(--gold)}
.entry .body{font-size:.965rem;line-height:1.68;color:var(--ink);
  white-space:pre-wrap;overflow-wrap:anywhere}
.entry.control .body{color:var(--ink3);font-family:var(--mono);font-size:.82rem}

.reply{border:1px solid var(--line2);border-radius:var(--r2);padding:1.05rem 1.2rem;
  background:linear-gradient(var(--panel),var(--bg2));animation:rise .3s both}
.reply .q{font:.78rem/1.5 var(--mono);color:var(--ink4);margin-bottom:.55rem;
  padding-bottom:.55rem;border-bottom:1px solid var(--line)}
.reply .a{font-size:.965rem;line-height:1.68}
.reply .src{margin-top:.7rem;font:.72rem/1 var(--mono);color:var(--ink4)}

/* ---- composer ---- */
.composer{border-top:1px solid var(--line);background:var(--bg2);padding:1rem 2rem 1.3rem}
.box{max-width:46rem;margin:0 auto}
.field{border:1px solid var(--line2);border-radius:var(--r2);background:var(--panel);
  transition:border-color .16s,box-shadow .16s}
.field:focus-within{border-color:var(--ink4);box-shadow:0 0 0 4px rgba(255,192,67,.06)}
.field textarea{width:100%;background:none;border:none;outline:none;resize:none;
  padding:.9rem 1.05rem .3rem;font-size:.95rem;line-height:1.6;min-height:2.9rem;
  max-height:14rem}
.field textarea::placeholder{color:var(--ink4)}
.tools{display:flex;align-items:center;gap:.5rem;padding:.5rem .6rem .6rem 1.05rem;
  flex-wrap:wrap}
.lanepick{display:flex;background:var(--bg);border:1px solid var(--line2);
  border-radius:8px;padding:2px;gap:2px}
.lanepick button{padding:.24rem .6rem;border-radius:6px;font-size:.78rem;
  font-weight:500;color:var(--ink4);transition:all .14s}
.lanepick button[aria-pressed=true]{background:var(--raise);color:var(--ink)}
.lanepick button[data-lane=summary][aria-pressed=true]{background:var(--gold-glow);
  color:var(--gold)}
.spacer{flex:1}
.note{font:.75rem/1.4 var(--mono);color:var(--ink4);padding:.5rem 1.05rem 0;
  max-width:46rem;margin:0 auto}
.note.warn{color:var(--gold)}

/* ---- empty ---- */
.void{max-width:30rem;margin:16vh auto;text-align:center;animation:rise .4s both}
.void .ico{width:46px;height:46px;margin:0 auto 1.1rem;border-radius:14px;
  border:1px solid var(--line2);background:var(--panel);
  display:grid;place-items:center;color:var(--ink4);font-size:1.2rem}
.void h2{font-size:1.06rem;font-weight:600;letter-spacing:-.015em;margin-bottom:.45rem}
.void p{color:var(--ink3);font-size:.92rem;line-height:1.65}
.void code{font-family:var(--mono);font-size:.85em;background:var(--panel2);
  padding:.14rem .38rem;border-radius:5px;color:var(--ink2)}

/* ---- share sheet ---- */
.veil{position:fixed;inset:0;background:rgba(4,5,7,.72);backdrop-filter:blur(6px);
  display:grid;place-items:center;padding:1.5rem;z-index:40;animation:fade .18s both}
@keyframes fade{from{opacity:0}to{opacity:1}}
.sheet{width:min(720px,100%);max-height:86vh;overflow:auto;background:var(--panel);
  border:1px solid var(--line2);border-radius:20px;
  box-shadow:0 40px 90px -30px rgba(0,0,0,.9);animation:pop .22s cubic-bezier(.2,.8,.3,1) both}
@keyframes pop{from{opacity:0;transform:translateY(12px) scale(.985)}to{opacity:1;transform:none}}
.sheet header{padding:1.35rem 1.6rem .9rem}
.sheet h2{font-size:1.1rem;font-weight:620;letter-spacing:-.018em;margin-bottom:.3rem}
.sheet header p{color:var(--ink3);font-size:.88rem;line-height:1.6}
.choices{display:grid;grid-template-columns:1fr 1fr;gap:.7rem;padding:0 1.6rem 1.1rem}
@media(max-width:560px){.choices{grid-template-columns:1fr}}
.choice{text-align:left;border:1px solid var(--line2);border-radius:var(--r2);
  padding:.85rem .95rem;transition:border-color .14s,background .14s}
.choice:hover{background:var(--panel2)}
.choice[aria-pressed=true]{border-color:var(--gold);background:var(--gold-glow)}
.choice b{display:block;font-size:.9rem;font-weight:600;margin-bottom:.16rem}
.choice span{font-size:.79rem;color:var(--ink3);line-height:1.5}
.preview{margin:0 1.6rem;border:1px solid var(--line);border-radius:var(--r2);
  background:var(--bg);overflow:hidden}
.preview .bar{padding:.55rem .9rem;border-bottom:1px solid var(--line);
  font:.7rem/1 var(--mono);color:var(--ink4);letter-spacing:.06em;text-transform:uppercase;
  display:flex;align-items:center;gap:.45rem}
.preview .bar .dot{width:6px;height:6px;border-radius:50%;background:var(--green)}
.preview .rows{padding:.85rem .95rem;display:flex;flex-direction:column;gap:.75rem;
  max-height:15rem;overflow:auto}
.prow{font-size:.87rem;line-height:1.6}
.prow .m{font:.7rem/1 var(--mono);color:var(--ink4);margin-bottom:.2rem}
.prow.hidden{color:var(--ink4);font-style:italic}
.sheet footer{display:flex;align-items:center;gap:.7rem;padding:1.1rem 1.6rem 1.4rem}
.sheet footer .spacer{flex:1}
.linkout{flex:1;min-width:0;font:.76rem/1 var(--mono);color:var(--ink3);
  background:var(--bg);border:1px solid var(--line2);border-radius:9px;
  padding:.5rem .65rem;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}

/* ---- shared view ---- */
.guest{max-width:46rem;margin:0 auto;padding:3rem 2rem 4rem}
.guest .crest{display:flex;align-items:center;gap:.6rem;margin-bottom:2rem;
  font-size:.86rem;color:var(--ink4)}
.guest h1{font-size:1.5rem;font-weight:660;letter-spacing:-.025em;margin-bottom:.4rem}
.guest .scope{display:inline-flex;align-items:center;gap:.4rem;margin-bottom:2.2rem;
  padding:.3rem .65rem;border-radius:99px;border:1px solid var(--line2);
  font:.75rem/1 var(--mono);color:var(--ink3)}
.guest .scope .dot{width:6px;height:6px;border-radius:50%;background:var(--gold)}
.guest .foot{margin-top:3rem;padding-top:1.4rem;border-top:1px solid var(--line);
  font-size:.82rem;color:var(--ink4);line-height:1.7}
.notice{max-width:30rem;margin:18vh auto;padding:0 1.5rem;text-align:center}
.notice h1{font-size:1.25rem;font-weight:620;letter-spacing:-.02em;margin-bottom:.5rem}
.notice p{color:var(--ink3);line-height:1.65}
`

const appJS = `
"use strict";
var cur=null, lane="content", sheet=null, sheetScope="summary";
var $=function(i){return document.getElementById(i)};
function esc(s){return String(s==null?"":s).replace(/[&<>"]/g,function(c){
  return {"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;"}[c]})}
function when(ts){var d=new Date(ts);if(isNaN(d))return ts;
  var now=new Date(), same=d.toDateString()===now.toDateString();
  return same ? d.toLocaleTimeString([],{hour:"numeric",minute:"2-digit"})
              : d.toLocaleDateString([],{month:"short",day:"numeric"})+" "+
                d.toLocaleTimeString([],{hour:"numeric",minute:"2-digit"})}
function api(p,o){return fetch(p,o).then(function(r){
  if(!r.ok)throw new Error("HTTP "+r.status);return r.json()})}

function threads(){return api("/app/api/threads").then(function(d){
  var el=$("threads");
  if(!d.threads.length){el.innerHTML='<div class="void" style="margin:4rem auto">'+
    '<div class="ico">+</div><h2>No threads yet</h2><p>Make one from an agent, or run '+
    '<code>lamdis thread new "title"</code>.</p></div>';return}
  el.innerHTML=d.threads.map(function(t){
    var sub=[t.entries+(t.entries===1?" entry":" entries")];
    if(t.last)sub.push(when(t.last));
    var pips="";
    if(t.shared)pips+='<span class="pip share">shared '+t.shared+'</span>';
    if(t.pending)pips+='<span class="pip wait">'+t.pending+' waiting</span>';
    return '<button class="thread" data-id="'+esc(t.id)+'" aria-current="'+(t.id===cur)+'">'+
      '<div class="name">'+esc(t.title)+'</div><div class="sub">'+esc(sub.join(" · "))+
      pips+'</div></button>'}).join("");
  Array.prototype.forEach.call(el.querySelectorAll(".thread"),function(n){
    n.onclick=function(){open(n.getAttribute("data-id"))}})})}

function render(entries){
  return entries.map(function(e){
    return '<article class="entry '+esc(e.lane)+'"><div class="spine"></div><div>'+
      '<div class="meta"><span class="auth">'+esc(e.who)+'</span><span>'+esc(when(e.ts))+
      '</span><span class="lane">'+esc(e.lane)+'</span></div>'+
      '<div class="body">'+esc(e.text)+'</div></div></article>'}).join("")}

function open(id){cur=id;$("share").disabled=false;threads();
  return api("/app/api/thread/"+encodeURIComponent(id)).then(function(d){
    window._entries=d.entries;
    $("title").textContent=d.title||"Untitled";
    var body=d.entries.length?render(d.entries):
      '<div class="void"><div class="ico">◇</div><h2>Nothing in here yet</h2>'+
      '<p>Write the first entry below. Anything you or your agents post lands here, '+
      'signed, in the order it happened.</p></div>';
    $("stream").innerHTML=body;
    $("feed").scrollTop=$("feed").scrollHeight})}

function setLane(l){lane=l;
  Array.prototype.forEach.call(document.querySelectorAll(".lanepick button"),function(b){
    b.setAttribute("aria-pressed",String(b.getAttribute("data-lane")===l))})}

function post(){var t=$("text").value.trim();if(!cur||!t)return;
  api("/app/api/post",{method:"POST",headers:{"content-type":"application/json"},
    body:JSON.stringify({thread:cur,text:t,lane:lane})}).then(function(){
      $("text").value="";grow();open(cur)})}

function ask(){var q=$("text").value.trim();if(!cur||!q)return;
  var n=$("note");n.textContent="Reading the thread…";n.className="note";
  api("/app/api/ask",{method:"POST",headers:{"content-type":"application/json"},
    body:JSON.stringify({thread:cur,question:q})}).then(function(d){
    if(d.error){n.textContent=d.error;n.className="note warn";return}
    n.textContent="";$("text").value="";grow();
    var box=document.createElement("div");box.className="reply";
    box.innerHTML='<div class="q">'+esc(q)+'</div><div class="a">'+esc(d.answer)+
      '</div><div class="src">read '+d.grounded_in+' entries · '+esc(d.model)+'</div>';
    $("stream").appendChild(box);$("feed").scrollTop=$("feed").scrollHeight})
   .catch(function(e){n.textContent=String(e.message);n.className="note warn"})}

function grow(){var t=$("text");t.style.height="auto";
  t.style.height=Math.min(t.scrollHeight,224)+"px"}

/* ---- share sheet: show them their view before you send it ---- */
function openSheet(){sheetScope="summary";sheet=document.createElement("div");
  sheet.className="veil";sheet.innerHTML=
   '<div class="sheet" role="dialog" aria-modal="true">'+
   '<header><h2>Show someone this thread</h2><p>They need no account and install '+
   'nothing. Pick how much of it they get — this is exactly what they will see.</p></header>'+
   '<div class="choices">'+
   '<button class="choice" data-scope="summary" aria-pressed="true"><b>Summary only</b>'+
   '<span>The lane you write for other people. Detail stays with you.</span></button>'+
   '<button class="choice" data-scope="read" aria-pressed="false"><b>Everything</b>'+
   '<span>The full thread, including what you wrote for yourself.</span></button></div>'+
   '<div class="preview"><div class="bar"><span class="dot"></span>Their view</div>'+
   '<div class="rows" id="prev"></div></div>'+
   '<footer><div class="linkout" id="linkout">Link appears when you create it</div>'+
   '<button class="btn" id="cancel">Cancel</button>'+
   '<button class="btn solid" id="mint">Create link</button></footer></div>';
  document.body.appendChild(sheet);
  sheet.onclick=function(e){if(e.target===sheet)closeSheet()};
  Array.prototype.forEach.call(sheet.querySelectorAll(".choice"),function(b){
    b.onclick=function(){sheetScope=b.getAttribute("data-scope");
      Array.prototype.forEach.call(sheet.querySelectorAll(".choice"),function(x){
        x.setAttribute("aria-pressed",String(x===b))});preview()}});
  sheet.querySelector("#cancel").onclick=closeSheet;
  sheet.querySelector("#mint").onclick=mint;
  preview()}
function closeSheet(){if(sheet){sheet.remove();sheet=null}}
function preview(){
  var rows=(window._entries||[]).filter(function(e){
    return e.lane==="summary"||(sheetScope==="read"&&e.lane==="content")});
  var kept=(window._entries||[]).filter(function(e){
    return sheetScope==="summary"&&e.lane==="content"}).length;
  var html=rows.map(function(e){return '<div class="prow"><div class="m">'+
    esc(e.who)+' · '+esc(when(e.ts))+'</div>'+esc(e.text)+'</div>'}).join("");
  if(!rows.length)html='<div class="prow hidden">Nothing in this lane yet — they '+
    'would see an empty thread.</div>';
  if(kept)html+='<div class="prow hidden">'+kept+' '+(kept===1?"entry stays":"entries stay")+
    ' with you and cannot be reached from this link.</div>';
  sheet.querySelector("#prev").innerHTML=html}
function mint(){
  api("/app/api/share",{method:"POST",headers:{"content-type":"application/json"},
    body:JSON.stringify({thread:cur,scope:sheetScope,days:30})}).then(function(d){
    var url=location.origin+d.path;
    if(navigator.clipboard)navigator.clipboard.writeText(url);
    var out=sheet.querySelector("#linkout");out.textContent=url+"  (copied)";
    sheet.querySelector("#mint").textContent="Copied — expires in "+d.days+" days"})}

$("post").onclick=post;$("ask").onclick=ask;$("share").onclick=openSheet;
$("text").oninput=grow;
$("text").onkeydown=function(e){
  if((e.metaKey||e.ctrlKey)&&e.key==="Enter"){e.preventDefault();post()}};
Array.prototype.forEach.call(document.querySelectorAll(".lanepick button"),function(b){
  b.onclick=function(){setLane(b.getAttribute("data-lane"))}});
document.onkeydown=function(e){if(e.key==="Escape")closeSheet()};
threads();
setInterval(function(){if(!sheet){cur?open(cur):threads()}},15000);
`

// appHTML is the owner's view.
func appHTML(model string, canAsk bool) string {
	note := "Asking is off. Set LAMDIS_OPENROUTER_KEY and restart to answer questions about a thread."
	noteClass := "note warn"
	if canAsk {
		note = ""
		noteClass = "note"
	}
	return `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="referrer" content="no-referrer">
<meta name="color-scheme" content="dark">
<title>Lamdis</title><style>` + appCSS + `</style></head><body>
<div class="shell">
  <aside class="rail">
    <div class="mark"><span class="glyph"></span> Lamdis <small>node</small></div>
    <div class="railhead">Threads</div>
    <div class="threads" id="threads"></div>
  </aside>
  <main class="main">
    <header class="head">
      <h1 id="title">Pick a thread</h1>
      <button class="btn key" id="share" disabled>Share</button>
    </header>
    <div class="feed" id="feed">
      <div class="stream" id="stream">
        <div class="void"><div class="ico">◈</div>
          <h2>Nothing selected</h2>
          <p>Threads are on the left. Everything in them is signed, in the order it
          happened, and yours until you decide otherwise.</p></div>
      </div>
    </div>
    <div class="composer">
      <div class="box">
        <div class="field">
          <textarea id="text" rows="1" placeholder="Write an entry, or ask a question about this thread…"></textarea>
          <div class="tools">
            <div class="lanepick">
              <button data-lane="content" aria-pressed="true">Keep to myself</button>
              <button data-lane="summary" aria-pressed="false">Shareable</button>
            </div>
            <span class="spacer"></span>
            <button class="btn" id="ask">Ask</button>
            <button class="btn solid" id="post">Post</button>
          </div>
        </div>
        <div class="` + noteClass + `" id="note">` + template.HTMLEscapeString(note) + `</div>
      </div>
    </div>
  </main>
</div>
<script>` + appJS + `</script></body></html>`
}

// sharedHTML is what a counterparty sees. No rail, no composer, no way to reach
// anything the link did not name.
func sharedHTML(full bool) string {
	scope := "Summary only"
	if full {
		scope = "Full thread"
	}
	return `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="referrer" content="no-referrer">
<meta name="color-scheme" content="dark">
<title>Shared thread — Lamdis</title><style>` + appCSS + `</style></head><body>
<div class="guest">
  <div class="crest"><span class="glyph" style="width:18px;height:18px;border-radius:6px;
    display:inline-block;background:linear-gradient(145deg,#FFC043,#C2820E)"></span>
    Shared with you through Lamdis</div>
  <h1 id="title">Loading…</h1>
  <div class="scope"><span class="dot"></span>` + template.HTMLEscapeString(scope) + `</div>
  <div id="stream"></div>
  <div class="foot">You are reading a copy of someone's working record, limited to what
  they chose to show. It is read only, nothing you do here is recorded, and the link
  expires. No account was needed and none was created.</div>
</div>
<script>
"use strict";
function esc(s){return String(s==null?"":s).replace(/[&<>"]/g,function(c){
  return {"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;"}[c]})}
function when(ts){var d=new Date(ts);return isNaN(d)?ts:
  d.toLocaleDateString([],{month:"short",day:"numeric"})+" "+
  d.toLocaleTimeString([],{hour:"numeric",minute:"2-digit"})}
fetch(location.pathname.replace(/\/$/,"")+"/api/thread").then(function(r){
  if(!r.ok)throw new Error("This link is no longer valid.");return r.json()
}).then(function(d){
  document.getElementById("title").textContent=d.title||"Shared thread";
  var rows=d.entries.filter(function(e){return e.lane!=="control"});
  document.getElementById("stream").innerHTML = rows.length ? rows.map(function(e){
    return '<article class="entry '+esc(e.lane)+'"><div class="spine"></div><div>'+
      '<div class="meta"><span class="auth">'+esc(e.who)+'</span><span>'+
      esc(when(e.ts))+'</span></div><div class="body">'+esc(e.text)+'</div></div></article>'
  }).join("") : '<div class="void"><div class="ico">◇</div><h2>Nothing shared yet</h2>'+
    '<p>Nothing has been posted into the part of this thread you can see.</p></div>';
}).catch(function(e){
  document.getElementById("title").textContent="Not available";
  document.getElementById("stream").innerHTML='<p style="color:var(--ink3)">'+
    esc(e.message)+'</p>'});
</script></body></html>`
}

func appNotice(title, body string) string {
	return `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="color-scheme" content="dark">
<title>` + template.HTMLEscapeString(title) + `</title><style>` + appCSS + `</style></head>
<body><div class="notice"><h1>` + template.HTMLEscapeString(title) + `</h1><p>` +
		template.HTMLEscapeString(body) + `</p></div></body></html>`
}

var appNoToken = appNotice("This node is not yours to open",
	"The interface is authenticated by a token kept in the node's data directory. "+
		"Run the node yourself and it prints the address to open.")
