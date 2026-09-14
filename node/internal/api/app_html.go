package api

import "html/template"

// The interface.
//
// One file, no build step, no dependencies: a node that runs from a single
// binary should not need a toolchain to show you anything.
//
// It has to answer four questions without a manual, because nobody reads one:
// what is a thread, what is the difference between writing and asking, who
// can see this and how do I change that, and how do my agents get in. Each of
// those is a visible affordance with the real command or identifier filled in,
// never a sentence about where to find it.

const appCSS = `
:root{
  --bg:#08090C; --bg2:#0C0E13; --panel:#111319; --panel2:#161922; --raise:#1B1F2A;
  --ink:#F2F5F9; --ink2:#A8B3C2; --ink3:#69748A; --ink4:#434C5E;
  --line:#191D26; --line2:#242A36;
  --gold:#FFC043; --gold-dim:#8A6413; --gold-glow:rgba(255,192,67,.13);
  --green:#4ADE80; --red:#F87171; --blue:#7DD3FC;
  --sans:'Inter var','Inter',-apple-system,BlinkMacSystemFont,'Segoe UI',system-ui,sans-serif;
  --mono:'SF Mono',ui-monospace,'JetBrains Mono',Menlo,monospace;
  color-scheme:dark;
}
*{box-sizing:border-box;margin:0;padding:0}
html,body{height:100%}
body{background:var(--bg);color:var(--ink);font-family:var(--sans);font-size:15px;
  line-height:1.6;-webkit-font-smoothing:antialiased;text-rendering:optimizeLegibility}
button,input,textarea,select{font:inherit;color:inherit}
button{cursor:pointer;background:none;border:none}
input,textarea{background:var(--bg);border:1px solid var(--line2);border-radius:9px;
  padding:.55rem .7rem;outline:none;width:100%;transition:border-color .14s}
input:focus,textarea:focus{border-color:var(--ink4)}
input::placeholder,textarea::placeholder{color:var(--ink4)}
::selection{background:var(--gold-glow);color:#fff}
::-webkit-scrollbar{width:10px;height:10px}
::-webkit-scrollbar-thumb{background:var(--line2);border-radius:99px;border:3px solid transparent;background-clip:content-box}
code,.mono{font-family:var(--mono)}
.muted{color:var(--ink3)}.dim{color:var(--ink4)}.small{font-size:.8rem}

.shell{display:grid;grid-template-columns:320px minmax(0,1fr);height:100vh}
@media(max-width:860px){.shell{grid-template-columns:1fr}.rail{display:none}}

/* rail */
.rail{background:var(--bg2);border-right:1px solid var(--line);display:flex;flex-direction:column;min-height:0}
.mark{display:flex;align-items:center;gap:.6rem;padding:1.3rem 1.4rem 1rem;font-size:1.02rem;font-weight:640;letter-spacing:-.012em}
.glyph{width:22px;height:22px;border-radius:7px;flex:none;background:linear-gradient(145deg,var(--gold),#C2820E);
  box-shadow:0 0 0 1px rgba(255,192,67,.25),0 4px 14px -4px rgba(255,192,67,.5)}
.railrow{display:flex;align-items:center;justify-content:space-between;padding:.2rem 1.4rem .6rem}
.railhead{font:600 .68rem/1 var(--mono);letter-spacing:.14em;text-transform:uppercase;color:var(--ink4)}
.plus{width:26px;height:26px;border-radius:7px;border:1px solid var(--line2);color:var(--ink2);
  display:grid;place-items:center;font-size:1rem;line-height:1;transition:all .14s}
.plus:hover{border-color:var(--gold);color:var(--gold)}
.threads{flex:1;overflow-y:auto;padding:0 .75rem;display:flex;flex-direction:column;gap:2px}
.thread{width:100%;text-align:left;padding:.72rem .8rem;border-radius:12px;border:1px solid transparent;
  transition:background .14s,border-color .14s;display:block;position:relative}
.thread:hover{background:var(--panel)}
.thread[aria-current=true]{background:var(--panel2);border-color:var(--line2)}
.thread[aria-current=true]:before{content:"";position:absolute;left:0;top:14px;bottom:14px;width:2px;border-radius:2px;background:var(--gold)}
.thread .name{font-size:.925rem;font-weight:560;letter-spacing:-.008em;line-height:1.35;margin-bottom:.2rem;
  display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden}
.thread .sub{font:.73rem/1.4 var(--mono);color:var(--ink4);display:flex;align-items:center;gap:.4rem;flex-wrap:wrap}
.pip{display:inline-flex;align-items:center;padding:.08rem .38rem;border-radius:5px;font-size:.68rem;font-weight:500}
.pip.share{background:rgba(74,222,128,.1);color:var(--green)}
.pip.wait{background:var(--gold-glow);color:var(--gold)}
.me{border-top:1px solid var(--line);padding:.85rem 1.4rem;display:flex;align-items:center;gap:.7rem}
.me .avatar{width:30px;height:30px;border-radius:50%;background:var(--panel2);border:1px solid var(--line2);
  display:grid;place-items:center;font-size:.78rem;font-weight:600;color:var(--ink2);flex:none}
.me .who{min-width:0;flex:1}
.me .who b{display:block;font-size:.86rem;font-weight:580;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.me .who span{font:.68rem/1.3 var(--mono);color:var(--ink4);display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.icon{width:30px;height:30px;border-radius:8px;border:1px solid var(--line2);color:var(--ink3);display:grid;place-items:center;transition:all .14s;position:relative}
.icon:hover{color:var(--ink);border-color:var(--ink4)}
.icon .dot{position:absolute;top:-4px;right:-4px;width:9px;height:9px;border-radius:50%;background:var(--gold);border:2px solid var(--bg2)}

/* main */
.main{display:flex;flex-direction:column;min-width:0;min-height:0}
.head{display:flex;align-items:center;gap:.6rem;padding:1.05rem 2rem;border-bottom:1px solid var(--line);
  background:rgba(8,9,12,.82);backdrop-filter:saturate(1.6) blur(14px);position:sticky;top:0;z-index:5}
.head h1{font-size:1.06rem;font-weight:620;letter-spacing:-.016em;flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.btn{border:1px solid var(--line2);border-radius:9px;padding:.44rem .82rem;font-size:.85rem;font-weight:500;color:var(--ink2);
  transition:color .14s,border-color .14s,background .14s;display:inline-flex;align-items:center;gap:.4rem;white-space:nowrap}
.btn:hover{color:var(--ink);border-color:var(--ink4);background:var(--panel)}
.btn.key{border-color:var(--gold-dim);color:var(--gold)}
.btn.key:hover{background:var(--gold-glow);border-color:var(--gold)}
.btn.solid{background:var(--gold);border-color:var(--gold);color:#120C00;font-weight:600}
.btn.solid:hover{background:#FFCF6B;border-color:#FFCF6B}
.btn.danger:hover{color:var(--red);border-color:var(--red)}
.btn.sm{padding:.28rem .6rem;font-size:.78rem;border-radius:7px}
.btn[disabled]{opacity:.4;pointer-events:none}
.count{background:var(--gold);color:#120C00;font:600 .68rem/1 var(--mono);padding:.15rem .4rem;border-radius:99px}

.feed{flex:1;overflow-y:auto;padding:2rem 2rem 1rem;scroll-behavior:smooth}
.stream{max-width:46rem;margin:0 auto;display:flex;flex-direction:column;gap:1.5rem}
.entry{display:grid;grid-template-columns:auto minmax(0,1fr);gap:0 1rem;animation:rise .3s cubic-bezier(.2,.8,.3,1) both}
@keyframes rise{from{opacity:0;transform:translateY(6px)}to{opacity:1;transform:none}}
.entry .spine{width:2px;border-radius:2px;background:var(--line2);grid-row:1/3}
.entry.summary .spine{background:linear-gradient(var(--gold),var(--gold-dim));box-shadow:0 0 12px -2px var(--gold-glow)}
.entry.control{opacity:.5}.entry.control .spine{background:var(--line)}
.entry .meta{display:flex;align-items:center;gap:.6rem;margin-bottom:.35rem;font:.75rem/1 var(--mono);color:var(--ink4)}
.entry .meta .auth{color:var(--ink2);font-weight:500}
.entry .lane{padding:.1rem .4rem;border-radius:5px;background:var(--panel2);font-size:.67rem;letter-spacing:.04em;text-transform:uppercase}
.entry.summary .lane{background:var(--gold-glow);color:var(--gold)}
.entry .body{font-size:.965rem;line-height:1.68;white-space:pre-wrap;overflow-wrap:anywhere}
.entry.control .body{color:var(--ink3);font-family:var(--mono);font-size:.82rem}

.reply{border:1px solid var(--line2);border-radius:16px;padding:1.05rem 1.2rem;background:linear-gradient(var(--panel),var(--bg2));animation:rise .3s both}
.reply .q{font:.78rem/1.5 var(--mono);color:var(--ink4);margin-bottom:.55rem;padding-bottom:.55rem;border-bottom:1px solid var(--line)}
.reply .a{font-size:.965rem;line-height:1.68;white-space:pre-wrap}
.reply .foot{display:flex;align-items:center;gap:.6rem;margin-top:.8rem;flex-wrap:wrap}
.reply .foot .src{font:.72rem/1 var(--mono);color:var(--ink4);flex:1}

/* composer */
.composer{border-top:1px solid var(--line);background:var(--bg2);padding:1rem 2rem 1.2rem}
.box{max-width:46rem;margin:0 auto}
.field{border:1px solid var(--line2);border-radius:16px;background:var(--panel);transition:border-color .16s,box-shadow .16s}
.field:focus-within{border-color:var(--ink4);box-shadow:0 0 0 4px rgba(255,192,67,.06)}
.field textarea{background:none;border:none;resize:none;padding:.9rem 1.05rem .3rem;font-size:.95rem;line-height:1.6;min-height:2.9rem;max-height:14rem;border-radius:0}
.tools{display:flex;align-items:center;gap:.5rem;padding:.5rem .6rem .6rem 1.05rem;flex-wrap:wrap}
.lanepick{display:flex;background:var(--bg);border:1px solid var(--line2);border-radius:8px;padding:2px;gap:2px}
.lanepick button{padding:.24rem .6rem;border-radius:6px;font-size:.78rem;font-weight:500;color:var(--ink4);transition:all .14s}
.lanepick button[aria-pressed=true]{background:var(--raise);color:var(--ink)}
.lanepick button[data-lane=summary][aria-pressed=true]{background:var(--gold-glow);color:var(--gold)}
.spacer{flex:1}
.note{font:.75rem/1.4 var(--mono);color:var(--ink4);padding:.5rem 1.05rem 0}
.note.warn{color:var(--gold)}.note.bad{color:var(--red)}

/* empty + guide */
.void{max-width:32rem;margin:14vh auto;text-align:center;animation:rise .4s both}
.void .ico{width:46px;height:46px;margin:0 auto 1.1rem;border-radius:14px;border:1px solid var(--line2);background:var(--panel);display:grid;place-items:center;color:var(--ink4);font-size:1.2rem}
.void h2{font-size:1.06rem;font-weight:600;letter-spacing:-.015em;margin-bottom:.45rem}
.void p{color:var(--ink3);font-size:.92rem;line-height:1.65}
.guide{max-width:52rem;margin:8vh auto 0;animation:rise .4s both}
.guide h2{font-size:1.35rem;font-weight:640;letter-spacing:-.022em;margin-bottom:.4rem}
.guide>p{color:var(--ink3);max-width:36rem;margin-bottom:1.8rem}
.cards{display:grid;grid-template-columns:repeat(3,1fr);gap:.8rem}
@media(max-width:900px){.cards{grid-template-columns:1fr}}
.card{text-align:left;border:1px solid var(--line2);border-radius:16px;padding:1.05rem 1.1rem;background:var(--panel);transition:border-color .14s,transform .14s}
.card:hover{border-color:var(--ink4);transform:translateY(-1px)}
.card .n{font:600 .68rem/1 var(--mono);color:var(--gold);letter-spacing:.1em;margin-bottom:.55rem}
.card b{display:block;font-size:.95rem;font-weight:600;margin-bottom:.3rem}
.card span{font-size:.83rem;color:var(--ink3);line-height:1.55}

/* sheets */
.veil{position:fixed;inset:0;background:rgba(4,5,7,.72);backdrop-filter:blur(6px);display:grid;place-items:center;padding:1.5rem;z-index:40;animation:fade .18s both}
@keyframes fade{from{opacity:0}to{opacity:1}}
.sheet{width:min(760px,100%);max-height:88vh;overflow:auto;background:var(--panel);border:1px solid var(--line2);border-radius:20px;
  box-shadow:0 40px 90px -30px rgba(0,0,0,.9);animation:pop .22s cubic-bezier(.2,.8,.3,1) both}
@keyframes pop{from{opacity:0;transform:translateY(12px) scale(.985)}to{opacity:1;transform:none}}
.sheet header{padding:1.35rem 1.6rem .6rem}
.sheet h2{font-size:1.1rem;font-weight:620;letter-spacing:-.018em;margin-bottom:.3rem}
.sheet header p{color:var(--ink3);font-size:.88rem;line-height:1.6}
.sheet section{padding:.5rem 1.6rem 1.1rem}
.sheet footer{display:flex;align-items:center;gap:.7rem;padding:.9rem 1.6rem 1.4rem;border-top:1px solid var(--line)}
.tabs{display:flex;gap:.25rem;padding:0 1.6rem;border-bottom:1px solid var(--line)}
.tabs button{padding:.6rem .8rem;font-size:.86rem;font-weight:500;color:var(--ink3);border-bottom:2px solid transparent;margin-bottom:-1px;transition:all .14s}
.tabs button[aria-selected=true]{color:var(--ink);border-bottom-color:var(--gold)}
.tabs button .count{margin-left:.35rem}
label.f{display:block;font-size:.78rem;font-weight:500;color:var(--ink2);margin:.7rem 0 .3rem}
.hint{font-size:.78rem;color:var(--ink4);line-height:1.5;margin-top:.3rem}
.choices{display:grid;grid-template-columns:1fr 1fr;gap:.6rem;margin-top:.5rem}
@media(max-width:560px){.choices{grid-template-columns:1fr}}
.choice{text-align:left;border:1px solid var(--line2);border-radius:14px;padding:.75rem .9rem;transition:border-color .14s,background .14s}
.choice:hover{background:var(--panel2)}
.choice[aria-pressed=true]{border-color:var(--gold);background:var(--gold-glow)}
.choice b{display:block;font-size:.88rem;font-weight:600;margin-bottom:.12rem}
.choice span{font-size:.78rem;color:var(--ink3);line-height:1.5}
.list{display:flex;flex-direction:column;gap:.45rem;margin-top:.6rem}
.row{display:flex;align-items:center;gap:.7rem;padding:.6rem .8rem;border:1px solid var(--line);border-radius:12px;background:var(--bg)}
.row .t{flex:1;min-width:0}
.row .t b{display:block;font-size:.88rem;font-weight:560;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.row .t span{font:.72rem/1.4 var(--mono);color:var(--ink4);display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.preview{margin-top:.8rem;border:1px solid var(--line);border-radius:14px;background:var(--bg);overflow:hidden}
.preview .bar{padding:.5rem .9rem;border-bottom:1px solid var(--line);font:.68rem/1 var(--mono);color:var(--ink4);letter-spacing:.06em;text-transform:uppercase;display:flex;align-items:center;gap:.45rem}
.preview .bar .dot{width:6px;height:6px;border-radius:50%;background:var(--green)}
.preview .rows{padding:.85rem .95rem;display:flex;flex-direction:column;gap:.7rem;max-height:13rem;overflow:auto}
.prow{font-size:.87rem;line-height:1.6}.prow .m{font:.7rem/1 var(--mono);color:var(--ink4);margin-bottom:.2rem}
.prow.hidden{color:var(--ink4);font-style:italic}
.cmd{position:relative;background:var(--bg);border:1px solid var(--line2);border-radius:12px;padding:.75rem 3.2rem .75rem .9rem;
  font:.8rem/1.55 var(--mono);color:var(--ink2);white-space:pre-wrap;overflow-wrap:anywhere;margin-top:.4rem}
.cmd button{position:absolute;top:.5rem;right:.5rem;font-size:.7rem;padding:.2rem .5rem;border:1px solid var(--line2);border-radius:6px;color:var(--ink3)}
.cmd button:hover{color:var(--ink);border-color:var(--ink4)}
.toggle{display:flex;align-items:center;gap:.6rem;margin-top:.8rem;cursor:pointer;font-size:.88rem}
.toggle input{width:auto}
.status{font:.75rem/1.5 var(--mono);padding:.5rem .8rem;border-radius:10px;margin-top:.7rem}
.status.ok{background:rgba(74,222,128,.08);color:var(--green)}
.status.bad{background:rgba(248,113,113,.08);color:var(--red)}
.linkout{flex:1;min-width:0;font:.76rem/1 var(--mono);color:var(--ink3);background:var(--bg);border:1px solid var(--line2);border-radius:9px;padding:.5rem .65rem;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}

/* guest */
.guest{max-width:46rem;margin:0 auto;padding:3rem 2rem 4rem}
.guest .crest{display:flex;align-items:center;gap:.6rem;margin-bottom:2rem;font-size:.86rem;color:var(--ink4)}
.guest h1{font-size:1.5rem;font-weight:660;letter-spacing:-.025em;margin-bottom:.4rem}
.guest .scope{display:inline-flex;align-items:center;gap:.4rem;margin-bottom:2.2rem;padding:.3rem .65rem;border-radius:99px;border:1px solid var(--line2);font:.75rem/1 var(--mono);color:var(--ink3)}
.guest .scope .dot{width:6px;height:6px;border-radius:50%;background:var(--gold)}
.guest .foot{margin-top:3rem;padding-top:1.4rem;border-top:1px solid var(--line);font-size:.82rem;color:var(--ink4);line-height:1.7}
.notice{max-width:30rem;margin:18vh auto;padding:0 1.5rem;text-align:center}
.notice h1{font-size:1.25rem;font-weight:620;letter-spacing:-.02em;margin-bottom:.5rem}
.notice p{color:var(--ink3);line-height:1.65}
`

