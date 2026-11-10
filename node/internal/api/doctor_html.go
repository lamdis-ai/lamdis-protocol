package api

import (
	"html"
	"strings"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/cases"
)

// doctorHTML is the page a doctor's office opens from the patient's link:
// what was denied and by when, a draft letter to edit and sign, and a place
// to upload the signed letter and records. It shows the least it can.
func doctorHTML(c *cases.Case, path string) string {
	esc := html.EscapeString
	val := func(f *cases.Fact) string {
		if f == nil || !f.Verified {
			return ""
		}
		return f.Value
	}
	var rows strings.Builder
	row := func(k, v string) {
		if v != "" {
			rows.WriteString("<tr><th>" + esc(k) + "</th><td>" + esc(v) + "</td></tr>")
		}
	}
	row("Patient", val(c.Facts.Patient))
	row("Insurer", val(c.Facts.Insurer))
	row("Member ID", val(c.Facts.MemberID))
	row("Claim / reference", val(c.Facts.ClaimNumber))
	row("What was denied", val(c.Facts.Service))
	row("Reason given", val(c.Facts.Reason))
	row("Type of denial", cases.CategoryLabel(c.Denial.Category))
	if d := c.NextDeadline; d != nil && d.Kind == "appeal" {
		row("Patient's appeal deadline", cases.HumanDate(d.Date))
	}
	draft := `<p class="muted">A draft letter of medical necessity is being prepared from the patient's documents. Refresh this page in a minute.</p>`
	if c.Doctor.LMN != nil {
		var b strings.Builder
		for _, s := range c.Doctor.LMN.Sentences {
			b.WriteString("<p>" + esc(s.Text) + "</p>")
		}
		draft = `<p class="muted">Prepared from the patient's documents. Review it, correct it, fill in every [bracketed] item, and sign it on your letterhead.</p>
<div class="draft" id="draft">` + b.String() + `</div>
<div class="row"><a class="btn ghost" href="` + esc(path) + `/lmn.html" target="_blank" rel="noopener">Open printable draft</a><button class="btn ghost" id="copy">Copy text</button></div>`
	}
	uploaded := ""
	if c.Doctor.Uploaded {
		uploaded = `<p class="ok">A letter has already been added. You can add more pages if you need to.</p>`
	}
	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="robots" content="noindex,nofollow"><meta name="referrer" content="no-referrer">
<title>Letter of medical necessity · Lamdis</title>
<link rel="icon" href="/favicon.svg">
<style>
:root{--bg:#f6f5f1;--card:#fff;--ink:#15181d;--muted:#5a6270;--line:#e2dfd6;--accent:#1f4fd1;--ok:#1d7349}
@media (prefers-color-scheme:dark){:root{--bg:#121417;--card:#1b1e23;--ink:#eef0f3;--muted:#a3abb8;--line:#2c3038;--accent:#7aa2ff;--ok:#5fc98f}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--ink);font:17px/1.55 -apple-system,BlinkMacSystemFont,"Segoe UI",system-ui,sans-serif}
main{max-width:760px;margin:0 auto;padding:28px 16px 64px}
h1{font-size:26px;line-height:1.2;margin:8px 0 6px}h2{font-size:19px;margin:0 0 10px}
.card{background:var(--card);border:1px solid var(--line);border-radius:14px;padding:20px;margin:16px 0}
.muted{color:var(--muted)}.ok{color:var(--ok);font-weight:600}
table{width:100%;border-collapse:collapse}th,td{text-align:left;padding:8px 0;border-bottom:1px solid var(--line);vertical-align:top}th{width:40%;color:var(--muted);font-weight:500;padding-right:12px}
.draft{border:1px solid var(--line);border-radius:10px;padding:16px;max-height:420px;overflow:auto;font-family:Georgia,serif;font-size:16px}
.draft p{margin:0 0 12px}
.row{display:flex;gap:10px;flex-wrap:wrap;margin-top:12px}
.btn{appearance:none;border:0;border-radius:10px;padding:12px 18px;font:600 16px/1 inherit;background:var(--accent);color:#fff;cursor:pointer;text-decoration:none;display:inline-block}
.btn.ghost{background:transparent;color:var(--accent);border:1px solid var(--line)}
.drop{border:2px dashed var(--line);border-radius:12px;padding:22px;text-align:center}
.brand{display:flex;align-items:center;gap:8px;font-weight:700;color:var(--ink);text-decoration:none}
small{color:var(--muted)}
</style></head><body><main>
<a class="brand" href="https://lamdis.ai" rel="noopener">` + markSVG + `Lamdis</a>
<h1>Your patient is appealing an insurance denial</h1>
<p class="muted">Your patient sent you this link so you can add a letter of medical necessity to their appeal. A letter from the treating physician is often what turns a denial around.</p>
<div class="card"><h2>The denial</h2><table>` + rows.String() + `</table></div>
<div class="card"><h2>Draft letter for you to edit and sign</h2>` + draft + `</div>
<div class="card"><h2>Add your signed letter</h2>` + uploaded + `
<p class="muted">Upload the signed letter (PDF or a clear photo). You can add supporting records too. Files go only to your patient's case.</p>
<div class="drop"><input type="file" id="f" accept="application/pdf,image/*" multiple><div class="row" style="justify-content:center"><button class="btn" id="up">Upload</button></div><p id="msg" class="muted" role="status"></p></div>
</div>
<p><small>Lamdis helps patients organize insurance appeals. It does not give medical or legal advice. This link works only for this one case; the patient can remove it at any time.</small></p>
</main>
<script>
(function(){
var up=document.getElementById("up"),f=document.getElementById("f"),msg=document.getElementById("msg");
var cp=document.getElementById("copy");
if(cp)cp.onclick=function(){var t=document.getElementById("draft").innerText;navigator.clipboard.writeText(t).then(function(){cp.textContent="Copied"})};
up.onclick=function(){
 if(!f.files.length){msg.textContent="Choose a file first.";return}
 up.disabled=true;var files=Array.prototype.slice.call(f.files),done=0;
 function next(){
  if(!files.length){msg.className="ok";msg.textContent="Thank you. "+done+" file"+(done==1?"":"s")+" added to your patient's appeal.";up.disabled=false;f.value="";return}
  var file=files.shift();msg.textContent="Uploading "+file.name+"…";
  fetch(location.pathname.replace(/\/$/,"")+"/upload?name="+encodeURIComponent(file.name),{method:"POST",body:file})
  .then(function(r){return r.json()}).then(function(j){if(j.error){msg.textContent=j.error;up.disabled=false;return}done++;next()})
  .catch(function(){msg.textContent="That didn't upload. Check your connection and try again.";up.disabled=false});
 }
 next();
};
})();
</script></body></html>`
}

// markSVG is the Lamdis mark, small.
const markSVG = `<svg width="22" height="22" viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="3" width="7" height="7" rx="1.5" fill="currentColor"/><rect x="3" y="11" width="7" height="7" rx="1.5" fill="currentColor" opacity=".8"/><rect x="11" y="11" width="7" height="7" rx="1.5" fill="currentColor" opacity=".6"/><rect x="3" y="19" width="15" height="2.5" rx="1.25" fill="currentColor" opacity=".5"/></svg>`
