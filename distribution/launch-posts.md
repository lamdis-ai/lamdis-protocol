# Launch posts (owner posts these; accounts are yours)

Every number below is grepped from the code. Do not add any that are not.

## Show HN (news.ycombinator.com/submit)

Title: Show HN: Lamdis – an API that pays a person nearby to check something, proved with a photo

URL: https://exchange.lamdis.ai/docs

First comment (post immediately after submitting):

I built this because my agents kept hitting the same wall: they can write the email, book the thing, file the claim, but they cannot find out whether the shop is actually open, whether the sign went up, whether the unit is really vacant.

Lamdis is a marketplace where an agent (or a script, or you) posts a small physical job at a lat/lon, a person nearby takes it, and the money only moves when their photo carries a one-time code that was issued for that job. No account is needed to try it; the sandbox runs the whole state machine with no money:

    curl -s -X POST https://exchange.lamdis.ai/v1/tasks \
      -H 'content-type: application/json' \
      -d '{"sandbox":true,"kind":"observe","predicate":"the shop at 123 Main St is open","lat":42.33,"lon":-83.05,"radius_m":150,"fee_minor":800}'

Real jobs are card-authorised, captured only on a verified photo, with a 7-day dispute window and named grounds. Platform fee is zero; I am trying to get adoption and cover proof-server costs, not extract a cut.

It speaks MCP (https://exchange.lamdis.ai/mcp), A2A (/.well-known/agent-card.json), plain REST (OpenAPI at /openapi.yaml), and there are LangChain, OpenAI Agents SDK, CrewAI, and Vercel AI SDK tool packages in the repo.

Honest state: supply is thin. If you are in Detroit and want to be one of the first people taking jobs, https://exchange.lamdis.ai/coverage. If you are building agents that need eyes on the ground, I would like to hear what you would ask for first.

Server is source-available (FSL-1.1-MIT, converts to MIT after two years); spec, SDKs, and receipt format are Apache-2.0. https://github.com/lamdis-ai/lamdis-protocol

## r/ClaudeAI and r/mcp

Title: An MCP server that pays a real person nearby to check something for your agent (free sandbox, no signup)

Body:

Add `https://exchange.lamdis.ai/mcp` as a remote MCP server (no auth) and your agent gets three tools: check whether a physical job is feasible at a location, run one, and read its status. With `sandbox: true` nothing is charged and nobody is dispatched; the whole flow runs synthetically so you can build against it.

Or in Claude Code:

    claude plugin marketplace add lamdis-ai/lamdis-plugins
    claude plugin install lamdis@lamdis-plugins

What it is for: "is this store open", "photograph the sign", "is this unit vacant", "read the meter". The worker's photo has to carry a one-time code issued for that job before money moves. Fee is zero.

What it is not yet: widely covered. First city is Detroit and it needs operators. Feedback on the tool descriptions from anyone who wires it into an agent is the most useful thing right now.

Repo: https://github.com/lamdis-ai/lamdis-protocol

## r/LangChain / r/AI_Agents

Title: LangChain / OpenAI Agents / CrewAI tools for hiring a person to verify something in the real world (sandbox is free)

Body:

    pip install "git+https://github.com/lamdis-ai/lamdis-protocol.git#subdirectory=integrations/langchain"

Three tools: `lamdis_check_feasible`, `lamdis_run_job` (sandbox by default), `lamdis_job_status`. The sandbox runs the full job lifecycle with no money and no account, so an agent can be built and tested end to end before anything real is posted. Real jobs are paid on a verified photo. Fee is zero.

Same three tools exist for the OpenAI Agents SDK, CrewAI, and the Vercel AI SDK, all in `integrations/` in the repo. I want to know what breaks when you wire it in.
