#!/usr/bin/env bash
set -euo pipefail

: "${OPENCOST_BROKER_URL:?OPENCOST_BROKER_URL is required}"
: "${OPENCOST_BROKER_TOKEN:?OPENCOST_BROKER_TOKEN is required}"

broker_url="${OPENCOST_BROKER_URL%/}"

echo "Checking broker liveness..."
curl -fsS "${broker_url}/healthz"
echo

echo "Checking broker pod list..."
curl -fsS \
  -H "Authorization: Bearer ${OPENCOST_BROKER_TOKEN}" \
  -H "Accept: application/json" \
  "${broker_url}/v1/pods"
echo

echo "Checking broker chaos scenario discovery..."
curl -fsS \
  -H "Authorization: Bearer ${OPENCOST_BROKER_TOKEN}" \
  -H "Accept: application/json" \
  "${broker_url}/v1/chaos"
echo

echo "Checking broker node facts..."
curl -fsS \
  -H "Authorization: Bearer ${OPENCOST_BROKER_TOKEN}" \
  -H "Accept: application/json" \
  "${broker_url}/v1/nodes"
echo

echo "Checking broker disk facts..."
curl -fsS \
  -H "Authorization: Bearer ${OPENCOST_BROKER_TOKEN}" \
  -H "Accept: application/json" \
  "${broker_url}/v1/disks"
echo

opencost_namespace="${OPENCOST_NAMESPACE:-opencost}"
opencost_deployment="${OPENCOST_DEPLOYMENT:-opencost}"
opencost_selector="${OPENCOST_SELECTOR:-app.kubernetes.io/name=opencost}"

echo "Checking broker deployment readiness..."
curl -fsS \
  -H "Authorization: Bearer ${OPENCOST_BROKER_TOKEN}" \
  -H "Accept: application/json" \
  "${broker_url}/v1/deployments/${opencost_deployment}?namespace=${opencost_namespace}"
echo

echo "Checking broker log read..."
curl -fsS \
  -H "Authorization: Bearer ${OPENCOST_BROKER_TOKEN}" \
  -H "Accept: application/json" \
  --get \
  --data-urlencode "namespace=${opencost_namespace}" \
  --data-urlencode "selector=${opencost_selector}" \
  --data-urlencode "tailLines=20" \
  "${broker_url}/v1/logs"
echo

echo "ops-broker smoke checks passed"
