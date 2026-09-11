package exchange

// Being findable at all.
//
// The exchange served eleven public pages and answered 404 for /robots.txt.
// No sitemap, no canonical URLs, nothing submitted to any index: `site:` on
// the domain returned nothing, so an agent told to go and find a marketplace
// for physical work could not find this one. For a market whose whole thesis
// is that agents discover it and post work into it, that is not a marketing
// problem.
//
// Three things live here. robots.txt, which says what may be crawled and
// where the sitemap is. sitemap.xml, which lists the pages worth indexing and
// is built from the configured base URL rather than a hardcoded host, because
// a staging deployment must not claim to be production. And IndexNow, the
// keyless protocol Bing, DuckDuckGo and Yandex accept, which is the only way
// to get indexed in hours rather than weeks.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
)

// indexNowEndpoint is the shared submission API. Bing, Yandex and DuckDuckGo
// all read from it, so one POST reaches every engine that participates.
const indexNowEndpoint = "https://api.indexnow.org/indexnow"

// crawlPage is one entry in the sitemap.
type crawlPage struct {
	Path       string
	ChangeFreq string
	Priority   string
}

// crawlPages is every page a crawler should have. It is deliberately short:
// the console, the work pages, the reviewer links and the pay pages are all
// either private, per-job, or credential-gated, and a sitemap that lists them
// is a sitemap full of 404s and 401s.
var crawlPages = []crawlPage{
	{"/", "daily", "1.0"},
	{"/board", "hourly", "0.9"},
	{"/coverage", "daily", "0.8"},
	{"/post", "weekly", "0.8"},
	{"/docs", "weekly", "0.9"},
	{"/how-it-works", "weekly", "0.7"},
	{"/about", "monthly", "0.5"},
	{"/terms", "monthly", "0.3"},
	{"/privacy", "monthly", "0.3"},
	{"/support", "monthly", "0.4"},
	{"/contact", "monthly", "0.4"},
	{"/llms.txt", "weekly", "0.6"},
	{"/openapi.yaml", "weekly", "0.6"},
	{"/.well-known/agent-card.json", "weekly", "0.6"},
	{"/.well-known/agent.json", "weekly", "0.4"},
}

// CrawlURLs is every absolute URL in the sitemap, in order.
func (s *Server) CrawlURLs() []string {
	b := s.base()
	out := make([]string, 0, len(crawlPages))
	for _, p := range crawlPages {
		if p.Path == "/" {
			out = append(out, b+"/")
			continue
		}
		out = append(out, b+p.Path)
	}
	return out
}

// IndexNowKey is the shared secret that proves this host asked for the
// submission.
//
// Derived from the exchange key rather than generated and stored, for the
// same reason a buyer token is (see BuyerToken): the exchange key is the only
// secret worth keeping, a derived value survives a restart with no file to
// lose, and the purpose string keeps this disjoint from every other thing
// derived from the same key. IndexNow wants 8–128 characters of [a-zA-Z0-9-];
// 32 hex digits is inside that.
func (s *Server) IndexNowKey() string {
	mac := hmac.New(sha256.New, []byte(s.Key))
	mac.Write([]byte("lamdis-indexnow-key"))
	return hex.EncodeToString(mac.Sum(nil))[:32]
}

// indexNowKeyLocation is where the engine fetches the key to check we own the
// host. It must be served from the same origin as the URLs being submitted.
func (s *Server) indexNowKeyLocation() string {
	return s.base() + "/" + s.IndexNowKey() + ".txt"
}

// robotsTXT is what a crawler reads first.
//
// Everything under /v1/ is disallowed except the handful of read-only JSON
// endpoints that are worth having in an index — the board, where supply is,
// where demand went unmet, the rails, the anchors, what the bootstrap loop
// found, and the exchange's own identity. The rest of /v1/ is either
// credential-gated or a POST, and a crawler hitting it learns nothing and
// wastes both our time and its budget. /w/, /r/, /my/, /paid/ and /pay/ are
// capability links: a crawler that indexes one publishes a credential.
func (s *Server) robotsTXT() string {
	return `User-agent: *
Allow: /
Disallow: /v1/
Allow: /v1/board
Allow: /v1/coverage
Allow: /v1/demand
Allow: /v1/rails
Allow: /v1/anchors
Allow: /v1/findings
Allow: /v1/bootstrap
Allow: /v1/exchange
Allow: /.well-known/agent-card.json
Allow: /.well-known/agent.json
Allow: /openapi.yaml
Disallow: /w/
Disallow: /r/
Disallow: /my/
Disallow: /paid/
Disallow: /pay/
Disallow: /console
Disallow: /mcp

Sitemap: ` + s.base() + `/sitemap.xml
`
}

