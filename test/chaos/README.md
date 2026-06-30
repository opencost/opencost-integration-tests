# Chaos Injection Integration Tests

This directory contains chaos engineering tests for OpenCost, designed to verify resilience and error handling under failure conditions.

The tests are broker-driven: the test runner calls a trusted broker, and the
broker creates allowlisted Chaos Mesh resources. Test code must not run direct
`kubectl` commands or pass arbitrary shell commands/manifests to the broker.

## Overview

The chaos injection suite tests how OpenCost behaves when:

- **OpenCost Pod Failure**: The main application container is killed
- **Prometheus Unavailability**: The Prometheus data source is unavailable
- **Network Partition**: OpenCost is partitioned from Prometheus
- **Network Latency**: Latency is added between OpenCost and Prometheus

Each scenario includes explicit **pass/fail criteria** to verify acceptable behavior.

## Prerequisites

### Required Tools

- Go 1.19+
- Running OpenCost and Prometheus instances
- Trusted broker reachable through `OPENCOST_BROKER_URL`
- Broker token in `OPENCOST_BROKER_TOKEN`
- Chaos Mesh installed in the target cluster

### Broker Access

Tests assume:
- The broker has Kubernetes access through its own ServiceAccount.
- The broker owns fixed chaos scenarios documented in `CHAOS_MESH_SETUP.md`.
- The test runner has only the broker URL/token, not kubeconfig.

### Broker Integration Points

The authoritative broker HTTP contract lives in `docs/broker-api-contract.md`.
Chaos tests call the broker through `pkg/cluster`; they should not build raw
broker URLs themselves.

The chaos tests currently use these broker operations:

```text
GET    /healthz
GET    /v1/chaos
POST   /v1/chaos/kill-opencost
POST   /v1/chaos/kill-prometheus
POST   /v1/chaos/partition-prometheus
POST   /v1/chaos/latency-prometheus
DELETE /v1/chaos/{scenario}
```

Every `/v1` request includes:

```text
Authorization: Bearer <OPENCOST_BROKER_TOKEN>
Accept: application/json
```

`GET /healthz` is unauthenticated and should return:

```json
{
  "status": "ok"
}
```

`GET /v1/chaos` should return the fixed scenarios the broker supports:

```json
{
  "scenarios": [
    { "id": "kill-opencost", "engine": "chaos-mesh" },
    { "id": "kill-prometheus", "engine": "chaos-mesh" },
    { "id": "partition-prometheus", "engine": "chaos-mesh" },
    { "id": "latency-prometheus", "engine": "chaos-mesh" }
  ]
}
```

Successful injection should return:

```json
{
  "injected": true,
  "scenario": "kill-opencost"
}
```

Successful cleanup should return:

```json
{
  "deleted": true,
  "scenario": "kill-opencost"
}
```

The test client validates health and required scenario support before injecting
chaos, so missing broker functionality fails early.

## Running Chaos Tests

### Enable Chaos Tests

```bash
export CHAOS_ENABLED=1
export OPENCOST_URL='http://localhost:9003'
export OPENCOST_BROKER_URL='http://localhost:8080'
export OPENCOST_BROKER_TOKEN='<dev-token>'
go test ./test/chaos -v -run 'TestChaosScenarios/Kill OpenCost Pod'
```

For local destructive runs, prefer a stable service URL for `OPENCOST_URL`
(NodePort, ingress, or an in-cluster runner). A `kubectl port-forward` directly
to OpenCost can be severed by the `kill-opencost` scenario because the test
intentionally replaces the pod behind that connection.

### Test-Specific Environment Variables

```bash
# Trusted broker connection
export OPENCOST_BROKER_URL='http://localhost:8080'
export OPENCOST_BROKER_TOKEN='<dev-token>'

# Dry-run mode logs the broker scenario without injecting it
export CHAOS_DRY_RUN=1

# Run all chaos tests
CHAOS_ENABLED=1 go test ./test/chaos -v
```

## Test Scenarios

### 1. Kill OpenCost Pod

**File**: `TestKillOpencostPod`

**What it tests**: Termination of the OpenCost application and recovery behavior

**Broker scenario**: `kill-opencost`

**Pass Criteria**:
- Broker injects the allowlisted OpenCost pod-kill scenario ✓
- API returns 503 (Service Unavailable) status code ✓
- Error responses are valid JSON ✓
- Broker cleanup removes the chaos resource ✓

**Expected duration**: ~30 seconds

```bash
CHAOS_ENABLED=1 go test ./test/chaos -v -run 'TestChaosScenarios/Kill OpenCost Pod'
```

---

### 2. Prometheus Network Partition

**File**: `TestPrometheusPartition`

**What it tests**: Network degradation between OpenCost and Prometheus

**Broker scenario**: `partition-prometheus`

**Pass Criteria**:
- Broker injects the allowlisted Prometheus partition scenario ✓
- At least 20% of requests succeed (with extended timeouts) ✓
- All error responses are JSON-formatted ✓
- Max latency observed > 3000ms ✓

**Expected duration**: ~60 seconds

