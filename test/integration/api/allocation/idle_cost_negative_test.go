package allocation

// Tests AllocationAPI return for negative values in idle costs
// This test primarily focuses on the __idle__ namespace (when the breakdown is grouped by namespace)
// idle costs - https://opencost.io/docs/specification#idle-costs by definition shouldn't be negative

// Test Cases
// Check all Resouce Usage Costs such as RAM, GPU, PersisitentVolume, LoadBalancer, etc
// Check for different time windows and breakdown
// Only __idle__ is tested here and not the resources (like ingress-nginx or folding-at-home)

// Pass Criteria
// No idle cost for any resource type is negative

import (
	"fmt"
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"testing"
	"time"
)

// Checks relevant cost fields in an AllocationResponseItem for negative values
func checkNegativeCosts(m api.AllocationResponseItem) (bool, []string, []float64) { // Added []float64 to return types
	isNegative := false       // Flag to track if any negative value is found
	var negativeFields []string // To store names of negative fields
	var negativeCosts []float64 // To store the negative values (already declared as float64)

	// Check each field individually
	if m.CPUCost < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "CPUCost")
		negativeCosts = append(negativeCosts, m.CPUCost) // Append the actual value
	}
	if m.GPUCost < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "GPUCost")
		negativeCosts = append(negativeCosts, m.GPUCost) // Append the actual value
	}
	if m.RAMCost < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "RAMCost")
		negativeCosts = append(negativeCosts, m.RAMCost) // Append the actual value
	}
	if m.NetworkCost < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "NetworkCost")
		negativeCosts = append(negativeCosts, m.NetworkCost) // Append the actual value
	}
	if m.LoadBalancerCost < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "LoadBalancerCost")
		negativeCosts = append(negativeCosts, m.LoadBalancerCost) // Append the actual value
	}

	// Check Idle-Cost fields
	if m.CPUCostIdle < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "CPUCostIdle")
		negativeCosts = append(negativeCosts, m.CPUCostIdle) // Append the actual value
	}
	if m.GPUCostIdle < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "GPUCostIdle")
		negativeCosts = append(negativeCosts, m.GPUCostIdle) // Append the actual value
	}
	if m.RAMCostIdle < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "RAMCostIdle")
		negativeCosts = append(negativeCosts, m.RAMCostIdle) // Append the actual value
	}

	// Check adjustment cost fields
	if m.CPUCostAdjustment < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "CPUCostAdjustment")
		negativeCosts = append(negativeCosts, m.CPUCostAdjustment) // Append the actual value
	}
	if m.GPUCostAdjustment < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "GPUCostAdjustment")
		negativeCosts = append(negativeCosts, m.GPUCostAdjustment) // Append the actual value
	}
	if m.NetworkCostAdjustment < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "NetworkCostAdjustment")
		negativeCosts = append(negativeCosts, m.NetworkCostAdjustment) // Append the actual value
	}
	if m.LoadBalancerCostAdjustment < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "LoadBalancerCostAdjustment")
		negativeCosts = append(negativeCosts, m.LoadBalancerCostAdjustment) // Append the actual value
	}

	return isNegative, negativeFields, negativeCosts // Return the new slice
}

func TestNegativeIdleCosts(t *testing.T) {
	apiObj := api.NewAPI()

	testCases := []struct {
		name        string
		window      string
		aggregate   string
		accumulate  string
		includeidle string
	}{
		{
			name:        "Today",
			window:      "today",
			aggregate:   "namespace",
			accumulate:  "true",
			includeidle: "true",
		},
		{
			name:        "Yesterday",
			window:      "yesterday",
			aggregate:   "node",
			accumulate:  "true",
			includeidle: "true",
		},
		{
			name:        "Last week",
			window:      "week",
			aggregate:   "service",
			accumulate:  "true",
			includeidle: "true",
		},
		{
			name:        "Last 14 days",
			window:      "14d",
			aggregate:   "pod",
			accumulate:  "true",
			includeidle: "true",
		},
		{
			name:        "Custom",
			window:      "%sT00:00:00Z,%sT00:00:00Z", // This can be generated dynamically based on the running time
			aggregate:   "namespace",
			accumulate:  "true",
			includeidle: "true",
		},
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			if tc.name == "Custom" {
				// Dynamically generate the "Custom" window
				now := time.Now()
				tc.window = fmt.Sprintf(tc.window,
					now.AddDate(0, 0, -2).Format("2006-01-02"),
					now.AddDate(0, 0, -1).Format("2006-01-02"),
				)
			}

			response, err := apiObj.GetAllocation(api.AllocationRequest{
				Window:      tc.window,
				Aggregate:   tc.aggregate,
				Accumulate:  tc.accumulate,
				IncludeIdle: tc.includeidle,
			})

			if err != nil {
				t.Fatalf("Error while calling Allocation API %v", err)
			}
			if response.Code != 200 {
				t.Errorf("API returned non-200 code")
			}
			t.Logf("Breakdown %v", tc.aggregate)
			for _, allocationRequestObjMap := range response.Data {
				// Check for any negative values in responseObj
				idleItem, idlepresent := allocationRequestObjMap["__idle__"]
				if !idlepresent {
					t.Errorf("__idle__ key is missing")
					continue
				}
				isNegative, negativeFields, negativeCosts := checkNegativeCosts(idleItem)

				if isNegative == true {
					t.Errorf("Found Negative Idle Value(s) %v: %v", negativeFields, negativeCosts)
				} else {
					t.Logf("No Negative Idle Values")
				}
			}
		})
	}
}
