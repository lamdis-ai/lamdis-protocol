# Connector directory submissions (need an account holder)

Both are where end users of Claude and ChatGPT discover remote MCP servers.
Both take the same endpoint and both require the account owner to submit.

## Anthropic — Claude connectors directory

- Endpoint: `https://exchange.lamdis.ai/mcp` (streamable HTTP — the only transport they accept)
- Auth: none for the guest tools; optional bearer `lam_sk_…` for the full buyer side
- Requirements their form asks for: a privacy policy URL (`https://lamdis.ai/privacy/`),
  terms (`https://lamdis.ai/terms/`), a support contact (`hello@lamdis.ai`), a logo
  (`https://lamdis.ai/lamdis-logo.png`), and a one-paragraph description — use the
  paragraph in `copy.md`. Submissions without a privacy policy are auto-rejected.
- Where: Claude.ai → Settings → Connectors → "Submit a connector" (organisation owner).

## OpenAI — ChatGPT connectors / Apps SDK

- ChatGPT connects to remote MCP servers directly; the same endpoint works.
- Developer mode: ChatGPT → Settings → Connectors → Advanced → Developer mode →
  add `https://exchange.lamdis.ai/mcp`, no auth. That is enough for your own account
  and for anyone you share the link with.
- Public directory listing goes through the Apps SDK submission (OpenAI developer
  account, app review). Use the same description and the OpenAPI at
  `https://exchange.lamdis.ai/openapi.json` if they ask for it.

## What to paste (both)

Tagline: Pay a person nearby to check it, proved with a photo
Description: see `copy.md` → "Paragraph".
Category: Productivity / Other.