```bash
CHAOS_ENABLED=1 go test ./test/chaos -v -run 'TestChaosScenarios/Prometheus Network Partition'
```

---

### 3. Prometheus Pod Down

**File**: `TestPrometheusDown`

**What it tests**: Complete Prometheus unavailability

**Broker scenario**: `kill-prometheus`

**Pass Criteria**:
- Broker injects the allowlisted Prometheus pod-kill scenario ✓
- OpenCost returns 5xx (Server Error) responses ✓
- All error responses are JSON-formatted ✓
- No success responses while Prometheus is down ✓
- Broker cleanup removes the chaos resource ✓

**Expected duration**: ~30 seconds

```bash
CHAOS_ENABLED=1 go test ./test/chaos -v -run 'TestChaosScenarios/Prometheus Pod Down'
```

---

### 4. Network Latency Injection

**File**: `TestNetworkLatency`

**What it tests**: API latency under network degradation

**Broker scenario**: `latency-prometheus`

**Pass Criteria**:
- Broker injects the allowlisted Prometheus latency scenario ✓
- At least 50% of requests succeed ✓
- Max observed latency > 2000ms ✓
- No requests fail entirely (unless timeouts) ✓

**Expected duration**: ~45 seconds

```bash
CHAOS_ENABLED=1 go test ./test/chaos -v -run 'TestChaosScenarios/Network Latency Injection'
```

---

## Pass/Fail Criteria Details

Each scenario defines a `PassCriteria` struct:

```go
type PassCriteria struct {
	MinSuccessRate    float64       // e.g., 0.5 = minimum 50% success
	MaxLatency        time.Duration // e.g., 10s max acceptable latency
	RequiredErrorCode int           // e.g., 503 for unavailable
	RequireJSONErrors bool          // Must errors be JSON
}
```

### Examples

**Pod Failure Criteria**:
```go
Criteria: PassCriteria{
	MinSuccessRate:    0.0,    // Expect 0% success (pod is down)
	RequiredErrorCode: 503,    // Service Unavailable
	RequireJSONErrors: true,   // All errors in JSON
}
```

**Network Partition Criteria**:
```go
Criteria: PassCriteria{
	MinSuccessRate:    0.2,              // Expect ≥20% success (degraded)
	MaxLatency:        10 * time.Second, // Accept up to 10s latency
	RequireJSONErrors: true,             // All errors in JSON
}
```

## Cleanup

Tests automatically:
1. Ask the broker to inject an allowlisted chaos scenario
2. Run API requests under failure conditions
3. Verify error handling
4. Ask the broker to remove the scenario's chaos resource

If cleanup fails, use the broker cleanup endpoint or cluster-admin tooling to
remove broker-owned Chaos Mesh resources.

```bash
curl -X DELETE \
  -H "Authorization: Bearer ${OPENCOST_BROKER_TOKEN}" \
  "${OPENCOST_BROKER_URL}/v1/chaos/latency-prometheus"
```

## Troubleshooting

### Tests Skip with "CHAOS_ENABLED not set"

Enable explicitly:

```bash
export CHAOS_ENABLED=1
```

### Tests Skip Because Broker Config Is Missing

Set the trusted broker connection:

```bash
export OPENCOST_BROKER_URL='http://localhost:8080'
export OPENCOST_BROKER_TOKEN='<dev-token>'
```

### Broker Fails To Inject

Check broker logs and verify Chaos Mesh is installed in the target cluster.
Cluster credentials should belong to the broker ServiceAccount, not the test
runner.


## Adding New Chaos Scenarios

To add a new scenario:

```go
func TestMyScenario(t *testing.T) {
	if os.Getenv("CHAOS_ENABLED") == "" {
		t.Skip("CHAOS_ENABLED not set")
	}

	scenario := ChaosScenario{
		Name: "My Failure Mode",
		Description: "What I'm testing",
		Setup: func(t *testing.T) {
			// Optional pre-flight checks
		},
		Inject: func(t *testing.T) {
			// Call a fixed broker scenario
		},
		Test: func(t *testing.T) {
			// Run assertions
		},
		Cleanup: func(t *testing.T) {
			// Delete the broker-owned chaos resource
		},
		Criteria: PassCriteria{
			MinSuccessRate:    0.5,
			RequireJSONErrors: true,
		},
		TimeoutAfter: 30 * time.Second,
	}

	runChaosScenario(t, scenario)
}
```

## Continuous Integration

To run chaos tests in CI/CD:

```yaml
# Example GitHub Actions workflow
- name: Run Chaos Tests
  env:
    CHAOS_ENABLED: "1"
    OPENCOST_BROKER_URL: ${{ secrets.OPENCOST_BROKER_URL }}
    OPENCOST_BROKER_TOKEN: ${{ secrets.OPENCOST_BROKER_TOKEN }}
  run: |
    go test ./test/chaos -v -timeout 10m
```

## References

- [Chaos Engineering Principles](https://principlesofchaos.org/)
- [Chaos Mesh](https://chaos-mesh.org/)
- [Kubernetes Pod Disruption Budgets](https://kubernetes.io/docs/tasks/run-application/configure-pdb/)
