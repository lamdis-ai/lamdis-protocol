package api

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// The console used to be one page with a rail of anchors, and two of the
// anchors went nowhere: the shell's "Capacity" pointed at #standing and the
// capacity pane at #supplier, neither of which anything defined. A link to
// nowhere teaches the reader the page is broken. Now every rail entry is a
// route, so the check is stronger: every console link the shell renders must
// be served, and every page must render under the shell with its side
// switch and the rail entry for itself marked current.
func TestEveryRailLinkIsAConsolePageThatRenders(t *testing.T) {
	mux := http.NewServeMux()
	(&Console{}).Register(mux)

	shell := shellTop("queue", "")
	for _, must := range []string{`id="rail-operator"`, `id="rail-buyer"`} {
		if !strings.Contains(shell, must) {
			t.Errorf("the shell has no %s", must)
		}
	}
	if strings.Contains(shell, `href="/console#`) || strings.Contains(shell, "#standing") {
		t.Error("the shell still links to console anchors; those are pages now")
	}
	links := regexp.MustCompile(`href="(/console[^"#?]*)"`).FindAllStringSubmatch(shell, -1)
	if len(links) < 12 {
		t.Fatalf("the shell links to %d console pages; expected both sides listed", len(links))
	}
	for _, m := range links {
		path := m[1]
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != http.StatusOK {
			t.Errorf("the rail links to %s, which answers %d", path, w.Code)
			continue
		}
		page := w.Body.String()
		for _, must := range []string{
			`id="mode-operator"`, `id="mode-buyer"`, // the side switch
			`href="` + path + `" aria-current="page"`, // and this page is current in its own rail
			`id="body"`, `var PAGE = "`, `boot(`,
		} {
			if !strings.Contains(page, must) {
				t.Errorf("%s does not render %s", path, must)
			}
		}
	}

	// Every page: no in-page anchor to nothing, and no link to a console
	// route that is not registered.
	for id, page := range consolePages {
		ids := map[string]bool{}
		for _, m := range regexp.MustCompile(`id="([\w-]+)"`).FindAllStringSubmatch(page, -1) {
			ids[m[1]] = true
		}
		for _, re := range []string{`href="#([\w-]+)"`, `href=\\"#([\w-]+)\\"`} {
			for _, m := range regexp.MustCompile(re).FindAllStringSubmatch(page, -1) {
				if !ids[m[1]] {
					t.Errorf("/console/%s links to #%s, which nothing on the page defines", id, m[1])
				}
			}
		}
		for _, m := range regexp.MustCompile(`href="/console/([\w-]+)"`).FindAllStringSubmatch(page, -1) {
			if _, ok := consolePages[m[1]]; !ok {
				t.Errorf("/console/%s links to /console/%s, which is not a page", id, m[1])
			}
		}
	}

	// A section that does not exist is a 404 in the shell, not a bare one.
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/console/nope", nil))
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), `class="rail"`) {
		t.Errorf("/console/nope answered %d without the shell", w.Code)
	}
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/console/earnings/extra", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("/console/earnings/extra answered %d, wanted 404", w.Code)
	}
}

// Every anchor the old page had is still a working address: the ones other
// pages and mail name are redirected client-side to the page they became,
// and the payment provider's return URLs are redirected server-side.
func TestOldConsoleAnchorsStillLand(t *testing.T) {
	for anchor, page := range map[string]string{
		"capacity": "/console/capacity", "standing": "/console/capacity",
		"integration": "/console/keys", "larger": "/console/larger",
		"sec-buyer": "/console/buy", "supplier": "/console/business",
		"earnings": "/console/earnings", "payout": "/console/earnings",
		"flight": "/console/work", "funds": "/console/funds",
		"spending": "/console/spending", "post": "/console/buy",
	} {
		if !strings.Contains(consoleJS, `"`+anchor+`": "`+page+`"`) {
			t.Errorf("#%s no longer redirects to %s", anchor, page)
		}
	}
	mux := http.NewServeMux()
	(&Console{}).Register(mux)
	for from, to := range map[string]string{
		"/console?payout=returned":               "/console/earnings?payout=returned",
		"/console?topup=done&session=cs_1":       "/console/funds?topup=done&session=cs_1",
		"/console?topup=cancelled":               "/console/funds?topup=cancelled",
		"/console/earnings?payout=returned":      "",
		"/console/funds?topup=done&session=cs_1": "",
	} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", from, nil))
		if to == "" {
			if w.Code != http.StatusOK {
				t.Errorf("%s answered %d", from, w.Code)
			}
			continue
		}
		if w.Code != http.StatusSeeOther || w.Header().Get("Location") != to {
			t.Errorf("%s answered %d %q, wanted a redirect to %s", from, w.Code, w.Header().Get("Location"), to)
		}
	}
	// The pages that handle the return read the query they are sent.
	if !strings.Contains(consolePages["funds"], `"topup"`) || !strings.Contains(consolePages["funds"], `"/v1/balance/confirm`) {
		t.Error("the funds page does not confirm a top-up on return")
	}
	if !strings.Contains(consolePages["earnings"], `"payout"`) {
		t.Error("the earnings page does not handle the payout return")
	}
}

// Each page fetches what it shows and nothing else. The one-page console read
// twelve routes to show any one of them.
func TestConsolePagesFetchOnlyWhatTheyShow(t *testing.T) {
	for id, forbidden := range map[string][]string{
		"work":     {`"/v1/spend"`, `"/v1/statement"`, `"/v1/capacity"`},
		"buy":      {`"/v1/payout"`, `"/v1/workers/holdings"`, `"/v1/statement"`},
		"spending": {`"/v1/payout"`, `"/v1/capacity"`, `"/v1/agent-keys"`},
		"keys":     {`"/v1/spend"`, `"/v1/payout"`},
		"larger":   {`"/v1/spend"`, `"/v1/payout"`, `"/v1/capacity"`},
	} {
		for _, f := range forbidden {
			if strings.Contains(consolePages[id], f) {
				t.Errorf("/console/%s still reads %s", id, f)
			}
		}
	}
	// The mode switch is two real links, remembered.
	for _, must := range []string{`lamdis.console.mode`, `"/console/buy"`, `sessionStorage`} {
		if !strings.Contains(consoleJS, must) {
			t.Errorf("the shared console script lacks %s", must)
		}
	}
}

// The buyer side has to reach the routes it needs with the credential it has.
func TestConsoleBuyerSideUsesSessionRoutes(t *testing.T) {
	all := ""
	for _, page := range consolePages {
		all += page
	}
	for _, must := range []string{
		`"/v1/tasks"`,         // posting a job from the browser
		`"/v1/quote"`,         // feasibility before money moves
		`/references`,         // reference photos after posting
		`data-evidence`,       // evidence rendered in the page
		`data-receipt`,        // receipt rendered in the page
		`confidence_ceiling`,  // and it says how sure anyone may be
		`"/v1/supplier"`,      // business profile
		`"/v1/statement`,      // statement and CSV
		`"/v1/alerts?on="`,    // alert toggle
		`"/v1/payout"`,        // payout status
		`support@lamdis.ai`,   // vetting is a person, and here is who
		`lamdis.console.mode`, // the mode is remembered
		`claude mcp add`,      // the one line that connects an agent
		`/v1/workers/giveback/`,
	} {
		if !strings.Contains(all, must) {
			t.Errorf("no console page uses %s", must)
		}
	}
	// The old evidence link handed a browser session a URL only an agent key
	// could open.
	if strings.Contains(all, `href="' + esc(j.evidence)`) {
		t.Error("spending rows still link straight to the agent-only evidence URL")
	}
}
