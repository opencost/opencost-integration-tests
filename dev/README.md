# Local OpenCost Dev Stack

Scripted local environment for running OpenCost integration tests against a k3d cluster with Prometheus, OpenCost, and seed workloads.

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/)
- [k3d](https://k3d.io/) v5+
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [helm](https://helm.sh/docs/intro/install/) v3+
- Go (see root `go.mod`)

Clone submodules and download dependencies from the repo root:

```sh
git submodule update --init --recursive
go mod download
```

## Quick start

```sh
# Start the stack
./dev/up.sh

# In another terminal: port-forward services
./dev/scripts/port-forward.sh

# Export env vars for tests
export OPENCOST_URL='http://localhost:9003'
export PROMETHEUS_URL='http://localhost:9090'
export OPENCOST_MCP_URL='http://localhost:8081'

# Run a representative test suite
./test/bats/bin/bats ./test/integration/query/count/test.bats
```

Tear down:

```sh
./dev/down.sh
```

## What gets deployed

| Component | Namespace | Notes |
|-----------|-----------|-------|
| k3d cluster `opencost-dev` | — | 1 server + 2 agents |
| Prometheus | `prometheus-system` | Includes kube-state-metrics with annotation allowlist |
| OpenCost | `opencost` | UI, MCP, custom pricing enabled |
| Seed workloads | `workload-test` | nginx Deployment + web StatefulSet with test annotations |

Helm values live in `dev/helm/`. Workload manifests live in `dev/manifests/workloads/`.

## Helper scripts

| Script | Purpose |
|--------|---------|
| `dev/scripts/port-forward.sh` | Forward Prometheus (9090) and OpenCost (9003, 8081) |
| `dev/scripts/deploy-workloads.sh` | Apply seed workloads |
| `dev/scripts/toggle-prometheus.sh on\|off` | Scale Prometheus server for outage/resilience testing |
| `dev/scripts/collect-logs.sh [dir]` | Dump pod status and logs for debugging |

## Running integration tests

After port-forwarding and exporting env vars:

```sh
# Recommended smoke tests
./test/bats/bin/bats ./test/integration/query/count/test.bats

# Broader allocation API tests
./test/bats/bin/bats ./test/integration/api/allocation/test.bats
```

### Local test matrix (k3d)

| Suite | Expected on local stack |
|-------|-------------------------|
| `query/count` | Pass |
| `api/allocation` — Pod/Namespace labels | Pass |
| `api/allocation` — Pod/Namespace annotations | Pass after kube-state-metrics allowlist is applied and metrics have scraped (~5–10 min) |
| `api/allocation` — controller kind consistency | May fail (OpenCost 500 on `controllerKind` filter on small clusters) |
| `api/allocation` — negative idle costs | May fail (`__idle__` missing on small/fresh clusters) |
| Demo-only tests (GPU, cloud LB, etc.) | Skip or fail — use `demo.infra.opencost.io` for those |

Verify annotation metrics are flowing:

```sh
curl -s 'http://localhost:9090/api/v1/query?query=kube_pod_annotations' | head -c 500
kubectl logs -n prometheus-system -l app.kubernetes.io/name=kube-state-metrics | grep -i allowlist
```

## Troubleshooting

**Prometheus annotation metrics empty**

Re-apply Helm values and wait for kube-state-metrics to restart:

```sh
helm upgrade --install prometheus prometheus-community/prometheus \
  -n prometheus-system -f dev/helm/prometheus-values.yaml
```

**Port 9090 already in use**

Stop the conflicting process or change the local port in `port-forward.sh`.

**OpenCost pods not ready**

```sh
kubectl get pods -A
./dev/scripts/collect-logs.sh
```

**MCP health check**

`GET /healthz` on port 8081 may require an MCP session header — that is expected. Tests use the allocation API on port 9003.
