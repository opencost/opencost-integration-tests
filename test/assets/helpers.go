package assets

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/env"
)

const (
	// fixtureID is the embedded fixed-pricing fixture the broker applies. Its
	// values must stay in sync with cmd/ops-broker/fixtures/pricing-fixed-v1.yaml.
	fixtureID = "pricing-fixed-v1"

	// hoursPerMonth is OpenCost's monthly→hourly divisor. Verified empirically:
	// OpenCost interprets custom CPU/RAM/storage as MONTHLY prices and emits the
	// per-unit hourly rate as value/730 (node_cpu_hourly_cost, node_ram_hourly_cost,
	// pv_hourly_cost). We therefore assert on those instantaneous metrics — which
	// reflect a price change immediately — rather than the windowed /assets cost,
	// which integrates historically-recorded pricing and lags a fresh fixture.
	hoursPerMonth = 730.0

	// Fixture values (monthly), and the per-unit hourly rates OpenCost should emit.
	fixtureCPUMonthly     = 1.0
	fixtureRAMMonthly     = 0.5
	fixtureStorageMonthly = 0.04

	expectedNodeCPUHourly = fixtureCPUMonthly / hoursPerMonth     // node_cpu_hourly_cost
	expectedNodeRAMHourly = fixtureRAMMonthly / hoursPerMonth     // node_ram_hourly_cost
	expectedPVHourly      = fixtureStorageMonthly / hoursPerMonth // pv_hourly_cost

	// assetsPricingTolerance matches the 0.05 used by the other pricing tests.
	assetsPricingTolerance = 0.05

	defaultReadyTimeout  = 3 * time.Minute
	defaultRetryInterval = 3 * time.Second
	metricPollTimeout    = 90 * time.Second
	metricPollInterval   = 3 * time.Second
)

// AssetsEnvironment holds configuration for the asset ground-truth suite. Like
// the chaos and restart suites, it reaches the cluster only through the broker.
type AssetsEnvironment struct {
	Enabled            bool
	DryRun             bool
	BrokerURL          string
	Token              string
	OpenCostNamespace  string
	OpenCostDeployment string
}

// LoadAssetsEnv loads the asset environment from OS variables.
func LoadAssetsEnv() AssetsEnvironment {
	return AssetsEnvironment{
		Enabled:            os.Getenv("ASSETS_ENABLED") != "",
		DryRun:             os.Getenv("ASSETS_DRY_RUN") != "",
		BrokerURL:          strings.TrimRight(os.Getenv("OPENCOST_BROKER_URL"), "/"),
		Token:              os.Getenv("OPENCOST_BROKER_TOKEN"),
		OpenCostNamespace:  getEnv("OPENCOST_NAMESPACE", "opencost"),
		OpenCostDeployment: getEnv("OPENCOST_DEPLOYMENT", "opencost"),
	}
}

func (e AssetsEnvironment) ValidateBrokerConfig() error {
	if e.BrokerURL == "" {
		return fmt.Errorf("OPENCOST_BROKER_URL is required for broker-driven asset tests")
	}
	if e.Token == "" {
		return fmt.Errorf("OPENCOST_BROKER_TOKEN is required for broker-driven asset tests")
	}
	return nil
}

// metricsURL returns OpenCost's Prometheus /metrics endpoint (same host as the
// model API in OPENCOST_URL).
func metricsURL() string {
	return strings.TrimRight(env.GetDefaultURL(), "/") + "/metrics"
}

// scrapeMetricValues fetches OpenCost's /metrics and returns the value of every
// series of the named metric (one per node / PV).
func scrapeMetricValues(url, metric string) ([]float64, error) {
	client := http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metrics endpoint %s returned status %d", url, resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseMetricValues(string(raw), metric), nil
}

// parseMetricValues extracts the float value of each series of `metric` from a
// Prometheus text-format exposition body.
func parseMetricValues(body, metric string) []float64 {
	var vals []float64
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.HasPrefix(line, metric) {
			continue
		}
		// Ensure the whole metric name matched, not a longer one sharing the prefix
		// (e.g. node_cpu_hourly_cost vs node_cpu_hourly_cost_total).
		rest := line[len(metric):]
		if rest != "" && rest[0] != '{' && rest[0] != ' ' && rest[0] != '\t' {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		v, err := strconv.ParseFloat(fields[len(fields)-1], 64)
		if err != nil {
			continue
		}
		vals = append(vals, v)
	}
	return vals
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
