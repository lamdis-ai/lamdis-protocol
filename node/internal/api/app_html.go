package api

import (
	"html/template"
	"strings"
	"time"
)

// The interface.
//
// One file, no build step, no dependencies: a node that runs from a single
// binary should not need a toolchain to show you anything.
//
// The whole thing has to fit in one sentence, because that is how much of a
// manual anyone reads: a thread is a workspace shared between you, your
// agent, and whoever you choose. Everything anyone writes there, the agent
// included, is the record; you decide who sees which part; the agent keeps
// working there when you are not. Nothing about keys, lanes, peers, nodes or
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

.shell{display:grid;grid-template-columns:300px minmax(0,1fr);height:100vh;height:100dvh}
.menu{display:none}
.scrim{display:none}
/* Touch targets and text sizes that survive a phone. 16px on inputs is not
   a style choice: anything smaller makes iOS zoom the page on focus. */
@media(max-width:860px){
  .shell{grid-template-columns:1fr}
  body{font-size:16px}
  input,textarea,select{font-size:16px}
  .btn{padding:.55rem .9rem}
  .btn.sm{padding:.4rem .7rem;font-size:.82rem}
  .icon{width:36px;height:36px}
  /* The rail slides over rather than vanishing: hiding it took the thread
     list, settings and sign-in with it. */
  .rail{position:fixed;inset:0 auto 0 0;width:min(84vw,320px);z-index:40;
        transform:translateX(-100%);transition:transform .22s cubic-bezier(.2,.8,.3,1);
        box-shadow:0 0 60px rgba(0,0,0,.7)}
  .shell.open .rail{transform:none}
  .scrim{display:block;position:fixed;inset:0;z-index:35;background:rgba(4,5,7,.6);
         opacity:0;pointer-events:none;transition:opacity .22s}
  .shell.open .scrim{opacity:1;pointer-events:auto}
  .menu{display:inline-grid;place-items:center;width:34px;height:34px;flex:none;
        border:1px solid var(--line2);border-radius:9px;color:var(--ink2)}
  .menu:hover{color:var(--ink)}
  /* The header has four things and a phone has room for two, so the
     secondary ones shrink to their icons rather than wrapping. */
  .head{padding:.6rem .7rem;gap:.4rem;position:sticky;top:0;z-index:20}
  .head h1{font-size:1rem;min-width:0}
  .head .pill{padding:.3rem .5rem;gap:0}
  .head .pill .full{display:none}
  .head .pill .short{display:inline}
  .head .pill.wait{gap:.3rem;padding:.3rem .55rem}
  .head .pill.wait .short{font-size:.76rem}
  .head .btn.solid{padding:.45rem .8rem;font-size:.85rem}
  .feed{padding:1rem .9rem .4rem;-webkit-overflow-scrolling:touch;overscroll-behavior:contain}
  .stream{gap:1.1rem}
  .composer{padding:.6rem .9rem calc(.7rem + env(safe-area-inset-bottom))}
  .field textarea{padding:.75rem .9rem .25rem;max-height:9rem}
  .tools .hint{display:none}
  .tools{padding:.3rem .45rem .45rem .9rem;gap:.4rem}
  .entry .body{font-size:.95rem;line-height:1.6}
  .entry .meta{flex-wrap:wrap;row-gap:.15rem}
  .rail .mark{padding:1rem 1.1rem .8rem}
  .newbtn{margin:0 .8rem .5rem}
  .threads{padding:0 .6rem}
  .thread{padding:.75rem .8rem}
  .me{padding:.7rem 1.1rem calc(.7rem + env(safe-area-inset-bottom))}
  /* Two columns of anything is one column on a phone. */
  .grid2,.setup,.qa{grid-template-columns:1fr}
  .rh{grid-template-columns:5.5rem 1fr auto}
  .linkbox{flex-direction:column;align-items:stretch}
  .linkbox .btn{justify-content:center}
  .cmd{padding-right:1rem;font-size:.78rem}
  .cmd button{position:static;display:block;margin-top:.5rem}
  .run{flex-wrap:wrap}
  .decision .free{flex-direction:column}
  .decision .free .btn{justify-content:center}
  .sheet{width:100%;max-height:92dvh;border-radius:18px 18px 0 0}
  .veil{padding:0;place-items:end center}
  .sheet header{padding:1.1rem 1.15rem .4rem}
  .sheet section{padding:.4rem 1.15rem 1rem}
  .sheet footer{padding:.8rem 1.15rem calc(1rem + env(safe-area-inset-bottom))}
  .choices{grid-template-columns:1fr}
  .void{margin:8vh auto;padding:0 .4rem}
  .void h2{font-size:1.12rem}
  .void p{font-size:.92rem}
}
@media(max-width:400px){
  .head .btn.solid{padding:.42rem .65rem}
  .rail{width:88vw}
  .rh{grid-template-columns:1fr;gap:.35rem}
}
/* A short screen is a phone in landscape: give the reading area the room. */
@media(max-height:520px) and (max-width:900px){
  .feed{padding-top:.5rem}
  .void{margin:3vh auto}
  .sheet{max-height:96dvh}
}
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
.pip.warn{background:var(--gold-glow);color:var(--gold)}
.nudge{display:flex;align-items:center;gap:.8rem;padding:.7rem .95rem;border:1px solid var(--gold-dim);border-radius:12px;background:var(--gold-glow);font-size:.86rem;color:var(--ink2);animation:rise .3s both}
.nudge span{flex:1}
.ref{color:var(--gold);cursor:pointer;border-bottom:1px dotted var(--gold-dim)}
.ref.dead{color:var(--ink3);cursor:default;border-bottom-style:dashed}
.tag.ai{background:rgba(125,211,252,.12);color:#7DD3FC}
.entry.agent .auth{color:#7DD3FC}
.conn{border:1px solid var(--line);border-radius:12px;padding:.8rem .9rem;margin-top:.5rem;background:var(--bg)}
.conn .top{display:flex;align-items:center;gap:.6rem}
.conn .top b{flex:1;font-size:.9rem;font-weight:560}
.conn .addr{font:.72rem var(--mono);color:var(--ink4);margin-top:.15rem;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.conn .picks{display:flex;flex-wrap:wrap;gap:.35rem;margin-top:.6rem}
.pick{border:1px solid var(--line2);border-radius:99px;padding:.22rem .6rem;font-size:.76rem;color:var(--ink3);cursor:pointer;background:none}
.pick[aria-pressed=true]{border-color:var(--gold);color:var(--gold);background:var(--gold-glow)}
.pick.ask[aria-pressed=true]{border-color:var(--blue);color:var(--blue);background:rgba(125,211,252,.1)}
.conn .probe{font-size:.78rem;color:var(--ink4);margin-top:.5rem;min-height:1.1rem}
.conn .probe.bad{color:var(--red)}
.conn .probe.ok{color:var(--green)}
.entry.q .body{color:var(--ink2);font-style:normal;padding-left:.9rem;border-left:2px solid var(--line2)}
.decision{border:1px solid rgba(125,211,252,.35);border-radius:14px;padding:.9rem 1.05rem;background:rgba(125,211,252,.05)}
.decision .ask{font-size:.95rem;line-height:1.6;margin-bottom:.7rem}
.decision .opts{display:flex;flex-wrap:wrap;gap:.45rem;margin-bottom:.6rem}
.decision .free{display:flex;gap:.5rem}
.decision .done{font:.78rem/1.5 var(--mono);color:var(--ink3)}
.run{font:.74rem/1.5 var(--mono);color:var(--ink4);display:flex;gap:.5rem;align-items:baseline}
.run summary{display:inline;cursor:pointer;color:var(--ink4)}
.run summary:before{content:""}
.run details{margin:0;border:none;padding:0;display:inline}
.run .more{margin:.3rem 0 0 1rem;color:var(--ink3);white-space:pre-wrap}
.run.bad{color:var(--red)}
.pill{display:inline-flex;align-items:center;gap:.35rem;border:1px solid var(--line2);border-radius:99px;padding:.3rem .7rem;font-size:.78rem;color:var(--ink2);transition:all .14s}
.pill:hover{border-color:var(--ink4);color:var(--ink)}
.pill i{width:7px;height:7px;border-radius:50%;background:var(--ink4)}
.pill .short{display:none;font-size:.76rem}
.pill.on i{background:#7DD3FC;box-shadow:0 0 8px #7DD3FC}
.pill.off{border-style:dashed;color:var(--ink3)}
.pill.off:hover{border-style:solid;color:var(--ink);border-color:var(--gold)}
.pill.wait i{background:var(--gold);box-shadow:0 0 8px var(--gold)}
.pip.wait{background:var(--gold-glow);color:var(--gold)}
.pip.auto{background:rgba(125,211,252,.12);color:#7DD3FC}
select{background:var(--bg);border:1px solid var(--line2);border-radius:10px;padding:.55rem .7rem;color:var(--ink);width:100%}
.grid2{display:grid;grid-template-columns:1fr 1fr;gap:.6rem}
@media(max-width:560px){.grid2{grid-template-columns:1fr}}
.check{display:flex;align-items:center;gap:.5rem;font-size:.86rem;color:var(--ink2);padding:.3rem 0}
.check input{width:auto}
.rh{display:grid;grid-template-columns:7.5rem 1fr auto;gap:.5rem;align-items:center;margin-top:.45rem}
.rh input[type=time]{font-family:var(--mono);font-size:.84rem}
.rh .x{color:var(--ink4);font-size:1rem;padding:0 .4rem}
.rh .x:hover{color:var(--red)}
.presets{display:flex;gap:.45rem;flex-wrap:wrap;margin-top:.55rem}
.thinking{font:.78rem/1.5 var(--mono);color:#7DD3FC;animation:pulse 1.2s ease-in-out infinite;display:flex;gap:.5rem;align-items:center}
.thinking:before{content:"";width:9px;height:9px;border-radius:50%;border:2px solid currentColor;border-right-color:transparent;animation:spin .7s linear infinite;flex:none}
@keyframes spin{to{transform:rotate(360deg)}}
@keyframes pulse{0%,100%{opacity:.45}50%{opacity:1}}
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

.veil{position:fixed;inset:0;background:rgba(4,5,7,.72);backdrop-filter:blur(6px);display:grid;place-items:center;padding:1.5rem;z-index:50;animation:fade .18s both}
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
var cur=null, me=null, entries=[], sheetEl=null, titles={}, agentInfo=null, busy=false, prices={}, modelList=null;
var $=function(i){return document.getElementById(i)};
function esc(s){return String(s==null?"":s).replace(/[&<>"]/g,function(c){return {"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;"}[c]})}
function when(ts){var d=new Date(ts);if(isNaN(d))return ts||"";var n=new Date();
  return d.toDateString()===n.toDateString()?d.toLocaleTimeString([],{hour:"numeric",minute:"2-digit"})
  :d.toLocaleDateString([],{month:"short",day:"numeric"})+" "+d.toLocaleTimeString([],{hour:"numeric",minute:"2-digit"})}
function ago(ts){if(!ts)return "never";var m=Math.round((Date.now()-new Date(ts))/60000);if(m<1)return "just now";if(m<60)return m+"m ago";var h=Math.round(m/60);if(h<48)return h+"h ago";return Math.round(h/24)+"d ago"}
function api(p,body){var o=body?{method:"POST",headers:{"content-type":"application/json"},body:JSON.stringify(body)}:undefined;
  return fetch(p,o).then(function(r){return r.text().then(function(t){var d;try{d=JSON.parse(t)}catch(e){d={error:t||("HTTP "+r.status)}}
    if(!r.ok&&!d.error)d.error="HTTP "+r.status;return d})})}
function copy(t){if(navigator.clipboard)navigator.clipboard.writeText(t)}
function initials(n){return (n&&n!=="you")?n.split(/\s+/).map(function(w){return w[0]}).join("").slice(0,2).toUpperCase():"·"}
function tid(){return encodeURIComponent(cur)}
function note(t,cls){$("note").textContent=t||"";$("note").className="note"+(cls?" "+cls:"")}

function loadMe(){return api("/app/api/me").then(function(d){me=d;$("me-name").textContent=d.name;$("me-av").textContent=initials(d.name);
  $("ask").hidden=!d.can_ask;if(!d.can_ask)note("Your agent is off until a model key is set. Open settings for how.","warn")})}
function loadAgent(){return api("/app/api/agent").then(function(d){agentInfo=d;return d})}
function loadModels(){if(modelList)return Promise.resolve(modelList);return api("/app/api/models").then(function(d){modelList=d.models||[];modelList.forEach(function(m){prices[m.id]={i:m.in_per_m,o:m.out_per_m}});return modelList})}
function cost(d){var p=prices[d.model];if(!p||!d.tokens)return "";var c=((d.tokens.prompt||0)*p.i+(d.tokens.completion||0)*p.o)/1e6;return c<0.0005?"<$0.001":"$"+c.toFixed(3)}

function threads(){return api("/app/api/threads").then(function(d){var el=$("threads");
  if(!d.threads.length){el.innerHTML="";empty();return}
  titles={};d.threads.forEach(function(t){titles[t.title.toLowerCase()]=t.id});
  el.innerHTML=d.threads.map(function(t){var sub=[];if(t.last)sub.push(when(t.last));else sub.push("empty");
    var pips="";if(t.waiting)pips+='<span class="pip wait">needs you</span>';if(t.auto)pips+='<span class="pip auto">agent on</span>';
    if(t.since_shared)pips+='<span class="pip warn">'+t.since_shared+' new since shared</span>';else if(t.ever_shared||t.shared)pips+='<span class="pip">shared</span>';
    return '<button class="thread" data-id="'+esc(t.id)+'" aria-current="'+(t.id===cur)+'"><div class="name">'+esc(t.title)+'</div><div class="sub">'+esc(sub.join(" · "))+pips+'</div></button>'}).join("");
  window._threads=d.threads;
  Array.prototype.forEach.call(el.querySelectorAll(".thread"),function(n){n.onclick=function(){open(n.getAttribute("data-id"))}});
  // Land in a thread rather than on a menu: somebody who has just arrived
  // wants the cursor blinking, not another button to press.
  if(!cur&&d.threads.length){open(d.threads[0].id);return}
  if(!cur)empty()})}

function empty(){$("title").textContent="Lamdis";$("share").disabled=true;$("agentbtn").hidden=true;$("linksbtn").hidden=true;$("more").hidden=true;
  $("stream").innerHTML='<div class="void"><h2>A thread is a workspace for you, your agent, and whoever you choose.</h2>'+
  '<p>Write notes. Ask your agent; its answers stay in the thread. Give it standing instructions and it keeps working while you are away. Share a summary, or everything, by link.</p>'+
  '<button class="btn solid" id="e-new">Start a thread</button></div>';$("e-new").onclick=newThread}

function body(t){return esc(t).replace(/\*\*([^*\n]{1,200})\*\*/g,"<b>$1</b>").replace(/\[\[([^\]]{1,120})\]\]/g,function(m,name){var id=titles[name.trim().toLowerCase()];
  return id?'<a class="ref" data-go="'+esc(id)+'">'+esc(name)+'</a>':'<span class="ref dead" title="No thread with this title">'+esc(name)+'</span>'})}
function meta(e,extra){return '<div class="meta"><span class="auth">'+esc(e.who)+'</span>'+(e.agent?'<span class="tag ai">'+esc(e.agent==="agent"?"agent":e.agent)+'</span>':'')+'<span>'+esc(when(e.ts))+'</span>'+(extra||'')+'</div>'}
function dataOf(e){var d=e.data||{};if(typeof d==="string"){try{d=JSON.parse(d)}catch(x){d={}}}return d}
function runRow(e){var d=dataOf(e);
  var more=[];if(d.threads_read&&d.threads_read.length)more.push("read: "+d.threads_read.map(function(id){var t=(window._threads||[]).filter(function(x){return x.id===id})[0];return t?t.title:id}).join(", "));
  if(d.tool_calls&&d.tool_calls.length)more.push("tools: "+d.tool_calls.join(", "));
  if(d.fetches&&d.fetches.length)more.push("fetched: "+d.fetches.map(function(f){return f.url+(f.error?" ("+f.error+")":"")}).join("\n         "));
  if(d.external&&d.external.length)more.push("external: "+d.external.map(function(x){return x.tool+(x.error?" ("+x.error+")":"")}).join(", "));
  if(d.tokens)more.push("tokens: "+(d.tokens.prompt||0)+" in, "+(d.tokens.completion||0)+" out · "+Math.round((d.duration_ms||0)/1000)+"s · "+esc(d.model||"")+(cost(d)?" · "+cost(d):""));
  if(d.problems&&d.problems.length)more.push("problems: "+d.problems.join("; "));
  if(d.error)more.push("error: "+d.error);
  return '<div class="run'+(d.outcome==="error"?' bad':'')+'"><span>'+esc(when(e.ts))+'</span><details><summary>'+(String(d.trigger||"").indexOf("reflect")===0?esc(String(d.trigger).replace("reflect:","")+" pass"):"agent ran ("+esc(d.trigger||"")+")")+' · '+esc(e.text)+(cost(d)?' · '+cost(d):'')+'</summary><div class="more">'+esc(more.join("\n"))+'</div></details></div>'}
function decisionCard(e,all){var reply=all.filter(function(x){return x.kind==="agent.decision_reply"&&x.replies_to===e.id})[0];
  var h='<article class="entry agent">'+meta(e,'<span class="tag">needs your call</span>')+'<div class="decision"><div class="ask">'+body(e.text)+'</div>';
  if(reply){var r=dataOf(reply);h+='<div class="done">You answered: '+esc((r.choice||"")+(r.text?" "+r.text:""))+'</div>'}
  else{h+='<div class="opts">'+(e.options||[]).map(function(o){return '<button class="btn sm" data-dec="'+esc(e.id)+'" data-choice="'+esc(o)+'">'+esc(o)+'</button>'}).join("")+'</div>'+
    '<div class="free"><input placeholder="Or answer in your own words" data-decin="'+esc(e.id)+'"><button class="btn sm solid" data-dec="'+esc(e.id)+'" data-free="1">Reply</button></div>'}
  return h+'</div></article>'}
function render(es){return es.map(function(e){
  if(e.kind==="thread.brief"||e.kind==="agent.decision_reply"||e.lane==="control")return "";
  if(e.kind==="agent.run")return runRow(e);
  if(e.kind==="agent.decision")return decisionCard(e,es);
  var sum=e.lane==="summary",q=e.kind==="chat.question";
  return '<article class="entry'+(sum?' summary':'')+(e.agent?' agent':'')+(q?' q':'')+'">'+meta(e,(sum?'<span class="tag">what you shared</span>':'')+(q?'<span class="tag">asked</span>':''))+'<div class="body">'+body(e.text)+'</div></article>'}).join("")}
function wire(){Array.prototype.forEach.call(document.querySelectorAll("[data-go]"),function(a){a.onclick=function(){open(a.getAttribute("data-go"))}});
  Array.prototype.forEach.call(document.querySelectorAll("[data-dec]"),function(b){b.onclick=function(){var id=b.getAttribute("data-dec"),choice=b.getAttribute("data-choice")||"",text="";
    if(b.getAttribute("data-free")){var inp=document.querySelector('[data-decin="'+id+'"]');text=inp?inp.value.trim():"";if(!text)return}
    b.disabled=true;thinking("Your agent is continuing…");api("/app/api/decision",{id:id,choice:choice,text:text}).then(function(d){if(d.error)note(d.error,"bad");open(cur)})}})}
function thinking(t){
  var el=document.createElement("div");el.className="thinking";el.id="thinking";
  el.textContent=t||"Your agent is reading…";$("stream").appendChild(el);$("feed").scrollTop=$("feed").scrollHeight;
  // After a few seconds a bare message reads as a hang, so say how long it
  // has been, and after a while say that this one is a long one.
  var t0=Date.now(), base=el.textContent;
  clearInterval(window._thinkTimer);
  window._thinkTimer=setInterval(function(){
    if(!document.getElementById("thinking")){clearInterval(window._thinkTimer);return}
    var s=Math.round((Date.now()-t0)/1000);
    if(s<3){el.textContent=base;return}
    el.textContent=base+" · "+s+"s"+(s>25?" · this one is taking a while":"");
  },500);
}

function agentPill(t){var b=$("agentbtn");b.hidden=false;var on=!!t.auto,wait=t.waiting>0;
  b.className="pill"+(wait?" wait":(on?" on":" off"));
  b.title=on?"What your agent does here on its own":"Your agent only answers when asked. Click to let it work on its own.";
  b.innerHTML='<i></i><span class="full">'+(wait?"Agent needs you":(on?"Agent on":"Agent off"))+'</span>'+
    '<span class="short">'+(wait?"you":"")+'</span>'}

function open(id){cur=id;$("share").disabled=false;drawer(false);threads();
  return api("/app/api/thread/"+encodeURIComponent(id)).then(function(d){if(d.error){note(d.error,"bad");return}entries=d.entries;$("title").textContent=d.title||"Untitled";
    var vis=d.entries.filter(function(e){return e.lane!=="control"&&e.kind!=="thread.brief"&&e.kind!=="agent.decision_reply"});
    var t=(window._threads||[]).filter(function(x){return x.id===id})[0]||{};agentPill(t);$("more").hidden=!t.mine;
    api("/app/api/thread/"+encodeURIComponent(id)+"/links").then(function(l){
      if(cur!==id)return;
      var b=$("linksbtn");b.hidden=!l.total;
      if(l.total){b.innerHTML='<span class="full">'+l.total+(l.total===1?" connection":" connections")+'</span><span class="short">⇄'+l.total+'</span>';window._links=l}});
    var nudge=t.since_shared?'<div class="nudge"><span>'+t.since_shared+(t.since_shared===1?" entry":" entries")+' since you last shared '+esc(when(t.last_shared))+'.</span><button class="btn sm solid" id="nudge-go">Send an update</button></div>':'';
    $("stream").innerHTML=nudge+(vis.length?render(d.entries):'<div class="void"><h2>'+esc(d.title)+'</h2>'+
      '<p>Write the first thing below, or ask your agent something. It answers from whatever is in here.</p>'+
      '<button class="btn" id="void-agent">Or let it work while you are away</button></div>');
    if(!vis.length&&$("void-agent"))$("void-agent").onclick=agentSheet;
    if(t.since_shared)$("nudge-go").onclick=shareSheet;
    wire();$("feed").scrollTop=$("feed").scrollHeight;if(!busy)$("text").focus()})}

function grow(){var t=$("text");t.style.height="auto";t.style.height=Math.min(t.scrollHeight,224)+"px"}
function post(){var t=$("text").value.trim();if(!cur||!t)return;
  api("/app/api/post",{thread:cur,text:t,lane:"content"}).then(function(d){if(d.error){note(d.error,"bad");return}
    $("text").value="";grow();note("");open(cur)})}
function ask(){var q=$("text").value.trim();if(!cur||!q||busy)return;busy=true;note("");
  $("text").value="";grow();var el=document.createElement("article");el.className="entry q";el.innerHTML='<div class="meta"><span class="auth">'+esc(me?me.name:"you")+'</span><span class="tag">asked</span></div><div class="body">'+esc(q)+'</div>';
  $("stream").appendChild(el);thinking();
  api("/app/api/chat",{thread:cur,text:q}).then(function(d){busy=false;if(d.error)note(d.error,"bad");open(cur)}).catch(function(){busy=false;open(cur)})}

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

/* The agent sheet: what it should do here on its own, and what it may reach. */
function agentSheet(){if(!cur)return;
  Promise.all([api("/app/api/thread/"+tid()+"/brief"),loadAgent()]).then(function(r){var d=r[0],a=r[1]||{},b=d.brief||{},reach=d.reach||{};var st=a.status||{};
    var tools=[];(reach.tools||[]).forEach(function(srv){(srv.tools||[]).forEach(function(t){tools.push({id:srv.name+"."+t,confirm:(srv.confirm||[]).indexOf(t)>=0})})});
    var s=sheet('<header><h2>What your agent does here</h2><p>Out of the box it only answers when you ask. Everything below is how it works without you: reacting when something arrives, and stopping to think at hours you choose. Everything it does gets written into this thread for you to read.</p></header><section>'+
    '<label class="f" for="brief">Standing instructions</label><textarea id="brief" rows="4" placeholder="Example: When the other side posts a price, compare it with our position in [[Q3 payments migration]] and note the gap. Ask me before agreeing to anything.">'+esc(b.text||"")+'</textarea>'+
    '<div class="grid2"><div><label class="f">Act when a new entry arrives</label><select id="b-on"><option value="off">No</option><option value="others">From other people</option><option value="all">From anyone, including me</option></select></div>'+
    '<div><label class="f">Also run on a schedule</label><select id="b-every"><option value="">No</option><option value="1h">Every hour</option><option value="6h">Every 6 hours</option><option value="24h">Once a day</option></select></div></div>'+
    '<label class="f">When it stops and thinks</label>'+
    '<p class="hint" style="margin-top:0">Reacting to what arrives is not the same as standing back. Give it an hour and a question, like you would give yourself.</p>'+
    '<div id="rhythms"></div>'+
    '<div class="presets"><button class="btn sm" data-preset="morning">+ Morning review</button><button class="btn sm" data-preset="evening">+ Evening reflection</button><button class="btn sm" data-preset="blank">+ Another time</button></div>'+
    '<label class="f">On its own, it may also use</label>'+
    '<label class="check"><input type="checkbox" id="b-web"> The web, on these domains: <input id="b-dom" placeholder="*.sec.gov, docs.stripe.com" style="flex:1"></label>'+
    (reach.allow_domains&&reach.allow_domains.length?'<p class="hint">Always allowed (from settings): '+esc(reach.allow_domains.join(", "))+'</p>':'')+
    (tools.length?tools.map(function(t){return '<label class="check"><input type="checkbox" data-tool="'+esc(t.id)+'"> '+esc(t.id)+(t.confirm?' <span class="dim">(asks you first)</span>':'')+'</label>'}).join(""):'<p class="hint">Nothing connected yet. Settings, then Connections, adds one in about a minute.</p>')+
    '<p class="hint">When you ask it something yourself, it can use everything you have connected and any public page, and every fetch is recorded in the thread.</p>'+
    '<p class="hint" style="margin-top:.8rem">Last run '+esc(ago(st.last_run))+' · '+esc(String(st.runs_today||0))+' runs today'+(st.last_error?' · <span style="color:var(--red)">'+esc(st.last_error)+'</span>':'')+(a.problem?' · <span style="color:var(--gold)">'+esc(a.problem)+'</span>':'')+'</p>'+
    '</section><footer><button class="btn" id="b-run">Run now</button><span class="spacer"></span><button class="btn" data-x>Cancel</button><button class="btn solid" id="b-save">Save</button></footer>');
    s.querySelector("[data-x]").onclick=closeSheet;
    s.querySelector("#b-on").value=b.on_new_entry||"off";s.querySelector("#b-every").value=b.every||"";s.querySelector("#b-web").checked=!!b.web;s.querySelector("#b-dom").value=(b.allow_domains||[]).join(", ");
    var zone="";try{zone=Intl.DateTimeFormat().resolvedOptions().timeZone||""}catch(e){}
    var rhythms=(b.rhythms||[]).slice();
    function drawRhythms(){var el=s.querySelector("#rhythms");
      el.innerHTML=rhythms.map(function(r,i){return '<div class="rh"><input type="time" value="'+esc(r.at||"")+'" data-rh-at="'+i+'">'+
        '<input value="'+esc(r.prompt||"")+'" placeholder="what should it think about?" data-rh-prompt="'+i+'">'+
        '<button class="x" data-rh-x="'+i+'" title="Remove">×</button></div>'}).join("");
      Array.prototype.forEach.call(el.querySelectorAll("[data-rh-at]"),function(n){n.oninput=function(){rhythms[+n.getAttribute("data-rh-at")].at=n.value}});
      Array.prototype.forEach.call(el.querySelectorAll("[data-rh-prompt]"),function(n){n.oninput=function(){rhythms[+n.getAttribute("data-rh-prompt")].prompt=n.value}});
      Array.prototype.forEach.call(el.querySelectorAll("[data-rh-x]"),function(n){n.onclick=function(){rhythms.splice(+n.getAttribute("data-rh-x"),1);drawRhythms()}});
    }
    drawRhythms();
    Array.prototype.forEach.call(s.querySelectorAll("[data-preset]"),function(btn){btn.onclick=function(){
      var k=btn.getAttribute("data-preset");
      if(k==="morning")rhythms.push({name:"morning",at:"07:30",zone:zone,prompt:"What needs me today, and did anything slip?"});
      else if(k==="evening")rhythms.push({name:"evening",at:"21:30",zone:zone,prompt:"Go back over today. Anything contradict what we agreed, or worth sleeping on?"});
      else rhythms.push({name:"",at:"12:00",zone:zone,prompt:""});
      drawRhythms()}});
    Array.prototype.forEach.call(s.querySelectorAll("[data-tool]"),function(c){c.checked=(b.tools||[]).indexOf(c.getAttribute("data-tool"))>=0});
    var read=function(){var doms=s.querySelector("#b-dom").value.split(",").map(function(x){return x.trim()}).filter(Boolean);
      var rl=rhythms.filter(function(r){return r.at}).map(function(r){return {name:r.name||"",at:r.at,zone:r.zone||zone,prompt:r.prompt||""}});
      return {text:s.querySelector("#brief").value,on_new_entry:s.querySelector("#b-on").value,every:s.querySelector("#b-every").value,web:s.querySelector("#b-web").checked,allow_domains:doms,rhythms:rl,
        tools:Array.prototype.filter.call(s.querySelectorAll("[data-tool]"),function(c){return c.checked}).map(function(c){return c.getAttribute("data-tool")})}};
    s.querySelector("#b-save").onclick=function(){api("/app/api/thread/"+tid()+"/brief",read()).then(function(r){if(r.error){alert(r.error);return}closeSheet();open(cur)})};
    s.querySelector("#b-run").onclick=function(){var btn=s.querySelector("#b-run");btn.disabled=true;btn.textContent="Running…";
      api("/app/api/thread/"+tid()+"/brief",read()).then(function(){return api("/app/api/thread/"+tid()+"/run")}).then(function(r){closeSheet();if(r.error)note(r.error,"bad");open(cur)})}})}

/* What this thread is tied to, and how it got tied. */
function linksSheet(){var l=window._links;if(!l)return;
  var row=function(x){return '<div class="row"><div class="t"><b>'+esc(x.title)+'</b><span>'+esc(x.why)+(x.count>1?" · "+x.count+" times":"")+'</span></div><button class="btn sm" data-go="'+esc(x.id)+'">Open</button></div>'};
  var block=function(title,items){return items.length?'<label class="f">'+title+'</label><div class="list">'+items.map(row).join("")+'</div>':''};
  var s=sheet('<header><h2>'+esc(l.title)+'</h2><p>What this thread is tied to. Links come from writing [[a thread title]] in a note, and from your agent actually opening another thread to answer something here.</p></header>'+
  '<section>'+block("This thread points at",l.out)+block("Pointed at by",l.in)+block("Your agent read, working here",l.read)+
  (l.total?'':'<p class="hint">Nothing yet. Write [[a thread title]] in a note to tie two together.</p>')+
  '</section><footer><span class="spacer"></span><button class="btn" data-x>Done</button></footer>');
  s.querySelector("[data-x]").onclick=closeSheet;
  Array.prototype.forEach.call(s.querySelectorAll("[data-go]"),function(b){b.onclick=function(){closeSheet();open(b.getAttribute("data-go"))}})}

/* Delete: only a steward can, and only from this node. */
function moreSheet(){if(!cur)return;var t=(window._threads||[]).filter(function(x){return x.id===cur})[0]||{};
  var s=sheet('<header><h2>'+esc(t.title||"This thread")+'</h2><p>You own this thread. Deleting removes it and everything in it from your node and stops every link you shared. Anyone who already pulled a copy to their own node keeps theirs; the record is append-only between nodes.</p></header>'+
  '<section><p class="hint">'+esc(String(t.entries||0))+' entries'+(t.shared?' · shared with '+t.shared+(t.shared===1?' person':' people'):'')+'</p></section>'+
  '<footer><button class="btn danger" id="del">Delete this thread</button><span class="spacer"></span><button class="btn" data-x>Cancel</button></footer>');
  s.querySelector("[data-x]").onclick=closeSheet;var d=s.querySelector("#del");
  d.onclick=function(){if(!d.getAttribute("data-armed")){d.setAttribute("data-armed","1");d.textContent="Click again to delete for good";return}
    d.disabled=true;api("/app/api/thread/"+tid()+"/delete",{}).then(function(r){if(r.error){alert(r.error);d.disabled=false;return}closeSheet();cur=null;threads()})}}

/* Share: one button, two choices, one link. */
function shareSheet(){if(!cur)return;var mode="summary";
  api("/app/api/thread/"+tid()+"/access").then(function(d){
    var s=sheet('<header><h2>Share this thread</h2><p>Anyone with the link reads what you have shared, as dated updates. No account needed. Stop sharing any time.</p></header>'+
    '<section><div class="choices"><button class="choice" data-m="summary" aria-pressed="true"><b>A summary</b><span>Your agent drafts it, you edit it. That is all they see.</span></button>'+
    '<button class="choice" data-m="read" aria-pressed="false"><b>Everything</b><span>Your notes, your questions, and what your agent wrote.</span></button></div>'+
    '<div id="sum"><label class="f" for="draft">What they will read</label><textarea id="draft" rows="5" placeholder="Drafting…"></textarea><p class="hint" id="drafthint"></p></div>'+
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
    api("/app/api/summarize",{thread:cur}).then(function(r){if(r.error){draft.placeholder="";hint.textContent=r.error;return}
      if(r.mode==="current"){draft.value=r.previous||draft.value;hint.textContent="Nothing new since your last update. This is what they can already read; edit it or just copy the link again.";return}
      if(!r.draft){if(!draft.value)draft.placeholder="Write what you want them to know.";hint.textContent="Set a model key in settings and this gets drafted for you.";return}
      draft.value=r.draft;s.querySelector("label[for=draft]").textContent=r.mode==="update"?"Your next update: what changed since last time":"What they will read";
      hint.textContent=r.mode==="update"?"Drafted from the "+r.since+(r.since===1?" entry":" entries")+" since your last update. Edit anything before you share it.":"Drafted from the thread. Edit anything before you share it."})
    s.querySelector("#go").onclick=function(){var label=s.querySelector("#label").value;var go=s.querySelector("#go");go.disabled=true;
      var mint=function(){api("/app/api/share",{thread:cur,scope:mode,days:30,label:label}).then(function(r){go.disabled=false;
        if(r.error){alert(r.error);return}var url=location.origin+r.path;copy(url);s.querySelector("#link").value=url;go.textContent="Copied";open(cur)})};
      if(mode==="summary"){var t=draft.value.trim();if(!t){alert("Write or draft a summary first.");go.disabled=false;return}
        if(existing.length&&existing[existing.length-1].text===t){mint()}else{api("/app/api/post",{thread:cur,text:t,lane:"summary"}).then(mint)}}
      else mint()};
    Array.prototype.forEach.call(s.querySelectorAll("[data-copy]"),function(b){b.onclick=function(){copy(b.getAttribute("data-copy"));b.textContent="Copied"}});
    Array.prototype.forEach.call(s.querySelectorAll("[data-kill]"),function(b){b.onclick=function(){api("/app/api/share/revoke",{id:b.getAttribute("data-kill")}).then(function(){shareSheet()})}})})}

/* Settings: name, your agent, what it may reach, and everything else folded away. */
function settings(){var m=me||{};var origin=location.origin;
  Promise.all([loadAgent(),loadModels()]).then(function(r){var a=r[0]||{},models=r[1]||[];var reach=a.reach||{},st=a.status||{};
  var curModel=reach.model||"";var known=models.some(function(m){return m.id===curModel});
  var opts=models.map(function(m){return '<option value="'+esc(m.id)+'"'+(m.id===curModel?' selected':'')+'>'+esc(m.id)+' · $'+m.in_per_m.toFixed(2)+' in / $'+m.out_per_m.toFixed(2)+' out per M'+(m.context?' · '+Math.round(m.context/1000)+'k':'')+'</option>'}).join("");
  if(!known&&curModel)opts='<option value="'+esc(curModel)+'" selected>'+esc(curModel)+' (current)</option>'+opts;
  var s=sheet('<header><h2>Settings</h2></header><section>'+
  '<label class="f">Your name</label><div style="display:flex;gap:.5rem"><input id="c-name" value="'+esc(m.name==="you"?"":m.name)+'" placeholder="How others will see you"><button class="btn" id="c-save">Save</button></div>'+
  '<label class="f">Model</label><select id="c-model">'+opts+'<option value="__custom">Another id…</option></select><input id="c-model-custom" placeholder="vendor/model-id as OpenRouter names it" hidden style="margin-top:.4rem">'+
  '<p class="hint">Any model OpenRouter serves that can call tools. Prices are live from OpenRouter; each run in a thread shows what it cost. Switching takes effect on the next question.</p>'+
  '<div class="grid2" style="margin-top:.5rem"><div><label class="f" style="margin-top:.3rem">OpenRouter key</label><input id="c-key" type="password" placeholder="'+(reach.has_key?(reach.key_from_env?"set in the environment":"saved on this machine"):"sk-or-…")+'"></div>'+
  '<div><label class="f" style="margin-top:.3rem">Or a local model server</label><input id="c-url" value="'+esc(reach.model_url||"")+'" placeholder="http://localhost:11434/v1"></div></div>'+
  '<label class="f">Key for that server, if it needs one</label><input id="c-urlkey" type="password" placeholder="'+(reach.has_url_key?"saved for this endpoint":"usually none — Ollama and vLLM need no key")+'">'+
  '<p class="hint">Credentials are kept on the node, readable only by it, and are never sent anywhere but the service they belong to. Your OpenRouter key goes to OpenRouter alone: point the agent at another server and that key stays behind.</p>'+
  '<button class="btn" id="c-modelsave" style="margin-top:.5rem">Save model settings</button>'+
  '<label class="f">Your agent</label>'+
  (a.problem?'<p class="hint" style="color:var(--gold)">'+esc(a.problem)+'</p>':'<p class="hint">Runs on '+esc(a.model||"")+'. Acting for you in '+esc(String(a.delegated_threads||0))+' thread'+(a.delegated_threads===1?"":"s")+'. Today: '+esc(String(st.runs_today||0))+' runs, '+esc(String(st.fetches_today||0))+' fetches, '+esc(String(st.tokens_today||0))+' tokens.'+(st.last_sync?' Synced with peers '+esc(ago(st.last_sync))+'.':'')+(st.last_sync_error?' <span style="color:var(--red)">Sync: '+esc(st.last_sync_error)+'</span>':'')+'</p>')+
  '<p class="hint">It has its own key, signed by yours, so anyone reading a thread can tell you from your agent. Anything it writes says so. It can never share or grant access.</p>'+
  '<p class="hint">Everything lives in <span class="mono">'+esc(reach.config_path||"").replace(/\/agent\.json$/,"")+'</span> on this machine. This page answers only on this machine; peers reach the node by signature, never by this token.</p>'+
  '<label class="f">The web, when nobody asked</label>'+
  '<select id="c-autoweb"><option value="listed">Only the sites I list below</option><option value="any">Any public page, same as when I ask</option><option value="off">None at all unless I ask</option></select>'+
  '<div id="c-domwrap" style="display:flex;gap:.5rem;margin-top:.5rem"><input id="c-dom" value="'+esc((reach.allow_domains||[]).join(", "))+'" placeholder="*.sec.gov, docs.stripe.com"><button class="btn" id="c-domsave">Save</button></div>'+
  '<p class="hint">When you ask it something yourself it may always fetch any public page, and every fetch is written into the thread. This is only about what it does while you are away. Private and local addresses are refused either way, and a thread can narrow this further but never widen it.</p>'+
  '<label class="f">Connections</label><p class="hint" style="margin-top:0">Anything that speaks MCP: your issue tracker, your calendar, your own service. Paste the address, then press Sign in and approve it in the window. Services that hand out plain tokens take one in the field instead. Press Test to see what a server offers, and tick what your agent may use.</p>'+
  '<div id="conns"></div>'+
  '<button class="btn" id="conn-add" style="margin-top:.6rem">+ Add a connection</button>'+
  '<label class="f">Connect a machine</label><p class="hint" style="margin-top:0">Leave an agent running on your laptop or a server, and it takes direction from a thread here. Write from your phone, it happens there.</p>'+
  (cur?'<button class="btn" id="c-link">Connect this thread to a machine</button><div id="c-linkout"></div>'
      :'<p class="hint">Open a thread first, then come back: a machine is connected to one thread.</p>')+
  '<label class="f">Use with Claude</label><div class="cmd">claude mcp add lamdis -- lamdis mcp<button data-copy="claude mcp add lamdis -- lamdis mcp">copy</button></div>'+
  '<p class="hint">Run that once. Claude Code can then read your threads and write into them; its entries are labelled. Any other AI that speaks MCP works the same way.</p>'+
  (m.can_ask?'':'<p class="hint">Your agent is off. Three ways on: an OpenRouter key of your own (openrouter.ai/keys), a model on this machine (Ollama, vLLM), or <a href="mailto:support@lamdis.ai?subject=Lamdis%20key%20request" style="color:var(--gold)">ask us for a starter key</a> and we will send you one with a small fixed credit.</p>')+
  '<details><summary>Advanced: identity, other nodes, direct grants, revoke the agent</summary>'+
  '<label class="f">Your identity</label><div class="cmd">'+esc(m.principal)+'<button data-copy="'+esc(m.principal)+'">copy</button></div>'+
  '<label class="f">Your agent’s identity</label><div class="cmd">'+esc(a.principal||"")+'<button data-copy="'+esc(a.principal||"")+'">copy</button></div>'+
  '<label class="f">This node</label><div class="cmd">'+esc(origin)+'<button data-copy="'+esc(origin)+'">copy</button></div>'+
  '<p class="hint">Someone running their own Lamdis can pair with you: <span class="mono">lamdis peer add &lt;name&gt; '+esc(origin)+'</span></p>'+
  '<label class="f">Pair with another node</label><div style="display:grid;grid-template-columns:1fr 2fr auto;gap:.5rem"><input id="p-name" placeholder="Name"><input id="p-url" placeholder="https://their-node"><button class="btn" id="p-add">Pair</button></div><div id="p-status"></div>'+
  '<div class="list" id="peers"></div>'+
  (cur?'<label class="f">Grant a paired person or agent access to this thread</label><div style="display:grid;grid-template-columns:2fr 1fr auto;gap:.5rem"><input id="g-to" placeholder="Paired name or identity"><input id="g-scope" value="summary" placeholder="summary | read | contribute"><button class="btn" id="g-go">Grant</button></div><div id="g-status"></div><div class="list" id="grants"></div>':'')+
  '<label class="f">Revoke the agent</label><button class="btn danger" id="c-revoke">Revoke its key everywhere</button><p class="hint">Severs it in every thread it acted in and removes its key. Restarting the node mints a fresh one.</p>'+
  '</details></section><footer><span class="spacer"></span><button class="btn" data-x>Done</button></footer>');
  s.querySelector("[data-x]").onclick=closeSheet;
  Array.prototype.forEach.call(s.querySelectorAll("[data-copy]"),function(b){b.onclick=function(){copy(b.getAttribute("data-copy"));b.textContent="copied"}});
  s.querySelector("#c-save").onclick=function(){api("/app/api/me",{name:s.querySelector("#c-name").value}).then(loadMe)};
  var conns=[];
  function connRow(c,i){
    var picks=(c._tools||c.allow||[]).map(function(t){
      var on=(c.allow||[]).indexOf(t)>=0, ask=(c.confirm||[]).indexOf(t)>=0;
      return '<button class="pick'+(ask?' ask':'')+'" aria-pressed="'+(on?"true":"false")+'" data-tool="'+esc(t)+'" data-i="'+i+'">'+esc(t)+(ask?' · asks first':'')+'</button>'}).join("");
    return '<div class="conn" data-conn="'+i+'">'+
      '<div class="top"><b>'+esc(c.name||"New connection")+'</b>'+
      (c.signed_in?'<span class="pip">signed in</span>':'')+
      '<button class="btn sm" data-signin="'+i+'">'+(c.signed_in?"Sign in again":"Sign in")+'</button>'+
      '<button class="btn sm" data-test="'+i+'">Test</button>'+
      '<button class="btn sm danger" data-del="'+i+'">Remove</button></div>'+
      (c._new?'<input placeholder="A short name, like github" value="'+esc(c.name||"")+'" data-f="name" data-i="'+i+'" style="margin-top:.5rem">':'')+
      '<input placeholder="https://mcp.example.com/mcp" value="'+esc(c.url||"")+'" data-f="url" data-i="'+i+'" style="margin-top:.4rem">'+
      (canHold
        ? '<input type="password" placeholder="'+(c.has_auth?"a credential is saved; type to replace it":"Or paste a token, if the service uses one")+'" data-f="auth" data-i="'+i+'" style="margin-top:.4rem">'
        : '<p class="hint" style="margin-top:.4rem">Add your email to this account before storing a credential here. Servers that need nothing still work.</p>')+
      (c.command?'<div class="addr">runs locally: '+esc(c.command)+'</div>':'')+
      '<div class="picks">'+picks+'</div>'+
      '<div class="probe" data-probe="'+i+'"></div></div>';
  }
  function drawConns(){
    var el=s.querySelector("#conns");
    el.innerHTML=conns.length?conns.map(connRow).join(""):'<p class="hint">Nothing connected yet.</p>';
    Array.prototype.forEach.call(el.querySelectorAll("[data-f]"),function(n){n.oninput=function(){conns[+n.getAttribute("data-i")][n.getAttribute("data-f")]=n.value}});
    Array.prototype.forEach.call(el.querySelectorAll("[data-test]"),function(b){b.onclick=function(){testConn(+b.getAttribute("data-test"))}});
    Array.prototype.forEach.call(el.querySelectorAll("[data-signin]"),function(b){b.onclick=function(){signInConn(+b.getAttribute("data-signin"))}});
    Array.prototype.forEach.call(el.querySelectorAll("[data-del]"),function(b){b.onclick=function(){var i=+b.getAttribute("data-del");
      var c=conns[i];conns.splice(i,1);drawConns();if(c.name&&!c._new)api("/app/api/tools/remove",{name:c.name})}});
    Array.prototype.forEach.call(el.querySelectorAll("[data-tool]"),function(b){b.onclick=function(){
      var i=+b.getAttribute("data-i"),t=b.getAttribute("data-tool"),c=conns[i];
      c.allow=c.allow||[];c.confirm=c.confirm||[];
      var on=c.allow.indexOf(t)>=0, ask=c.confirm.indexOf(t)>=0;
      /* off -> allowed -> allowed but asks first -> off */
      if(!on){c.allow.push(t)}
      else if(!ask){c.confirm.push(t)}
      else {c.allow.splice(c.allow.indexOf(t),1);c.confirm.splice(c.confirm.indexOf(t),1)}
      drawConns();saveConn(i)}});
  }
  // Sign in the way the protocol actually specifies: ask the server who
  // guards it, introduce ourselves, and let the person approve in a window.
  // Nobody types a secret.
  function signInConn(i){
    var c=conns[i],p=s.querySelector('[data-probe="'+i+'"]');
    if(!c.name||!c.url){p.className="probe bad";p.textContent="Give it a name and an address first.";return}
    p.className="probe";p.textContent="Asking the server how it wants to be signed in to…";
    saveConn(i);
    api("/app/api/tools/auth/start",{name:c.name,url:c.url}).then(function(r){
      if(r.error){p.className="probe bad";p.textContent=r.error;return}
      p.textContent="Approve it in the window that just opened.";
      var win=window.open(r.authorize,"lamdis-connect","width=520,height=680");
      if(!win){p.className="probe bad";p.textContent="Your browser blocked the window. Allow pop-ups for this site and try again.";return}
      var done=function(e){ if(e.origin!==location.origin||!e.data||e.data.lamdis!=="tools-auth")return;
        window.removeEventListener("message",done);
        api("/app/api/tools").then(function(d){conns=(d.servers||[]).map(function(x){x._tools=x.allow;return x});drawConns();
          var q=s.querySelector('[data-probe="'+i+'"]');if(q){q.className="probe ok";q.textContent="Connected. Press Test to see what it offers."}})};
      window.addEventListener("message",done)})}

  function testConn(i){
    var c=conns[i],p=s.querySelector('[data-probe="'+i+'"]');
    p.className="probe";p.textContent="Connecting…";
    api("/app/api/tools/probe",{name:c.name,url:c.url,auth:c.auth||"",header:c.header||""}).then(function(r){
      if(r.error){p.className="probe bad";p.textContent=r.error;return}
      c._tools=r.tools||[];c.auth="";
      p.className="probe ok";p.textContent=(c._tools.length||0)+" tools. Tick the ones it may use; tick again to make it ask first.";
      drawConns();saveConn(i)})}
  function saveConn(i){
    var c=conns[i];if(!c.name||!c.url)return;
    api("/app/api/tools",{name:c.name,url:c.url,auth:c.auth||"",header:c.header||"",allow:c.allow||[],confirm:c.confirm||[]}).then(function(r){
      if(r.error){var p=s.querySelector('[data-probe="'+i+'"]');p.className="probe bad";p.textContent=r.error;return}
      c._new=false;c.auth="";c.has_auth=true})}
  var canHold=true;
  api("/app/api/tools").then(function(d){canHold=d.may_hold_secrets!==false;
    conns=(d.servers||[]).map(function(x){x._tools=x.allow;return x});drawConns()});
  s.querySelector("#conn-add").onclick=function(){conns.push({_new:true,name:"",url:"",allow:[],confirm:[]});drawConns()};

  var sel=s.querySelector("#c-model"),cust=s.querySelector("#c-model-custom");sel.onchange=function(){cust.hidden=sel.value!=="__custom";if(!cust.hidden)cust.focus()};
  s.querySelector("#c-modelsave").onclick=function(){var b=s.querySelector("#c-modelsave");var id=sel.value==="__custom"?cust.value.trim():sel.value;var body={model:id,model_url:s.querySelector("#c-url").value};var k=s.querySelector("#c-key").value.trim();if(k)body.openrouter_key=k;var uk=s.querySelector("#c-urlkey").value.trim();if(uk)body.model_url_key=uk;
    api("/app/api/agent/config",body).then(function(r){b.textContent=r.error?"Failed: "+r.error:"Saved";loadMe()})};
  var aw=s.querySelector("#c-autoweb");aw.value=reach.auto_web||"listed";
  function drawWeb(){s.querySelector("#c-domwrap").hidden=(aw.value!=="listed")}
  drawWeb();
  aw.onchange=function(){drawWeb();api("/app/api/agent/config",{auto_web:aw.value})};
  var lk=s.querySelector("#c-link");
  if(lk)lk.onclick=function(){lk.disabled=true;lk.textContent="Asking…";
    api("/app/api/link/offer",{thread:cur}).then(function(r){lk.disabled=false;lk.textContent="Connect this thread to a machine";
      var out=s.querySelector("#c-linkout");
      if(r.error){out.innerHTML='<div class="status bad">'+esc(r.error)+'</div>';return}
      out.innerHTML='<p class="hint" style="margin-top:.7rem">On the machine, with Lamdis installed, run this within fifteen minutes:</p>'+
        '<div class="cmd">'+esc(r.command)+'<button data-copy="'+esc(r.command)+'">copy</button></div>'+
        '<p class="hint">Then <span class="mono">lamdis agent</span> there. It works only in the directory you start it in, asks before reaching anywhere else, and writes everything it does into this thread.</p>';
      Array.prototype.forEach.call(out.querySelectorAll("[data-copy]"),function(b){b.onclick=function(){copy(b.getAttribute("data-copy"));b.textContent="copied"}})})};
  s.querySelector("#c-domsave").onclick=function(){var b=s.querySelector("#c-domsave");api("/app/api/agent/config",{allow_domains:s.querySelector("#c-dom").value.split(",")}).then(function(r){b.textContent=r.error?"Failed":"Saved"})};
  var rv=s.querySelector("#c-revoke");rv.onclick=function(){if(rv.getAttribute("data-armed")){api("/app/api/agent/revoke").then(function(r){rv.textContent=r.error?r.error:"Revoked in "+r.revoked_in+" threads";loadMe()})}else{rv.setAttribute("data-armed","1");rv.textContent="Click again to confirm"}};
  function peers(){var l=(me&&me.peers)||[];s.querySelector("#peers").innerHTML=l.map(function(p){return '<div class="row"><div class="t"><b>'+esc(p.name)+'</b><span>'+esc(p.url)+'</span></div></div>'}).join("")}
  peers();
  s.querySelector("#p-add").onclick=function(){var st=s.querySelector("#p-status");st.innerHTML='<div class="status">Reaching their node…</div>';
    api("/app/api/peers",{name:s.querySelector("#p-name").value,url:s.querySelector("#p-url").value}).then(function(d){st.innerHTML='<div class="status '+(d.error||!d.reached?"bad":"ok")+'">'+esc(d.error||d.note)+'</div>';loadMe().then(peers)})};
  if(cur){var grants=function(){api("/app/api/thread/"+tid()+"/access").then(function(d){s.querySelector("#grants").innerHTML=d.grants.map(function(g){
      return '<div class="row"><div class="t"><b>'+esc(g.name)+'</b><span>'+esc(g.scopes.join(", "))+'</span></div><button class="btn sm danger" data-rv="'+esc(g.principal)+'">Revoke</button></div>'}).join("");
      Array.prototype.forEach.call(s.querySelectorAll("[data-rv]"),function(b){b.onclick=function(){api("/app/api/thread/"+tid()+"/revoke",{principal:b.getAttribute("data-rv")}).then(grants)}})})};
    grants();s.querySelector("#g-go").onclick=function(){var st=s.querySelector("#g-status");
      api("/app/api/thread/"+tid()+"/grant",{to:s.querySelector("#g-to").value,scopes:[s.querySelector("#g-scope").value.trim()||"summary"]}).then(function(r){
        st.innerHTML='<div class="status '+(r.error?"bad":"ok")+'">'+esc(r.error||("Granted to "+r.name))+'</div>';grants()})}}})}

function drawer(open){var sh=document.getElementById("shell");if(sh)sh.classList.toggle("open",open!==false?open:false)}
if($("menu"))$("menu").onclick=function(){var sh=document.getElementById("shell");sh.classList.toggle("open")};
if($("scrim"))$("scrim").onclick=function(){drawer(false)};
$("post").onclick=post;$("ask").onclick=ask;$("share").onclick=shareSheet;$("agentbtn").onclick=agentSheet;$("linksbtn").onclick=linksSheet;$("more").onclick=moreSheet;$("new").onclick=newThread;$("gear").onclick=settings;
$("text").oninput=grow;
/* Enter sends, Shift+Enter is a new line, and Command or Control Enter
   keeps it as a note instead of asking. What every chat box does. */
$("text").onkeydown=function(e){
  if(e.key!=="Enter"||e.isComposing)return;
  if(e.shiftKey)return;
  e.preventDefault();
  if(e.metaKey||e.ctrlKey){post();return}
  if(!$("ask").hidden)ask(); else post();
};
document.onkeydown=function(e){if(e.key==="Escape")closeSheet()};
loadMe().then(threads);loadModels().then(function(){if(cur)open(cur)});setInterval(function(){if(!sheetEl&&!busy){cur?open(cur):threads()}},15000);
`

// appHTML is the owner's view.
func appHTML(model string, canAsk bool) string {
	return `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><meta name="referrer" content="no-referrer"><meta name="color-scheme" content="dark">
<title>Lamdis</title><style>` + appCSS + `</style></head><body>
<div class="shell" id="shell">
  <div class="scrim" id="scrim"></div>
  <aside class="rail">
    <div class="mark"><svg class="cube" style="width:22px;height:24px;color:var(--gold);flex:none" viewBox="0 0 20 22" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round" aria-hidden="true"><path d="M2 5.5 7 3l5 2.5-5 2.5z M2 5.5v4.6l5 2.5V8 M12 5.5v4.6L7 12.6"/><path d="M2 10.1v4.6l5 2.5v-4.6 M12 10.1v4.6l-5 2.5"/><path d="M7 17.2l5-2.5 5 2.5-5 2.5z M12 19.7v2 M17 17.2v2l-5 2.5 M2 14.7l5 2.5"/></svg> Lamdis</div>
    <button class="newbtn" id="new">+ New thread</button>
    <div class="threads" id="threads"></div>
    <div class="me"><div class="avatar" id="me-av">·</div><b id="me-name">you</b><button class="icon" id="gear" title="Settings">⚙</button></div>
  </aside>
  <main class="main">
    <header class="head"><button class="menu" id="menu" aria-label="Threads">☰</button><h1 id="title">Lamdis</h1><button class="pill" id="agentbtn" hidden><i></i>Agent</button><button class="pill" id="linksbtn" hidden>Connected</button><button class="btn solid" id="share" disabled>Share</button><button class="icon" id="more" title="More" hidden>⋯</button></header>
    <div class="feed" id="feed"><div class="stream" id="stream"></div></div>
    <div class="composer"><div class="box">
      <div class="field">
        <textarea id="text" rows="1" placeholder="Write a note, or ask your agent…"></textarea>
        <div class="tools"><span class="hint">Enter asks · Shift+Enter for a new line · ⌘Enter saves it as a note</span><button class="btn" id="ask">Ask</button><button class="btn solid" id="post">Save</button></div>
      </div>
      <div class="note" id="note"></div>
    </div></div>
  </main>
</div>
<script>` + appJS + `</script></body></html>`
}

// sharedHTML is what the other person sees. No rail, no composer, nothing
// beyond what the link named.
// sharedHTML is the page somebody who was sent a link sees. The content is
// in the markup, not fetched afterwards, so anything that reads a page can
// read this one.
func sharedHTML(full bool, title string, entries []appEntry) string {
	scope := "A summary"
	if full {
		scope = "The whole thread"
	}
	esc := template.HTMLEscapeString
	var body strings.Builder
	for _, e := range entries {
		who := e.Who
		if e.Agent != "" {
			who += " · agent"
		}
		when := e.TS
		if t, err := time.Parse(time.RFC3339, e.TS); err == nil {
			when = t.Format("2 Jan, 3:04 PM")
		}
		cls := "entry"
		if e.Lane == "summary" {
			cls += " summary"
		}
		body.WriteString(`<article class="` + cls + `"><div class="meta"><span class="auth">` +
			esc(who) + `</span><span>` + esc(when) + `</span></div><div class="body">` +
			esc(e.Text) + `</div></article>`)
	}
	if len(entries) == 0 {
		body.WriteString(`<p class="muted">Nothing has been shared here yet.</p>`)
	}
	// A description so a preview card says something, and the title so a
	// tab does.
	desc := "Shared through Lamdis"
	if len(entries) > 0 {
		desc = entries[len(entries)-1].Text
		if len(desc) > 200 {
			desc = desc[:200] + "…"
		}
	}
	return `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><meta name="referrer" content="no-referrer"><meta name="color-scheme" content="dark">
<meta name="robots" content="noindex,nofollow">
<title>` + esc(title) + ` — shared through Lamdis</title>
<meta property="og:title" content="` + esc(title) + `">
<meta property="og:description" content="` + esc(desc) + `">
<style>` + appCSS + `</style></head><body>
<div class="guest"><div class="crest"><svg viewBox="0 0 20 22" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round" aria-hidden="true" style="width:18px;height:20px;color:var(--gold)"><path d="M2 5.5 7 3l5 2.5-5 2.5z M2 5.5v4.6l5 2.5V8 M12 5.5v4.6L7 12.6"/><path d="M2 10.1v4.6l5 2.5v-4.6 M12 10.1v4.6l-5 2.5"/><path d="M7 17.2l5-2.5 5 2.5-5 2.5z M12 19.7v2 M17 17.2v2l-5 2.5 M2 14.7l5 2.5"/></svg>Shared with you through Lamdis</div>
  <h1>` + esc(title) + `</h1><div class="scope"><i></i>` + esc(scope) + `</div>
  <div id="stream" style="display:flex;flex-direction:column;gap:1.4rem">` + body.String() + `</div>
  <div class="foot">Someone chose to show you this. It is read only, nothing you do here is recorded, and the link expires. No account was needed and none was created.</div></div>
</body></html>`
}

func appNotice(title, body string) string {
	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="dark">
<title>` + template.HTMLEscapeString(title) + `</title><style>` + appCSS + `</style></head>
<body><div class="notice"><h1>` + template.HTMLEscapeString(title) + `</h1><p>` + template.HTMLEscapeString(body) + `</p></div></body></html>`
}

var appNoToken = appNotice("This node is not yours to open",
	"The interface is authenticated by a token kept in the node's data directory. Run the node yourself and it opens the right address.")
