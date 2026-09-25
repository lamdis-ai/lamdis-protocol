package api

import (
	"testing"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/agent"
)

// Naming a connection is a question nobody can answer better than the
// address itself, so the address should answer it.
func TestAnAddressNamesItsOwnConnection(t *testing.T) {
	for addr, want := range map[string]string{
		"https://mcp.linear.app/mcp":    "linear",
		"https://mcp.notion.com/mcp":    "notion",
		"https://api.github.com/mcp":    "github",
		"https://www.sentry.io/mcp":     "sentry",
		"https://server.plane.so/v1":    "plane",
		"https://mcp.example.co.uk/mcp": "example",
		"http://localhost:9000/mcp":     "tools",
		"not a url at all":              "tools",
	} {
		if got := suggestName(addr); got != want {
			t.Errorf("%s named %q, wanted %q", addr, got, want)
		}
	}
}

// Two connections cannot share a name, and the person who pasted the second
// address should not be the one who finds that out.
func TestASecondConnectionToTheSamePlaceGetsItsOwnName(t *testing.T) {
	have := []agent.ToolServer{{Name: "linear"}, {Name: "linear2"}}
	if got := freeName("linear", have); got != "linear3" {
		t.Fatalf("collided: %q", got)
	}
	if got := freeName("notion", have); got != "notion" {
		t.Fatalf("a free name was changed: %q", got)
	}
}

// A server's own name is better than one guessed from the address, but
// only after it is made fit to be a label.
func TestAServersOwnNameIsTidiedBeforeItIsUsed(t *testing.T) {
	for raw, want := range map[string]string{
		"orderdesk":         "orderdesk",
		"github-mcp-server": "github",
		"Notion MCP":        "notion",
		"mcp-linear":        "linear",
		"Sentry MCP Server": "sentry",
		"  Plane  ":         "plane",
		"!!!":               "tools",
		"averyveryverylongnameindeedthatnobodywants": "averyveryverylongnameind",
	} {
		if got := sane(raw); got != want {
			t.Errorf("%q became %q, wanted %q", raw, got, want)
		}
	}
}
