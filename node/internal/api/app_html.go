package api

import "html/template"

// The interface.
//
// One file, no build step, no dependencies: a node that runs from a single
// binary should not need a toolchain to show you anything.
//
// The whole thing has to fit in one sentence, because that is how much of a
// manual anyone reads: write things down with your AI, then share a summary
// or everything with anyone by link. Everything you write is private. Sharing
// is one button and two choices. Nothing about keys, lanes, peers, nodes or
// grants appears on the main path; the protocol is underneath, not in front.

const appCSS = `
:root{
  --bg:#08090C; --bg2:#0C0E13; --panel:#111319; --panel2:#161922; --raise:#1B1F2A;
  --ink:#F2F5F9; --ink2:#A8B3C2; --ink3:#69748A; --ink4:#434C5E;
  --line:#191D26; --line2:#242A36;
  --gold:#FFC043; --gold-dim:#8A6413; --gold-glow:rgba(255,192,67,.13);
  --green:#4ADE80; --red:#F87171;
  --sans:'Inter var','Inter',-apple-system,BlinkMacSystemFont,'Segoe UI',system-ui,sans-serif;
  --mono:'SF Mono',ui-monospace,'JetBrains Mono',Menlo,monospace;
  color-scheme:dark;
}
*{box-sizing:border-box;margin:0;padding:0}
html,body{height:100%}
body{background:var(--bg);color:var(--ink);font-family:var(--sans);font-size:15px;line-height:1.6;-webkit-font-smoothing:antialiased}
button,input,textarea{font:inherit;color:inherit}
button{cursor:pointer;background:none;border:none}
input,textarea{background:var(--bg);border:1px solid var(--line2);border-radius:10px;padding:.6rem .75rem;outline:none;width:100%;transition:border-color .14s}
input:focus,textarea:focus{border-color:var(--ink4)}
input::placeholder,textarea::placeholder{color:var(--ink4)}
::selection{background:var(--gold-glow);color:#fff}
::-webkit-scrollbar{width:10px}::-webkit-scrollbar-thumb{background:var(--line2);border-radius:99px;border:3px solid transparent;background-clip:content-box}
.mono{font-family:var(--mono)}.dim{color:var(--ink4)}.muted{color:var(--ink3)}.small{font-size:.82rem}

.shell{display:grid;grid-template-columns:300px minmax(0,1fr);height:100vh}
@media(max-width:860px){.shell{grid-template-columns:1fr}.rail{display:none}}
.rail{background:var(--bg2);border-right:1px solid var(--line);display:flex;flex-direction:column;min-height:0}
.mark{display:flex;align-items:center;gap:.6rem;padding:1.3rem 1.4rem 1rem;font-size:1.02rem;font-weight:640;letter-spacing:-.012em}
.glyph{width:22px;height:22px;border-radius:7px;flex:none;background:linear-gradient(145deg,var(--gold),#C2820E);box-shadow:0 0 0 1px rgba(255,192,67,.25),0 4px 14px -4px rgba(255,192,67,.5)}
.newbtn{margin:0 .9rem .6rem;padding:.55rem .8rem;border-radius:10px;border:1px dashed var(--line2);color:var(--ink3);font-size:.86rem;text-align:left;transition:all .14s}
.newbtn:hover{border-color:var(--gold);color:var(--gold);border-style:solid}
.threads{flex:1;overflow-y:auto;padding:0 .7rem;display:flex;flex-direction:column;gap:2px}
.thread{width:100%;text-align:left;padding:.7rem .8rem;border-radius:12px;border:1px solid transparent;transition:background .14s,border-color .14s;position:relative}
.thread:hover{background:var(--panel)}
.thread[aria-current=true]{background:var(--panel2);border-color:var(--line2)}
.thread[aria-current=true]:before{content:"";position:absolute;left:0;top:14px;bottom:14px;width:2px;border-radius:2px;background:var(--gold)}
.thread .name{font-size:.925rem;font-weight:560;line-height:1.35;margin-bottom:.15rem;display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden}
.thread .sub{font:.73rem/1.4 var(--mono);color:var(--ink4);display:flex;gap:.45rem;align-items:center}
.pip{display:inline-flex;padding:.06rem .36rem;border-radius:5px;font-size:.66rem;font-weight:500;background:rgba(74,222,128,.1);color:var(--green)}
.me{border-top:1px solid var(--line);padding:.8rem 1.4rem;display:flex;align-items:center;gap:.7rem}
.me .avatar{width:30px;height:30px;border-radius:50%;background:var(--panel2);border:1px solid var(--line2);display:grid;place-items:center;font-size:.76rem;font-weight:600;color:var(--ink2);flex:none}
.me b{flex:1;min-width:0;font-size:.88rem;font-weight:560;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.icon{width:30px;height:30px;border-radius:8px;border:1px solid var(--line2);color:var(--ink3);display:grid;place-items:center;transition:all .14s}
.icon:hover{color:var(--ink);border-color:var(--ink4)}

.main{display:flex;flex-direction:column;min-width:0;min-height:0}
.head{display:flex;align-items:center;gap:.6rem;padding:1.05rem 2rem;border-bottom:1px solid var(--line);background:rgba(8,9,12,.82);backdrop-filter:saturate(1.6) blur(14px)}
.head h1{font-size:1.06rem;font-weight:620;letter-spacing:-.016em;flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.btn{border:1px solid var(--line2);border-radius:9px;padding:.44rem .85rem;font-size:.85rem;font-weight:500;color:var(--ink2);transition:all .14s;display:inline-flex;align-items:center;gap:.4rem;white-space:nowrap}
.btn:hover{color:var(--ink);border-color:var(--ink4);background:var(--panel)}
.btn.solid{background:var(--gold);border-color:var(--gold);color:#120C00;font-weight:600}
.btn.solid:hover{background:#FFCF6B;border-color:#FFCF6B}
.btn.danger:hover{color:var(--red);border-color:var(--red)}
.btn.sm{padding:.28rem .6rem;font-size:.78rem;border-radius:7px}
.btn[disabled]{opacity:.4;pointer-events:none}

.feed{flex:1;overflow-y:auto;padding:2rem 2rem 1rem;scroll-behavior:smooth}
.stream{max-width:44rem;margin:0 auto;display:flex;flex-direction:column;gap:1.4rem}
.entry{animation:rise .3s cubic-bezier(.2,.8,.3,1) both}
@keyframes rise{from{opacity:0;transform:translateY(6px)}to{opacity:1;transform:none}}
.entry .meta{display:flex;align-items:center;gap:.55rem;margin-bottom:.3rem;font:.75rem/1 var(--mono);color:var(--ink4)}
.entry .meta .auth{color:var(--ink2);font-weight:500}
.entry .tag{padding:.1rem .4rem;border-radius:5px;background:var(--gold-glow);color:var(--gold);font-size:.66rem;letter-spacing:.04em;text-transform:uppercase}
.entry .body{font-size:.965rem;line-height:1.68;white-space:pre-wrap;overflow-wrap:anywhere}
.entry.summary .body{padding:.75rem .95rem;border:1px solid var(--gold-dim);border-radius:12px;background:var(--gold-glow)}
.reply{border:1px solid var(--line2);border-radius:16px;padding:1rem 1.15rem;background:linear-gradient(var(--panel),var(--bg2));animation:rise .3s both}
.reply .q{font:.78rem/1.5 var(--mono);color:var(--ink4);margin-bottom:.5rem;padding-bottom:.5rem;border-bottom:1px solid var(--line)}
.reply .a{font-size:.965rem;line-height:1.68;white-space:pre-wrap}
.reply .foot{display:flex;align-items:center;gap:.6rem;margin-top:.75rem}
.reply .foot span{font:.72rem/1 var(--mono);color:var(--ink4);flex:1}

.composer{border-top:1px solid var(--line);background:var(--bg2);padding:1rem 2rem 1.1rem}
.box{max-width:44rem;margin:0 auto}
.field{border:1px solid var(--line2);border-radius:16px;background:var(--panel);transition:border-color .16s,box-shadow .16s}
.field:focus-within{border-color:var(--ink4);box-shadow:0 0 0 4px rgba(255,192,67,.06)}
.field textarea{background:none;border:none;resize:none;padding:.9rem 1.05rem .3rem;font-size:.95rem;line-height:1.6;min-height:2.9rem;max-height:14rem;border-radius:0}
.tools{display:flex;align-items:center;gap:.5rem;padding:.4rem .6rem .6rem 1.05rem}
.tools .hint{flex:1;font-size:.76rem;color:var(--ink4)}
.note{font:.75rem/1.4 var(--mono);color:var(--ink4);padding:.5rem 1.05rem 0}
.note.warn{color:var(--gold)}.note.bad{color:var(--red)}

.void{max-width:30rem;margin:16vh auto;text-align:center;animation:rise .4s both}
.void h2{font-size:1.25rem;font-weight:640;letter-spacing:-.02em;margin-bottom:.5rem}
.void p{color:var(--ink3);font-size:.95rem;line-height:1.65;margin-bottom:1.2rem}

.veil{position:fixed;inset:0;background:rgba(4,5,7,.72);backdrop-filter:blur(6px);display:grid;place-items:center;padding:1.5rem;z-index:40;animation:fade .18s both}
@keyframes fade{from{opacity:0}to{opacity:1}}
.sheet{width:min(640px,100%);max-height:88vh;overflow:auto;background:var(--panel);border:1px solid var(--line2);border-radius:20px;box-shadow:0 40px 90px -30px rgba(0,0,0,.9);animation:pop .22s cubic-bezier(.2,.8,.3,1) both}
@keyframes pop{from{opacity:0;transform:translateY(12px) scale(.985)}to{opacity:1;transform:none}}
.sheet header{padding:1.35rem 1.6rem .5rem}
.sheet h2{font-size:1.12rem;font-weight:620;letter-spacing:-.018em;margin-bottom:.3rem}
.sheet header p{color:var(--ink3);font-size:.88rem;line-height:1.6}
.sheet section{padding:.5rem 1.6rem 1.2rem}
.sheet footer{display:flex;align-items:center;gap:.7rem;padding:.9rem 1.6rem 1.3rem;border-top:1px solid var(--line)}
.spacer{flex:1}
label.f{display:block;font-size:.8rem;font-weight:500;color:var(--ink2);margin:.9rem 0 .35rem}
.hint{font-size:.79rem;color:var(--ink4);line-height:1.5;margin-top:.3rem}
.choices{display:grid;grid-template-columns:1fr 1fr;gap:.6rem}
@media(max-width:560px){.choices{grid-template-columns:1fr}}
.choice{text-align:left;border:1px solid var(--line2);border-radius:14px;padding:.85rem .95rem;transition:all .14s}
.choice:hover{background:var(--panel2)}
.choice[aria-pressed=true]{border-color:var(--gold);background:var(--gold-glow)}
.choice b{display:block;font-size:.9rem;font-weight:600;margin-bottom:.15rem}
.choice span{font-size:.79rem;color:var(--ink3);line-height:1.5}
.list{display:flex;flex-direction:column;gap:.45rem;margin-top:.5rem}
.row{display:flex;align-items:center;gap:.7rem;padding:.6rem .8rem;border:1px solid var(--line);border-radius:12px;background:var(--bg)}
.row .t{flex:1;min-width:0}
.row .t b{display:block;font-size:.88rem;font-weight:560;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.row .t span{font:.72rem/1.4 var(--mono);color:var(--ink4);display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.cmd{position:relative;background:var(--bg);border:1px solid var(--line2);border-radius:12px;padding:.75rem 3.4rem .75rem .9rem;font:.8rem/1.55 var(--mono);color:var(--ink2);white-space:pre-wrap;overflow-wrap:anywhere;margin-top:.4rem}
.cmd button{position:absolute;top:.5rem;right:.5rem;font-size:.7rem;padding:.2rem .5rem;border:1px solid var(--line2);border-radius:6px;color:var(--ink3)}
.cmd button:hover{color:var(--ink);border-color:var(--ink4)}
.linkbox{display:flex;gap:.5rem;align-items:center;margin-top:.9rem}
.linkbox input{font:.8rem/1 var(--mono);color:var(--ink2)}
details{margin-top:1.2rem;border-top:1px solid var(--line);padding-top:.9rem}
summary{cursor:pointer;font-size:.84rem;color:var(--ink3);list-style:none}
summary::-webkit-details-marker{display:none}
summary:before{content:"▸ ";color:var(--ink4)}details[open] summary:before{content:"▾ "}
.status{font:.75rem/1.5 var(--mono);padding:.5rem .8rem;border-radius:10px;margin-top:.7rem}
.status.ok{background:rgba(74,222,128,.08);color:var(--green)}.status.bad{background:rgba(248,113,113,.08);color:var(--red)}

.guest{max-width:44rem;margin:0 auto;padding:3rem 2rem 4rem}
.guest .crest{display:flex;align-items:center;gap:.6rem;margin-bottom:2rem;font-size:.86rem;color:var(--ink4)}
.guest h1{font-size:1.5rem;font-weight:660;letter-spacing:-.025em;margin-bottom:.4rem}
.guest .scope{display:inline-flex;align-items:center;gap:.4rem;margin-bottom:2rem;padding:.3rem .65rem;border-radius:99px;border:1px solid var(--line2);font:.75rem/1 var(--mono);color:var(--ink3)}
.guest .scope i{width:6px;height:6px;border-radius:50%;background:var(--gold)}
.guest .foot{margin-top:3rem;padding-top:1.4rem;border-top:1px solid var(--line);font-size:.82rem;color:var(--ink4);line-height:1.7}
.notice{max-width:30rem;margin:18vh auto;padding:0 1.5rem;text-align:center}
.notice h1{font-size:1.25rem;font-weight:620;margin-bottom:.5rem}.notice p{color:var(--ink3);line-height:1.65}
`

