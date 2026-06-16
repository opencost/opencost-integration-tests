package allocation

// Description - Verify that allocation total cost is conserved when the same
// window is queried using different aggregation levels.
//
// Implementation Details
// - Every request uses the same completed historical window and query parameters.
// - Idle cost is included separately and is not redistributed.
// - Each aggregation's total is calculated by summing every returned item's TotalCost.
// - Cluster aggregation is used as the comparison baseline.
// - Small differences can occur from floating-point accumulation and rounding.
//
// Passing Criteria
// - Cluster, node, namespace, pod, and container totals match within the
//   documented absolute-or-relative tolerance.
// - Failure output identifies the divergent aggregation and difference.

import (
	"math"
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/api"
)

const (
	aggregationAbsoluteTolerance = 0.01
	aggregationRelativeTolerance = 0.001
)

var aggregationLevels = []string{
	"cluster",
	"node",
	"namespace",
	"pod",
	"container",
}

func sumAllocationTotalCost(t *testing.T, response *api.AllocationResponse) float64 {
	t.Helper()

	total := 0.0
	itemCount := 0

	for _, allocationSet := range response.Data {
		for _, item := range allocationSet {
			total += item.TotalCost
			itemCount++
		}
	}

	if itemCount == 0 {
		t.Fatal("allocation response contained no items")
	}

	return total
}

func aggregationAbsoluteDiff(expected, actual float64) float64 {
	return math.Abs(expected - actual)
}

func aggregationRelativeDiff(expected, actual float64) float64 {
	denominator := math.Max(math.Abs(expected), math.Abs(actual))
	if denominator == 0 {
		return 0
	}

	return aggregationAbsoluteDiff(expected, actual) / denominator
}

func withinAggregationTolerance(expected, actual float64) bool {
	return aggregationAbsoluteDiff(expected, actual) <= aggregationAbsoluteTolerance ||
		aggregationRelativeDiff(expected, actual) <= aggregationRelativeTolerance
}

func fetchAggregationTotal(
	t *testing.T,
	apiClient *api.API,
	window string,
	aggregation string,
) float64 {
	t.Helper()

	allocationRequest := api.AllocationRequest{
		Window:      window,
		Aggregate:   aggregation,
		Accumulate:  "true",
		IncludeIdle: "true",
		ShareIdle:   "false",
	}

	allocationResponse, err := apiClient.GetAllocation(allocationRequest)
	if err != nil {
		t.Fatalf("Error while calling Allocation API for aggregation %s: %v", aggregation, err)
	}

	if allocationResponse.Code != 200 {
		t.Fatalf("API returned non-200 code for aggregation %s: %d", aggregation, allocationResponse.Code)
	}

	return sumAllocationTotalCost(t, allocationResponse)
}

func TestAllocationCostConservedAcrossAggregations(t *testing.T) {
	apiClient := api.NewAPI()

	// A completed historical window prevents totals from changing between the
	// sequential aggregation requests.
	window := "yesterday"

	aggregationTotals := make(map[string]float64, len(aggregationLevels))

	for _, aggregation := range aggregationLevels {
		total := fetchAggregationTotal(t, apiClient, window, aggregation)
		aggregationTotals[aggregation] = total
		t.Logf("aggregation=%s totalCost=%.6f", aggregation, total)
	}

	clusterTotal, ok := aggregationTotals["cluster"]
	if !ok {
		t.Fatal("cluster aggregation total not found")
	}

	for aggregation, total := range aggregationTotals {
		if aggregation == "cluster" {
			continue
		}

		if !withinAggregationTolerance(clusterTotal, total) {
			t.Errorf(
				"aggregation=%s diverged from cluster: cluster=%.6f %s=%.6f diff=%.6f relativeDiff=%.4f%%",
				aggregation,
				clusterTotal,
				aggregation,
				total,
				aggregationAbsoluteDiff(clusterTotal, total),
				aggregationRelativeDiff(clusterTotal, total)*100,
			)
		}
	}
}