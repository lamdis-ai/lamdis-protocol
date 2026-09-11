package exchange

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The failure these cover: an agent that reached exchange.lamdis.ai by name
// and asked it the questions the agent protocols agree on — where is your
// card, where is your API document — got 404 for both.

// Both well-known paths answer with the same parseable card, carrying every
// field the A2A specification requires.
func TestAgentCardIsServedAtBothWellKnownPaths(t *testing.T) {
	_, h := seoServer(t)
	var first []byte
	for _, p := range []string{"/.well-known/agent-card.json", "/.well-known/agent.json"} {
		w := get(t, h, p)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d", p, w.Code)
		}
		if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("%s content type is %q", p, ct)
		}
		if first == nil {
			first = w.Body.Bytes()
		} else if !bytes.Equal(first, w.Body.Bytes()) {
			t.Errorf("%s serves a different card from the first path", p)
		}
	}

	var card map[string]any
	if err := json.Unmarshal(first, &card); err != nil {
		t.Fatalf("card is not JSON: %v", err)
	}
	for _, k := range []string{"protocolVersion", "name", "description", "url",
		"version", "capabilities", "skills", "defaultInputModes",
		"defaultOutputModes", "provider", "documentationUrl",
		"securitySchemes", "supportedInterfaces"} {
		if _, ok := card[k]; !ok {
			t.Errorf("card is missing %q", k)
		}
	}
	if card["url"] != "https://exchange.example/v1" {
		t.Errorf("url = %v, want the REST base at the configured origin", card["url"])
	}
	if card["documentationUrl"] != "https://exchange.example/docs" {
		t.Errorf("documentationUrl = %v", card["documentationUrl"])
	}

	skills, _ := card["skills"].([]any)
	if len(skills) != 2 {
		t.Fatalf("want 2 skills, got %d", len(skills))
	}
	for _, sk := range skills {
		m := sk.(map[string]any)
		for _, k := range []string{"id", "name", "description", "tags", "examples"} {
			if _, ok := m[k]; !ok {
				t.Errorf("skill %v is missing %q", m["id"], k)
			}
		}
		if ex, _ := m["examples"].([]any); len(ex) == 0 {
			t.Errorf("skill %v has no examples", m["id"])
		}
	}

	// Nothing here is an A2A task endpoint, and the card must not pretend.
	caps := card["capabilities"].(map[string]any)
	for _, k := range []string{"streaming", "pushNotifications", "stateTransitionHistory"} {
		if v, _ := caps[k].(bool); v {
			t.Errorf("capabilities.%s is true but is not implemented", k)
		}
	}
	desc := card["description"].(string)
	for _, want := range []string{"/mcp", "/openapi.yaml", "not an A2A task-lifecycle",
		"Coverage today is zero", "No account"} {
		if !strings.Contains(desc, want) {
			t.Errorf("description does not say %q", want)
		}
	}
	// A first job needs no credential: the requirement list is empty.
	if sec, _ := card["security"].([]any); len(sec) != 0 {
		t.Errorf("security = %v, want none required", sec)
	}
}

// The OpenAPI document is served as the YAML the repository validates, and
// the JSON name points at it rather than vanishing.
func TestOpenAPIIsServed(t *testing.T) {
	_, h := seoServer(t)
	w := get(t, h, "/openapi.yaml")
	if w.Code != http.StatusOK {
		t.Fatalf("/openapi.yaml: %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/yaml") {
		t.Errorf("content type is %q", ct)
	}
	if !strings.HasPrefix(w.Body.String(), "openapi: 3.1.0") {
		t.Errorf("body does not start with the openapi version line: %.40q", w.Body.String())
	}
	if !bytes.Equal(w.Body.Bytes(), openapiYAML) {
		t.Error("served body differs from the embedded document")
	}

	w = get(t, h, "/openapi.json")
	if w.Code != http.StatusNotFound {
		t.Fatalf("/openapi.json: %d, want 404 with a pointer", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("/openapi.json body is not JSON: %v", err)
	}
	if body["openapi"] != "https://exchange.example/openapi.yaml" {
		t.Errorf("pointer = %v", body["openapi"])
	}
}

// The embedded copy is spec/openapi.yaml, byte for byte. If this fails, run
// `go generate ./internal/exchange/` (or copy the file) — the two must not
// drift.
func TestEmbeddedOpenAPIMatchesSpec(t *testing.T) {
	src := filepath.Join("..", "..", "..", "spec", "openapi.yaml")
	want, err := os.ReadFile(src)
	if err != nil {
		if os.IsNotExist(err) {
			// Only the module is present (a container build, say). The
			// repository's own run of this test is the guard.
			t.Skipf("%s not present alongside the module", src)
		}
		t.Fatal(err)
	}
	if !bytes.Equal(want, openapiYAML) {
		t.Fatalf("internal/exchange/openapi/openapi.yaml differs from spec/openapi.yaml; refresh the copy with go generate")
	}
}

// The crawler-facing files know about the discovery paths.
func TestDiscoveryPathsAreCrawlable(t *testing.T) {
	_, h := seoServer(t)
	robots := get(t, h, "/robots.txt").Body.String()
	sitemap := get(t, h, "/sitemap.xml").Body.String()
	for _, p := range []string{"/.well-known/agent-card.json", "/.well-known/agent.json", "/openapi.yaml"} {
		if !strings.Contains(robots, "Allow: "+p) {
			t.Errorf("robots.txt does not allow %s", p)
		}
		if !strings.Contains(sitemap, "<loc>https://exchange.example"+p+"</loc>") {
			t.Errorf("sitemap does not list %s", p)
		}
	}
}
