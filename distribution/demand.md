# Lamdis Exchange — where the demand already is

Research note, 2026-09-02. Every price cited. Nothing here has been posted.

---

## 1. Twenty buyer segments already paying humans to check an address

| # | Segment / who buys | What a check pays today | Incumbents | Why proof-on-payment via an agent API wins |
|---|---|---|---|---|
| 1 | **Mortgage servicers, occupancy verification** | HUD caps ongoing inspections at **$30**; nationals pay contractors **$5–$10**, some **$3** for two photos ([Safeguard](https://safeguardproperties.com/hud-update-to-property-inspection-fees/), [PreservationTalk](https://www.preservationtalk.com/threads/another-safeguard-vendor-looking-for-10-00-inspections.2553/)) | Safeguard, MCS, NFR | Layers of subcontracting eat most of the fee; a direct board pays the walker more and the buyer less. |
| 2 | **GSE-reimbursed exterior inspections** | Fannie Mae reimburses **$20** per exterior inspection ([TENA](https://www.tenaco.com/fannie-mae-increases-expense-reimbursement-limit-for-interior-and-exterior-property-inspections/), [Servicing Guide D2-2-10](https://servicing-guide.fanniemae.com/svc/d2-2-10/requirements-performing-property-inspections)) | same nationals | A geofenced photo with a one-time code beats a scanned PDF form as evidence. |
| 3 | **Vacant-property insurers / carriers** | Policies require inspection every **7–30 days**, some every **48–72 hrs** ([Veritech](https://veritech-security.com/insurance-requirements-for-vacant-properties-what-your-policy-really-expects/), [Vacant Express](https://vacantexpress.com/2026/05/27/how-often-should-you-inspect-a-vacant-property/)) | Paramount, in-house | Recurring, cheap, and the whole product is a dated photo — the exact shape of an observe job. |
| 4 | **Lenders' drive-by property condition** | **$10–$15** per drive-by; BPO **$50–$200** ([Nolo](https://www.nolo.com/legal-encyclopedia/what-fees-can-the-lender-charge-if-im-late-mortgage-payments.html), [Wikipedia BPO](https://en.wikipedia.org/wiki/Broker%27s_price_opinion)) | TrendSource, NFR | Below any dispatch platform's floor; a zero-fee exchange is the only economics that clears. |
| 5 | **Construction lenders, draw inspections** | **$75–$150** residential, **$250–$350** commercial ([Inspectify](https://knowledge.inspectify.com/inspection-type-guide-for-construction-lenders)) | Millman, RAZE, Inspectify | The `stages` field already splits pay per evidenced milestone — draws are staged jobs. |
| 6 | **Retail merchandising audits** | Gigwalk gigs **$3–$100**, most **$3–$12** ([Gigwalk review](https://wellkeptwallet.com/gigwalk-review/)) | Gigwalk, Survey.com | An agent can post 400 store checks in one call instead of building a panel. |
| 7 | **CPG brands, shelf/display checks** | Field Agent **$3–$20**; Observa **$3–$25** ([Loud Money Moves](https://loudmoneymoves.com/observa-app-review/)) | Field Agent, Observa | Brands already accept phone photos; only the ordering interface changes. |
| 8 | **Mystery shopping** | **$10–$20** per shop typical, up to **$50** ([MoneyMakingMommy](https://www.moneymakingmommy.com/mystery-shopping-jobs/)) | MSPA networks | Bonus_minor pays a premium on the finding without biasing the "no" answer. |
| 9 | **Out-of-home / billboard proof-of-performance** | Nationwide POP photos in **24–48 hrs** ([FotoFetch](https://www.fotofetch.com/)) | FotoFetch, OOH Audit, Carroll | Verify a flight the morning it starts, not two weeks later. |
| 10 | **Zoning/permit sign compliance** | Signs must be posted **15 days** before hearing, with a **photograph** filed by affidavit ([Prince George's Co. §27-125.03](http://princegeorges-md.elaws.us/code/coor_subtitle27_pt3_div1_subdiv1_sec27-125.03), [PWC VA](https://www.pwcva.gov/assets/2025-06/Sign%20Posting%20Instructions%20for%20Land%20Use%20Review%20Cases.pdf)) | applicant/expediter | Legal deadline plus photo requirement plus house number in frame — the native deliverable. |
| 11 | **Short-term-rental compliance (cities)** | Granicus Host Compliance monitors **60+ listing sites**, but evidence collection is desk-based ([Granicus](https://granicus.com/product/short-term-rentals-host-compliance/)) | Granicus, Deckard | Desk monitoring can't prove a lockbox is on the door. A $12 observe job can. |
| 12 | **EV charge-point operators (NEVI)** | NEVI mandates **>97% annual uptime per port**; third-party field audits recommended ([TEAL](https://tealcom.io/post/reliability-redefined-inside-nevis-97-uptime-mandate-for-ev-chargers/), [edrv](https://www.edrv.io/guide/nevi-minimum-standards-for-ev-charging-infrastructure)) | ChargerHelp, in-house | Telemetry says "available"; only a person confirms the screen works. |
| 13 | **Merchant acquirers, underwriting site visits** | On-site inspection required at minimum by underwriting standards ([OCC Handbook](https://www.occ.treas.gov/publications-and-resources/publications/comptrollers-handbook/files/merchant-processing/pub-ch-merchant-processing.pdf), [FraudPractice](https://www.fraudpractice.com/operational-techniques/on-site-merchant-inspections)) | Metro Site Inspections, QuadraPay | Onboarding is API-driven; the site visit is the last manual step. |
| 14 | **Process servers / skip trace** | **$50–$300** per attempt; extra attempts **$25–$50**; skip trace **$25–$75** up ([Undisputed Legal](https://undisputedlegal.com/how-much-does-a-process-server-cost-a-complete-breakdown/), [Central Valley](https://centralvalleyprocessservers.com/2024/10/06/do-process-servers-charge-for-each-attempt/)) | ABC Legal, One Legal | A $10 occupancy observe before dispatch kills the $50 wasted attempt. |
| 15 | **Insurance underwriting exterior inspections** | Field inspectors **$18.99–$28.46/hr** ([ZipRecruiter](https://www.ziprecruiter.com/Jobs/Insurance-Field-Inspector/--in-California)) | Davies, National Risk Services | Per-job pricing beats per-hour when the job is 8 minutes long. |
| 16 | **Google Business Profile video verification** | Google demands video of **street sign, building number, and signage**; agencies charge **$20–$799/mo** ([Google](https://support.google.com/business/answer/14271705?hl=en), [LocalSEOProducts](https://www.localseoproducts.com/roundups/best-google-business-profile-verification-services)) | local SEO agencies | Google's own spec *is* "house number in frame" — identical deliverable. |
| 17 | **Remote renters & relocation** | FTC and Michigan AG both advise "ask someone you trust to see it" ([FTC](https://consumer.ftc.gov/articles/rental-listing-scams), [Michigan AG](https://www.michigan.gov/consumerprotection/protect-yourself/consumer-alerts/scams/rental-listing-scams-how-to-spot-and-dodge-them)) | none — informal favours | Consumer agencies recommend the product; nobody sells it. |
| 18 | **Mapping / AI labs needing ground truth** | Hivemapper pays per-km in HONEY, higher in under-mapped hexes ([docs](https://docs.hivemapper.com/contribute/driving/)) | Hivemapper/Bee Maps, Premise (**$0.20–$1.25**/task, [Frugal for Less](https://www.frugalforless.com/the-premise-app-pays-you-to-take-pictures-from-your-phone/)) | Labs want *targeted* ground truth at named coordinates, not dashcam sweeps. |
| 19 | **Agent developers generally** | RentAHuman pays **$1–$100** per task, claims **787k+** humans, 100+ countries ([RentAHuman](https://rentahuman.ai/)) | RentAHuman, ClawGig, WORQ | Proven demand; we differ on no-account first job and zero fee. |
| 20 | **Errands / line-standing / small do-jobs** | TaskRabbit errands **$28/hr**; line-standing **$27/hr** ([TaskRabbit](https://www.taskrabbit.com/cost-guides/run-errands), [Axios](https://www.axios.com/2025/02/02/professional-line-stander-taskrabbit)) | TaskRabbit, Thumbtack | No agent-facing API exists there; `POST /v1/tasks` with no header is the whole onboarding. |

**Wedge confirmed:** segments 1–4 and 6–8 all clear in the **$3–$30** band. Segments 1, 4, 6, 7 and 18 sit *below* what any human-dispatch incumbent can profitably serve, because their take rate exceeds the job value.

---

## 2. Ten places agent builders ask for physical-world tools

Rules verified where possible; **check the live sidebar before posting.** Nothing posted.

**1. r/AI_Agents** — reddit.com/r/AI_Agents. Explicitly tolerates self-promotion, but demonstrated behaviour beats feature lists ([RedditMaster](https://www.redditmaster.com/best-subreddits/for-ai-agents)).
> Everyone's agent stack stops at the same wall: it can browse, call APIs, and reason, and then somebody asks "is the sign actually up?" and the trace dies. Three patterns I've seen work. (a) Treat the physical step as a *tool call that returns evidence*, not a human handoff — your planner shouldn't know a person did it. (b) Make the predicate falsifiable before dispatch: "a sign reading X is visible from the sidewalk at address Y" beats "check on the store." (c) Pay for admissible evidence either way, or you've built an incentive to answer yes. I've been building that as an exchange (exchange.lamdis.ai) where a job is one POST and payment settles on proof. Happy to share the predicate schema.

**2. Hacker News, Show HN** — news.ycombinator.com/showhn.html. Must be runnable, no landing pages or signup walls; don't use HN primarily for promotion ([guidelines](https://news.ycombinator.com/showhn.html), [dang](https://news.ycombinator.com/item?id=24354016)).
> Show HN: an exchange where an agent pays a person to check whether something is true at an address. The first job needs no account — `POST /v1/tasks` with no auth header, or `claude mcp add --transport http lamdis https://exchange.lamdis.ai/mcp`. A job is a predicate ("a permit placard is posted at 123 Main"), a fee in minor units, and a geofence. The worker returns a photo containing a one-time code and the house number; payment settles on that evidence, and an honest "no" pays the same as a "yes" so nobody is bribed toward the answer you want. Zero platform fee. Supply is thin and Detroit-centred today — that's the honest limitation. Curious what predicates break it.

**3. r/mcp** — reddit.com/r/mcp. One of the two largest MCP communities; rules not machine-readable, check sidebar ([overview](https://www.aibuilderclub.com/blog/best-reddit-communities-ai-builders-2026)).
> Most MCP servers wrap an API that already exists. The interesting gap is tools with no API behind them at all — anything that requires a body at a location. Design notes if you're attempting one: the tool should be `read-only`-annotated when it observes and `destructive` when it causes something, because Claude's connector review rejects mis-annotated tools outright. Return structured evidence, not prose: photo URL, capture timestamp, GPS at capture, and the nonce you issued. And make the failure case cheap — an agent that can't distinguish "no evidence yet" from "the answer is no" will loop. Ours is at `https://exchange.lamdis.ai/mcp` if you want a reference implementation to argue with.

**4. n8n Community, "Built with n8n"** — community.n8n.io/c/built-with-n8n/15. Category exists to show off what you built ([category](https://community.n8n.io/t/about-the-built-with-n8n-category/3411)); MCP Client Tool node is the connection path ([docs](https://docs.n8n.io/integrations/builtin/cluster-nodes/sub-nodes/n8n-nodes-langchain.toolmcp)).
> Sharing a workflow pattern for anyone automating field checks. Trigger on a row in your property/store sheet → AI Agent node → MCP Client Tool pointed at a remote server → wait node polling job status → write photo URL and verdict back to the sheet. Two gotchas: set `N8N_COMMUNITY_PACKAGES_ALLOW_TOOL_USAGE=true` or the node won't appear as a tool in the Agent node, and give the poll a real backoff — physical jobs settle in hours, not seconds, so a tight loop just burns executions. I'm using exchange.lamdis.ai/mcp as the server (it posts a paid job at an address and returns a geofenced photo), but the pattern holds for any streamable-HTTP server.

**5. Cursor Forum, "Built for Cursor"** — forum.cursor.com/t/about-the-built-for-cursor-category/146852. For extensions, MCP servers, rules and integrations; posts must include setup instructions and a repo/download link.
> Setup notes for an MCP server that does something Cursor can't: dispatch a paid real-world check. Add to `mcp.json` as a streamable-HTTP server pointing at `https://exchange.lamdis.ai/mcp`; no key needed for the first job. Where it's actually useful in a dev loop: verifying that a deploy of physical signage, a kiosk, a store display or a QR code is live at a real address, without leaving the editor. Two things I'd do differently if you build something similar — keep tool count under about six so the model doesn't thrash, and return a single structured object rather than a chatty string, because Cursor's agent will re-summarise prose and lose the evidence URL.

**6. OpenAI Community Forum** — community.openai.com. Developer forum; standard Discourse norms, contribute before linking.
> If you're building with the Apps SDK and hitting the "my agent can't act physically" wall: the pattern that works is to model the physical action as a normal MCP tool with a strict return schema, and to make the *predicate* the contract rather than the task description. "Is a sign reading X visible from the sidewalk at Y" is checkable; "check the store" is not. Also worth knowing before you submit to the directory: you must supply the MCP server URL directly even when it already backs a Codex integration, and you must verify domain control. I've been running this against exchange.lamdis.ai/mcp. Happy to compare notes on review turnaround.

**7. Anthropic Connectors Directory** — [submission docs](https://claude.com/docs/connectors/building/submission). Not a forum: a listing. Streamable HTTP only (SSE rejected), every tool annotated read-only/destructive, public privacy policy or immediate rejection.
> (Submission copy, not a post.) Lamdis Exchange lets Claude commission a person to verify a fact at a physical address, or perform a small errand, and pays only against evidence. Read-only tools: search jobs, get job status, get evidence. Destructive tools: create task, award bid, release payment. Observation fees settle whichever way the finding goes, so the connector cannot be steered toward a preferred answer. Evidence is a photograph containing a one-time code we issue and the address's house number, plus capture GPS inside a stated radius. Zero platform fee; the worker receives the posted amount. Server: `https://exchange.lamdis.ai/mcp`.

**8. Glama MCP directory** — [glama.ai](https://glama.ai/). Auto-indexes open-source servers from GitHub; form-based, manually reviewed submission ([guide](https://tallyfy.com/how-to-list-mcp-server-registry-smithery-glama-pulsemcp/)).
> (Listing copy.) An MCP server whose tools have no API behind them — they have people. Post a predicate and a fee at a lat/lon; a person nearby claims it, photographs the answer with a one-time code and the house number in frame, and gets paid on evidence. Observations pay the same for "no" as for "yes". Six tools, all annotated. First call works with no account. Useful for: occupancy and vacancy checks, storefront hours, signage and permit verification, EV charger uptime, shelf audits, and anything an agent currently guesses at from stale web data. Transport streamable HTTP at `https://exchange.lamdis.ai/mcp`. Supply is currently concentrated in metro Detroit.

**9. Smithery** — [smithery.ai](https://smithery.ai/). Publish with `smithery mcp publish "https://your-server.com" -n org/name` or the web dashboard ([guide](https://tallyfy.com/how-to-list-mcp-server-registry-smithery-glama-pulsemcp/)).
> (Listing copy as above; Smithery's one-line summary: "Pay a person nearby to verify a fact at an address; settles on photographic proof.")

**10. r/ClaudeAI and r/LocalLLaMA** — near-universal restriction on direct promotion; the working ratio is roughly 90% help to 10% mention ([Linkeddit](https://linkeddit.com/blog/best-subreddits-for-ai-marketing-2026)).
> A thing I got wrong for a while: I treated "the model can't do X" and "the model can't reach X" as the same failure. They aren't. Reaching failures are solvable with a tool; capability failures aren't. Almost everything people describe as "my agent can't handle the real world" is a reaching failure — the model knows exactly what evidence would settle the question, it just has no arm. So the useful exercise is: write down the single photograph that would end the argument, then ask what it would cost to obtain. Usually $5–$20. I built an exchange for exactly that (exchange.lamdis.ai) but the exercise is worth doing even if you never use it.

---

## 3. Ten Detroit jobs the exchange could post this week

**Sources:** BSEED Vacant Property Registrations — `https://services2.arcgis.com/qvkbeam7Wirps6zC/arcgis/rest/services/bseed_vacant_property_registrations/FeatureServer/0/query` ([portal](https://data.detroitmi.gov/datasets/detroitmi::vacant-property-registrations-1/about)); Detroit Charge Ahead sites — `.../Detroit_Charge_Ahead_sites_view/FeatureServer/0/query`; OSM via Overpass for storefront hours and street furniture. All coordinates come from those queries, not estimates.

Note from `server.go`: `attempt_minor` "applies to do-jobs only" — for observations use `bonus_minor`. Fees are minor units (cents).

```jsonc
// 1. Vacant commercial storefront — VPO2026-00463, registered 2026-04-10
{"kind":"observe","predicate":"The building at 7421 W McNichols Rd is boarded or unoccupied, with no active business trading from it.","deliverable":"Two photos from the public sidewalk: one showing the storefront and one showing the house number 7421, both including the one-time code.","where":"7421 W McNichols Rd, Detroit, MI 48221","area":"Fitzgerald/Marygrove, Detroit","lat":42.416966,"lon":-83.145592,"radius_m":60,"fee_minor":1200,"bonus_minor":300,"ttl_seconds":172800}

// 2. Vacant residential — VPO2026-00864
{"kind":"observe","predicate":"9921 Mansfield St shows no sign of occupancy: no vehicle, no curtains, mail or debris accumulated.","deliverable":"Three photos from the street including the house number and the one-time code.","where":"9921 Mansfield St, Detroit, MI 48227","area":"Joy Community, Detroit","lat":42.368091,"lon":-83.203649,"radius_m":50,"fee_minor":800,"bonus_minor":200,"ttl_seconds":172800}

// 3. Vacant residential — VPO2025-01434
{"kind":"observe","predicate":"19348 Montrose St is secured: doors and ground-floor windows are intact and closed.","deliverable":"Photos of the front and the visible side elevation with house number and one-time code in frame.","where":"19348 Montrose St, Detroit, MI 48235","area":"Greenfield, Detroit","lat":42.433268,"lon":-83.201858,"radius_m":50,"fee_minor":800,"bonus_minor":200,"ttl_seconds":172800}

// 4. Charge Ahead site listed "Construction" — is it live yet?
{"kind":"observe","predicate":"A public EV charging station at 18551 Grand River Ave is installed and powered on, with a lit or responsive screen.","deliverable":"Photo of the charger showing its screen state, a photo showing the street address, and the one-time code visible in both.","where":"18551 Grand River Ave, Detroit, MI 48223","area":"Grandmont-Rosedale, Detroit","lat":42.40269,"lon":-83.22434,"radius_m":80,"fee_minor":1800,"bonus_minor":700,"ttl_seconds":259200}

// 5. Second Charge Ahead "Construction" site
{"kind":"observe","predicate":"A public EV charging station at 10185 Gratiot Ave is installed and accepting a session start.","deliverable":"Photo of the charger screen, photo of the address, one-time code in frame.","where":"10185 Gratiot Ave, Detroit, MI 48213","area":"Denby, Detroit","lat":42.39601,"lon":-83.00388,"radius_m":80,"fee_minor":1800,"bonus_minor":700,"ttl_seconds":259200}

// 6. Charge Ahead site listed "Active" — uptime spot check
{"kind":"observe","predicate":"Both charging ports at the Coleman A. Young Community Center are available and free of physical damage or blocked parking.","deliverable":"One photo per port plus a wide shot of the bay, one-time code in every frame.","report":[{"key":"ports_working","label":"Ports responding"},{"key":"bay_blocked","label":"Bay blocked by a non-EV"}],"where":"2751 Robert Bradby Dr, Detroit, MI 48207","area":"Lafayette Park, Detroit","lat":42.34586,"lon":-83.0243,"radius_m":70,"fee_minor":1400,"bonus_minor":400,"ttl_seconds":172800}

// 7. Posted hours vs. OSM record (Tu-Sa 10:00-21:00; Su 11:30-19:00)
{"kind":"observe","predicate":"The hours posted on the door of Supino Pizzeria, 2457 Russell St, match Tuesday-Saturday 10:00-21:00 and Sunday 11:30-19:00.","deliverable":"A legible close photo of the posted hours sign with the one-time code held beside it, plus a shot showing the street number.","where":"2457 Russell St, Detroit, MI 48207","area":"Eastern Market, Detroit","lat":42.34540,"lon":-83.04004,"radius_m":40,"fee_minor":600,"bonus_minor":200,"ttl_seconds":172800}

// 8. Storefront still trading? OSM records opening_hours as "Not open"
{"kind":"observe","predicate":"A business is currently trading from 1301 Broadway St under the name Guns & Butter.","deliverable":"Photo of the frontage showing signage or its absence, plus the street number, with the one-time code in frame.","where":"1301 Broadway St, Detroit, MI 48226","area":"Downtown Detroit","lat":42.33465,"lon":-83.04583,"radius_m":40,"fee_minor":600,"bonus_minor":200,"ttl_seconds":172800}

// 9. Multi-tenant signage — OSM lists four tenants at one address
{"kind":"observe","predicate":"Signage at 4240 Cass Ave names Source Booksellers and Go Sy Thai as current tenants.","deliverable":"Photo of the full tenant signage board and a close photo of each name, one-time code and street number visible.","where":"4240 Cass Ave, Detroit, MI 48201","area":"Midtown Detroit","lat":42.35150,"lon":-83.06390,"radius_m":40,"fee_minor":1000,"bonus_minor":300,"ttl_seconds":172800}

// 10. Street sign at a corner — cheapest possible calibration job
{"kind":"observe","predicate":"A legible street-name blade for Russell St is present and upright at its corner with Adelaide St.","deliverable":"One photo of the sign from below showing both blades, one-time code in frame.","where":"Russell St at Adelaide St, Detroit, MI 48207","area":"Eastern Market, Detroit","lat":42.34650,"lon":-83.03950,"radius_m":45,"fee_minor":400,"bonus_minor":100,"ttl_seconds":86400}
```

Total if all ten post: **$104.00** in fees plus **$33.00** contingent bonuses.

---

## 4. Partnership targets

**Fleets / drones — companies that already take work over an API**

| Target | Docs | What integration takes |
|---|---|---|
| **Uber Direct** | [developer.uber.com/docs/deliveries](https://developer.uber.com/docs/deliveries/overview), [official SDK](https://github.com/uber/uber-direct-sdk) | Map a `do` job to Create Delivery; sandbox account first. Their courier becomes an alternative supply pool for fetch/deliver jobs. |
| **DoorDash Drive** | [developer.doordash.com/en-US/api/drive/](https://developer.doordash.com/en-US/api/drive/) | JWT from a Developer Portal access key; call Create Quote to validate coverage before committing funds — maps cleanly onto our commit-then-post flow. |
| **Zeitview (ex-DroneBase)** | [blog.zeitview.com/2017/03/09/the-dronebase-api](https://blog.zeitview.com/2017/03/09/the-dronebase-api); alternative [Skydio Cloud API](https://cloud.skydio.com/documentation) | Enterprise bulk-order API returning results server-to-server. Right partner for roof and rooftop-array predicates a walker can't satisfy. |

**Field-service dispatch software**

| Target | Docs | What integration takes |
|---|---|---|
| **Jobber** | [developer.getjobber.com/docs/using_jobbers_api/setting_up_webhooks/](https://developer.getjobber.com/docs/using_jobbers_api/setting_up_webhooks/) | GraphQL app in the Developer Center; subscribe to topics, needs matching read scope, **must ack within 1 second** so process asynchronously. Sell as "pre-visit verification". |
| **Housecall Pro** | [docs.housecallpro.com/docs/housecall-public-api/46e9e1be07621-webhooks](https://docs.housecallpro.com/docs/housecall-public-api/46e9e1be07621-webhooks) | Webhooks are **MAX-plan only**; signing secret provided on enable. Narrower installed base, lowest engineering cost. |
| **ServiceTitan** | [developer-next.servicetitan.io/docs/webhooks/](https://developer-next.servicetitan.io/docs/webhooks/) | V2 webhooks with optional HMAC; retries at 10/30/60/300s. **V1 deprecated 2026-03-31** — anyone still on V1 is migrating now, which is the moment to be in the conversation. |

**Agent platforms**

| Target | Docs | What integration takes |
|---|---|---|
| **Anthropic Connectors Directory** | [claude.com/docs/connectors/building/submission](https://claude.com/docs/connectors/building/submission) | Streamable HTTP (SSE rejected), read-only/destructive annotation on every tool, public privacy policy or automatic rejection. Cheapest high-trust distribution available. |
| **OpenAI Apps SDK / ChatGPT directory + ACP** | [developers.openai.com/plugins/deploy/submission](https://developers.openai.com/plugins/deploy/submission), [developers.openai.com/commerce](https://developers.openai.com/commerce), [ACP spec](https://github.com/agentic-commerce-protocol/agentic-commerce-protocol) | Identity verification, domain-control proof, `api.apps.write` permission, MCP server URL supplied directly. ACP's delegate-payment primitive is the natural route to agent-funded jobs. |
| **Zapier MCP Client + n8n** | [docs.zapier.com/mcp/home](https://docs.zapier.com/mcp/home), [help article](https://help.zapier.com/hc/en-us/articles/38777069364109-Connect-remote-MCP-servers-to-Zapier-using-MCP-Client), [n8n MCP Client Tool](https://docs.n8n.io/integrations/builtin/cluster-nodes/sub-nodes/n8n-nodes-langchain.toolmcp) | Both consume a remote MCP endpoint with no per-platform build. Zapier exposes our tools as triggers/actions across 9,000+ apps; n8n needs `N8N_COMMUNITY_PACKAGES_ALLOW_TOOL_USAGE=true`. Near-zero-cost reach. |

*Cursor* is a near-free fourth: streamable-HTTP servers drop into `mcp.json` ([docs](https://docs.cursor.com/context/model-context-protocol)); "Built for Cursor" is the distribution surface.