const appJS = `
"use strict";
var cur=null, lane="content", me=null, entries=[], sheetEl=null;
var $=function(i){return document.getElementById(i)};
function esc(s){return String(s==null?"":s).replace(/[&<>"]/g,function(c){return {"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;"}[c]})}
function when(ts){var d=new Date(ts);if(isNaN(d))return ts||"";var n=new Date();
  return d.toDateString()===n.toDateString()?d.toLocaleTimeString([],{hour:"numeric",minute:"2-digit"})
  :d.toLocaleDateString([],{month:"short",day:"numeric"})+" "+d.toLocaleTimeString([],{hour:"numeric",minute:"2-digit"})}
function api(p,body){var o=body?{method:"POST",headers:{"content-type":"application/json"},body:JSON.stringify(body)}:undefined;
  return fetch(p,o).then(function(r){return r.text().then(function(t){
    var d;try{d=JSON.parse(t)}catch(e){d={error:t||("HTTP "+r.status)}}
    if(!r.ok&&!d.error)d.error="HTTP "+r.status;return d})})}
function copy(t){if(navigator.clipboard)navigator.clipboard.writeText(t)}
function initials(n){return (n||"?").split(/\s+/).map(function(w){return w[0]}).join("").slice(0,2).toUpperCase()}

/* ---- identity ---- */
function loadMe(){return api("/app/api/me").then(function(d){me=d;
  $("me-name").textContent=d.name;$("me-id").textContent=d.principal;$("me-av").textContent=initials(d.name==="you"?"":d.name)||"·";
  $("inbox-dot").hidden=!d.pending;$("ask").hidden=!d.can_ask;$("note").textContent=d.can_ask?"":"Asking is off. Set LAMDIS_OPENROUTER_KEY in your node's .env to answer questions about a thread.";
  $("note").className=d.can_ask?"note":"note warn"})}

/* ---- threads ---- */
function threads(){return api("/app/api/threads").then(function(d){
  var el=$("threads");
  if(!d.threads.length){el.innerHTML='<p class="dim small" style="padding:.4rem .8rem">No threads yet.</p>';guide();return}
  el.innerHTML=d.threads.map(function(t){
    var sub=[t.entries+(t.entries===1?" entry":" entries")];if(t.last)sub.push(when(t.last));
    var pips="";if(t.shared)pips+='<span class="pip share">shared '+t.shared+'</span>';
    if(t.pending)pips+='<span class="pip wait">'+t.pending+' waiting</span>';
    return '<button class="thread" data-id="'+esc(t.id)+'" aria-current="'+(t.id===cur)+'"><div class="name">'+esc(t.title)+
      '</div><div class="sub">'+esc(sub.join(" · "))+pips+'</div></button>'}).join("");
  Array.prototype.forEach.call(el.querySelectorAll(".thread"),function(n){n.onclick=function(){open(n.getAttribute("data-id"))}});
  if(!cur)guide()})}

function guide(){$("title").textContent="Lamdis";$("people").disabled=true;
  $("stream").innerHTML='<div class="guide"><h2>A place your agents can share what they know.</h2>'+
  '<p>A thread is a signed record about one topic. You and your agents write into it. You decide, entry by entry, '+
  'what stays with you and what someone else may see.</p><div class="cards">'+
  '<button class="card" id="g-new"><div class="n">01</div><b>Start a thread</b><span>One topic per thread: a deal, a project, a migration, a person. Give it a title and go.</span></button>'+
  '<button class="card" id="g-connect"><div class="n">02</div><b>Connect an agent</b><span>One line gives Claude Code or any MCP client a way to read and write your threads.</span></button>'+
  '<button class="card" id="g-share"><div class="n">03</div><b>Show someone a thread</b><span>A person, an agent, or anyone with a link. Only the layer you choose, for as long as you choose.</span></button>'+
  '</div></div>';
  $("g-new").onclick=newThreadSheet;$("g-connect").onclick=connectSheet;
  $("g-share").onclick=function(){alert("Open a thread first, then use People in the top right.")}}

function render(es){return es.map(function(e){return '<article class="entry '+esc(e.lane)+'"><div class="spine"></div><div>'+
  '<div class="meta"><span class="auth">'+esc(e.who)+'</span><span>'+esc(when(e.ts))+'</span><span class="lane">'+
  (e.lane==="summary"?"shareable":e.lane==="content"?"kept":e.lane)+'</span></div><div class="body">'+esc(e.text)+'</div></div></article>'}).join("")}

function open(id){cur=id;$("people").disabled=false;threads();
  return api("/app/api/thread/"+encodeURIComponent(id)).then(function(d){entries=d.entries;
    $("title").textContent=d.title||"Untitled";
    var visible=d.entries.filter(function(e){return e.lane!=="control"});
    $("stream").innerHTML=visible.length?render(visible):
      '<div class="void"><div class="ico">◇</div><h2>Empty thread</h2><p>Write the first entry below, or have an agent post into it. '+
      'Use <b>Keep to myself</b> for detail and <b>Shareable</b> for what a counterparty may read.</p></div>';
    $("feed").scrollTop=$("feed").scrollHeight})}

/* ---- composer ---- */
function setLane(l){lane=l;Array.prototype.forEach.call(document.querySelectorAll(".lanepick button"),function(b){
  b.setAttribute("aria-pressed",String(b.getAttribute("data-lane")===l))})}
function grow(){var t=$("text");t.style.height="auto";t.style.height=Math.min(t.scrollHeight,224)+"px"}
function post(){var t=$("text").value.trim();if(!cur||!t)return;
  api("/app/api/post",{thread:cur,text:t,lane:lane}).then(function(d){
    if(d.error){$("note").textContent=d.error;$("note").className="note bad";return}
    $("text").value="";grow();$("note").textContent="";open(cur)})}
function ask(){var q=$("text").value.trim();if(!cur||!q)return;
  var n=$("note");n.textContent="Reading the thread…";n.className="note";
  api("/app/api/ask",{thread:cur,question:q}).then(function(d){
    if(d.error){n.textContent=d.error;n.className="note warn";return}
    n.textContent="";$("text").value="";grow();
    var box=document.createElement("div");box.className="reply";
    box.innerHTML='<div class="q">'+esc(q)+'</div><div class="a">'+esc(d.answer)+'</div>'+
      '<div class="foot"><span class="src">Answered from '+d.grounded_in+' entries · '+esc(d.model)+' · not saved</span>'+
      '<button class="btn sm key" data-save>Save as shareable summary</button><button class="btn sm" data-x>Dismiss</button></div>';
    box.querySelector("[data-save]").onclick=function(){
      api("/app/api/post",{thread:cur,text:d.answer,lane:"summary"}).then(function(){box.remove();open(cur)})};
    box.querySelector("[data-x]").onclick=function(){box.remove()};
    $("stream").appendChild(box);$("feed").scrollTop=$("feed").scrollHeight})}

/* ---- sheets ---- */
function sheet(html){closeSheet();sheetEl=document.createElement("div");sheetEl.className="veil";
  sheetEl.innerHTML='<div class="sheet" role="dialog" aria-modal="true">'+html+'</div>';
  document.body.appendChild(sheetEl);sheetEl.onclick=function(e){if(e.target===sheetEl)closeSheet()};
  return sheetEl.firstChild}
function closeSheet(){if(sheetEl){sheetEl.remove();sheetEl=null}}

function newThreadSheet(){var s=sheet('<header><h2>Start a thread</h2><p>One topic. Everything written into it is signed and kept in order.</p></header>'+
  '<section><label class="f">Title</label><input id="nt-title" placeholder="118 Maple closing" autofocus>'+
  '<label class="toggle"><input type="checkbox" id="nt-disc"> Discoverable</label>'+
  '<p class="hint">Discoverable threads advertise their title to people you have paired with, so their agents can ask for access. '+
  'Nothing inside is visible until you approve a request.</p></section>'+
  '<footer><span class="spacer"></span><button class="btn" data-x>Cancel</button><button class="btn solid" data-go>Create</button></footer>');
  s.querySelector("[data-x]").onclick=closeSheet;
  s.querySelector("[data-go]").onclick=function(){var t=s.querySelector("#nt-title").value.trim();if(!t)return;
    api("/app/api/threads",{title:t,discoverable:s.querySelector("#nt-disc").checked}).then(function(d){
      if(d.error){alert(d.error);return}closeSheet();threads().then(function(){open(d.id)})})};
  s.querySelector("#nt-title").onkeydown=function(e){if(e.key==="Enter")s.querySelector("[data-go]").click()}}

function connectSheet(){var m=me||{};var origin=location.origin;
  var s=sheet('<header><h2>Connect</h2><p>Who you are on the network, and how agents and other nodes reach this one.</p></header>'+
  '<section><label class="f">Your name</label><div style="display:flex;gap:.5rem"><input id="c-name" value="'+esc(m.name==="you"?"":m.name)+'" placeholder="How others will see you">'+
  '<button class="btn" id="c-save">Save</button></div>'+
  '<label class="f">Your identity</label><div class="cmd">'+esc(m.principal)+'<button data-copy="'+esc(m.principal)+'">copy</button></div>'+
  '<p class="hint">Give this to anyone who wants to grant you access to their threads.</p>'+
  '<label class="f">Give an agent access to this node</label><div class="cmd">claude mcp add lamdis -- lamdis mcp<button data-copy="claude mcp add lamdis -- lamdis mcp">copy</button></div>'+
  '<p class="hint">Any MCP client works the same way. The agent can then list, read, search and post into your threads with the permissions you hold.</p>'+
  '<label class="f">Let another node reach this one</label><div class="cmd">'+esc(origin)+'<button data-copy="'+esc(origin)+'">copy</button></div>'+
  '<p class="hint">Give them this URL. On their side: <code>lamdis peer add &lt;your-name&gt; '+esc(origin)+'</code>. Grants decide what they can pull.</p>'+
  '<label class="f">Pair with someone</label><div style="display:grid;grid-template-columns:1fr 2fr auto;gap:.5rem">'+
  '<input id="p-name" placeholder="Name"><input id="p-url" placeholder="https://their-node.example"><button class="btn" id="p-add">Pair</button></div>'+
  '<div id="p-status"></div><div class="list" id="peers"></div></section>'+
  '<footer><span class="spacer"></span><button class="btn" data-x>Done</button></footer>');
  s.querySelector("[data-x]").onclick=closeSheet;
  Array.prototype.forEach.call(s.querySelectorAll("[data-copy]"),function(b){b.onclick=function(){copy(b.getAttribute("data-copy"));b.textContent="copied"}});
  s.querySelector("#c-save").onclick=function(){api("/app/api/me",{name:s.querySelector("#c-name").value}).then(loadMe)};
  function peers(){var list=(me&&me.peers)||[];s.querySelector("#peers").innerHTML=list.length?list.map(function(p){
    return '<div class="row"><div class="t"><b>'+esc(p.name)+'</b><span>'+esc(p.url)+(p.principal?' · '+esc(p.principal):' · not reached yet')+'</span></div></div>'}).join("")
    :'<p class="hint">Nobody paired yet. Pairing fetches their identity so you never copy key strings.</p>'}
  peers();
  s.querySelector("#p-add").onclick=function(){var st=s.querySelector("#p-status");st.innerHTML='<div class="status">Reaching their node…</div>';
    api("/app/api/peers",{name:s.querySelector("#p-name").value,url:s.querySelector("#p-url").value}).then(function(d){
      st.innerHTML='<div class="status '+(d.error?"bad":d.reached?"ok":"bad")+'">'+esc(d.error||d.note)+'</div>';loadMe().then(peers)})}}

function peopleSheet(tab){if(!cur)return;tab=tab||"person";
  api("/app/api/thread/"+encodeURIComponent(cur)+"/access").then(function(d){
    var s=sheet('<header><h2>Who can see this</h2><p>'+esc(d.title)+(d.discoverable?' · discoverable':'')+'</p></header>'+
    '<div class="tabs"><button data-tab="person" aria-selected="false">A person or agent</button><button data-tab="link" aria-selected="false">A link</button>'+
    '<button data-tab="requests" aria-selected="false">Requests'+(d.requests.length?'<span class="count">'+d.requests.length+'</span>':'')+'</button></div>'+
    '<section id="body"></section><footer><span class="spacer"></span><button class="btn" data-x>Done</button></footer>');
    s.querySelector("[data-x]").onclick=closeSheet;
    Array.prototype.forEach.call(s.querySelectorAll(".tabs button"),function(b){b.onclick=function(){show(b.getAttribute("data-tab"))}});
    function show(t){Array.prototype.forEach.call(s.querySelectorAll(".tabs button"),function(b){b.setAttribute("aria-selected",String(b.getAttribute("data-tab")===t))});
      var b=s.querySelector("#body");
      if(t==="person")b.innerHTML=personTab(d);else if(t==="link")b.innerHTML=linkTab(d);else b.innerHTML=requestsTab(d);wire(t)}
    function scopeChoices(id,def){return '<div class="choices" data-scope="'+id+'">'+
      '<button class="choice" data-v="summary" aria-pressed="'+(def==="summary")+'"><b>Shareable layer only</b><span>What you marked shareable. Detail stays with you.</span></button>'+
      '<button class="choice" data-v="read" aria-pressed="'+(def==="read")+'"><b>Everything</b><span>The full thread, including what you kept.</span></button>'+
      '<button class="choice" data-v="contribute" aria-pressed="'+(def==="contribute")+'"><b>Everything, and they can write</b><span>Their entries appear here, signed by them.</span></button></div>'}
    function personTab(d){var peers=(me&&me.peers)||[];
      return '<p class="hint" style="margin-top:.6rem">A grant is signed by you and verifiable by their node. Revoke it any time.</p>'+
      '<label class="f">Who</label><input id="g-to" list="peerlist" placeholder="'+(peers.length?"A paired name, or paste an identity":"Paste their identity (ed25519:…), or pair with them under Connect")+'">'+
      '<datalist id="peerlist">'+peers.map(function(p){return '<option value="'+esc(p.name)+'">'}).join("")+'</datalist>'+
      '<label class="f">How much</label>'+scopeChoices("g","summary")+
      '<label class="f">For how long</label><input id="g-days" type="number" min="0" placeholder="Days, blank for no expiry" style="max-width:14rem">'+
      '<div style="margin-top:.8rem"><button class="btn solid" id="g-go">Grant access</button><span id="g-status" class="hint" style="margin-left:.6rem"></span></div>'+
      '<label class="f" style="margin-top:1.2rem">Currently</label><div class="list">'+(d.grants.length?d.grants.map(function(g){
        return '<div class="row"><div class="t"><b>'+esc(g.name)+'</b><span>'+esc(g.scopes.join(", "))+(g.expires?' · until '+esc(when(g.expires)):'')+'</span></div>'+
        '<button class="btn sm danger" data-revoke="'+esc(g.principal)+'">Revoke</button></div>'}).join(""):'<p class="hint">Nobody yet.</p>')+'</div>'}
    function linkTab(d){return '<p class="hint" style="margin-top:.6rem">For someone with no account. Read only, expires, and you can withdraw it here.</p>'+
      '<label class="f">Who is it for</label><input id="l-label" placeholder="The lender, the inspector, my accountant… (only you see this)">'+
      '<label class="f">How much</label><div class="choices" data-scope="l">'+
      '<button class="choice" data-v="summary" aria-pressed="true"><b>Shareable layer only</b><span>What you marked shareable.</span></button>'+
      '<button class="choice" data-v="read" aria-pressed="false"><b>Everything</b><span>The full thread.</span></button></div>'+
      '<div class="preview"><div class="bar"><span class="dot"></span>Their view</div><div class="rows" id="prev"></div></div>'+
      '<div style="display:flex;gap:.6rem;align-items:center;margin-top:.8rem"><div class="linkout" id="linkout">Link appears when you create it</div><button class="btn solid" id="l-go">Create link</button></div>'+
      '<label class="f" style="margin-top:1.2rem">Active links</label><div class="list">'+(d.links.length?d.links.map(function(l){
        return '<div class="row"><div class="t"><b>'+esc(l.label||"Unlabelled link")+'</b><span>'+esc(l.lanes.join(" + "))+' · until '+esc(when(l.expires))+'</span></div>'+
        '<button class="btn sm" data-copylink="'+esc(location.origin+l.path)+'">Copy</button><button class="btn sm danger" data-kill="'+esc(l.id)+'">Withdraw</button></div>'}).join(""):'<p class="hint">None.</p>')+'</div>'}
    function requestsTab(d){return '<p class="hint" style="margin-top:.6rem">People whose agents asked for access to this thread.</p><div class="list">'+
      (d.requests.length?d.requests.map(function(q){return '<div class="row"><div class="t"><b>'+esc(q.name)+'</b><span>asked for '+esc(q.scopes.join(", "))+
        (q.reason?' — '+esc(q.reason):'')+' · '+esc(when(q.at))+'</span></div>'+
        '<button class="btn sm key" data-approve="'+esc(q.principal)+'">Approve</button><button class="btn sm danger" data-deny="'+esc(q.principal)+'">Deny</button></div>'}).join("")
      :'<p class="hint">No requests. '+(d.discoverable?'This thread is discoverable, so requests can arrive.':'This thread is not discoverable; only people you grant directly can see it.')+'</p>')+'</div>'}
    function pick(id){var el=s.querySelector('[data-scope="'+id+'"] [aria-pressed="true"]');return el?el.getAttribute("data-v"):"summary"}
    function preview(){var sc=pick("l");var rows=entries.filter(function(e){return e.lane==="summary"||(sc==="read"&&e.lane==="content")});
      var kept=entries.filter(function(e){return sc==="summary"&&e.lane==="content"}).length;
      var h=rows.map(function(e){return '<div class="prow"><div class="m">'+esc(e.who)+' · '+esc(when(e.ts))+'</div>'+esc(e.text)+'</div>'}).join("");
      if(!rows.length)h='<div class="prow hidden">Nothing in this layer yet — they would see an empty thread.</div>';
      if(kept)h+='<div class="prow hidden">'+kept+' '+(kept===1?"entry stays":"entries stay")+' with you and cannot be reached from this link.</div>';
      var p=s.querySelector("#prev");if(p)p.innerHTML=h}
    function wire(t){Array.prototype.forEach.call(s.querySelectorAll(".choices"),function(g){
        Array.prototype.forEach.call(g.querySelectorAll(".choice"),function(b){b.onclick=function(){
          Array.prototype.forEach.call(g.querySelectorAll(".choice"),function(x){x.setAttribute("aria-pressed",String(x===b))});preview()}})});
      if(t==="link"){preview();s.querySelector("#l-go").onclick=function(){
        api("/app/api/share",{thread:cur,scope:pick("l"),days:30,label:s.querySelector("#l-label").value}).then(function(r){
          if(r.error){alert(r.error);return}var url=location.origin+r.path;copy(url);
          s.querySelector("#linkout").textContent=url+"  (copied)";s.querySelector("#l-go").textContent="Copied";refresh("link")})}}
      if(t==="person"){s.querySelector("#g-go").onclick=function(){var to=s.querySelector("#g-to").value.trim();if(!to)return;
        var days=parseInt(s.querySelector("#g-days").value,10)||0;var st=s.querySelector("#g-status");st.textContent="…";
        api("/app/api/thread/"+encodeURIComponent(cur)+"/grant",{to:to,scopes:[pick("g")],days:days}).then(function(r){
          if(r.error){st.textContent=r.error;return}st.textContent="Granted to "+r.name;refresh("person")})}}
      Array.prototype.forEach.call(s.querySelectorAll("[data-revoke]"),function(b){b.onclick=function(){
        api("/app/api/thread/"+encodeURIComponent(cur)+"/revoke",{principal:b.getAttribute("data-revoke")}).then(function(){refresh("person")})}});
      Array.prototype.forEach.call(s.querySelectorAll("[data-kill]"),function(b){b.onclick=function(){
        api("/app/api/share/revoke",{id:b.getAttribute("data-kill")}).then(function(){refresh("link")})}});
      Array.prototype.forEach.call(s.querySelectorAll("[data-copylink]"),function(b){b.onclick=function(){copy(b.getAttribute("data-copylink"));b.textContent="Copied"}});
      Array.prototype.forEach.call(s.querySelectorAll("[data-approve]"),function(b){b.onclick=function(){
        api("/app/api/thread/"+encodeURIComponent(cur)+"/decide",{principal:b.getAttribute("data-approve"),approve:true}).then(function(){refresh("requests")})}});
      Array.prototype.forEach.call(s.querySelectorAll("[data-deny]"),function(b){b.onclick=function(){
        api("/app/api/thread/"+encodeURIComponent(cur)+"/decide",{principal:b.getAttribute("data-deny"),approve:false}).then(function(){refresh("requests")})}})}
    function refresh(t){api("/app/api/thread/"+encodeURIComponent(cur)+"/access").then(function(nd){d=nd;show(t);loadMe();threads()})}
    show(tab)})}

/* ---- wiring ---- */
$("post").onclick=post;$("ask").onclick=ask;$("people").onclick=function(){peopleSheet("person")};
$("new").onclick=newThreadSheet;$("connect").onclick=connectSheet;$("inbox").onclick=function(){peopleSheet("requests")};
$("text").oninput=grow;$("text").onkeydown=function(e){if((e.metaKey||e.ctrlKey)&&e.key==="Enter"){e.preventDefault();post()}};
Array.prototype.forEach.call(document.querySelectorAll(".lanepick button"),function(b){b.onclick=function(){setLane(b.getAttribute("data-lane"))}});
document.onkeydown=function(e){if(e.key==="Escape")closeSheet()};
loadMe().then(threads);
setInterval(function(){if(!sheetEl){cur?open(cur):threads();loadMe()}},15000);
`

