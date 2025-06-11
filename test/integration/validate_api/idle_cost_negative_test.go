package validate_api

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
	"time"
	"testing"
	"github.com/opencost/opencost-integration-tests/pkg/api"
)

// Checks relevant cost fields in an AllocationResponseItem for negative values
func checkNegativeCosts(m api.AllocationResponseItem) (bool, []string){

	isNegative := false // Flag to track if any negative value is found
	var negativeFields []string // To store names of negative fields

	// Check each field individually
	if m.CPUCost < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "CPUCost")
	}
	if m.GPUCost < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "GPUCost")
	}
	if m.RAMCost < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "RAMCost")
	}
	if m.NetworkCost < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "NetworkCost")
	}
	if m.LoadBalancerCost < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "LoadBalancerCost")
	}

	// Check Idle-Cost fields
	if m.CPUCostIdle < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "CPUCostIdle")
	}
	if m.GPUCostIdle < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "GPUCostIdle")
	}
	if m.RAMCostIdle < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "RAMCostIdle")
	}

	// Check adjustment cost fields
	if m.CPUCostAdjustment < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "CPUCostAdjustment")
	}
	if m.GPUCostAdjustment < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "GPUCostAdjustment")
	}
	if m.NetworkCostAdjustment < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "NetworkCostAdjustment")
	}
	if m.LoadBalancerCostAdjustment < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "LoadBalancerCostAdjustment")
	}

	return isNegative, negativeFields
}

func TestNegativeIdleCosts(t *testing.T) {
	apiObj := api.NewAPI()

	testCases := []struct {
		name		string
		window      string
		aggregate   string
		accumulate  string
		includeidle string
	}{
		{
			name: "Today",
			window: "today",
			aggregate: "namespace",
			accumulate: "false",
			includeidle: "true",
		},
		{ // This test is meant to fail because there is no includeidle field, i.e no __idle__
			name: "Missing includeIdle",
			window: "today",
			aggregate: "namespace",
			accumulate: "false",
		},
		{
			name: "Yesterday",
			window: "yesterday",
			aggregate: "node",
			accumulate: "false",
			includeidle: "true",
		},
		// {
		// 	name: "Last week",
		// 	window: "week",
		// 	aggregate: "service",
		// 	accumulate: "false",
		// 	includeidle: "true",
		// },
		// {
		// 	name: "Last 14 days",
		// 	window: "14d",
		// 	aggregate: "pod",
		// 	accumulate: "false",
		// 	includeidle: "true",
		// },
		// {
		// 	name: "Custom",
		// 	window: "%sT00:00:00Z,%sT00:00:00Z", // This can be generated dynamically based on the running time
		// 	aggregate: "namespace",
		// 	accumulate: "false",
		// 	includeidle: "true",
		// },
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			// if tc.name == "Custom" {
			// 	// Dynamically generate the "Custom" window
			// 	now := time.Now()
			// 	tc.window = fmt.Sprintf(tc.window,
			// 		now.AddDate(0, 0, -2).Format("2006-01-02"),
			// 		now.AddDate(0, 0, -1).Format("2006-01-02"),
			// 	)
			// }

			response, err := apiObj.GetAllocation(api.AllocationRequest{
				Window: tc.window,
				Aggregate: tc.aggregate,
				Accumulate: tc.accumulate,
				IncludeIdle: tc.includeidle,
			})

			if err != nil {
				t.Fatalf("Error while calling Allocation API %v", err)
			}
			if response.Code != 200 {
				t.Errorf("API returned non-200 code")
			}
			t.Logf("Breakdown %v", tc.aggregate)
			for i, allocationRequestObjMap := range response.Data {
				t.Logf("Response Data Step Index %d:\n", i+1)
				// Check for any negative values in responseObj
				idleItem, idlepresent := allocationRequestObjMap["__idle__"]
				if !idlepresent {
					t.Errorf("__idle__ key is missing")
					continue
				}
				isNegative, negativeFields := checkNegativeCosts(idleItem)

				if isNegative == true {
					t.Errorf("Found Negative Value(s) %v", negativeFields)
				} else {
					t.Logf("No Negative Values")
				}
			}
		})
	}
}
