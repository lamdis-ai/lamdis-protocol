package api

// themeCSS is the exchange's design system, in one place.
//
// Every page includes it, so the palette and type scale exist once rather than
// being copied into four templates that then drift. The rules below are the
// ones worth stating:
//
// Colour carries exactly three meanings. Gold is money. Green is healthy or
// done. Amber is blocked or degraded. Nothing else borrows them, which is what
// makes a screen readable at a glance instead of decorative.
//
// Display type is a geometric sans set tight and heavy; monospace is reserved
// for anything a machine produced — codes, amounts, coordinates, identifiers.
// The split is meaningful rather than aesthetic: if it is mono, a computer
// wrote it.
const themeCSS = `
:root {
  --bg:      #070A0E;
  --rail:    #090D12;
  --panel:   #0D1218;
  --panel-2: #121A22;
  --ink:     #EDF1F5;
  --ink-2:   #93A4B3;
  --ink-3:   #5D6E7C;
  --rule:    #1B242E;
  --rule-2:  #2B3945;
  --gold:    #FFB627;
  --green:   #3FCF71;
  --warn:    #E8873D;
  --warn-bg: #180F08;
  --bad:     #E06C6C;

  --sans: system-ui, -apple-system, "Segoe UI Variable", "Segoe UI", Roboto, sans-serif;
  --mono: ui-monospace, "SF Mono", SFMono-Regular, Menlo, Consolas, monospace;

  color-scheme: dark;
}

* { box-sizing: border-box; }

/* A class that sets display beats the hidden attribute, so anything marked
   hidden stays on screen. This has to win. */
[hidden] { display: none !important; }
/* A licensed skill reads differently from a preference: claiming one is a
   statement about credentials, not about taste. */
.kind.lic[aria-pressed=true] { border-color: var(--gold); }
.kind.lic::after { content: " \00b7 licensed"; opacity: .5; font-size: .78em; }
.sw2 { background: none; border: 1px solid var(--rule-2); color: var(--ink);
  border-radius: 6px; padding: .3rem .7rem; font: inherit; font-size: .82rem;
  cursor: pointer; }
.sw2:hover { border-color: var(--gold); }

body {
  margin: 0;
  background: var(--bg);
  color: var(--ink);
  font: 400 15px/1.5 var(--sans);
  -webkit-font-smoothing: antialiased;
}

a { color: inherit; }
button, input, textarea { font: inherit; }

.mono { font-family: var(--mono); font-variant-numeric: tabular-nums; }

.label {
  font: 600 .62rem/1 var(--mono);
  letter-spacing: .15em;
  text-transform: uppercase;
  color: var(--ink-3);
}

/* --- top bar -------------------------------------------------------------- */

.top {
  display: flex; align-items: center; gap: 1rem;
  height: 3.1rem; padding: 0 1rem;
  border-bottom: 1px solid var(--rule); background: var(--rail);
  position: sticky; top: 0; z-index: 30;
}
.mark { font: 700 .95rem/1 var(--sans); letter-spacing: -.03em; text-decoration: none; }
.mark b { color: var(--gold); }
.top .right { margin-left: auto; display: flex; align-items: center; gap: 1rem; }
.top .right a { font-size: .84rem; color: var(--ink-2); text-decoration: none; }
.top .right a:hover { color: var(--ink); }
.health { display: flex; align-items: center; gap: .4rem; font-size: .8rem; color: var(--ink-2); }
.beacon {
  width: .42rem; height: .42rem; border-radius: 50%; background: var(--green);
  box-shadow: 0 0 0 0 rgba(63,207,113,.5); animation: ping 2.4s infinite;
}
.beacon.off { background: var(--ink-3); animation: none; }
@keyframes ping {
  70%  { box-shadow: 0 0 0 .3rem rgba(63,207,113,0); }
  100% { box-shadow: 0 0 0 0 rgba(63,207,113,0); }
}

/* --- shell ---------------------------------------------------------------- */

.shell { display: grid; grid-template-columns: 1fr; min-height: calc(100vh - 3.1rem); }
@media (min-width: 56rem) { .shell { grid-template-columns: 13.5rem 1fr; } }

.rail {
  border-right: 1px solid var(--rule); background: var(--rail);
  padding: 1rem .6rem; display: flex; flex-direction: column; gap: .1rem;
}
@media (max-width: 56rem) {
  .rail {
    flex-direction: row; overflow-x: auto;
    border-right: 0; border-bottom: 1px solid var(--rule);
  }
  .rail .grp { display: none; }
}
.rail .grp { padding: .9rem .65rem .4rem; }
.rail a {
  display: flex; align-items: center; gap: .55rem; width: 100%;
  padding: .5rem .65rem; border-radius: 3px;
  color: var(--ink-2); text-decoration: none; white-space: nowrap; font-size: .88rem;
}
.rail a:hover { background: var(--panel); color: var(--ink); }
.rail a[aria-current="page"] { background: var(--panel-2); color: var(--ink); }
.rail a:focus-visible { outline: 2px solid var(--gold); outline-offset: -2px; }
.rail .n {
  margin-left: auto; padding: .1rem .34rem; border-radius: 2px;
  background: var(--rule-2); color: var(--ink-2); font: 600 .66rem/1.5 var(--mono);
}
.rail .n.hot { background: #4A3410; color: var(--gold); }

.main { padding: 1.4rem 1.25rem 4rem; min-width: 0; }
@media (min-width: 56rem) { .main { padding: 1.6rem 2rem 4rem; } }

h1 { margin: 0 0 .3rem; font: 700 1.45rem/1.15 var(--sans); letter-spacing: -.03em; }
.lead { margin: 0 0 1.4rem; color: var(--ink-2); font-size: .92rem; }

/* Multi-part work, explained. A contractor arriving at the console found
   capacity sliders and an open board, and no answer to the question they
   actually have: what does a job too big for one visit look like here. */
.scope { border: 1px solid var(--rule); border-radius: 3px; margin: 0 0 1.4rem;
         overflow: hidden; }
.scope > .hd { display: flex; gap: .6rem; align-items: baseline;
               padding: .8rem .95rem; background: var(--panel);
               border-bottom: 1px solid var(--rule); }
.scope > .hd b { font-size: .92rem; }
.scope > .hd span { font-size: .78rem; color: var(--ink-3); margin-left: auto; }
.piece { display: flex; gap: .75rem; padding: .8rem .95rem;
         border-bottom: 1px solid var(--rule); align-items: flex-start; }
.piece:last-child { border-bottom: 0; }
.piece .num { width: 1.35rem; height: 1.35rem; flex: none; border-radius: 50%;
              background: var(--panel-2); color: var(--ink-2);
              font: 700 .72rem/1.35rem var(--mono); text-align: center; }
.piece.blocked .num { background: #3A2510; color: var(--warn); }
.piece .t { font-size: .88rem; }
.piece .s { font-size: .78rem; color: var(--ink-3); margin-top: .18rem; }
.piece .s.warn { color: var(--warn); }
.piece .amt { margin-left: auto; font: 500 .8rem/1 var(--mono); color: var(--ink-2);
              white-space: nowrap; }
.note-box { border: 1px solid var(--rule); border-left: 2px solid var(--gold);
            background: var(--panel); padding: .75rem .9rem; margin: 0 0 1.2rem;
            font-size: .84rem; color: var(--ink-2); border-radius: 0 3px 3px 0; }
.note-box b { color: var(--ink); }
/* What the job is, what proves it, and what the buyer could not pin down.
   None of this used to reach the board, which is why bidding was guesswork. */
.dv { font-size: .8rem; color: var(--ink-2); margin-top: .3rem; }
.bf { font-size: .8rem; color: var(--ink-3); margin-top: .3rem; font-style: italic;
      border-left: 2px solid var(--rule-2); padding-left: .55rem; }
.wh { font-size: .78rem; color: var(--warn); margin-top: .3rem; }
/* The queue, rebuilt. Summary first, state read from the stripe before any
   word is read, money right-aligned in tabular figures, and stages shown as
   what they are — money-weighted progress, not a schedule. */
.glance { display: grid; grid-template-columns: repeat(auto-fit, minmax(8.5rem, 1fr));
  gap: 1px; background: var(--rule); border: 1px solid var(--rule);
  margin: 0 0 1.25rem; border-radius: 3px; overflow: hidden; }
.glance .g { background: var(--panel); padding: .8rem .85rem; }
.glance dt { font: 600 .64rem/1 var(--sans); text-transform: uppercase;
  letter-spacing: .09em; color: var(--ink-3); margin-bottom: .4rem; }
.glance dd { margin: 0; font: 600 1.3rem/1 var(--mono); font-variant-numeric: tabular-nums; }
.glance dd small { font: 500 .7rem/1 var(--mono); color: var(--ink-3); margin-left: .3rem; }
.glance .money dd { color: var(--gold); }
.glance .wait dd { color: var(--warn); }

.tabs { display: flex; gap: .15rem; margin: 0 0 1rem; border-bottom: 1px solid var(--rule); }
.tabs .tab { appearance: none; background: none; border: 0;
  border-bottom: 2px solid transparent; color: var(--ink-3);
  font: 600 .85rem/1 var(--sans); padding: .55rem .75rem; cursor: pointer; }
.tabs .tab[aria-selected="true"] { color: var(--ink); border-bottom-color: var(--gold); }
.tabs .tab:hover { color: var(--ink-2); }
.tabs .tab:focus-visible { outline: 2px solid var(--gold); outline-offset: -2px; }
.tabs .tn { font: 500 .72rem/1 var(--mono); color: var(--ink-3); margin-left: .35rem; }

.job { border-bottom: 1px solid var(--rule); }
.job:last-child { border-bottom: 0; }
.jrow { display: flex; align-items: flex-start; gap: .9rem; width: 100%; text-align: left;
  padding: .8rem 1rem .8rem .75rem; background: none; color: inherit; font: inherit;
  border: 0; border-left: 3px solid var(--rule-2); cursor: pointer; }
.jrow:hover { background: var(--panel); }
.jrow:focus-visible { outline: 2px solid var(--gold); outline-offset: -2px; }
.job.go > .jrow { border-left-color: var(--green); }
.job.wait > .jrow { border-left-color: var(--warn); }
.jbody { flex: 1; min-width: 0; }
.jt { font-weight: 600; margin: 0 0 .2rem; line-height: 1.35; }
.job .amt { text-align: right; flex: none; min-width: 6.2rem; }
.job .amt .n { font: 600 1.02rem/1.2 var(--mono); font-variant-numeric: tabular-nums;
  color: var(--gold); }
.job .amt .n.quiet { color: var(--ink-2); }
.job .amt .s { font: 500 .68rem/1.4 var(--mono); color: var(--ink-3); margin-top: .12rem; }

/* Named stagebar, not rail. .rail is the navigation sidebar above, and the
   stage bar silently inherited its background, padding and border — so it
   rendered as an invisible gap. Two unrelated things with one class name is
   how a stylesheet quietly cancels itself. */
.stagebar { display: flex; gap: 2px; margin-top: .5rem; }
.stagebar .seg { height: 5px; border-radius: 1px; background: var(--panel-2); }
.stagebar .seg.paid { background: var(--green); }
.stagebar .seg.now { background: var(--gold); }
.stagekey { display: flex; gap: .6rem; flex-wrap: wrap; margin-top: .35rem;
  font: 500 .7rem/1.4 var(--mono); color: var(--ink-3); }
.stagekey b { color: var(--ink-3); font-weight: 500; }
.stagekey .paid b, .stagekey .paid { color: var(--green); }
.stagekey .now b, .stagekey .now { color: var(--gold); }

.jopen { display: none; padding: 0 1rem 1rem 1.75rem; }
.job.on .jopen { display: block; }
.job.on > .jrow { background: var(--panel); }
.jgrid { display: grid; grid-template-columns: repeat(auto-fit, minmax(14rem, 1fr));
  gap: 1rem; padding-top: .85rem; border-top: 1px solid var(--rule); }
.jgrid h4 { margin: 0 0 .45rem; font: 600 .66rem/1 var(--sans); text-transform: uppercase;
  letter-spacing: .09em; color: var(--ink-3); }
.fx { font-size: .86rem; color: var(--ink-2); margin: 0 0 .35rem; }
.fx b { color: var(--ink); font-weight: 600; }
.fn { font: 500 .72rem/1.5 var(--mono); color: var(--ink-3); margin: .5rem 0 0; }

/* A job this account cannot carry yet, greyed rather than hidden. */
.r.shut { opacity: .55; }
.r.shut .amt { color: var(--ink-3); }

/* What the buyer supplied so the work can be priced and the place found. */
.shots { display: flex; gap: .5rem; flex-wrap: wrap; margin-top: .55rem; }
.ref { margin: 0; width: 7rem; }
.ref img { display: block; width: 100%; aspect-ratio: 4/3; object-fit: cover;
  border: 1px solid var(--rule-2); border-radius: 3px; background: var(--panel); }
.ref.id img { border-color: var(--gold); }
.ref figcaption { font: 500 .68rem/1.35 var(--mono); color: var(--ink-3); margin-top: .25rem; }
.ref figcaption b { display: block; color: var(--gold); font-weight: 500; }

.unk { margin: .6rem 0 0; padding: .6rem .7rem; background: var(--bg);
       border: 1px solid var(--rule); border-radius: 3px; }
.unk-h { margin: 0 0 .5rem; font-size: .78rem; color: var(--gold); }
.unk-r { display: block; margin-bottom: .6rem; font-size: .8rem; }
.unk-r > span { display: block; color: var(--ink-2); }
.unk-r i { color: var(--ink-3); font-style: normal; }
.unk-n { color: var(--ink-3) !important; font-size: .75rem; margin-bottom: .25rem; }
.unk-r input[type=text] { width: 100%; margin-top: .25rem; }
.unk-f { display: flex; align-items: center; gap: .35rem; margin-top: .3rem;
         font-size: .74rem; color: var(--ink-3); }
.unk-f input { width: auto; margin: 0; }

pre.api { background: var(--bg); border: 1px solid var(--rule); border-radius: 3px;
          padding: .8rem .9rem; overflow-x: auto; margin: 0 0 1.2rem;
          font: 500 .76rem/1.6 var(--mono); color: var(--ink-2); }
pre.api b { color: var(--gold); font-weight: 500; }
h2 {
  margin: 1.8rem 0 .7rem; font: 600 .64rem/1 var(--mono);
  letter-spacing: .15em; text-transform: uppercase; color: var(--ink-3);
}

/* --- status strip --------------------------------------------------------- */

.strip {
  display: flex; align-items: center; gap: .6rem; flex-wrap: wrap;
  padding: .65rem .85rem; margin-bottom: 1.3rem; border-radius: 3px;
  border: 1px solid var(--rule-2); background: var(--panel);
  font-size: .87rem; color: var(--ink-2);
}
.strip b { color: var(--ink); }
.strip a { color: var(--gold); }
.strip .d { width: .42rem; height: .42rem; border-radius: 50%; background: var(--green); flex: none; }
.strip.warn { border-color: #3A2510; background: var(--warn-bg); color: #E7D7C4; }
.strip.warn .d { background: var(--warn); }

/* --- metrics -------------------------------------------------------------- */

.metrics {
  display: grid; gap: 1px; background: var(--rule);
  border: 1px solid var(--rule); border-radius: 3px; overflow: hidden;
  margin-bottom: 1.5rem; grid-template-columns: repeat(2, 1fr);
}
@media (min-width: 46rem) { .metrics { grid-template-columns: repeat(4, 1fr); } }
.metric { background: var(--bg); padding: .9rem .95rem; }
.metric dt {
  font: 600 .6rem/1 var(--mono); letter-spacing: .13em;
  text-transform: uppercase; color: var(--ink-3);
}
.metric dd {
  margin: .45rem 0 0; font: 700 1.35rem/1 var(--sans);
  letter-spacing: -.03em; font-variant-numeric: tabular-nums;
}
.metric dd small { font: 500 .72rem/1 var(--mono); color: var(--ink-3); margin-left: .2rem; }
.metric.money dd { color: var(--gold); }
.metric .trend { margin-top: .35rem; font: 500 .7rem/1 var(--mono); color: var(--ink-3); }

/* --- rows ----------------------------------------------------------------- */

.rows { border: 1px solid var(--rule); border-radius: 3px; overflow: hidden; }
.r {
  display: flex; align-items: center; gap: .9rem;
  padding: .8rem .95rem; background: var(--bg); border-bottom: 1px solid var(--rule);
}
.r:last-child { border-bottom: 0; }
.r:hover { background: var(--panel); }
.r .grow { min-width: 0; flex: 1; }
.r .t { font-size: .93rem; }
.r .m { margin-top: .22rem; font: 400 .78rem/1.35 var(--mono); color: var(--ink-3); }
.r .amt {
  flex: none; font: 600 .95rem/1 var(--mono);
  color: var(--gold); font-variant-numeric: tabular-nums;
}
.r .amt.none { color: var(--ink-3); }
.r .when {
  flex: none; font: 500 .76rem/1 var(--mono);
  color: var(--ink-3); width: 4rem; text-align: right;
}

.chip {
  display: inline-block; margin-right: .45rem; padding: .16rem .4rem;
  border-radius: 2px; border: 1px solid var(--rule-2);
  font: 600 .58rem/1 var(--mono); letter-spacing: .1em;
  text-transform: uppercase; color: var(--ink-3); vertical-align: .07em;
}
.chip.ok   { color: var(--green); border-color: #1C4530; }
.chip.hot  { color: var(--gold);  border-color: #4A3410; }
.chip.bad  { color: var(--bad);   border-color: #43201F; }
.chip.warn { color: var(--warn);  border-color: #3A2510; }

/* --- controls ------------------------------------------------------------- */

.btn {
  display: inline-flex; align-items: center; justify-content: center;
  height: 2.25rem; padding: 0 .9rem; border-radius: 3px; cursor: pointer;
  border: 1px solid var(--rule-2); background: none; color: var(--ink);
  font-size: .85rem; font-weight: 500; text-decoration: none; white-space: nowrap;
  transition: background .14s, border-color .14s;
}
.btn:hover:not(:disabled) { border-color: var(--ink-3); background: var(--panel); }
.btn:disabled { opacity: .45; cursor: not-allowed; }
.btn:focus-visible { outline: 2px solid var(--gold); outline-offset: 2px; }
.btn.go { background: var(--gold); border-color: var(--gold); color: #120C00; font-weight: 600; }
.btn.go:hover:not(:disabled) { background: #FFC24D; border-color: #FFC24D; }
.btn.sm { height: 1.95rem; padding: 0 .7rem; font-size: .8rem; }
.btn.wide { width: 100%; }

input[type="text"], input[type="email"], textarea {
  width: 100%; padding: .6rem .75rem;
  border: 1px solid var(--rule-2); border-radius: 3px;
  background: var(--panel); color: var(--ink); font-size: .92rem;
}
input::placeholder, textarea::placeholder { color: var(--ink-3); }
input:focus, textarea:focus { outline: none; border-color: var(--gold); }

.err { margin-top: .5rem; min-height: 1.1rem; font-size: .84rem; color: var(--bad); }
.err.ok { color: var(--green); }
.empty { padding: 2.5rem 1rem; text-align: center; color: var(--ink-3); font-size: .9rem; }
.note { margin: 1rem 0 0; font-size: .84rem; color: var(--ink-3); }

/* --- instrument panel ------------------------------------------------------
   The exchange is the console the landing page promises: a dark ground with a
   faint grid, glass panels over it, big tabular numbers, and one live canvas
   that shows where the work is. Blue is software (an observe job, an agent);
   gold is money and a do job; green is proven. Nothing else borrows them. */
:root { --blue: #5FB0FF; --glass: rgba(13, 18, 24, .72); --glass-2: rgba(18, 26, 34, .8); }
body::before {
  content: ""; position: fixed; inset: 0; z-index: -1; pointer-events: none;
  background:
    radial-gradient(120% 70% at 75% -10%, rgba(95,176,255,.10), transparent 60%),
    radial-gradient(80% 60% at 10% 110%, rgba(255,182,39,.07), transparent 60%),
    linear-gradient(rgba(43,57,69,.22) 1px, transparent 1px) 0 0 / 3.5rem 3.5rem,
    linear-gradient(90deg, rgba(43,57,69,.22) 1px, transparent 1px) 0 0 / 3.5rem 3.5rem,
    var(--bg);
  mask-image: radial-gradient(100% 100% at 50% 40%, #000 30%, transparent 100%);
  -webkit-mask-image: radial-gradient(100% 100% at 50% 40%, #000 30%, transparent 100%);
}
.top { background: rgba(9,13,18,.82); -webkit-backdrop-filter: blur(10px); backdrop-filter: blur(10px); }
.rail { background: rgba(9,13,18,.6); -webkit-backdrop-filter: blur(10px); backdrop-filter: blur(10px); }
.glass, .metrics, .rows, .glance, .scope, .strip, .note-box, .unk, pre.api {
  background: var(--glass); -webkit-backdrop-filter: blur(12px); backdrop-filter: blur(12px);
}
.metric, .glance .g, .r { background: transparent; }
.r:hover, .jrow:hover, .job.on > .jrow { background: rgba(18,26,34,.7); }
.glass { border: 1px solid var(--rule); border-radius: 6px; padding: 1.1rem 1.2rem; }
/* HUD: the four or five numbers that decide what to do next. */
.hud { display: grid; gap: .7rem; grid-template-columns: repeat(2, 1fr); margin: 0 0 1.4rem; }
@media (min-width: 46rem) { .hud { grid-template-columns: repeat(var(--cols, 4), 1fr); } }
.hud .t { position: relative; overflow: hidden; border: 1px solid var(--rule); border-radius: 6px;
  padding: .95rem 1.05rem; background: var(--glass); -webkit-backdrop-filter: blur(12px); backdrop-filter: blur(12px);
  box-shadow: inset 0 1px 0 rgba(255,255,255,.04); }
.hud .t::after { content: ""; position: absolute; left: 0; right: 0; top: 0; height: 2px;
  background: linear-gradient(90deg, var(--rule-2), transparent); }
.hud .t.money::after { background: linear-gradient(90deg, var(--gold), transparent); }
.hud .t.ok::after { background: linear-gradient(90deg, var(--green), transparent); }
.hud .t.wait::after { background: linear-gradient(90deg, var(--warn), transparent); }
.hud .t.soft::after { background: linear-gradient(90deg, var(--blue), transparent); }
.hud .k { font: 600 .6rem/1 var(--mono); letter-spacing: .15em; text-transform: uppercase; color: var(--ink-3); }
.hud .v { margin-top: .5rem; font: 700 1.6rem/1 var(--sans); letter-spacing: -.03em; font-variant-numeric: tabular-nums; }
.hud .v small { font: 500 .7rem/1 var(--mono); color: var(--ink-3); margin-left: .25rem; letter-spacing: 0; }
.hud .t.money .v { color: var(--gold); }
.hud .t.ok .v { color: var(--green); }
.hud .t.wait .v { color: var(--warn); }
.hud .t.soft .v { color: var(--blue); }
.hud .s { margin-top: .4rem; font: 500 .7rem/1.4 var(--mono); color: var(--ink-3); }
/* Kind chips: blue for finding out, gold for making it true. */
.chip.obs { color: var(--blue); border-color: #1F3C5A; }
.chip.do  { color: var(--gold); border-color: #4A3410; }
/* The live canvas: where the work is, from where you stand. */
.radar-wrap { position: relative; border: 1px solid var(--rule); border-radius: 6px; overflow: hidden;
  background: var(--glass); -webkit-backdrop-filter: blur(12px); backdrop-filter: blur(12px); margin: 0 0 1.4rem; }
.radar-wrap canvas { display: block; width: 100%; height: 15rem; }
.radar-wrap .cap { position: absolute; left: .9rem; top: .75rem; font: 600 .6rem/1 var(--mono);
  letter-spacing: .15em; text-transform: uppercase; color: var(--ink-3); display: flex; gap: .6rem; align-items: center; }
.radar-wrap .cap .beacon { width: .38rem; height: .38rem; }
.radar-wrap .legend { position: absolute; right: .9rem; bottom: .7rem; display: flex; gap: .9rem;
  font: 500 .62rem/1 var(--mono); letter-spacing: .1em; text-transform: uppercase; color: var(--ink-3); }
.radar-wrap .legend i { display: inline-block; width: .5rem; height: .5rem; border-radius: 50%; margin-right: .35rem; vertical-align: -.02em; }
.radar-wrap .legend .obs i { background: var(--blue); box-shadow: 0 0 6px var(--blue); }
.radar-wrap .legend .do i { background: var(--gold); box-shadow: 0 0 6px var(--gold); }
.radar-wrap .legend .you i { background: var(--green); box-shadow: 0 0 6px var(--green); }
.radar-wrap .hint { position: absolute; left: .9rem; bottom: .7rem; font: 500 .66rem/1.4 var(--mono); color: var(--ink-3); max-width: 60%; }
/* Arrival: rows and panels rise into place, staggered. */
.rv { opacity: 0; transform: translateY(10px); animation: rise .5s cubic-bezier(.2,.7,.2,1) forwards; animation-delay: calc(var(--i, 0) * 45ms); }
@keyframes rise { to { opacity: 1; transform: none; } }
.btn.go { box-shadow: 0 0 0 0 rgba(255,182,39,.0); transition: box-shadow .2s, background .14s; }
.btn.go:hover:not(:disabled) { box-shadow: 0 0 18px rgba(255,182,39,.35); }
.pill { display: inline-flex; align-items: center; gap: .4rem; padding: .3rem .6rem; border-radius: 999px;
  border: 1px solid var(--rule-2); font: 600 .62rem/1 var(--mono); letter-spacing: .12em; text-transform: uppercase; color: var(--ink-2); }
.pill .beacon { width: .38rem; height: .38rem; }
.mode { display: inline-flex; border: 1px solid var(--rule-2); border-radius: 999px; padding: 2px; gap: 2px; }
.mode button { appearance: none; border: 0; background: none; color: var(--ink-3); border-radius: 999px;
  padding: .35rem .8rem; font: 600 .72rem/1 var(--mono); letter-spacing: .12em; text-transform: uppercase; cursor: pointer; }
.mode button[aria-pressed="true"] { background: var(--gold); color: #120C00; }
@media (prefers-reduced-motion: reduce) {
  * { animation: none !important; transition: none !important; }
  .rv { opacity: 1; transform: none; }
}

/* --- masthead, rail and page furniture (queue, job, sign-in) ---------------
   The top bar carries one live reading — how much work is open and what it
   adds up to — and the rail's counts glow when they are non-zero. Eyebrows
   are the mono uppercase captions the landing page uses over every panel. */
.top { height: 3.4rem; gap: 1.1rem; }
.top .mark { display: flex; align-items: center; gap: .5rem; font-size: 1rem; }
.top .mark i { width: .5rem; height: .5rem; border-radius: 1px; background: var(--gold);
  box-shadow: 0 0 10px rgba(255,182,39,.55); transform: rotate(45deg); }
.top .live { gap: .45rem; color: var(--ink-3); white-space: nowrap; }
.top .live .n { color: var(--ink); }
.top .live b { color: var(--gold); font-weight: 600; }
.top .live .sep { color: var(--rule-2); }
.top .live .beacon { animation-duration: 1.8s; }
.top .right .auth { padding: .38rem .7rem; border: 1px solid var(--rule-2); border-radius: 999px;
  font: 600 .64rem/1 var(--mono); letter-spacing: .12em; text-transform: uppercase; color: var(--ink-2); }
.top .right .auth:hover { border-color: var(--gold); color: var(--gold); }
@media (max-width: 46rem) { .top .live { display: none; } .top .health { display: none; } }
.rail .n:empty { display: none; }
.rail .n.hot { box-shadow: 0 0 10px rgba(255,182,39,.3); }
.rail .n.soft { background: #14283B; color: var(--blue); box-shadow: 0 0 10px rgba(95,176,255,.25); }
.eyebrow { margin: 0 0 .45rem; font: 600 .62rem/1 var(--mono); letter-spacing: .18em;
  text-transform: uppercase; color: var(--gold); }
.eyebrow.blue { color: var(--blue); }
.eyebrow.dim { color: var(--ink-3); }
.hud .v.flash { animation: flash .9s ease-out; }
@keyframes flash { 0% { text-shadow: 0 0 18px currentColor; } 100% { text-shadow: 0 0 0 transparent; } }
.pill.quiet { color: var(--ink-3); border-style: dashed; }
.pill.ok { color: var(--green); border-color: #1C4530; }
.pill.warn { color: var(--warn); border-color: #3A2510; }
`