// appHTML is the owner's view.
func appHTML(model string, canAsk bool) string {
	return `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="referrer" content="no-referrer"><meta name="color-scheme" content="dark">
<title>Lamdis</title><style>` + appCSS + `</style></head><body>
<div class="shell">
  <aside class="rail">
    <div class="mark"><span class="glyph"></span> Lamdis</div>
    <div class="railrow"><span class="railhead">Threads</span><button class="plus" id="new" title="Start a thread">+</button></div>
    <div class="threads" id="threads"></div>
    <div class="me">
      <div class="avatar" id="me-av">·</div>
      <div class="who"><b id="me-name">you</b><span id="me-id"></span></div>
      <button class="icon" id="inbox" title="Access requests"><span id="inbox-dot" class="dot" hidden></span>✉</button>
      <button class="icon" id="connect" title="Connect agents and people">⚙</button>
    </div>
  </aside>
  <main class="main">
    <header class="head">
      <h1 id="title">Lamdis</h1>
      <button class="btn key" id="people" disabled>People</button>
    </header>
    <div class="feed" id="feed"><div class="stream" id="stream"></div></div>
    <div class="composer"><div class="box">
      <div class="field">
        <textarea id="text" rows="1" placeholder="Write an entry into this thread, or ask it a question…"></textarea>
        <div class="tools">
          <div class="lanepick">
            <button data-lane="content" aria-pressed="true" title="Detail. Only you and people you grant everything to.">Keep to myself</button>
            <button data-lane="summary" aria-pressed="false" title="The layer a counterparty may read.">Shareable</button>
          </div>
          <span class="spacer"></span>
          <button class="btn" id="ask" title="Answer from this thread only. Nothing is saved unless you keep it.">Ask</button>
          <button class="btn solid" id="post" title="Sign it into the record. ⌘⏎">Post</button>
        </div>
      </div>
      <div class="note" id="note"></div>
    </div></div>
  </main>
</div>
<script>` + appJS + `</script></body></html>`
}

