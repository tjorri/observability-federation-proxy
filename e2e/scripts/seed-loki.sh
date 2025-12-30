#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
E2E_DIR="$(dirname "$SCRIPT_DIR")"

export KUBECONFIG="${E2E_DIR}/kubeconfig"

echo "==> Starting port-forward to Loki"
kubectl port-forward svc/loki -n observability 3100:3100 &
PF_PID=$!
sleep 3

cleanup() {
    echo "==> Stopping port-forward"
    kill $PF_PID 2>/dev/null || true
}
trap cleanup EXIT

LOKI_URL="http://localhost:3100"
NOW_NS=$(date +%s)000000000

# Push logs for each tenant
for tenant in tenant-alpha tenant-beta tenant-gamma; do
    echo "==> Pushing logs for tenant: ${tenant}"

    # Push sample log entries
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${LOKI_URL}/loki/api/v1/push" \
        -H "Content-Type: application/json" \
        -H "X-Scope-OrgID: ${tenant}" \
        -d "{
            \"streams\": [{
                \"stream\": {
                    \"job\": \"e2e-test\",
                    \"namespace\": \"${tenant}\",
                    \"app\": \"test-app\",
                    \"level\": \"info\"
                },
                \"values\": [
                    [\"${NOW_NS}\", \"E2E test log message for tenant ${tenant}\"],
                    [\"$((NOW_NS + 1000000))\", \"INFO: Application started successfully in ${tenant}\"],
                    [\"$((NOW_NS + 2000000))\", \"DEBUG: Processing request in ${tenant}\"]
                ]
            }]
        }")

    if [ "$HTTP_CODE" -eq 204 ]; then
        echo "    Successfully pushed logs for ${tenant}"
    else
        echo "    Warning: Got HTTP ${HTTP_CODE} when pushing logs for ${tenant}"
    fi
done

echo "==> Loki seeding complete"
