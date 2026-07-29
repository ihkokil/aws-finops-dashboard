#!/usr/bin/env bash
set -euo pipefail

# Provisions Grafana Dashboards and Prometheus Datasource via Grafana HTTP REST API
GRAFANA_URL="${1:-http://localhost:3000}"
ADMIN_USER="${2:-admin}"
ADMIN_PASS="${3:-admin}"

echo "=== Provisioning Grafana Dashboards via REST API ==="
echo "Target URL: ${GRAFANA_URL}"

# Wait for Grafana healthz
until curl -s "${GRAFANA_URL}/api/health" | grep "ok" >/dev/null; do
    echo "[INFO] Waiting for Grafana service at ${GRAFANA_URL}..."
    sleep 2
done

echo "[INFO] Grafana API ready. Provisioning dashboards..."

DASHBOARD_DIR="$(dirname "$0")/../dashboard/grafana/dashboards"

for dash_file in "${DASHBOARD_DIR}"/*.json; do
    echo "[INFO] Uploading $(basename "${dash_file}")..."
    PAYLOAD=$(jq -n --argfile doc "${dash_file}" '{"dashboard": $doc, "overwrite": true}')
    
    curl -s -X POST "${GRAFANA_URL}/api/dashboards/db" \
        -u "${ADMIN_USER}:${ADMIN_PASS}" \
        -H "Content-Type: application/json" \
        -d "${PAYLOAD}" > /dev/null
done

echo "=== Grafana Dashboards Provisioned Successfully! ==="
