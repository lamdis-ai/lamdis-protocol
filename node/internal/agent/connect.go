package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Connecting by asking.
//
// A person says "connect my calendar" in a channel. The agent looks for a
// way in (find_connection), and when it finds one it puts a card in the
// channel (offer_connection). The person presses Connect, signs in on the
// service's own page in a window of its own, and the connection is saved.
//
// The agent never holds the credential and never completes a sign-in: it can
// only propose. Everything that would let it act as the person passes
// between the person's browser, the service, and the vault.

// KindConnect is the card; KindConnectReply records how it ended.
const (
	KindConnect      = "agent.connect"
	KindConnectReply = "agent.connect_reply"
)

// CatalogEntry is one service with a known, official way in.
type CatalogEntry struct {
	Name    string   `json:"name"`
	Aliases []string `json:"aliases,omitempty"`
	URL     string   `json:"url,omitempty"`
	// How: "oauth" (sign in on their page), "key" (paste a key from their
	// settings), "open" (nothing needed), or "none" (no official way).
	How  string `json:"how"`
	Note string `json:"note,omitempty"`
}

// Candidate is one way to connect something, from wherever it was found.
type Candidate struct {
	Name   string `json:"name"`
	URL    string `json:"url,omitempty"`
	How    string `json:"how"`
	Source string `json:"source"` // catalog | registry
	Note   string `json:"note,omitempty"`
	vendor string
}

// FindConnection looks a service up: the catalog first (checked by hand),
// then the public MCP registry. It never connects anything.
func FindConnection(ctx context.Context, query string) []Candidate {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	var out []Candidate
	seen := map[string]bool{}
	for _, e := range Catalog {
		if matches(q, e) {
			c := Candidate{Name: e.Name, URL: e.URL, How: e.How, Source: "catalog", Note: e.Note}
			out = append(out, c)
			seen[strings.TrimSuffix(e.URL, "/")] = true
		}
	}
	for _, c := range rankRegistry(q, searchRegistry(ctx, q)) {
		if !seen[strings.TrimSuffix(c.URL, "/")] {
			out = append(out, c)
			seen[strings.TrimSuffix(c.URL, "/")] = true
		}
	}
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}

func matches(q string, e CatalogEntry) bool {
	names := append([]string{e.Name}, e.Aliases...)
	for _, n := range names {
		n = strings.ToLower(n)
		if n == q || hasWords(q, n) || (len(q) >= 4 && hasWords(n, q)) {
			return true
		}
	}
	return false
}

// hasWords reports whether phrase appears in s as whole words.
func hasWords(s, phrase string) bool {
	for i := 0; ; {
		j := strings.Index(s[i:], phrase)
		if j < 0 {
			return false
		}
		j += i
		end := j + len(phrase)
		if (j == 0 || !isWordByte(s[j-1])) && (end == len(s) || !isWordByte(s[end])) {
			return true
		}
		i = j + 1
	}
}

func isWordByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '.'
}

// registryBase is the official MCP registry. Variable for tests.
var registryBase = "https://registry.modelcontextprotocol.io"

var registryHTTP = &http.Client{Timeout: 8 * time.Second}

// searchRegistry asks the public registry for servers matching q and keeps
// only those reachable over the network (a hosted agent cannot run a
// program on somebody's laptop). The registry is a directory anyone can
// publish to, so these are offered as "listed", never as official.
func searchRegistry(ctx context.Context, q string) []Candidate {
	u := registryBase + "/v0.1/servers?limit=30&version=latest&search=" + url.QueryEscape(q)
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Accept", "application/json")
	resp, err := registryHTTP.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	return parseRegistry(raw)
}

type registryServer struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Remotes     []struct {
		Type    string `json:"type"`
		URL     string `json:"url"`
		Headers []struct {
			Value string `json:"value"`
		} `json:"headers"`
	} `json:"remotes"`
}

