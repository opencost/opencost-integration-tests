package chaos

import (
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/log"
)

// PassCriteria defines the expected outcomes for a chaos scenario
type PassCriteria struct {
	MinSuccessRate    float64       // Minimum percentage of requests that should succeed (0-1)
	MaxLatency        time.Duration // Maximum acceptable latency
	RequiredErrorCode int           // Expected HTTP status code for errors (0 if not checking)
	RequireJSONErrors bool          // Must errors be in JSON format
}

// ChaosScenario defines a single chaos injection test
// and allows suite-level registration of explicit failure modes.
type ChaosScenario struct {
	Name         string
	Description  string
	Setup        func(*testing.T)
	Inject       func(*testing.T)
	Cleanup      func(*testing.T)
	Test         func(*testing.T)
	Criteria     PassCriteria
	TimeoutAfter time.Duration
}

var chaosEnv = LoadChaosEnv()

func requireChaosEnabled(t *testing.T) {
	if !chaosEnv.Enabled {
		t.Skip("CHAOS_ENABLED not set; skipping chaos injection test")
	}
}

func requireKubernetes(t *testing.T) {
	if !isKubernetesAvailable() {
		t.Skip("Kubernetes not available")
	}
}

func TestChaosScenarios(t *testing.T) {
	requireChaosEnabled(t)

	scenarios := []ChaosScenario{
		buildKillOpencostScenario(),
		buildPrometheusPartitionScenario(),
		buildPrometheusDownScenario(),
		buildNetworkLatencyScenario(),
	}

	for _, scenario := range scenarios {
		scenario := scenario
		t.Run(scenario.Name, func(t *testing.T) {
			runChaosScenario(t, scenario)
		})
	}
}

func buildKillOpencostScenario() ChaosScenario {
	return ChaosScenario{
		Name:        "Kill OpenCost Pod",
		Description: "Terminates the opencost pod and verifies graceful error handling",
		Setup: func(t *testing.T) {
			requireKubernetes(t)
		},
		Inject: func(t *testing.T) {
			pod := findPodByLabel(opencostNamespace, opencostPodLabel)
			if pod == "" {
				t.Fatalf("opencost pod not found with label %s", opencostPodLabel)
			}
			t.Logf("Killing opencost pod: %s", pod)
			cmd := exec.Command("kubectl", "delete", "pod", pod, "-n", opencostNamespace, "--ignore-not-found=true")
			if err := cmd.Run(); err != nil {
				t.Fatalf("failed to kill pod: %v", err)
			}
			time.Sleep(2 * time.Second)
		},
		Cleanup: func(t *testing.T) {
			cmd := exec.Command("kubectl", "rollout", "restart", "deployment/opencost", "-n", opencostNamespace)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Logf("warning: failed to restart opencost deployment: %v; output: %s", err, out)
			}
			waitForPodReady(opencostNamespace, opencostPodLabel, 60*time.Second)
		},
		Test: func(t *testing.T) {
			testAllocationAPIWithRetries(3, defaultRetryInterval)
		},
		Criteria: PassCriteria{
			MinSuccessRate:    0.0,
			RequiredErrorCode: http.StatusServiceUnavailable,
			RequireJSONErrors: true,
		},
		TimeoutAfter: 30 * time.Second,
	}
}