const appJS = `
"use strict";
var cur=null, me=null, entries=[], sheetEl=null;
var $=function(i){return document.getElementById(i)};
function esc(s){return String(s==null?"":s).replace(/[&<>"]/g,function(c){return {"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;"}[c]})}
function when(ts){var d=new Date(ts);if(isNaN(d))return ts||"";var n=new Date();
  return d.toDateString()===n.toDateString()?d.toLocaleTimeString([],{hour:"numeric",minute:"2-digit"})
  :d.toLocaleDateString([],{month:"short",day:"numeric"})+" "+d.toLocaleTimeString([],{hour:"numeric",minute:"2-digit"})}
function api(p,body){var o=body?{method:"POST",headers:{"content-type":"application/json"},body:JSON.stringify(body)}:undefined;
  return fetch(p,o).then(function(r){return r.text().then(function(t){var d;try{d=JSON.parse(t)}catch(e){d={error:t||("HTTP "+r.status)}}
    if(!r.ok&&!d.error)d.error="HTTP "+r.status;return d})})}
function copy(t){if(navigator.clipboard)navigator.clipboard.writeText(t)}
function initials(n){return (n&&n!=="you")?n.split(/\s+/).map(function(w){return w[0]}).join("").slice(0,2).toUpperCase():"·"}
function tid(){return encodeURIComponent(cur)}

function loadMe(){return api("/app/api/me").then(function(d){me=d;$("me-name").textContent=d.name;$("me-av").textContent=initials(d.name);
  $("ask").hidden=!d.can_ask;$("note").textContent=d.can_ask?"":"Answers are off until a model key is set. Open settings for how.";$("note").className=d.can_ask?"note":"note warn"})}

function threads(){return api("/app/api/threads").then(function(d){var el=$("threads");
  if(!d.threads.length){el.innerHTML="";empty();return}
  el.innerHTML=d.threads.map(function(t){var sub=[];if(t.last)sub.push(when(t.last));else sub.push("empty");
    return '<button class="thread" data-id="'+esc(t.id)+'" aria-current="'+(t.id===cur)+'"><div class="name">'+esc(t.title)+'</div><div class="sub">'+esc(sub.join(" · "))+
      (t.shared?'<span class="pip">shared</span>':'')+'</div></button>'}).join("");
  Array.prototype.forEach.call(el.querySelectorAll(".thread"),function(n){n.onclick=function(){open(n.getAttribute("data-id"))}});
  if(!cur)empty()})}

function empty(){$("title").textContent="Lamdis";$("share").disabled=true;
  $("stream").innerHTML='<div class="void"><h2>Write things down with your AI. Share what you choose.</h2>'+
  '<p>Start a thread for one topic. Everything you and your AI write in it stays private until you share a summary, or all of it, with someone by link.</p>'+
  '<button class="btn solid" id="e-new">Start a thread</button></div>';$("e-new").onclick=newThread}

function render(es){return es.map(function(e){var sum=e.lane==="summary";
  return '<article class="entry'+(sum?' summary':'')+'"><div class="meta"><span class="auth">'+esc(e.who)+'</span><span>'+esc(when(e.ts))+'</span>'+
    (sum?'<span class="tag">what you shared</span>':'')+'</div><div class="body">'+esc(e.text)+'</div></article>'}).join("")}

function open(id){cur=id;$("share").disabled=false;threads();
  return api("/app/api/thread/"+encodeURIComponent(id)).then(function(d){entries=d.entries;$("title").textContent=d.title||"Untitled";
    var vis=d.entries.filter(function(e){return e.lane!=="control"});
    $("stream").innerHTML=vis.length?render(vis):'<div class="void"><h2>'+esc(d.title)+'</h2><p>Nothing here yet. Write the first thing below, or ask your AI to.</p></div>';
    $("feed").scrollTop=$("feed").scrollHeight})}

function grow(){var t=$("text");t.style.height="auto";t.style.height=Math.min(t.scrollHeight,224)+"px"}
function post(){var t=$("text").value.trim();if(!cur||!t)return;
  api("/app/api/post",{thread:cur,text:t,lane:"content"}).then(function(d){if(d.error){$("note").textContent=d.error;$("note").className="note bad";return}
    $("text").value="";grow();$("note").textContent="";open(cur)})}
function ask(){var q=$("text").value.trim();if(!cur||!q)return;var n=$("note");n.textContent="Reading…";n.className="note";
  api("/app/api/ask",{thread:cur,question:q}).then(function(d){if(d.error){n.textContent=d.error;n.className="note warn";return}
    n.textContent="";$("text").value="";grow();var box=document.createElement("div");box.className="reply";
    box.innerHTML='<div class="q">'+esc(q)+'</div><div class="a">'+esc(d.answer)+'</div><div class="foot"><span>Answer · not saved</span>'+
      '<button class="btn sm" data-keep>Keep</button><button class="btn sm" data-x>Dismiss</button></div>';
    box.querySelector("[data-keep]").onclick=function(){api("/app/api/post",{thread:cur,text:d.answer,lane:"content"}).then(function(){box.remove();open(cur)})};
    box.querySelector("[data-x]").onclick=function(){box.remove()};$("stream").appendChild(box);$("feed").scrollTop=$("feed").scrollHeight})}

function sheet(html){closeSheet();sheetEl=document.createElement("div");sheetEl.className="veil";
  sheetEl.innerHTML='<div class="sheet" role="dialog" aria-modal="true">'+html+'</div>';document.body.appendChild(sheetEl);
  sheetEl.onclick=function(e){if(e.target===sheetEl)closeSheet()};return sheetEl.firstChild}
function closeSheet(){if(sheetEl){sheetEl.remove();sheetEl=null}}

function newThread(){var s=sheet('<header><h2>Start a thread</h2><p>One topic per thread. A deal, a project, a person, a decision.</p></header>'+
  '<section><input id="nt" placeholder="What is it about?" autofocus></section>'+
  '<footer><span class="spacer"></span><button class="btn" data-x>Cancel</button><button class="btn solid" data-go>Start</button></footer>');
  s.querySelector("[data-x]").onclick=closeSheet;
  var go=function(){var t=s.querySelector("#nt").value.trim();if(!t)return;api("/app/api/threads",{title:t}).then(function(d){if(d.error){alert(d.error);return}closeSheet();threads().then(function(){open(d.id)})})};
  s.querySelector("[data-go]").onclick=go;s.querySelector("#nt").onkeydown=function(e){if(e.key==="Enter")go()}}

/* Share: one button, two choices, one link. */
function shareSheet(){if(!cur)return;var mode="summary";
  api("/app/api/thread/"+tid()+"/access").then(function(d){
    var s=sheet('<header><h2>Share this thread</h2><p>Anyone with the link can read it. No account needed. You can stop sharing any time.</p></header>'+
    '<section><div class="choices"><button class="choice" data-m="summary" aria-pressed="true"><b>A summary</b><span>Your AI drafts it, you edit it. That is all they see.</span></button>'+
    '<button class="choice" data-m="read" aria-pressed="false"><b>Everything</b><span>The whole thread as it is.</span></button></div>'+
    '<div id="sum"><label class="f">What they will read</label><textarea id="draft" rows="5" placeholder="Drafting…"></textarea><p class="hint" id="drafthint"></p></div>'+
    '<label class="f">Who is it for? <span class="dim">(only you see this)</span></label><input id="label" placeholder="The lender, my accountant, Sam…">'+
    '<div class="linkbox"><input id="link" readonly placeholder="Your link will appear here"><button class="btn solid" id="go">Create link</button></div>'+
    (d.links.length?'<label class="f">Currently shared with</label><div class="list">'+d.links.map(function(l){return '<div class="row"><div class="t"><b>'+esc(l.label||"Someone")+'</b><span>'+
      (l.lanes.indexOf("content")>=0?"everything":"a summary")+' · until '+esc(when(l.expires))+'</span></div><button class="btn sm" data-copy="'+esc(location.origin+l.path)+'">Copy link</button>'+
      '<button class="btn sm danger" data-kill="'+esc(l.id)+'">Stop</button></div>'}).join("")+'</div>':'')+
    '</section><footer><span class="spacer"></span><button class="btn" data-x>Done</button></footer>');
    s.querySelector("[data-x]").onclick=closeSheet;
    var draft=s.querySelector("#draft"),hint=s.querySelector("#drafthint");
    function setMode(m){mode=m;Array.prototype.forEach.call(s.querySelectorAll(".choice"),function(b){b.setAttribute("aria-pressed",String(b.getAttribute("data-m")===m))});
      s.querySelector("#sum").hidden=(m!=="summary")}
    Array.prototype.forEach.call(s.querySelectorAll(".choice"),function(b){b.onclick=function(){setMode(b.getAttribute("data-m"))}});
    var existing=entries.filter(function(e){return e.lane==="summary"});
    if(existing.length){draft.value=existing[existing.length-1].text;hint.textContent="This is what you shared last time. Edit it if things have moved on."}
    else{api("/app/api/summarize",{thread:cur}).then(function(r){if(r.error){draft.placeholder="";hint.textContent=r.error;return}
      if(!r.draft){draft.placeholder="Write what you want them to know.";hint.textContent="Set a model key in settings and this gets drafted for you.";return}
      draft.value=r.draft;hint.textContent="Drafted from the thread. Edit anything before you share it."})}
    s.querySelector("#go").onclick=function(){var label=s.querySelector("#label").value;var go=s.querySelector("#go");go.disabled=true;
      var mint=function(){api("/app/api/share",{thread:cur,scope:mode,days:30,label:label}).then(function(r){go.disabled=false;
        if(r.error){alert(r.error);return}var url=location.origin+r.path;copy(url);s.querySelector("#link").value=url;go.textContent="Copied";open(cur)})};
      if(mode==="summary"){var t=draft.value.trim();if(!t){alert("Write or draft a summary first.");go.disabled=false;return}
        if(existing.length&&existing[existing.length-1].text===t){mint()}else{api("/app/api/post",{thread:cur,text:t,lane:"summary"}).then(mint)}}
      else mint()};
    Array.prototype.forEach.call(s.querySelectorAll("[data-copy]"),function(b){b.onclick=function(){copy(b.getAttribute("data-copy"));b.textContent="Copied"}});
    Array.prototype.forEach.call(s.querySelectorAll("[data-kill]"),function(b){b.onclick=function(){api("/app/api/share/revoke",{id:b.getAttribute("data-kill")}).then(function(){shareSheet()})}})})}

/* Settings: name, your AI, and everything else folded away. */
function settings(){var m=me||{};var origin=location.origin;
  var s=sheet('<header><h2>Settings</h2></header><section>'+
  '<label class="f">Your name</label><div style="display:flex;gap:.5rem"><input id="c-name" value="'+esc(m.name==="you"?"":m.name)+'" placeholder="How others will see you"><button class="btn" id="c-save">Save</button></div>'+
  '<label class="f">Use with Claude</label><div class="cmd">claude mcp add lamdis -- lamdis mcp<button data-copy="claude mcp add lamdis -- lamdis mcp">copy</button></div>'+
  '<p class="hint">Run that once. Claude can then read your threads and write into them. Any other AI that speaks MCP works the same way.</p>'+
  (m.can_ask?'<p class="hint">Answers come from '+esc(m.model)+'.</p>':'<p class="hint">To get answers and drafted summaries, put <span class="mono">LAMDIS_OPENROUTER_KEY=…</span> in the <span class="mono">.env</span> file next to your data and restart. Keys are at openrouter.ai/keys.</p>')+
  '<details><summary>Advanced: identity, other nodes, direct grants</summary>'+
  '<label class="f">Your identity</label><div class="cmd">'+esc(m.principal)+'<button data-copy="'+esc(m.principal)+'">copy</button></div>'+
  '<label class="f">This node</label><div class="cmd">'+esc(origin)+'<button data-copy="'+esc(origin)+'">copy</button></div>'+
  '<p class="hint">Someone running their own Lamdis can pair with you: <span class="mono">lamdis peer add &lt;name&gt; '+esc(origin)+'</span></p>'+
  '<label class="f">Pair with another node</label><div style="display:grid;grid-template-columns:1fr 2fr auto;gap:.5rem"><input id="p-name" placeholder="Name"><input id="p-url" placeholder="https://their-node"><button class="btn" id="p-add">Pair</button></div><div id="p-status"></div>'+
  '<div class="list" id="peers"></div>'+
  (cur?'<label class="f">Grant a paired person or agent access to this thread</label><div style="display:grid;grid-template-columns:2fr 1fr auto;gap:.5rem"><input id="g-to" placeholder="Paired name or identity"><input id="g-scope" value="summary" placeholder="summary | read | contribute"><button class="btn" id="g-go">Grant</button></div><div id="g-status"></div><div class="list" id="grants"></div>':'')+
  '</details></section><footer><span class="spacer"></span><button class="btn" data-x>Done</button></footer>');
  s.querySelector("[data-x]").onclick=closeSheet;
  Array.prototype.forEach.call(s.querySelectorAll("[data-copy]"),function(b){b.onclick=function(){copy(b.getAttribute("data-copy"));b.textContent="copied"}});
  s.querySelector("#c-save").onclick=function(){api("/app/api/me",{name:s.querySelector("#c-name").value}).then(loadMe)};
  function peers(){var l=(me&&me.peers)||[];s.querySelector("#peers").innerHTML=l.map(function(p){return '<div class="row"><div class="t"><b>'+esc(p.name)+'</b><span>'+esc(p.url)+'</span></div></div>'}).join("")}
  peers();
  s.querySelector("#p-add").onclick=function(){var st=s.querySelector("#p-status");st.innerHTML='<div class="status">Reaching their node…</div>';
    api("/app/api/peers",{name:s.querySelector("#p-name").value,url:s.querySelector("#p-url").value}).then(function(d){st.innerHTML='<div class="status '+(d.error||!d.reached?"bad":"ok")+'">'+esc(d.error||d.note)+'</div>';loadMe().then(peers)})};
  if(cur){var grants=function(){api("/app/api/thread/"+tid()+"/access").then(function(d){s.querySelector("#grants").innerHTML=d.grants.map(function(g){
      return '<div class="row"><div class="t"><b>'+esc(g.name)+'</b><span>'+esc(g.scopes.join(", "))+'</span></div><button class="btn sm danger" data-rv="'+esc(g.principal)+'">Revoke</button></div>'}).join("");
      Array.prototype.forEach.call(s.querySelectorAll("[data-rv]"),function(b){b.onclick=function(){api("/app/api/thread/"+tid()+"/revoke",{principal:b.getAttribute("data-rv")}).then(grants)}})})};
    grants();s.querySelector("#g-go").onclick=function(){var st=s.querySelector("#g-status");
      api("/app/api/thread/"+tid()+"/grant",{to:s.querySelector("#g-to").value,scopes:[s.querySelector("#g-scope").value.trim()||"summary"]}).then(function(r){
        st.innerHTML='<div class="status '+(r.error?"bad":"ok")+'">'+esc(r.error||("Granted to "+r.name))+'</div>';grants()})}}}

$("post").onclick=post;$("ask").onclick=ask;$("share").onclick=shareSheet;$("new").onclick=newThread;$("gear").onclick=settings;
$("text").oninput=grow;$("text").onkeydown=function(e){if((e.metaKey||e.ctrlKey)&&e.key==="Enter"){e.preventDefault();post()}};
document.onkeydown=function(e){if(e.key==="Escape")closeSheet()};
loadMe().then(threads);setInterval(function(){if(!sheetEl){cur?open(cur):threads()}},15000);
`

