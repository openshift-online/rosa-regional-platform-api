#!/usr/bin/env bash
set -euo pipefail

REGION=${REGION:-us-east-2}

API_ID=$(aws apigateway get-rest-apis --region $REGION --query "items[?starts_with(name, 'regional-cluster-')].id | [0]" --output text)
STAGES=$(aws apigateway get-stages --rest-api-id $API_ID --region $REGION \
  --query "item[].{ URL:join('', ['https://', '$API_ID', '.execute-api.$REGION.amazonaws.com/', stageName])}" \
  --output text)
echo $STAGES
