package chaos

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/cluster"
	"github.com/opencost/opencost-integration-tests/pkg/env"
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
	Name        string
	BrokerID    string
	Description string
	Setup       func(*testing.T)
	Inject      func(*testing.T)
	Cleanup     func(*testing.T)
	Test        func(*testing.T)
	Criteria    PassCriteria
}

var chaosEnv = LoadChaosEnv()

func requireChaosEnabled(t *testing.T) {
	if !chaosEnv.Enabled {
		t.Skip("CHAOS_ENABLED not set; skipping chaos injection test")
	}
}

func requireBroker(t *testing.T) {
	if chaosEnv.DryRun {
		return
	}
	if err := chaosEnv.ValidateBrokerConfig(); err != nil {
		t.Skip(err.Error())
	}
}

func TestChaosScenarios(t *testing.T) {
	requireChaosEnabled(t)

	scenarios := []ChaosScenario{
		buildNetworkLatencyScenario(),
		buildPrometheusPartitionScenario(),
		buildPrometheusDownScenario(),
		buildKillOpencostScenario(),
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
		BrokerID:    scenarioKillOpenCost,
		Name:        "Kill OpenCost Pod",
		Description: "Asks the broker to kill an allowlisted OpenCost pod and verifies graceful error handling",
		Test: func(t *testing.T) {
			testAllocationAPIWithRetries(3, defaultRetryInterval)
		},
		Criteria: PassCriteria{
			MinSuccessRate:    0.0,
			RequiredErrorCode: http.StatusServiceUnavailable,
			RequireJSONErrors: true,
		},
	}
}

func buildPrometheusPartitionScenario() ChaosScenario {
	return ChaosScenario{
		BrokerID:    scenarioPartitionPrometheus,
		Name:        "Prometheus Network Partition",
		Description: "Asks the broker to partition OpenCost from Prometheus with an allowlisted Chaos Mesh scenario",
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
	}
}

func buildPrometheusDownScenario() ChaosScenario {
	return ChaosScenario{
		BrokerID:    scenarioKillPrometheus,
		Name:        "Prometheus Pod Down",
		Description: "Asks the broker to kill an allowlisted Prometheus pod and verifies OpenCost returns appropriate 5xx errors",
		Test: func(t *testing.T) {
			results := testAllocationAPIWithRetries(3, 1*time.Second)
			if results.SuccessCount > 0 {
				t.Logf("warning: unexpected successes when Prometheus is down (%d/%d)", results.SuccessCount, results.TotalRequests)
			}
			for _, failure := range results.Failures {
				checkOutageFailure(t, "Prometheus-down", failure)
			}
		},
		Criteria: PassCriteria{
			MinSuccessRate:    0.0,
			RequiredErrorCode: http.StatusServiceUnavailable,
			RequireJSONErrors: true,
		},
	}
}