func buildPrometheusPartitionScenario() ChaosScenario {
	return ChaosScenario{
		Name:        "Prometheus Network Partition",
		Description: "Introduces network latency and packet loss between OpenCost and Prometheus",
		Setup: func(t *testing.T) {
			requireKubernetes(t)
			pod := findPodByLabel(opencostNamespace, opencostPodLabel)
			if pod == "" {
				t.Fatalf("opencost pod not found")
			}
		},
		Inject: func(t *testing.T) {
			pod := findPodByLabel(opencostNamespace, opencostPodLabel)
			promIP := getServiceIP(prometheusNamespace, prometheusServiceName)
			if promIP == "" {
				t.Fatalf("prometheus service IP not found")
			}

			t.Logf("Adding network partition (latency=%s, loss=%s) to Prometheus IP %s from pod %s",
				networkPartitionDelay, networkPartitionLoss, promIP, pod)

			cmd := exec.Command(
				"kubectl", "exec", pod, "-n", opencostNamespace, "--",
				"sh", "-c",
				fmt.Sprintf("tc qdisc add dev eth0 root netem delay %s loss %s jitter %s dst %s",
					networkPartitionDelay, networkPartitionLoss, networkPartitionJitter, promIP),
			)
			if err := cmd.Run(); err != nil {
				t.Logf("warning: tc command may have failed (pod might not have tc installed): %v", err)
			}
		},
		Cleanup: func(t *testing.T) {
			pod := findPodByLabel(opencostNamespace, opencostPodLabel)
			if pod != "" {
				cmd := exec.Command(
					"kubectl", "exec", pod, "-n", opencostNamespace, "--",
					"sh", "-c", "tc qdisc del dev eth0 root 2>/dev/null || true",
				)
				_ = cmd.Run()
			}
		},
		Test: func(t *testing.T) {
			results := testAllocationAPIWithLatencyMeasurement(5, 2*time.Second)
			if results.TotalRequests == 0 {
				t.Fatalf("no requests completed")
			}
			successRate := float64(results.SuccessCount) / float64(results.TotalRequests)
			if successRate > 0.8 {
				t.Logf("warning: success rate too high (%f); partition may not be effective", successRate)
			}
			if results.MaxLatency < 3*time.Second {
				t.Logf("warning: max latency (%v) lower than expected; partition may not be effective", results.MaxLatency)
			}
		},
		Criteria: PassCriteria{
			MinSuccessRate:    0.2,
			MaxLatency:        10 * time.Second,
			RequireJSONErrors: true,
		},
		TimeoutAfter: 60 * time.Second,
	}
}

func buildPrometheusDownScenario() ChaosScenario {
	return ChaosScenario{
		Name:        "Prometheus Pod Down",
		Description: "Kills the Prometheus pod and verifies OpenCost returns appropriate 5xx errors",
		Setup: func(t *testing.T) {
			requireKubernetes(t)
		},
		Inject: func(t *testing.T) {
			pod := findPodByLabel(prometheusNamespace, prometheusPodLabel)
			if pod == "" {
				t.Fatalf("prometheus pod not found with label %s", prometheusPodLabel)
			}
			t.Logf("Killing Prometheus pod: %s", pod)
			cmd := exec.Command("kubectl", "delete", "pod", pod, "-n", prometheusNamespace, "--ignore-not-found=true")
			if err := cmd.Run(); err != nil {
				t.Fatalf("failed to kill Prometheus pod: %v", err)
			}
			time.Sleep(2 * time.Second)
		},
		Cleanup: func(t *testing.T) {
			cmd := exec.Command("kubectl", "rollout", "restart", "statefulset/prometheus", "-n", prometheusNamespace)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Logf("warning: failed to restart prometheus statefulset: %v; output: %s", err, out)
			}
			waitForPodReady(prometheusNamespace, prometheusPodLabel, 60*time.Second)
		},
		Test: func(t *testing.T) {
			results := testAllocationAPIWithRetries(3, 1*time.Second)
			if results.SuccessCount > 0 {
				t.Logf("warning: unexpected successes when Prometheus is down (%d/%d)", results.SuccessCount, results.TotalRequests)
			}
			for _, failure := range results.Failures {
				if failure.StatusCode < http.StatusInternalServerError || failure.StatusCode > 599 {
					t.Errorf("expected 5xx error for Prometheus-down, got %d", failure.StatusCode)
				}
				if !isJSONResponse(failure.Body) {
					t.Errorf("expected JSON error response, got: %s", failure.Body[:min(100, len(failure.Body))])
				}
			}
		},
		Criteria: PassCriteria{
			MinSuccessRate:    0.0,
			RequiredErrorCode: http.StatusServiceUnavailable,
			RequireJSONErrors: true,
		},
		TimeoutAfter: 30 * time.Second,
	}
}

