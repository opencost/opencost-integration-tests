// test/integration/query/validation/cost_aggregation_test.go
package validation

import (
	"fmt"
	"math"
	"testing"
)

// TestCostAggregationConsistency verifies that the sum of individual resource costs equals the total cost
func TestCostAggregationConsistency(t *testing.T) {
	// Fetch allocation data with idle included
	allocResp, err := FetchAllocationData(true)
	if err != nil {
		t.Fatalf("Failed to fetch allocation data: %v", err)
	}

	// Check for cost aggregation consistency
	var inconsistentAllocations []string

	// Iterate through each allocation set in the data array
	for _, allocSet := range allocResp.Data {
		// Check each allocation entry
		for namespaceName, alloc := range allocSet {
			// Calculate the sum of individual resource costs
			resourceSum := alloc.CPUCost + alloc.RAMCost + alloc.GPUCost +
				alloc.NetworkCost + alloc.LoadBalancerCost +
				alloc.PVCost + alloc.ExternalCost + alloc.SharedCost

			// Compare with totalCost (allowing small epsilon for floating point comparison)
			epsilon := 0.001 // Small value to account for floating point imprecision
			diff := math.Abs(resourceSum - alloc.TotalCost)

			if diff > epsilon {
				inconsistentAllocations = append(inconsistentAllocations,
					fmt.Sprintf("%s: sum of resources (%.5f) ≠ totalCost (%.5f), diff: %.5f",
						namespaceName, resourceSum, alloc.TotalCost, diff))
			}
		}
	}

	// Fail the test if inconsistent allocations were found
	if len(inconsistentAllocations) > 0 {
		t.Errorf("Found cost aggregation inconsistencies in %d allocations: %v",
			len(inconsistentAllocations), inconsistentAllocations)
	}
}
