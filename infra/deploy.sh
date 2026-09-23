#!/usr/bin/env bash
# Build and deploy Skipli with CloudFormation.
#
#   APP_EMAIL=... APP_EMAIL_PASSWORD=... JWT_SECRET=... ./infra/deploy.sh
#
# (app/.envrc already exports these for direnv users.) Optional:
#   STACK_NAME    default: skipli
#   AWS_REGION    default: ap-southeast-7, the region the API's DynamoDB
#                 client is hard-wired to
#   CREATE_TABLE  "true" to create the Skipli table, default "false" to reuse
#                 an existing one
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
ROOT="$PWD"

STACK_NAME="${STACK_NAME:-skipli}"
AWS_REGION="${AWS_REGION:-ap-southeast-7}"
CREATE_TABLE="${CREATE_TABLE:-false}"
export AWS_REGION

: "${APP_EMAIL:?APP_EMAIL must be set}"
: "${APP_EMAIL_PASSWORD:?APP_EMAIL_PASSWORD must be set}"
: "${JWT_SECRET:?JWT_SECRET must be set}"

for cmd in aws go pnpm zip; do
  command -v "$cmd" >/dev/null || { echo "missing required command: $cmd" >&2; exit 1; }
done

BUILD_DIR="$(mktemp -d)"
trap 'rm -rf "$BUILD_DIR"' EXIT

stack_output() {
  aws cloudformation describe-stacks --stack-name "$1" \
    --query "Stacks[0].Outputs[?OutputKey=='$2'].OutputValue" --output text
}

echo "==> Artifacts bucket"
aws cloudformation deploy \
  --stack-name "$STACK_NAME-artifacts" \
  --template-file infra/artifacts.yaml \
  --no-fail-on-empty-changeset
ARTIFACT_BUCKET="$(stack_output "$STACK_NAME-artifacts" ArtifactBucketName)"

echo "==> Building API"
(
  cd app
  GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
    go build -tags lambda.norpc -trimpath -ldflags="-s -w" \
    -o "$BUILD_DIR/bootstrap" ./cmd/skipli
)
(cd "$BUILD_DIR" && zip -q -X api.zip bootstrap)

# Content-addressed key: a new build means a new key, which is what makes
# CloudFormation actually update the function's code.
CODE_KEY="api/$(sha256sum "$BUILD_DIR/api.zip" | cut -c1-16).zip"
aws s3 cp --only-show-errors "$BUILD_DIR/api.zip" "s3://$ARTIFACT_BUCKET/$CODE_KEY"

echo "==> Deploying stack $STACK_NAME ($AWS_REGION)"
aws cloudformation deploy \
  --stack-name "$STACK_NAME" \
  --template-file infra/template.yaml \
  --capabilities CAPABILITY_IAM \
  --no-fail-on-empty-changeset \
  --parameter-overrides \
    "CodeBucket=$ARTIFACT_BUCKET" \
    "CodeKey=$CODE_KEY" \
    "CreateTable=$CREATE_TABLE" \
    "AppEmail=$APP_EMAIL" \
    "AppEmailPassword=$APP_EMAIL_PASSWORD" \
    "JwtSecret=$JWT_SECRET"

SITE_BUCKET="$(stack_output "$STACK_NAME" SiteBucketName)"
DISTRIBUTION_ID="$(stack_output "$STACK_NAME" DistributionId)"
SITE_URL="$(stack_output "$STACK_NAME" SiteUrl)"

echo "==> Building frontend"
(cd ui && pnpm install --frozen-lockfile && pnpm build)

echo "==> Uploading frontend"
# Hashed assets can be cached forever; everything else (index.html, the
# favicon) must be revalidated so a deploy shows up straight away.
aws s3 sync --only-show-errors --delete ui/dist/assets "s3://$SITE_BUCKET/assets" \
  --cache-control "public, max-age=31536000, immutable"
aws s3 sync --only-show-errors --delete ui/dist "s3://$SITE_BUCKET" \
  --exclude "assets/*" \
  --cache-control "no-cache"

aws cloudfront create-invalidation \
  --distribution-id "$DISTRIBUTION_ID" \
  --paths "/index.html" "/" >/dev/null

echo "==> Done: $SITE_URL"