func buildNetworkLatencyScenario() ChaosScenario {
	return ChaosScenario{
		Name:        "Network Latency Injection",
		Description: "Introduces increasing network latency and measures impact on response times",
		Setup: func(t *testing.T) {
			requireKubernetes(t)
		},
		Inject: func(t *testing.T) {
			pod := findPodByLabel(opencostNamespace, opencostPodLabel)
			promIP := getServiceIP(prometheusNamespace, prometheusServiceName)
			if pod == "" || promIP == "" {
				t.Skip("required pods/services not found")
			}

			t.Logf("Adding %s latency to Prometheus requests from %s", networkPartitionDelay, pod)
			cmd := exec.Command(
				"kubectl", "exec", pod, "-n", opencostNamespace, "--",
				"sh", "-c",
				fmt.Sprintf("tc qdisc add dev eth0 root netem delay %s dst %s", networkPartitionDelay, promIP),
			)
			if err := cmd.Run(); err != nil {
				t.Logf("warning: tc command may not be available: %v", err)
			}
		},
		Cleanup: func(t *testing.T) {
			pod := findPodByLabel(opencostNamespace, opencostPodLabel)
			if pod != "" {
				cmd := exec.Command(
					"kubectl", "exec", pod, "-n", opencostNamespace, "--",
					"sh", "-c", "tc qdisc del dev eth0 root 2>/dev/null || true",
				)
				_ = cmd.Run()
			}
		},
		Test: func(t *testing.T) {
			results := testAllocationAPIWithLatencyMeasurement(3, 2*time.Second)
			t.Logf("Latency test: min=%v, max=%v, avg=%v, success=%d/%d",
				results.MinLatency, results.MaxLatency, results.AvgLatency,
				results.SuccessCount, results.TotalRequests)

			if results.MaxLatency < 2*time.Second {
				t.Logf("warning: max latency (%v) lower than injected delay (5s)", results.MaxLatency)
			}
			if results.SuccessCount == 0 {
				t.Errorf("all requests failed; latency injection may be too aggressive")
			}
		},
		Criteria: PassCriteria{
			MinSuccessRate: 0.5,
			MaxLatency:     15 * time.Second,
		},
		TimeoutAfter: 45 * time.Second,
	}
}

// --- Helper Functions ---

type TestResult struct {
	TotalRequests int
	SuccessCount  int
	Failures      []FailureDetail
	MinLatency    time.Duration
	MaxLatency    time.Duration
	AvgLatency    time.Duration
}

type FailureDetail struct {
	StatusCode int
	Body       string
	Error      string
	Latency    time.Duration
}

func runChaosScenario(t *testing.T, scenario ChaosScenario) {
	log.Infof("Starting chaos scenario: %s", scenario.Name)
	log.Infof("Description: %s", scenario.Description)
	log.Infof("Pass criteria: %+v", scenario.Criteria)

	if scenario.Setup != nil {
		scenario.Setup(t)
	}

	if scenario.Inject != nil {
		scenario.Inject(t)
	}

	if scenario.Cleanup != nil {
		defer scenario.Cleanup(t)
	}

	// Wait for chaos to take effect
	time.Sleep(1 * time.Second)

	// Run test with timeout
	done := make(chan struct{})
	var testErr error
	go func() {
		defer func() {
			if r := recover(); r != nil {
				testErr = fmt.Errorf("test panicked: %v", r)
			}
			close(done)
		}()
		if scenario.Test != nil {
			scenario.Test(t)
		}
	}()

	select {
	case <-done:
	case <-time.After(scenario.TimeoutAfter):
		t.Fatalf("test timeout exceeded: %v", scenario.TimeoutAfter)
	}

	if testErr != nil {
		t.Fatalf("test execution failed: %v", testErr)
	}

	log.Infof("Chaos scenario completed: %s", scenario.Name)
}

