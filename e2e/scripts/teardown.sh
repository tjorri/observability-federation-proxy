#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
E2E_DIR="$(dirname "$SCRIPT_DIR")"

CLUSTER_NAME="${CLUSTER_NAME:-ofp-e2e}"

echo "==> Deleting kind cluster: ${CLUSTER_NAME}"
kind delete cluster --name "${CLUSTER_NAME}" || true

rm -f "${E2E_DIR}/kubeconfig"

echo "==> Cleanup complete"
