package integration

import (
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/api"
)

const (
	defaultWindow    = "1d"
	defaultAggregate = "namespace"
)

// TestNegativeIdleCosts checks for negative costs in the idle allocation
// This test specifically targets the bug where idle allocation shows negative costs
func TestNegativeIdleCosts(t *testing.T) {

	testCases := []struct {
		name      string
		window    string
		aggregate string
		idle      string
	}{
		{
			name:      "Daily by namespace",
			window:    defaultWindow,
			aggregate: defaultAggregate,
			idle:      "true",
		},
		{
			name:      "Weekly by cluster",
			window:    "7d",
			aggregate: "cluster",
			idle:      "true",
		},
		{
			name:      "Monthly by namespace",
			window:    "30d",
			aggregate: defaultAggregate,
			idle:      "true",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a := api.NewAPI()

			req := api.AllocationRequest{
				Window:    tc.window,
				Aggregate: tc.aggregate,
				Idle:      tc.idle,
			}

			resp, err := a.GetAllocation(req)
			if err != nil {
				t.Fatalf("Failed to get allocation data: %v", err)
			}

			if resp.Code != 200 {
				t.Fatalf("Expected status code 200, got %d", resp.Code)
			}

			if len(resp.Data) == 0 {
				t.Skip("No allocation data returned, skipping verification")
			}

			idleFound := false
			// Check for negative costs in idle allocation
			for _, itemMap := range resp.Data {
				if idle, exists := itemMap["__idle__"]; exists {
					idleFound = true
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

			if !idleFound {
				t.Skip("No idle allocation found in the response, skipping verification")
			}
		})
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
