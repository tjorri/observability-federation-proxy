#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
E2E_DIR="$(dirname "$SCRIPT_DIR")"
PROJECT_ROOT="$(dirname "$E2E_DIR")"

CLUSTER_NAME="${CLUSTER_NAME:-ofp-e2e}"
KUBECONFIG_PATH="${E2E_DIR}/kubeconfig"

echo "==> Creating kind cluster: ${CLUSTER_NAME}"
kind create cluster --name "${CLUSTER_NAME}" --config "${SCRIPT_DIR}/kind-config.yaml" --kubeconfig "${KUBECONFIG_PATH}"

export KUBECONFIG="${KUBECONFIG_PATH}"

echo "==> Creating observability namespace"
kubectl apply -f "${E2E_DIR}/manifests/namespace.yaml"

echo "==> Deploying Loki"
kubectl apply -f "${E2E_DIR}/manifests/loki/"

echo "==> Deploying Mimir"
kubectl apply -f "${E2E_DIR}/manifests/mimir/"

echo "==> Creating tenant namespaces"
kubectl apply -f "${E2E_DIR}/manifests/tenants/"

echo "==> Waiting for Loki to be ready"
kubectl wait --for=condition=available --timeout=180s deployment/loki -n observability

echo "==> Waiting for Mimir to be ready"
kubectl wait --for=condition=available --timeout=180s deployment/mimir -n observability

echo "==> Seeding test data"
"${SCRIPT_DIR}/seed-loki.sh"
"${SCRIPT_DIR}/seed-mimir.sh"

echo "==> E2E infrastructure ready"
echo "    Kubeconfig: ${KUBECONFIG_PATH}"
echo "    Cluster: ${CLUSTER_NAME}"
