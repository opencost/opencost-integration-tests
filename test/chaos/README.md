# Chaos Injection Integration Tests

This directory contains chaos engineering tests for OpenCost, designed to verify resilience and error handling under failure conditions.

## Overview

The chaos injection suite tests how OpenCost behaves when:

- **OpenCost Pod Failure**: The main application container is killed
- **Prometheus Unavailability**: The Prometheus data source is unavailable
- **Network Partition**: High latency and packet loss between services
- **Gradual Latency**: Progressive network delay increases

Each scenario includes explicit **pass/fail criteria** to verify acceptable behavior.

## Prerequisites

### Required Tools

- `kubectl` (configured with access to target cluster)
- `tc` (traffic control; Linux network simulator, part of `iproute2`)
- Go 1.19+
- Running OpenCost and Prometheus instances

### Kubernetes Access

Tests assume:
- OpenCost deployed in `opencost` namespace (configurable)
- Prometheus deployed in `prometheus` namespace (configurable)
- Pods labeled with `app=opencost` and `app.kubernetes.io/name=prometheus`

## Running Chaos Tests

### Enable Chaos Tests

```bash
export CHAOS_ENABLED=1
go test ./test/chaos -v -run 'TestChaosScenarios/Kill OpenCost Pod'
```

### Test-Specific Environment Variables

```bash
# Custom namespaces
export OPENCOST_NAMESPACE=monitoring
export PROMETHEUS_NAMESPACE=metrics

# Dry-run mode (plan changes without executing)
export CHAOS_DRY_RUN=1

# Specific Kubernetes context
export KUBE_CONTEXT=my-cluster

# Run all chaos tests
CHAOS_ENABLED=1 go test ./test/chaos -v
```

## Test Scenarios

### 1. Kill OpenCost Pod

**File**: `TestKillOpencostPod`

**What it tests**: Termination of the OpenCost application and recovery behavior

**Pass Criteria**:
- Pod is killed successfully ✓
- API returns 503 (Service Unavailable) status code ✓
- Error responses are valid JSON ✓
- Pod automatically restarts via Kubernetes orchestration ✓

**Expected duration**: ~30 seconds

```bash
CHAOS_ENABLED=1 go test ./test/chaos -v -run 'TestChaosScenarios/Kill OpenCost Pod'
```

---

### 2. Prometheus Network Partition

**File**: `TestPrometheusPartition`

**What it tests**: Network degradation between OpenCost and Prometheus

**Pass Criteria**:
- Latency of 5000ms is introduced ✓
- Packet loss of 50% is introduced ✓
- At least 20% of requests succeed (with extended timeouts) ✓
- All error responses are JSON-formatted ✓
- Max latency observed > 3000ms (validates tc rule effectiveness) ✓

**Implementation**: Uses Linux `tc` (traffic control) to add:
- 5000ms delay
- 50% packet loss
- ±100ms jitter

**Expected duration**: ~60 seconds

```bash
CHAOS_ENABLED=1 go test ./test/chaos -v -run 'TestChaosScenarios/Prometheus Network Partition'
```

---

### 3. Prometheus Pod Down

**File**: `TestPrometheusDown`

**What it tests**: Complete Prometheus unavailability

**Pass Criteria**:
- Prometheus pod is killed ✓
- OpenCost returns 5xx (Server Error) responses ✓
- All error responses are JSON-formatted ✓
- No success responses while Prometheus is down ✓
- Prometheus pod automatically restarts ✓

**Expected duration**: ~30 seconds

```bash
CHAOS_ENABLED=1 go test ./test/chaos -v -run 'TestChaosScenarios/Prometheus Pod Down'
```

---

### 4. Network Latency Injection

**File**: `TestNetworkLatency`

**What it tests**: API latency under network degradation

**Pass Criteria**:
- 5000ms latency is successfully injected ✓
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
1. Kill/stress the target component
2. Run API requests under failure conditions
3. Verify error handling
4. **Restore the environment** (restart pods, remove network rules)

If cleanup fails, manually restore:

```bash
# Restart OpenCost
kubectl rollout restart deployment/opencost -n opencost

# Restart Prometheus
kubectl rollout restart statefulset/prometheus -n prometheus

# Remove network traffic control rules
kubectl exec <pod-name> -n opencost -- tc qdisc del dev eth0 root
```

## Troubleshooting

### "tc: command not found"

The `tc` command may not be available in container images. Install it:

```bash
kubectl exec <opencost-pod-name> -n opencost -- apt-get install -y iproute2
# or for Alpine
kubectl exec <opencost-pod-name> -n opencost -- apk add --no-cache iproute2
```

### Tests Skip with "CHAOS_ENABLED not set"

Enable explicitly:

```bash
export CHAOS_ENABLED=1
```

### Pod Not Found

Verify pod labels:

```bash
kubectl get pods -n opencost -L app
kubectl get pods -n prometheus -L app.kubernetes.io/name
```

If labels differ, update the constants in `chaos_injection_test.go`:

```go
const opencostPodLabel = "your-custom-label=opencost"
```

### Kubernetes Connection Fails

Check context and credentials:

```bash
kubectl cluster-info
kubectl auth can-i get pods --all-namespaces
```

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
			// Pre-flight checks
		},
		Inject: func(t *testing.T) {
			// Apply failure condition
		},
		Test: func(t *testing.T) {
			// Run assertions
		},
		Cleanup: func(t *testing.T) {
			// Restore environment
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
    KUBECONFIG: /tmp/kubeconfig
  run: |
    go test ./test/chaos -v -timeout 10m
```

## References

- [Linux tc (traffic control) Manual](https://man7.org/linux/man-pages/man8/tc.8.html)
- [Chaos Engineering Principles](https://principlesofchaos.org/)
- [Kubernetes Pod Disruption Budgets](https://kubernetes.io/docs/tasks/run-application/configure-pdb/)
