# What this can cost, and how to stop it

## Inference cannot run away

The OpenRouter account is **prepaid with auto top-up off**, so the account
balance is the ceiling on every model call made by anything here. There is
no card to fall back on and no refill.

Underneath that, each key carries its own cap that never resets, so one
surface cannot eat another's room:

| Key | Cap | Spends it |
|---|---|---|
| `lamdis hosted app` | $10 | every account on app.lamdis.ai that has not brought its own key |
| `lamdis.ai demo box` | $5 | the try box on the landing page |
| `Default key` | none | your own machine only |

The caps add up to more than the balance on purpose: the balance is the
real stop, and the caps decide which surface runs out first.

**When the balance hits zero the agent stops answering** on both surfaces
and says so. Nothing breaks and nothing is lost; threads, connections and
settings are all still there. Adding credit starts it again.

### What bounds it below the ceiling

- Somebody spending a key they did not supply gets a short menu of cheap
  models, 25 runs, 60,000 tokens and 20 page fetches a day.
- The try box allows 8 questions per visitor per day, 3 a minute, and 400 a
  day in total, at roughly $0.0003 a question.
- Bringing your own OpenRouter key lifts the model restriction, and then it
  is your balance, not this one.

## AWS is bounded by shape, not by a hard cap

Nothing here scales with traffic in a way that can surprise you.

| Thing | Bound |
|---|---|
| The hosted app | one ECS task, never more than two |
| The try box | a Lambda with reserved concurrency of 5 |
| Storage | EFS holding kilobytes per account |
| CloudFront | pay per request; the responses are small JSON |

Existing budgets already alert both addresses: `aws-daily-tripwire` at $30 a
day, `aws-monthly-guardrail` at $400 a month with a forecast warning, and
`monthly-total` at $500.

Added for this: an SNS topic `lamdis-alerts` with three CloudWatch alarms,
for the demo being hit unusually hard, the demo failing, and the app having
no running task. **Both email addresses have to confirm the subscription
before any of them can reach you.**

## Stopping it

```sh
ops/panic.sh          # stop serving, keep everything
ops/panic.sh --start  # put it back
```

That takes the demo offline in the same second and drains the app. No data
is touched.

To stop the *money* rather than the load, delete a key at
openrouter.ai/settings/keys. That is instant and affects nothing else.