// appHTML is the owner's view.
func appHTML(model string, canAsk bool) string {
	return `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><meta name="referrer" content="no-referrer"><meta name="color-scheme" content="dark">
<title>Lamdis</title><style>` + appCSS + `</style></head><body>
<div class="shell">
  <aside class="rail">
    <div class="mark"><span class="glyph"></span> Lamdis</div>
    <button class="newbtn" id="new">+ New thread</button>
    <div class="threads" id="threads"></div>
    <div class="me"><div class="avatar" id="me-av">·</div><b id="me-name">you</b><button class="icon" id="gear" title="Settings">⚙</button></div>
  </aside>
  <main class="main">
    <header class="head"><h1 id="title">Lamdis</h1><button class="btn solid" id="share" disabled>Share</button></header>
    <div class="feed" id="feed"><div class="stream" id="stream"></div></div>
    <div class="composer"><div class="box">
      <div class="field">
        <textarea id="text" rows="1" placeholder="Write something down, or ask a question about this thread…"></textarea>
        <div class="tools"><span class="hint">Private until you share it.</span><button class="btn" id="ask">Ask</button><button class="btn solid" id="post">Save</button></div>
      </div>
      <div class="note" id="note"></div>
    </div></div>
  </main>
</div>
<script>` + appJS + `</script></body></html>`
}

