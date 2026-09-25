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
  --bg:#FFFAF3; --bg2:#FBF4EA; --side:#FFFCF7; --panel:#FFFFFF; --panel2:#FBF6EF; --raise:#F3ECE1;
  --ink:#1C1A17; --ink2:#5E5A53; --ink3:#8C867C; --ink4:#B0A99E;
  --line:#EEE7DC; --line2:#E3DACB;
  --gold:#FFB22E; --gold-text:#A86200; --gold-ink:#1C1A17; --gold-dim:#F2D29B; --gold-glow:#FFF1DC;
  --blue:#4C6FF5; --blue-glow:#EEF1FF;
  --green:#1F9E68; --red:#D6453A;
  --measure:52rem;
  --sans:'Figtree',-apple-system,BlinkMacSystemFont,'Segoe UI',system-ui,sans-serif;
  --display:'Bricolage Grotesque','Avenir Next',ui-rounded,system-ui,sans-serif;
  --mono:ui-monospace,'SF Mono',Menlo,monospace;
  color-scheme:light;
}
*{box-sizing:border-box;margin:0;padding:0}
[hidden]{display:none!important}
html,body{height:100%}
body{background:var(--bg);color:var(--ink);font-family:var(--sans);font-size:15px;line-height:1.55;-webkit-font-smoothing:antialiased}
button,input,textarea,select{font:inherit;color:inherit}
button{cursor:pointer;background:none;border:none}
input,textarea{background:var(--bg);border:1px solid var(--line2);border-radius:10px;padding:.6rem .75rem;outline:none;width:100%;transition:border-color .14s}
input:focus,textarea:focus{border-color:var(--ink4)}
input::placeholder,textarea::placeholder{color:var(--ink4)}
:focus-visible{outline:2px solid var(--blue);outline-offset:2px}
::selection{background:#FFE2A8;color:var(--ink)}
::-webkit-scrollbar{width:10px;height:10px}::-webkit-scrollbar-thumb{background:var(--line2);border-radius:99px;border:3px solid transparent;background-clip:content-box}
.mono{font-family:var(--mono)}.dim{color:var(--ink4)}.muted{color:var(--ink3)}.small{font-size:.82rem}
.kick{font:500 .7rem/1.2 var(--mono);letter-spacing:.09em;text-transform:uppercase;color:var(--ink3)}
.hash{font-family:var(--mono);color:var(--ink4);font-size:.9em}

/* ---- the frame ---- */
.shell{display:grid;grid-template-columns:264px minmax(0,1fr);height:100vh;height:100dvh}
.rail{background:var(--bg2);border-right:1px solid var(--line);display:flex;flex-direction:column;min-height:0;padding:1rem .75rem .75rem;gap:1rem}
.brand{display:flex;align-items:center;gap:.6rem;padding:.1rem .5rem;font-family:var(--display);font-weight:800;font-size:1.2rem;letter-spacing:-.02em}
.jump{display:flex;align-items:center;gap:.6rem;height:36px;padding:0 .75rem;border-radius:10px;border:1px solid var(--line2);background:var(--panel);color:var(--ink4);font-size:.84rem;text-align:left;transition:border-color .14s}
.jump:hover{border-color:var(--ink4);color:var(--ink3)}
.jump span{flex:1}
.jump kbd{font:.72rem var(--mono);color:var(--ink4)}
.navs{display:flex;flex-direction:column;gap:2px}
.navi{display:flex;align-items:center;gap:.65rem;height:34px;padding:0 .65rem;border-radius:8px;font-size:.9rem;color:var(--ink2);text-align:left;width:100%;transition:background .12s,color .12s}
.navi:hover{background:var(--panel);color:var(--ink)}
.navi[aria-current=true]{background:var(--raise);color:var(--ink)}
.navi svg{flex:none;opacity:.85}
.navi .n{margin-left:auto;font-size:.68rem;font-weight:600;padding:.05rem .45rem;border-radius:99px;background:var(--gold);color:var(--gold-ink)}
.sect{display:flex;align-items:center;justify-content:space-between;padding:0 .65rem .3rem}
.sect button{color:var(--ink3);font-size:1.1rem;line-height:1;width:24px;height:24px;border-radius:6px}
.sect button:hover{background:var(--panel);color:var(--ink)}
.chans{flex:1;min-height:0;overflow-y:auto;display:flex;flex-direction:column;gap:1px;margin:0 -.2rem;padding:0 .2rem}
.chan{display:flex;align-items:center;gap:.5rem;min-height:32px;padding:.3rem .65rem;border-radius:8px;font-size:.9rem;color:var(--ink3);text-align:left;width:100%;transition:background .12s,color .12s}
.chan:hover{background:var(--panel);color:var(--ink2)}
.chan .nm{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.chan.unread{color:var(--ink);font-weight:600}
.chan[aria-current=true]{background:var(--raise);color:var(--ink)}
.chan .dot{width:8px;height:8px;border-radius:99px;background:var(--gold);flex:none;box-shadow:0 0 0 3px var(--gold-glow)}
.chan .ag{font-size:.66rem;color:var(--blue);flex:none}
.chan.kid{padding-left:1.9rem;font-size:.87rem}
.chan.proj{font-weight:600;color:var(--ink)}
.chan .stk{flex:none;color:var(--gold-text)}
.grp{display:flex;align-items:center;margin-left:-.35rem}
.grp .chan{flex:1;min-width:0;padding-left:.2rem}
.fold{width:20px;height:28px;display:grid;place-items:center;color:var(--ink4);flex:none}
.fold svg{transition:transform .15s}
.fold[aria-expanded=true] svg{transform:rotate(90deg)}
.addkid{text-align:left;padding:.2rem .65rem .35rem 1.9rem;font-size:.78rem;color:var(--ink4)}
.addkid:hover{color:var(--gold-text)}
.chan[draggable]{cursor:grab}
.drop-on{background:var(--gold-glow)!important;box-shadow:inset 0 0 0 2px var(--gold);border-radius:8px}
body.dragging .chan.proj{box-shadow:inset 0 0 0 1px var(--gold-dim)}
body.dragging .sect{box-shadow:inset 0 0 0 1px var(--line2);border-radius:8px}
.chans .sep{height:1px;background:var(--line);margin:.45rem .65rem}
.crumb{display:inline-flex;align-items:center;gap:.3rem;color:var(--ink3);font-weight:500;cursor:pointer}
.crumb:hover{color:var(--ink)}
.head h1 .stk{color:var(--gold-text);align-self:center}
.head h1 .tag{margin-left:.4rem;align-self:center}
.pill.fa{border-color:var(--blue);color:var(--blue);background:var(--blue-glow);font-weight:600}
.kidlink{color:var(--ink);cursor:pointer}
.kidlink:hover{color:var(--gold-text)}
.panel select{font-size:.84rem;padding:.4rem .55rem}
.walled{padding:.8rem .85rem;border-radius:12px;background:var(--blue-glow);margin:0 -.2rem}
.chips{display:flex;flex-wrap:wrap;gap:.35rem}
.achip{display:inline-flex;align-items:center;gap:.35rem;height:28px;padding:0 .6rem;border-radius:99px;background:var(--panel);border:1px solid var(--line2);font-size:.82rem;font-weight:500}
.achip i{width:8px;height:8px;border-radius:99px;flex:none}
.achip button{color:var(--ink4);font-size:.95rem;line-height:1;margin-left:.1rem}
.achip button:hover{color:var(--red)}
.who{width:auto;max-width:11rem;height:30px;padding:0 .6rem;border-radius:99px;font-size:.78rem;font-weight:600;background:var(--panel);color:var(--ink)}
.seg3{display:flex;border:1px solid var(--line2);border-radius:10px;padding:2px;gap:2px}
.seg3 button{flex:1;height:30px;border-radius:8px;font-size:.78rem;color:var(--ink3);display:inline-flex;align-items:center;justify-content:center;gap:.25rem}
.seg3 button[aria-pressed=true]{background:var(--ink);color:#fff}
.agentcard{display:flex;flex-direction:column;gap:.6rem;padding:.75rem;border:1px solid var(--line2);border-radius:12px;background:var(--panel);text-align:left;width:100%;transition:border-color .14s}
.agentcard:hover{border-color:var(--ink4)}
.agentcard .top{display:flex;align-items:center;gap:.6rem}
.agentcard .who2{display:flex;flex-direction:column;line-height:1.25;min-width:0;flex:1}
.agentcard .who2 b{font-size:.86rem;font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.agentcard .who2 span{font-size:.76rem;color:var(--ink3)}
.agentcard .live{width:8px;height:8px;border-radius:99px;background:var(--ink4);flex:none}
.agentcard .live.on{background:var(--green);box-shadow:0 0 0 4px rgba(31,158,104,.14)}
.agentcard .live.bad{background:var(--gold);box-shadow:0 0 0 4px var(--gold-glow)}
.agentcard .spend{display:flex;justify-content:space-between;font:.72rem var(--mono);color:var(--ink3)}
.agentcard .spend b{font-weight:500;color:var(--ink2)}
.av{width:30px;height:30px;border-radius:99px;display:grid;place-items:center;font-size:.76rem;font-weight:700;flex:none;background:var(--panel2);color:var(--ink2);border:1px solid var(--line2)}
.av.you{background:var(--gold);color:var(--gold-ink);border-color:transparent}
.av.bot{border-radius:9px;background:var(--blue);color:#fff;border-color:transparent}
.av.peer{background:#E9E3FF;color:var(--gold-ink);border-color:transparent}
.me{display:flex;align-items:center;gap:.6rem;padding:0 .4rem}
.me .av{width:26px;height:26px;font-size:.7rem}
.me b{flex:1;min-width:0;font-size:.84rem;font-weight:500;color:var(--ink2);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.icon{width:32px;height:32px;border-radius:8px;border:1px solid var(--line2);color:var(--ink3);display:grid;place-items:center;transition:all .14s;flex:none}
.icon:hover{color:var(--ink);border-color:var(--ink4)}

.main{display:flex;flex-direction:column;min-width:0;min-height:0;position:relative}
.view{display:flex;flex-direction:column;min-height:0;flex:1}
.scroll{flex:1;overflow-y:auto;min-height:0}
.menu{display:none}
.scrim{display:none}

/* ---- buttons ---- */
.btn{border:1px solid var(--line2);border-radius:9px;padding:.44rem .85rem;font-size:.85rem;font-weight:500;color:var(--ink2);transition:all .14s;display:inline-flex;align-items:center;gap:.4rem;white-space:nowrap;background:var(--panel2)}
.btn:hover{color:var(--ink);border-color:var(--ink4)}
.btn.solid{background:var(--ink);border-color:var(--ink);color:#fff;font-weight:600}
.btn.solid:hover{background:#000;border-color:#000}
.btn.ghost{background:none}
.btn.danger:hover{color:var(--red);border-color:var(--red)}
.btn.sm{padding:.3rem .65rem;font-size:.8rem;border-radius:8px}
.btn[disabled]{opacity:.4;pointer-events:none}
.pill{display:inline-flex;align-items:center;gap:.4rem;height:30px;border:1px solid var(--line2);border-radius:99px;padding:0 .75rem;font-size:.8rem;color:var(--ink2);transition:all .14s;white-space:nowrap}
.pill:hover{border-color:var(--ink4);color:var(--ink)}
.pill i{width:7px;height:7px;border-radius:50%;background:var(--ink4)}
.pill .short{display:none}
.pill.on i{background:var(--green);box-shadow:0 0 8px rgba(31,158,104,.6)}
.pill.off{border-style:dashed;color:var(--ink3)}
.pill.off:hover{border-style:solid;border-color:var(--gold);color:var(--ink)}
.pill.wait{border-color:var(--gold-dim);color:var(--gold-text);background:var(--gold-glow)}
.pill.wait i{background:var(--gold);box-shadow:0 0 8px var(--gold)}
.tag{display:inline-flex;align-items:center;height:19px;padding:0 .45rem;border-radius:99px;font-size:.68rem;font-weight:500;background:var(--raise);color:var(--ink3)}
.tag.ai{background:var(--blue-glow);color:var(--blue)}
.tag.call{background:var(--gold-glow);color:var(--gold-text)}
.pip{display:inline-flex;padding:.06rem .4rem;border-radius:99px;font-size:.66rem;font-weight:500;background:rgba(31,158,104,.1);color:var(--green)}
.pip.warn,.pip.wait{background:var(--gold-glow);color:var(--gold-text)}
.pip.auto{background:var(--blue-glow);color:var(--blue)}

/* ---- Today ---- */
.today{max-width:78rem;width:100%;margin:0 auto;padding:3.4rem 4rem 4rem}
.hello{font-family:var(--display);font-weight:800;font-size:3.4rem;line-height:1;letter-spacing:-.04em;margin:.55rem 0 .5rem;text-wrap:balance}
.hello em{font-style:normal;background:linear-gradient(100deg,#E08A00,#FF7A45 55%,#4C6FF5);-webkit-background-clip:text;background-clip:text;color:transparent}
.lede{font-size:1.02rem;color:var(--ink2);max-width:44rem;line-height:1.55}
.tgrid{display:grid;grid-template-columns:minmax(0,1fr) 22rem;gap:1.8rem;margin-top:2.4rem;align-items:start}
.col{display:flex;flex-direction:column;gap:.85rem;min-width:0}
.col .kick{margin-top:.6rem}
.col .kick:first-child{margin-top:0}
.card{background:var(--panel);border:1px solid var(--line2);border-radius:16px}
.dcard{padding:1.2rem 1.35rem;display:flex;flex-direction:column;gap:.85rem;border-color:#F2D29B;background:linear-gradient(#FFF7E8,var(--panel));animation:rise .3s both}
.dcard .where{display:flex;align-items:center;gap:.5rem;font-size:.82rem;color:var(--ink2)}
.dcard .where .tag{margin-left:auto}
.dcard .where a{color:inherit;text-decoration:none;cursor:pointer}
.dcard .where a:hover{color:var(--ink)}
.dcard .q{font-size:1.05rem;line-height:1.55;white-space:pre-wrap}
.opts{display:flex;flex-wrap:wrap;gap:.5rem}
.opts .btn{height:36px}
.free{display:flex;gap:.5rem}
.free input{height:36px;padding:0 .7rem}
.runs{display:flex;flex-direction:column}
.runrow{display:flex;gap:1rem;padding:1rem 1.25rem;border-bottom:1px solid var(--line);text-align:left;width:100%;transition:background .12s}
.runrow:last-child{border-bottom:none}
.runrow:hover{background:var(--panel2)}
.runrow .tm{width:3.4rem;flex:none;font:.76rem var(--mono);color:var(--ink3);padding-top:.15rem}
.runrow .bd{display:flex;flex-direction:column;gap:.25rem;min-width:0}
.runrow .ttl{font-size:.9rem;color:var(--ink)}
.runrow .ttl span{color:var(--ink3)}
.runrow .tx{font-size:.88rem;color:var(--ink2);line-height:1.5;display:-webkit-box;-webkit-line-clamp:3;-webkit-box-orient:vertical;overflow:hidden;white-space:pre-wrap}
.runrow .mt{font:.72rem var(--mono);color:var(--ink4)}
.runrow.bad .mt{color:var(--red)}
.up{padding:.35rem 0}
.uprow{display:flex;align-items:center;gap:.8rem;padding:.7rem 1.1rem;text-align:left;width:100%}
.uprow:hover{background:var(--panel2)}
.uprow .tm{width:3rem;font:.76rem var(--mono);color:var(--ink3);flex:none}
.uprow .bd{display:flex;flex-direction:column;min-width:0}
.uprow b{font-size:.88rem;font-weight:500;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.uprow span{font-size:.76rem;color:var(--ink3)}
.link{display:block;padding:.55rem 1.1rem .7rem;font-size:.82rem;color:var(--gold-text);text-align:left}
.link:hover{color:#FFC24F}
.side-card{padding:1.1rem;display:flex;flex-direction:column;gap:.6rem}
.side-card b{font-size:.95rem;font-weight:600}
.side-card p{font-size:.86rem;color:var(--ink2);line-height:1.5}
.side-card .btn{align-self:flex-start}
.quiet{padding:1.1rem 1.25rem;color:var(--ink3);font-size:.9rem;line-height:1.55}
.starters{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:.75rem}
.starter{padding:1rem 1.1rem;text-align:left;display:flex;flex-direction:column;gap:.35rem;transition:border-color .14s,transform .14s}
.starter:hover{border-color:var(--ink4);transform:translateY(-1px)}
.starter b{font-size:.92rem;font-weight:600}
.starter span{font-size:.82rem;color:var(--ink3);line-height:1.45}

/* ---- a channel ---- */
.head{display:flex;align-items:center;gap:.75rem;height:60px;flex:none;padding:0 1.6rem;border-bottom:1px solid var(--line);background:rgba(255,250,243,.86);backdrop-filter:saturate(1.5) blur(14px);position:relative;z-index:5}
.head h1{font-size:1.02rem;font-weight:600;letter-spacing:-.01em;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;display:flex;align-items:baseline;gap:.35rem}
.tabs{display:flex;gap:2px;margin-left:.6rem}
.tabs button{font-size:.8rem;padding:.3rem .65rem;border-radius:7px;color:var(--ink3)}
.tabs button:hover{color:var(--ink)}
.tabs button[aria-pressed=true]{background:var(--raise);color:var(--ink)}
.head .spacer{flex:1}
.avs{display:flex;align-items:center}
.avs .av{width:26px;height:26px;font-size:.66rem;border:2px solid var(--bg);margin-left:-6px}
.avs .av:first-child{margin-left:0}
.avs .more{font:.72rem var(--mono);color:var(--ink3);margin-left:.4rem}
.body2{flex:1;display:flex;min-height:0}
.convo{flex:1;display:flex;flex-direction:column;min-width:0;min-height:0}
.feed{flex:1;overflow-y:auto;padding:1.8rem 2rem .8rem;scroll-behavior:smooth}
.stream{max-width:var(--measure);margin:0 auto;display:flex;flex-direction:column;gap:1.35rem}
.entry{display:flex;gap:.85rem;animation:rise .3s cubic-bezier(.2,.8,.3,1) both}
.entry .av{width:34px;height:34px;font-size:.8rem;margin-top:.1rem}
.entry .av.bot{border-radius:10px}
.entry .c{display:flex;flex-direction:column;gap:.3rem;min-width:0;flex:1}
.entry .meta{display:flex;align-items:center;flex-wrap:wrap;gap:.5rem;font-size:.75rem;color:var(--ink4);font-family:var(--mono)}
.entry .meta .auth{font-family:var(--sans);font-size:.88rem;font-weight:600;color:var(--ink)}
.entry .body{font-size:.96rem;line-height:1.65;white-space:pre-wrap;overflow-wrap:anywhere;color:var(--ink)}
.entry .body b{color:var(--ink)}
.entry.q .body{color:var(--ink)}
.entry.summary .body{padding:.8rem 1rem;border:1px solid var(--gold-dim);border-radius:12px;background:var(--gold-glow)}
.entry.cont{margin-top:-.8rem}
.entry.cont .av{visibility:hidden;height:0}
@keyframes rise{from{opacity:0;transform:translateY(6px)}to{opacity:1;transform:none}}
.ref{color:var(--gold-text);cursor:pointer;border-bottom:1px dotted var(--gold-dim)}
.ref.dead{color:var(--ink3);cursor:default;border-bottom-style:dashed}
.mention{color:var(--blue);font-weight:500}

.runcard{border:1px solid var(--line2);background:var(--panel);border-radius:12px;max-width:40rem;overflow:hidden}
.runcard summary{list-style:none;display:flex;align-items:center;gap:.55rem;padding:.6rem .85rem;font-size:.82rem;color:var(--ink2);cursor:pointer}
.runcard summary::-webkit-details-marker{display:none}
details.runcard summary:before,details.runcard[open] summary:before{content:none}
.runcard summary .st{width:16px;height:16px;border-radius:99px;display:grid;place-items:center;flex:none;background:rgba(31,158,104,.14);color:var(--green)}
.runcard.bad summary .st{background:rgba(214,69,58,.14);color:var(--red)}
.runcard.wait summary .st{background:var(--gold-glow);color:var(--gold-text)}
.runcard summary .mt{margin-left:auto;font:.72rem var(--mono);color:var(--ink4)}
.runcard summary .chev{color:var(--ink4);transition:transform .15s}
.runcard[open] summary .chev{transform:rotate(90deg)}
.runcard .steps{display:flex;flex-direction:column;gap:.3rem;padding:.1rem .85rem .75rem 2.3rem;font-size:.8rem;color:var(--ink3)}
.runcard .steps span{overflow-wrap:anywhere}
.runcard .steps .er{color:var(--red)}
.runcard .steps .fine{font:.72rem var(--mono);color:var(--ink4);margin-top:.2rem}
.runcard{margin-top:0}

.decision{border:1px solid #F2D29B;background:linear-gradient(#FFF7E8,var(--panel));border-radius:14px;padding:1rem 1.15rem;display:flex;flex-direction:column;gap:.75rem;max-width:44rem}
.decision .ask{font-size:.98rem;line-height:1.55;white-space:pre-wrap}
.connect{border:1px solid var(--line2);background:var(--panel);border-radius:16px;padding:1rem 1.1rem;display:flex;flex-direction:column;gap:.8rem;max-width:34rem;box-shadow:0 12px 30px -20px rgba(80,55,20,.3)}
.cc-top{display:flex;align-items:center;gap:.75rem}
.cc-logo{width:40px;height:40px;border-radius:12px;background:var(--blue-glow);color:var(--blue);display:grid;place-items:center;font-family:var(--display);font-weight:800;font-size:1.15rem;flex:none}
.cc-t{display:flex;flex-direction:column;line-height:1.25;min-width:0}
.cc-t b{font-size:1rem;font-weight:700}
.cc-t span{font:.76rem var(--mono);color:var(--ink3);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.cc-why{font-size:.95rem;line-height:1.5;color:var(--ink)}
.cc-act{display:flex;gap:.5rem;flex-wrap:wrap}
.cc-act .btn{height:38px;border-radius:99px;padding:0 1.1rem}
.cc-key{display:flex;gap:.5rem}
.cc-status{font-size:.85rem;color:var(--ink2);display:flex;align-items:center;flex-wrap:wrap;gap:.3rem}
.cc-status:empty{display:none}
.cc-status.bad{color:var(--red)}
.cc-fine{font-size:.76rem;color:var(--ink3);line-height:1.45}
.cc-done{font-size:.86rem;font-weight:600;color:var(--ink3)}
.cc-done.ok{color:var(--green)}
.cc-done.ok:before{content:"✓ "}
.dcard .connect{box-shadow:none;border:none;padding:0;background:none}
.decision .done{font:.78rem/1.5 var(--mono);color:var(--ink3)}
.nudge{display:flex;align-items:center;gap:.8rem;padding:.7rem .95rem;border:1px solid var(--gold-dim);border-radius:12px;background:var(--gold-glow);font-size:.86rem;color:var(--ink2);animation:rise .3s both}
.nudge span{flex:1}

.thinking{display:flex;gap:.85rem;align-items:center;font-size:.86rem;color:var(--blue)}
.thinking .av{width:34px;height:34px;border-radius:10px;animation:breathe 1.4s ease-in-out infinite}
.thinking .lbl{display:flex;align-items:center;gap:.5rem}
.thinking .lbl:before{content:"";width:10px;height:10px;border-radius:50%;border:2px solid currentColor;border-right-color:transparent;animation:spin .7s linear infinite;flex:none}
@keyframes spin{to{transform:rotate(360deg)}}
@keyframes breathe{0%,100%{opacity:.55}50%{opacity:1}}

.composer{padding:.6rem 2rem 1.3rem;flex:none}
.cbox{max-width:var(--measure);margin:0 auto;border:2px solid var(--ink);background:var(--panel);border-radius:22px;box-shadow:0 5px 0 var(--ink);transition:transform .15s,box-shadow .15s}
.cbox:focus-within{transform:translateY(-1px);box-shadow:0 7px 0 var(--ink)}
.cbox textarea:focus-visible{outline:none}
.cbox textarea{background:none;border:none;resize:none;padding:.95rem 1.1rem .35rem;font-size:.98rem;line-height:1.55;min-height:3rem;max-height:14rem;border-radius:0}
.crow{display:flex;align-items:center;gap:.45rem;padding:.35rem .55rem .6rem .8rem}
.seg{display:inline-flex;border:1px solid var(--line2);border-radius:99px;padding:2px;gap:2px}
.seg button{height:26px;padding:0 .7rem;border-radius:99px;font-size:.78rem;color:var(--ink3)}
.seg button[aria-pressed=true]{background:var(--raise);color:var(--ink)}
.chip{height:30px;padding:0 .7rem;border-radius:99px;border:1px solid var(--line2);font-size:.78rem;color:var(--ink3);display:inline-flex;align-items:center;gap:.35rem}
.chip:hover{color:var(--ink);border-color:var(--ink4)}
.crow .model{margin-left:auto;font:.72rem var(--mono);color:var(--ink4);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;max-width:14rem}
.send{width:38px;height:38px;border-radius:99px;background:var(--gold);display:grid;place-items:center;flex:none;color:var(--gold-ink);transition:background .14s,opacity .14s}
.send:hover{background:#FFC24F}
.send[disabled]{opacity:.35;pointer-events:none}
.note{font:.75rem/1.4 var(--mono);color:var(--ink4);padding:.45rem 1.1rem 0;max-width:var(--measure);margin:0 auto}
.note:empty{display:none}
.note.warn{color:var(--gold-text)}.note.bad{color:var(--red)}

.panel{width:18.5rem;flex:none;border-left:1px solid var(--line);background:var(--side);overflow-y:auto;padding:1.4rem 1.25rem;display:flex;flex-direction:column;gap:1.4rem}
.panel .blk{display:flex;flex-direction:column;gap:.55rem}
.panel .ln{display:flex;align-items:center;justify-content:space-between;gap:.6rem;font-size:.86rem}
.panel .ln span:first-child{min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.panel .sub{font-size:.8rem;color:var(--ink3);line-height:1.5}
.panel .edit{font-size:.8rem;color:var(--gold-text);text-align:left}
.panel .edit:hover{color:#FFC24F}
.switch{width:36px;height:21px;border-radius:99px;background:var(--line2);position:relative;flex:none;transition:background .16s}
.switch:after{content:"";position:absolute;left:2px;top:2px;width:17px;height:17px;border-radius:99px;background:var(--ink3);transition:transform .16s,background .16s}
.switch[aria-checked=true]{background:var(--gold)}
.switch[aria-checked=true]:after{transform:translateX(15px);background:var(--gold-ink)}
@media(max-width:1250px){.panel{display:none}}

.void{max-width:32rem;margin:14vh auto;text-align:center;animation:rise .4s both;display:flex;flex-direction:column;align-items:center;gap:.8rem}
.void h2{font-family:var(--display);font-weight:800;font-size:2.2rem;line-height:1.05;letter-spacing:-.035em}
.void p{color:var(--ink2);font-size:.95rem;line-height:1.6}

/* ---- Scheduled ---- */
.sched{max-width:64rem;width:100%;margin:0 auto;padding:3.4rem 4rem 4rem;display:flex;flex-direction:column;gap:1.6rem}
.sched .hello{font-size:2.9rem}
.addsched{display:grid;grid-template-columns:minmax(0,1fr) 13rem 7.5rem auto;gap:.5rem;padding:.6rem;align-items:center}
.addsched input,.addsched select{height:40px}
.stable .hd,.srow{display:grid;grid-template-columns:minmax(0,1fr) 10rem 3rem 2rem;gap:1rem;align-items:center;padding:.9rem 1.3rem}
.stable .hd{border-bottom:1px solid var(--line);padding-top:.7rem;padding-bottom:.7rem}
.srow{border-bottom:1px solid var(--line)}
.srow:last-child{border-bottom:none}
.srow.paused .what{opacity:.5}
.srow .what{display:flex;flex-direction:column;gap:.15rem;min-width:0}
.srow .what b{font-size:.95rem;font-weight:500}
.srow .what span{font-size:.82rem;color:var(--ink3);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.srow .what a{color:var(--ink2);cursor:pointer}
.srow .when{font-size:.88rem;color:var(--ink2)}
.srow .x{color:var(--ink4);font-size:1.1rem;width:28px;height:28px;border-radius:7px}
.srow .x:hover{color:var(--red);background:var(--panel2)}
.trio{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:.75rem}
.trio .card{padding:1rem 1.1rem;display:flex;flex-direction:column;gap:.3rem}
.trio b{font-size:.9rem}
.trio span{font-size:.82rem;color:var(--ink3);line-height:1.5}

/* ---- jump to ---- */
.pal{position:fixed;inset:0;z-index:70;background:rgba(60,45,25,.28);backdrop-filter:blur(4px);display:flex;justify-content:center;align-items:flex-start;padding:12vh 1rem 1rem;animation:fade .14s both}
.pal .box{width:min(560px,100%);background:var(--panel);border:1px solid var(--line2);border-radius:16px;box-shadow:0 40px 90px -30px rgba(80,55,20,.28);overflow:hidden;animation:pop .18s both}
.pal input:focus-visible{outline:none}
.pal input{border:none;border-bottom:1px solid var(--line);border-radius:0;padding:1rem 1.15rem;font-size:1rem;background:none}
.pal .res{max-height:50vh;overflow-y:auto;padding:.4rem}
.pal .it{display:flex;align-items:center;gap:.7rem;width:100%;text-align:left;padding:.6rem .75rem;border-radius:9px;font-size:.92rem;color:var(--ink2)}
.pal .it[aria-selected=true]{background:var(--raise);color:var(--ink)}
.pal .it small{margin-left:auto;font:.7rem var(--mono);color:var(--ink4)}

/* ---- sheets ---- */
.veil{position:fixed;inset:0;background:rgba(60,45,25,.32);backdrop-filter:blur(6px);display:grid;place-items:center;padding:1.5rem;z-index:50;animation:fade .18s both}
@keyframes fade{from{opacity:0}to{opacity:1}}
.sheet{width:min(640px,100%);max-height:88vh;overflow:auto;background:var(--panel);border:1px solid var(--line2);border-radius:20px;box-shadow:0 40px 90px -30px rgba(80,55,20,.28);animation:pop .22s cubic-bezier(.2,.8,.3,1) both}
@keyframes pop{from{opacity:0;transform:translateY(12px) scale(.985)}to{opacity:1;transform:none}}
.sheet header{padding:1.4rem 1.6rem .5rem}
.sheet h2{font-family:var(--display);font-size:1.7rem;font-weight:800;letter-spacing:-.03em;line-height:1.1;margin-bottom:.4rem}
.sheet header p{color:var(--ink2);font-size:.88rem;line-height:1.6}
.sheet section{padding:.5rem 1.6rem 1.2rem}
.sheet footer{display:flex;align-items:center;gap:.7rem;padding:.9rem 1.6rem 1.3rem;border-top:1px solid var(--line)}
.spacer{flex:1}
label.f{display:block;font-size:.8rem;font-weight:500;color:var(--ink2);margin:1rem 0 .35rem}
.hint{font-size:.8rem;color:var(--ink3);line-height:1.5;margin-top:.3rem}
.choices{display:grid;grid-template-columns:1fr 1fr;gap:.6rem}
.choice{text-align:left;border:1px solid var(--line2);border-radius:14px;padding:.85rem .95rem;transition:all .14s}
.choice:hover{background:var(--panel2)}
.choice[aria-pressed=true]{border-color:var(--gold);background:var(--gold-glow)}
.choice b{display:block;font-size:.9rem;font-weight:600;margin-bottom:.15rem}
.choice span{font-size:.8rem;color:var(--ink3);line-height:1.5}
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
.runcard details,.runcard{margin-top:0}
.runcard{border-top:1px solid var(--line2);padding-top:0}
.status{font:.75rem/1.5 var(--mono);padding:.5rem .8rem;border-radius:10px;margin-top:.7rem}
.status.ok{background:rgba(31,158,104,.08);color:var(--green)}.status.bad{background:rgba(214,69,58,.08);color:var(--red)}
select{background:var(--bg);border:1px solid var(--line2);border-radius:10px;padding:.55rem .7rem;color:var(--ink);width:100%}
.grid2{display:grid;grid-template-columns:1fr 1fr;gap:.6rem}
.check{display:flex;align-items:center;gap:.5rem;font-size:.86rem;color:var(--ink2);padding:.3rem 0}
.check input{width:auto}
.rh{display:grid;grid-template-columns:7.5rem 1fr auto auto;gap:.5rem;align-items:center;margin-top:.45rem}
.rh input[type=time]{font-family:var(--mono);font-size:.84rem}
.rh .x{color:var(--ink4);font-size:1rem;padding:0 .4rem}
.rh .x:hover{color:var(--red)}
.presets{display:flex;gap:.45rem;flex-wrap:wrap;margin-top:.55rem}
.conn{border:1px solid var(--line);border-radius:12px;padding:.8rem .9rem;margin-top:.5rem;background:var(--bg)}
.conn .top{display:flex;align-items:center;gap:.6rem}
.conn .top b{flex:1;font-size:.9rem;font-weight:560}
.conn .addr{font:.72rem var(--mono);color:var(--ink4);margin-top:.15rem;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.addconn{display:flex;gap:.5rem;margin-top:.5rem}
.addconn input{flex:1;min-width:0}
.conn .sum{font-size:.78rem;color:var(--ink3);margin-top:.35rem}
.modes{display:inline-flex;border:1px solid var(--line2);border-radius:9px;overflow:hidden;margin-top:.6rem}
.modes button{background:none;border:0;border-right:1px solid var(--line2);padding:.3rem .7rem;font-size:.78rem;color:var(--ink3);cursor:pointer}
.modes button:last-child{border-right:0}
.modes button[aria-pressed=true]{background:var(--gold-glow);color:var(--gold-text);font-weight:560}
.conn .each{margin-top:.6rem;border-top:1px solid var(--line);padding-top:.5rem}
.conn .each summary{font-size:.78rem;color:var(--ink3);cursor:pointer}
.trow{display:flex;align-items:flex-start;gap:.6rem;padding:.4rem 0;border-bottom:1px solid var(--line)}
.trow:last-child{border-bottom:0}
.trow .tn{flex:1;min-width:0}
.trow .tn b{font:.78rem/1.3 var(--mono);font-weight:500;color:var(--ink2);display:block}
.trow .tn span{font-size:.74rem;color:var(--ink4);display:block;margin-top:.1rem}
.trow .tw{font-size:.68rem;color:var(--gold-text);white-space:nowrap;padding-top:.1rem}
.conn .probe{font-size:.78rem;color:var(--ink4);margin-top:.5rem;min-height:1.1rem}
.conn .probe.bad{color:var(--red)}
.conn .probe.ok{color:var(--green)}

/* ---- phones ---- */
@media(max-width:1100px){
  .tgrid{grid-template-columns:1fr}
  .today,.sched{padding:2.4rem 2rem 3rem}
  .starters,.trio{grid-template-columns:1fr}
}
@media(max-width:860px){
  .shell{grid-template-columns:1fr}
  body{font-size:16px}
  input,textarea,select{font-size:16px}
  .rail{position:fixed;inset:0 auto 0 0;width:min(84vw,300px);z-index:40;transform:translateX(-100%);transition:transform .22s cubic-bezier(.2,.8,.3,1);box-shadow:0 0 60px rgba(80,55,20,.22);padding-bottom:calc(.75rem + env(safe-area-inset-bottom))}
  .shell.open .rail{transform:none}
  .scrim{display:block;position:fixed;inset:0;z-index:35;background:rgba(60,45,25,.28);opacity:0;pointer-events:none;transition:opacity .22s}
  .shell.open .scrim{opacity:1;pointer-events:auto}
  .menu{display:inline-grid;place-items:center;width:40px;height:40px;flex:none;border:1px solid var(--line2);border-radius:11px;color:var(--ink2)}
  .mbar{display:flex!important}
  .head{padding:0 .8rem;gap:.5rem;height:56px;position:sticky;top:0}
  .tabs,.avs{display:none}
  .head .pill .full{display:none}
  .head .pill .short{display:inline}
  .head .pill{padding:0 .6rem}
  .today,.sched{padding:1.2rem 1.1rem 2.5rem}
  .hello{font-size:2.4rem}
  .sched .hello{font-size:2.1rem}
  .feed{padding:1.1rem .9rem .4rem;overscroll-behavior:contain}
  .composer{padding:.5rem .7rem calc(.7rem + env(safe-area-inset-bottom))}
  .cbox{border-radius:16px}
  .crow .model{display:none}
  .chip .lbl{display:none}
  .opts{flex-direction:column}
  .opts .btn{height:44px;justify-content:center}
  .free{flex-direction:column}
  .free input,.free .btn{height:44px}
  .free .btn{justify-content:center}
  .addsched{grid-template-columns:1fr}
  .stable .hd{display:none}
  .srow{grid-template-columns:minmax(0,1fr) auto auto;row-gap:.3rem}
  .srow .when{grid-column:1;grid-row:2;font-size:.8rem;color:var(--ink3)}
  .sheet{width:100%;max-height:92dvh;border-radius:18px 18px 0 0}
  .veil{padding:0;place-items:end center}
  .sheet header{padding:1.2rem 1.15rem .4rem}
  .sheet section{padding:.4rem 1.15rem 1rem}
  .sheet footer{padding:.8rem 1.15rem calc(1rem + env(safe-area-inset-bottom))}
  .grid2,.choices{grid-template-columns:1fr}
  .rh{grid-template-columns:6rem 1fr auto auto}
  .linkbox{flex-direction:column;align-items:stretch}
  .cmd{padding-right:1rem;font-size:.78rem}
  .cmd button{position:static;display:block;margin-top:.5rem}
  .pal{padding-top:8vh}
}
.mbar{display:none;align-items:center;gap:.7rem;padding:.8rem 1.1rem 0}
.mbar b{font-weight:600}

/* ---- someone else's view of a shared channel, and plain notices ---- */
.guest{max-width:52rem;margin:0 auto;padding:3rem 2rem 4rem}
.guest .crest{display:flex;align-items:center;gap:.6rem;margin-bottom:2rem;font-size:.86rem;color:var(--ink3)}
.guest h1{font-family:var(--display);font-size:2.6rem;font-weight:800;letter-spacing:-.035em;line-height:1.08;margin-bottom:.6rem}
.guest .scope{display:inline-flex;align-items:center;gap:.4rem;margin-bottom:2rem;padding:.3rem .65rem;border-radius:99px;border:1px solid var(--line2);font:.75rem/1 var(--mono);color:var(--ink3)}
.guest .scope i{width:6px;height:6px;border-radius:50%;background:var(--gold)}
.guest .entry{display:block}
.guest .entry .meta{margin-bottom:.35rem}
.guest .foot{margin-top:3rem;padding-top:1.4rem;border-top:1px solid var(--line);font-size:.82rem;color:var(--ink3);line-height:1.7}
.notice{max-width:30rem;margin:18vh auto;padding:0 1.5rem;text-align:center}
.notice h1{font-family:var(--display);font-size:2rem;font-weight:800;letter-spacing:-.03em;margin-bottom:.5rem}.notice p{color:var(--ink2);line-height:1.65}
`

const appJS = `
"use strict";
var cur=null, me=null, entries=[], sheetEl=null, titles={}, agentInfo=null, busy=false, prices={}, modelList=null;
var view="today", tab="all", mode="ask", todayData=null, threadList=[], team=[], here=[], target="";
var $=function(i){return document.getElementById(i)};
function esc(s){return String(s==null?"":s).replace(/[&<>"]/g,function(c){return {"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;"}[c]})}
function when(ts){var d=new Date(ts);if(isNaN(d))return ts||"";var n=new Date();
  return d.toDateString()===n.toDateString()?d.toLocaleTimeString([],{hour:"numeric",minute:"2-digit"})
  :d.toLocaleDateString([],{month:"short",day:"numeric"})+" "+d.toLocaleTimeString([],{hour:"numeric",minute:"2-digit"})}
function clock(ts){var d=new Date(ts);if(isNaN(d))return "";var n=new Date();
  if(d.toDateString()===n.toDateString())return d.toLocaleTimeString([],{hour:"2-digit",minute:"2-digit",hour12:false});
  return d.toLocaleDateString([],{weekday:"short"})}
function ago(ts){if(!ts)return "never";var m=Math.round((Date.now()-new Date(ts))/60000);if(m<1)return "just now";if(m<60)return m+"m ago";var h=Math.round(m/60);if(h<48)return h+"h ago";return Math.round(h/24)+"d ago"}
function api(p,body){var o=body?{method:"POST",headers:{"content-type":"application/json"},body:JSON.stringify(body)}:undefined;
  return fetch(p,o).then(function(r){return r.text().then(function(t){var d;try{d=JSON.parse(t)}catch(e){d={error:t||("HTTP "+r.status)}}
    if(!r.ok&&!d.error)d.error="HTTP "+r.status;return d})})}
function copy(t){if(navigator.clipboard)navigator.clipboard.writeText(t)}
function initials(n){return (n&&n!=="you"&&n!=="your agent")?n.split(/\s+/).map(function(w){return w[0]}).join("").slice(0,2).toUpperCase():(n==="your agent"?"A":"Y")}
function tid(){return encodeURIComponent(cur)}
function note(t,cls){$("note").textContent=t||"";$("note").className="note"+(cls?" "+cls:"")}
function each(sel,fn,root){Array.prototype.forEach.call((root||document).querySelectorAll(sel),fn)}
function agentName(){var n=agentInfo&&agentInfo.name;return (!n||n==="your agent")?"your agent":n}
function AgentName(){var n=agentName();return n==="your agent"?"Your agent":n}
function threadOf(id){return threadList.filter(function(x){return x.id===id})[0]||{}}
function seen(id,ts){try{var s=JSON.parse(localStorage.getItem("lamdis.seen")||"{}");if(ts===undefined)return s[id]||"";s[id]=ts;localStorage.setItem("lamdis.seen",JSON.stringify(s))}catch(e){return ""}}
var ICON={
  check:'<svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3.2" aria-hidden="true"><path d="M5 12l5 5L20 7"/></svg>',
  x:'<svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3.2" aria-hidden="true"><path d="M6 6l12 12M18 6L6 18"/></svg>',
  wait:'<svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" aria-hidden="true"><path d="M12 7v5l3 2"/></svg>',
  chev:'<svg class="chev" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" aria-hidden="true"><path d="M9 6l6 6-6 6"/></svg>',
  stack:'<svg class="stk" width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round" aria-hidden="true"><path d="M12 3l9 5-9 5-9-5z"/><path d="M3 13l9 5 9-5"/></svg>',
  bolt:'<svg width="13" height="13" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M13 2L4 14h7l-1 8 9-12h-7z"/></svg>',
  clock:'<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/></svg>'
};

/* ---- who and what ---- */
function loadMe(){return api("/app/api/me").then(function(d){me=d;$("me-name").textContent=d.name;$("me-av").textContent=initials(d.name);
  if(!d.can_ask){mode="note";drawMode();note(AgentName()+" is off until a model key is set. Open settings for how.","warn")}})}
function loadAgent(){return api("/app/api/agent").then(function(d){agentInfo=d;drawAgentCard();return d})}
function loadModels(){if(modelList)return Promise.resolve(modelList);return api("/app/api/models").then(function(d){modelList=d.models||[];modelList.forEach(function(m){prices[m.id]={i:m.in_per_m,o:m.out_per_m}});return modelList})}
function costOf(d){var p=prices[d.model];if(!p||!d.tokens)return 0;return ((d.tokens.prompt||0)*p.i+(d.tokens.completion||0)*p.o)/1e6}
function money(c){return c<0.0005?"<$0.001":"$"+c.toFixed(3)}
function cost(d){var c=costOf(d);return c?money(c):""}
function drawAgentCard(){var a=agentInfo||{},st=a.status||{};var n=AgentName();
  $("ag-name").textContent=n;$("ag-av").textContent=initials(n==="Your agent"?"A":n);
  var live=$("ag-live");var problem=a.problem||(a.revoked?"revoked":"");
  live.className="live"+(problem?" bad":((todayData&&todayData.watching.length)||(todayData&&todayData.schedules.some(function(s){return !s.paused}))?" on":""));
  var fa=((a.reach&&a.reach.autonomy)==="auto");
  $("ag-state").textContent=problem?"needs setting up":(fa?"full auto":(live.className.indexOf("on")>0?"working on its own":"answers when asked"));
  var runs=todayRunsToday(),spent=0;runs.forEach(function(r){spent+=costOf(r.data||{})});
  $("ag-spend").innerHTML='<span>today</span><b>'+runs.length+(runs.length===1?" run":" runs")+(spent?" · "+money(spent):"")+'</b>';
  var m=(a.reach&&a.reach.model)||a.model||"";$("model").textContent=m}
function todayRunsToday(){if(!todayData)return [];var d=new Date().toDateString();return todayData.runs.filter(function(r){return new Date(r.ts).toDateString()===d}).map(function(r){return {data:parseData(r.data)}})}
function parseData(d){if(!d)return {};if(typeof d==="string"){try{return JSON.parse(d)}catch(x){return {}}}return d}

/* ---- the rail ---- */
function threads(){return api("/app/api/threads").then(function(d){threadList=d.threads||[];window._threads=threadList;
  titles={};threadList.forEach(function(t){titles[t.title.toLowerCase()]=t.id});
  var el=$("threads");
  function row(t,kid){var unread=t.last&&t.id!==cur&&seen(t.id)&&t.last>seen(t.id);
    var right=t.waiting?'<span class="dot" title="Needs you"></span>':(t.full_auto?'<span class="ag">auto</span>':(t.auto?'<span class="ag">agent</span>':''));
    var mark=t.is_project?ICON.stack:'<span class="hash">#</span>';
    return '<button class="chan'+(unread?' unread':'')+(kid?' kid':'')+(t.is_project?' proj':'')+'" data-id="'+esc(t.id)+'"'+(t.is_project?' data-drop="'+esc(t.id)+'"':' draggable="true"')+' aria-current="'+(view==="chan"&&t.id===cur)+'">'+mark+'<span class="nm">'+esc(t.title)+'</span>'+right+'</button>'}
  var projects=threadList.filter(function(t){return t.is_project}),loose=threadList.filter(function(t){return !t.is_project&&!t.project});
  var h="";
  projects.forEach(function(p){var kids=threadList.filter(function(t){return t.project===p.id});var open=!folded(p.id);
    h+='<div class="grp"><button class="fold" data-fold="'+esc(p.id)+'" aria-label="'+(open?"Collapse":"Expand")+'" aria-expanded="'+open+'">'+ICON.chev+'</button>'+row(p,false)+'</div>';
    if(open)h+=kids.map(function(k){return row(k,true)}).join("")+'<button class="addkid" data-addkid="'+esc(p.id)+'">+ channel</button>'});
  if(projects.length&&loose.length)h+='<div class="sep"></div>';
  h+=loose.map(function(t){return row(t,false)}).join("");
  el.innerHTML=threadList.length?h:'<p class="hint" style="padding:0 .65rem">No channels yet.</p>';
  each(".chan",function(n){n.onclick=function(){go("chan",n.getAttribute("data-id"))}},el);
  each("[draggable]",function(n){n.ondragstart=function(e){e.dataTransfer.setData("text/lamdis-channel",n.getAttribute("data-id"));e.dataTransfer.effectAllowed="move";document.body.classList.add("dragging")};
    n.ondragend=function(){document.body.classList.remove("dragging");each(".drop-on",function(x){x.classList.remove("drop-on")})}},el);
  function target(n,project){n.ondragover=function(e){if(Array.prototype.indexOf.call(e.dataTransfer.types,"text/lamdis-channel")<0)return;e.preventDefault();n.classList.add("drop-on")};
    n.ondragleave=function(){n.classList.remove("drop-on")};
    n.ondrop=function(e){e.preventDefault();n.classList.remove("drop-on");var id=e.dataTransfer.getData("text/lamdis-channel");if(!id)return;var t=threadOf(id);if((t.project||"")===project)return;moveTo(id,project)}}
  each("[data-drop]",function(n){target(n,n.getAttribute("data-drop"))},el);
  var ls=document.querySelector(".sect");if(ls)target(ls,"");
  each("[data-fold]",function(b){b.onclick=function(){var id=b.getAttribute("data-fold");folded(id,!folded(id));threads()}},el);
  each("[data-addkid]",function(b){b.onclick=function(){newThread("",b.getAttribute("data-addkid"))}},el);
  var w=threadList.reduce(function(s,t){return s+(t.waiting||0)},0);
  $("nav-today-n").hidden=!w;$("nav-today-n").textContent=w;
  document.title=(w?"("+w+") ":"")+"Lamdis";
  return threadList})}
function folded(id,v){try{var f=JSON.parse(localStorage.getItem("lamdis.folded")||"{}");if(v===undefined)return !!f[id];f[id]=v;localStorage.setItem("lamdis.folded",JSON.stringify(f))}catch(e){return false}}
function drawNav(){each(".navi[data-view]",function(n){n.setAttribute("aria-current",String(n.getAttribute("data-view")===view))});
  each(".chan",function(n){n.setAttribute("aria-current",String(view==="chan"&&n.getAttribute("data-id")===cur))})}

/* ---- where you are ---- */
function go(v,id,quiet){view=v;if(v==="chan"){cur=id;tab="all"}drawer(false);closePal();
  $("v-today").hidden=v!=="today";$("v-sched").hidden=v!=="sched";$("v-chan").hidden=v!=="chan";
  var h=v==="chan"?"#c/"+encodeURIComponent(id):(v==="sched"?"#scheduled":"#today");
  if(!quiet&&location.hash!==h)history.pushState(null,"",h);
  drawNav();
  if(v==="today")return today();
  if(v==="sched")return scheduled();
  return open(id)}
function fromHash(){var h=location.hash||"";
  if(h.indexOf("#c/")===0){var id=decodeURIComponent(h.slice(3));if(id)return go("chan",id,true)}
  if(h==="#scheduled")return go("sched",null,true);
  return go("today",null,true)}
window.onpopstate=fromHash;

/* ---- Today ---- */
function loadToday(){return api("/app/api/today").then(function(d){if(!d.error){todayData=d;drawAgentCard()}return d})}
function greeting(){var h=new Date().getHours();return h<5?"Still up.":h<12?"Good morning.":h<18?"Good afternoon.":"Good evening."}
function words(n){return ["No","One","Two","Three","Four","Five","Six","Seven","Eight","Nine","Ten"][n]||String(n)}
function today(){return Promise.all([loadToday(),threads()]).then(function(r){var d=r[0]||{};if(d.error){$("today").innerHTML='<div class="void"><p>'+esc(d.error)+'</p></div>';return}
  window._tsig=JSON.stringify([d.decisions.length,d.runs.length&&d.runs[0].id,d.schedules.length]);
  var n=d.decisions.length, runs=d.runs, chans={}, spent=0, reads=0;
  runs.forEach(function(x){chans[x.thread]=1;var dd=parseData(x.data);spent+=costOf(dd);reads+=(dd.threads_read||[]).length});
  var nc=Object.keys(chans).length;
  var date=new Date().toLocaleDateString([],{weekday:"long",day:"numeric",month:"long"});
  var head=n?greeting()+' <em>'+words(n)+(n===1?" thing":" things")+'</em> '+(n===1?"needs":"need")+' you.':greeting()+' <em>Nothing</em> needs you.';
  var lede=runs.length?"Since yesterday "+esc(agentName())+" worked "+(runs.length===1?"once":runs.length+" times")+" across "+nc+(nc===1?" channel":" channels")+(spent?", and spent "+money(spent):"")+".":
    (threadList.length?esc(AgentName())+" has not worked on its own yet. Give a channel a morning review, or let it react when someone writes.":"A channel is a place for one subject: you, "+esc(agentName())+", and anyone you bring in. Everything written there is the record the agent reads.");
  var h='<span class="kick">'+esc(date)+'</span><h1 class="hello">'+head+'</h1><p class="lede">'+lede+'</p>';
  if(!threadList.length){
    h+='<div style="margin-top:2.4rem" class="col"><span class="kick">Start with one</span><div class="starters">'+
      starter("A decision I'm making","Quotes, options, who said what. It keeps the numbers straight.","A decision: ")+
      starter("A project with a deadline","It notices what slipped and asks before anything leaves.","Project: ")+
      starter("Something I'm keeping an eye on","It checks on a schedule and tells you when it changes.","Watching: ")+
      '</div></div>';
    $("today").innerHTML=h;each("[data-starter]",function(b){b.onclick=function(){newThread(b.getAttribute("data-starter"))}});return}
  var left='<div class="col">';
  if(n){left+='<span class="kick">Needs you</span>'+d.decisions.map(function(x){if(x.connect)return '<div class="card dcard"><div class="where"><span class="hash">#</span><a data-open="'+esc(x.thread)+'">'+esc(x.title)+'</a><span class="tag ai">connect</span></div>'+connectCard(x.id,x.connect.service,x.connect.url,x.text,null,x.thread)+'</div>';return '<div class="card dcard"><div class="where"><span class="hash">#</span><a data-open="'+esc(x.thread)+'">'+esc(x.title)+'</a><span class="tag call">your call</span></div>'+
      '<div class="q">'+body(x.text)+'</div>'+decisionControls(x.id,x.options)+'</div>'}).join("")}
  left+='<span class="kick">'+(runs.length?"While you were away":"Recent")+'</span>';
  left+=runs.length?'<div class="card runs">'+runs.slice(0,12).map(function(x){var dd=parseData(x.data);var bad=dd.outcome==="error";
      var label=trigLabel(dd.trigger);
      var mt=[];if((dd.threads_read||[]).length)mt.push("read "+dd.threads_read.length);if((dd.tool_calls||[]).length)mt.push((dd.tool_calls.length)+(dd.tool_calls.length===1?" step":" steps"));if(cost(dd))mt.push(cost(dd));if(bad)mt.push(dd.error||"failed");
      return '<button class="runrow'+(bad?' bad':'')+'" data-open="'+esc(x.thread)+'"><span class="tm">'+esc(clock(x.ts))+'</span><span class="bd"><span class="ttl"><span>#</span>'+esc(x.title)+' · <span>'+esc(label)+'</span></span>'+
        '<span class="tx">'+esc(x.followed||x.text||"Nothing worth writing down.")+'</span><span class="mt">'+esc(mt.join(" · "))+'</span></span></button>'}).join("")+'</div>'
    :'<div class="card quiet">Nothing ran on its own in the last day and a half. When '+esc(agentName())+' has schedules or is watching a channel, what it did shows up here.</div>';
  left+='</div>';
  var up=upcoming(d.schedules);
  var right='<div class="col"><span class="kick">Coming up</span><div class="card up">'+(up.length?up.slice(0,5).map(function(s){return '<button class="uprow" data-open="'+esc(s.thread)+'"><span class="tm">'+esc(s._label)+'</span><span class="bd"><b>'+esc(s.name?cap(s.name)+" review":(s.prompt||"Check in"))+'</b><span>#'+esc(s.title)+' · '+esc(s._every)+'</span></span></button>'}).join(""):
      '<p class="quiet" style="padding:.8rem 1.1rem">Nothing scheduled.</p>')+'<button class="link" data-go-sched>'+(up.length?"All schedules →":"Schedule something →")+'</button></div>';
  if(d.watching.length)right+='<div class="card side-card"><b>Watching '+d.watching.length+(d.watching.length===1?" channel":" channels")+'</b><p>'+esc(AgentName())+' wakes when someone '+(d.watching.some(function(w){return w.on==="all"})?"writes":"else writes")+' in '+d.watching.map(function(w){return "#"+esc(w.title)}).join(", ")+'.</p></div>';
  var lonely=threadList.filter(function(t){return !t.shared&&!t.ever_shared&&t.entries>0}).sort(function(a,b){return (b.last||"")>(a.last||"")?1:-1})[0];
  if(lonely)right+='<div class="card side-card"><b>Bring someone in</b><p>A channel gets useful when someone else writes in it too. They get a link; no account needed.</p><button class="btn sm" data-invite="'+esc(lonely.id)+'">Share #'+esc(lonely.title)+'</button></div>';
  right+='</div>';
  $("today").innerHTML=h+'<div class="tgrid">'+left+right+'</div>';
  each("[data-open]",function(b){b.onclick=function(){go("chan",b.getAttribute("data-open"))}},$("today"));
  each("[data-go-sched]",function(b){b.onclick=function(){go("sched")}},$("today"));
  each("[data-invite]",function(b){b.onclick=function(){cur=b.getAttribute("data-invite");shareSheet()}},$("today"));
  wireDecisions($("today"),function(){today()});wireConnect($("today"))})}
function trigLabel(t){t=String(t||"");if(t.indexOf("reflect:")===0)return (t.slice(8)?cap(t.slice(8))+" review":"review");
  return {chat:"you asked",entry:"something arrived",peer_entry:"someone wrote",schedule:"on schedule",manual:"run by hand",decision:"after your answer",code:"a task"}[t]||t||"ran"}
function starter(t,s,pre){return '<button class="card starter" data-starter="'+esc(pre)+'"><b>'+esc(t)+'</b><span>'+esc(s)+'</span></button>'}
function cap(s){s=String(s||"");return s.charAt(0).toUpperCase()+s.slice(1)}
function upcoming(list){var now=new Date(),nowM=now.getHours()*60+now.getMinutes();
  return (list||[]).filter(function(s){return !s.paused}).map(function(s){var o=Object.assign({},s);
    if(s.at){var p=s.at.split(":"),m=(+p[0])*60+(+p[1]);o._sort=m>=nowM?m-nowM:m+1440-nowM;o._label=m>=nowM?s.at:"tmrw";o._every="daily "+s.at}
    else{o._sort=2000;o._label="~"+s.every;o._every="every "+s.every.replace(/0m0s$/,"").replace(/m0s$/,"m")}
    return o}).sort(function(a,b){return a._sort-b._sort})}

/* decisions, wherever they appear */
function decisionControls(id,options){return '<div class="opts">'+(options||[]).map(function(o,i){return '<button class="btn'+(i===0?" solid":"")+'" data-dec="'+esc(id)+'" data-choice="'+esc(o)+'">'+esc(o)+'</button>'}).join("")+'</div>'+
  '<div class="free"><input placeholder="Or answer in your own words" data-decin="'+esc(id)+'"><button class="btn" data-dec="'+esc(id)+'" data-free="1">Reply</button></div>'}
function wireDecisions(root,after){each("[data-dec]",function(b){b.onclick=function(){var id=b.getAttribute("data-dec"),choice=b.getAttribute("data-choice")||"",text="";
    if(b.getAttribute("data-free")){var inp=root.querySelector('[data-decin="'+id+'"]');text=inp?inp.value.trim():"";if(!text){if(inp)inp.focus();return}}
    each('[data-dec="'+id+'"]',function(x){x.disabled=true},root);b.textContent=choice?choice+" · "+agentName()+" is continuing…":"Sent · "+agentName()+" is continuing…";
    api("/app/api/decision",{id:id,choice:choice,text:text}).then(function(d){if(d.error)alert(d.error);after()})}},root)}

/* ---- a channel ---- */
function body(t){return esc(t).replace(/\*\*([^*\n]{1,200})\*\*/g,"<b>$1</b>").replace(/(^|\s)@([A-Za-z][\w.-]{0,31})/g,'$1<span class="mention">@$2</span>').replace(/\[\[([^\]]{1,120})\]\]/g,function(m,name){var id=titles[name.trim().toLowerCase()];
  return id?'<a class="ref" data-go="'+esc(id)+'">'+esc(name)+'</a>':'<span class="ref dead" title="No channel with this title">'+esc(name)+'</span>'})}
function isBot(e){return !!e.agent||e.kind==="agent.run"||e.kind==="agent.decision"}
var HUES=["#4C6FF5","#1F9E68","#D9480F","#9C36B5","#0C8599","#C2255C","#5F3DC4","#E67700"];
function hueOf(id){var h=0;for(var i=0;i<id.length;i++)h=(h*31+id.charCodeAt(i))>>>0;return HUES[1+h%(HUES.length-1)]}
function avatar(e){if(isBot(e)){var n=e.who==="your agent"?AgentName():e.who;return '<div class="av bot"'+(e.persona?' style="background:'+hueOf(e.persona)+'"':'')+'>'+esc(initials(n==="Your agent"?"A":n))+'</div>'}
  return '<div class="av '+(e.mine?"you":"peer")+'">'+esc(initials(e.who))+'</div>'}
function whoName(e){if(e.who==="your agent")return AgentName();return e.who}
function metaLine(e,extra){return '<div class="meta"><span class="auth">'+esc(whoName(e))+'</span>'+(e.agent?'<span class="tag ai">'+esc(e.agent==="agent"?"agent":e.agent)+'</span>':'')+(extra||'')+'<span>'+esc(when(e.ts))+'</span></div>'}
function runCard(e){var d=parseData(e.data);var bad=d.outcome==="error",wait=d.outcome==="waiting";
  var secs=Math.max(1,Math.round((d.duration_ms||0)/1000));
  var trig=String(d.trigger||"");
  var lead=bad?"Stopped: "+(d.error||"something went wrong"):(trig.indexOf("reflect:")===0?cap(trig.slice(8))+" review · worked for "+secs+"s":"Worked for "+secs+"s");
  if(wait)lead="Waiting on you · worked for "+secs+"s";
  var steps=[];
  (d.threads_read||[]).forEach(function(id){steps.push("Read #"+esc(threadOf(id).title||id))});
  (d.tool_calls||[]).forEach(function(t){if(t==="read_thread"||t==="list_threads")return;steps.push(esc(stepName(t)))});
  (d.fetches||[]).forEach(function(f){steps.push('Opened '+esc(f.url)+(f.error?' <span class="er">('+esc(f.error)+')</span>':''))});
  (d.external||[]).forEach(function(x){steps.push('Used '+esc(x.tool)+(x.error?' <span class="er">('+esc(x.error)+')</span>':''))});
  (d.problems||[]).forEach(function(p){steps.push('<span class="er">'+esc(p)+'</span>')});
  var mt=[];if(steps.length)mt.push(steps.length+(steps.length===1?" step":" steps"));if(cost(d))mt.push(cost(d));
  var fine=(d.tokens?(d.tokens.prompt||0)+" in · "+(d.tokens.completion||0)+" out · ":"")+esc(d.model||"");
  return '<details class="runcard'+(bad?' bad':(wait?' wait':''))+'"><summary><span class="st">'+(bad?ICON.x:(wait?ICON.wait:ICON.check))+'</span><span>'+esc(lead)+'</span><span class="mt">'+esc(mt.join(" · "))+'</span>'+ICON.chev+'</summary>'+
    '<div class="steps">'+(steps.length?steps.map(function(s){return '<span>'+s+'</span>'}).join(""):'<span>Read this channel</span>')+'<span class="fine">'+fine+'</span></div></details>'}
function stepName(t){var m={search_threads:"Searched your channels",post_note:"Wrote a note",ask_person:"Asked you",fetch_url:"Opened a page",read_file:"Read a file",list_files:"Listed files",search_files:"Searched files",edit_file:"Edited a file",write_file:"Wrote a file",run:"Ran a command",open_path:"Asked to open a folder",where:"Checked where it may work"};return m[t]||("Used "+t)}
function render(es){var out=[],prev=null;
  es.forEach(function(e){
    if(e.kind==="thread.brief"||e.kind==="agent.decision_reply"||e.kind==="agent.connect_reply"||e.kind==="project.member"||e.kind==="project.is"||e.lane==="control")return;
    if(e.kind==="agent.connect"){var cd=parseData(e.data);var rep=es.filter(function(x){return x.kind==="agent.connect_reply"&&x.replies_to===e.id})[0];
      out.push('<div class="entry">'+avatar(e)+'<div class="c">'+metaLine(e)+connectCard(e.id,cd.service,cd.url,e.text,rep?parseData(rep.data):null,cur)+'</div></div>');prev={key:"bot:"+e.author,ts:e.ts};return}
    if(tab==="agent"&&!isBot(e))return;
    var key=(isBot(e)?"bot:":"")+e.author;
    var cont=prev&&prev.key===key&&(new Date(e.ts)-new Date(prev.ts))<5*60000&&e.kind!=="agent.decision";
    if(e.kind==="agent.run"){out.push('<div class="entry'+(cont?' cont':'')+'">'+avatar(e)+'<div class="c">'+(cont?'':metaLine(e,'<span class="tag ai">agent</span>'))+runCard(e)+'</div></div>');prev={key:key,ts:e.ts};return}
    if(e.kind==="agent.decision"){var reply=es.filter(function(x){return x.kind==="agent.decision_reply"&&x.replies_to===e.id})[0];
      var inner='<div class="decision"><div style="display:flex;gap:.5rem;align-items:center"><span class="tag call">your call</span><span class="small muted">'+esc(AgentName())+' will not guess on this one</span></div><div class="ask">'+body(e.text)+'</div>';
      if(reply){var r=parseData(reply.data);inner+='<div class="done">You answered: '+esc((r.choice||"")+(r.text?" "+r.text:""))+'</div>'}else inner+=decisionControls(e.id,e.options);
      out.push('<div class="entry">'+avatar(e)+'<div class="c">'+metaLine(e)+inner+'</div></div></div>');prev={key:key,ts:e.ts};return}
    var sum=e.lane==="summary",q=e.kind==="chat.question";
    var extra=(sum?'<span class="tag call">what you shared</span>':'')+(q?'<span class="tag">asked</span>':'');
    out.push('<div class="entry'+(sum?' summary':'')+(q?' q':'')+(cont&&!extra?' cont':'')+'">'+avatar(e)+'<div class="c">'+(cont&&!extra?'':metaLine(e,extra))+'<div class="body">'+body(e.text)+'</div></div></div>');
    prev={key:key,ts:e.ts}});
  return out.join("")}
function thinking(t){
  var el=document.createElement("div");el.className="thinking";el.id="thinking";
  el.innerHTML='<div class="av bot">'+esc(initials(AgentName()==="Your agent"?"A":AgentName()))+'</div><span class="lbl"></span>';
  var lbl=el.querySelector(".lbl");lbl.textContent=t||AgentName()+" is reading…";$("stream").appendChild(el);$("feed").scrollTop=$("feed").scrollHeight;
  var t0=Date.now(), base=lbl.textContent;clearInterval(window._thinkTimer);
  window._thinkTimer=setInterval(function(){if(!document.getElementById("thinking")){clearInterval(window._thinkTimer);return}
    var s=Math.round((Date.now()-t0)/1000);lbl.textContent=s<3?base:base+" · "+s+"s"+(s>25?" · this one is taking a while":"")},500)}
function agentPill(t){var b=$("agentbtn");b.hidden=false;var on=!!t.auto,wait=t.waiting>0;
  if(t.full_auto&&!wait){b.className="pill fa";b.title=AgentName()+" decides and acts here without asking";b.innerHTML=ICON.bolt+'<span class="full">Full auto</span><span class="short"></span>';return}
  b.className="pill"+(wait?" wait":(on?" on":" off"));
  b.title=on?"What "+agentName()+" does here on its own":AgentName()+" only answers when asked. Click to let it work on its own.";
  b.innerHTML='<i></i><span class="full">'+(wait?AgentName()+" needs you":(on?AgentName()+" is on":AgentName()+" is off"))+'</span><span class="short">'+(wait?"you":"")+'</span>'}
function members(t){var a='<div class="av you">'+esc(initials(me?me.name:"you"))+'</div><div class="av bot">'+esc(initials(AgentName()==="Your agent"?"A":AgentName()))+'</div>';
  var n=(t.shared||0);if(n)a+='<span class="more">+'+n+'</span>';$("avs").innerHTML=a}
function open(id){if(id!==cur){here=[];target=""}cur=id;$("share").disabled=false;threads();
  return api("/app/api/thread/"+encodeURIComponent(id)).then(function(d){if(cur!==id)return;if(d.error){note(d.error,"bad");$("stream").innerHTML='<div class="void"><h2>Not here</h2><p>That channel is not on this account.</p></div>';return}
    entries=d.entries;var lastE=d.entries[d.entries.length-1];window._sig=d.entries.length+":"+(lastE?lastE.id:"");
    var tt=threadOf(id),par=tt.project?threadOf(tt.project):null;
    $("title").innerHTML=(par?'<a class="crumb" data-go="'+esc(par.id)+'">'+ICON.stack+esc(par.title)+'</a><span class="dim">/</span>':'')+(tt.is_project?ICON.stack:'<span class="hash">#</span>')+esc(d.title||"untitled")+(tt.is_project?'<span class="tag">project · '+(tt.children||0)+'</span>':'');
    each("[data-go]",function(a){a.onclick=function(){go("chan",a.getAttribute("data-go"))}},$("title"));
    $("text").placeholder=(tt.is_project&&mode==="ask"?"Ask "+agentName()+" across all "+(tt.children||0)+" channels in "+d.title+"…":mode==="ask"?"Ask "+agentName()+" about #"+String(d.title||"this channel").replace(/…$/,"")+"…":"Write in #"+(d.title||"this channel")+" — @"+(agentName()==="your agent"?"agent":agentName())+" to ask");
    var vis=d.entries.filter(function(e){return e.lane!=="control"&&e.kind!=="thread.brief"&&e.kind!=="agent.decision_reply"&&e.kind!=="agent.connect_reply"&&e.kind!=="project.member"&&e.kind!=="project.is"});
    var t=threadOf(id);agentPill(t);members(t);$("more").hidden=!t.mine;if(t.last)seen(id,t.last);
    api("/app/api/thread/"+encodeURIComponent(id)+"/links").then(function(l){if(cur!==id)return;var b=$("linksbtn");b.hidden=!l.total;
      if(l.total){b.innerHTML='<span class="full">'+l.total+(l.total===1?" link":" links")+'</span><span class="short">⇄'+l.total+'</span>';window._links=l}});
    each(".tabs button",function(b){b.setAttribute("aria-pressed",String(b.getAttribute("data-tab")===tab))});
    var nudge=t.since_shared?'<div class="nudge"><span>'+t.since_shared+(t.since_shared===1?" message":" messages")+' since you last shared '+esc(when(t.last_shared))+'.</span><button class="btn sm solid" id="nudge-go">Send an update</button></div>':'';
    var html=render(d.entries);
    $("stream").innerHTML=nudge+(vis.length?(html||'<div class="void"><p>'+esc(AgentName())+' has not done anything here yet.</p></div>'):'<div class="void"><h2>#'+esc(d.title)+'</h2>'+
      '<p>Write what you know, or ask '+esc(agentName())+' something. It answers from what is in here and in your other channels.</p>'+
      '<button class="btn" id="void-agent">Let it work here while you are away</button></div>');
    if($("void-agent"))$("void-agent").onclick=agentSheet;
    if(t.since_shared)$("nudge-go").onclick=shareSheet;
    each("[data-go]",function(a){a.onclick=function(){go("chan",a.getAttribute("data-go"))}},$("stream"));
    wireDecisions($("stream"),function(){open(cur)});wireConnect($("stream"));
    $("feed").scrollTop=$("feed").scrollHeight;if(!busy&&window.innerWidth>860)$("text").focus();
    drawPanel(id)})}

/* the right-hand panel: what the agent does here, in one glance */
function drawPanel(id){var p=$("panel");if(window.innerWidth<=1250){return}
  Promise.all([api("/app/api/thread/"+encodeURIComponent(id)+"/brief"),api("/app/api/thread/"+encodeURIComponent(id)+"/access")]).then(function(r){if(cur!==id)return;
    var b=(r[0]&&r[0].brief)||{},acc=r[1]||{};var on=b.on_new_entry&&b.on_new_entry!=="off";
    var rh=(b.rhythms||[]);var tools=(b.tools||[]);
    var me2=threadOf(id),h="";
    if(me2.is_project){var kids=threadList.filter(function(x){return x.project===id}),loose=threadList.filter(function(x){return !x.is_project&&!x.project});
      h+='<div class="blk"><span class="kick">Channels in this project</span>'+(kids.length?kids.map(function(k){return '<div class="ln"><a class="kidlink" data-go="'+esc(k.id)+'"># '+esc(k.title)+'</a><span class="small muted">'+(k.shared||k.ever_shared?"shared":"just you")+'</span></div>'}).join(""):'<span class="sub">None yet. Give each vendor or partner their own.</span>')+
        '<button class="edit" id="p-addkid">+ New channel here</button>'+(loose.length?'<select id="p-movein"><option value="">Move a channel in…</option>'+loose.map(function(x){return '<option value="'+esc(x.id)+'">#'+esc(x.title)+'</option>'}).join("")+'</select>':'')+
        '<span class="sub">Only you see this project. '+esc(AgentName())+' here reads every channel in it and can write into each one. Nobody in those channels can see the others.</span></div>'}
    if(me2.project){var pp=threadOf(me2.project);
      h+='<div class="blk walled"><span class="kick">Walled</span><span class="sub">Part of <a class="kidlink" data-go="'+esc(pp.id)+'">'+esc(pp.title||"a project")+'</a>. '+esc(AgentName())+' here sees only this channel, so nothing from the other channels can reach the people in this one.</span><button class="edit" id="p-moveout">Take it out of the project</button></div>'}
    h+='<div class="blk"><span class="kick">'+esc(AgentName())+' here</span><div class="ln"><span>Works on its own</span><button class="switch" role="switch" aria-checked="'+on+'" id="p-on" aria-label="Works on its own"></button></div>'+
      '<span class="sub">'+(on?"Wakes when "+(b.on_new_entry==="all"?"anyone":"someone else")+" writes here, and asks before anything leaves Lamdis.":"Answers when you ask. Turn this on and it reads what arrives and acts on its own.")+'</span></div>';
    h+='<div class="blk"><span class="kick">Schedules</span>'+(rh.length?rh.map(function(x){return '<div class="ln"><span'+(x.paused?' class="dim"':'')+'>'+esc(x.name?cap(x.name)+" review":"Check-in")+'</span><span class="mono small muted">'+(x.paused?"paused":esc(x.at))+'</span></div>'+(x.prompt?'<span class="sub">“'+esc(x.prompt)+'”</span>':'')}).join(""):'<span class="sub">None. A morning review is the usual first one.</span>')+
      (b.every?'<div class="ln"><span>Check-in</span><span class="mono small muted">every '+esc(b.every)+'</span></div>':'')+'<button class="edit" id="p-sched">+ Add a schedule</button></div>';
    h+='<div class="blk"><span class="kick">On its own it may use</span>'+(tools.length||b.web?(b.web?'<div class="ln"><span>The web</span><span class="tag">'+((b.allow_domains||[]).length?(b.allow_domains.length+" sites"):"listed sites")+'</span></div>':'')+tools.map(function(t){return '<div class="ln"><span>'+esc(t)+'</span></div>'}).join(""):'<span class="sub">Only this record. When you ask it yourself, it may use everything you have connected.</span>')+'</div>';
    var links=(acc.links||[]),grants=(acc.grants||[]);
    h+='<div class="blk"><span class="kick">Who sees what</span>'+(links.length||grants.length?grants.map(function(g){return '<div class="ln"><span>'+esc(g.name)+'</span><span class="small muted">'+esc(g.scopes.join(", "))+'</span></div>'}).join("")+links.map(function(l){return '<div class="ln"><span>'+esc(l.label||"A link")+'</span><span class="small muted">'+(l.lanes.indexOf("content")>=0?"everything":"summaries")+'</span></div>'}).join(""):'<span class="sub">Only you and '+esc(agentName())+'.</span>')+'<button class="edit" id="p-share">Share…</button></div>';
    here=(b.agents||[]).slice();drawWho();
    var inTeam=team.filter(function(p){return here.indexOf(p.id)>=0}),notHere=team.filter(function(p){return here.indexOf(p.id)<0});
    h+='<div class="blk"><span class="kick">Agents here</span><div class="chips"><span class="achip"><i style="background:var(--blue)"></i>'+esc(AgentName())+'</span>'+
      inTeam.map(function(p){return '<span class="achip"><i style="background:'+hueOf(p.id)+'"></i>'+esc(p.name)+'<button data-unhere="'+esc(p.id)+'" aria-label="Remove '+esc(p.name)+'">×</button></span>'}).join("")+'</div>'+
      (notHere.length?'<select id="p-addagent"><option value="">Add an agent here…</option>'+notHere.map(function(p){return '<option value="'+esc(p.id)+'">'+esc(p.name)+'</option>'}).join("")+'</select>':'')+
      '<button class="edit" id="p-team">'+(team.length?'Edit your team':'+ Create agents with their own expertise')+'</button></div>';
    var au=b.autonomy||"";var glob=(agentInfo&&agentInfo.reach&&agentInfo.reach.autonomy)||"ask";
    h+='<div class="blk"><span class="kick">When it has a choice</span><div class="seg3" role="group" aria-label="Autonomy here">'+
      '<button data-au="" aria-pressed="'+(au==="")+'">Default</button><button data-au="ask" aria-pressed="'+(au==="ask")+'">Ask me</button><button data-au="auto" aria-pressed="'+(au==="auto")+'">'+ICON.bolt+' Full auto</button></div>'+
      '<span class="sub">'+(au==="auto"||(au===""&&glob==="auto")?"Full auto: it decides and acts without stopping, including actions in connected services. Daily limits still apply.":"It stops for your call and asks before changing anything.")+(au===""?" (Same as everywhere: "+(glob==="auto"?"full auto":"ask me")+".)":"")+'</span></div>';
    h+='<button class="btn sm ghost" id="p-edit" style="align-self:flex-start">Everything it does here…</button>';
    p.innerHTML=h;
    $("p-on").onclick=function(){var nb=Object.assign({},b);nb.on_new_entry=on?"off":"others";if(!nb.rhythms)nb.rhythms=[];
      api("/app/api/thread/"+encodeURIComponent(id)+"/brief",nb).then(function(r){if(r.error){alert(r.error);return}threads().then(function(){agentPill(threadOf(id));drawPanel(id)});loadToday()})};
    $("p-sched").onclick=function(){scheduleSheet(id)};
    each("[data-go]",function(a){a.onclick=function(){go("chan",a.getAttribute("data-go"))}},p);
    each("[data-au]",function(x){x.onclick=function(){var nb=Object.assign({},b);nb.autonomy=x.getAttribute("data-au");if(!nb.rhythms)nb.rhythms=[];if(!nb.on_new_entry)nb.on_new_entry="off";
      api("/app/api/thread/"+encodeURIComponent(id)+"/brief",nb).then(function(r){if(r.error){alert(r.error);return}threads().then(function(){agentPill(threadOf(id));drawPanel(id)})})}},p);
    if($("p-addkid"))$("p-addkid").onclick=function(){newThread("",id)};
    var saveHere=function(list){var nb=Object.assign({},b);nb.agents=list;if(!nb.rhythms)nb.rhythms=[];if(!nb.on_new_entry)nb.on_new_entry="off";
      api("/app/api/thread/"+encodeURIComponent(id)+"/brief",nb).then(function(r){if(r.error){alert(r.error);return}drawPanel(id)})};
    if($("p-addagent"))$("p-addagent").onchange=function(){if(this.value)saveHere(here.concat([this.value]))};
    each("[data-unhere]",function(x){x.onclick=function(){var v=x.getAttribute("data-unhere");saveHere(here.filter(function(y){return y!==v}))}},p);
    $("p-team").onclick=function(){teamSheet()};
    if($("p-movein"))$("p-movein").onchange=function(){var v=this.value;if(!v)return;api("/app/api/project/move",{thread:v,project:id}).then(function(){threads().then(function(){drawPanel(id);open(id)})})};
    if($("p-moveout"))$("p-moveout").onclick=function(){api("/app/api/project/move",{thread:id,project:""}).then(function(){threads().then(function(){open(id)})})};$("p-share").onclick=shareSheet;$("p-edit").onclick=agentSheet})}

/* ---- connecting a service from a card the agent offered ----
   The agent can only propose. Signing in happens on the service's own page
   in a window of its own; a key, if one is needed, goes into this field and
   straight to the vault, never into the channel or to the model. */
function connectCard(id,service,url,text,reply,thread){var host="";try{host=new URL(url).host}catch(e){}
  var h='<div class="connect" data-connect="'+esc(id)+'" data-service="'+esc(service||host)+'" data-url="'+esc(url||"")+'" data-thread="'+esc(thread||"")+'">'+
    '<div class="cc-top"><div class="cc-logo">'+esc(String(service||host||"?").charAt(0).toUpperCase())+'</div><div class="cc-t"><b>'+esc(service||host)+'</b><span>'+esc(host)+'</span></div></div>'+
    (text?'<p class="cc-why">'+body(text)+'</p>':'');
  if(reply)return h+'<div class="cc-done'+(reply.ok?' ok':'')+'">'+(reply.ok?'Connected'+(reply.tools?' · '+reply.tools+(reply.tools===1?' thing':' things')+' it can do':''):esc(reply.text||"Not connected"))+'</div></div>';
  return h+'<div class="cc-act"><button class="btn solid" data-cc-go>Connect '+esc(service||host)+'</button><button class="btn ghost" data-cc-no>Not now</button></div>'+
    '<div class="cc-key" hidden><input type="password" autocomplete="off" placeholder="Paste the key from '+esc(service||host)+'’s settings"><button class="btn solid" data-cc-key>Save</button></div>'+
    '<div class="cc-status" aria-live="polite"></div><p class="cc-fine">You sign in on '+esc(service||host)+'’s own page. '+esc(AgentName())+' never sees your password, and anything that changes something asks you first.</p></div>'}
function wireConnect(root){each("[data-connect]",function(card){var go=card.querySelector("[data-cc-go]");if(!go)return;
  var id=card.getAttribute("data-connect"),svc=card.getAttribute("data-service"),url=card.getAttribute("data-url"),thread=card.getAttribute("data-thread");
  var st=card.querySelector(".cc-status"),keyBox=card.querySelector(".cc-key");
  function say(t,bad){st.textContent=t||"";st.className="cc-status"+(bad?" bad":"")}
  function allOf(tools){return {allow:tools.map(function(t){return t.name}),confirm:tools.filter(function(t){return t.writes}).map(function(t){return t.name}),known:tools}}
  function finish(name,tools){var m=allOf(tools||[]);
    return api("/app/api/tools",{name:name,url:url,allow:m.allow,confirm:m.confirm,known:m.known}).then(function(r){if(r.error){say(r.error,true);go.disabled=false;return}
      return api("/app/api/connect",{id:id,ok:true,name:svc,tools:(tools||[]).length}).then(function(){loadToday();
        say("Connected. "+AgentName()+" is picking up where you left off…");
        return api("/app/api/chat",{thread:thread,text:"I've connected "+svc+". Carry on with what I asked."}).then(function(){if(view==="chan")open(cur);else today()})})})}
  function needsAccount(msg){say(msg,true);if(window.show&&/email/i.test(msg)){var k=document.createElement("button");k.className="btn sm";k.textContent="Add your email";k.onclick=function(){window.show("gate")};st.appendChild(document.createTextNode(" "));st.appendChild(k)}}
  go.onclick=function(){go.disabled=true;say("Looking at "+svc+"…");
    api("/app/api/tools/probe",{name:"",url:url}).then(function(p){var name=p.name||svc.toLowerCase().replace(/[^a-z0-9]+/g,"-");
      if(p.needs_signin){
        return api("/app/api/tools",{name:name,url:url,allow:[],confirm:[]}).then(function(r){if(r.error){needsAccount(r.error);go.disabled=false;return}
          return api("/app/api/tools/auth/start",{name:name,url:url}).then(function(a){if(a.error){needsAccount(a.error);go.disabled=false;return}
            var win=window.open(a.authorize,"lamdis-connect","width=520,height=700");
            if(!win){say("Your browser blocked the sign-in window. Allow pop-ups for this site and press Connect again.",true);go.disabled=false;return}
            say("Sign in to "+svc+" in the window that opened.");
            var done=function(e){if(e.origin!==location.origin||!e.data||e.data.lamdis!=="tools-auth")return;window.removeEventListener("message",done);
              say("Signed in. Reading what "+svc+" offers…");
              api("/app/api/tools/probe",{name:name,url:url}).then(function(q){if(q.error){say(q.error,true);go.disabled=false;return}finish(name,q.tools)})};
            window.addEventListener("message",done)})})}
      if(p.error){say(svc+" wants a key rather than a sign-in. Paste it below; it goes straight into your vault.");keyBox.hidden=false;keyBox.querySelector("input").focus();
        keyBox.querySelector("[data-cc-key]").onclick=function(){var k=keyBox.querySelector("input").value.trim();if(!k)return;
          api("/app/api/tools/probe",{name:name,url:url,auth:k}).then(function(q){if(q.error){say(q.error,true);return}
            api("/app/api/tools",{name:name,url:url,auth:k,allow:[],confirm:[]}).then(function(r){if(r.error){needsAccount(r.error);return}keyBox.querySelector("input").value="";finish(name,q.tools)})})};
        return}
      return finish(name,p.tools)})};
  card.querySelector("[data-cc-no]").onclick=function(){api("/app/api/connect",{id:id,ok:false}).then(function(){loadToday();if(view==="chan")open(cur);else today()})}},root)}

/* ---- the composer ---- */
function grow(){var t=$("text");t.style.height="auto";t.style.height=Math.min(t.scrollHeight,224)+"px";$("send").disabled=!t.value.trim()}
function drawMode(){each(".seg button",function(b){b.setAttribute("aria-pressed",String(b.getAttribute("data-mode")===mode))});
  var an=agentName()==="your agent"?"agent":agentName();$("mode-ask").textContent="Ask "+(an==="agent"?"agent":an);
  if(cur&&view==="chan"){var t=threadOf(cur);$("text").placeholder=mode==="ask"?"Ask "+agentName()+" about #"+String(t.title||"this channel").replace(/…$/,"")+"…":"Write in #"+(t.title||"this channel")+" — @"+an+" to ask"}}
function mentionsAgent(t){var n=agentName()==="your agent"?"agent":agentName();var re=new RegExp("(^|\\s)@("+n.replace(/[.*+?^${}()|[\]\\]/g,"\\$&")+"|agent)\\b","i");return re.test(t)}
function send(){var t=$("text").value.trim();if(!cur||!t)return;
  var at=team.filter(function(p){return new RegExp("(^|\\s)@"+p.name.replace(/[.*+?^${}()|[\]\\]/g,"\\$&")+"\\b","i").test(t)})[0];if(at){target=at.id;return ask()}
  if(mode==="ask"||mentionsAgent(t)){if(me&&!me.can_ask){note(AgentName()+" is off until a model key is set.","warn");return}ask();return}
  post()}
function post(){var t=$("text").value.trim();if(!cur||!t)return;
  api("/app/api/post",{thread:cur,text:t,lane:"content"}).then(function(d){if(d.error){note(d.error,"bad");return}
    $("text").value="";grow();note("");open(cur)})}
function ask(){var q=$("text").value.trim();if(!cur||!q||busy)return;busy=true;note("");
  $("text").value="";grow();var el=document.createElement("div");el.className="entry q";
  el.innerHTML='<div class="av you">'+esc(initials(me?me.name:"you"))+'</div><div class="c"><div class="meta"><span class="auth">'+esc(me?me.name:"you")+'</span><span class="tag">asked</span></div><div class="body">'+body(q)+'</div></div>';
  var v=$("stream").querySelector(".void");if(v)v.remove();
  $("stream").appendChild(el);thinking();
  thinkingAs(target);
  api("/app/api/chat",{thread:cur,text:q,persona:target}).then(function(d){busy=false;if(d.error)note(d.error,"bad");open(cur);loadToday()}).catch(function(){busy=false;open(cur)})}

function thinkingAs(pid){var p=team.filter(function(x){return x.id===pid})[0];var th=$("thinking");if(!th||!p)return;
  var av=th.querySelector(".av");av.textContent=initials(p.name);av.style.background=hueOf(p.id);th.querySelector(".lbl").textContent=p.name+" is working…"}
function loadTeam(){return api("/app/api/agents").then(function(d){team=d.agents||[];return team})}
/* who can be asked in this channel: the main agent and the members added here */
function drawWho(){var w=$("who");if(!w)return;var opts=[{id:"",name:AgentName()}].concat(team.filter(function(p){return here.indexOf(p.id)>=0}));
  if(target&&!opts.some(function(o){return o.id===target}))target="";
  w.hidden=opts.length<2||mode!=="ask";
  w.innerHTML=opts.map(function(o){return '<option value="'+esc(o.id)+'"'+(o.id===target?" selected":"")+'>'+esc(o.name)+'</option>'}).join("");
  var nm=target?(team.filter(function(p){return p.id===target})[0]||{}).name:agentName();
  if(view==="chan"&&mode==="ask"){var tt=threadOf(cur);$("text").placeholder=tt.is_project?"Ask "+nm+" across all "+(tt.children||0)+" channels in "+tt.title+"…":"Ask "+nm+" about #"+String(tt.title||"this channel").replace(/…$/,"")+"…"}}

/* The team: more than one agent, each with its own manner and expertise. */
var PRESETS=[
  {name:"Scout",about:"A thorough researcher. Finds options, providers and facts on the web, checks them, and always gives the web address for each."},
  {name:"Haggler",about:"A calm, firm negotiator. Compares offers against what we agreed and the budget, spots leverage, and drafts counter-offers. Never agrees to anything; proposes."},
  {name:"Ledger",about:"A careful bookkeeper. Keeps running totals, deposits, due dates and what is still unpaid. Shows the arithmetic."},
  {name:"Devil",about:"A devil's advocate. Looks for what could go wrong, what is missing, and what a sceptic would ask. Short and direct."}];
function teamSheet(editId){loadTeam().then(function(){var ed=editId?team.filter(function(p){return p.id===editId})[0]:null;
  var mc=(agentInfo&&agentInfo.reach&&agentInfo.reach.model_choices)||[];
  var s=sheet('<header><h2>Your team</h2><p>Add agents with their own personality and expertise. Put them in channels or projects, then ask one with @name. They work under your agent\u2019s permissions and never reach further.</p></header><section>'+
    '<div class="list">'+'<div class="row"><div class="av bot">'+esc(initials(AgentName()==="Your agent"?"A":AgentName()))+'</div><div class="t"><b>'+esc(AgentName())+'</b><span>main agent · everywhere</span></div></div>'+
    team.map(function(p){return '<div class="row"><div class="av bot" style="background:'+hueOf(p.id)+'">'+esc(initials(p.name))+'</div><div class="t"><b>'+esc(p.name)+'</b><span>'+esc(p.about)+'</span></div><button class="btn sm" data-ed="'+esc(p.id)+'">Edit</button><button class="btn sm danger" data-rm="'+esc(p.id)+'">Remove</button></div>'}).join("")+'</div>'+
    '<label class="f">'+(ed?'Edit '+esc(ed.name):'Add an agent')+'</label>'+(ed?'':'<div class="presets">'+PRESETS.map(function(p,i){return '<button class="btn sm" data-pre="'+i+'">'+esc(p.name)+'</button>'}).join("")+'</div>')+
    '<input id="tm-name" maxlength="32" placeholder="Name, e.g. Scout" value="'+esc(ed?ed.name:"")+'" style="margin-top:.55rem">'+
    '<textarea id="tm-about" rows="3" placeholder="Personality and expertise: what it is good at and how it should work" style="margin-top:.5rem">'+esc(ed?ed.about:"")+'</textarea>'+
    (mc.length>1?'<select id="tm-model" style="margin-top:.5rem"><option value="">Same model as '+esc(AgentName())+'</option>'+mc.map(function(m){return '<option'+(ed&&ed.model===m?" selected":"")+'>'+esc(m)+'</option>'}).join("")+'</select>':'')+
    '</section><footer><span class="spacer"></span><button class="btn" data-x>Done</button><button class="btn solid" id="tm-go">'+(ed?'Save':'Add to team')+'</button></footer>');
  s.querySelector("[data-x]").onclick=closeSheet;
  each("[data-pre]",function(b){b.onclick=function(){var p=PRESETS[+b.getAttribute("data-pre")];s.querySelector("#tm-name").value=p.name;s.querySelector("#tm-about").value=p.about}},s);
  each("[data-ed]",function(b){b.onclick=function(){teamSheet(b.getAttribute("data-ed"))}},s);
  each("[data-rm]",function(b){b.onclick=function(){if(!b.getAttribute("data-armed")){b.setAttribute("data-armed","1");b.textContent="Sure?";return}api("/app/api/agents/remove",{id:b.getAttribute("data-rm")}).then(function(){teamSheet();if(view==="chan")open(cur)})}},s);
  s.querySelector("#tm-go").onclick=function(){var m=s.querySelector("#tm-model");
    api("/app/api/agents",{id:ed?ed.id:"",name:s.querySelector("#tm-name").value,about:s.querySelector("#tm-about").value,model:m?m.value:""}).then(function(r){if(r.error){alert(r.error);return}
      loadTeam().then(function(){if(view==="chan")open(cur);teamSheet()})})}})}

/* ---- sheets ---- */
function sheet(html){closeSheet();sheetEl=document.createElement("div");sheetEl.className="veil";
  sheetEl.innerHTML='<div class="sheet" role="dialog" aria-modal="true">'+html+'</div>';document.body.appendChild(sheetEl);
  sheetEl.onclick=function(e){if(e.target===sheetEl)closeSheet()};return sheetEl.firstChild}
function closeSheet(){if(sheetEl){sheetEl.remove();sheetEl=null}}

function newThread(prefill,project){var pj=project?threadOf(project):null;
  var s=sheet('<header><h2>'+(pj?'New channel in '+esc(pj.title):'New channel')+'</h2><p>'+(pj?'Give each vendor, contractor or partner their own channel. They see only theirs; you see them all in the project.':'One subject per channel: a deal, a person, a decision. '+esc(AgentName())+' reads all of it.')+'</p></header>'+
  '<section><input id="nt" placeholder="'+(pj?'Who or what is it for? e.g. tiler-acme':'What is it about?')+'" value="'+esc(typeof prefill==="string"?prefill:"")+'" autofocus>'+
  (pj?(function(){var others=threadList.filter(function(x){return !x.is_project&&x.project!==project});if(!others.length)return "";
    return '<label class="f">Or add channels you already have</label><div class="list">'+others.map(function(x){var inP=x.project?threadOf(x.project):null;
      return '<div class="row"><div class="t"><b># '+esc(x.title)+'</b><span>'+(inP?'now in '+esc(inP.title):(x.shared||x.ever_shared?'shared':'just you'))+'</span></div><button class="btn sm" data-addin="'+esc(x.id)+'">Add</button></div>'}).join("")+'</div>'+
      '<p class="hint">Once it is in the project, '+esc(agentName())+' in that channel sees only that channel, and the project sees it.</p>'})():'')+'</section>'+
  '<footer><span class="spacer"></span><button class="btn" data-x>Cancel</button><button class="btn solid" data-go>Create</button></footer>');
  s.querySelector("[data-x]").onclick=closeSheet;var inp=s.querySelector("#nt");setTimeout(function(){inp.focus();inp.setSelectionRange(inp.value.length,inp.value.length)},30);
  var gogo=function(){var t=inp.value.trim();if(!t)return;(project?api("/app/api/project/"+encodeURIComponent(project)+"/channels",{title:t}):api("/app/api/threads",{title:t})).then(function(d){if(d.error){alert(d.error);return}closeSheet();threads().then(function(){go("chan",d.id)})})};
  s.querySelector("[data-go]").onclick=gogo;inp.onkeydown=function(e){if(e.key==="Enter")gogo()};
  each("[data-addin]",function(b){b.onclick=function(){b.disabled=true;moveTo(b.getAttribute("data-addin"),project).then(function(){b.textContent="Added ✓";var sp=b.parentNode.querySelector(".t span");if(sp)sp.textContent="in "+pj.title})}},s)}

/* Put a channel in a project (or take it out with project ""), from anywhere. */
function moveTo(thread,project){return api("/app/api/project/move",{thread:thread,project:project||""}).then(function(r){if(r&&r.error){alert(r.error);return r}
  return threads().then(function(){if(view==="chan"&&cur)open(cur);return r})})}

function newProject(){var s=sheet('<header><h2>New project</h2><p>A project holds several channels: one per vendor, contractor or partner. Each of them sees only their own channel. Here, you and '+esc(agentName())+' see all of them and coordinate.</p></header>'+
  '<section><input id="np" placeholder="Kitchen renovation, Q4 vendor search…" autofocus></section>'+
  '<footer><span class="spacer"></span><button class="btn" data-x>Cancel</button><button class="btn solid" data-go>Create project</button></footer>');
  s.querySelector("[data-x]").onclick=closeSheet;var inp=s.querySelector("#np");setTimeout(function(){inp.focus()},30);
  var gogo=function(){var t=inp.value.trim();if(!t)return;api("/app/api/projects",{title:t}).then(function(d){if(d.error){alert(d.error);return}closeSheet();threads().then(function(){go("chan",d.id)})})};
  s.querySelector("[data-go]").onclick=gogo;inp.onkeydown=function(e){if(e.key==="Enter")gogo()}}
function newChooser(){var s=sheet('<header><h2>New</h2></header><section><div class="choices">'+
  '<button class="choice" data-k="c"><b># Channel</b><span>One subject, with '+esc(agentName())+' and anyone you invite.</span></button>'+
  '<button class="choice" data-k="p"><b>'+ICON.stack+' Project</b><span>Several channels, one per vendor or partner. They each see only theirs.</span></button></div></section>');
  s.querySelector('[data-k="c"]').onclick=function(){newThread()};s.querySelector('[data-k="p"]').onclick=newProject}

/* A schedule in one line: when, and what to think about. */
function scheduleSheet(id){id=id||cur;var zone="";try{zone=Intl.DateTimeFormat().resolvedOptions().timeZone||""}catch(e){}
  var opts=threadList.map(function(t){return '<option value="'+esc(t.id)+'"'+(t.id===id?" selected":"")+'>#'+esc(t.title)+'</option>'}).join("");
  var s=sheet('<header><h2>Schedule a review</h2><p>Once a day at this time, '+esc(agentName())+' stands back and looks at the whole channel with your question in mind, then writes down what it found. Reacting to what arrives is a different switch.</p></header><section>'+
    '<label class="f">Channel</label><select id="s-ch">'+opts+'</select>'+
    '<div class="grid2"><div><label class="f">Time</label><input id="s-at" type="time" value="07:30"></div><div><label class="f">Name</label><input id="s-name" placeholder="morning"></div></div>'+
    '<label class="f">What should it think about?</label><input id="s-q" placeholder="What changed, and what is now at risk?">'+
    '<div class="presets"><button class="btn sm" data-p="morning|07:30|What needs me today, and did anything slip?">Morning review</button><button class="btn sm" data-p="evening|21:30|Go back over today. Anything contradict what we agreed?">Evening reflection</button><button class="btn sm" data-p="friday|16:00|What should I know before the weekend?">End of day</button></div>'+
    '</section><footer><span class="spacer"></span><button class="btn" data-x>Cancel</button><button class="btn solid" id="s-go">Schedule it</button></footer>');
  s.querySelector("[data-x]").onclick=closeSheet;
  each("[data-p]",function(b){b.onclick=function(){var p=b.getAttribute("data-p").split("|");s.querySelector("#s-name").value=p[0];s.querySelector("#s-at").value=p[1];s.querySelector("#s-q").value=p[2]}},s);
  s.querySelector("#s-go").onclick=function(){var ch=s.querySelector("#s-ch").value;if(!ch)return;
    addRhythm(ch,{name:s.querySelector("#s-name").value.trim(),at:s.querySelector("#s-at").value,zone:zone,prompt:s.querySelector("#s-q").value.trim()}).then(function(r){if(r&&r.error){alert(r.error);return}closeSheet();
      if(view==="sched")scheduled();else if(view==="chan")open(cur);else today()})}}
function addRhythm(ch,rh){return api("/app/api/thread/"+encodeURIComponent(ch)+"/brief").then(function(d){var b=Object.assign({on_new_entry:"off"},d.brief||{});
  b.rhythms=(b.rhythms||[]).concat([rh]);return api("/app/api/thread/"+encodeURIComponent(ch)+"/brief",b)}).then(function(r){loadToday();threads();return r})}
function editRhythm(ch,idx,fn){return api("/app/api/thread/"+encodeURIComponent(ch)+"/brief").then(function(d){var b=Object.assign({on_new_entry:"off"},d.brief||{});
  b.rhythms=(b.rhythms||[]).slice();if(idx===-1){fn(b,null)}else{fn(b,b.rhythms[idx])}
  return api("/app/api/thread/"+encodeURIComponent(ch)+"/brief",b)})}

/* ---- Scheduled ---- */
function scheduled(){return Promise.all([loadToday(),threads()]).then(function(r){var d=r[0]||{};if(d.error)return;
  var list=d.schedules||[];var zone="";try{zone=Intl.DateTimeFormat().resolvedOptions().timeZone||""}catch(e){}
  var busiest=threadList.slice().sort(function(a,b){return (b.last||"")>(a.last||"")?1:-1})[0]||{};
  var opts=threadList.map(function(t){return '<option value="'+esc(t.id)+'"'+(t.id===busiest.id?" selected":"")+'>#'+esc(t.title)+'</option>'}).join("");
  var h='<div><span class="kick">Scheduled</span><h1 class="hello">When '+esc(agentName())+' <em>stops and thinks</em></h1><p class="lede">Each one is a time of day and a question. It looks at the whole channel, not just the last message, and writes down what it found.</p></div>';
  h+=threadList.length?'<div class="card addsched"><input id="a-q" placeholder="What should it think about? e.g. what changed, and what is now at risk?"><select id="a-ch">'+opts+'</select><input id="a-at" type="time" value="07:30"><button class="btn solid" id="a-go" style="height:40px">Add</button></div>':'<div class="card quiet">Create a channel first; a schedule belongs to one.</div>';
  h+='<div class="card stable"><div class="hd kick"><span>What</span><span>When</span><span></span><span></span></div>'+(list.length?list.map(function(s,i){
      var when=s.at?("Daily "+s.at):("Every "+String(s.every).replace(/0m0s$/,"").replace(/m0s$/,"m"));
      var name=s.at?(s.name?cap(s.name)+" review":"Review at "+s.at):"Regular check-in";
      return '<div class="srow'+(s.paused?' paused':'')+'"><div class="what"><b>'+esc(name)+'</b><span><a data-open="'+esc(s.thread)+'">#'+esc(s.title)+'</a>'+(s.prompt?' · “'+esc(s.prompt)+'”':'')+(s.paused?" · paused":"")+'</span></div><span class="when">'+esc(when)+'</span>'+
        (s.index>=0?'<button class="switch" role="switch" aria-checked="'+(!s.paused)+'" aria-label="On" data-tog="'+i+'"></button>':'<span></span>')+'<button class="x" aria-label="Remove" data-del="'+i+'">×</button></div>'}).join(""):'<p class="quiet">Nothing scheduled yet. A morning review on your busiest channel is a good first one.</p>')+'</div>';
  var w=d.watching||[];
  h+='<div class="trio"><div class="card"><b>When something arrives</b><span>'+(w.length?w.length+(w.length===1?" channel wakes ":" channels wake ")+esc(agentName())+" when someone writes: "+w.map(function(x){return "#"+esc(x.title)}).join(", ")+".":"No channel wakes it yet. Turn on “Works on its own” in a channel.")+'</span></div>'+
    '<div class="card"><b>At a time of day</b><span>Stands back and looks at the whole thing, the way you would over coffee.</span></div>'+
    '<div class="card"><b>Always bounded</b><span>Runs, tokens and fetches are capped per day. When it hits one it stops and says so.</span></div></div>';
  $("sched").innerHTML=h;
  each("[data-open]",function(b){b.onclick=function(){go("chan",b.getAttribute("data-open"))}},$("sched"));
  each("[data-tog]",function(b){b.onclick=function(){var s=list[+b.getAttribute("data-tog")];b.disabled=true;
    editRhythm(s.thread,s.index,function(br,rh){if(rh)rh.paused=!rh.paused}).then(function(r){if(r&&r.error)alert(r.error);scheduled()})}},$("sched"));
  each("[data-del]",function(b){b.onclick=function(){var s=list[+b.getAttribute("data-del")];
    if(!b.getAttribute("data-armed")){b.setAttribute("data-armed","1");b.textContent="?";b.title="Click again to remove";return}
    editRhythm(s.thread,s.index,function(br){if(s.index===-1)br.every="";else br.rhythms.splice(s.index,1)}).then(function(r){if(r&&r.error)alert(r.error);scheduled()})}},$("sched"));
  if($("a-go"))$("a-go").onclick=function(){var q=$("a-q").value.trim(),at=$("a-at").value,ch=$("a-ch").value;if(!at||!ch)return;
    $("a-go").disabled=true;addRhythm(ch,{name:"",at:at,zone:zone,prompt:q}).then(function(r){if(r&&r.error)alert(r.error);scheduled()})}})}

/* ---- jump anywhere ---- */
var palEl=null,palSel=0,palItems=[];
function openPal(){if(palEl)return;palEl=document.createElement("div");palEl.className="pal";
  palEl.innerHTML='<div class="box" role="dialog" aria-label="Jump to"><input id="pal-q" placeholder="Jump to a channel, or type a command…" autocomplete="off"><div class="res" id="pal-res" role="listbox"></div></div>';
  document.body.appendChild(palEl);palEl.onclick=function(e){if(e.target===palEl)closePal()};
  var q=$("pal-q");q.oninput=function(){palSel=0;drawPal()};q.onkeydown=function(e){
    if(e.key==="ArrowDown"){palSel=Math.min(palSel+1,palItems.length-1);drawPal();e.preventDefault()}
    else if(e.key==="ArrowUp"){palSel=Math.max(palSel-1,0);drawPal();e.preventDefault()}
    else if(e.key==="Enter"){var it=palItems[palSel];if(it){closePal();it.run()}e.preventDefault()}
    else if(e.key==="Escape")closePal()};
  drawPal();setTimeout(function(){q.focus()},10)}
function closePal(){if(palEl){palEl.remove();palEl=null}}
function drawPal(){var q=($("pal-q").value||"").toLowerCase().trim();
  var all=[{t:"Today",k:"view",run:function(){go("today")}},{t:"Scheduled",k:"view",run:function(){go("sched")}}]
    .concat(threadList.map(function(t){return {t:(t.is_project?"":"#")+t.title,k:t.waiting?"needs you":(t.is_project?"project":"channel"),run:function(){go("chan",t.id)}}}))
    .concat([{t:"New channel",k:"create",run:function(){newThread()}},{t:"New project",k:"create",run:newProject},{t:"Schedule a review",k:"create",run:function(){scheduleSheet()}},{t:"Name "+agentName(),k:"agent",run:nameSheet},{t:"Your team of agents",k:"agent",run:function(){teamSheet()}},{t:"Settings",k:"settings",run:settings}]);
  palItems=q?all.filter(function(x){return x.t.toLowerCase().indexOf(q)>=0}):all;
  $("pal-res").innerHTML=palItems.length?palItems.map(function(x,i){return '<button class="it" role="option" data-i="'+i+'" aria-selected="'+(i===palSel)+'">'+esc(x.t)+'<small>'+esc(x.k)+'</small></button>'}).join(""):'<p class="hint" style="padding:.6rem .75rem">Nothing matches.</p>';
  each(".it",function(b){b.onclick=function(){var it=palItems[+b.getAttribute("data-i")];closePal();it.run()}},$("pal-res"))}

/* ---- naming the agent ---- */
function nameSheet(){var a=agentInfo||{};var curName=agentName()==="your agent"?"":agentName();
  var glob=(a.reach&&a.reach.autonomy)||"ask";
  var s=sheet('<header><h2>Your agent</h2><p>Write @name in any channel and it answers. People in a shared channel see this name next to everything it writes.</p></header>'+
    '<section><label class="f" for="an">Name</label><input id="an" maxlength="32" placeholder="Juniper, Atlas, Friday…" value="'+esc(curName)+'">'+
    '<label class="f">When it has a choice to make</label><div class="choices">'+
    '<button class="choice" data-au="ask" aria-pressed="'+(glob!=="auto")+'"><b>Ask me first</b><span>It stops for your call and asks before changing anything in your connected services. Recommended.</span></button>'+
    '<button class="choice" data-au="auto" aria-pressed="'+(glob==="auto")+'"><b>'+ICON.bolt+' Full auto</b><span>It decides and acts, and tells you what it chose and why. Daily limits still apply, and it can never share or grant access.</span></button></div>'+
    '<p class="hint">Any channel can override this in its side panel.'+(a.problem?' '+esc(a.problem):' Runs on '+esc((a.reach&&a.reach.model)||a.model||"")+'.')+'</p></section>'+
    '<footer><button class="btn ghost" id="an-set">Settings</button><button class="btn ghost" id="an-team">Your team</button><span class="spacer"></span><button class="btn" data-x>Cancel</button><button class="btn solid" id="an-go">Save</button></footer>');
  s.querySelector("[data-x]").onclick=closeSheet;s.querySelector("#an-set").onclick=settings;s.querySelector("#an-team").onclick=function(){teamSheet()};var inp=s.querySelector("#an");setTimeout(function(){inp.focus()},30);
  var pick=glob;each("[data-au]",function(x){x.onclick=function(){pick=x.getAttribute("data-au");each("[data-au]",function(y){y.setAttribute("aria-pressed",String(y===x))},s)}},s);
  var save=function(){api("/app/api/agent/config",{name:inp.value,autonomy:pick}).then(function(r){if(r.error){alert(r.error);return}closeSheet();loadAgent().then(function(){drawMode();if(view==="chan")open(cur);else if(view==="today")today();else scheduled()})})};
  s.querySelector("#an-go").onclick=save;inp.onkeydown=function(e){if(e.key==="Enter")save()}}

/* The agent sheet: what it should do here on its own, and what it may reach. */
function agentSheet(){if(!cur)return;
  Promise.all([api("/app/api/thread/"+tid()+"/brief"),loadAgent()]).then(function(r){var d=r[0],a=r[1]||{},b=d.brief||{},reach=d.reach||{};var st=a.status||{};
    var tools=[];(reach.tools||[]).forEach(function(srv){(srv.tools||[]).forEach(function(t){tools.push({id:srv.name+"."+t,confirm:(srv.confirm||[]).indexOf(t)>=0})})});
    var s=sheet('<header><h2>What '+esc(agentName())+' does here</h2><p>Out of the box it only answers when you ask. Everything below is how it works without you: reacting when something arrives, and stopping to think at hours you choose. Everything it does is written into this channel for you to read.</p></header><section>'+
    '<label class="f" for="brief">Standing instructions</label><textarea id="brief" rows="4" placeholder="Example: When the other side posts a price, compare it with our position in [[Q3 payments migration]] and note the gap. Ask me before agreeing to anything.">'+esc(b.text||"")+'</textarea>'+
    '<div class="grid2"><div><label class="f">Act when a new message arrives</label><select id="b-on"><option value="off">No</option><option value="others">From other people</option><option value="all">From anyone, including me</option></select></div>'+
    '<div><label class="f">Also run on a schedule</label><select id="b-every"><option value="">No</option><option value="1h">Every hour</option><option value="6h">Every 6 hours</option><option value="24h">Once a day</option></select></div></div>'+
    '<label class="f">Schedules: when it stops and thinks</label>'+
    '<p class="hint" style="margin-top:0">Reacting to what arrives is not the same as standing back. Give it an hour and a question, like you would give yourself.</p>'+
    '<div id="rhythms"></div>'+
    '<div class="presets"><button class="btn sm" data-preset="morning">+ Morning review</button><button class="btn sm" data-preset="evening">+ Evening reflection</button><button class="btn sm" data-preset="blank">+ Another time</button></div>'+
    '<label class="f">On its own, it may also use</label>'+
    '<label class="check"><input type="checkbox" id="b-web"> The web, on these domains: <input id="b-dom" placeholder="*.sec.gov, docs.stripe.com" style="flex:1"></label>'+
    (reach.allow_domains&&reach.allow_domains.length?'<p class="hint">Always allowed (from settings): '+esc(reach.allow_domains.join(", "))+'</p>':'')+
    (tools.length?tools.map(function(t){return '<label class="check"><input type="checkbox" data-tool="'+esc(t.id)+'"> '+esc(t.id)+(t.confirm?' <span class="dim">(asks you first)</span>':'')+'</label>'}).join(""):'<p class="hint">Nothing connected yet. Settings, then Connections, adds one in about a minute.</p>')+
    '<p class="hint">When you ask it something yourself, it can use everything you have connected and any public page, and every fetch is recorded in the channel.</p>'+
    '<p class="hint" style="margin-top:.8rem">Last run '+esc(ago(st.last_run))+' · '+esc(String(st.runs_today||0))+' runs today'+(st.last_error?' · <span style="color:var(--red)">'+esc(st.last_error)+'</span>':'')+(a.problem?' · <span style="color:var(--gold-text)">'+esc(a.problem)+'</span>':'')+'</p>'+
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
      var rl=rhythms.filter(function(r){return r.at}).map(function(r){return {name:r.name||"",at:r.at,zone:r.zone||zone,prompt:r.prompt||"",paused:!!r.paused}});
      return {text:s.querySelector("#brief").value,on_new_entry:s.querySelector("#b-on").value,every:s.querySelector("#b-every").value,web:s.querySelector("#b-web").checked,allow_domains:doms,rhythms:rl,
        tools:Array.prototype.filter.call(s.querySelectorAll("[data-tool]"),function(c){return c.checked}).map(function(c){return c.getAttribute("data-tool")})}};
    s.querySelector("#b-save").onclick=function(){api("/app/api/thread/"+tid()+"/brief",read()).then(function(r){if(r.error){alert(r.error);return}closeSheet();open(cur);loadToday()})};
    s.querySelector("#b-run").onclick=function(){var btn=s.querySelector("#b-run");btn.disabled=true;btn.textContent="Running…";
      api("/app/api/thread/"+tid()+"/brief",read()).then(function(){return api("/app/api/thread/"+tid()+"/run")}).then(function(r){closeSheet();if(r.error)note(r.error,"bad");open(cur)})}})}

/* What this channel is tied to, and how it got tied. */
function linksSheet(){var l=window._links;if(!l)return;
  var row=function(x){return '<div class="row"><div class="t"><b>'+esc(x.title)+'</b><span>'+esc(x.why)+(x.count>1?" · "+x.count+" times":"")+'</span></div><button class="btn sm" data-go="'+esc(x.id)+'">Open</button></div>'};
  var block=function(title,items){return items.length?'<label class="f">'+title+'</label><div class="list">'+items.map(row).join("")+'</div>':''};
  var s=sheet('<header><h2>'+esc(l.title)+'</h2><p>What this channel is tied to. Links come from writing [[a channel title]] in a message, and from '+esc(agentName())+' actually opening another channel to answer something here.</p></header>'+
  '<section>'+block("This channel points at",l.out)+block("Pointed at by",l.in)+block("'+esc(AgentName())+' read, working here",l.read)+
  (l.total?'':'<p class="hint">Nothing yet. Write [[a channel title]] in a message to tie two together.</p>')+
  '</section><footer><span class="spacer"></span><button class="btn" data-x>Done</button></footer>');
  s.querySelector("[data-x]").onclick=closeSheet;
  Array.prototype.forEach.call(s.querySelectorAll("[data-go]"),function(b){b.onclick=function(){closeSheet();go("chan",b.getAttribute("data-go"))}})}

/* Delete: only a steward can, and only from this node. */
function moreSheet(){if(!cur)return;var t=(window._threads||[]).filter(function(x){return x.id===cur})[0]||{};
  var s=sheet('<header><h2>'+esc(t.title||"This channel")+'</h2><p>You own this channel. Deleting removes it and everything in it from your node and stops every link you shared. Anyone who already pulled a copy to their own node keeps theirs; the record is append-only between nodes.</p></header>'+
  '<section><p class="hint">'+esc(String(t.entries||0))+' messages'+(t.shared?' · shared with '+t.shared+(t.shared===1?' person':' people'):'')+'</p>'+
  (!t.is_project?'<label class="f" for="mv">Project</label><select id="mv"><option value="">Not in a project</option>'+threadList.filter(function(x){return x.is_project}).map(function(x){return '<option value="'+esc(x.id)+'"'+(t.project===x.id?" selected":"")+'>'+esc(x.title)+'</option>'}).join("")+'</select>'+
    '<p class="hint">In a project, '+esc(agentName())+' here sees only this channel, and the project sees it along with the others.</p>':'')+'</section>'+
  '<footer><button class="btn danger" id="del">Delete this channel</button><span class="spacer"></span><button class="btn" data-x>Cancel</button></footer>');
  s.querySelector("[data-x]").onclick=closeSheet;var d=s.querySelector("#del");
  var mv=s.querySelector("#mv");if(mv)mv.onchange=function(){mv.disabled=true;moveTo(cur,mv.value).then(function(){mv.disabled=false})};
  d.onclick=function(){if(!d.getAttribute("data-armed")){d.setAttribute("data-armed","1");d.textContent="Click again to delete for good";return}
    d.disabled=true;api("/app/api/thread/"+tid()+"/delete",{}).then(function(r){if(r.error){alert(r.error);d.disabled=false;return}closeSheet();cur=null;go("today")})}}

/* Share: one button, two choices, one link. */
function shareSheet(){if(!cur)return;var mode="summary";
  api("/app/api/thread/"+tid()+"/access").then(function(d){
    var s=sheet('<header><h2>Share #'+esc((threadOf(cur).title)||"this channel")+'</h2><p>Anyone with the link reads what you have shared, as dated updates. No account needed. Stop sharing any time.</p></header>'+
    '<section><div class="choices"><button class="choice" data-m="summary" aria-pressed="true"><b>A summary</b><span>'+esc(AgentName())+' drafts it, you edit it. That is all they see.</span></button>'+
    '<button class="choice" data-m="read" aria-pressed="false"><b>Everything</b><span>Your messages, your questions, and what '+esc(agentName())+' wrote.</span></button></div>'+
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
      hint.textContent=r.mode==="update"?"Drafted from the "+r.since+(r.since===1?" entry":" entries")+" since your last update. Edit anything before you share it.":"Drafted from the channel. Edit anything before you share it."})
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
  '<p class="hint">Any model OpenRouter serves that can call tools. Prices are live from OpenRouter; each run shows what it cost. Switching takes effect on the next question.</p>'+
  '<div class="grid2" style="margin-top:.5rem"><div><label class="f" style="margin-top:.3rem">OpenRouter key</label><input id="c-key" type="password" placeholder="'+(reach.has_key?(reach.key_from_env?"set in the environment":"saved on this machine"):"sk-or-…")+'"></div>'+
  '<div><label class="f" style="margin-top:.3rem">Or a local model server</label><input id="c-url" value="'+esc(reach.model_url||"")+'" placeholder="http://localhost:11434/v1"></div></div>'+
  '<label class="f">Key for that server, if it needs one</label><input id="c-urlkey" type="password" placeholder="'+(reach.has_url_key?"saved for this endpoint":"usually none — Ollama and vLLM need no key")+'">'+
  '<p class="hint">Credentials are kept on the node, readable only by it, and are never sent anywhere but the service they belong to. Your OpenRouter key goes to OpenRouter alone: point the agent at another server and that key stays behind.</p>'+
  '<button class="btn" id="c-modelsave" style="margin-top:.5rem">Save model settings</button>'+
  '<label class="f">Your agent</label><div style="display:flex;gap:.5rem"><input id="c-aname" maxlength="32" value="'+esc(agentName()==="your agent"?"":agentName())+'" placeholder="Give it a name, e.g. Juniper"><button class="btn" id="c-anamesave">Save</button></div>'+
  (a.problem?'<p class="hint" style="color:var(--gold-text)">'+esc(a.problem)+'</p>':'<p class="hint">Runs on '+esc(a.model||"")+'. Acting for you in '+esc(String(a.delegated_threads||0))+' channel'+(a.delegated_threads===1?"":"s")+'. Today: '+esc(String(st.runs_today||0))+' runs, '+esc(String(st.fetches_today||0))+' fetches, '+esc(String(st.tokens_today||0))+' tokens.'+(st.last_sync?' Synced with peers '+esc(ago(st.last_sync))+'.':'')+(st.last_sync_error?' <span style="color:var(--red)">Sync: '+esc(st.last_sync_error)+'</span>':'')+'</p>')+
  '<p class="hint">It has its own key, signed by yours, so anyone reading a channel can tell you from your agent. Anything it writes says so. It can never share or grant access.</p>'+
  '<p class="hint">Everything lives in <span class="mono">'+esc(reach.config_path||"").replace(/\/agent\.json$/,"")+'</span> on this machine. This page answers only on this machine; peers reach the node by signature, never by this token.</p>'+
  '<label class="f">The web, when nobody asked</label>'+
  '<select id="c-autoweb"><option value="listed">Only the sites I list below</option><option value="any">Any public page, same as when I ask</option><option value="off">None at all unless I ask</option></select>'+
  '<div id="c-domwrap" style="display:flex;gap:.5rem;margin-top:.5rem"><input id="c-dom" value="'+esc((reach.allow_domains||[]).join(", "))+'" placeholder="*.sec.gov, docs.stripe.com"><button class="btn" id="c-domsave">Save</button></div>'+
  '<p class="hint">When you ask it something yourself it may always fetch any public page, and every fetch is written into the channel. This is only about what it does while you are away. Private and local addresses are refused either way, and a channel can narrow this further but never widen it.</p>'+
  '<label class="f">Connections</label><p class="hint" style="margin-top:0">Anything that speaks MCP: your issue tracker, your calendar, your own service. Paste its address and press Connect. Lamdis names it, signs you in if the service wants that, and reads back what it offers. Anything that changes something asks you first until you say otherwise.</p>'+
  '<div class="addconn"><input id="conn-url" placeholder="https://mcp.example.com/mcp" autocomplete="off" spellcheck="false"><button class="btn solid" id="conn-add">Connect</button></div>'+
  '<div id="conn-add-status" class="hint"></div>'+
  '<div id="conns"></div>'+
  '<label class="f">Connect a machine</label><p class="hint" style="margin-top:0">Leave an agent running on your laptop or a server, and it takes direction from a channel here. Write from your phone, it happens there.</p>'+
  (cur?'<button class="btn" id="c-link">Connect this channel to a machine</button><div id="c-linkout"></div>'
      :'<p class="hint">Open a channel first, then come back: a machine is connected to one channel.</p>')+
  '<label class="f">Use with Claude</label><div class="cmd">claude mcp add lamdis -- lamdis mcp<button data-copy="claude mcp add lamdis -- lamdis mcp">copy</button></div>'+
  '<p class="hint">Run that once. Claude Code can then read your channels and write into them; its entries are labelled. Any other AI that speaks MCP works the same way.</p>'+
  (m.can_ask?'':'<p class="hint">Your agent is off. Three ways on: an OpenRouter key of your own (openrouter.ai/keys), a model on this machine (Ollama, vLLM), or <a href="mailto:support@lamdis.ai?subject=Lamdis%20key%20request" style="color:var(--gold-text)">ask us for a starter key</a> and we will send you one with a small fixed credit.</p>')+
  '<details><summary>Advanced: identity, other nodes, direct grants, revoke the agent</summary>'+
  '<label class="f">Your identity</label><div class="cmd">'+esc(m.principal)+'<button data-copy="'+esc(m.principal)+'">copy</button></div>'+
  '<label class="f">Your agent’s identity</label><div class="cmd">'+esc(a.principal||"")+'<button data-copy="'+esc(a.principal||"")+'">copy</button></div>'+
  '<label class="f">This node</label><div class="cmd">'+esc(origin)+'<button data-copy="'+esc(origin)+'">copy</button></div>'+
  '<p class="hint">Someone running their own Lamdis can pair with you: <span class="mono">lamdis peer add &lt;name&gt; '+esc(origin)+'</span></p>'+
  '<label class="f">Pair with another node</label><div style="display:grid;grid-template-columns:1fr 2fr auto;gap:.5rem"><input id="p-name" placeholder="Name"><input id="p-url" placeholder="https://their-node"><button class="btn" id="p-add">Pair</button></div><div id="p-status"></div>'+
  '<div class="list" id="peers"></div>'+
  (cur?'<label class="f">Grant a paired person or agent access to this channel</label><div style="display:grid;grid-template-columns:2fr 1fr auto;gap:.5rem"><input id="g-to" placeholder="Paired name or identity"><input id="g-scope" value="summary" placeholder="summary | read | contribute"><button class="btn" id="g-go">Grant</button></div><div id="g-status"></div><div class="list" id="grants"></div>':'')+
  '<label class="f">Revoke the agent</label><button class="btn danger" id="c-revoke">Revoke its key everywhere</button><p class="hint">Severs it in every channel it acted in and removes its key. Restarting the node mints a fresh one.</p>'+
  '</details></section><footer><span class="spacer"></span><button class="btn" data-x>Done</button></footer>');
  s.querySelector("[data-x]").onclick=closeSheet;
  Array.prototype.forEach.call(s.querySelectorAll("[data-copy]"),function(b){b.onclick=function(){copy(b.getAttribute("data-copy"));b.textContent="copied"}});
  s.querySelector("#c-save").onclick=function(){api("/app/api/me",{name:s.querySelector("#c-name").value}).then(loadMe)};
  s.querySelector("#c-anamesave").onclick=function(){var b=s.querySelector("#c-anamesave");api("/app/api/agent/config",{name:s.querySelector("#c-aname").value}).then(function(r){b.textContent=r.error?"Failed":"Saved";loadAgent().then(drawMode)})};
  var conns=[];
  // A connection shows what it is and one decision: how much of it the
  // agent may use. The per-tool list is still there for anyone who wants
  // it, but nobody has to open it to get a working, safe connection.
  function meta(c){
    var all=(c._tools||[]).length, ask=(c.confirm||[]).length, on=(c.allow||[]).length;
    if(c._failed)return "It did not answer. Press Check to try again.";
    if(c.needs_signin)return "Waiting on you to sign in.";
    if(c.disabled&&!all)return "Off.";
    if(c.disabled)return "Off. Your agent cannot use this.";
    if(!all&&!on)return "Nothing read back yet — press Check.";
    var bits=[on+" of "+(all||on)+(on===1?" thing":" things")+" allowed"];
    if(ask)bits.push(ask+" asks you first");
    return bits.join(" · ");
  }
  // Three named modes. "Everything" still makes the tools that change
  // something ask first, because that is what makes a connection safe to
  // leave running rather than one you have to sit and watch.
  function modeOf(c){
    if(c.disabled)return "off";
    var all=(c._tools||[]);
    if(!all.length)return (c.allow||[]).length?"all":"off";
    var on=(c.allow||[]).length;
    if(!on)return "off";
    var writes=all.filter(function(t){return t.writes}).length;
    if(on===all.length)return "all";
    if(on===all.length-writes)return "read";
    return "some";
  }
  function setMode(c,m){
    var all=(c._tools||[]);
    c.disabled=(m==="off");
    if(m==="off"){c.allow=[];c.confirm=[];return}
    if(m==="all"){
      c.allow=all.map(function(t){return t.name});
      c.confirm=all.filter(function(t){return t.writes}).map(function(t){return t.name});
      return}
    if(m==="read"){
      c.allow=all.filter(function(t){return !t.writes}).map(function(t){return t.name});
      c.confirm=[]}
  }
  function toolRows(c,i){
    return (c._tools||[]).map(function(t){
      var on=(c.allow||[]).indexOf(t.name)>=0, ask=(c.confirm||[]).indexOf(t.name)>=0;
      return '<div class="trow">'+
        '<label class="check" style="margin:0"><input type="checkbox" data-t="'+esc(t.name)+'" data-i="'+i+'"'+(on?" checked":"")+'></label>'+
        '<div class="tn"><b>'+esc(t.name)+'</b>'+(t.what?'<span>'+esc(t.what)+'</span>':'')+'</div>'+
        (t.writes?'<span class="tw">changes things</span>':'')+
        '<label class="check" style="margin:0;font-size:.72rem'+(on?'':';opacity:.4')+'"><input type="checkbox" data-a="'+esc(t.name)+'" data-i="'+i+'"'+(ask?" checked":"")+(on?"":" disabled")+'> ask first</label>'+
        '</div>'}).join("");
  }
  function connRow(c,i){
    var m=modeOf(c);
    return '<div class="conn" data-conn="'+i+'">'+
      '<div class="top"><b>'+esc(c.name||"New connection")+'</b>'+
      (c.signed_in?'<span class="pip">signed in</span>':'')+
      (c.needs_signin?'<button class="btn sm solid" data-signin="'+i+'">Sign in</button>':'')+
      '<button class="btn sm" data-test="'+i+'">Check</button>'+
      '<button class="btn sm danger" data-del="'+i+'">Remove</button></div>'+
      '<div class="addr">'+esc(c.url||c.command||"")+'</div>'+
      '<div class="sum">'+esc(meta(c))+'</div>'+
      '<div class="modes" role="group" aria-label="How much of this the agent may use">'+
        '<button data-m="all" data-i="'+i+'" aria-pressed="'+(m==="all")+'">Everything</button>'+
        '<button data-m="read" data-i="'+i+'" aria-pressed="'+(m==="read")+'">Read only</button>'+
        '<button data-m="off" data-i="'+i+'" aria-pressed="'+(m==="off")+'">Off</button>'+
      '</div>'+
      ((c._tools||[]).length?'<details class="each"'+(m==="some"?" open":"")+'><summary>Choose each one'+(m==="some"?" (you have)":"")+'</summary>'+toolRows(c,i)+'</details>':'')+
      (c.needs_auth&&canHold?'<input type="password" placeholder="'+(c.has_auth?"a credential is saved; type to replace it":"If this service uses a plain token, paste it here")+'" data-f="auth" data-i="'+i+'" style="margin-top:.5rem">':'')+
      '<div class="probe" data-probe="'+i+'"></div></div>';
  }
  function drawConns(){
    var el=s.querySelector("#conns");
    el.innerHTML=conns.length?conns.map(connRow).join(""):'<p class="hint">Nothing connected yet.</p>';
    Array.prototype.forEach.call(el.querySelectorAll("[data-f]"),function(n){n.oninput=function(){conns[+n.getAttribute("data-i")][n.getAttribute("data-f")]=n.value};
      n.onchange=function(){saveConn(+n.getAttribute("data-i"))}});
    Array.prototype.forEach.call(el.querySelectorAll("[data-test]"),function(b){b.onclick=function(){testConn(+b.getAttribute("data-test"))}});
    Array.prototype.forEach.call(el.querySelectorAll("[data-signin]"),function(b){b.onclick=function(){signInConn(+b.getAttribute("data-signin"))}});
    Array.prototype.forEach.call(el.querySelectorAll("[data-del]"),function(b){b.onclick=function(){var i=+b.getAttribute("data-del");
      var c=conns[i];conns.splice(i,1);drawConns();if(c.name&&!c._new)api("/app/api/tools/remove",{name:c.name})}});
    Array.prototype.forEach.call(el.querySelectorAll("[data-m]"),function(b){b.onclick=function(){
      var i=+b.getAttribute("data-i");setMode(conns[i],b.getAttribute("data-m"));drawConns();saveConn(i)}});
    Array.prototype.forEach.call(el.querySelectorAll("[data-t]"),function(cb){cb.onchange=function(){
      var i=+cb.getAttribute("data-i"),t=cb.getAttribute("data-t"),c=conns[i];
      c.allow=c.allow||[];c.confirm=c.confirm||[];
      var at=c.allow.indexOf(t);
      if(cb.checked){if(at<0)c.allow.push(t)}
      else{if(at>=0)c.allow.splice(at,1);var ac=c.confirm.indexOf(t);if(ac>=0)c.confirm.splice(ac,1)}
      drawConns();saveConn(i)}});
    Array.prototype.forEach.call(el.querySelectorAll("[data-a]"),function(cb){cb.onchange=function(){
      var i=+cb.getAttribute("data-i"),t=cb.getAttribute("data-a"),c=conns[i];
      c.confirm=c.confirm||[];var ac=c.confirm.indexOf(t);
      if(cb.checked){if(ac<0)c.confirm.push(t)}else if(ac>=0)c.confirm.splice(ac,1);
      drawConns();saveConn(i)}});
  }
  // Sign in the way the protocol actually specifies: ask the server who
  // guards it, introduce ourselves, and let the person approve in a window.
  // Nobody types a secret.
  function signInConn(i){
    var c=conns[i],p=s.querySelector('[data-probe="'+i+'"]');
    if(!c.name||!c.url){p.className="probe bad";p.textContent="Paste an address first.";return}
    p.className="probe";p.textContent="Asking the server how it wants to be signed in to…";
    saveConn(i);
    api("/app/api/tools/auth/start",{name:c.name,url:c.url}).then(function(r){
      if(r.error){p.className="probe bad";p.textContent=r.error;return}
      p.textContent="Approve it in the window that just opened.";
      var win=window.open(r.authorize,"lamdis-connect","width=520,height=680");
      if(!win){p.className="probe bad";p.textContent="Your browser blocked the window. Allow pop-ups for this site and try again.";return}
      var done=function(e){ if(e.origin!==location.origin||!e.data||e.data.lamdis!=="tools-auth")return;
        window.removeEventListener("message",done);
        // Signed in, so read back what it offers without being asked to.
        reload().then(function(){var j=indexOfName(c.name);if(j>=0)testConn(j)})};
      window.addEventListener("message",done)})}

  // testConn is the one round trip that fills everything in: the name, the
  // sign-in question, and the list of what the server offers.
  function testConn(i,quiet){
    var c=conns[i],p=s.querySelector('[data-probe="'+i+'"]');
    if(p&&!quiet){p.className="probe";p.textContent="Connecting…"}
    return api("/app/api/tools/probe",{name:c.name,url:c.url,auth:c.auth||"",header:c.header||""}).then(function(r){
      var q=s.querySelector('[data-probe="'+i+'"]');
      if(r.error){if(q){q.className="probe bad";q.textContent=r.error}c.needs_auth=true;c._failed=true;drawConns();return}
      if(r.needs_signin){c.needs_signin=true;c.needs_auth=true;
        if(q){q.className="probe";q.textContent="This service wants you to sign in. Press Sign in and approve it."}
        drawConns();return}
      c.needs_signin=false;c._failed=false;c._tools=r.tools||[];c.auth="";
      // First look at a connection: allow it all, and let the things that
      // change something ask first. That is the setting most people would
      // have picked, so nobody has to pick it.
      if(!(c.allow||[]).length&&!c._touched){c._touched=true;setMode(c,"all")}
      if(q){q.className="probe ok";q.textContent=""}
      drawConns();saveConn(i)})}
  function saveConn(i){
    var c=conns[i];if(!c||!c.name||!c.url)return;
    api("/app/api/tools",{name:c.name,url:c.url,auth:c.auth||"",header:c.header||"",
      allow:c.allow||[],confirm:c.confirm||[],disabled:!!c.disabled,known:c._tools||[]}).then(function(r){
      if(r.error){var p=s.querySelector('[data-probe="'+i+'"]');if(p){p.className="probe bad";p.textContent=r.error}return}
      c._new=false;c.auth="";c.has_auth=true})}
  function indexOfName(n){for(var i=0;i<conns.length;i++)if(conns[i].name===n)return i;return -1}
  var canHold=true;
  function reload(){
    return api("/app/api/tools").then(function(d){
      canHold=d.may_hold_secrets!==false;
      var was={};conns.forEach(function(c){was[c.name]=c});
      conns=(d.servers||[]).map(function(x){
        var old=was[x.name]||{};
        x._tools=(x.known&&x.known.length?x.known:old._tools)||[];
        x.needs_signin=!!x.wants_signin&&!x.signed_in;
        x.needs_auth=!!x.wants_signin||!!x.has_auth;
        x._touched=true;return x});
      drawConns()})}
  reload();
  // One field, one press. The address is the only thing we cannot work out.
  s.querySelector("#conn-add").onclick=function(){
    var f=s.querySelector("#conn-url"), st=s.querySelector("#conn-add-status");
    var u=(f.value||"").trim();
    if(!u){f.focus();return}
    if(!/^https?:\/\//.test(u))u="https://"+u;
    st.textContent="Looking at "+u+"…";
    api("/app/api/tools/probe",{name:"",url:u}).then(function(r){
      if(r.error&&!r.name){st.textContent=r.error;return}
      var c={name:r.name,url:u,allow:[],confirm:[],_new:true,_touched:false};
      if(r.needs_signin){c.needs_signin=true;c.needs_auth=true}
      else if(r.error){c.needs_auth=true;c._failed=true}
      else{c._tools=r.tools||[];setMode(c,"all");c._touched=true}
      conns.push(c);f.value="";
      st.textContent=r.needs_signin?"Added "+c.name+". Press Sign in on it to finish."
        :(r.error?"Added "+c.name+", but it would not answer: "+r.error
                 :"Added "+c.name+" with "+((c._tools||[]).length)+" things it can do.");
      drawConns();
      if(!r.needs_signin&&!r.error)saveConn(conns.length-1)})};

  var sel=s.querySelector("#c-model"),cust=s.querySelector("#c-model-custom");sel.onchange=function(){cust.hidden=sel.value!=="__custom";if(!cust.hidden)cust.focus()};
  s.querySelector("#c-modelsave").onclick=function(){var b=s.querySelector("#c-modelsave");var id=sel.value==="__custom"?cust.value.trim():sel.value;var body={model:id,model_url:s.querySelector("#c-url").value};var k=s.querySelector("#c-key").value.trim();if(k)body.openrouter_key=k;var uk=s.querySelector("#c-urlkey").value.trim();if(uk)body.model_url_key=uk;
    api("/app/api/agent/config",body).then(function(r){b.textContent=r.error?"Failed: "+r.error:"Saved";loadMe()})};
  var aw=s.querySelector("#c-autoweb");aw.value=reach.auto_web||"listed";
  function drawWeb(){s.querySelector("#c-domwrap").hidden=(aw.value!=="listed")}
  drawWeb();
  aw.onchange=function(){drawWeb();api("/app/api/agent/config",{auto_web:aw.value})};
  var lk=s.querySelector("#c-link");
  if(lk)lk.onclick=function(){lk.disabled=true;lk.textContent="Asking…";
    api("/app/api/link/offer",{thread:cur}).then(function(r){lk.disabled=false;lk.textContent="Connect this channel to a machine";
      var out=s.querySelector("#c-linkout");
      if(r.error){out.innerHTML='<div class="status bad">'+esc(r.error)+'</div>';return}
      out.innerHTML='<p class="hint" style="margin-top:.7rem">On the machine, with Lamdis installed, run this within fifteen minutes:</p>'+
        '<div class="cmd">'+esc(r.command)+'<button data-copy="'+esc(r.command)+'">copy</button></div>'+
        '<p class="hint">Then <span class="mono">lamdis agent</span> there. It works only in the directory you start it in, asks before reaching anywhere else, and writes everything it does into this channel.</p>';
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


function drawer(o){var sh=$("shell");if(sh)sh.classList.toggle("open",!!o)}
each("[data-menu]",function(b){b.onclick=function(){$("shell").classList.toggle("open")}});
$("scrim").onclick=function(){drawer(false)};
$("send").onclick=send;$("share").onclick=shareSheet;$("agentbtn").onclick=agentSheet;$("linksbtn").onclick=linksSheet;$("more").onclick=moreSheet;
$("new").onclick=newChooser;$("gear").onclick=settings;$("jump").onclick=openPal;$("agentcard").onclick=nameSheet;$("schedbtn").onclick=function(){scheduleSheet(cur)};
each(".navi[data-view]",function(b){b.onclick=function(){go(b.getAttribute("data-view"))}});
each(".seg button",function(b){b.onclick=function(){mode=b.getAttribute("data-mode");drawMode();drawWho();$("text").focus()}});
$("who").onchange=function(){target=this.value;drawWho();$("text").focus()};
each(".tabs button",function(b){b.onclick=function(){tab=b.getAttribute("data-tab");open(cur)}});
$("text").oninput=grow;
/* Enter sends in the chosen mode, Shift+Enter is a new line, and Command or
   Control Enter keeps it as a message without asking. What every chat box does. */
$("text").onkeydown=function(e){
  if(e.key!=="Enter"||e.isComposing)return;
  if(e.shiftKey)return;
  e.preventDefault();
  if(e.metaKey||e.ctrlKey){post();return}
  send();
};
document.addEventListener("keydown",function(e){
  if((e.metaKey||e.ctrlKey)&&(e.key==="k"||e.key==="K")){e.preventDefault();palEl?closePal():openPal();return}
  if(e.key==="Escape"){closeSheet();closePal();drawer(false)}});
/* Quietly keep up with what the agent and other people write, without
   redrawing (and replaying every animation) when nothing changed. */
function refresh(){if(sheetEl||busy||palEl||document.hidden)return;
  if(view==="chan"&&cur){api("/app/api/thread/"+tid()).then(function(d){if(d.error)return;var l=d.entries[d.entries.length-1];var sig=d.entries.length+":"+(l?l.id:"");
    if(sig!==window._sig){window._sig=sig;open(cur)}else threads()});return}
  api("/app/api/today").then(function(d){if(d.error)return;var sig=JSON.stringify([d.decisions.length,d.runs.length&&d.runs[0].id,d.schedules.length]);
    if(sig!==window._tsig){window._tsig=sig;if(view==="today")today();else if(view==="sched")scheduled()}else threads()})}
/* Arriving with something to do (the landing page's box sends ?q=): make it
   a channel and put the question to the agent straight away, so the first
   thing a new person sees is their own work being done. */
function arriving(){var q="";try{q=(new URLSearchParams(location.search).get("q")||"").trim().slice(0,2000)}catch(e){}
  if(!q)return false;history.replaceState(null,"",location.pathname+"#today");
  var title=q.replace(/\s+/g," ");if(title.length>48){title=title.slice(0,48);var sp=title.lastIndexOf(" ");if(sp>24)title=title.slice(0,sp);title+="…"}
  api("/app/api/threads",{title:title}).then(function(d){if(d.error){go("today");return}
    threads().then(function(){go("chan",d.id).then(function(){$("text").value=q;grow();
      if(me&&me.can_ask){mode="ask";drawMode();ask()}else{mode="note";drawMode();post()}})})});
  return true}
Promise.all([loadMe(),loadAgent().catch(function(){}),loadModels().catch(function(){}),loadToday().catch(function(){}),loadTeam().catch(function(){})]).then(function(){drawMode();if(!arriving())return fromHash()});
setInterval(refresh,15000);
`

// appShell is the application's frame, shared by the node's own page and the
// hosted one. extra goes at the foot of the rail (the hosted page puts the
// account there).
func appShell(extra string) string {
	return `<div class="shell" id="shell">
  <div class="scrim" id="scrim"></div>
  <aside class="rail">
    <div class="brand">` + railMark + `lamdis</div>
    <button class="jump" id="jump"><svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><circle cx="11" cy="11" r="7"/><path d="M20 20l-3.5-3.5"/></svg><span>Jump to…</span><kbd>⌘K</kbd></button>
    <nav class="navs">
      <button class="navi" data-view="today"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M2 12h2M20 12h2M5 5l1.5 1.5M17.5 17.5L19 19M5 19l1.5-1.5M17.5 6.5L19 5"/></svg>Today<span class="n" id="nav-today-n" hidden></span></button>
      <button class="navi" data-view="sched"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/></svg>Scheduled</button>
    </nav>
    <div class="sect"><span class="kick">Channels</span><button id="new" aria-label="New channel" title="New channel">+</button></div>
    <div class="chans" id="threads"></div>
    <button class="agentcard" id="agentcard" title="Name your agent"><span class="top"><span class="av bot" id="ag-av">A</span><span class="who2"><b id="ag-name">Your agent</b><span id="ag-state">…</span></span><span class="live" id="ag-live"></span></span><span class="spend" id="ag-spend"></span></button>
    <div class="me"><div class="av you" id="me-av">·</div><b id="me-name">you</b><button class="icon" id="gear" aria-label="Settings" title="Settings"><svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><circle cx="12" cy="12" r="3"/><path d="M19 12a7 7 0 00-.1-1.2l2-1.5-2-3.4-2.3.9a7 7 0 00-2-1.2L14 3h-4l-.6 2.6a7 7 0 00-2 1.2l-2.3-.9-2 3.4 2 1.5a7 7 0 000 2.4l-2 1.5 2 3.4 2.3-.9a7 7 0 002 1.2L10 21h4l.6-2.6a7 7 0 002-1.2l2.3.9 2-3.4-2-1.5c.07-.4.1-.8.1-1.2z"/></svg></button></div>
    ` + extra + `
  </aside>
  <main class="main">
    <section class="view" id="v-today"><div class="mbar"><button class="menu" data-menu aria-label="Channels">` + menuIcon + `</button><b>Today</b></div><div class="scroll"><div class="today" id="today"></div></div></section>
    <section class="view" id="v-sched" hidden><div class="mbar"><button class="menu" data-menu aria-label="Channels">` + menuIcon + `</button><b>Scheduled</b></div><div class="scroll"><div class="sched" id="sched"></div></div></section>
    <section class="view" id="v-chan" hidden>
      <header class="head"><button class="menu" data-menu aria-label="Channels">` + menuIcon + `</button><h1 id="title"></h1>
        <div class="tabs"><button data-tab="all" aria-pressed="true">Conversation</button><button data-tab="agent">Agent activity</button></div>
        <span class="spacer"></span><div class="avs" id="avs"></div>
        <button class="pill" id="agentbtn" hidden></button><button class="pill" id="linksbtn" hidden></button>
        <button class="btn solid sm" id="share" disabled>Share</button><button class="icon" id="more" aria-label="More" title="More" hidden>⋯</button></header>
      <div class="body2">
        <div class="convo">
          <div class="feed" id="feed"><div class="stream" id="stream"></div></div>
          <div class="composer"><div class="cbox">
            <textarea id="text" rows="1" placeholder="Ask your agent…"></textarea>
            <div class="crow">
              <div class="seg" role="group" aria-label="Send as"><button data-mode="ask" id="mode-ask" aria-pressed="true">Ask agent</button><button data-mode="note" aria-pressed="false">Message</button></div>
              <select class="who" id="who" hidden aria-label="Who to ask"></select>
              <button class="chip" id="schedbtn" title="Schedule a review">` + clockIcon + `<span class="lbl">Schedule</span></button>
              <span class="model" id="model"></span>
              <button class="send" id="send" disabled aria-label="Send"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" aria-hidden="true"><path d="M12 19V5M5 12l7-7 7 7"/></svg></button>
            </div>
          </div><div class="note" id="note"></div></div>
        </div>
        <aside class="panel" id="panel" aria-label="This channel"></aside>
      </div>
    </section>
  </main>
</div>`
}

const railMark = `<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke-width="1.6" stroke-linejoin="round" aria-hidden="true"><path d="M4 7l5-3 5 3v6l-5 3-5-3z" stroke="#E08A00"/><path d="M10 14l5-3 5 3v6l-5 3-5-3z" stroke="#4C6FF5"/></svg>`
const menuIcon = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M4 7h16M4 12h16M4 17h10"/></svg>`
const clockIcon = `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/></svg>`

// appHTML is the owner's view.
func appHTML(model string, canAsk bool) string {
	return `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover"><meta name="referrer" content="no-referrer"><meta name="color-scheme" content="light"><meta name="theme-color" content="#FFFAF3">
<title>Lamdis</title><style>` + appCSS + `</style></head><body>
` + appShell("") + `
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
<meta name="viewport" content="width=device-width,initial-scale=1"><meta name="referrer" content="no-referrer"><meta name="color-scheme" content="light">
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
	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="light">
<title>` + template.HTMLEscapeString(title) + `</title><style>` + appCSS + `</style></head>
<body><div class="notice"><h1>` + template.HTMLEscapeString(title) + `</h1><p>` + template.HTMLEscapeString(body) + `</p></div></body></html>`
}

var appNoToken = appNotice("This node is not yours to open",
	"The interface is authenticated by a token kept in the node's data directory. Run the node yourself and it opens the right address.")
