package exchange

// Being findable by the protocols agents already use.
//
// Search is one way an agent finds a place to post work (see seo.go). The
// other is asking a host directly, at the paths the agent protocols agreed
// on: an A2A client fetches /.well-known/agent-card.json before it does
// anything else, and any HTTP client that wants to know what a REST API
// accepts looks for its OpenAPI document. Neither existed here, so an agent
// that arrived at this host by name learned nothing it could act on.
//
// Two things live here. The A2A Agent Card, which describes what the exchange
// can do in the vocabulary that spec defines, and is careful to say what it
// is not: the exchange speaks MCP and REST, not an A2A binding, and the card
// advertises capability rather than a task lifecycle. And the OpenAPI spec,
// embedded at build time from a copy of spec/openapi.yaml so the binary
// serves exactly the document the repository validates.

import (
	_ "embed"
	"encoding/json"
	"net/http"
)

// a2aWellKnownPaths is where an A2A client looks for the card. The 1.0
// specification (and 0.3 before it) names agent-card.json; earlier drafts
// used agent.json and clients built against them still fetch it, so both
// answer.
var a2aWellKnownPaths = []string{
	"/.well-known/agent-card.json",
	"/.well-known/agent.json",
}

// a2aProtocolVersion is the A2A specification version this card is written
// against. It says which fields to expect, not that the exchange speaks an
// A2A binding — the card is explicit that it does not.
const a2aProtocolVersion = "1.0"

// apiVersion is the REST API version, the same string as spec/openapi.yaml's
// info.version. The card's version is the API's version because the API is
// what the card points at.
const apiVersion = "1"

// openapiYAML is spec/openapi.yaml, copied into this package because
// go:embed cannot reach above the module root (node/). The copy is kept
// honest by TestEmbeddedOpenAPIMatchesSpec, which compares it byte-for-byte
// with the original, and refreshed by:
//
//go:generate cp ../../../spec/openapi.yaml openapi/openapi.yaml
//go:embed openapi/openapi.yaml
var openapiYAML []byte

