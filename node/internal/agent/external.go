package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// External tools are MCP servers the person configures in agent.json. The
// agent is an MCP client to them, sees only the tools the person allowlisted,
// and treats everything they return as data. Tools marked confirm are never
// called until the person says yes to that exact call.

// ExternalCall is one external tool call as recorded in agent.run.
type ExternalCall struct {
	Tool     string `json:"tool"`
	ArgsHash string `json:"args_sha256"`
	Bytes    int    `json:"bytes"`
	Error    string `json:"error,omitempty"`
}

type externalTool struct {
	server  ToolServer
	session *sdk.ClientSession
	tool    *sdk.Tool
	confirm bool
}

// externals holds live sessions for one run.
type externals struct {
	tools    map[string]*externalTool // "server.tool"
	sessions []*sdk.ClientSession
}

// connectExternals opens every configured server and collects allowlisted
// tools. A server that fails to start is reported, not fatal: the run goes
// on with what connected, and the run record says what was missing.
func connectExternals(ctx context.Context, cfg Config, wanted func(name string) bool) (*externals, []string) {
	ex := &externals{tools: map[string]*externalTool{}}
	var problems []string
	for _, srv := range cfg.Tools {
		if srv.Name == "" || (srv.Command == "" && srv.URL == "") || len(srv.Allow) == 0 {
			continue
		}
		anyWanted := false
		for _, t := range srv.Allow {
			if wanted(srv.Name + "." + t) {
				anyWanted = true
				break
			}
		}
		if !anyWanted {
			continue
		}
		var transport sdk.Transport
		if srv.Command != "" {
			cmd := exec.CommandContext(ctx, srv.Command, srv.Args...)
			cmd.Env = append(os.Environ(), srv.Env...)
			transport = &sdk.CommandTransport{Command: cmd}
		} else {
			if _, err := PublicHost(srv.URL); err != nil && !strings.HasPrefix(srv.URL, "http://localhost") && !strings.HasPrefix(srv.URL, "http://127.0.0.1") {
				problems = append(problems, srv.Name+": "+err.Error())
				continue
			}
			transport = &sdk.StreamableClientTransport{Endpoint: srv.URL}
		}
		client := sdk.NewClient(&sdk.Implementation{Name: "lamdis-agent", Version: "1"}, nil)
		sess, err := client.Connect(ctx, transport, nil)
		if err != nil {
			problems = append(problems, srv.Name+": "+err.Error())
			continue
		}
		ex.sessions = append(ex.sessions, sess)
		list, err := sess.ListTools(ctx, nil)
		if err != nil {
			problems = append(problems, srv.Name+": "+err.Error())
			continue
		}
		allow := map[string]bool{}
		for _, t := range srv.Allow {
			allow[t] = true
		}
		confirm := map[string]bool{}
		for _, t := range srv.Confirm {
			confirm[t] = true
		}
		for _, t := range list.Tools {
			if !allow[t.Name] || !wanted(srv.Name+"."+t.Name) {
				continue
			}
			ex.tools[srv.Name+"."+t.Name] = &externalTool{server: srv, session: sess, tool: t, confirm: confirm[t.Name]}
		}
	}
	return ex, problems
}

func (ex *externals) close() {
	for _, s := range ex.sessions {
		s.Close()
	}
}

// specs describes the connected tools to the model. Names use "__" between
// server and tool because OpenAI-style function names cannot contain dots.
func (ex *externals) specs() []ToolSpec {
	var out []ToolSpec
	for name, t := range ex.tools {
		params := map[string]any{"type": "object", "properties": map[string]any{}}
		if t.tool.InputSchema != nil {
			if raw, err := json.Marshal(t.tool.InputSchema); err == nil {
				var m map[string]any
				if json.Unmarshal(raw, &m) == nil && m["type"] != nil {
					params = m
				}
			}
		}
		desc := t.tool.Description
		if t.confirm {
			desc += " (The person confirms each call before it runs.)"
		}
		out = append(out, ToolSpec{Name: modelName(name), Description: desc, Parameters: params})
	}
	return out
}

func modelName(name string) string { return strings.ReplaceAll(name, ".", "__") }
func humanName(name string) string { return strings.ReplaceAll(name, "__", ".") }

// call runs one external tool and returns its text.
func (ex *externals) call(ctx context.Context, name string, args map[string]any) (string, ExternalCall) {
	rec := ExternalCall{Tool: name, ArgsHash: hashArgs(args)}
	t := ex.tools[name]
	if t == nil {
		rec.Error = "no such tool"
		return "", rec
	}
	res, err := t.session.CallTool(ctx, &sdk.CallToolParams{Name: t.tool.Name, Arguments: args})
	if err != nil {
		rec.Error = err.Error()
		return "", rec
	}
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*sdk.TextContent); ok {
			sb.WriteString(tc.Text)
			sb.WriteString("\n")
		}
	}
	out := strings.TrimSpace(sb.String())
	rec.Bytes = len(out)
	if res.IsError {
		rec.Error = "tool reported an error"
	}
	if len(out) > 16_000 {
		out = out[:16_000] + "\n[truncated]"
	}
	return out, rec
}

func hashArgs(args map[string]any) string {
	raw, _ := json.Marshal(args)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8])
}

// untrusted wraps text the agent did not get from the person: a fetched
// page, a tool result. The system prompt says once what the wrapper means.
func untrusted(source, text string) string {
	if strings.TrimSpace(text) == "" {
		text = "(empty)"
	}
	return fmt.Sprintf("<untrusted source=%q>\n%s\n</untrusted>", source, text)
}
