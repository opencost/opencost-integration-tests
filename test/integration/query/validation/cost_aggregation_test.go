package validation

import (
	"fmt"
	"math"
	"testing"
)

func TestCostAggregationConsistency(t *testing.T) {

	allocResp, err := FetchAllocationData(true)
	if err != nil {
		t.Fatalf("Failed to fetch allocation data: %v", err)
	}

	var inconsistentAllocations []string

	for _, allocSet := range allocResp.Data {

		for namespaceName, alloc := range allocSet {

			resourceSum := alloc.CPUCost + alloc.RAMCost + alloc.GPUCost +
				alloc.NetworkCost + alloc.LoadBalancerCost +
				alloc.PVCost + alloc.ExternalCost + alloc.SharedCost

			epsilon := 0.001
			diff := math.Abs(resourceSum - alloc.TotalCost)

			if diff > epsilon {
				inconsistentAllocations = append(inconsistentAllocations,
					fmt.Sprintf("%s: sum of resources (%.5f) ≠ totalCost (%.5f), diff: %.5f",
						namespaceName, resourceSum, alloc.TotalCost, diff))
			}
		}
	}

	if len(inconsistentAllocations) > 0 {
		t.Errorf("Found cost aggregation inconsistencies in %d allocations: %v",
			len(inconsistentAllocations), inconsistentAllocations)
	}
}
