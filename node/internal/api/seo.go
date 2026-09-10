package api

import (
	"html"
	"strings"
)

// What a crawler needs before it will index a page.
//
// The exchange was invisible: `site:exchange.lamdis.ai` returned nothing,
// /robots.txt was a 404, there was no sitemap, and not one page carried a
// canonical URL or a description. An agent told to go and find a marketplace
// for physical work could not find this one, which is the same as it not
// existing.
//
// None of the pages are templated — each is a constant assembled at init —
// so the tags are inserted once, at registration, when the base URL is
// finally known. Doing it per request would rebuild a 200 KB string for
// every crawler hit.

// seoAnchor is the one line every page here shares. The tags go directly
// after it, which puts them in the head where a crawler looks.
const seoAnchor = `<meta name="viewport" content="width=device-width,initial-scale=1">`

// SEOTags is the canonical URL, the description and the indexing permission.
//
// The canonical is absolute because a relative canonical is ignored by half
// the crawlers that read it, and it is built from the configured base URL so
// a staging deployment does not tell Google it is exchange.lamdis.ai.
func SEOTags(baseURL, path, description string) string {
	base := strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	canonical := base + path
	if path == "/" {
		canonical = base + "/"
	}
	return "\n<link rel=\"canonical\" href=\"" + html.EscapeString(canonical) + "\">" +
		"\n<meta name=\"description\" content=\"" + html.EscapeString(description) + "\">" +
		"\n<meta name=\"robots\" content=\"index,follow\">" +
		"\n<meta property=\"og:description\" content=\"" + html.EscapeString(description) + "\">" +
		"\n<meta property=\"og:url\" content=\"" + html.EscapeString(canonical) + "\">" +
		"\n<meta property=\"og:type\" content=\"website\">"
}

// WithSEO returns the page with the tags in its head.
//
// A page without the anchor is returned unchanged rather than mangled: a
// missing canonical is a page that ranks badly, a corrupted head is a page
// that does not render.
func WithSEO(page, baseURL, path, description string) string {
	i := strings.Index(page, seoAnchor)
	if i < 0 {
		return page
	}
	i += len(seoAnchor)
	return page[:i] + SEOTags(baseURL, path, description) + page[i:]
}