// sharedHTML is what a counterparty sees. No rail, no composer, no way to reach
// anything the link did not name.
func sharedHTML(full bool) string {
	scope := "Shareable layer only"
	if full {
		scope = "Full thread"
	}
	return `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="referrer" content="no-referrer"><meta name="color-scheme" content="dark">
<title>Shared thread — Lamdis</title><style>` + appCSS + `</style></head><body>
<div class="guest">
  <div class="crest"><span class="glyph" style="width:18px;height:18px;border-radius:6px"></span>Shared with you through Lamdis</div>
  <h1 id="title">Loading…</h1>
  <div class="scope"><span class="dot"></span>` + template.HTMLEscapeString(scope) + `</div>
  <div id="stream" style="display:flex;flex-direction:column;gap:1.4rem"></div>
  <div class="foot">You are reading part of someone's working record, limited to what they chose to show.
  It is read only, nothing you do here is recorded, and the link expires. No account was needed and none was created.</div>
</div>
<script>
"use strict";
function esc(s){return String(s==null?"":s).replace(/[&<>"]/g,function(c){return {"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;"}[c]})}
function when(ts){var d=new Date(ts);return isNaN(d)?ts:d.toLocaleDateString([],{month:"short",day:"numeric"})+" "+d.toLocaleTimeString([],{hour:"numeric",minute:"2-digit"})}
fetch(location.pathname.replace(/\/$/,"")+"/api/thread").then(function(r){if(!r.ok)throw new Error("This link is no longer valid.");return r.json()})
.then(function(d){document.getElementById("title").textContent=d.title||"Shared thread";
  var rows=d.entries.filter(function(e){return e.lane!=="control"});
  document.getElementById("stream").innerHTML=rows.length?rows.map(function(e){
    return '<article class="entry '+esc(e.lane)+'"><div class="spine"></div><div><div class="meta"><span class="auth">'+esc(e.who)+'</span><span>'+
      esc(when(e.ts))+'</span></div><div class="body">'+esc(e.text)+'</div></div></article>'}).join("")
    :'<div class="void" style="margin:2rem auto"><div class="ico">◇</div><h2>Nothing shared yet</h2><p>Nothing has been posted into the part of this thread you can see.</p></div>'})
.catch(function(e){document.getElementById("title").textContent="Not available";
  document.getElementById("stream").innerHTML='<p class="muted">'+esc(e.message)+'</p>'});
</script></body></html>`
}

func appNotice(title, body string) string {
	return `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="dark">
<title>` + template.HTMLEscapeString(title) + `</title><style>` + appCSS + `</style></head>
<body><div class="notice"><h1>` + template.HTMLEscapeString(title) + `</h1><p>` +
		template.HTMLEscapeString(body) + `</p></div></body></html>`
}

var appNoToken = appNotice("This node is not yours to open",
	"The interface is authenticated by a token kept in the node's data directory. "+
		"Run the node yourself and it prints the address to open.")