// parseRegistry reads both shapes the registry has used: a list of servers,
// and a list of {server, _meta} wrappers.
func parseRegistry(raw []byte) []Candidate {
	var page struct {
		Servers []json.RawMessage `json:"servers"`
	}
	if json.Unmarshal(raw, &page) != nil {
		return nil
	}
	var out []Candidate
	for _, item := range page.Servers {
		var wrapped struct {
			Server *registryServer `json:"server"`
		}
		var s registryServer
		if json.Unmarshal(item, &wrapped) == nil && wrapped.Server != nil {
			s = *wrapped.Server
		} else if json.Unmarshal(item, &s) != nil {
			continue
		}
		for _, r := range s.Remotes {
			if !strings.HasPrefix(r.URL, "https://") || strings.Contains(r.URL, "{") {
				continue // needs local setup or a templated address
			}
			if r.Type != "" && r.Type != "streamable-http" && r.Type != "http" && r.Type != "sse" {
				continue
			}
			templated := false
			for _, h := range r.Headers {
				if strings.Contains(h.Value, "{") {
					templated = true // wants a key for somebody else's gateway
				}
			}
			if templated {
				continue
			}
			name := s.Title
			if name == "" {
				name = s.Name
			}
			out = append(out, Candidate{Name: name, URL: r.URL, How: "check", Source: "registry",
				Note: trunc(firstSentence(s.Description), 160), vendor: vendorOf(s.Name)})
			break
		}
	}
	return out
}

// vendorOf reads the publisher's domain from a registry name such as
// "com.notion/mcp" or "io.github.github/github-mcp-server".
func vendorOf(name string) string {
	ns, _, _ := strings.Cut(strings.ToLower(name), "/")
	parts := strings.Split(ns, ".")
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, ".")
}

// rankRegistry puts servers published under the service's own domain first:
// the registry is open to anyone, and a wrapper around a service is not the
// service.
func rankRegistry(q string, cs []Candidate) []Candidate {
	word := strings.Fields(q)
	own := func(c Candidate) bool {
		for _, w := range word {
			if len(w) >= 3 && strings.Contains(c.vendor, w) {
				return true
			}
		}
		return false
	}
	sort.SliceStable(cs, func(i, j int) bool { return own(cs[i]) && !own(cs[j]) })
	for i := range cs {
		if own(cs[i]) {
			cs[i].Note = strings.TrimSpace("Published by " + cs[i].vendor + ". " + cs[i].Note)
		} else {
			cs[i].Note = strings.TrimSpace("Published by " + cs[i].vendor + ", not by the service itself. " + cs[i].Note)
		}
	}
	return cs
}

// describeCandidates is what the model reads back from find_connection.
func describeCandidates(query string, cs []Candidate, have []ToolServer) string {
	var sb strings.Builder
	for _, t := range have {
		if strings.Contains(strings.ToLower(t.Name+" "+t.URL), strings.ToLower(query)) {
			fmt.Fprintf(&sb, "ALREADY CONNECTED: %s (%s). Use its tools; no need to connect again.\n", t.Name, t.URL)
		}
	}
	if len(cs) == 0 {
		sb.WriteString("Nothing found for " + query + ". Say so plainly. Do not invent an address. " +
			"If the person has a link to the service's MCP server, offer_connection can take it.\n")
		return sb.String()
	}
	for _, c := range cs {
		switch {
		case c.How == "none":
			fmt.Fprintf(&sb, "- %s: NO official way to connect. %s\n", c.Name, c.Note)
		case c.Source == "catalog":
			fmt.Fprintf(&sb, "- %s (official) %s — %s. %s\n", c.Name, c.URL, howWords(c.How), c.Note)
		default:
			fmt.Fprintf(&sb, "- %s (listed in the public MCP registry, not checked by Lamdis) %s. %s\n", c.Name, c.URL, c.Note)
		}
	}
	sb.WriteString("Prefer an official entry. Offer one with offer_connection; the person signs in themselves. " +
		"For a registry listing, say who publishes it if the name makes that clear, and that it is not official.")
	return sb.String()
}

func howWords(h string) string {
	switch h {
	case "oauth":
		return "the person signs in on the service's own page"
	case "key":
		return "the person pastes a key from the service's settings"
	case "open":
		return "nothing to sign in to"
	}
	return "sign-in checked when they connect"
}
