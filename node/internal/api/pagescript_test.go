package api

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// A page that calls a function nobody defined is a broken page, and the
// browser only says so at the moment somebody clicks.
//
// This is the second bug of exactly this shape. The first was a work page that
// never called /submit, so nothing was ever verified and nobody was ever paid.
// The second was this: the reviewer panel called enrol(), which has never
// existed in any scope the page can see. It threw a ReferenceError
// synchronously, before the .catch that would have reported it was attached,
// and left the reviewer looking at a disabled button reading "Finding one…"
// with no way forward. Every test passed throughout, because tests call routes
// and never load the page.
//
// So: parse the assembled page, collect what it calls, collect what it defines,
// and fail on the difference. It is a coarse check and it would have caught
// both bugs on the commit that introduced them.
func TestPagesDefineEverythingTheyCall(t *testing.T) {
	pages := map[string]string{
		"reviewPageHTML":  reviewPageHTML,
		"boardPageHTML":   boardPageHTML,
		"consolePageHTML": consolePageHTML,
		"workPageHTML":    workPageHTML,
		"signInPageHTML":  signInPageHTML,
		"jobPage":         jobPage(&Listing{Job: "j-1", Title: "a job"}),
	}
	for name, page := range pages {
		t.Run(name, func(t *testing.T) {
			missing := undefinedCalls(page)
			if len(missing) > 0 {
				t.Errorf("%s calls %v, which nothing defines.\n"+
					"A browser raises ReferenceError here and the page stops "+
					"mid-handler, usually with a control left disabled.", name, missing)
			}
		})
	}
}

var (
	// A call: an identifier followed by "(", not preceded by "." or a word
	// character, so property calls and longer names are excluded.
	callRe = regexp.MustCompile(`(?:^|[^.\w$])([a-zA-Z_$][\w$]*)\s*\(`)
	// The three ways this codebase introduces a name.
	funcRe  = regexp.MustCompile(`function\s+([a-zA-Z_$][\w$]*)\s*\(`)
	varFnRe = regexp.MustCompile(`(?:var|let|const)\s+([a-zA-Z_$][\w$]*)\s*=`)
	paramRe = regexp.MustCompile(`function\s*\(([^)]*)\)`)
)

// browserGlobals are names the runtime provides. Kept explicit rather than
// inferred: a name arriving here should be a decision, because every addition
// is a promise that the browser really does supply it.
var browserGlobals = map[string]bool{
	"fetch": true, "setTimeout": true, "clearTimeout": true, "setInterval": true,
	"clearInterval": true, "parseInt": true, "parseFloat": true, "isNaN": true, "isFinite": true,
	"encodeURIComponent": true, "decodeURIComponent": true, "alert": true,
	"String": true, "Number": true, "Boolean": true, "Array": true, "Object": true,
	"Date": true, "Math": true, "JSON": true, "Promise": true, "Error": true,
	"Uint8Array": true, "DataView": true, "FileReader": true,
	"URLSearchParams": true, "Uint32Array": true, "Int32Array": true, "ArrayBuffer": true,
	"TextEncoder": true, "TextDecoder": true, "URL": true, "Blob": true, "FormData": true,
	"localStorage": true, "sessionStorage": true, "document": true, "window": true,
	"history": true, "location": true, "console": true, "crypto": true, "navigator": true,
	// Control flow, which the call regexp cannot tell from a call.
	"if": true, "for": true, "while": true, "switch": true, "catch": true,
	"return": true, "typeof": true, "function": true, "async": true, "await": true,
	"new": true, "do": true, "else": true, "throw": true, "in": true, "of": true,
}

// stripNoise removes what is not executable: comments, the stylesheet, and
// string literals. Without this the scan reports words out of prose — the
// comment explaining a bug names the very identifier it warns about.
func stripNoise(page string) string {
	page = regexp.MustCompile(`(?s)<style>.*?</style>`).ReplaceAllString(page, "")
	page = regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(page, "")
	page = regexp.MustCompile(`(?m)//.*$`).ReplaceAllString(page, "")
	page = regexp.MustCompile(`"(?:[^"\\\n]|\\.)*"`).ReplaceAllString(page, `""`)
	page = regexp.MustCompile(`'(?:[^'\\\n]|\\.)*'`).ReplaceAllString(page, `''`)
	return page
}

func undefinedCalls(page string) []string {
	page = stripNoise(page)
	defined := map[string]bool{}
	for _, m := range funcRe.FindAllStringSubmatch(page, -1) {
		defined[m[1]] = true
	}
	for _, m := range varFnRe.FindAllStringSubmatch(page, -1) {
		defined[m[1]] = true
	}
	// Parameters are locals and may well be called (callbacks, thunks).
	for _, m := range paramRe.FindAllStringSubmatch(page, -1) {
		for _, p := range strings.Split(m[1], ",") {
			if p = strings.TrimSpace(p); p != "" {
				defined[p] = true
			}
		}
	}
	seen := map[string]bool{}
	var missing []string
	for _, m := range callRe.FindAllStringSubmatch(page, -1) {
		n := m[1]
		if defined[n] || browserGlobals[n] || seen[n] {
			continue
		}
		seen[n] = true
		missing = append(missing, n)
	}
	sort.Strings(missing)
	return missing
}

// The specific regression, named, so the failure says what broke rather than
// leaving somebody to rediscover it.
func TestReviewPanelVerifyAnotherIsWired(t *testing.T) {
	if strings.Contains(stripNoise(reviewPageHTML), "enrol(") {
		t.Fatal("the panel calls enrol(), which does not exist; " +
			"\"Verify another\" wedges on \"Finding one…\" forever")
	}
	if !strings.Contains(reviewPageHTML, `"/v1/workers/assign"`) {
		t.Fatal("nothing asks for the next panel")
	}
}
