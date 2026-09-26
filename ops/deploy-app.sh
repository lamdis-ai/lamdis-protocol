#!/bin/sh
# Deploy app.lamdis.ai: build, push, and roll the hosted app with no overlap.
#
#   ops/deploy-app.sh v36
#
# Every hosted account is a SQLite database on EFS. Two servers must never
# have the same account open at once, so this stops the old server before
# the new one starts, instead of letting ECS run them side by side. That
# costs a minute or two of downtime per deploy, which is the price of never
# having two writers on one file.
set -eu
TAG="${1:?usage: ops/deploy-app.sh vNN}"
PROFILE="${AWS_PROFILE:-aws-admin}"
REGION="us-east-1"
REPO="730082756200.dkr.ecr.us-east-1.amazonaws.com/lamdis-app"
SERVICE="arn:aws:ecs:us-east-1:730082756200:service/lamdis/lamdis-app"
TMP="$(mktemp -d)"
cd "$(dirname "$0")/../node"

say() { printf '%s\n' "$*"; }

say "Building ${TAG} for arm64…"
aws ecr get-login-password --profile "$PROFILE" --region "$REGION" | docker login -u AWS --password-stdin "${REPO%/*}" >/dev/null
docker buildx build --no-cache --platform linux/arm64 -t "$REPO:${TAG}" --push . >/dev/null

say "Registering a task definition with ${TAG}…"
aws ecs describe-task-definition --task-definition lamdis-app --profile "$PROFILE" --region "$REGION" \
  --query taskDefinition > "$TMP/cur.json"
python3 - "$TMP" "${TAG}" <<'EOF'
import json, sys
tmp, tag = sys.argv[1], sys.argv[2]
d = json.load(open(tmp + "/cur.json"))
for k in ["taskDefinitionArn", "revision", "status", "requiresAttributes", "compatibilities",
          "registeredAt", "registeredBy", "deregisteredAt"]:
    d.pop(k, None)
for c in d["containerDefinitions"]:
    if "lamdis-app" in c["image"]:
        c["image"] = c["image"].rsplit(":", 1)[0] + ":" + tag
json.dump(d, open(tmp + "/new.json", "w"))
EOF
NEW="$(aws ecs register-task-definition --cli-input-json "file://$TMP/new.json" --profile "$PROFILE" --region "$REGION" \
  --query taskDefinition.taskDefinitionArn --output text)"
say "  $NEW"

say "Stopping the running server…"
aws ecs update-express-gateway-service --profile "$PROFILE" --region "$REGION" --service-arn "$SERVICE" \
  --scaling-target 'minTaskCount=0,maxTaskCount=0' --query 'service.status.statusCode' --output text >/dev/null
i=0
while [ "$(aws ecs list-tasks --cluster lamdis --desired-status RUNNING --profile "$PROFILE" --region "$REGION" --query 'length(taskArns)' --output text)" != "0" ]; do
  i=$((i+1)); [ $i -gt 60 ] && { say "still running after 10 minutes; stopping here"; exit 1; }
  sleep 10
done
# EFS releases a stopped client's file locks after a short lease.
sleep 30

say "Starting ${TAG} alone…"
aws ecs update-express-gateway-service --service-arn "$SERVICE" --task-definition-arn "$NEW" \
  --profile "$PROFILE" --region "$REGION" --query 'service.serviceArn' --output text >/dev/null
# Switching the task definition is itself a deployment, and ECS Express keeps
# the previous version running until a new one has baked. Let that (empty)
# deployment finish at zero tasks before asking for one, or the old version
# comes back up alongside the new.
i=0
until [ "$(aws ecs describe-services --cluster lamdis --services lamdis-app --profile "$PROFILE" --region "$REGION" \
      --query 'length(services[0].deployments[?rolloutState==`IN_PROGRESS`])' --output text)" = "0" ]; do
  i=$((i+1)); [ $i -gt 90 ] && { say "the empty deployment did not settle; stopping here"; exit 1; }
  sleep 10
done
aws ecs update-express-gateway-service --profile "$PROFILE" --region "$REGION" --service-arn "$SERVICE" \
  --scaling-target 'minTaskCount=1,maxTaskCount=1' --query 'service.status.statusCode' --output text >/dev/null
i=0
until [ "$(curl -s -o /dev/null -w '%{http_code}' https://app.lamdis.ai/healthz)" = "200" ]; do
  i=$((i+1)); [ $i -gt 90 ] && { say "not healthy after 15 minutes"; exit 1; }
  sleep 10
done
# Healthy is not enough: make sure only one server is left.
i=0
until [ "$(aws ecs list-tasks --cluster lamdis --desired-status RUNNING --profile "$PROFILE" --region "$REGION" --query 'length(taskArns)' --output text)" = "1" ]; do
  i=$((i+1)); [ $i -gt 60 ] && { say "more than one server is still running; look before deploying again"; exit 1; }
  sleep 10
done
say "Live: ${TAG}"
