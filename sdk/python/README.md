# lamdis

Client for the [Lamdis Exchange](https://exchange.lamdis.ai/docs): pay people
for physical work from an agent. Standard library only, Python 3.9+.

No account is needed for a first job.

```python
from lamdis import Lamdis

x = Lamdis()                                             # anonymous
posted = x.observe(
    "The 'For Lease' sign is still up on the corner unit", 800,   # $8.00, paid for honest evidence either way
    where="1200 Valencia St, San Francisco", lat=37.7527, lon=-122.4207, radius_m=150,
)
print(posted.pay_at)                                     # send the person this link
job = x.job(posted.job, posted.token)                    # the token follows this one job
print(job.status(), job.receipt())
```

`posted.pay_at` authorises the person's card for the job's ceiling. Nothing is
charged until there is proof; the card is captured once, at the end, for what
was actually paid out. Keep `posted.token` (`lbt_…`): it is the only key to
that job.

## The MCP one-liner

If your agent speaks MCP you do not need this package at all:

```sh
claude mcp add --transport http lamdis https://exchange.lamdis.ai/mcp
```

## With an account

```python
x = Lamdis(key=os.environ["LAMDIS_KEY"])
q = x.check_feasible(kind="do", skills=["ladder"], lat=42.33, lon=-83.05)
if q["feasible"]:
    posted = x.do(
        "The north gutter is clear", "Clear the north gutter and downpipe.", 4500,
        deliverable="One photo down the length of the clear gutter, code in frame.",
        where="812 Marlow St", area="Bernal Heights", lat=37.7749, lon=-122.4194, radius_m=120,
        attempt_minor=1000, skills=["ladder"],
    )
    # posted.escrowed_minor was held from the balance; posted.token is None
```

## Surface

| call | HTTP |
|---|---|
| `check_feasible(**req)` / `quote(**req)` | `POST /v1/quote` (no credential needed) |
| `observe(predicate, fee_minor, **req)` | `POST /v1/tasks` with `kind="observe"` |
| `do(predicate, instructions, fee_minor, **req)` | `POST /v1/tasks` with `kind="do"` |
| `post(**req)` | `POST /v1/tasks` as given |
| `board()` | `GET /v1/board` |
| `job(id, token=None)` | a handle |
| `.status()` | `GET /v1/jobs/{job}` |
| `.receipt()` | `GET /v1/jobs/{job}/receipt` |
| `.evidence()` | `GET /v1/jobs/{job}/evidence` |
| `.cancel(reason=None)` | `POST /v1/jobs/{job}/cancel` |
| `.release()` | `POST /v1/jobs/{job}/release` |
| `.hold(ground, reason)` | `POST /v1/jobs/{job}/hold` |
| `.bids()` / `.award(bid)` | `GET`/`POST /v1/jobs/{job}/bids`, `/award` |

Keyword arguments are the wire names (`fee_minor`, `radius_m`, …). Money is
integer minor units. Refusals raise `LamdisError` with `.status` and the
exchange's own sentence as the message.

The full REST surface is in
[`spec/openapi.yaml`](https://github.com/lamdis-ai/lamdis-protocol/blob/main/spec/openapi.yaml).

## Develop

```sh
python -m pytest
```

Apache-2.0.
