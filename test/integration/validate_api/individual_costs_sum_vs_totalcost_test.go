package validate_api

// Description - Assert AllocationResponseItem.TotalCost is equal to the sum of Resource Costs like CPUCost, GPUCost, etc.
// Specification - https://opencost.io/docs/specification#foundational-definitions
// Formula - Total Cluster Costs = Workload Costs + Cluster Idle Costs + Cluster Overhead Costs

// Implementation Details
// - Cluster Overhead costs are not available to us, so we skip it.
// - Only Testing the sum of the "Workload costs"

// Passing Criteria
// AllocationResponseItem.TotalCost and "CalculatedCost", when rounded to first two decimal places, must exhibit a variance not exceeding 0.5.

import (
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"math"
	"testing"
)

func RoundUpToTwoDecimals(num float64) float64 {

	temp := num * 100
	roundedTemp := math.Round(temp)
	return roundedTemp / 100
}

// Checks relevant cost fields in an AllocationResponseItem for negative values
func addIndividualCosts(m api.AllocationResponseItem) (float64, float64) {

	calculated_totalCost := 0.0
	// Add Resource Usage Costs for item
	calculated_totalCost += m.CPUCost + m.GPUCost + m.RAMCost + m.NetworkCost + m.LoadBalancerCost
	for _, persistentVolume := range m.PersistentVolumes {
		calculated_totalCost += persistentVolume.Cost
	}
	return RoundUpToTwoDecimals(calculated_totalCost), RoundUpToTwoDecimals(m.TotalCost)
}
func TestSumofCostsnTotalCosts(t *testing.T) {
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
			accumulate:  "false",
			includeidle: "true",
		},
		{
			name:        "Yesterday",
			window:      "yesterday",
			aggregate:   "cluster",
			accumulate:  "false",
			includeidle: "true",
		},
		// {
		// 	name: "Last week",
		// 	window: "week",
		// 	aggregate: "container",
		// 	accumulate: "false",
		// 	includeidle: "true",
		// },
		// {
		// 	name: "Last 14 days",
		// 	window: "14d",
		// 	aggregate: "service",
		// 	accumulate: "false",
		// 	includeidle: "true",
		// },
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

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
			for i, allocationRequestObjMap := range response.Data {
				t.Logf("Response Data Step Index %d:\n", i+1)
				// Check for any negative values in responseObj
				for mapkey, allocationRequestObj := range allocationRequestObjMap {
					t.Logf("Name: %v\n", mapkey)
					calculated_totalCost, totalCost := addIndividualCosts(allocationRequestObj)

					// Compute Error Margin
					acceptableErrorMargin := 0.5
					errorMargin := math.Abs(totalCost-calculated_totalCost) / totalCost
					if errorMargin > acceptableErrorMargin {
						t.Errorf("Resource Costs: %v", allocationRequestObj)
						t.Errorf("Total Cost: %v did not match computed cost: %v", totalCost, calculated_totalCost)
					} else {
						t.Logf("%v passed", mapkey)
					}

				}
			}
		})
	}
}
