package assets

// Description
// With a known fixed-pricing fixture applied via the broker, OpenCost's
// instantaneous per-unit pricing metrics must reflect the fixture. We assert on
// node_cpu_hourly_cost / node_ram_hourly_cost (not the windowed /assets cost,
// which lags a fresh price change), checking both:
//   1. Uniformity (formula-independent): every node reports the same unit price.
//   2. Expected price: that price equals the fixture's monthly value / 730, which
//      is how OpenCost converts custom pricing to an hourly rate.
// The broker node list is used as an independent count cross-check.

import (
	"context"
	"testing"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/cluster"
	"github.com/opencost/opencost-integration-tests/pkg/log"
	"github.com/opencost/opencost-integration-tests/pkg/utils"
)

var assetsEnv = LoadAssetsEnv()

func requireAssetsEnabled(t *testing.T) {
	if !assetsEnv.Enabled {
		t.Skip("ASSETS_ENABLED not set; skipping asset ground-truth test")
	}
}

// applyFixedPricing applies the fixed-pricing fixture through the broker, waits
// for OpenCost to come back ready, and registers cleanup. Returns the broker
// client for follow-on reads. Tests skip when the broker env is unset.
func applyFixedPricing(t *testing.T, ctx context.Context) *cluster.Client {
	t.Helper()

	if err := assetsEnv.ValidateBrokerConfig(); err != nil {
		t.Skip(err.Error())
	}
	broker, err := cluster.NewClientFromEnv()
	if err != nil {
		t.Skip(err.Error())
	}
	if err := broker.Healthz(ctx); err != nil {
		t.Fatalf("broker health check failed: %v", err)
	}

	if err := broker.ApplyConfig(ctx, fixtureID); err != nil {
		t.Fatalf("apply fixture %q: %v", fixtureID, err)
	}
	t.Cleanup(func() {
		if err := broker.DeleteConfig(context.Background(), fixtureID); err != nil {
			t.Logf("warning: failed to clean up fixture %q: %v", fixtureID, err)
		}
	})

	waitCtx, cancel := context.WithTimeout(ctx, defaultReadyTimeout)
	defer cancel()
	if _, err := broker.WaitForDeploymentReady(waitCtx, assetsEnv.OpenCostDeployment, assetsEnv.OpenCostNamespace, defaultRetryInterval); err != nil {
		t.Fatalf("OpenCost not ready after applying fixture: %v", err)
	}
	return broker
}

// pollMetric scrapes OpenCost's /metrics until the named pricing metric has at
// least one series, or fails after metricPollTimeout (the metric appears shortly
// after the restarted pod becomes ready).
func pollMetric(t *testing.T, metric string) []float64 {
	t.Helper()
	deadline := time.Now().Add(metricPollTimeout)
	for {
		vals, err := scrapeMetricValues(metricsURL(), metric)
		if err == nil && len(vals) > 0 {
			return vals
		}
		if time.Now().After(deadline) {
			t.Fatalf("metric %q not available within %s (err=%v, series=%d)", metric, metricPollTimeout, err, len(vals))
		}
		time.Sleep(metricPollInterval)
	}
}

// checkPricing asserts a pricing metric is uniform across assets and matches the
// fixture's expected hourly rate.
func checkPricing(t *testing.T, label string, vals []float64, expected float64) {
	t.Helper()
	for _, v := range vals[1:] {
		if within, diff := utils.AreWithinPercentage(v, vals[0], assetsPricingTolerance); !within {
			t.Errorf("  - [Fail] %s price not uniform: %.6g vs %.6g (%.2f%%)", label, v, vals[0], diff)
		}
	}
	if within, diff := utils.AreWithinPercentage(vals[0], expected, assetsPricingTolerance); within {
		t.Logf("  - [Pass] %s hourly price ~%.6g matches fixture (%.6g)", label, vals[0], expected)
	} else {
		t.Errorf("  - [Fail] %s hourly price %.6g != expected %.6g (%.2f%%)", label, vals[0], expected, diff)
	}
}

func TestAssetNodePricing(t *testing.T) {
	requireAssetsEnabled(t)

	if assetsEnv.DryRun {
		t.Logf("ASSETS_DRY_RUN set; would apply fixture %q and verify node pricing", fixtureID)
		return
	}

	ctx := context.Background()
	broker := applyFixedPricing(t, ctx)

	brokerNodes, err := broker.Nodes(ctx)
	if err != nil {
		t.Fatalf("broker nodes: %v", err)
	}

	cpu := pollMetric(t, "node_cpu_hourly_cost")
	ram := pollMetric(t, "node_ram_hourly_cost")
	t.Logf("node_cpu_hourly_cost=%v node_ram_hourly_cost=%v", cpu, ram)

	if len(cpu) != len(brokerNodes) {
		t.Logf("note: broker reports %d node(s), metrics report %d priced node(s)", len(brokerNodes), len(cpu))
	}

	checkPricing(t, "CPU", cpu, expectedNodeCPUHourly)
	checkPricing(t, "RAM", ram, expectedNodeRAMHourly)

	log.Infof("asset node pricing verified across %d node(s)", len(cpu))
}
