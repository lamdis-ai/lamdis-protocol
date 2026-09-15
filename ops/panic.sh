#!/bin/sh
# Stop everything, now.
#
#   ops/panic.sh          stop serving
#   ops/panic.sh --start  put it back
#
# Inference cannot run away on its own: the OpenRouter account is prepaid
# with auto top-up off, so the balance is a hard ceiling and every key on it
# also carries a cap that never refills. This is for the other half, where
# something is being hammered and you want it to stop while you look.
set -eu

PROFILE="${AWS_PROFILE:-aws-admin}"
REGION="${AWS_REGION:-us-east-1}"
SERVICE="arn:aws:ecs:us-east-1:730082756200:service/lamdis/lamdis-app"
FN="lamdis-try"

say() { printf '%s\n' "$*"; }

if [ "${1:-}" = "--start" ]; then
  say "Starting the hosted app…"
  aws ecs update-express-gateway-service --profile "$PROFILE" --region "$REGION" \
    --service-arn "$SERVICE" --scaling-target 'minTaskCount=1,maxTaskCount=2' \
    --query 'service.status.statusCode' --output text
  say "Starting the public demo…"
  aws lambda put-function-concurrency --profile "$PROFILE" --region "$REGION" \
    --function-name "$FN" --reserved-concurrent-executions 5 \
    --query 'ReservedConcurrentExecutions' --output text
  say ""
  say "Both are back. Give the app a few minutes to answer."
  exit 0
fi

say "Stopping the public demo (takes effect immediately)…"
aws lambda put-function-concurrency --profile "$PROFILE" --region "$REGION" \
  --function-name "$FN" --reserved-concurrent-executions 0 \
  --query 'ReservedConcurrentExecutions' --output text

say "Stopping the hosted app…"
aws ecs update-express-gateway-service --profile "$PROFILE" --region "$REGION" \
  --service-arn "$SERVICE" --scaling-target 'minTaskCount=0,maxTaskCount=0' \
  --query 'service.status.statusCode' --output text

say ""
say "Serving has stopped. Nothing was deleted and no data was touched:"
say "  accounts   EFS fs-05981263e88aee227, access point fsap-08d03dabbd7fd5c43"
say "  back up    ops/panic.sh --start"
say ""
say "If the worry is spend rather than load, the money stops at openrouter.ai/settings/keys:"
say "  lamdis hosted app   the key every hosted account shares"
say "  lamdis.ai demo box  the key behind the try box on the site"
say "Deleting either one stops that half of the spending in the same second."
