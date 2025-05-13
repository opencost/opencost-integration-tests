package integration

import (
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/api"
)

// TestNegativeIdleCosts checks for negative costs in the idle allocation
// This test specifically targets the bug where idle allocation shows negative costs
func TestNegativeIdleCosts(t *testing.T) {
	// Create API client
	a := api.NewAPI()

	// Create allocation request
	req := api.AllocationRequest{
		Window:     "1d",
		Aggregate:  "namespace",
		Idle:       "true",
		Accumulate: "false",
	}

	resp, err := a.GetAllocation(req)
	if err != nil {
		t.Fatalf("Failed to get allocation data: %v", err)
	}

	if resp.Code != 200 {
		t.Fatalf("Expected status code 200, got %d", resp.Code)
	}

	// Check for negative costs in idle allocation
	for _, itemMap := range resp.Data {
		if idle, exists := itemMap["__idle__"]; exists {
			if idle.TotalCost < 0 {
				t.Errorf("Found negative total cost in idle allocation: %f", idle.TotalCost)
			}
			if idle.CPUCost < 0 {
				t.Errorf("Found negative CPU cost in idle allocation: %f", idle.CPUCost)
			}
			if idle.RAMCost < 0 {
				t.Errorf("Found negative RAM cost in idle allocation: %f", idle.RAMCost)
			}
			if idle.GPUCost < 0 {
				t.Errorf("Found negative GPU cost in idle allocation: %f", idle.GPUCost)
			}
		}
	}
}

// Additional test cases that could be considered:
//
// 1. Aggregation Level Tests:
//    - Test different aggregation levels (namespace, controller, pod, container)
//    - Verify idle costs are properly distributed across aggregation levels
//    - Check if total idle costs remain consistent across different aggregations
//
// 2. Cost Consistency Tests:
//    - Verify total cost equals sum of component costs (CPU + RAM + GPU)
//    - Check if resource hours are consistent with minutes
//    - Validate cost adjustments don't result in negative values
//
// 3. Edge Case Tests:
//    - Test with zero resources
//    - Test with maximum resource limits
//    - Test with unusual time windows or intervals
