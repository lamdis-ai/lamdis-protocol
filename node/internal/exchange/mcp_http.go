package exchange

import (
	"encoding/json"
	"net/http"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	nodemcp "github.com/lamdis-ai/lamdis-protocol/node/internal/mcp"
)

// The front door.
//
// Using this exchange from an agent meant: clone a repo, build a Go binary,
// write an .mcp.json pointing at it, and put a key in the environment. Three
// gates before anybody has seen a single tool, and every one of them loses
// people who were only ever going to spend ten minutes finding out whether
// this was interesting.
//
// A remote endpoint removes two of them:
//
//	claude mcp add --transport http lamdis https://exchange.lamdis.ai/mcp \
//	  --header "Authorization: Bearer lam_..."
//
// One line, no build, no binary. The key went too, for a first job: with no
// credential the caller gets the guest tools, and a job they post comes back
// with a pay link and a token instead of drawing on a balance (guest.go). The
// tools that spend a balance still need the key that owns it, because an
// endpoint anybody could call anonymously would otherwise be an endpoint
// anybody could spend somebody else's money through.
//
// The security property that makes this safe is worth stating plainly, because
// getting it wrong would be catastrophic and the wrong version looks almost
// identical: the server is built per request, from the key that request
// presented. There is no shared Exchange client and no server-held key. Two
// agents calling this endpoint at the same time are two different principals
// with two different balances, and neither can reach the other's.

// mcpPath is where the endpoint lives. Named once so the docs, the console and
// the handler cannot drift.
const mcpPath = "/mcp"

func (s *Server) registerMCP(mux *http.ServeMux) {
	h := sdk.NewStreamableHTTPHandler(s.mcpServerFor, nil)
	mux.Handle("POST "+mcpPath, s.requireAgentKey(h))
	mux.Handle("GET "+mcpPath, s.requireAgentKey(h))
	mux.Handle("DELETE "+mcpPath, s.requireAgentKey(h))
}

// mcpServerFor builds a server bound to the credential this request presented.
//
// Called per request by the SDK. It reads the key back out of the request
// rather than closing over anything, which is what keeps one caller's tools
// from ever holding another caller's balance.
//
// The credential also decides which half of the exchange you get. An agent key
// belongs to somebody who pays for work, and gets the tools that send work out.
// An operator's session token belongs to somebody who does the work, and gets
// the tools that find it. One URL, because asking a person to know which of two
// endpoints they are is a gate for no reason — and because plenty of people are
// both.
func (s *Server) mcpServerFor(r *http.Request) *sdk.Server {
	srv := sdk.NewServer(&sdk.Implementation{
		Name: "lamdis-exchange", Version: "1",
	}, nil)
	key := agentKeyFrom(r)
	if key == "" {
		// Nobody at all. The gateless surface: reads, the feasibility check,
		// and posting a job that comes back with a pay link and a token.
		nodemcp.RegisterGuest(srv, nodemcp.NewExchange(s.BaseURL, ""))
		return srv
	}

	buyer := false
	if s.agents != nil {
		probe := r.Clone(r.Context())
		probe.Header.Set("X-Lamdis-Key", key)
		_, _, buyer = s.agents.AuthenticateAgent(probe)
	}
	if buyer {
		nodemcp.RegisterExchange(srv, nodemcp.NewExchange(s.BaseURL, key))
		return srv
	}
	// Otherwise it is an operator's own token, and they get the supply side.
	nodemcp.RegisterOperator(srv, nodemcp.NewOperator(s.BaseURL, key))
	return srv
}

// agentKeyFrom reads the caller's key from either header.
//
// Authorization: Bearer is what every MCP client sends, because it is what the
// --header flag is for and what OAuth would use. X-Lamdis-Key is what the rest
// of this API already takes. Accepting both means the documented one-liner
// works and nothing that already worked stops.
func agentKeyFrom(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("Authorization")); v != "" {
		if len(v) > 7 && strings.EqualFold(v[:7], "bearer ") {
			return strings.TrimSpace(v[7:])
		}
		return v
	}
	return strings.TrimSpace(r.Header.Get("X-Lamdis-Key"))
}

// requireAgentKey refuses an unauthenticated caller before the MCP machinery
// runs, and says how to get a key rather than returning a bare 401.
//
// The refusal is deliberately helpful. Somebody hitting this has already
// decided to try the exchange; sending them away with "unauthorized" and no
// next step wastes the one moment they were willing to spend on it.
func (s *Server) requireAgentKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := agentKeyFrom(r)
		if key == "" {
			// No credential is not an error any more: the caller gets the
			// guest tools, and a job they post comes back with a pay link.
			next.ServeHTTP(w, r)
			return
		}
		// Checked here so a bad credential fails once, plainly, rather than as
		// an error inside every tool call the agent then tries.
		//
		// Either credential is valid: an agent key for somebody buying work, an
		// operator's own session token for somebody doing it.
		if !s.knownCredential(r, key) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]any{
				"error": "that key is not valid on this exchange",
				"how": "To send work out, issue an agent key at " + s.BaseURL +
					"/console under Integration. To find work, sign in at " +
					s.BaseURL + "/signin and use the token from your console.",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// knownCredential reports whether this is somebody we recognise, either side.
func (s *Server) knownCredential(r *http.Request, key string) bool {
	if s.agents != nil {
		probe := r.Clone(r.Context())
		probe.Header.Set("X-Lamdis-Key", key)
		if _, _, ok := s.agents.AuthenticateAgent(probe); ok {
			return true
		}
	}
	if s.Workers != nil {
		probe := r.Clone(r.Context())
		probe.Header.Set("Authorization", "Bearer "+key)
		if _, err := s.Workers.Authenticate(probe, nil, s.now()); err == nil {
			return true
		}
	}
	return false
}
