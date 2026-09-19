package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// The difference that matters to somebody leaving an agent running is
// between a tool that reads and a tool that changes something. Nobody
// should have to work that out from a list of function names.
func TestThingsThatChangeSomethingAreRecognised(t *testing.T) {
	for name, want := range map[string]bool{
		"create_issue":     true,
		"delete_file":      true,
		"send_message":     true,
		"update_record":    true,
		"pay_invoice":      true,
		"repo_create_pr":   true,
		"search_issues":    false,
		"list_projects":    false,
		"get_user":         false,
		"read_page":        false,
		"describe_dataset": false,
	} {
		if got := writes(&sdk.Tool{Name: name}); got != want {
			t.Errorf("%s: writes=%v, wanted %v", name, got, want)
		}
	}
	// A server that says a tool is read-only is believed over the name.
	if writes(&sdk.Tool{Name: "create_report", Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true}}) {
		t.Error("a declared read-only tool was called a write")
	}
	yes := true
	if !writes(&sdk.Tool{Name: "tidy", Annotations: &sdk.ToolAnnotations{DestructiveHint: &yes}}) {
		t.Error("a declared destructive tool was called a read")
	}
}

// A tool's description is the only thing that tells a person what it is
// for, but MCP descriptions run to paragraphs and a list needs one line.
func TestADescriptionIsCutToSomethingReadable(t *testing.T) {
	long := "Creates a new issue in the given project. Accepts a title, a body, " +
		"an assignee, a list of labels, a milestone, and a due date, all of which " +
		"are optional except the title, which is required and must be non-empty."
	got := firstSentence(long)
	if got != "Creates a new issue in the given project" {
		t.Fatalf("not cut at the first sentence: %q", got)
	}
	// One long sentence with no full stop still has to fit.
	run := strings.Repeat("words that keep going ", 20)
	if got := firstSentence(run); len([]rune(got)) > 161 {
		t.Fatalf("a run-on was left at %d characters", len([]rune(got)))
	}
	if firstSentence("  Reads\na thing.  ") != "Reads a thing" {
		t.Fatalf("whitespace survived: %q", firstSentence("  Reads\na thing.  "))
	}
}

// The whole point of pasting an address: one round trip has to come back
// with the name, the descriptions and which tools change something, or the
// person is back to filling in a form.
func TestOnePasteLearnsEverythingAboutAServer(t *testing.T) {
	srv := sdk.NewServer(&sdk.Implementation{Name: "orderdesk", Version: "2"}, nil)
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "search_orders",
		Description: "Finds orders by customer, date or status. Returns at most fifty.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in struct{}) (*sdk.CallToolResult, struct{}, error) {
		return nil, struct{}{}, nil
	})
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "refund_order",
		Description: "Refunds an order to the original payment method.",
	}, func(ctx context.Context, req *sdk.CallToolRequest, in struct{}) (*sdk.CallToolResult, struct{}, error) {
		return nil, struct{}{}, nil
	})

	handler := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return srv }, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	res, err := Probe(context.Background(), ToolServer{Name: "x", URL: ts.URL}, true)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if res.Name != "orderdesk" {
		t.Errorf("the server's own name was not picked up: %q", res.Name)
	}
	if len(res.Tools) != 2 {
		t.Fatalf("tools: %+v", res.Tools)
	}
	by := map[string]ProbeTool{}
	for _, tl := range res.Tools {
		by[tl.Name] = tl
	}
	if by["search_orders"].Writes {
		t.Error("a search was called a write")
	}
	if !by["refund_order"].Writes {
		t.Error("a refund was not called a write")
	}
	if by["search_orders"].What != "Finds orders by customer, date or status" {
		t.Errorf("description not carried through: %q", by["search_orders"].What)
	}
}
