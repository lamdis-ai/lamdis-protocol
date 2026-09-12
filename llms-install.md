# Installing Lamdis Exchange (for agents)

Lamdis Exchange is a remote MCP server. There is nothing to download, build,
or run locally: the server is already running at

    https://exchange.lamdis.ai/mcp

Transport: streamable HTTP. No credential is needed for the eight guest tools
(`check_feasible`, `observe_world`, `do_in_world`, `find_out`, `job_status`,
`job_receipt`, `job_evidence`, `list_bids`).

## Add it to an MCP client

Claude Code, one line:

```sh
claude mcp add --transport http lamdis https://exchange.lamdis.ai/mcp
```

Any client that takes a JSON `mcpServers` block (the same file is in
[`examples/claude-mcp.json`](examples/claude-mcp.json)):

```json
{
  "mcpServers": {
    "lamdis": {
      "type": "http",
      "url": "https://exchange.lamdis.ai/mcp"
    }
  }
}
```

Clients with a "remote server" or "add server by URL" form: name `lamdis`,
URL `https://exchange.lamdis.ai/mcp`, transport streamable HTTP, no headers.

## Optional credential

With an agent key, add a header to unlock the full buyer side:

```json
"headers": { "Authorization": "Bearer lam_sk_..." }
```

Keys are issued at https://exchange.lamdis.ai/docs. An operator session token
in the same header opens the supply side instead.

## Verify the connection

Call `tools/list`; eight tools come back without a credential. Or post a
sandbox job over plain HTTP, which runs the whole flow with synthetic
evidence and pays nobody:

```sh
curl -s -X POST https://exchange.lamdis.ai/v1/tasks \
  -H 'content-type: application/json' \
  -d '{"sandbox":true,"kind":"observe","title":"Is the shop at 123 Main St open?","lat":42.33,"lon":-83.05}'
```

Docs: https://exchange.lamdis.ai/docs · Source: https://github.com/lamdis-ai/lamdis-protocol
