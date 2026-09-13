# Integrations

Give an agent the Lamdis Exchange as a tool, in the framework it already uses.
Each directory is a self-contained package that installs straight from this
repository; nothing here is on npm or PyPI.

| framework | package | install |
|---|---|---|
| [LangChain](langchain/) | `lamdis_langchain` | `pip install "git+https://github.com/lamdis-ai/lamdis-protocol.git#subdirectory=integrations/langchain"` |
| [OpenAI Agents SDK](openai-agents/) | `lamdis_openai_agents` | `pip install "git+https://github.com/lamdis-ai/lamdis-protocol.git#subdirectory=integrations/openai-agents"` |
| [CrewAI](crewai/) | `lamdis_crewai` | `pip install "git+https://github.com/lamdis-ai/lamdis-protocol.git#subdirectory=integrations/crewai"` |
| [Vercel AI SDK](vercel-ai/) | `@lamdis/ai-sdk-tools` | `pnpm add "github:lamdis-ai/lamdis-protocol#path:integrations/vercel-ai"` — npm: `npx degit lamdis-ai/lamdis-protocol/integrations/vercel-ai vendor/lamdis` then `npm install ./vendor/lamdis` |

Every package exposes the same three tools, named the same way:

| tool | call | what comes back |
|---|---|---|
| `lamdis_check_feasible` | `POST /v1/quote` | `reachable`, `feasible`, `why`, `advice`, `settled_here` |
| `lamdis_run_job` | `POST /v1/tasks` — **`sandbox: true` by default** | sandbox: `job`, `token`, `status` URL; real: `pay_at`, `token`, `amount_minor` |
| `lamdis_job_status` | `GET /v1/jobs/{job}` with the `lbt_` token | `taken`, `submissions`, `results[]` |

None of them needs an account, a key or a card. A sandbox job costs nothing
and involves nobody: it walks the real state machine — taken, evidence
submitted, verified, settled, receipt — against a simulated operator in about
ten seconds, and every reply says so in words. Set `sandbox=false` to post a
real job; that returns a `pay_at` link to hand the person and a token to
follow the job with.

Each package has a smoke test that calls the live sandbox; they are how these
were checked. Set `LAMDIS_BASE_URL` to point any of them at another exchange.
