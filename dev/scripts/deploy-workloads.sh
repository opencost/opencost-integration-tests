#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

kubectl apply -f "$REPO_ROOT/dev/manifests/workloads/"
echo "Seed workloads applied from dev/manifests/workloads/"
