# The AI agent infrastructure landscape, September 2026

Research completed 2026-09-13. Every company named has a URL. Figures were taken
from primary sources — company sites, protocol specs, IETF Datatracker, GitHub
issue trackers, EU institutional pages — wherever possible. Anything that could
not be confirmed from a primary source is marked **[UNVERIFIED]**.

A method note that matters for how much weight to put on this: mid-research the
session's web-search budget ran out, so the later work was done by direct fetches
against primary URLs, the GitHub API, and the Hacker News Algolia API. That
turned out to *improve* the quality of the evidence — protocol issue trackers and
IETF Datatracker are far better sources for "is this layer real" than search
results are — but it means coverage is deep on protocols and standards and
thinner on private funding rounds. Several vendor funding figures are marked
unverified for exactly that reason. Reddit blocked automated access throughout,
so builder sentiment is Hacker News and GitHub only.

Some figures here are **original measurement rather than reporting**: the full
official MCP Registry was paginated (102,569 records), its Prometheus `/metrics`
read, and 400 randomly-sampled registered servers liveness-probed with a real MCP
handshake, all on 2026-09-13. Where that contradicts the common narrative — it
does, in both directions — the measurement is stated with its limits.

---

## The map in one table

| Layer | State | Serious players | What is actually missing |
|---|---|---|---|
| **Payment rails** | **Crowded, fragmenting** | x402 (Linux Foundation), MPP (Stripe+Tempo), UCP (Google+Shopify), AP2 (→FIDO), ACP (OpenAI+Stripe), Visa, Mastercard, Amex, Cloudflare, Circle, Coinbase | Nothing. Six protocols for a market doing ~$24M/month, roughly half of it wash trading |
| **Payment trust layer** | **Empty** | — | Refunds, disputes, chargebacks, invoicing/VAT, proof of delivery, verifiable metering, liability for agent error |
| **Identity: base plumbing** | **Converged** | RFC 9421, RFC 9728, RFC 8707, CIMD, ID-JAG; Cloudflare, Okta, Microsoft, Google | Nothing. This part genuinely got solved |
| **Identity: agent-as-principal** | **Empty core, crowded periphery** | ~40 vendors, mostly repositioned NHI/PAM; 5 acquired in 16 months | Delegation across N hops, per-task scoping, proving an agent acts for a given human, revocation |
| **Discovery / registries** | **Cataloged, not discoverable** | MCP Registry (**31,680 unique servers, measured**), Smithery, Glama, PulseMCP, Docker MCP Catalog (300, curated), Composio | Semantic runtime discovery and functional verification — both **out of scope by design** |
| **A2A / orchestration** | **Emerging cross-org; crowded at orchestration** | A2A v1.0 (**1,956 repos**) vs LangGraph (**77,824**); Temporal ($5B), Sierra ($15B), CrewAI, MS Agent Framework | Identity, accountability, idempotency. `COMPLETED` that means "correct", not "stopped" |
| **Memory / context** | **Crowded; losing to the baseline** | Mem0, Zep, Cognee, Supermemory; **Letta exited the category**; AWS/MS/Anthropic/Google all ship it | Beating naive long-context. Measured **31–33% less accurate at 14–77× the cost** |
| **Human-in-the-loop** | **Empty as a category, solved as a feature** | No funded pure-plays left; HumanLayer pivoted away | Nothing worth a company. Free in every framework *and* in MCP Tasks / A2A |
| **Observability / evals** | **Crowded, consolidating** | LangSmith, Braintrust, Arize, Comet Opik, Datadog; Langfuse→ClickHouse, W&B→CoreWeave, Humanloop→Anthropic | Nothing structural. ~30 players; 8 acquired, 2 dead, 1 pivoted out in 13 months |
| **Sandboxing / execution** | **Crowded, commoditizing, load-bearing** | Modal (only one publishing revenue), E2B, Daytona, Cloudflare, Vercel, Fly; Browserbase, Browser Use | Nothing structural. Anthropic lists 11 interchangeable backends; no pricing power |
| **Guardrails / prompt-injection** | **Consolidated and technically discredited** | 12 acquisitions; Lakera→Check Point, Prompt→SentinelOne, Invariant→Snyk, Aim→Cato | Honesty. 12 published defenses broken at >90% attack success rate |
| **Governance / GRC compliance** | **Consolidating (closed)** | Credo AI, Holistic AI, Vanta, Drata, Trustible, OneTrust, Zenity, Obsidian, Microsoft Purview | Nothing. Gartner MQ exists, $1B M&A cleared |
| **Portable agent receipts** | **Empty** | Handshake.AI (pre-product) | A signed, portable record a *third party* can verify. Everything shipping is first-party |
| **Outcome verification** | **Empty** | Nobody with traction | Adjudication. Every protocol ships evidence and explicitly defers judgment |

---

## 1. Payments and commerce — crowded rails, empty trust layer

### The protocols

There are now at least **six competing payment protocols**, and the same
companies are funding several of them at once.