// shellTop renders the masthead and left rail.
//
// Taking it as a function rather than a constant keeps the current page
// highlighted without every template hand-writing the same nav and getting the
// aria-current wrong.
func shellTop(current, status string) string {
	nav := func(href, label, id string) string {
		cur := ""
		if id == current {
			cur = ` aria-current="page"`
		}
		return `<a href="` + href + `"` + cur + `>` + label + `</a>`
	}
	beacon := `<span class="beacon" id="h-beacon"></span>`
	if status == "" {
		beacon = `<span class="beacon off" id="h-beacon"></span>`
		status = "Not signed in"
	}
	// The live pill is filled by whichever page includes it: the board and the
	// job page both read /v1/board and write the count and the sum here.
	return `<header class="top">
  <a class="mark" href="/board"><i></i><span>lamdis<b>.</b></span></a>
  <span class="pill live" id="live"><span class="beacon" id="live-beacon"></span>Live` +
		`<span class="sep">&middot;</span><span class="n" id="live-n">&ndash;</span>&nbsp;open` +
		`<span class="sep">&middot;</span><b id="live-sum">&ndash;</b>&nbsp;on the board</span>
  <div class="right">
    <span class="health">` + beacon + `<span id="h-text">` + status + `</span></span>
    <a class="auth" id="h-auth" href="/signin">Sign in</a>
  </div>
</header>
<div class="shell">
  <nav class="rail">
    <span class="label grp">Work</span>
    ` + nav("/board", `Queue <span class="n hot" id="n-queue"></span>`, "queue") + `
    ` + nav("/board#holding", `In flight <span class="n" id="n-flight"></span>`, "flight") + `
    <span class="label grp">Operation</span>
    ` + nav("/console", "Earnings", "earnings") + `
    ` + nav("/console#capacity", "Capacity", "capacity") + `
    ` + nav("/console#sec-buyer", "Buyer", "buyer") + `
    <span class="label grp">About</span>
    ` + nav("/how-it-works", "How this works", "trust") + `
    ` + nav("/docs", "API", "docs") + `
  </nav>
  <main class="main">`
}

const shellBottom = `  </main>
</div>`
