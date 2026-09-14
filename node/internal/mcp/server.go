// Package mcp exposes a node to AI agents over the Model Context Protocol.
// Agents can read, post, search, and sync. Deliberately absent: grant,
// deny, revoke — approvals are human-signed acts and never tool calls, so
// an agent cannot silently expand its own access.
package mcp

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/embed"
	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/perm"
	"github.com/lamdis-ai/lamdis-protocol/node/internal/store"
	syncp "github.com/lamdis-ai/lamdis-protocol/node/internal/sync"
)

// Node bundles what the MCP tools need from the running node.
type Node struct {
	Store    store.Store
	Key      ed25519.PrivateKey
	Self     string            // person principal id
	Peers    map[string]string // peer name -> base URL
	Embedder embed.Embedder    // nil = FTS-only search
}

const serverInstructions = `This node stores permissioned context threads shared between people's agents
via the Lamdis Protocol. Entries you post are signed and visible to whoever
the thread's stewards have granted access. Use search_context to answer
questions from shared context; use sync_peers first when freshness matters.
Access approvals are made by humans outside this interface — if a thread you
need is missing, ask your human to grant or request access.`

// NewServer builds the MCP server with the node's tool surface.
func NewServer(n *Node, version string) *sdk.Server {
	s := sdk.NewServer(&sdk.Implementation{Name: "lamdis", Version: version},
		&sdk.ServerOptions{Instructions: serverInstructions})

	type empty struct{}
	sdk.AddTool(s, &sdk.Tool{Name: "whoami",
		Description: "This node's person principal and configured peers."},
		func(ctx context.Context, req *sdk.CallToolRequest, _ empty) (*sdk.CallToolResult, any, error) {
			var peers []string
			for name, url := range n.Peers {
				peers = append(peers, name+" -> "+url)
			}
			return textResult("principal: %s\npeers: %s", n.Self, strings.Join(peers, ", ")), nil, nil
		})

	sdk.AddTool(s, &sdk.Tool{Name: "list_threads",
		Description: "List context threads held on this node, with titles."},
		func(ctx context.Context, req *sdk.CallToolRequest, _ empty) (*sdk.CallToolResult, any, error) {
			ids, err := n.Store.Threads(ctx)
			if err != nil {
				return nil, nil, err
			}
			var b strings.Builder
			for _, id := range ids {
				fmt.Fprintf(&b, "%s  %s\n", id, threadTitle(ctx, n.Store, id))
			}
			if b.Len() == 0 {
				b.WriteString("no threads yet — create one with create_thread")
			}
			return textResult("%s", b.String()), nil, nil
		})

	type readArgs struct {
		Thread string `json:"thread" jsonschema:"thread id from list_threads"`
		Limit  int    `json:"limit,omitempty" jsonschema:"max entries, newest last (default 50)"`
	}
	sdk.AddTool(s, &sdk.Tool{Name: "read_thread",
		Description: "Read a thread's entries (all lanes this node holds), oldest first."},
		func(ctx context.Context, req *sdk.CallToolRequest, a readArgs) (*sdk.CallToolResult, any, error) {
			entries, err := n.Store.Entries(ctx, a.Thread,
				[]protolog.Lane{protolog.LaneControl, protolog.LaneSummary, protolog.LaneContent})
			if err != nil {
				return nil, nil, err
			}
			limit := a.Limit
			if limit <= 0 {
				limit = 50
			}
			if len(entries) > limit {
				entries = entries[len(entries)-limit:]
			}
			var b strings.Builder
			for _, e := range entries {
				fmt.Fprintf(&b, "[%s|%s] %s %s: %s\n", e.Lane, e.Kind, e.TS, short(e.Author), entryText(e))
			}
			return textResult("%s", b.String()), nil, nil
		})

	type postArgs struct {
		Thread string `json:"thread" jsonschema:"thread id"`
		Text   string `json:"text" jsonschema:"the content to post"`
		Lane   string `json:"lane,omitempty" jsonschema:"content (default) or summary — summaries are what summary-scoped peers see"`
	}
	sdk.AddTool(s, &sdk.Tool{Name: "post_entry",
		Description: "Append a signed entry to a thread. Use lane=summary to publish a shareable summary of the thread's state."},
		func(ctx context.Context, req *sdk.CallToolRequest, a postArgs) (*sdk.CallToolResult, any, error) {
			lane, kind := protolog.LaneContent, protolog.KindMessage
			if a.Lane == "summary" {
				lane, kind = protolog.LaneSummary, protolog.KindSummary
			}
			tl, err := n.Store.Thread(ctx, a.Thread)
			if err != nil {
				return nil, nil, err
			}
			author, err := protolog.NewAuthor(tl, n.Key)
			if err != nil {
				return nil, nil, err
			}
			// Record which agent wrote this so people can tell their own words
			// from their agent's in the interface. The name comes from the MCP
			// client's handshake (e.g. "claude-code"); it is a label, not proof.
			body := map[string]any{"text": a.Text}
			if ip := req.Session.InitializeParams(); ip != nil && ip.ClientInfo != nil && ip.ClientInfo.Name != "" {
				body["agent"] = ip.ClientInfo.Name
			} else {
				body["agent"] = "agent"
			}
			e, err := author.Append(protolog.Draft{Kind: kind, Lane: lane, Body: body})
			if err != nil {
				return nil, nil, err
			}
			if err := n.Store.AppendEntries(ctx, []*protolog.Entry{e}); err != nil {
				return nil, nil, err
			}
			return textResult("posted %s to %s lane", e.ID, lane), nil, nil
		})

	type createArgs struct {
		Title string `json:"title" jsonschema:"short human-readable thread title"`
	}
	sdk.AddTool(s, &sdk.Tool{Name: "create_thread",
		Description: "Create a new context thread owned by this node's person."},
		func(ctx context.Context, req *sdk.CallToolRequest, a createArgs) (*sdk.CallToolResult, any, error) {
			tl, genesis, err := protolog.NewThread(n.Key, a.Title, nil)
			if err != nil {
				return nil, nil, err
			}
			if err := n.Store.AppendEntries(ctx, tl.Entries()); err != nil {
				return nil, nil, err
			}
			return textResult("created thread %s", genesis.ID), nil, nil
		})

	type searchArgs struct {
		Query string `json:"query" jsonschema:"what to look for, natural language"`
		K     int    `json:"k,omitempty" jsonschema:"max results (default 8)"`
	}
	sdk.AddTool(s, &sdk.Tool{Name: "search_context",
		Description: "Hybrid semantic + full-text search across every thread and lane this node holds."},
		func(ctx context.Context, req *sdk.CallToolRequest, a searchArgs) (*sdk.CallToolResult, any, error) {
			k := a.K
			if k <= 0 {
				k = 8
			}
			sr := store.SearchRequest{Query: a.Query, K: k,
				Lanes: []protolog.Lane{protolog.LaneControl, protolog.LaneSummary, protolog.LaneContent}}
			if n.Embedder != nil {
				w := embed.Worker{Store: n.Store, Embedder: n.Embedder}
				for {
					done, err := w.Tick(ctx)
					if err != nil || done == 0 {
						break
					}
				}
				if vecs, err := n.Embedder.Embed(ctx, []string{a.Query}); err == nil {
					sr.QueryVec = vecs[0]
				}
			}
			hits, err := n.Store.Search(ctx, sr)
			if err != nil {
				return nil, nil, err
			}
			var b strings.Builder
			for _, h := range hits {
				fmt.Fprintf(&b, "%.4f thread=%s [%s|%s] %s\n", h.Rank, h.Thread, h.Lane, h.Kind, h.Snippet)
			}
			if b.Len() == 0 {
				b.WriteString("no results")
			}
			return textResult("%s", b.String()), nil, nil
		})

	type reqArgs struct {
		Peer   string `json:"peer" jsonschema:"peer name from whoami"`
		Thread string `json:"thread" jsonschema:"thread title fragment or id, from the peer's discoverable threads"`
		Scopes string `json:"scopes" jsonschema:"comma-separated: summary,search (the gist) or contribute,read,search (full)"`
		Reason string `json:"reason" jsonschema:"one line: why you need it — the human deciding reads this"`
	}
	sdk.AddTool(s, &sdk.Tool{Name: "request_access",
		Description: "Ask another person for access to one of their threads. A HUMAN approves or denies — this tool cannot grant anything."},
		func(ctx context.Context, req *sdk.CallToolRequest, a reqArgs) (*sdk.CallToolResult, any, error) {
			url, ok := n.Peers[a.Peer]
			if !ok {
				return nil, nil, fmt.Errorf("unknown peer %q", a.Peer)
			}
			t := api.NewHTTPTransport(url, n.Key)
			threads, err := t.Discover(ctx)
			if err != nil {
				return nil, nil, err
			}
			target := ""
			low := strings.ToLower(a.Thread)
			for _, th := range threads {
				if th.ID == a.Thread || strings.Contains(strings.ToLower(th.Title), low) {
					if target != "" {
						return nil, nil, fmt.Errorf("%q is ambiguous among their discoverable threads", a.Thread)
					}
					target = th.ID
				}
			}
			if target == "" {
				return nil, nil, fmt.Errorf("no discoverable thread of %s matches %q", a.Peer, a.Thread)
			}
			var scopes []string
			for _, sc := range strings.Split(a.Scopes, ",") {
				scopes = append(scopes, strings.TrimSpace(sc))
			}
			e, err := syncp.BuildAccessRequest(n.Key, target, scopes, a.Reason, nil)
			if err != nil {
				return nil, nil, err
			}
			if err := t.RequestAccess(ctx, e); err != nil {
				return nil, nil, err
			}
			return textResult("requested %s on %s's thread — a human on their side decides", a.Scopes, a.Peer), nil, nil
		})

	sdk.AddTool(s, &sdk.Tool{Name: "list_access_requests",
		Description: "Pending access requests on this node's threads. Surface them to your human — only they can approve (lamdis approve <thread> <who>)."},
		func(ctx context.Context, req *sdk.CallToolRequest, _ empty) (*sdk.CallToolResult, any, error) {
			ids, err := n.Store.Threads(ctx)
			if err != nil {
				return nil, nil, err
			}
			var b strings.Builder
			for _, id := range ids {
				tl, err := n.Store.Thread(ctx, id)
				if err != nil {
					continue
				}
				st := perm.Fold(id, tl.Entries())
				for _, r := range st.PendingRequests() {
					fmt.Fprintf(&b, "%s wants %s on %q — %s\n", short(r.Principal), strings.Join(r.Scopes, ","), st.Title, r.Reason)
				}
			}
			if b.Len() == 0 {
				b.WriteString("no pending requests")
			}
			return textResult("%s", b.String()), nil, nil
		})

	sdk.AddTool(s, &sdk.Tool{Name: "sync_peers",
		Description: "Sync with every configured peer: pull permitted threads, push this node's own entries. Run before answering freshness-sensitive questions."},
		func(ctx context.Context, req *sdk.CallToolRequest, _ empty) (*sdk.CallToolResult, any, error) {
			if len(n.Peers) == 0 {
				return textResult("no peers configured"), nil, nil
			}
			var b strings.Builder
			for name, url := range n.Peers {
				client := &syncp.Client{Store: n.Store, Peer: api.NewHTTPTransport(url, n.Key), Self: n.Self}
				counts, err := client.SyncAll(ctx)
				if err != nil {
					fmt.Fprintf(&b, "%s: error: %v\n", name, err)
					continue
				}
				total := 0
				for _, c := range counts {
					total += c
				}
				fmt.Fprintf(&b, "%s: %d threads visible, %d entries exchanged\n", name, len(counts), total)
			}
			return textResult("%s", b.String()), nil, nil
		})

	return s
}

// RunStdio serves MCP over stdio until the client disconnects.
func RunStdio(ctx context.Context, n *Node, version string) error {
	return NewServer(n, version).Run(ctx, &sdk.StdioTransport{})
}

func textResult(format string, args ...any) *sdk.CallToolResult {
	return &sdk.CallToolResult{Content: []sdk.Content{
		&sdk.TextContent{Text: fmt.Sprintf(format, args...)},
	}}
}

func threadTitle(ctx context.Context, s store.Store, id string) string {
	tl, err := s.Thread(ctx, id)
	if err != nil {
		return ""
	}
	g := tl.Get(id)
	if g == nil {
		return ""
	}
	var body struct {
		Title string `json:"title"`
	}
	json.Unmarshal(g.Body, &body)
	return body.Title
}

func entryText(e *protolog.Entry) string {
	var body struct {
		Text  string `json:"text"`
		Title string `json:"title"`
	}
	json.Unmarshal(e.Body, &body)
	if body.Text != "" {
		return body.Text
	}
	if body.Title != "" {
		return body.Title
	}
	return "(" + e.Kind + ")"
}

func short(principal string) string {
	if len(principal) > 20 {
		return principal[:20] + "…"
	}
	return principal
}