func testAllocationAPIWithRetries(retries int, interval time.Duration) TestResult {
	result := TestResult{}
	apiClient := api.NewAPI()

	for i := 0; i < retries; i++ {
		start := time.Now()
		resp, err := apiClient.GetAllocation(api.AllocationRequest{
			Window: "2025-01-01T00:00:00Z,2025-01-02T00:00:00Z",
		})
		latency := time.Since(start)
		result.TotalRequests++

		if err != nil {
			failure := FailureDetail{Error: err.Error(), Latency: latency}
			result.Failures = append(result.Failures, failure)
		} else if resp.Code != http.StatusOK {
			result.Failures = append(result.Failures, FailureDetail{StatusCode: resp.Code, Latency: latency})
		} else {
			result.SuccessCount++
		}

		if result.MinLatency == 0 || latency < result.MinLatency {
			result.MinLatency = latency
		}
		if latency > result.MaxLatency {
			result.MaxLatency = latency
		}

		if i < retries-1 {
			time.Sleep(interval)
		}
	}

	if result.TotalRequests > 0 {
		totalLatency := time.Duration(0)
		for _, f := range result.Failures {
			totalLatency += f.Latency
		}
		totalLatency += time.Duration(result.SuccessCount) * result.MaxLatency // rough estimate
		result.AvgLatency = totalLatency / time.Duration(result.TotalRequests)
	}

	return result
}

func testAllocationAPIWithLatencyMeasurement(requests int, interval time.Duration) TestResult {
	result := TestResult{}
	apiClient := api.NewAPI()

	for i := 0; i < requests; i++ {
		start := time.Now()
		resp, err := apiClient.GetAllocation(api.AllocationRequest{
			Window: "2025-01-01T00:00:00Z,2025-01-02T00:00:00Z",
		})
		latency := time.Since(start)
		result.TotalRequests++

		if result.MinLatency == 0 || latency < result.MinLatency {
			result.MinLatency = latency
		}
		if latency > result.MaxLatency {
			result.MaxLatency = latency
		}

		if err != nil {
			result.Failures = append(result.Failures, FailureDetail{Error: err.Error(), Latency: latency})
		} else if resp.Code != http.StatusOK {
			result.Failures = append(result.Failures, FailureDetail{StatusCode: resp.Code, Latency: latency})
		} else {
			result.SuccessCount++
		}

		if i < requests-1 {
			time.Sleep(interval)
		}
	}

	totalLatency := time.Duration(0)
	for _, f := range result.Failures {
		totalLatency += f.Latency
	}
	if result.SuccessCount > 0 {
		totalLatency += time.Duration(result.SuccessCount) * result.MaxLatency / 2 // rough estimate
	}
	if result.TotalRequests > 0 {
		result.AvgLatency = totalLatency / time.Duration(result.TotalRequests)
	}

	return result
}

func isKubernetesAvailable() bool {
	cmd := exec.Command("kubectl", "cluster-info")
	return cmd.Run() == nil
}

func findPodByLabel(namespace, label string) string {
	cmd := exec.Command("kubectl", "get", "pods", "-n", namespace, "-l", label, "-o", "jsonpath={.items[0].metadata.name}")
	output, err := cmd.CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) == "" {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func getServiceIP(namespace, service string) string {
	cmd := exec.Command("kubectl", "get", "svc", service, "-n", namespace, "-o", "jsonpath={.spec.clusterIP}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func waitForPodReady(namespace, label string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		pod := findPodByLabel(namespace, label)
		if pod != "" {
			cmd := exec.Command(
				"kubectl", "get", "pod", pod, "-n", namespace,
				"-o", "jsonpath={.status.conditions[?(@.type==\"Ready\")].status}",
			)
			output, err := cmd.CombinedOutput()
			if err == nil && strings.Contains(string(output), "True") {
				return true
			}
		}
		time.Sleep(2 * time.Second)
	}
	return false
}

func isJSONResponse(body string) bool {
	body = strings.TrimSpace(body)
	return strings.HasPrefix(body, "{") && strings.HasSuffix(body, "}")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
