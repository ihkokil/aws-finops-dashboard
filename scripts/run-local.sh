#!/usr/bin/env bash
set -euo pipefail

# Runs FinOps collector locally using AWS SSO session
PROFILE="${1:-default}"
REGION="${2:-us-east-1}"

echo "=== Running AWS FinOps Collector Locally ==="
echo "AWS Profile: ${PROFILE}"
echo "AWS Region:  ${REGION}"

# Login via SSO if configured
if aws sts get-caller-identity --profile "${PROFILE}" >/dev/null 2>&1; then
    echo "[INFO] Valid AWS session detected."
else
    echo "[INFO] Session expired or invalid. Triggering aws sso login..."
    aws sso login --profile "${PROFILE}"
fi

export AWS_PROFILE="${PROFILE}"
export AWS_REGION="${REGION}"

cd "$(dirname "$0")/../collector"

echo "[INFO] Building latest collector binary..."
go build -o collector ./cmd/collector

echo "[INFO] Executing collector scan..."
./collector --output-format all --output-dir ../reports/ --days 14

echo "[SUCCESS] Scan complete! Check reports/ directory."
