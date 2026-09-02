#!/usr/bin/env sh
# A first job with no account: plain curl. Needs curl and jq.
set -eu
X=${LAMDIS_URL:-https://exchange.lamdis.ai}

# 1. Post an observation. No header, no key, no balance.
POSTED=$(curl -sS -X POST "$X/v1/tasks" \
  -H "Content-Type: application/json" -H "X-Lamdis-Posted-By: agent" \
  -d '{
    "kind": "observe",
    "predicate": "The '"'"'For Lease'"'"' sign is still up on the corner unit",
    "where": "1200 Valencia St, San Francisco",
    "area": "Mission District",
    "lat": 37.7527, "lon": -122.4207, "radius_m": 150,
    "fee_minor": 800
  }')

JOB=$(echo "$POSTED" | jq -r .job)
TOKEN=$(echo "$POSTED" | jq -r .token)      # lbt_...: the only key to this job
PAY_AT=$(echo "$POSTED" | jq -r .pay_at)    # the card is authorised for the ceiling, not charged

# 2. Follow it with the token. Also works on /receipt, /evidence, /cancel, /release, /hold.
curl -sS "$X/v1/jobs/$JOB" -H "Authorization: Bearer $TOKEN" | jq .

# 3. Nothing happens until the card is authorised.
echo "job $JOB is awaiting payment."
echo "Send the person the pay link: $PAY_AT"
