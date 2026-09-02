# Listing copy

Reuse these verbatim. Plain language, no hype, no emoji.

## Tagline (60 characters or fewer)

    Pay a person nearby to check it, proved with a photo

## Short description (100 characters or fewer, for the MCP Registry)

    Pay people nearby for physical work: find out if something is true, or have it done.

## One-line description (directories that want one sentence)

    An agent pays people nearby for physical work and gets photo evidence back; money moves only on proof.

## Paragraph

Lamdis Exchange is a remote MCP server where an agent pays people nearby to do
physical work: find out whether something is true at an address, or have
something done. Whoever goes photographs it with a one-time code, the evidence
is checked, and money moves only on proof. The exchange takes no fee. Connect
over streamable HTTP at https://exchange.lamdis.ai/mcp; with no credential an
agent gets eight tools (check_feasible, observe_world, do_in_world, find_out,
job_status, job_receipt, job_evidence, list_bids), and a job posted that way
comes back with a pay link and a token to follow it. An agent key
(Authorization: Bearer lam_sk_...) opens the rest of the buyer side, and an
operator session token opens the supply side, where a worker's agent finds and
takes jobs.

## Connection details

- Transport: streamable HTTP
- URL: https://exchange.lamdis.ai/mcp
- Auth: none for the guest tools; `Authorization: Bearer lam_sk_...` agent key
  for the full buyer side; operator session token for the supply side
- One-liner: `claude mcp add --transport http lamdis https://exchange.lamdis.ai/mcp`
- Source: https://github.com/lamdis-ai/lamdis-protocol
- Site: https://lamdis.ai · Docs: https://exchange.lamdis.ai/docs
- Categories to pick where a directory asks: productivity / other / logistics /
  location services

## Entry lines, per list

### punkpeye/awesome-mcp-servers — Agreements & Coordination

- [lamdis-ai/lamdis-protocol](https://github.com/lamdis-ai/lamdis-protocol) 🏎️ ☁️ - An agent pays people nearby for physical work: find out whether something is true at an address, or have something done. Proof is a photograph carrying a one-time code; money moves only on proof and the exchange takes no fee. Remote streamable HTTP at https://exchange.lamdis.ai/mcp — eight tools with no credential.

### appcypher/awesome-mcp-servers — Robotics & Physical AI

- <img height="14" src="https://www.google.com/s2/favicons?domain=lamdis.ai&sz=64" alt="Lamdis logo"> [Lamdis Exchange](https://github.com/lamdis-ai/lamdis-protocol) - Pay people nearby for physical work: find out whether something is true at an address, or have something done, with a photograph carrying a one-time code as proof. Remote MCP at https://exchange.lamdis.ai/mcp.

### jaw9c/awesome-remote-mcp-servers — table row

| Lamdis Exchange | Physical Tasks | `https://exchange.lamdis.ai/mcp` | Open / API Key | [Lamdis](https://lamdis.ai) |

### mcp.so — issue body for github.com/chatmcp/mcpso/issues/new

    Name: Lamdis Exchange
    Repo: https://github.com/lamdis-ai/lamdis-protocol
    Website: https://lamdis.ai
    MCP endpoint: https://exchange.lamdis.ai/mcp (streamable HTTP)
    Description: An agent pays people nearby for physical work: find out whether
    something is true at an address, or have something done. Proof is a photograph
    carrying a one-time code; money moves only on proof and the exchange takes no
    fee. No credential is needed for eight tools: check_feasible, observe_world,
    do_in_world, find_out, job_status, job_receipt, job_evidence, list_bids.

### mcpservers.org/submit — form fields

    Server Name: Lamdis Exchange
    Short Description: Pay people nearby for physical work: find out if something is true, or have it done.
    Link: https://github.com/lamdis-ai/lamdis-protocol
    Category: Other
    Contact Email: (your address)

### Glama connector form (glama.ai/mcp/servers -> Add MCP Server -> Connector)

    Name: Lamdis Exchange
    Short description: Pay people nearby for physical work, proved by a photo with a one-time code.
    Server URL: https://exchange.lamdis.ai/mcp
    Transport: streamable-http
    Test credentials: not needed — the guest tools work with no credential

### Smithery (smithery.ai/new, URL method)

    Public HTTPS URL: https://exchange.lamdis.ai/mcp
    Title: Lamdis Exchange
    Description: (the paragraph above)
    Auth: none required for the guest tools; optional bearer agent key

### PulseMCP (pulsemcp.com/submit)

    Name: Lamdis Exchange
    URL: https://exchange.lamdis.ai/mcp
    Source: https://github.com/lamdis-ai/lamdis-protocol
    Description: (the paragraph above)

### Cursor (cursor.directory/plugins/new)

    Name: Lamdis Exchange
    URL / endpoint: https://exchange.lamdis.ai/mcp
    Description: (the one-line description above)
    Link: https://github.com/lamdis-ai/lamdis-protocol
