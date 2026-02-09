#!/bin/bash
# Initialize DynamoDB Local with authz tables for E2E testing

set -e

ENDPOINT="${DYNAMODB_ENDPOINT:-http://localhost:8180}"
REGION="${AWS_REGION:-us-east-1}"

# DynamoDB Local requires credentials to be set (even fake ones)
export AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-dummy}"
export AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-dummy}"

export AWS_PAGER=""

echo "Initializing DynamoDB tables at $ENDPOINT..."

# Wait for DynamoDB to be ready
echo "Waiting for DynamoDB Local to be ready..."
for i in {1..30}; do
    if aws dynamodb list-tables --endpoint-url "$ENDPOINT" --region "$REGION" >/dev/null 2>&1; then
        echo "DynamoDB Local is ready!"
        break
    fi
    if [ $i -eq 30 ]; then
        echo "Timeout waiting for DynamoDB Local"
        exit 1
    fi
    sleep 1
done

# Function to create table if it doesn't exist
create_table() {
    local table_name=$1
    shift

    # Check if table exists
    if aws dynamodb describe-table --endpoint-url "$ENDPOINT" --region "$REGION" --table-name "$table_name" >/dev/null 2>&1; then
        echo "Table $table_name already exists, skipping..."
        return 0
    fi

    echo "Creating table $table_name..."
    aws dynamodb create-table --endpoint-url "$ENDPOINT" --region "$REGION" --table-name "$table_name" "$@" --billing-mode PAY_PER_REQUEST
}

# 1. Accounts table (PK: accountId)
create_table "rosa-authz-accounts" \
    --attribute-definitions AttributeName=accountId,AttributeType=S \
    --key-schema AttributeName=accountId,KeyType=HASH

# 2. Admins table (PK: accountId, SK: principalArn)
create_table "rosa-authz-admins" \
    --attribute-definitions \
        AttributeName=accountId,AttributeType=S \
        AttributeName=principalArn,AttributeType=S \
    --key-schema \
        AttributeName=accountId,KeyType=HASH \
        AttributeName=principalArn,KeyType=RANGE

# 3. Groups table (PK: accountId, SK: groupId)
create_table "rosa-authz-groups" \
    --attribute-definitions \
        AttributeName=accountId,AttributeType=S \
        AttributeName=groupId,AttributeType=S \
    --key-schema \
        AttributeName=accountId,KeyType=HASH \
        AttributeName=groupId,KeyType=RANGE

# 4. Members table (PK: accountId, SK: groupId#memberArn, GSI: member-groups-index)
create_table "rosa-authz-group-members" \
    --attribute-definitions \
        AttributeName=accountId,AttributeType=S \
        'AttributeName=groupId#memberArn,AttributeType=S' \
        'AttributeName=accountId#memberArn,AttributeType=S' \
        AttributeName=groupId,AttributeType=S \
    --key-schema \
        AttributeName=accountId,KeyType=HASH \
        'AttributeName=groupId#memberArn,KeyType=RANGE' \
    --global-secondary-indexes \
        '[{
            "IndexName": "member-groups-index",
            "KeySchema": [
                {"AttributeName": "accountId#memberArn", "KeyType": "HASH"},
                {"AttributeName": "groupId", "KeyType": "RANGE"}
            ],
            "Projection": {"ProjectionType": "ALL"}
        }]'

# Seed privileged account for e2e testing
echo "Seeding privileged account for e2e tests..."
if aws dynamodb get-item --endpoint-url "$ENDPOINT" --region "$REGION" \
    --table-name "rosa-authz-accounts" \
    --key '{"accountId": {"S": "000000000000"}}' \
    --projection-expression "accountId" | grep -q "000000000000"; then
    echo "Privileged account already exists, skipping..."
else
    aws dynamodb put-item --endpoint-url "$ENDPOINT" --region "$REGION" \
        --table-name "rosa-authz-accounts" \
        --item '{
            "accountId": {"S": "000000000000"},
            "privileged": {"BOOL": true},
            "createdAt": {"S": "2024-01-01T00:00:00Z"}
        }'
    echo "Privileged account created."
fi

echo ""
echo "All DynamoDB tables created successfully!"

# List tables to verify
echo ""
echo "Tables in DynamoDB Local:"
aws dynamodb list-tables --endpoint-url "$ENDPOINT" --region "$REGION"