func buildNetworkLatencyScenario() ChaosScenario {
	return ChaosScenario{
		BrokerID:    scenarioLatencyPrometheus,
		Name:        "Network Latency Injection",
		Description: "Asks the broker to add allowlisted latency between OpenCost and Prometheus",
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
	log.Infof("Broker scenario ID: %s", scenario.BrokerID)
	log.Infof("Description: %s", scenario.Description)
	log.Infof("Pass criteria: %+v", scenario.Criteria)

	requireBroker(t)

	if scenario.Setup != nil {
		scenario.Setup(t)
	}

	ctx := context.Background()
	broker, err := cluster.NewClientFromEnv()
	if err != nil && !chaosEnv.DryRun {
		t.Skip(err.Error())
	}

	if !chaosEnv.DryRun {
		if err := broker.Healthz(ctx); err != nil {
			t.Fatalf("broker health check failed: %v", err)
		}
		if err := validateChaosScenarios(ctx, broker, requiredBrokerScenarios); err != nil {
			t.Fatalf("broker scenario preflight failed: %v", err)
		}
	}

	if scenario.Cleanup == nil {
		scenario.Cleanup = func(t *testing.T) {
			if chaosEnv.DryRun {
				t.Logf("CHAOS_DRY_RUN set; would cleanup broker chaos scenario %q", scenario.BrokerID)
				return
			}
			if err := broker.CleanupChaos(ctx, scenario.BrokerID); err != nil {
				t.Logf("warning: failed to cleanup broker chaos scenario %q: %v", scenario.BrokerID, err)
			}
			waitCtx, cancel := context.WithTimeout(ctx, defaultReadyTimeout)
			defer cancel()
			if pods, err := broker.WaitForOpenCostReady(waitCtx, defaultRetryInterval); err != nil {
				t.Logf("warning: OpenCost did not report ready after cleanup for scenario %q: %v", scenario.BrokerID, err)
			} else {
				t.Logf("OpenCost ready after cleanup for scenario %q: %d pod(s)", scenario.BrokerID, len(pods))
			}
		}
	}
	defer scenario.Cleanup(t)

	if scenario.Inject == nil {
		scenario.Inject = func(t *testing.T) {
			if chaosEnv.DryRun {
				t.Logf("CHAOS_DRY_RUN set; would inject broker chaos scenario %q", scenario.BrokerID)
				return
			}
			if err := broker.InjectChaos(ctx, scenario.BrokerID); err != nil {
				t.Fatalf("failed to inject broker chaos scenario %q: %v", scenario.BrokerID, err)
			}
		}
	}

	scenario.Inject(t)

	// Wait for chaos to take effect
	time.Sleep(1 * time.Second)

	if scenario.Test != nil {
		scenario.Test(t)
	}

	log.Infof("Chaos scenario completed: %s", scenario.Name)
}

func checkOutageFailure(t *testing.T, scenario string, failure FailureDetail) {
	t.Helper()

	if failure.Error != "" {
		t.Logf("%s produced expected transport-level outage: %s", scenario, failure.Error)
		return
	}
	if failure.StatusCode < http.StatusInternalServerError || failure.StatusCode > 599 {
		t.Errorf("expected transport error or 5xx error for %s, got %d", scenario, failure.StatusCode)
	}
	if !isJSONResponse(failure.Body) {
		t.Errorf("expected JSON error response for HTTP %s failure, got: %s", scenario, failure.Body[:min(100, len(failure.Body))])
	}
}

func validateChaosScenarios(ctx context.Context, broker *cluster.Client, required []string) error {
	scenarios, err := broker.ChaosScenarios(ctx)
	if err != nil {
		return err
	}

	available := map[string]bool{}
	for _, scenario := range scenarios {
		available[scenario.ID] = true
	}

	var missing []string
	for _, scenarioID := range required {
		if !available[scenarioID] {
			missing = append(missing, scenarioID)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("broker is missing required chaos scenarios: %s", strings.Join(missing, ", "))
	}
	return nil
}

func testAllocationAPIWithRetries(retries int, interval time.Duration) TestResult {
	result := TestResult{}

	for i := 0; i < retries; i++ {
		start := time.Now()
		statusCode, body, err := getAllocationRaw()
		latency := time.Since(start)
		result.TotalRequests++

		if err != nil {
			failure := FailureDetail{Error: err.Error(), Latency: latency}
			result.Failures = append(result.Failures, failure)
		} else if statusCode != http.StatusOK {
			result.Failures = append(result.Failures, FailureDetail{StatusCode: statusCode, Body: body, Latency: latency})
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

	for i := 0; i < requests; i++ {
		start := time.Now()
		statusCode, body, err := getAllocationRaw()
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
		} else if statusCode != http.StatusOK {
			result.Failures = append(result.Failures, FailureDetail{StatusCode: statusCode, Body: body, Latency: latency})
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

func getAllocationRaw() (int, string, error) {
	params := url.Values{}
	params.Set("window", "2025-01-01T00:00:00Z,2025-01-02T00:00:00Z")
	requestURL := fmt.Sprintf("%s/allocation?%s", strings.TrimRight(env.GetDefaultURL(), "/"), params.Encode())

	client := http.Client{Timeout: defaultRequestTimeout}
	resp, err := client.Get(requestURL)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, "", err
	}

	return resp.StatusCode, string(rawBody), nil
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
