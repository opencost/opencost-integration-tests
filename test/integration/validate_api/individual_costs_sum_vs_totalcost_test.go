package validate_api

// Description
// Validate TotalCost is equal to sum of individual resource costs
// In the demo, ingress-nginx resource costs do not add up to totalcost (0.04 + 0.0 + 0.0 + 0.0 != 0.22)

// Testing Strategy
// Testing opencost specification - https://opencost.io/docs/specification#foundational-definitions
// This test assumes based on the dasboard that 4 resources contribute to the totalcost, although they could be more
// Resources considered include RAM, GPU, CPU, PersistentVolume (tricky because this is a map of slices)

// Formula: Total Cluster Costs	= Workload Costs + Cluster Idle Costs +	Cluster Overhead Costs
// https://github.com/opencost/opencost/blob/develop/core/pkg/opencost/allocation.go#L897
// Cluster Overhead costs are not available to us, so we skip it.
// Based on my understanding of the documentation we are actually testing for sum of the "Workload costs" only

// Pass Criteria
// If the rounded value upto 2 decimal values of TotolCost and CalculatedCost match. (We may also consider an accepted error percentage = +- 1%)


import (
	"math"
	"testing"
	"github.com/opencost/opencost-integration-tests/pkg/api"
)

func RoundUpToTwoDecimals(num float64) float64 {

	temp := num * 100
	roundedTemp := math.Round(temp)
	return roundedTemp / 100
}

// Checks relevant cost fields in an AllocationResponseItem for negative values
func addIndividualCosts(m api.AllocationResponseItem) (float64, float64){

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
		{
			name: "Yesterday",
			window: "yesterday",
			aggregate: "cluster",
			accumulate: "false",
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
				for mapkey, allocationRequestObj := range allocationRequestObjMap {
					t.Logf("Name: %v\n", mapkey)
					calculated_totalCost, totalCost := addIndividualCosts(allocationRequestObj)

					// Compare and check
					if calculated_totalCost != totalCost { // Consider an accepted_error parameter that allows a difference = +- 1%
						t.Errorf("Total Cost: %v did not match computed cost: %v", totalCost, calculated_totalCost)
					} else {
						t.Logf("%v passed", mapkey)
					}

			}
		}
		})
	}
}
