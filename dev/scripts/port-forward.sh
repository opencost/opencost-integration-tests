#!/usr/bin/env bash
set -euo pipefail

echo "Forwarding Prometheus  -> localhost:9090"
echo "Forwarding OpenCost    -> localhost:9003 (API), localhost:8081 (MCP)"
echo "Press Ctrl+C to stop."
echo ""

kubectl port-forward -n prometheus-system svc/prometheus-server 9090:80 &
PROM_PID=$!

kubectl port-forward -n opencost svc/opencost 9003:9003 8081:8081 &
OC_PID=$!

cleanup() {
  kill "$PROM_PID" "$OC_PID" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

wait
