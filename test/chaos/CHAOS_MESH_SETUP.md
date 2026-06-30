# Chaos Mesh Setup for Broker-Driven Chaos Tests

This document describes the preferred chaos-engine direction for the chaos
integration tests.

Instead of letting tests or broker callers pass raw `kubectl` commands, the
trusted broker should create and delete allowlisted Chaos Mesh resources. Chaos
Mesh performs the actual pod and network faults inside the cluster.

## Why Use a Chaos Engine

Direct `kubectl` calls and container `tc` commands work for local experiments,
but they are too flexible for the broker contract:

- callers may accidentally request arbitrary Kubernetes targets
- network chaos depends on tools being installed inside application containers
- cleanup is tied to command behavior inside the target pod
- the broker would need to own more risky command construction

With Chaos Mesh, the broker only performs fixed Kubernetes API operations:

- create an allowlisted `PodChaos` or `NetworkChaos` resource
- poll status if needed
- delete that exact resource during cleanup

The caller never supplies a shell command, raw manifest, pod name, or arbitrary
selector.

## Layering

```text
integration test
  -> pkg/cluster client
  -> trusted broker HTTP endpoint
  -> Kubernetes API creates allowlisted Chaos Mesh CR
  -> Chaos Mesh controller injects the fault
```

The broker still needs Kubernetes permissions, but those permissions are scoped
to creating/deleting known chaos resources and reading readiness state. The test
runner never gets kubeconfig.

## Day-0 Decision

Recommended chaos engine:

```text
Chaos Mesh
```

## Why Not Netflix Chaos Monkey

Netflix Chaos Monkey is not the right fit for this repository's broker-driven
Kubernetes tests.

The Netflix tool is designed around Spinnaker-managed applications, a MySQL
database, and cron-generated termination schedules. It randomly terminates
instances during configured working hours. That model is useful for production
instance-resilience programs, but it does not match this test suite's needs:

- no Spinnaker dependency in the local k3d/dev stack
- no MySQL state store needed for integration tests
- no weekday cron scheduler needed for deterministic CI
- no direct support for Kubernetes `PodChaos`/`NetworkChaos` custom resources
- no direct fit for the broker contract where tests request one fixed scenario
  and immediately assert OpenCost behavior

For this repository, "chaos monkey" should mean the general testing pattern:
inject controlled faults and verify recovery. The concrete implementation should
be Chaos Mesh resources created by the trusted broker.

## Supported Broker Operations

The HTTP paths and response shapes live in `docs/broker-api-contract.md`.

Current chaos operations:

| Broker operation | Chaos Mesh resource | Purpose |
| --- | --- | --- |
| `GET /v1/chaos` | none | List supported allowlisted chaos scenarios |
| `POST /v1/chaos/kill-opencost` | `PodChaos` | Kill one allowlisted OpenCost pod |
| `POST /v1/chaos/kill-prometheus` | `PodChaos` | Kill one allowlisted Prometheus pod |
| `POST /v1/chaos/partition-prometheus` | `NetworkChaos` | Block traffic between OpenCost and Prometheus |
| `POST /v1/chaos/latency-prometheus` | `NetworkChaos` | Add latency between OpenCost and Prometheus |
| `DELETE /v1/chaos/{scenario}` | delete named CR | Remove active chaos resources for one scenario |

## Installation Prerequisite

The target k3d/dev cluster must have Chaos Mesh installed before these tests run.
One common local installation path is Helm:

```bash
helm repo add chaos-mesh https://charts.chaos-mesh.org
helm repo update
kubectl create namespace chaos-mesh
helm install chaos-mesh chaos-mesh/chaos-mesh -n chaos-mesh
```

CI should install Chaos Mesh during cluster setup, before deploying the trusted
broker.

## Broker Permissions

The broker service account should have only the permissions needed for its fixed
operations.

Minimum read permissions:

```text
get/list pods
get/list services
get deployments
```

Restart permission:

```text
patch deployments
```

Chaos Mesh permissions:

```text
get/list/watch/create/delete podchaos.chaos-mesh.org
get/list/watch/create/delete networkchaos.chaos-mesh.org
```

Avoid granting broad write access to pods. If a scenario can be expressed as a
Chaos Mesh CRD, prefer that over `pods/exec`.

## Static Scenario Model

Each scenario is hardcoded in the broker:

```text
kill-opencost
kill-prometheus
partition-prometheus
latency-prometheus
```

The broker maps each scenario to a fixed resource template. The caller can only
select the scenario. The caller must not provide:

- raw YAML
- shell commands
- arbitrary namespaces
- arbitrary pod names
- arbitrary label selectors
- arbitrary latency/loss values

If a parameter is truly needed later, add it to the HTTP contract first and
validate it with a tight allowlist or regex.

## Local Developer Flow

1. Start the local k3d/dev cluster.
2. Install Chaos Mesh.
3. Deploy OpenCost and Prometheus.
4. Deploy the broker with the scoped service account.
5. Run tests with:

```bash
export CHAOS_ENABLED=1
export OPENCOST_BROKER_URL='http://localhost:<port>'
export OPENCOST_BROKER_TOKEN='<dev-token>'
go test ./test/chaos -v
```

The chaos tests should not import `os/exec` or call `kubectl` directly.
