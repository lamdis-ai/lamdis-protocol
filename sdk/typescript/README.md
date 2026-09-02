# lamdis

Client for the [Lamdis Exchange](https://exchange.lamdis.ai/docs): pay people
for physical work from an agent. Zero dependencies, ESM and CJS, Node 18+.

No account is needed for a first job.

```ts
import { Lamdis } from "lamdis";

const x = new Lamdis();                                  // anonymous
const posted = await x.observe({
  predicate: "The 'For Lease' sign is still up on the corner unit",
  where: "1200 Valencia St, San Francisco", lat: 37.7527, lon: -122.4207, radius_m: 150,
  fee_minor: 800,                                        // $8.00, paid for honest evidence either way
});
console.log(posted.payAt);                               // send the person this link
const job = x.job(posted.job, posted.token);             // the token follows this one job
console.log(await job.status(), await job.receipt());
```

`posted.payAt` authorises the person's card for the job's ceiling. Nothing is
charged until there is proof; the card is captured once, at the end, for what
was actually paid out. Keep `posted.token` (`lbt_…`): it is the only key to
that job.

## The MCP one-liner

If your agent speaks MCP you do not need this package at all:

```sh
claude mcp add --transport http lamdis https://exchange.lamdis.ai/mcp
```

## With an account

An account adds a balance, keys with spending limits, projects, sites and
named suppliers. Issue a key from the console and pass it:

```ts
const x = new Lamdis({ key: process.env.LAMDIS_KEY });
const q = await x.checkFeasible({ kind: "do", skills: ["ladder"], lat: 42.33, lon: -83.05 });
if (q.feasible) {
  const posted = await x.do({
    predicate: "The north gutter is clear",
    instructions: "Clear the north gutter and downpipe.",
    deliverable: "One photo down the length of the clear gutter, code in frame.",
    where: "812 Marlow St", area: "Bernal Heights", lat: 37.7749, lon: -122.4194, radius_m: 120,
    fee_minor: 4500, attempt_minor: 1000, skills: ["ladder"],
  });
  // posted.escrowedMinor was held from the balance; posted.token is undefined
}
```

## Surface

| call | HTTP |
|---|---|
| `checkFeasible(req)` / `quote(req)` | `POST /v1/quote` (no credential needed) |
| `observe(req)` | `POST /v1/tasks` with `kind: "observe"` |
| `do(req)` | `POST /v1/tasks` with `kind: "do"` |
| `post(req)` | `POST /v1/tasks` as given |
| `board()` | `GET /v1/board` |
| `job(id, token?)` | a handle |
| `.status()` | `GET /v1/jobs/{job}` |
| `.receipt()` | `GET /v1/jobs/{job}/receipt` |
| `.evidence()` | `GET /v1/jobs/{job}/evidence` |
| `.cancel(reason?)` | `POST /v1/jobs/{job}/cancel` |
| `.release()` | `POST /v1/jobs/{job}/release` |
| `.hold(ground, reason)` | `POST /v1/jobs/{job}/hold` |
| `.anchor(sha256?)` | `GET /v1/jobs/{job}/receipt/anchor` |
| `.bids()` / `.award(bid)` | `GET`/`POST /v1/jobs/{job}/bids`, `/award` |

Public routes with no SDK method (plain `fetch`): `GET /v1/rails`,
`GET /v1/anchors`, `GET /v1/findings`, `GET /v1/bootstrap`. Operators:
`GET`/`PUT /v1/payout/usdc`. An anonymous post may carry `posted.payUsdc`
when the USDC rail is on.

Field names on requests are the wire names (`fee_minor`, `radius_m`, …).
Money is integer minor units. Refusals throw `LamdisError` with `.status` and
the exchange's own sentence as `.message`.

The full REST surface is in
[`spec/openapi.yaml`](https://github.com/lamdis-ai/lamdis-protocol/blob/main/spec/openapi.yaml).

## Develop

```sh
npm install
npm run build && npm test
```

Apache-2.0.
