#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME=opencost-dev
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

helm repo add prometheus-community https://prometheus-community.github.io/helm-charts 2>/dev/null || true
helm repo add opencost https://opencost.github.io/opencost-helm-chart 2>/dev/null || true
helm repo update

if ! k3d cluster list | grep -q "$CLUSTER_NAME"; then
  k3d cluster create "$CLUSTER_NAME" --agents 2
fi

kubectl create namespace prometheus-system --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace opencost --dry-run=client -o yaml | kubectl apply -f -

helm upgrade --install prometheus prometheus-community/prometheus \
  -n prometheus-system \
  -f "$REPO_ROOT/dev/helm/prometheus-values.yaml"

helm upgrade --install opencost opencost/opencost \
  -n opencost \
  -f "$REPO_ROOT/dev/helm/opencost-values.yaml"

echo "Waiting for core deployments..."
kubectl wait --for=condition=available deployment/prometheus-server \
  -n prometheus-system --timeout=300s
kubectl wait --for=condition=available deployment/opencost \
  -n opencost --timeout=300s

"$REPO_ROOT/dev/scripts/deploy-workloads.sh"

echo ""
echo "Stack is up. Check pods: kubectl get pods -A"
echo ""
echo "Port-forward (in a separate terminal):"
echo "  $REPO_ROOT/dev/scripts/port-forward.sh"
echo ""
echo "Export test env vars:"
echo "  export OPENCOST_URL='http://localhost:9003'"
echo "  export PROMETHEUS_URL='http://localhost:9090'"
echo "  export OPENCOST_MCP_URL='http://localhost:8081'"
echo ""
echo "Run tests:"
echo "  ./test/bats/bin/bats ./test/integration/query/count/test.bats"
