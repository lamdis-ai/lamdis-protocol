package api

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Documentation that names an endpoint which does not exist is worse than no
// documentation: it sends a developer to write code against a 404 and blame
// their own request.
//
// This walks every endpoint printed on the docs page and in llms.txt, and
// fails if the codebase does not register it.
func TestDocsOnlyPromiseEndpointsThatExist(t *testing.T) {
	docs, err := os.ReadFile("docs_html.go")
	if err != nil {
		t.Fatal(err)
	}
	claimed := map[[2]string]bool{}
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`<td>(GET|POST|PUT|DELETE) ([^<]+)</td>`),
		regexp.MustCompile(`(?m)^(GET|POST|PUT|DELETE)\s+(/\S+)`),
	} {
		for _, m := range re.FindAllStringSubmatch(string(docs), -1) {
			claimed[[2]string{m[1], strings.TrimSpace(m[2])}] = true
		}
	}
	if len(claimed) < 10 {
		t.Fatalf("only found %d documented endpoints; the parser is probably wrong",
			len(claimed))
	}

	registered := map[[2]string]bool{}
	route := regexp.MustCompile(`HandleFunc\("(GET|POST|PUT|DELETE) ([^"]+)"`)
	files, _ := filepath.Glob("../*/*.go")
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, m := range route.FindAllStringSubmatch(string(b), -1) {
			registered[[2]string{m[1], normalizePath(m[2])}] = true
		}
	}

	for c := range claimed {
		if !registered[[2]string{c[0], normalizePath(c[1])}] {
			t.Errorf("the docs promise %s %s, which is not mounted anywhere",
				c[0], c[1])
		}
	}
}

// normalizePath makes {job} and {id} comparable across doc prose and routes.
func normalizePath(p string) string {
	return regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString(p, "{}")
}

// llms.txt is the first thing an agent reads, and it used to open by telling
// the reader to sign in and get a key — for a service where a first job needs
// no account at all. The gate it advertised did not exist, and the readers who
// believed it left.
func TestLLMsTxtDoesNotDemandAnAccount(t *testing.T) {
	head := llmsTXT
	if i := strings.Index(head, "## Authentication"); i > 0 {
		head = head[:i]
	}
	if strings.Contains(head, "Sign in to get a key") {
		t.Error("llms.txt still opens by asking for a key")
	}
	for _, want := range []string{
		"No account is needed",
		"claude mcp add --transport http lamdis https://exchange.lamdis.ai/mcp",
		`"sandbox":true`,
		"Coverage today is zero",
		"/v1/demand",
		"/coverage",
	} {
		if !strings.Contains(head, want) {
			t.Errorf("llms.txt does not open with %q", want)
		}
	}
	// The sandbox command has to be one somebody actually ran.
	if !strings.Contains(head, "curl -sX POST https://exchange.lamdis.ai/v1/tasks") {
		t.Error("llms.txt has no runnable ten-second example")
	}
}
