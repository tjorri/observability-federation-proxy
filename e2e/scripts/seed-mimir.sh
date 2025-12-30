#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
E2E_DIR="$(dirname "$SCRIPT_DIR")"
PROJECT_ROOT="$(dirname "$E2E_DIR")"

export KUBECONFIG="${E2E_DIR}/kubeconfig"

echo "==> Starting port-forward to Mimir"
kubectl port-forward svc/mimir -n observability 8080:8080 &
PF_PID=$!
sleep 3

cleanup() {
    echo "==> Stopping port-forward"
    kill $PF_PID 2>/dev/null || true
}
trap cleanup EXIT

MIMIR_URL="http://localhost:8080"

# Push metrics using the Go seeder tool
echo "==> Running Mimir seeder"
cd "${PROJECT_ROOT}"
go run ./e2e/cmd/seed-mimir/main.go \
    --url "${MIMIR_URL}" \
    --tenants "tenant-alpha,tenant-beta,tenant-gamma"

echo "==> Mimir seeding complete"