// sitemapXML lists the crawlable pages.
//
// lastmod is the date this process came up. It is honest — a deployment is
// the only thing that changes any of these pages, since every one of them is
// a constant compiled into the binary — and it stops the sitemap claiming a
// freshness it does not have.
func (s *Server) sitemapXML() string {
	day := s.started.UTC().Format("2006-01-02")
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	base := s.base()
	for _, p := range crawlPages {
		loc := base + p.Path
		if p.Path == "/" {
			loc = base + "/"
		}
		b.WriteString("  <url>\n")
		b.WriteString("    <loc>" + xmlEscape(loc) + "</loc>\n")
		b.WriteString("    <lastmod>" + day + "</lastmod>\n")
		b.WriteString("    <changefreq>" + p.ChangeFreq + "</changefreq>\n")
		b.WriteString("    <priority>" + p.Priority + "</priority>\n")
		b.WriteString("  </url>\n")
	}
	b.WriteString("</urlset>\n")
	return b.String()
}

// xmlEscape is enough for a URL in a <loc>: the five predefined entities.
func xmlEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// registerSEO mounts everything a crawler needs.
//
// node carries the exchange's own principal: submission is an operator act,
// guarded exactly as /v1/panels is, because it speaks for this host to a
// third party.
func (s *Server) registerSEO(mux *http.ServeMux, node *api.Server) {
	robots := s.robotsTXT()
	mux.HandleFunc("GET /robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		fmt.Fprint(w, robots)
	})
	mux.HandleFunc("GET /sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		fmt.Fprint(w, s.sitemapXML())
	})

	// The ownership proof: a file at the root whose name is the key and whose
	// body is the key. Registered under its literal path, because the name is
	// known at boot.
	key := s.IndexNowKey()
	mux.HandleFunc("GET /"+key+".txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, key)
	})

	// Public, so the submission can be made from anywhere — a laptop, a CI
	// job, a curl — without holding the exchange's key. The key is not a
	// secret: it is served at its own URL by design, and all it authorises is
	// telling a search engine to look at pages that are already public.
	mux.HandleFunc("GET /v1/indexnow", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResponse(w, map[string]any{
			"host":        hostOf(s.base()),
			"key":         key,
			"keyLocation": s.indexNowKeyLocation(),
			"urlList":     s.CrawlURLs(),
			"endpoint":    indexNowEndpoint,
		})
	})

	mux.HandleFunc("POST /v1/indexnow/submit", node.WithAuth(
		func(w http.ResponseWriter, r *http.Request, principal string, _ []byte) {
			// Only this exchange may speak for this host. WithAuth proves who
			// signed; this proves it was us.
			if principal != s.PID {
				writeError(w, http.StatusForbidden, "not this exchange's principal")
				return
			}
			status, err := s.SubmitIndexNow(r.Context())
			if err != nil {
				writeJSONResponse(w, map[string]any{
					"submitted": false, "error": err.Error(),
					"urls": len(crawlPages),
				})
				return
			}
			writeJSONResponse(w, map[string]any{
				"submitted": status >= 200 && status < 300,
				"status":    status, "urls": len(crawlPages),
			})
		}))
}

// hostOf is the bare host IndexNow wants, without scheme or path.
func hostOf(base string) string {
	h := strings.TrimPrefix(strings.TrimPrefix(base, "https://"), "http://")
	if i := strings.IndexByte(h, '/'); i >= 0 {
		h = h[:i]
	}
	return h
}

// SubmitIndexNow tells the participating engines to come and look.
//
// It returns the HTTP status the endpoint gave, and never panics or exits: a
// search engine being unreachable is not a reason for an exchange to stop
// serving jobs, so every failure here is a log line and an error value.
func (s *Server) SubmitIndexNow(ctx context.Context) (int, error) {
	body, err := json.Marshal(map[string]any{
		"host":        hostOf(s.base()),
		"key":         s.IndexNowKey(),
		"keyLocation": s.indexNowKeyLocation(),
		"urlList":     s.CrawlURLs(),
	})
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		indexNowEndpoint, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("indexnow: submission failed: %v", err)
		return 0, err
	}
	defer resp.Body.Close()
	reply, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	// 200 and 202 both mean accepted; 403 means the key file could not be
	// read, which is worth saying plainly because it is the only failure a
	// deployment can actually fix.
	switch {
	case resp.StatusCode == http.StatusForbidden:
		log.Printf("indexnow: refused (403) — %s must serve the key %q",
			s.indexNowKeyLocation(), s.IndexNowKey())
	case resp.StatusCode >= 300:
		log.Printf("indexnow: %s answered %d: %s", indexNowEndpoint,
			resp.StatusCode, strings.TrimSpace(string(reply)))
	default:
		log.Printf("indexnow: submitted %d urls for %s, status %d",
			len(crawlPages), hostOf(s.base()), resp.StatusCode)
	}
	return resp.StatusCode, nil
}
