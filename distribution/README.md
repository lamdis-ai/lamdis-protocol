# Distribution

Getting the Lamdis Exchange MCP server (remote, streamable HTTP,
`https://exchange.lamdis.ai/mcp`) listed where agents look for tools.

Copy to paste into forms: [`copy.md`](copy.md).
MCP Registry entry: [`server.json`](server.json) (domain namespace) and
[`server.github.json`](server.github.json) (GitHub namespace).
Docker catalog entry: [`docker-mcp-registry/lamdis/`](docker-mcp-registry/lamdis).

Both `server.json` files validate against
`https://static.modelcontextprotocol.io/schemas/2025-12-11/server.schema.json`
(ajv, draft-07). Note the registry's 100-character cap on `description`.

## Status

| Target | How it accepts a listing | Remote HTTP? | Status |
|---|---|---|---|
| [MCP Registry](https://registry.modelcontextprotocol.io) | `mcp-publisher` CLI publishes `server.json`; namespace must be proven | yes (`remotes[].type: streamable-http`) | **published** — `ai.lamdis/exchange` 0.1.0, DNS namespace proven by a TXT record on the apex; key at `~/.lamdis/mcp-registry.key` |
| [punkpeye/awesome-mcp-servers](https://github.com/punkpeye/awesome-mcp-servers) | PR editing `README.md` | yes | PR open: https://github.com/punkpeye/awesome-mcp-servers/pull/13498 |
| [jaw9c/awesome-remote-mcp-servers](https://github.com/jaw9c/awesome-remote-mcp-servers) | PR adding a table row | remote only | PR open: https://github.com/jaw9c/awesome-remote-mcp-servers/pull/710 |
| [docker/mcp-registry](https://github.com/docker/mcp-registry) (Docker MCP Catalog / Docker Desktop) | PR adding `servers/<name>/{server.yaml,tools.json,readme.md}` | yes (`type: remote`) | PR open: https://github.com/docker/mcp-registry/pull/4896 |
| [mcp.so](https://mcp.so) | GitHub issue on `chatmcp/mcpso` | yes | issue filed: https://github.com/chatmcp/mcpso/issues/3898 |
| [appcypher/awesome-mcp-servers](https://github.com/appcypher/awesome-mcp-servers) | was a PR to `README.md` | — | **dead end** — repo archived 2026; entry text kept in `copy.md` if it reopens |
| [modelcontextprotocol/servers](https://github.com/modelcontextprotocol/servers) | no longer lists third-party servers; README points at the MCP Registry | — | **nothing to do** — covered by the MCP Registry row |
| [Glama](https://glama.ai/mcp/servers) | web: *Add MCP Server → Connector*; needs a Glama sign-in to claim the listing later | yes (`streamable-http` only) | **paste** — fields in `copy.md` |
| [Smithery](https://smithery.ai/new) | web: URL method, paste the public HTTPS endpoint; Smithery scans it for tools | yes | **paste** — fields in `copy.md` |
| [PulseMCP](https://www.pulsemcp.com/submit) | web form | yes | **paste** — form was paused for a pipeline rebuild; check it is reopened first |
| [mcpservers.org](https://mcpservers.org/submit) | web form (this is where `wong2/awesome-mcp-servers` sends submissions; that repo takes no PRs) | yes | **paste** — fields in `copy.md`; free listing, the $39 option is optional |
| [Cursor](https://cursor.directory/plugins/new) | web form on cursor.directory (the old `cursor/mcp-servers` repo is archived and redirects here) | yes | **paste** — fields in `copy.md` |
| n8n MCP servers registry | no public submission path documented; the node panel list is curated by n8n. Users can already connect via the **MCP Client Tool** node with the URL | yes | **no action** — nothing to submit; document the MCP Client Tool URL instead |
| LangChain / LlamaIndex | neither runs a server directory; `langchain-mcp-adapters` and `llama-index-tools-mcp` consume any MCP server by URL | yes | **no action** — worth an examples/ snippet, not a listing |

## MCP Registry

The registry is a CLI publish, and it needs a human once for the namespace, so
it is left for you.

    brew install mcp-publisher   # or the release tarball from modelcontextprotocol/registry

Two namespace options; `server.json` is written for the first.

1. **`ai.lamdis/exchange`** — the reverse-DNS of `lamdis.ai`, proven by a key
   pair. Either a TXT record on the **apex** of `lamdis.ai` (not a selector
   name), or a file at `https://lamdis.ai/.well-known/mcp-registry-auth`:

       KEY=$(openssl genpkey -algorithm ed25519 -outform DER | tail -c 32 | xxd -p -c 64)
       # publish the printed TXT record / well-known file, then:
       mcp-publisher login dns  --domain lamdis.ai --private-key "$KEY"
       mcp-publisher login http --domain lamdis.ai --private-key "$KEY"
       mcp-publisher publish    # from this directory

   Ed25519 keygen needs OpenSSL 3, not macOS LibreSSL (`brew install openssl@3`).

2. **`io.github.lamdis-ai/exchange`** (`server.github.json`) — GitHub device
   login, no DNS work. `sterlingbm` is an Owner of `lamdis-ai`, which is what
   the registry requires for an org namespace:

       mcp-publisher login github     # device code in the browser
       cp server.github.json server.json && mcp-publisher publish

Either way: versions are immutable, publishing is one-way (no unpublish), and
updates mean publishing a new `version`. `description` is capped at 100
characters. Getting into the official registry is also the cheapest route into
the aggregators that mirror it.
