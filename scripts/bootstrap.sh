#!/usr/bin/env bash
set -euo pipefail

# Provisions S3 Bucket and DynamoDB Lock Table for Terraform Remote Backend
REGION="${1:-us-east-1}"
ENV="${2:-dev}"
ACCOUNT_ID=$(aws sts get-caller-identity --query "Account" --output text)

BUCKET_NAME="aws-finops-tfstate-${ENV}-${ACCOUNT_ID}"
TABLE_NAME="aws-finops-tfstate-locks"

echo "=== AWS FinOps Terraform Backend Bootstrap ==="
echo "Account ID: ${ACCOUNT_ID}"
echo "Region:     ${REGION}"
echo "Env:        ${ENV}"
echo "Bucket:     ${BUCKET_NAME}"
echo "Table:      ${TABLE_NAME}"
echo "============================================="

# 1. Create S3 Bucket
if aws s3api head-bucket --bucket "${BUCKET_NAME}" 2>/dev/null; then
    echo "[INFO] S3 bucket ${BUCKET_NAME} already exists."
else
    echo "[INFO] Creating S3 state bucket..."
    if [ "${REGION}" == "us-east-1" ]; then
        aws s3api create-bucket --bucket "${BUCKET_NAME}" --region "${REGION}"
    else
        aws s3api create-bucket --bucket "${BUCKET_NAME}" --region "${REGION}" \
            --create-bucket-configuration LocationConstraint="${REGION}"
    fi
    aws s3api put-bucket-versioning --bucket "${BUCKET_NAME}" \
        --versioning-configuration Status=Enabled
    aws s3api put-bucket-encryption --bucket "${BUCKET_NAME}" \
        --server-side-encryption-configuration '{"Rules":[{"ApplyServerSideEncryptionByDefault":{"SSEAlgorithm":"AES256"}}]}'
    echo "[SUCCESS] S3 Bucket created."
fi

# 2. Create DynamoDB Table for State Locking
if aws dynamodb describe-table --table-name "${TABLE_NAME}" --region "${REGION}" 2>/dev/null; then
    echo "[INFO] DynamoDB table ${TABLE_NAME} already exists."
else
    echo "[INFO] Creating DynamoDB state lock table..."
    aws dynamodb create-table \
        --table-name "${TABLE_NAME}" \
        --attribute-definitions AttributeName=LockID,AttributeType=S \
        --key-schema AttributeName=LockID,KeyType=HASH \
        --billing-mode PAY_PER_REQUEST \
        --region "${REGION}"
    echo "[SUCCESS] DynamoDB table created."
fi

echo "=== Bootstrap Complete ==="
