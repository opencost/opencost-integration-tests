#!/usr/bin/env bash
set -euo pipefail

NAMESPACE=prometheus-system
DEPLOYMENT=prometheus-server

usage() {
  echo "Usage: $0 [on|off]"
  echo "  on  - scale Prometheus server to 1 replica"
  echo "  off - scale Prometheus server to 0 replicas"
  exit 1
}

STATE="${1:-}"
case "$STATE" in
  on)
    REPLICAS=1
    ;;
  off)
    REPLICAS=0
    ;;
  *)
    usage
    ;;
esac

kubectl scale deployment "$DEPLOYMENT" -n "$NAMESPACE" --replicas="$REPLICAS"
echo "Prometheus $DEPLOYMENT scaled to $REPLICAS replica(s) in $NAMESPACE"
