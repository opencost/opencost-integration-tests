#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT_DIR="${1:-$REPO_ROOT/dev/logs/$(date +%Y%m%d-%H%M%S)}"

mkdir -p "$OUT_DIR"

echo "Collecting logs to $OUT_DIR"

kubectl get pods -A -o wide >"$OUT_DIR/pods.txt" 2>&1 || true

for ns in opencost prometheus-system workload-test kube-system; do
  kubectl get all -n "$ns" >"$OUT_DIR/${ns}-resources.txt" 2>&1 || true
done

kubectl logs -n opencost -l app.kubernetes.io/instance=opencost --all-containers --tail=-1 \
  >"$OUT_DIR/opencost.log" 2>&1 || true

kubectl logs -n prometheus-system -l app.kubernetes.io/name=prometheus --all-containers --tail=-1 \
  >"$OUT_DIR/prometheus-server.log" 2>&1 || true

kubectl logs -n prometheus-system -l app.kubernetes.io/name=kube-state-metrics --all-containers --tail=-1 \
  >"$OUT_DIR/kube-state-metrics.log" 2>&1 || true

echo "Done. Logs written to $OUT_DIR"
