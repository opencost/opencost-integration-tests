package restart

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/cluster"
	"github.com/opencost/opencost-integration-tests/pkg/env"
	"github.com/opencost/opencost-integration-tests/pkg/log"
	"github.com/opencost/opencost-integration-tests/pkg/utils"
)

var restartEnv = LoadRestartEnv()

func requireRestartEnabled(t *testing.T) {
	if !restartEnv.Enabled {
		t.Skip("RESTART_ENABLED not set; skipping restart recovery test")
	}
}

// TestRestartRecovery drives the full restart vertical through the ops-broker:
// trigger a rolling restart, wait for the deployment + pods to report ready,
// confirm the restarted process logged no panic, and verify the allocation API
// serves traffic again.
func TestRestartRecovery(t *testing.T) {
	requireRestartEnabled(t)

	if restartEnv.DryRun {
		t.Log("RESTART_DRY_RUN set; would restart OpenCost and verify recovery")
		return
	}
	if err := restartEnv.ValidateBrokerConfig(); err != nil {
		t.Skip(err.Error())
	}

	ctx := context.Background()
	broker, err := cluster.NewClientFromEnv()
	if err != nil {
		t.Skip(err.Error())
	}

	if err := broker.Healthz(ctx); err != nil {
		t.Fatalf("broker health check failed: %v", err)
	}

	// Capture a fixed historical window before the restart so we can prove
	// Prometheus history survived (the core of #78).
	window := fixedHistoricalWindow()
	beforeCost, beforeCount, err := aggregateAllocationTotalCost(window)
	if err != nil {
		t.Fatalf("baseline allocation query failed: %v", err)
	}
	log.Infof("baseline for window %s: totalCost=%.4f across %d allocation(s)", window, beforeCost, beforeCount)

	// Trigger the rolling restart.
	if err := broker.RestartOpenCost(ctx); err != nil {
		t.Fatalf("restart OpenCost: %v", err)
	}
	log.Infof("restart triggered; waiting for OpenCost to become ready")

	waitCtx, cancel := context.WithTimeout(ctx, defaultReadyTimeout)
	defer cancel()

	// Wait for the rollout (rollout status analogue).
	info, err := broker.WaitForDeploymentReady(waitCtx, restartEnv.OpenCostDeployment, restartEnv.OpenCostNamespace, defaultRetryInterval)
	if err != nil {
		t.Fatalf("OpenCost deployment did not become ready after restart: %v", err)
	}
	log.Infof("OpenCost deployment ready: %d/%d replicas", info.ReadyReplicas, info.DesiredReplicas)

	// Confirm the new pods report ready too.
	pods, err := broker.WaitForOpenCostReady(waitCtx, defaultRetryInterval)
	if err != nil {
		t.Fatalf("OpenCost pods not ready after restart: %v", err)
	}
	for _, p := range pods {
		t.Logf("pod %s phase=%s ready=%t restarts=%d", p.Name, p.Phase, p.Ready, p.RestartCount)
	}

	// The restarted process must not have crashed.
	lines, err := broker.Logs(ctx, cluster.LogsRequest{
		Namespace: restartEnv.OpenCostNamespace,
		Selector:  restartEnv.OpenCostSelector,
		TailLines: defaultLogTailLines,
	})
	if err != nil {
		t.Fatalf("read OpenCost logs: %v", err)
	}
	if panicLine := FindPanic(lines); panicLine != "" {
		t.Errorf("OpenCost log shows a crash after restart: %s", panicLine)
	}

	// The API must serve traffic again.
	status, body, err := getAllocationRaw()
	if err != nil {
		t.Fatalf("allocation request failed after restart: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("allocation status = %d after restart, want 200; body=%s", status, body)
	}

	// The same fixed historical window must report the same cost after the
	// restart — proving Prometheus history was not lost.
	afterCost, afterCount, err := aggregateAllocationTotalCost(window)
	if err != nil {
		t.Fatalf("post-restart allocation query failed: %v", err)
	}
	log.Infof("post-restart for window %s: totalCost=%.4f across %d allocation(s)", window, afterCost, afterCount)

	if within, diff := utils.AreWithinPercentage(afterCost, beforeCost, historyTolerance); within {
		t.Logf("  - [Pass] historical window cost preserved (~%.4f)", afterCost)
	} else {
		t.Errorf("  - [Fail] historical window cost changed across restart: before=%.4f after=%.4f (%.2f%%)", beforeCost, afterCost, diff)
	}
	if afterCount < beforeCount {
		t.Errorf("  - [Fail] historical window lost allocations across restart: before=%d after=%d", beforeCount, afterCount)
	}

	log.Infof("restart recovery verified")
}

// fixedHistoricalWindow returns the previous full UTC day as an RFC3339 window.
// It is in the past, so its data should be stable across a restart.
func fixedHistoricalWindow() string {
	end := time.Now().UTC().Truncate(time.Hour).Add(-24 * time.Hour)
	start := end.Add(-24 * time.Hour)
	return fmt.Sprintf("%s,%s", start.Format(time.RFC3339), end.Format(time.RFC3339))
}

// aggregateAllocationTotalCost sums totalCost (and counts allocations) across a
// namespace-aggregated, accumulated query for the given window.
func aggregateAllocationTotalCost(window string) (float64, int, error) {
	resp, err := api.NewAPI().GetAllocation(api.AllocationRequest{
		Window:     window,
		Aggregate:  "namespace",
		Accumulate: "true",
	})
	if err != nil {
		return 0, 0, err
	}
	total := 0.0
	count := 0
	for _, set := range resp.Data {
		for _, item := range set {
			total += item.TotalCost
			count++
		}
	}
	return total, count, nil
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
