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
# Compiled here, where the module proxy is reachable, and shipped as the only
# new layer: a slow uplink then has one small thing to push, and pushes retry.
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o lamdis-linux-arm64 ./cmd/lamdis
trap 'rm -f "$(pwd)/lamdis-linux-arm64"' EXIT
docker buildx build --platform linux/arm64 -f Dockerfile.prebuilt -t "$REPO:${TAG}" --load . >/dev/null
i=0
until docker push -q "$REPO:${TAG}" >/dev/null; do
  i=$((i+1)); [ $i -ge 5 ] && { say "push kept failing; stopping before touching the service"; exit 1; }
  say "  push failed; retrying ($i)"; sleep 15
  aws ecr get-login-password --profile "$PROFILE" --region "$REGION" | docker login -u AWS --password-stdin "${REPO%/*}" >/dev/null
done

# Since 2026-10-10 app.lamdis.ai runs on the shared prototype box (EC2 shared-box,
# 54.85.29.91, docker compose in /opt/box; the EFS is mounted at /mnt/lamdis), behind
# CloudFront E1RHFILNB4M2UF -> app-origin.lamdis.ai. The ECS Express service and its ALB
# were deleted. `docker compose up` recreates the container, stopping the old one
# before the new one starts, so there is still exactly one writer on each account file.
BOX="${BOX:-ec2-user@54.85.29.91}"
say "Starting ${TAG} on the shared box…"
ssh -i "$HOME/.ssh/shared-box.pem" "$BOX" "set -e; cd /opt/box
  aws ecr get-login-password --region $REGION | sudo docker login -u AWS --password-stdin ${REPO%/*} >/dev/null
  sudo sed -i '/^LAMDIS_TAG=/d' .env 2>/dev/null || true
  echo LAMDIS_TAG=${TAG} | sudo tee -a .env >/dev/null
  sudo /opt/box/fetch-env.sh >/dev/null  # env (incl. secrets) from Secrets Manager shared-box/env
  sudo docker compose --profile lamdis pull -q lamdis-app
  sudo docker compose --profile lamdis up -d lamdis-app"
i=0
until [ "$(curl -s -o /dev/null -w '%{http_code}' https://app.lamdis.ai/healthz)" = "200" ]; do
  i=$((i+1)); [ $i -gt 30 ] && { say "not healthy after 5 minutes: ssh -i ~/.ssh/shared-box.pem $BOX 'cd /opt/box && sudo docker compose --profile lamdis logs --tail 50 lamdis-app'"; exit 1; }
  sleep 10
done
say "Live: ${TAG}"