// sharedHTML is what the other person sees. No rail, no composer, nothing
// beyond what the link named.
func sharedHTML(full bool) string {
	scope := "A summary"
	if full {
		scope = "The whole thread"
	}
	return `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><meta name="referrer" content="no-referrer"><meta name="color-scheme" content="dark">
<title>Shared — Lamdis</title><style>` + appCSS + `</style></head><body>
<div class="guest"><div class="crest"><span class="glyph" style="width:18px;height:18px;border-radius:6px"></span>Shared with you through Lamdis</div>
  <h1 id="title">Loading…</h1><div class="scope"><i></i>` + template.HTMLEscapeString(scope) + `</div>
  <div id="stream" style="display:flex;flex-direction:column;gap:1.4rem"></div>
  <div class="foot">Someone chose to show you this. It is read only, nothing you do here is recorded, and the link expires. No account was needed and none was created.</div></div>
<script>
"use strict";
function esc(s){return String(s==null?"":s).replace(/[&<>"]/g,function(c){return {"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;"}[c]})}
function when(ts){var d=new Date(ts);return isNaN(d)?ts:d.toLocaleDateString([],{month:"short",day:"numeric"})+" "+d.toLocaleTimeString([],{hour:"numeric",minute:"2-digit"})}
fetch(location.pathname.replace(/\/$/,"")+"/api/thread").then(function(r){if(!r.ok)throw new Error("This link is no longer valid.");return r.json()})
.then(function(d){document.getElementById("title").textContent=d.title||"Shared";var rows=d.entries.filter(function(e){return e.lane!=="control"});
  document.getElementById("stream").innerHTML=rows.length?rows.map(function(e){return '<article class="entry"><div class="meta"><span class="auth">'+esc(e.who)+'</span><span>'+esc(when(e.ts))+'</span></div><div class="body">'+esc(e.text)+'</div></article>'}).join("")
  :'<p class="muted">Nothing has been shared here yet.</p>'})
.catch(function(e){document.getElementById("title").textContent="Not available";document.getElementById("stream").innerHTML='<p class="muted">'+esc(e.message)+'</p>'});
</script></body></html>`
}

func appNotice(title, body string) string {
	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="dark">
<title>` + template.HTMLEscapeString(title) + `</title><style>` + appCSS + `</style></head>
<body><div class="notice"><h1>` + template.HTMLEscapeString(title) + `</h1><p>` + template.HTMLEscapeString(body) + `</p></div></body></html>`
}

var appNoToken = appNotice("This node is not yours to open",
	"The interface is authenticated by a token kept in the node's data directory. Run the node yourself and it opens the right address.")