| Protocol | Owner | Governance | Rails |
|---|---|---|---|
| [x402](https://www.x402.org/) | Coinbase | [Linux Foundation](https://www.linuxfoundation.org/press/linux-foundation-announces-operational-launch-of-x402-foundation-to-standardize-internet-native-payments-for-ai-agents-and-applications), operational July 2026, 40 members | Stablecoin-first |
| [MPP](https://mpp.dev) | **Stripe** + [Tempo](https://tempo.xyz) | [IETF individual draft](https://datatracker.ietf.org/doc/draft-ryan-httpauth-payment/), not WG-adopted | Payment-method-agnostic |
| [UCP](https://techcrunch.com/2026/01/11/google-announces-a-new-protocol-to-facilitate-commerce-using-ai-agents/) | Google + Shopify | Vendor-led | Cards, Google Pay, PayPal |
| [AP2](https://ap2-protocol.org/) | Google | [Donated to FIDO Alliance](https://blog.google/products-and-platforms/platforms/google-pay/agent-payments-protocol-fido-alliance/), April 2026 | Cards, plus x402 as a rail |
| [ACP](https://www.agenticcommerce.dev/) | OpenAI + Stripe | No standards body | Cards via Stripe |
| L402 | Lightning Labs | De facto | Bitcoin Lightning |

The hedging is systematic. **Stripe** co-authored ACP with OpenAI, co-authored
MPP with Tempo, sits as a Premier member of the x402 Foundation, and now leads
with **UCP** on its own [agentic commerce page](https://stripe.com/use-cases/agentic-commerce)
— which does not mention ACP, x402 or AP2 at all. **Google** maintains both AP2
and UCP. **Visa** and **Mastercard** are Premier x402 members while running their
own competing identity schemes.

### The volume reality

x402 is the only protocol in the category with independently auditable volume,
which is precisely why it is the only one we can prove is small:

- **75.41M transactions / $24.24M** in 30 days — average payment **~$0.32**
  ([x402.org](https://www.x402.org/), corroborated by [Circle](https://www.circle.com/blog/introducing-circle-agent-stack-financial-infrastructure-for-the-agentic-economy))
- [CoinDesk, March 2026](https://www.coindesk.com/markets/2026/03/11/coinbase-backed-ai-payments-protocol-wants-to-fix-micropayment-but-demand-is-just-not-there-yet):
  Artemis analysis found **~50% of transactions are artificial** — self-dealing
  and wash trading
- An [independent survey of the 14,865-listing x402 Bazaar](https://github.com/rikocr8orh8/x402-bazaar-survey/blob/master/POST.md)
  found only **520 listings (3.5%) with genuine repeat demand** and total real
  GMV of **$3,000–6,000 per month** — and the biggest real endpoints are Twitter
  search, Tavily and Exa, i.e. established products wrapped in a paywall
- [Chainalysis](https://www.chainalysis.com/blog/x402-agentic-payments-adoption/)
  traced the Q4 2025 surge to a meme-coin mint, and found x402 wallets look like
  crypto traders, not commerce users

Do not read x402's measurability as x402 being worse than its rivals. **Not one
of UCP, MPP, ACP, AP2, Visa TAP, Mastercard Agent Pay, Circle Agent Stack,
Cloudflare Monetization Gateway or Stripe's Agentic Commerce Suite has published
a single transaction count or GMV figure.**

### The cautionary tale

OpenAI's **Instant Checkout** launched September 2025 and was **killed in March
2026**. Per [Modern Retail's post-mortem](https://www.modernretail.co/technology/what-went-wrong-with-chatgpts-instant-checkout/):
only **~30 Shopify merchants** ever onboarded against "over a million" promised;
**Walmart measured 3× worse conversion** in-chat than click-through to its own
site; Etsy saw no meaningful volume. Merchants did not want it.

The lesson the whole industry absorbed: **"discover in AI, buy on site" beat
centralized agent checkout, with data.** UCP's design internalises this. ACP's
did not, and ACP lost.

### What builders complain about

From the [Cloudflare Monetization Gateway HN thread](https://news.ycombinator.com/item?id=48746914)
(355 points, 251 comments) and the x402 issue tracker — note that none of these
are technical, they are all business-layer:

- **Invoicing and VAT are entirely unsolved.** *"Someone makes a request to my
  company's paid service, I return 402 and get a stablecoin back. Who do I invoice
  for this revenue? What value added tax do I apply?"*
- **No refunds, no chargebacks, no disputes.** The x402 FAQ says the `exact`
  scheme is *"a push payment — irreversible once executed."*
- **No proof of delivery.** You can prove you paid; you cannot prove you got the
  thing ([#2833](https://github.com/x402-foundation/x402/issues/2833),
  [#1195](https://github.com/x402-foundation/x402/issues/1195))
- **Metering is trust-me-bro by spec admission.** [#3001](https://github.com/x402-foundation/x402/issues/3001):
  the `upto` scheme lets a malicious server charge the full authorized maximum;
  the client's signature proves the ceiling, never the correct amount
- **Sellers carry counterparty risk.** [#1645](https://github.com/x402-foundation/x402/issues/1645)
  documents that a server can call `/settle` immediately and return nothing, and
  that the same signed auth replayed to N servers makes N−1 of them work for free
- **The registry doesn't work.** [#3284](https://github.com/x402-foundation/x402/issues/3284):
  a developer settled a real payment, spec-compliant, and still wasn't indexed

### Where the money is actually moving

The clearest signal is what YC is funding. Not more payment protocols —
**guardrails around them**: [Allowance](https://useallowance.com) (spend control),
[Mount](https://mount.insure) (agent insurance), [ORO AI](https://www.oroagents.com)
(agentic commerce evals), [Agentcard](https://agentcard.sh) (S26, cards for
agents), [Orthogonal](https://orthogonal.com) (W26, $4.3M led by Pantera) — which
already abstracts over *both* x402 and MPP because it assumes neither wins.

And the only shipped answer anywhere to "who eats the loss when my agent screws
up" is **Amex's Agent Purchase Protection** (April 2026), which underwrites
cardholders against AI agent errors
([PYMNTS](https://www.pymnts.com/news/artificial-intelligence/2026/american-express-to-back-purchases-made-by-customers-ai-agents/)).
One company, out of the entire industry, has taken a position on liability.

**Verdict: rails CROWDED and commoditizing. Trust layer EMPTY.**

---

## 2. Identity, authentication, authorization — converged plumbing, empty core

### What genuinely got solved

The base layer converged, and this deserves credit:

| Primitive | Standard | Status |
|---|---|---|
| Request signing | RFC 9421 HTTP Message Signatures | Published RFC — universal substrate |
| Resource discovery | RFC 9728 | RFC; MCP servers **MUST** implement |
| Audience binding | RFC 8707 | RFC; MCP clients **MUST** send `resource` |
| Client registration | [CIMD](https://datatracker.ietf.org/doc/draft-ietf-oauth-client-id-metadata-document/) | **WG-adopted**; a URL *is* the client ID |
| Enterprise delegation | [ID-JAG](https://datatracker.ietf.org/doc/draft-ietf-oauth-identity-assertion-authz-grant/) | **WG document** |

The single most impressive convergence in the whole landscape: Okta's Cross App
Access went vendor → IETF → **stable MCP Enterprise-Managed Authorization
extension**, shipping as [Okta Agent SSO, GA 24 August 2026](https://www.okta.com/newsroom/press-releases/okta-brings-first-class-identity-to-ai-agents-with-agent-sso/),
included in core Okta SSO plans at no extra charge.

Also genuinely shipped: **[Cloudflare Web Bot Auth](https://developers.cloudflare.com/bots/concepts/bot/verified-bots/web-bot-auth/)**
and [signed agents](https://blog.cloudflare.com/signed-agents/) (GA Aug 2025),
now with [BotBase](https://blog.cloudflare.com/botbase-for-operators/) (GA Aug
2026) — cryptographic agent provenance at internet scale, and the only
mass-deployed agent identity in production. **[Microsoft Entra Agent ID](https://learn.microsoft.com/en-us/entra/agent-id/what-is-microsoft-entra-agent-id)**
is GA with "agent identity blueprints." **[Google Cloud Agent Identity](https://docs.cloud.google.com/iam/docs/agent-identity-overview)**
is technically the most elegant — a new IAM principal type with SPIFFE IDs,
mTLS + DPoP, no impersonation, no long-lived keys.

### What is empty

**The agent-as-principal primitive.** What an agent's identity *is*, and how a
human's authority attenuates across a chain of agents, is unstandardized and
unowned:

- **Eleven-plus competing IETF individual drafts, zero adopted** — including
  [agent-grants](https://datatracker.ietf.org/doc/draft-mishra-oauth-agent-grants/),
  [attenuated delegation](https://datatracker.ietf.org/doc/draft-hamr-oauth-agent-delegation/),
  [agent identity framework](https://datatracker.ietf.org/doc/draft-sharif-agent-identity-framework/),
  [agent PKI](https://datatracker.ietf.org/doc/draft-sharif-apki-agent-pki/)
- **The IETF declined to charter a home for it.** The [AUDIT BoF](https://datatracker.ietf.org/doc/bofreq-kuhlewind-agent-use-of-delegation-and-interaction-traceability-audit/)
  — agent delegation *and* traceability, proposed by Ericsson and Microsoft —
  was **DECLINED 6 July 2026**. So was [GARR](https://datatracker.ietf.org/doc/bofreq-sharaf-garr-global-agent-registry-and-resolution/),
  the global agent registry proposal
- **The coordination mechanism is a list.** [CATALIST](https://datatracker.ietf.org/doc/bofreq-farrel-coordinating-agent-to-agent-list-of-efforts-catalist/)
  exists *"to coordinate among the various initiatives."* When your convergence
  mechanism is a catalogue, you are fragmented
- **An author of OAuth is writing a replacement.** Dick Hardt's [AAuth](https://aauth.dev/)
  is at [draft -10, 143 pages](https://datatracker.ietf.org/doc/draft-hardt-oauth-aauth-protocol/),
  outside the working group. That is the clearest possible signal that the people
  who built OAuth do not think it suffices
- **MCP core has no agent concept at all.** The [current spec, revision 2026-07-28](https://modelcontextprotocol.io/specification/versioning),
  models only *"an MCP client acting as an OAuth 2.1 client on behalf of a
  resource owner."* No agent identifier, no delegation chain

Exactly **one** agent-auth document is WG-adopted anywhere:
[draft-klrc-aiagent-auth](https://datatracker.ietf.org/doc/draft-klrc-aiagent-auth/),
in WIMSE, authored by Okta, OpenAI, AWS, Ping and Zscaler — and its explicit
thesis is anti-fragmentation: *"Rather than defining new protocols, this document
describes how existing and widely deployed standards can be applied."*

### Why the crowd is misleading

Roughly 40 vendors sell "agent identity." Most are repositioned non-human-identity,
PAM or secrets products. The tell is consolidation speed: **Astrix→Cisco,
Oasis→Cyera ($1B), CyberArk→Palo Alto, SGNL→CrowdStrike, Permiso→Okta — all
inside about 16 months.** Categories absorbed that fast were features.

Two large 2026 rounds went to general IAM replatforms with agent framing:
[NewCore](https://newcore.com/newsroom/newcore-emerges-from-stealth-66m) ($66M
seed, June 2026, Cyberstarts/Index/Evolution) and
[Oak](https://techcrunch.com/2026/07/15/backed-by-60m-in-funding-oak-steps-out-of-stealth-to-fix-the-identity-mess-that-ai-agents-are-making-worse/)
($60M seed, Accel/CRV/Greylock) — of which TechCrunch itself noted "limited
specifics on their agent-specific mechanisms."

### What builders complain about

From [Ask HN: How are you handling identities for AI agents?](https://news.ycombinator.com/item?id=45781723):

> *"The biggest problem I see with OIDC for agents is delegation — specifically,
> how one agent delegates authority to another agent acting on its behalf...
> instead of 'who is this agent?' we should be asking 'what specific action is
> this agent authorized to perform right now, and who in the chain vouches for
> it?'"*

> *"SPIFFE isn't designed for delegation at all... It gives workloads
> authenticated identity, not transitive authority."*

And the line that is the actual market opportunity, after someone laid out the
correct-but-unbuildable SPIFFE + macaroons + OPA + VC stack:

> *"I do understand what you are saying, but in my head feels a bit too
> overcomplicated to just tell any developer doing AI agents to do all this
> stuff, there must be a cleaner way to do it."*

What the best in-house implementation looks like: Uber's
["Solving the Identity Crisis for AI Agents"](https://www.uber.com/en-US/blog/solving-the-agent-identity-crisis/)
(May 2026) — a bespoke Security Token Service, a custom `act_chain` JWT claim
preserving user → agent → agent → tool, minutes-long TTLs — and *dynamic access
control and a unified enforcement plane still on the roadmap.*

**Verdict: base plumbing CONVERGED. Agent-as-principal EMPTY, with a crowded,
consolidating periphery of repositioned IAM.**

---

## 3. Discovery and registries — cataloged, but not discoverable

This section rests on original measurement: the full official registry was
paginated on 2026-09-13, its Prometheus `/metrics` read, and **400 random
registered servers liveness-probed with a real MCP `initialize` handshake.**

### What is actually in the registry

| Measurement | Value |
|---|---|
| Total version records | **102,569** |
| **Unique servers (isLatest)** | **31,680** |
| Servers with remote endpoints | 19,239 (61%) |
| `io.github.*` namespaces | 21,332 (67.3%) |

New unique servers per month went from 175 (Nov 2025) to **6,669 (Aug 2026)**.
Growth is real and steep — **and heavily diluted by bulk stuffing**:

| Publishers with ≥N servers | Accounts | Servers | % of registry |
|---|---|---|---|
| ≥500 | **3** | 4,527 | **14.3%** |
| ≥10 | 155 | 9,775 | **30.9%** |

Three accounts are 14% of the registry. One (`io.github.sadri-dridi`, 2,077
servers) publishes one-function HTTP-header toys — *"ACAO star-or-origin, value
discarded."* **22 servers ship the literal placeholder "description of my MCP
server"** and 8 ship *"An MCP server that provides [describe what your server
does]"* — unedited scaffolding that passed publication. That said, **89.7% of
namespaces hold exactly one server**, so the long tail is genuine, not all spam.

### Correcting the "dead registry" narrative — it was tested

400 random remote endpoints, real MCP handshake:

| Result | % |
|---|---|
| 403 (auth- or bot-gated) | 51.8% |
| **Speaks MCP unauthenticated** | **23.0%** |
| 401 (auth-gated) | 13.5% |
| **Clearly dead** | **7.5%** |

**Only 7.5% are dead links.** The registry is not a graveyard. (Caveat: some 403s
are Cloudflare bot-blocks on an unauthenticated probe, so "88% reachable" is an
upper bound.)

**The real problem is that you cannot tell what any of them do.** 65% require
credentials before revealing a tool list. There is no capability index, no
tool-level schema in registry metadata, and **no functional verification of any
kind.**

### Discovery is not merely unsolved — it is out of scope by design

The official docs, verbatim:

> *"The MCP Registry is **not intended to be directly consumed by host
> applications**. Instead, host applications should consume other MCP registries,
> such as downstream marketplaces."*

> *"The metadata hosted by the MCP Registry is deliberately **unopinionated**."*

And it is **still labeled "in preview"** twelve months after launch, with
data-reset warnings. There is **no semantic discovery** — substring search over
`server.json`, no embeddings, no capability taxonomy. An agent needing a
capability at runtime has a **list**, not a search.

**The traffic proves how it is really used.** From `/metrics`: ~5.0M cumulative
calls to *list* endpoints versus only ~320K individual server lookups, and **1,245
lifetime publishes**. A 15:1 list-to-lookup ratio is the signature of
**aggregators bulk-mirroring on a cron** — exactly as the docs prescribe — not
agents querying at runtime.

### Governance — correcting an earlier claim

MCP is **not** directly a Linux Foundation project. On **2025-12-09 Anthropic
donated MCP to the [Agentic AI Foundation](https://aaif.io/)**, a directed fund
under the Linux Foundation, **co-founded with Block and OpenAI** (supporters:
Google, Microsoft, AWS, Cloudflare, Bloomberg). Founding projects: **MCP, goose,
and OpenAI's AGENTS.md**.
([Anthropic announcement](https://www.anthropic.com/news/donating-the-model-context-protocol-and-establishing-of-the-agentic-ai-foundation))

Note that **Anthropic's own post claims "more than 10,000 active public MCP
servers"** against 31,680 registered — the gap between *registered* and *active*,
and Anthropic's smaller number is the more honest one.

Trust model is **namespace authentication only** — reverse-DNS via GitHub OIDC,
DNS or HTTP challenge. Security scanning is explicitly **delegated** to npm/PyPI/
Docker Hub and to downstream aggregators.

### Third-party directories

| Player | Count |
|---|---|
| [Docker MCP Catalog](https://hub.docker.com/mcp) | **"300+ tools" (verified)** — curated, containerized |
| [Composio](https://composio.dev/) | **"1,500+ apps" (verified)** — managed auth, not a registry |
| [Smithery](https://smithery.ai/) | "20,946+ MCPs" per its own page; verification criteria unpublished |
| [Glama](https://glama.ai/mcp/servers), [mcp.so](https://mcp.so/), [PulseMCP](https://www.pulsemcp.com/servers), [Pipedream](https://mcp.pipedream.com/), [Klavis](https://www.klavis.ai/) | JS-rendered, counts **unverified**. PulseMCP has **paused new submissions** |

**Docker's curated catalog is 300 tools against the official registry's 31,680.
Curation costs ~99% of the catalog.** That ratio is the entire trust story.

An independent audit ([SafeDep](https://safedep.io/the-state-of-mcp-registries/))
finds ~1,691 unique underlying packages behind a far larger entry count and
concludes the registry *"has successfully solved the problem of discovery"* with
trust as the next challenge. **The measurements above sharpen that: it solved
*cataloging*, not discovery.**

**One honest negative finding:** scanning all 31,680 descriptions for
prompt-injection patterns surfaced **only 12** with instruction-like text, mostly
benign marketing. The "registry rife with injected descriptions" concern is
**more theoretical than present today.**

**Verdict: EMERGING — cataloged but not discoverable. Semantic runtime discovery
and functional verification are both genuinely absent.**

---

## 4. A2A, orchestration, multi-agent — impressive specs, invisible adoption

The single most load-bearing datapoint, GitHub-wide `requirements.txt` counts:

| Package | Repos | PyPI/mo |
|---|---|---|
| `langgraph` | **77,824** | 52M |
| `crewai` | 12,720 | 19.1M |
| `openai-agents` | 5,560 | 22.9M |
| `agent-framework` (Microsoft) | 2,552 | 724K |
| **`a2a-sdk` (Google A2A)** | **1,956** | 12.5M |
| `acp-sdk` (IBM) | 75 | — |

**MCP out-downloads A2A roughly 21x** (`mcp` at 262M/mo). And `a2a-sdk` tracks
`google-adk` almost exactly — it is an optional extra of Google ADK, suggesting
much A2A "adoption" is Google-ADK-transitive rather than deliberate.

### A2A: excellent governance, no visible users

[a2a-protocol.org](https://a2a-protocol.org/latest/) ·
[GitHub](https://github.com/a2aproject/A2A), 25,756 stars

- Launched 2025-04-09; **donated to the Linux Foundation 2025-06-23**; **v1.0.0
  shipped 2026-03-12**; **v1.0.1 on 2026-05-28 — then 3.5 months of silence**;
  **joined the Agentic AI Foundation 2026-08-27**, making it a sibling of MCP.
- TSC: AWS, Cisco, Google, IBM Research, Microsoft, Salesforce, SAP, ServiceNow.
- **Vendor-dominated**: 161 contributors, but the top contributor (a Google
  employee) has 179 commits — more than the next 15 combined.
- **The Linux Foundation's own "Enterprise Production Use" press release names
  ZERO production users and zero use cases.** No named, detailed A2A production
  case study was found anywhere. **Unverified — likely because none is public.**
- **OpenAI declined to adopt it.** A community PR (1,200 lines, 30 tests) was
  rejected: *"we don't have immediate plans to add A2A support."*

**The most revealing artifact in this layer is A2A's own issue tracker.** Every
top-commented open issue in 2026 is about trust, and all remain **OPEN**:
**#1672 Agent Identity Verification (658 comments, open since March)**, #1786
Cryptographic Agent Identity (244), #1829 Ed25519 message signing (143), #1628
`trust.signals[]` (124), and **#1713 "Cross-org, no shared AS: accountability
layer for first-contact A2A transactions."**

**A2A standardized the envelope — not who you are, and not who pays when it
breaks.**

### The rest of the protocol field is mostly stalled or dead

- **IBM/LF ACP — dead, merged into A2A.** The repo is **archived** (last push
  2025-08-25); the site carries a migration banner.
- **[AGNTCY](https://agntcy.org/)** — real engineering, near-zero traction:
  `oasf` 333 stars, `slim` 215, `dir` 187. **Its Identity component has had zero
  commits since 2026-02-24** — the most strategically valuable piece, abandoned.
  LangChain and Galileo are no longer listed as partners.
- **[AP2](https://github.com/google-agentic-commerce/AP2)** — 3,177 stars but
  **zero commits in 90 days** and no PyPI package, despite "60+ organizations."
- **NANDA (MIT)** — top repo last pushed 2025-04-23. **Effectively dead.**
- **[x402](https://github.com/x402-foundation/x402)** — 6,608 stars, own
  foundation, very active. **The most vital non-MCP protocol in the field.**

### Orchestration: crowded, consolidating, and where the money actually went

- **[Temporal](https://temporal.io/blog/temporal-raises-usd300m-series-d-at-a-usd5b-valuation)**
  — **$300M Series D at $5B, 2026-02-17, led by a16z.** The most credible numbers
  anywhere in this report: revenue **+380% YoY**, **9.1T lifetime actions, 1.86T
  from AI-native companies**. Customers: **OpenAI, Block, ADP, Yum! Brands**.
- **LangChain** — $125M Series B at $1.25B, 2025-10-20, led by IVP. **77,824
  repos is the strongest real-usage signal in the layer.**
- **[Sierra](https://sierra.ai/)** — **$950M at >$15B, 2026-05-04**, **>$150M
  ARR** — but this is single-vendor agent-to-*human*.
- **CrewAI** — $18M, 2024-10-22 (Insight + boldstart), no later round found.
  58,461 stars. Self-reported "65% of Fortune 500" and "450M+ workflows/month"
  are **unaudited; treat with suspicion.**
- **AutoGen is in maintenance mode**, merged into Microsoft Agent Framework
  (GA 2025-10-01). **OpenAI's Swarm is officially deprecated.**

### Agent-hires-agent: essentially not happening

- **Salesforce Agentforce**: a TD Cowen partner survey found **0% of partners saw
  Agentforce drive booking activity**; Salesforce pivoted it toward deterministic
  automation in Dec 2025.
- **Fetch.ai/Agentverse** claims "2.7 million agents" — a *registration* count
  with no transaction volume, on its own uAgents rather than A2A. **Marketing.**
- The only place agent-to-agent commerce actually moves is **crypto rails around
  x402**, not A2A/AP2. **The ecosystem routed around the enterprise standard.**

### What builders complain about

- **[Cognition, "Don't Build Multi-Agents"](https://cognition.com/blog/dont-build-multi-agents)**
  — *"running multiple agents in collaboration only results in fragile systems."*
  From the team with the most production coding-agent hours.
- **Anthropic's pro-case is honest about cost**:
  [its multi-agent research system](https://www.anthropic.com/engineering/multi-agent-research-system)
  is 90.2% better than single-agent but uses **~15× more tokens** — and is
  read-only, single-org, with no side effects.
- **Berkeley, [Why Do Multi-Agent LLM Systems Fail?](https://arxiv.org/abs/2503.13657)**
  — 1,600+ traces, 7 frameworks, **14 distinct failure modes**.
- **[Ask HN: Is anyone using the A2A protocol?](https://news.ycombinator.com/item?id=48582679)**
  — *"Nope, not a single use case i witnessed used a2a in the final product."* A
  separate "agents in production" thread contains **zero mentions of A2A or any
  multi-agent protocol.**
- **CrewAI #5802 (119 comments, OPEN): tool re-execution on retry has no
  idempotency guard — "duplicate payments, emails, trades possible."** LangGraph
  #7417 (open): long tool calls **silently re-executed** from checkpoint.
- **Google shipped an agent orchestrator (Scion) that does not use Google's own
  agent protocol.**

The deepest problem, from [A2A issue #1976](https://github.com/a2aproject/A2A/issues/1976),
open since June 2026 with no sponsor:

> *"In core A2A, `COMPLETED` means 'the agent stopped working,' not 'the output
> meets the requester's intent.' There is no interoperable, cross-vendor way to
> declare, up front, what 'done' looks like; to gate the `WORKING → COMPLETED`
> transition on an explicit client or human approval; or to reject an output with
> structured, machine-readable feedback."*

**The dominant agent-to-agent protocol has no concept of correctness.** That
sentence is the hinge of this entire report.

**Verdict: EMERGING at the cross-org layer; the orchestration sub-layer is
CROWDED and consolidating. Governance is excellent, adoption is logos on slides.**

---

## 5. Memory and context — crowded, and losing to the naive baseline

### Players and funding

| Player | Funding | Traction |
|---|---|---|
| [Mem0](https://mem0.ai) | **$24M total — $20M Series A led by Basis Set, 2025-10-28** | 65,229 stars / 2.78M PyPI mo |
| [Zep](https://www.getzep.com) | **Never announced — unverified.** YC W24, ~10 people | graphiti 30,851 stars; named customers Samsung, Zscaler, Twin Health, HoneyBook; **SOC 2 Type II** |
| [Letta](https://www.letta.com) | **$10M seed led by Felicis @ $70M post, 2024-09-23** | 24,722 stars — **but see below** |
| [Cognee](https://www.cognee.ai) | Unverified | 30,665 stars |
| [Supermemory](https://supermemory.ai) | **$2.6M seed led by Susa, 2025-10-06** | 29,653 stars |

Vector DBs are all fleeing upward: **Chroma** now calls itself *"open-source
search infrastructure"* and trained its own 20B model; **turbopuffer** trained
SID-1 and has the most impressive customer wall in the layer (Anthropic,
Cognition, Notion, Linear, Atlassian, Ramp, NYT, Harvey); **Pinecone**
repositioned to a *"knowledge platform for AI agents."*

**Commoditization proof from below:** **CrewAI rebuilt its agent memory on
LanceDB** and ships it by default — and CrewAI was a flagship Mem0 integration.

### Memory is not "being absorbed" — it already was

- **Anthropic** ships a memory tool, context editing and **compaction**, with docs
  now stating *"server-side compaction is the primary strategy."* Critically, the
  memory tool is **client-side by design** — Anthropic took the *interface* and
  left storage open, which turns memory into a free protocol and vendors into
  interchangeable backends.
- **AWS Bedrock AgentCore Memory published a commodity price card**: **$0.25 per
  1,000 events**, $0.75/1,000 records/mo, $0.50/1,000 retrievals. That is the
  exact primitive Mem0 sells at $249/mo.
- **Microsoft Foundry Agent Service Memory is free** — you pay only model tokens.
- **Google** ships the Interactions API (GA June 2026) plus Vertex Memory Bank.

### The most damaging evidence in this entire report

An independent, reproducible benchmark with an open-sourced harness —
[MemBench, 4,000 cases](https://fastpaca.com/blog/memory-isnt-one-thing):

| System | Precision | Latency | Cost (4k cases) |
|---|---|---|---|
| **long-context baseline** | **84.6%** | **7.8s** | **$1.98** |
| Mem0 | 49.3% | 154.5s | $24.88 |
| Zep | 51.6% | 224s | **~$152.60** |

**Memory systems were 14–77× more expensive and 31–33% less accurate than naive
long context.** The Zep run was aborted after 9 hours, averaging **1,028 LLM calls
per case**. The diagnosis — *"LLM-on-Write"*: *"The error happens at write time,
meaning the data is corrupted before it hits the database. No amount of retrieval
optimization can fix a database filled with hallucinations."*

**The benchmarks are saturated and gamed.** Mem0 and Zep publicly fought over
LoCoMo; both now claim 92–95%. **Mem0's own research page shows 92.5% on LoCoMo
but 48.6% on BEAM 10M** — worse than a coin flip — and Mem0's own blog concedes
*"high scores do not guarantee that the system behaves reliably."* A dozen weekend
Show HN projects each claim to beat Mem0 on LoCoMo. **When a benchmark can be
beaten by a weekend project, the benchmark is the moat, and there is no moat.**

**Peer-reviewed evidence memory makes models worse:** Writer's *"Recalling Too
Well"* (ICLR 2026 workshop) finds memory systems amplify **sycophancy up to 25×**.

**The category's inventor published its obituary.** Letta CTO Sarah Wooders,
[*"Why Memory Isn't a Plugin"*](https://www.letta.com/blog/why-memory-isnt-a-plugin):
*"Asking to plug memory into an agent harness is like asking to plug driving into
a car… **And even then, it's hard to do much better than just grep.**"* **Letta
has left memory infrastructure entirely** — letta.com now sells Letta Code, a
$20/mo coding-agent competitor. Even Zep concedes *"Markdown is not agent
memory"* opens with *"For a single agent and a single user it is hard to beat."*

**Emerging and unsolved: memory security.** September 2026 alone produced a
distinct arXiv cluster — *Agent Memory Is a Surface for Endogenous Authorization
Laundering* (2609.01836), *Revoked but Still Authoritative* (2609.08258), *Does
Your Agent's Memory Survive a Model Upgrade?* (2609.05339).

**What survives is narrow and enterprise-shaped**: bitemporal correctness,
provenance, cross-provider portability, ABAC, SOC 2 Type II, BYOC, retention,
audit. **Zep is the only player clearly executing that thesis** — and that it has
never announced a funding round is the most conspicuous open question in the
layer.

**Verdict: CROWDED and oversupplied at the commodity middle, squeezed from three
sides, and losing to the naive baseline on accuracy, latency and cost.**

---

## 6. Human-in-the-loop — the category dissolved

This is the cleanest finding in the report, and it cuts against the brief's
premise.

**[HumanLayer](https://www.humanlayer.dev/) — the flagship HITL company — no
longer sells HITL.** Its site today sells "a multiplayer coding agent IDE and
cloud." Pricing is $100/user/month. Named customers are Upstart, Casco, Osmosis,
Ambral, Nautilus, Weave, Roadrunner. Investors listed include Y Combinator,
Massive, Pioneer Fund, Vercel, Browserbase. **The approval API is not mentioned
anywhere on the page.**

Corroborating detail: the OSS repo carries a maintainer notice reading *"the code
here is pretty much all deprecated"*, **the original HITL API docs now return a
hard 404** (`require_approval`, `human_as_tool`, Slack/email contact channels,
escalations — all gone), and the entire blog since April 2025 is coding-agent
content with **no pivot announcement**. Their biggest-ever HN post is about
writing CLAUDE.md files (748 points); the HITL launch peaked at 354 points in
November 2024 and was never mentioned again.

There are no funded pure-plays left in the category, because approval shipped as
a free feature everywhere: Claude Agent SDK permissions, LangGraph interrupts,
OpenAI Agent Builder (where "Human approval" is literally a drag-and-drop Logic
node), Temporal signals, Inngest `waitForEvent`, n8n's "Send and Wait for
Response," Zapier, and UiPath Action Center — which solved this for RPA years
before agents existed.

**It became a protocol feature too**, which removes the last reason to buy it.
The [MCP Tasks extension](https://modelcontextprotocol.io/extensions/tasks/overview)
names the use case explicitly — *"CI pipelines, batch processing, **human
approvals**"* and *"**Human-in-the-loop workflows.** Approval gates, review steps"*
— with an `input_required` state and durable `taskId` handles surviving client
crashes. [MCP elicitation](https://modelcontextprotocol.io/specification/2026-07-28/client/elicitation)
adds form and URL modes. [A2A v1.0](https://a2a-protocol.org/latest/specification/)
defines `TASK_STATE_INPUT_REQUIRED` and `TASK_STATE_AUTH_REQUIRED`.

The Claude Agent SDK case is the sharpest: it ships essentially HumanLayer's
original product natively — a six-step permission flow, `canUseTool` with
approve / approve-with-modified-input / approve-and-remember / reject-with-message,
a callback that can stay pending indefinitely, and a **`PermissionRequest` hook
with a `defer` decision** so the process can exit and resume later from a
persisted session. That is durable async human approval, built in. The docs even
say the Slack part out loud.

The adjacent policy-engine players — [Oso](https://www.osohq.com/) (now pivoted
to "Oso for Agents"), [Cerbos](https://www.cerbos.dev/),
[OPA](https://www.openpolicyagent.org/), [Permit.io](https://www.permit.io/)
(MCP Gateway), [Arcade.dev](https://arcade.dev) — are real businesses, but they
sell authorization, not human approval. Note that **OPA's creators joined Apple in
August 2025 and `styra.com` no longer resolves**; OPA's published roadmap contains
zero AI/agent/MCP items.

### The empirical case against human approval — now sourced

- **Anthropic, 2026-08-07** ([blog](https://claude.com/blog/auto-mode-default-in-claude-code),
  covered by [Simon Willison](https://simonwillison.net/2026/Aug/8/auto-mode/)):
  a model classifier blocked **89%** of harmful actions while **human reviewers
  rejected only 13.6%** (n = 1,053 paid testers), with human catch rate
  **degrading to ~5% over long sessions**. Production telemetry: harmful
  unintended actions in **6.3%** of manually-approved flagged sessions vs **2.4%**
  under auto mode. Auto mode became the Claude Code default on 2026-08-14.
- **Independent study, 2026-08-06** ([HN, 340 points](https://news.ycombinator.com/item?id=49195468)
  → [scalex.dev](https://scalex.dev/blog/ai-agent-permissions-stats/)): 40,000
  runs, **409,000 approve/deny decisions**, average accuracy **66.3%**, **7% of
  users approved literally everything**, and `npm run analyze` approved **64.7%**
  of the time *with a visible malicious payload in the history log*.

The market has already voted. Products that *add* approvals get 1–6 HN points
(Preloop, AgentGate, Ottr, Axon, DashClaw, Sentinel — a graveyard). Products that
*remove or contain* them get 78–340 (`nah`, Fence, OneCLI). The complaint is not
"this tool is bad," it is **"I don't need this."**

**The one live gap** — from a user on that thread: auto mode *"is looking for
security threats, not the model misinterpreting my intent, and it can't be tuned
to look for things like 'please gate tool calls which may delete data.'"*
Blast-radius classification tuned to **data destruction rather than security
threats** is genuinely unmet. It is a feature, not a company.

**Verdict: EMPTY as a category, SOLVED as a feature. Do not build here.**

---

## 7. Observability, evals, tracing — crowded and consolidating

**~30 players tracked. 8 acquired, 2 confirmed shut down, 1 likely wound down,
1 pivoted out — in 13 months.**

### The consolidation table

| Target | Acquirer | Date | Terms |
|---|---|---|---|
| [Weights & Biases](https://wandb.ai/site/weave/) | CoreWeave | closed 2025-05-05 | **~$1.7B** |
| [Humanloop](https://humanloop.com) | **Anthropic** | 2025-08-13 | Undisclosed — **acqui-hire, explicitly no IP or assets**. Platform sunset |
| Statsig | **OpenAI** | Sept 2025 | **$1.1B all-stock** |
| Neptune.ai | **OpenAI** | Dec 2025 | Undisclosed. **Site gone**, redirects to openai.com |
| [Langfuse](https://langfuse.com/) | **ClickHouse** | **2026-01-16** | Undisclosed. OSS + Cloud preserved |
| [Helicone](https://www.helicone.ai/) | Mintlify | 2026-03-03 | Undisclosed. **Maintenance mode only** |
| [Traceloop / OpenLLMetry](https://www.traceloop.com/) | ServiceNow | Mar 2026 | Folded into AI Control Tower |
| Velvet | Arize AI | 2025-03-13 | Folded into Arize AX |
| WhyLabs | — | 2025 | **Shut down**, open-sourced |
| Gentrace | — | — | **Shut down**, MIT-licensed |
| Freeplay | — | — | Likely wound down |
| Vellum | — | — | **Pivoted out** — now a consumer AI assistant |

The **[ClickHouse acquisition of Langfuse](https://clickhouse.com/blog/clickhouse-raises-400-million-series-d-acquires-langfuse-launches-postgres)**
was announced alongside a **$400M Series D led by Dragoneer**. Langfuse had
20,000+ GitHub stars and 26M+ monthly SDK installs at acquisition, and
[its own post](https://langfuse.com/blog/joining-clickhouse) commits to staying
open source and self-hostable.

**Correction to a widely-repeated claim: [Patronus AI](https://www.patronus.ai/)
was NOT acquired.** It raised a
**[$50M Series B led by Greenfield Partners on 2026-06-25](https://techcrunch.com/2026/06/25/patronus-ai-lands-50m-to-build-digital-worlds-that-stress-test-ai-agents/)**
(~$70M total; Lightspeed, Datadog, Samsung), with revenue up 15x YoY — **but it
pivoted out of evals**, and now leads with Digital World Models and RL
environments. That is an escape from the layer, not a bet on it.

### The funded independents

| Player | Funding (verified) | Adoption |
|---|---|---|
| [LangChain / LangSmith](https://www.langchain.com/langsmith) | **$125M Series B led by IVP at $1.25B, 2025-10-21** (CapitalG, Sapphire, Sequoia, Benchmark, ServiceNow, Workday, Cisco, Datadog, Databricks) | **langsmith: 94.3M PyPI/mo, 24.7M npm/mo** — largest distribution in the layer. Expedia, Autodesk, Nvidia, Workday, Coinbase |
| [Braintrust](https://www.braintrust.dev/) | **$80M Series B led by ICONIQ, 2026-02-17, ~$800M valuation** (a16z, Greylock, Elad Gil); $36M A (2024-10-08) | Notion, Replit, Cloudflare, Ramp, Dropbox. Shipped **behavior specs**, an open standard for supervising long-horizon agents |
| [Langfuse](https://langfuse.com/) | Acquired by ClickHouse | **34,540 stars**, 23.2M PyPI/mo. 50,000+ companies, 21 of Fortune 50, 90B+ observations/mo |
| [Arize / Phoenix](https://arize.com/) | **$70M Series C led by Adams Street, 2025-02-20** (M12, Datadog, PagerDuty, Battery, TCV) | Phoenix 11,440 stars. Booking.com, Duolingo, Uber, Wayfair. **Phoenix is ELv2, not OSI open source** |
| [Comet / Opik](https://www.comet.com/site/products/opik/) | ~$70M total | **Opik 21,991 stars — the surprise #2 in OSS.** Netflix, Uber, Etsy, Shopify |
| [Galileo](https://galileo.ai/) | **$45M Series B led by Scale Venture Partners, 2024-10-15** | **PyPI only 37,770/mo — very weak bottom-up.** Twilio, Comcast, HP |
| [W&B Weave](https://wandb.ai/site/weave/) | Inside CoreWeave | **Only 1,128 stars** — an order of magnitude behind Langfuse |
| [Fiddler](https://www.fiddler.ai) | $30M Series C (**date unverified**); Lightspeed, IQT, Lockheed Martin Ventures | Nielsen, Mastercard, DTCC. No OSS footprint |
| [Datadog](https://www.datadoghq.com/product/llm-observability/) | Public | **Free to 40K LLM spans/mo, then $160/mo per 100K** — and is simultaneously an *investor* in LangChain, Arize **and** Patronus |

**Note a direct conflict in my sources:** one research pass reported
**Galileo acquired by Cisco around April 2026**; another found Galileo still
independent on its 2024 Series B. **I could not resolve this — treat Galileo's
status as UNVERIFIED.**

### Two structural facts settle this layer

**First, OpenAI's Agents SDK tracing is free and on by default** — exporting to
OpenAI's dashboard *even when you use non-OpenAI models* — and its docs then list
**28 third-party "external trace processors"**: W&B, Arize Phoenix, MLflow,
Braintrust, Logfire, AgentOps, LangSmith, Maxim, Opik, Langfuse, Langtrace,
Galileo, Portkey, LangDB, Agenta, PostHog, PromptLayer, HoneyHive, Datadog and
more. **Twenty-eight vendors queuing to be optional plugins on someone else's
free default.** That list, more than any funding number, tells you what this layer
is. (OpenAI is also **shutting down its own Evals API** — read-only 2026-10-31,
gone 2026-11-30.)

**Second, there is still no stable trace format.** The GenAI semantic conventions
**moved out of the main semconv repo into
[a dedicated one created 2026-05-05](https://github.com/open-telemetry/semantic-conventions-genai)
— 354 stars and ZERO published releases**, with the README's Schema URL section
reading literally `TODO`. Every document is `Status: Development`, including
`gen-ai-agent-spans.md`. The stabilization effort was **only scoped on
2026-09-11**, and the agent-vs-workflow span distinction is still unresolved and
may be merged. **Every "OTel-native, no lock-in" claim in this layer is
aspirational.**

### What builders complain about

- **Cloud account required to see your own data** — *"Built this because LangSmith
  needs a cloud account just to see my own traces"*
  ([HN](https://news.ycombinator.com/item?id=48063206)), echoed across 15+
  local-first Show HNs, all near-zero traction
- **Evals resist productization** because the moat is domain knowledge the vendor
  cannot have: *"Off-the-shelves Eval/Annotations/Prompt Optimization tools are
  sub-par because they can only be generic"*
- **LLM-as-judge distrust**, best quote in the layer: *"The judge was passing
  everything at TNR 0.00, so **the suite was decorative**"*
- **Visibility without enforcement** — *"there is good observability. You can see
  logs, traces, cost dashboards. But **the actual shutdown mechanism often ends up
  being manual**"* ([HN](https://news.ycombinator.com/item?id=47002748))
- **Tracing is not agent tracing** — *"Langfuse tracks LLM calls but doesn't
  understand agent topology — tool calls, handoffs, decision trees"*

Verified capital into independents is **~$633M**, with realistic deployed capital
**$1.5–2B** and **$2.8B+ of acquisition value realized**. But note where the exits
went: a GPU cloud, a database vendor, a docs company, a workflow vendor, and two
model labs. **Almost nobody exited to another observability company.** That is
what it looks like when a capability stops being a product and becomes a feature
of something larger.

**Verdict: CROWDED, consolidating. Ten funded startups means crowded even though
none has won — and here a third of the field has already been bought or died.**

## 8. Sandboxing and safe execution — commoditizing, but load-bearing

An Ask HN thread opens by counting **"37+ sandboxing solutions launched within the
past year."** This is the layer everything else terminates in, and it has no
pricing power.

### Four independent proofs of a race to the bottom

1. **[E2B](https://e2b.dev/) and [Daytona](https://www.daytona.io) publish
   identical prices to five decimals** — $0.0504/vCPU-hr, $0.0162/GiB-hr. There
   is a reference price and nobody can move it.
2. **[Northflank](https://northflank.com) sells the same CPU for one-third of it**
   ($0.01667/vCPU-hr).
3. **[Kernel](https://www.kernel.sh) advertises "50–98% lower costs than prior
   providers"** on its own pricing page. Discounting is the headline feature.
4. **Everyone is retreating to free-idle billing** (Cloudflare, AWS AgentCore,
   Vercel, Fly, Kernel). Agent workloads mostly *wait on model calls*, so
   free-idle structurally collapses revenue per sandbox-hour.

**And the floor is zero:** Anthropic gives away **1,550 container-hours per org
per month** ($0.05/hr thereafter, free entirely when paired with web search), and
**Docker ships a free microVM sandbox CLI (`sbx`), free for commercial use**.

### The players

| Player | Isolation | Funding | Notes |
|---|---|---|---|
| [Modal](https://modal.com) | gVisor | **$355M Series C at $4.65B, 2026-05-21** (General Catalyst, Redpoint); $80M B at $1.1B Sept 2025 | **>$300M annualized revenue, 5x since the B. Sandboxes alone are >1/3 of revenue.** The **only company in the entire layer that publishes revenue** |
| [E2B](https://e2b.dev/) | Firecracker | **$21M Series A, 2025, Insight Partners.** No 2026 round found | Apache-2.0, 13.8k stars. Claims >1B sandboxes, >100k teams, "94 of the Fortune 100 signed up" (**a free-signup metric**). **No revenue figure anywhere** |
| [Daytona](https://www.daytona.io) | microVM | **$24M Series A, 2026-02-05, FirstMark** (+Datadog, Figma Ventures); ~$31M total | Sub-90ms creation. Cursor self-hosted machines, Devin Outposts, LangChain Open SWE. **Went closed source 2026-06-11** |
| [Cloudflare Sandboxes](https://developers.cloudflare.com/sandbox/) | Linux containers | n/a | **GA 2026-04-13** |
| [Vercel Sandbox](https://vercel.com/docs/sandbox) | Firecracker | n/a | **GA 2026-01-30.** 10,000 concurrent sandboxes on Pro/Enterprise |
| [Fly.io Sprites](https://fly.io/sprites) | microVM | No 2025/26 round found | Launched 2026-01-09. Persistent rather than ephemeral, **free while idle** |
| [Blaxel](https://blaxel.ai) | microVM, ~25ms resume | $7.3M seed, 2025-12-03, First Round | **ACQUIRED BY BASETEN, 2026-09-10.** Nine months seed to exit |
| Ona (ex-Gitpod) | kernel-level policy | — | **JOINED OPENAI, 2026-06-11** |
| [CodeSandbox](https://codesandbox.io) | microVM | — | **"now part of Together AI"** (date/terms unverified) |
| [Runloop](https://www.runloop.ai/) | custom hypervisor | Unverified | Coding-agent evals / RL environments |

**The isolation tech is free and mature:**
[Firecracker](https://github.com/firecracker-microvm/firecracker) 36.7k stars,
[gVisor](https://github.com/google/gvisor) 19.3k, Kata 8.7k, microsandbox 8.2k.
**~73k stars of Apache-2.0 commodity isolation. No sandbox startup owns its own
moat technology.**

### The document that caps the layer

Anthropic's self-hosted sandbox interface **names eleven interchangeable
third-party backends, alphabetically, with no preference expressed**: AWS Lambda
MicroVMs, Blaxel, Cloudflare, Daytona, E2B, Fly.io, GKE Agent Sandbox, Modal,
Namespace, Superserve, Vercel. **That is a specification for a substitutable
component** — excellent distribution today, a permanent cap on pricing power.

And **Cloudflare is arguing the product is mostly unnecessary**: its
[`@cloudflare/computer`](https://blog.cloudflare.com/cloudflare-computer/) post
(2026-08-03) states that **"containers are required for less than 10% of agent
work."** If even roughly right, that deletes ~90% of the billable unit.

Consolidation in 15 months: **three acquisitions** (Ona→OpenAI, Blaxel→Baseten,
CodeSandbox→Together), **two category exits** (Scrapybara, MultiOn), **two
up-stack pivots** (Airtop, Morph), **one open-source retreat** (Daytona).
**No independent sandbox startup raised a Series B.**

### Browser sandboxing — the price war is worse

[Browser Use](https://browser-use.com) charges **$0.02/browser-hr against
[Browserbase](https://www.browserbase.com/)'s $0.10–$0.12 — a 5–6x undercut** —
on **$17M raised, 114k GitHub stars, 28.9M monthly downloads and a team of 7**.
Browserbase has **$67.5M total ($40M Series B, June 2025, at $300M post-money)**,
10,000+ companies, 35M+ monthly sessions, and customers Microsoft, Ramp, Lovable,
Clay, DeepMind. Cloudflare Browser Run sells the same thing at $0.09.

Others: [Steel](https://steel.dev), [Anchor](https://anchorbrowser.io),
[Hyperbrowser](https://www.hyperbrowser.ai), [Notte](https://notte.cc),
[Skyvern](https://www.skyvern.com), [Lightpanda](https://lightpanda.io) (a
from-scratch browser engine in Zig — genuinely differentiated). **Scrapybara and
MultiOn have exited the category entirely; Airtop pivoted up-stack.**

**Reliability is the complaint, and it changed the architecture.** From the best
thread in the layer ([134 points](https://news.ycombinator.com/item?id=47780971)):
*"Runtime agents were brittle. It felt like trying to debug/audit a black box"*
and *"Sites like Booking.com... **Playwright is detected and often blocked**."*
The 2026 winning move is to **use the LLM at build time to emit deterministic
Playwright code and bypass the DOM at runtime** — Anchor's Web Action Cache,
Airtop's Agent Builder and Libretto all converged there. That is a reliability
complaint severe enough to reject the category's core premise.

### The guardrails sub-layer: consolidated and technically discredited

**Twelve acquisitions in ~24 months**, by effectively the entire network/endpoint
security incumbent set:

| Target | Acquirer | Date | Price |
|---|---|---|---|
| Robust Intelligence | Cisco | 2024-08 | ~$400M *reported*, undisclosed |
| Protect AI | Palo Alto | closed 2025-07-22 | Undisclosed |
| [Invariant Labs](https://snyk.io/news/snyk-acquires-invariant-labs-to-accelerate-agentic-ai-security-innovation/) | **Snyk** | 2025-06-24 | Undisclosed |
| Prompt Security | SentinelOne | closed 2025-09-05 | Undisclosed (~$250M reported) |
| Aim Security | Cato Networks | 2025-09-03 | ~$350M *reported*, unofficial |
| CalypsoAI | F5 | 2025-09-11 | **$180M — the only cleanly disclosed price** |
| Pangea | CrowdStrike | 2025-09-16 | **$260M** |
| [Lakera](https://www.checkpoint.com/press-releases/check-point-acquires-lakera-to-deliver-end-to-end-ai-security-for-enterprises/) | Check Point | 2025-09-16 | Undisclosed in the PR (~$300M reported) |
| SplxAI | Zscaler | 2025-11-03 | Undisclosed. **Seed to exit in 8 months on a $7M seed** |
| TrojAI | A10 Networks | ~2026-06-15 | Undisclosed |
| Virtue AI | Fortinet | 2026-08-17 | Undisclosed |
| [Guardrails AI](https://www.guardrailsai.com/blog/guardrails-ai-joins-harvey) | **Harvey** | **2026-09-08** | Undisclosed |

**Only 1 of 12 prices cleanly disclosed.** Reported prices cluster $180M–$400M —
good seed outcomes, not category-defining ones.

**Every acquired vendor's open-source guardrail is now dead:** Protect AI's
**LLM Guard archived**, its **Rebuff archived** (dead since 2024), and **Lakera's
PINT benchmark archived 2026-04-16** — *the industry's main public
prompt-injection-detection benchmark is archived.* The exception is
**[Snyk's agent-scan](https://github.com/snyk/agent-scan)** (ex-Invariant
`mcp-scan`), 3,035 stars, actively maintained and broadened to cover agent
*skills*. The actual adoption winner is
**[promptfoo](https://github.com/promptfoo/promptfoo) at 25,061 stars — and it is
a testing tool, not a runtime guardrail.**

Still independent and funded: [Zenity](https://zenity.io) ($125M Series C led by
Norwest, 2026-08-03), [HiddenLayer](https://hiddenlayer.com/) ($100M Series B led
by Delta-v, 2026-09-02, ARR 10x YoY — **the most obvious next acquisition
target**), [Noma](https://noma.security/) ($100M Series B, 2025-07-31),
[Straiker](https://straiker.ai) ($64M Series A), [WitnessAI](https://witness.ai)
($58M), [Lasso](https://lasso.security) ($30M), and
**[Pillar Security](https://www.pillar.security/), stalled at a $9M seed for 17
months.**

**The technical indictment matters more than the M&A.** "The Attacker Moves
Second" ([arXiv:2510.09023](https://arxiv.org/abs/2510.09023)) — authored jointly
by **OpenAI, Anthropic and Google DeepMind** researchers — broke **12 published
defenses at >90% attack success rate**, against defenses that *"originally
reported near-zero attack success rates."* **Human red-teaming achieved 100%
success against all defenses tested.** This should make you distrust every vendor
detection-rate number in this report.

The first-party admissions are just as damning. **Microsoft's own Prompt Shields
docs**: *"Prompt Shields may not catch all attack vectors... **Always implement
additional validation layers**."* **Anthropic's own honest number**: an **11.2%
residual prompt-injection success rate** for Claude in Chrome autonomous mode
after mitigations. And Anthropic's "720/720 attacks blocked" claim for auto mode
**was bypassed ~80% reliably three weeks later** by Johann Rehberger — with the
perverse detail that **auto mode blocked Claude's own cleanup command**, making
the safety mechanism part of the failure.

**Google has declared prompt-injection data exfiltration in Antigravity a known
issue, ineligible for bug bounty** — the finding that drew
[768 HN points](https://www.promptarmor.com/resources/google-antigravity-exfiltrates-data).

The market bought probabilistic filters and sold them as security boundaries. The
defensible ground is architectural — which is why this layer, despite
commoditizing, remains load-bearing.

**Verdict: CROWDED and commoditizing at the sandbox layer; CONSOLIDATED and
technically discredited at the guardrails layer.**

## 9. Audit trail, provenance, compliance — closed at the top, empty at the bottom

### The regulatory driver just moved, and this is the most important fact here

**The EU AI Act high-risk deadline was delayed, and it is law, not a proposal.**

The "Digital Omnibus on AI" was approved by Parliament **16 June 2026** (423–57,
174 abstentions), published as **Regulation (EU) 2026/1744** on 24 July 2026, and
entered into force **27 July 2026** — six days before the deadline it superseded.

- **Annex III stand-alone high-risk → 2 December 2027** (was 2 Aug 2026)
- **Annex I product-embedded high-risk → 2 August 2028** (was 2 Aug 2027)
- The Commission's original conditional trigger tied to standards readiness was
  **rejected in trilogue** and replaced with fixed calendar dates

Sources: [European Commission](https://digital-strategy.ec.europa.eu/en/policies/regulatory-framework-ai),
[European Parliament, 16 June 2026](https://www.europarl.europa.eu/news/en/press-room/20260611IPR45207/ai-act-ep-approves-simplification-measures-and-nudifier-app-ban).
**[UNVERIFIED at quotation level]**: EUR-Lex returned HTTP 202 with an empty body,
so the recital text was not read verbatim; the regulation number and all dates are
confirmed by both sources above.

**Implication: any thesis that depended on an August 2026 compliance scramble
lost its forcing function this July.** That pull slid roughly 16 months.

What the Act requires when it lands: [Article 12](https://artificialintelligenceact.eu/article/12/)
is a build-time *capability* duty (systems must "technically allow for the
automatic recording of events"); [Article 19](https://artificialintelligenceact.eu/article/19/)
and [Article 26(6)](https://artificialintelligenceact.eu/article/26/) require
providers and deployers to keep those logs for **at least six months**;
[Article 99](https://artificialintelligenceact.eu/article/99/) puts record-keeping
breaches in the **€15M / 3% of worldwide turnover** tier.

**Critically, the AI Act contains nothing specific to agentic AI.** It is
technology-neutral: an agent is regulated by what it is used for, not by the fact
that it acts autonomously.

### "Is audit trail just a feature of observability?" — No, and the distinction is sharp

Checked directly:

- **[Langfuse audit logs](https://langfuse.com/docs/administration/audit-logs)** —
  Enterprise tier, and they log **API keys, datasets, prompts, project and org
  membership changes**. Explicitly *Langfuse platform user activity, not AI agent
  behavior.*
- **[Datadog Audit Trail](https://docs.datadoghq.com/account_management/audit_trail/)** —
  Datadog platform user/admin activity. **Maximum retention 90 days**, which is
  *below the AI Act's six-month floor* unless you archive elsewhere.
- **[LangSmith](https://docs.langchain.com/langsmith/administration-overview)** —
  no audit-log or compliance-certification content at all.

Observability gives you rich, mutable, vendor-held telemetry for debugging.
Compliance needs retained, attributable, tamper-evident records for a third party.
**Different products. No observability vendor credibly claims the second.**

### The GRC layer is closed

A Gartner Magic Quadrant for AI Governance Platforms exists as of July 2026, and
$1B-scale M&A has cleared: **Oasis Security → Cyera for $1 billion** (~late July
2026), **Astrix → Cisco** (reported $250–350M), **Galileo → Cisco**,
**Prompt Security → SentinelOne**.

Then four incumbents shipped competing agent-governance products inside two weeks:
**Okta Agent SSO** (24 Aug 2026), **IBM watsonx Orchestrate AgentOps Agent** (31
Aug, with a Trace Inspector), **Broadcom AgentMinder** (31 Aug, verifies agent
identity *and mission*), **Dataiku Agent Management** (Sept, GA Oct). Reported
alongside: **$435M across 12 financings** in enterprise agent security/governance,
April–September 2026. **[Single-source — [Yahoo Finance](https://finance.yahoo.com/technology/ai/articles/agent-governance-stack-forming-four-204405621.html)
— though the individual launches are independently checkable.]**

Vendors, with what was verified:

| Vendor | Funding | Status |
|---|---|---|
| [Credo AI](https://www.credo.ai/product) | **$21M Series B, 30 Jul 2024**; $41.3M total; [no Series C](https://www.credo.ai/blog/accelerating-global-growth-and-innovation-in-ai-governance-with-21-million-in-new-capital) | Deepest agent model (Agent Registry, Agent Cards). **"Agent Governor" is a research preview.** Customers: Mastercard, Autodesk, Databricks, Northrop Grumman |
| [Vanta](https://www.vanta.com/products/ai-governance) | **$150M at $4B**, ~2025; **~$300M ARR** | AI Governance is **waitlist-only** |
| [Drata](https://drata.com/products/agent-governance) | **[UNVERIFIED]** | Claims "every decision in a **tamper-evident record**" — but **limited/early access, no named production customers** |
| [Holistic AI](https://www.holisticai.com/) | **$35M**, May 2024. A circulating "$200M" figure **could not be verified — likely erroneous** | 2026 Gartner MQ Challenger. **No named customers on site** |
| [Trustible](https://www.trustible.ai/) | "Over $6M" — Harlem Capital, Alumni Ventures, Tau | Leidos, Nuix, Molson Coors, Google, Databricks |
| [Vijil](https://www.vijil.ai/) | **$17M led by Brightmind, 25 Nov 2025**; $23M total | SmartRecruiters, DigitalOcean. Gartner Cool Vendor |
| [Zenity](https://zenity.io) | **$125M Series C, Norwest, Aug 2026**; claims ~$100M ARR | Gartner: "the company to beat in AI Agent Governance" |
| [Obsidian Security](https://www.obsidiansecurity.com/) | **$85M Series D at $1.1B**, ~4 Aug 2026 (Reuters) | T-Mobile, Databricks, S&P Global, Snowflake |
| [Microsoft Purview](https://learn.microsoft.com/en-us/purview/ai-agents) | n/a | Prompts and responses in the unified audit log — **but the coverage matrix shows ✕ for ChatGPT Enterprise agents and ✕ for Anthropic Claude Enterprise agents.** It is an audit trail for Microsoft's agents |

### The one genuinely empty sub-layer: portable receipts

**Nobody ships a signed, portable, third-party-verifiable record of what an agent
did.**

- **C2PA does not cover this.** The [2.2 spec](https://spec.c2pa.org/specifications/specifications/2.2/specs/C2PA_Specification.html)
  is media-asset provenance; its `actions` assertion documents operations
  performed *on an asset*. v2.0 actively **narrowed** scope, removing W3C
  Verifiable Credentials. Anyone pitching "C2PA for agent outputs" is describing
  something the spec does not do.
- **NIST has nothing agent-specific.** The [AI RMF](https://www.nist.gov/itl/ai-risk-management-framework)
  revision underway contains no agent logging, provenance or audit-trail work.
- **The best-in-class implementation is first-party and non-portable.** The
  [Anthropic Compliance API](https://platform.claude.com/docs/en/manage-claude/compliance-api)
  gives programmatic access to the org Activity Feed and, for Enterprise, **full
  session transcripts for Claude Code, Cowork, Claude Science and Claude for
  Microsoft 365** — a more concrete agent-action audit trail than any dedicated
  governance startup ships. But it proves things to the customer's *own* auditor,
  not to a counterparty. It raises the bar for this sub-layer; it does not close it.
- **The only attempt at the portable version is pre-product.**
  [Handshake.AI](https://handshake.ai) specifies signed receipts committing to a
  hash of the result, DIDs (`did:hsk:agent:...`), and a delegation chain of signed
  tokens narrowing scope at each hop — verifiable offline, "no phoning home." It
  explicitly wraps MCP and supplies the primitives A2A and AP2 "defer to a future
  layer." **Status: v0.2.3 spec, early access, SDKs in preview, production
  registry still planned. No funding, team or customers disclosed.** (Note the
  name collision with the unrelated student-jobs company.)
- Inside the payment protocols, the one merged receipt primitive is x402's
  [`extension-offer-and-receipt`](https://github.com/x402-foundation/x402/blob/main/specs/extensions/extension-offer-and-receipt.md)
  — **seller self-attestation**, on a wire format explicitly marked *not stable*.

**[UNVERIFIED]**: OpenAI's audit-log schema and whether Operator / ChatGPT Agent /
AgentKit actions are logged distinctly — platform.openai.com and help.openai.com
returned 403 to every automated fetch. Only [trust.openai.com](https://trust.openai.com)
was readable (SOC 2 Type 2, ISO 27001/27017/27018/27701, **ISO/IEC 42001:2023**,
PCI DSS v4.0.1, FedRAMP 20x).

**Verdict: GRC platforms CONSOLIDATING (closed). Portable cryptographic receipts
EMPTY. Regulatory urgency DEFLATED to December 2027.**

---

## 10. Verification of outcomes — the emptiest layer, and the most interesting

The question: an agent commissions work — from another agent, a human, a service.
How does anyone prove the work was actually done to spec before money moves?

### Every major protocol ships evidence and explicitly defers adjudication

| Protocol | What it does about non-delivery |
|---|---|
| **x402** | [FAQ](https://docs.x402.org/faq): `exact` is *"a push payment — irreversible once executed."* Refunds only as "seller sends a new token transfer back." **Silent on disputes, chargebacks, buyer protection, non-delivery** |
| **AP2** | Mandates and receipts create audit trails supporting *"dispute resolution."* **"Aiding in dispute resolution" is the entire dispute story** |
| **ACP** | Places "settlement, refunds, chargebacks and compliance" with merchant/PSP; lists **refund semantics as out of scope**; defines an `Adjustment` of kind `dispute` **with no process that produces one** |
| **A2A** | `COMPLETED` means the agent stopped working. No acceptance gate |

**The pattern is consistent and appears deliberate: sign everything, adjudicate
nothing.**

### The proposals exist; none has merged

x402 has **299 open issues**. Merged schemes are `exact`, `upto`, `auth-capture`,
`batch-settlement` — **no escrow scheme**. Merged extensions include `bazaar`,
`builder_code`, `offer-and-receipt` — **no dispute extension**.

Open and unmerged:
[#508 paywall fraud](https://github.com/x402-foundation/x402/issues/508) (from
Brave's security team),
[#1645 buyer protection](https://github.com/x402-foundation/x402/issues/1645),
[#2222 `scheme: "escrow"`](https://github.com/x402-foundation/x402/issues/2222)
(24 comments, author pinging maintainers by name, **no label after four months**),
[#2887 correctness/dispute layer](https://github.com/x402-foundation/x402/issues/2887)
(37 comments), [#2001](https://github.com/x402-foundation/x402/issues/2001),
[#2833](https://github.com/x402-foundation/x402/issues/2833),
[#2943](https://github.com/x402-foundation/x402/issues/2943),
[#3065](https://github.com/x402-foundation/x402/issues/3065),
[#1247](https://github.com/x402-foundation/x402/issues/1247).

#2887's opening line is the thesis of this whole section:

> *"Forty members are standardizing how agents pay. Nobody's standardizing what
> happens when the thing an agent paid for turns out to be wrong."*

AP2, all open: [#224 PactEscrow](https://github.com/google-agentic-commerce/AP2/issues/224),
[#99 verifiable receipts](https://github.com/google-agentic-commerce/AP2/issues/99),
[#338 dispute-time evidence](https://github.com/google-agentic-commerce/AP2/issues/338).

ACP: [#303, a full Dispute Resolution Extension](https://github.com/agentic-commerce-protocol/agentic-commerce-protocol/issues/303)
— filed 9 Sept 2026, by far the most rigorous artifact in this space (deterministic
decision rules, 120-day window, loser-pays fees, mandatory human sign-off,
provider-independence requirements) — **awaiting a sponsor, with one comment, and
that comment is the author's own.** Its motivation line:

> *"Card networks resolve **fraud**; they have said they do not yet resolve agent
> **scope and performance**."*

**That the most complete dispute design in the ecosystem has zero institutional
uptake is itself the finding.**

### Crypto verifiable compute: no genuine non-speculative usage anywhere

Assessed against four tiers — whitepaper / testnet / mainnet-with-farmed-usage /
real paying non-crypto-native customers. **Nothing reached the fourth.**

- **EigenLayer / EigenCloud** — a16z $100M Series B (Feb 2024) + $70M token
  purchase (Jun 2025). TEE-based, not zero-knowledge. **Exactly three named
  production integrations.** Protocol revenue near zero per DefiLlama
- **[Ritual](https://ritual.net/blog/introducing-ritual)** — $25M Series A
  (Archetype, Nov 2023). **The funded product is dead** — its own docs say
  *"No Infernet. Ritual Chain replaces the Infernet protocol entirely."* Ritual
  Chain is testnet
- **Gensyn** — $43M a16z Series A; mainnet April 2026; **flagship application is
  a prediction market**, not paid training
- **zkML** (EZKL, Lagrange DeepProve, Giza, Modulus) — overhead **100,000–
  1,000,000×**; **~300k gas ≈ $20 per on-chain verification**. **Exactly one
  production use found.** Modulus acquired; Giza pivoted away from zkML
- **Bittensor** — Yuma consensus is a stake-weighted median of validators'
  *subjective scores*, not cryptographic verification
- **Agent tokens** — ai16z/ElizaOS went **$2.4B → ~$2.3M market cap**; the founder
  declared the token dead and wound down the foundation in August 2026

**[ERC-8004 "Trustless Agents"](https://github.com/ethereum/ERCs/blob/master/ERCS/erc-8004.md)**
— status **Draft**, created August 2025, thirteen months as a draft. Read its
framing carefully: its Validation registry provides *"generic hooks for requesting
and recording independent validators checks."* **It standardises hooks for
validation; it does not validate.** And: *"Payments are orthogonal to this
protocol and not covered here."*

The perfect illustration: AP2's **PactEscrow** is deployed on Arbitrum with real
contract addresses and an MCP server, and its claimed live usage is *"SWORN
Protocol completed 3 production cycles."* Three. Its verification is
`submitWork(pactId, sha256(work))` → payer approves. **A hash commitment proves
the worker committed to *some bytes*, not that the bytes are good** — a flaw
common to nearly every escrow design in this space.

### Agent-to-agent output verification is structurally empty

If agent A pays agent B for a research report, who checks it's good? Today: A's
own evals, or nobody. **Every evaluator in the market is retained by one side of
the transaction.** Braintrust positions explicitly as internal developer tooling.
Galileo was absorbed into Cisco. **Patronus AI — which originally marketed itself
as independent third-party evaluation — repositioned away from that.** The
direction of travel is wrong: evaluators are being absorbed into enterprise
security stacks where the customer is the agent's *operator*, not its counterparty.

The one project with real community interest is academic:
[Guardians](https://github.com/metareflection/guardians) (151 stars), implementing
Erik Meijer's *"Guardians of the Agents: Formal Verification of AI Workflows,"*
CACM January 2026.

### The human-work rail is contracting, not expanding

**Amazon Mechanical Turk — the original API over human labour — closes 30
September 2026**, confirmed on [mturk.com](https://www.mturk.com/) and
[requester.mturk.com](https://requester.mturk.com/). The canonical programmatic
human-work API is being retired at precisely the moment agents acquire the ability
to call one.

- **[Upwork](https://www.upwork.com/developer/documentation/graphql/api/docs/index.html)**
  — GraphQL API contains **zero mentions of "agent" or "AI."** Its Uma Recruiter
  is Upwork's own agent sourcing freelancers *for a human client* — the inverse
- **[Fiverr Go](https://www.fiverr.com/news/fiverr-go)** — lets *freelancers*
  license AI models of their own work. Also the inverse. No public agent API
- **Mercor** ($350M at $10B valuation), **Scale AI** (Meta paid $14.3B for 49%),
  **Surge**, **Prolific**, **Toloka** — all enterprise- or researcher-initiated.
  **No agent-callable API surfaced for any of them**

### The direct competitors

**[RentAHuman.ai](https://rentahuman.ai)** — YC Spring 2026, 3 people, founder
Alexander Liteplo. A real agent API: REST plus a published MCP server
(`npx rentahuman-mcp`) with `search_humans`, `create_bounty`, `accept_application`,
`x402_fund_wallet`. An agent can onboard with **zero human involvement** by paying
a $10 USDC x402 challenge. YC page claims *"500k users and $20k MRR in 2 weeks."*

Be skeptical. [Gizmodo, Feb 2026](https://gizmodo.com/rent-a-human-site-lets-ai-agents-hire-an-irl-set-of-opposable-thumbs-2000717958)
reported **70,000 human signups against ~70 connected AI agents** — a 1000:1
imbalance — only **13% connected a crypto wallet**, crypto-only payment, and
**one documented completed task.** The site now claims 802,586 registered humans;
treat as self-reported.

**Its verification model is escrow plus requester-defined evidence plus manual
buyer approval** — *"Only release funds when you're satisfied."* **That is buyer
attestation, not neutral verification** — architecturally identical to every other
attempt in this report.

Also live but tiny: [Sinkai](https://sinkai.tokyo) (Japan; `POST /api/call_human`;
explicitly for "on-site checks, physical evidence collection, and local human
verification").

**Two pivots away from this exact thesis** are themselves a demand signal:
HumanLayer (above), and **Payman AI**, which reportedly moved from "AI agents
paying humans" to enterprise/community-bank banking automation — though it has the
most real adoption evidence of any startup in the payments section, with named
bank customers (Middlesex Federal Savings, Citizens State Bank of Ouray) and ICBA
ThinkTECH selection. **[Payman's pivot is UNVERIFIED — based on current versus
remembered site copy.]**

### The startups attacking this head-on: a graveyard of correct diagnoses

| Project | Traction |
|---|---|
| [Settld](https://settld.work) — intercepts 402, escrows, deterministic verification | [HN: 2 points](https://news.ycombinator.com/item?id=47011510) |
| [Agntor](https://github.com/agntor/agntor) — identity + trust score + escrow + AI judge | **11 GitHub stars** |
| [BountyBook](https://www.bountybook.ai/) — escrowed USDC, AI oracle verifies | Live, **"0 open" bounties**, self-described experimental |
| [ClawGig](https://clawgig.ai) | Domain returns HTTP 402 |
| [OQP](https://github.com/OrangeproAI/open-qa-protocol) | **18 stars** |
| MeshLedger, agent-escrow-protocol, UAIP, Backproto | **3, 6, ~0 stars** |

Every one independently identified the exact gap. **None has traction.**

### What the market actually bought instead

Two things are working commercially, and both route around per-transaction proof:

1. **Insurance.** [Armilla](https://www.armilla.ai/) sells affirmative AI
   liability insurance and an **AI Performance Warranty** as a **Coverholder at
   Lloyd's**, backed by Chaucer, AXIS Capital, Convex, Swiss Re, Greenlight Re —
   explicitly covering *"AI Agent Mistakes… including failure to escalate
   recommendations, incorrect decisions causing damages."* [AIUC](https://www.aiuc.com/)
   sells the AIUC-1 standard plus certification plus insurance, with **KPMG**
   (first Big Four firm certified), **Intercom** (its Fin agent) and **ElevenLabs**
   as customers, and Schellman as first accredited auditor.
2. **Vertical integration.** [Sierra](https://sierra.ai/) sells on outcomes —
   *"Pay for a job well done"* — to Rocket Mortgage, Gap, SoftBank, Uber,
   Vanguard. But **how a "resolution" is defined and adjudicated is not
   published.** It is a bilateral contract term measured by the vendor.

**Insurance answers "what if the work wasn't done" with actuarial pooling instead
of per-transaction proof — and it is shipping now, with name-brand customers,
while verification rails are not.** That is the most important competitive fact in
this section.

**Verdict: EMPTY — and honestly, empty mostly for lack of demand rather than
neglect.** The argument on both sides is in the conclusion.

---

## Conclusion: which layers are most underserved

Three tests were applied: (1) is the gap named by the people who own the rails,
(2) is the supply of credible attempts low relative to the need, and (3) would
solving it unblock something that is currently blocked.

### 1. Outcome verification — the emptiest, and the one to be most careful about

**Why it is underserved:** Every major protocol documents this gap in public, at
length, in its own issue tracker, and then declines to close it. x402 has an
escrow proposal with 24 comments and no label after four months. ACP has a
complete dispute-resolution design awaiting a sponsor whose only comment is the
author's own. A2A's `COMPLETED` explicitly means "the agent stopped working," not
"the output is correct." The card networks have said publicly that they resolve
fraud, not agent scope and performance. This is not a gap nobody noticed. It is a
gap everybody noticed and nobody wants.

**Why to be skeptical anyway — and this is the honest part.** The reason nobody
wants it may simply be that there is no demand yet:

- **The transactions are too small to dispute.** Average x402 payment is **~$0.32**.
  ACP #303's own proposed fee floor is **$1–$50 per resolution** — *more than the
  median disputed transaction is worth.* Dispute infrastructure is economically
  irrational below a transaction-size threshold, and the market sits far below it.
- **The institutions that would need it have chosen not to build it.** Visa,
  Mastercard, Amex, Google, Stripe, AWS, Shopify and Adyen are all in the x402
  Foundation. They have unlimited capacity to ship a dispute scheme. They haven't.
- **Everyone who built it found no buyers.** A dozen independent projects, all at
  0–18 GitHub stars and 1–3 HN points. BountyBook shows zero open bounties.
  PactEscrow's flagship customer completed three cycles.
- **The labour rail is contracting.** MTurk closes in 17 days. RentAHuman has
  ~1000 registered humans per connected agent.
- **The market already bought a cheaper answer: insurance.** Armilla and AIUC are
  shipping with Lloyd's syndicates, Swiss Re, KPMG and Intercom behind them.
  Actuarial pooling beats per-transaction cryptographic proof on cost, and it is
  winning right now.

**The counter-argument that keeps this in first place:** $0.32 reflects API
metering — paying for a weather call. The moment agents commission *work* rather
than *data*, values jump two to four orders of magnitude and every argument above
inverts. A $40 job that goes uncompleted is worth disputing; a 39-cent API call is
not. And the hard prerequisite is already done: the **evidence primitives have
merged** (x402's offer-and-receipt, AP2's mandates, ACP's dispute Adjustment).
Adjudication is missing *by explicit scoping decision*, not because anyone failed
to solve it. That is a very different and much better situation than a problem
nobody can formulate.

**A second ecosystem independently filed the same bug.** It is not only the
payment protocols. **CrewAI issue #5802 — 119 comments, open — reports that tool
re-execution on retry has no idempotency guard, making "duplicate payments,
emails, trades possible."** LangGraph #7417 reports long tool calls silently
re-executed from checkpoint. A2A #1713 asks for an "accountability layer for
first-contact transactions" between parties with no shared auth server. Four
separate ecosystems — payment protocols, orchestration frameworks, agent
protocols, and durable execution — have converged on the same missing primitive:
**a record that binds intent to payment to deliverable, is idempotent under
retry, and is checkable by someone who ran none of the services involved.**

**The trap to avoid:** do not position as a neutral judge of *quality*. That is
both economically and epistemically hard — the only available judges are the buyer
(interested party), an LLM (gameable, itself unverifiable), or a credentialed
human (costs more than most transactions are worth). ACP #303 confronts this
honestly, lands on the human, and that is probably why it will not be adopted at
current transaction sizes.

**The defensible band** is the narrow, boring one that five independent proposals
are converging on and nobody owns: **standardised evidence capture** — a signed,
hash-chained record binding *intent → payment → deliverable*, with a
**content-derived join key** so the record is recomputable by a party who runs
none of the involved services. Scope verification to machine-checkable "done" — a
geotagged photo with intact EXIF, a merged PR, an HTTP probe, a delivery
confirmation — not quality judgment. And model the states every digital-first
design omits, flagged by a physical-commerce operator in x402 #2887: **a pending
state that can sit open for days**, and **"paid and later refunded" as a terminal
state distinct from "paid and slashed."**

### 2. Delegated authority across a chain — the most badly needed relative to supply

**Why it is underserved:** OAuth's on-behalf-of is one hop. Agents are N hops.
There is no adopted standard that expresses "A acts for B acts for human C, and
each hop narrows." Eleven-plus competing IETF drafts, zero adopted. The OAuth WG
took on "Complex Delegation" in its June 2026 recharter and has **no milestone and
no adopted document**. The IETF **declined the BoF** that would have owned it.
MCP core has no agent principal at all. An author of OAuth is writing a 143-page
replacement outside the working group.

**Why this is the highest-confidence gap:** unlike verification, demand here is
demonstrated rather than hypothetical. Uber built a bespoke STS with a custom
`act_chain` JWT claim because nothing existed. Every interesting startup in the
space — Alter, Clawvisor, Britive, Aembit — is building a proprietary policy
engine for the same reason. The complaint is not "we wish this existed," it is "we
already built our own and it was expensive."

**Where the opening sits:** between "give the agent a governed identity" (what the
IdPs sell) and "the agent should never hold a durable credential" (what the YC
cohort is building). Nobody has made the second one *boring enough for a normal
developer to adopt* — which is exactly what that HN commenter said: *"there must
be a cleaner way to do it."*

**The caveat:** this is standards work, and standards work is slow, political, and
hostile to startups. Okta won the enterprise slice by doing the IETF work first
and the product second, over about two years. That is the price of entry.

**A strong corroborating signal from a different direction.** The
approval, observability and sandboxing layers all independently collapse into the
same unmet need, and it is adjacent to this one. Human-in-the-loop gave up on
prevention-by-asking and now points at sandboxes. Sandboxing's own best critics
say the sandbox constrains the filesystem, not the credentials — *"almost exactly
the same security risk surface"* — and the credentials are the entire point of the
agent. Observability has visibility but no enforcement: *"the actual shutdown
mechanism often ends up being manual."* Three separate builders, in three separate
threads, described hand-building the same thing: **scoped short-lived credentials
the agent never sees, egress allowlisting through a logging proxy, and a real kill
switch.** Granola reached it from the opposite direction — *"The auth policies
don't get tired."* Auth0's CIBA, Permit.io's MCP Gateway, Cerbos' per-hop PBAC,
Google DeepMind's CaMeL capability model, Meta's Rule of Two, and Anthropic's own
credential-masking proxy are all partial approaches to that one boundary. **Nobody
has made it boring enough for a normal developer to adopt** — which is the same
sentence that ends the paragraph above.

### 3. Portable agent receipts — smaller, but genuinely unoccupied

**Why it is underserved:** every audit and governance product shipping today
proves things to the *operator's own auditor*. Anthropic's Compliance API is the
best-in-class version of that shape — full session transcripts for Claude Code and
Cowork — and it is still first-party and non-portable. Microsoft Purview's own
coverage matrix shows ✕ for Anthropic and ✕ for OpenAI agents. C2PA is media-only
and narrowed its scope. NIST has nothing. The only attempt at a portable,
offline-verifiable receipt is Handshake.AI, which is pre-product with no
disclosed funding, team or customers.

**Why to discount it:** the regulatory forcing function just slid 16 months to
December 2027, and the GRC layer above it has consolidated with $1B M&A and a
Gartner MQ. This is a real hole, but it is a hole in the floor of a building
somebody else already owns.

### What not to build

- **Another payment protocol.** There are six. The rails will be given away by
  Cloudflare, Stripe and Coinbase.
- **Human-in-the-loop as a product.** The category dissolved; the flagship company
  left; it ships free in every framework and in the protocols themselves.
- **Observability or evals.** ~30 players, 8 acquired, and OpenAI lists 28 of them
  as optional plugins to its own free tracer.
- **Prompt-injection guardrails.** Twelve acquisitions, the exit window closed
  around Q4 2025, and 12 published defenses were broken at >90% attack success
  rate.
- **An MCP directory.** There are at least six, one lists 20,946 servers, and one
  has paused submissions.

### The one-line version

**The layer everyone is building is the layer that already works — moving money.
The layer nobody is building is the layer that decides whether the money should
have moved.** The first is crowded by six protocols and forty foundation members
doing $24M a month, half of it wash trading. The second is documented as missing
in four protocols' issue trackers, has a complete design sitting unsponsored, and
is currently being answered by Lloyd's of London underwriters instead of by
software.

Whether that is an opportunity or a correctly-priced absence depends entirely on
one variable: **whether agents start commissioning work rather than metering
data.** At $0.32 a transaction, the absence is correct. At $40, it is a hole.

---

## Everything that could not be verified

**Layer-wide:** Reddit blocked all automated access; builder sentiment is Hacker
News and GitHub only. The session's web-search budget was exhausted partway, so
private funding figures are systematically weaker than protocol and standards
evidence.

**Payments:** Mastercard primary sources (403 on every fetch) — all Agent Pay /
AP4M detail is secondary. Amex primary sources (403) — ACE details via payments
press only. Anthropic's Sept 2026 commerce announcement — corroborated by seven
secondary outlets, not primary-verified. Ant International's reported "120M
transactions in one week (Feb 2026)" — third-party repo only; if accurate it
dwarfs all Western protocols and deserves dedicated follow-up. Payman's funding —
no round found anywhere. Skyfire's funding after Oct 2024 — none found in two
years. Crossmint's €20.7M April 2026 round — investor's own page only. **Volume
for every protocol except x402.**

**Identity:** Funding rounds for WorkOS, Descope, Stytch, Clerk, Aembit, Entro,
Token Security, Britive. NewCore's reported $300M seed valuation (secondary).
Entra Agent ID's exact GA date. Google Cloud Agent Identity GA vs preview (docs
don't state it). Cloudflare's reported "84% of AI browser traffic" (secondary).

**Discovery / orchestration / memory:** The registry figures are my own
measurements (full pagination of 102,569 records, `/metrics` read, and a
400-server liveness probe on 2026-09-13), not vendor claims — but the **"88%
reachable" figure is an upper bound**, since an unknown share of the 51.8% HTTP
403s are Cloudflare bot-blocks on an unauthenticated probe rather than genuine
auth gates. Third-party directory counts for Glama, mcp.so, PulseMCP, Pipedream
and Klavis are JS-rendered and **unverified**; Cloudflare and Vercel MCP hosting
were not researched. **No named A2A production deployment was found anywhere** —
treat "A2A is in production" as unverified in both directions. Funding for **Zep
(never announced), Cognee, turbopuffer, Restate, DBOS, Honcho** and Chroma's later
rounds is unverified, as is Inngest's $21M series label. CrewAI's self-reported
"65% of Fortune 500" and "450M+ workflows/month" are **unaudited and likely count
any `pip install`**. AGNTCY's loss of LangChain and Galileo as partners is
inferred from a current partner list, not an announcement. Pinecone acquisition
rumors are **unverified**.

**HITL / observability / sandboxing:** HumanLayer's funding amount — aggregators
report figures from $500K to "more than $3M" that are mutually inconsistent, and
TechCrunch has zero articles on them; treat any number as unverified. **Galileo's
status is an unresolved conflict between my own sources** — one pass reported a
Cisco acquisition ~April 2026, another found it independent on its Oct 2024
Series B. Funding for Steel, Anchor, Hyperbrowser, Notte, Skyvern, Lightpanda,
Runloop, Northflank, Oso, Cerbos, Permit.io, Arcade.dev, Honeycomb, Temporal.
E2B's ">$37M total." CodeSandbox→Together AI date and terms. Ona→OpenAI terms.
The OWASP GenAI LLM Top 10 2025-vs-2026 edition conflict (the homepage and the
canonical list page disagree). Prompt Security's acquisition consideration
(SentinelOne 10-Q not read line-by-line). A claim that OpenClaw was absorbed by
OpenAI — single-blog sourced, **unverified**. The widely-repeated OpenClaw figures
(42,000 exposed instances, 78% unpatched, 1.5M leaked keys) come from a vendor
content-marketing post — **treat the numbers as unverified** even though the
underlying incidents are well corroborated.

*(The 13.6%-vs-89% and 66.3%-over-409k approval statistics were flagged as
unverified in an earlier draft and have since been traced to primary sources —
Anthropic's 2026-08-07 auto-mode post and the scalex.dev study respectively. They
are now cited inline in section 6.)*

**Audit / verification:** EUR-Lex verbatim recital text for Reg. (EU) 2026/1744
(HTTP 202, empty body) — regulation number and dates confirmed from two EU
sources. OpenAI's audit-log schema (403). Drata, Arthur, Noma, Knostic, Token
Security, Britive funding. The *"Can Trustless Agents Be Trusted?"* ERC-8004 Sybil
audit and its reported 73.5% / 59.2% / 90.6% fake-review rates — **arXiv returned
nothing and then rate-limited; strong lead, do not cite without confirming.** The
Yahoo Finance "$435M across 12 financings" aggregate and the Alice ($140M) and AIR
($50M) rounds — single-source. Sierra's outcome-measurement methodology (not
published). RentAHuman's 802,586 figure (self-reported, contradicted by Gizmodo's
70,000 in Feb 2026). AIUC's funding.
