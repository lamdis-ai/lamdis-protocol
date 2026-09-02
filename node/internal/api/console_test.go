package api

import (
	"regexp"
	"strings"
	"testing"
)

// The console serves two audiences and used to give them one rail. An
// operator's anchors — capacity, earnings — meant nothing to a buyer, and two
// of the links on the page went to sections that did not exist: the shell's
// "Capacity" pointed at #standing and the capacity pane pointed at #supplier,
// neither of which any page defined. A link to nowhere is worse than no link;
// it teaches the reader the page is broken.
func TestConsoleHasARailPerModeAndNoDeadAnchors(t *testing.T) {
	for _, must := range []string{
		`id="rail-operator"`, `id="rail-buyer"`,
		`id="mode-operator"`, `id="mode-buyer"`,
		`id="sec-operator"`, `id="sec-buyer"`,
	} {
		if !strings.Contains(consolePageHTML, must) {
			t.Errorf("the console has no %s", must)
		}
	}
	// Every in-page anchor on the console must land on something.
	ids := map[string]bool{}
	for _, m := range regexp.MustCompile(`id="([\w-]+)"`).FindAllStringSubmatch(consolePageHTML, -1) {
		ids[m[1]] = true
	}
	for _, m := range regexp.MustCompile(`href="#([\w-]+)"`).FindAllStringSubmatch(consolePageHTML, -1) {
		if !ids[m[1]] {
			t.Errorf("the console links to #%s, which nothing on the page defines", m[1])
		}
	}
	for _, m := range regexp.MustCompile(`href=\\"#([\w-]+)\\"`).FindAllStringSubmatch(consolePageHTML, -1) {
		if !ids[m[1]] {
			t.Errorf("the console links to #%s, which nothing on the page defines", m[1])
		}
	}
	// And the shared shell, which every other page renders, must only point
	// at console sections that exist.
	shell := shellTop("queue", "")
	for _, m := range regexp.MustCompile(`href="/console#([\w-]+)"`).FindAllStringSubmatch(shell, -1) {
		if !ids[m[1]] {
			t.Errorf("the shell links to /console#%s, which the console does not define", m[1])
		}
	}
	if strings.Contains(shell, "#standing") {
		t.Error("the shell still links to /console#standing")
	}
}

// The buyer side has to reach the routes it needs with the credential it has.
func TestConsoleBuyerSideUsesSessionRoutes(t *testing.T) {
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
	} {
		if !strings.Contains(consolePageHTML, must) {
			t.Errorf("the console does not use %s", must)
		}
	}
	// The old evidence link handed a browser session a URL only an agent key
	// could open.
	if strings.Contains(consolePageHTML, `href="' + esc(j.evidence)`) {
		t.Error("spending rows still link straight to the agent-only evidence URL")
	}
}
