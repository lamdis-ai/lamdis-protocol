package exchange

import (
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// The failure these cover: exchange.lamdis.ai was in no index at all.
// /robots.txt was a 404, there was no sitemap, and no page said what it was.
// An agent that finds things by searching could not find the exchange, which
// for a market that expects to be discovered by agents is fatal rather than
// cosmetic.

func seoServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	s, err := Open(key, "https://exchange.example/", Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return s, s.Handler()
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	return w
}

// A crawler's first request must be answered, and it must point at the map.
func TestRobotsTxtIsServed(t *testing.T) {
	_, h := seoServer(t)
	w := get(t, h, "/robots.txt")
	if w.Code != http.StatusOK {
		t.Fatalf("/robots.txt: %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		"User-agent: *", "Allow: /", "Disallow: /v1/",
		"Allow: /v1/board", "Allow: /v1/coverage", "Allow: /v1/demand",
		"Allow: /v1/rails", "Allow: /v1/anchors", "Allow: /v1/findings",
		"Allow: /v1/bootstrap", "Allow: /v1/exchange",
		"Disallow: /w/", "Disallow: /r/", "Disallow: /my/",
		"Disallow: /paid/", "Disallow: /pay/", "Disallow: /console",
		"Disallow: /mcp",
		"Sitemap: https://exchange.example/sitemap.xml",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("robots.txt is missing %q:\n%s", want, body)
		}
	}
	// The capability links are credentials. Indexing one publishes it.
	if strings.Contains(body, "Allow: /w/") || strings.Contains(body, "Allow: /r/") {
		t.Error("robots.txt allows a capability path")
	}
}

// The sitemap lists the pages a person or an agent should land on, and names
// them with the configured origin rather than a host somebody typed once.
func TestSitemapListsThePublicPagesAtTheBaseURL(t *testing.T) {
	_, h := seoServer(t)
	w := get(t, h, "/sitemap.xml")
	if w.Code != http.StatusOK {
		t.Fatalf("/sitemap.xml: %d", w.Code)
	}
	body := w.Body.String()
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/xml") {
		t.Errorf("sitemap content type is %q", ct)
	}
	for _, p := range []string{"/", "/board", "/coverage", "/post", "/docs",
		"/how-it-works", "/about", "/terms", "/privacy", "/support",
		"/contact", "/llms.txt"} {
		want := "<loc>https://exchange.example" + p + "</loc>"
		if !strings.Contains(body, want) {
			t.Errorf("sitemap is missing %s", want)
		}
	}
	if strings.Contains(body, "lamdis.ai") {
		t.Errorf("sitemap hardcodes a host instead of using the base URL:\n%s", body)
	}
	// lastmod is the day this process came up, not a fiction.
	if !regexp.MustCompile(`<lastmod>\d{4}-\d{2}-\d{2}</lastmod>`).MatchString(body) {
		t.Errorf("sitemap has no usable lastmod:\n%s", body)
	}
	if !strings.Contains(body, "<changefreq>") {
		t.Error("sitemap says nothing about how often pages change")
	}
	// Every URL in the sitemap must actually answer.
	for _, u := range []string{"/board", "/coverage", "/post", "/docs",
		"/how-it-works", "/about", "/terms", "/privacy", "/support",
		"/contact", "/llms.txt"} {
		if code := get(t, h, u).Code; code != http.StatusOK {
			t.Errorf("sitemap lists %s, which answers %d", u, code)
		}
	}
}