// agentCard is the A2A Agent Card for this exchange, built against the
// configured base URL so a staging deployment describes itself and not
// production.
//
// Field names follow the A2A JSON encoding (camelCase). Where 0.3 and 1.0
// disagree the card carries both spellings — url alongside
// supportedInterfaces, security alongside securityRequirements — so a client
// of either vintage reads the same facts.
func (s *Server) agentCard() map[string]any {
	base := s.base()
	return map[string]any{
		"protocolVersion": a2aProtocolVersion,
		"name":            "Lamdis Exchange",
		"description": "A marketplace where AI agents pay people for " +
			"physical work. An agent states what should become true at a " +
			"place, holds the money for it, and settles against verified " +
			"evidence that it happened. No account, key or card is needed " +
			"for a first job. " +
			"This card advertises capability; it is not an A2A task-lifecycle " +
			"endpoint. The exchange does not implement the A2A JSON-RPC, gRPC " +
			"or HTTP+JSON bindings (no message/send, tasks/get, streaming or " +
			"push notifications). Its native interfaces are MCP at " + base +
			"/mcp (Streamable HTTP; tools: check_feasible, observe_world, " +
			"do_in_world, find_out, job_status, job_receipt, job_evidence, " +
			"list_bids) and REST at " + base + "/v1, described by the OpenAPI " +
			"document at " + base + "/openapi.yaml. " +
			"Coverage today is zero: no operator has registered yet, so a real " +
			"job would sit unclaimed and check_feasible says so. Add " +
			"\"sandbox\": true to see the full claim/evidence/verify/settle " +
			"cycle run in seconds with no credential.",
		// The REST base. Not an A2A endpoint — see the description — but the
		// URL an agent should start from.
		"url":              base + "/v1",
		"version":          apiVersion,
		"documentationUrl": base + "/docs",
		"provider": map[string]any{
			"organization": "Lamdis",
			"url":          base + "/",
		},
		// None of the A2A capabilities are implemented, and saying so is the
		// point: a client that reads true here would try a method that 404s.
		"capabilities": map[string]any{
			"streaming":              false,
			"pushNotifications":      false,
			"stateTransitionHistory": false,
			"extendedAgentCard":      false,
		},
		// The interfaces that actually exist. The binding names are not A2A
		// bindings (JSONRPC, GRPC, HTTP+JSON) on purpose: a conforming A2A
		// client will decline to use them, which is correct, and any other
		// reader learns where to go.
		"supportedInterfaces": []map[string]any{
			{"url": base + "/mcp", "protocolBinding": "MCP"},
			{"url": base + "/v1", "protocolBinding": "OPENAPI"},
		},
		"defaultInputModes":  []string{"application/json", "text/plain"},
		"defaultOutputModes": []string{"application/json"},
		"skills": []map[string]any{
			{
				"id":   "find_out",
				"name": "Find out whether something is true at a place",
				"description": "Send someone to a location to observe and " +
					"report, with photo or other evidence, whether a stated " +
					"predicate holds. The agent gives a place (lat/lon or " +
					"address), a radius and the thing to check; a person goes, " +
					"records what is there, and the exchange verifies the " +
					"evidence and returns a signed receipt. REST: POST /v1/tasks " +
					"with kind \"observe\". MCP: observe_world / find_out.",
				"tags": []string{"observe", "verify", "physical-world",
					"evidence", "ground-truth"},
				"examples": []string{
					"Is the new sign up at the front of 1234 Woodward Ave, Detroit?",
					"Is the store at this address open right now, and what do the posted hours say?",
					"Photograph the meter reading on the unit behind the building.",
					"Is the parking lot at this location more than half full at 5pm?",
				},
				"inputModes":  []string{"application/json", "text/plain"},
				"outputModes": []string{"application/json"},
			},
			{
				"id":   "do_in_world",
				"name": "Have something done at a place",
				"description": "Pay a person to carry out a bounded physical " +
					"task at a location and prove it was done. The agent states " +
					"what should become true, sets a fee ceiling that is held " +
					"(not charged) until the work is verified, and receives a " +
					"receipt settled against evidence. REST: POST /v1/tasks with " +
					"kind \"act\". MCP: do_in_world.",
				"tags": []string{"act", "errand", "physical-world", "escrow",
					"proof-of-completion"},
				"examples": []string{
					"Pick up the package left at the front desk of this building and drop it at the post office.",
					"Put this printed notice on the community board at the library.",
					"Water the plants on the porch at this address and send a photo.",
				},
				"inputModes":  []string{"application/json", "text/plain"},
				"outputModes": []string{"application/json"},
			},
		},
		// Credentials that exist, none of which a first job needs. An empty
		// requirement list means exactly that.
		"securitySchemes": map[string]any{
			"lamdisKey": map[string]any{
				"type": "apiKey", "in": "header", "name": "X-Lamdis-Key",
				"description": "Agent key (lam_sk_...) issued from an " +
					"optional account; adds a balance, spending limits, " +
					"projects, sites and suppliers. Also accepted by /mcp as " +
					"Authorization: Bearer.",
			},
			"jobToken": map[string]any{
				"type": "http", "scheme": "bearer", "bearerFormat": "lbt_...",
				"description": "Per-job token returned by POST /v1/tasks " +
					"when no credential is sent. Reads and controls that one " +
					"job and nothing else.",
			},
		},
		"security":             []any{},
		"securityRequirements": []any{},
	}
}

// registerDiscovery mounts the agent card and the OpenAPI document.
//
// The card is rendered once: it depends on configuration only, and a request
// must not be able to change what the host says about itself.
func (s *Server) registerDiscovery(mux *http.ServeMux) {
	card, err := json.MarshalIndent(s.agentCard(), "", "  ")
	if err != nil {
		// Every value is a literal or a string; this cannot fail, and if it
		// somehow does, a boot that dies is better than a host with no card.
		panic("agent card: " + err.Error())
	}
	serveCard := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write(card)
	}
	for _, p := range a2aWellKnownPaths {
		mux.HandleFunc("GET "+p, serveCard)
	}

	// RFC 9512 registered application/yaml; it is what a client that asked
	// for a YAML document expects to see back.
	mux.HandleFunc("GET /openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write(openapiYAML)
	})
	// There is no YAML parser in this module, so the JSON rendering is not
	// served. A client that guesses the conventional name is told where the
	// document is rather than left with a bare 404.
	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{
			"error":   "the OpenAPI document is served as YAML only",
			"openapi": s.base() + "/openapi.yaml",
		})
	})
}
