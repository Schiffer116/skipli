#!/usr/bin/env bash
# Deploy a single CloudFormation stack from infra/<name>.yaml.
#
#   infra/deploy.sh ui
#
set -euo pipefail

NAME="${1:?usage: infra/deploy.sh <template-name>}"
AWS_REGION="${AWS_REGION:-ap-southeast-7}"

cfn-lint infra/"$NAME".yaml

aws cloudformation deploy \
  --region 'us-east-1' \
  --stack-name "skipli-$NAME" \
  --template-file "infra/$NAME.yaml" \
  --parameter-overrides file://infra/"$NAME"_parameters.json \
  --capabilities CAPABILITY_IAM CAPABILITY_AUTO_EXPAND \
  --no-fail-on-empty-changeset \
  --disable-rollback