// IndexNow will not accept a submission unless the host serves the key at its
// own name. A missing file is a 403 from the endpoint and no indexing at all.
func TestIndexNowKeyFileServesTheKey(t *testing.T) {
	s, h := seoServer(t)
	key := s.IndexNowKey()
	if len(key) != 32 {
		t.Fatalf("key is %d characters, want 32: %q", len(key), key)
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(key) {
		t.Fatalf("key is not 32 hex digits: %q", key)
	}
	// Derived, so a restart does not invalidate what was submitted.
	if again := s.IndexNowKey(); again != key {
		t.Fatalf("key is not stable: %q then %q", key, again)
	}
	w := get(t, h, "/"+key+".txt")
	if w.Code != http.StatusOK {
		t.Fatalf("/%s.txt: %d", key, w.Code)
	}
	if strings.TrimSpace(w.Body.String()) != key {
		t.Fatalf("key file says %q, want %q", w.Body.String(), key)
	}
}

// The report exists so a submission can be made from outside the process —
// a laptop, a CI job — without holding the exchange's key.
func TestIndexNowReportCarriesKeyAndURLs(t *testing.T) {
	_, h := seoServer(t)
	w := get(t, h, "/v1/indexnow")
	if w.Code != http.StatusOK {
		t.Fatalf("/v1/indexnow: %d", w.Code)
	}
	var out struct {
		Host        string   `json:"host"`
		Key         string   `json:"key"`
		KeyLocation string   `json:"keyLocation"`
		URLList     []string `json:"urlList"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Host != "exchange.example" {
		t.Errorf("host is %q", out.Host)
	}
	if out.KeyLocation != "https://exchange.example/"+out.Key+".txt" {
		t.Errorf("key location is %q", out.KeyLocation)
	}
	if len(out.URLList) != len(crawlPages) {
		t.Errorf("submitting %d urls, sitemap has %d", len(out.URLList), len(crawlPages))
	}
	for _, u := range out.URLList {
		if !strings.HasPrefix(u, "https://exchange.example/") {
			t.Errorf("url %q is not on this host", u)
		}
	}
}

// Submitting speaks for the host to a third party, so it is guarded the way
// creating a panel is: the exchange's own principal, or nothing.
func TestIndexNowSubmitRefusesTheUnauthenticated(t *testing.T) {
	_, h := seoServer(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/indexnow/submit", strings.NewReader("{}")))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated submit: %d, want 401", w.Code)
	}
}

// Every page in the sitemap has to tell a crawler what it is. Without a
// description a result is a bare URL, and without a canonical the same page
// under two paths competes with itself.
func TestCrawlablePagesCarryTheirTags(t *testing.T) {
	_, h := seoServer(t)
	for _, p := range crawlPages {
		if p.Path == "/" || p.Path == "/llms.txt" {
			continue // a redirect and a text file, neither of which has a head
		}
		body := get(t, h, p.Path).Body.String()
		want := `<link rel="canonical" href="https://exchange.example` + p.Path + `">`
		if !strings.Contains(body, want) {
			t.Errorf("%s has no canonical %s", p.Path, want)
		}
		m := regexp.MustCompile(`<meta name="description" content="([^"]+)">`).FindStringSubmatch(body)
		if m == nil {
			t.Errorf("%s has no description", p.Path)
		} else if len(strings.Fields(m[1])) < 8 {
			t.Errorf("%s has a description of %d words: %q", p.Path, len(strings.Fields(m[1])), m[1])
		}
		if !strings.Contains(body, `<meta name="robots" content="index,follow">`) {
			t.Errorf("%s does not invite indexing", p.Path)
		}
	}
}

// Two trust pages sharing one description is two pages an index treats as
// one, and only one of them ranks.
func TestTrustPageDescriptionsDiffer(t *testing.T) {
	_, h := seoServer(t)
	seen := map[string]string{}
	re := regexp.MustCompile(`<meta name="description" content="([^"]+)">`)
	for _, p := range []string{"/how-it-works", "/terms", "/privacy",
		"/about", "/support", "/contact"} {
		m := re.FindStringSubmatch(get(t, h, p).Body.String())
		if m == nil {
			t.Fatalf("%s has no description", p)
		}
		if other, dup := seen[m[1]]; dup {
			t.Errorf("%s and %s share a description", p, other)
		}
		seen[m[1]] = p
	}
}

// The docs page is what a search engine should understand as an API, and what
// an agent reading structured data should be able to price at zero.
func TestDocsCarriesWebAPIJSONLD(t *testing.T) {
	_, h := seoServer(t)
	body := get(t, h, "/docs").Body.String()
	m := regexp.MustCompile(`(?s)<script type="application/ld\+json">(.*?)</script>`).FindStringSubmatch(body)
	if m == nil {
		t.Fatal("/docs carries no JSON-LD")
	}
	var ld map[string]any
	if err := json.Unmarshal([]byte(m[1]), &ld); err != nil {
		t.Fatalf("JSON-LD does not parse: %v\n%s", err, m[1])
	}
	if ld["@type"] != "WebAPI" {
		t.Errorf("@type is %v, want WebAPI", ld["@type"])
	}
	if ld["documentation"] != "https://exchange.example/docs" {
		t.Errorf("documentation is %v", ld["documentation"])
	}
	prov, _ := ld["provider"].(map[string]any)
	if prov["name"] != "Lamdis" {
		t.Errorf("provider is %v", ld["provider"])
	}
	offers, _ := ld["offers"].(map[string]any)
	if offers["price"] != "0" {
		t.Errorf("offers is %v", ld["offers"])
	}
}
