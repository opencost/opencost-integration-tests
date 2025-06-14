package count

// Description - Asserts Pod Results from /allocation match /allocation/summary

import (
	// "fmt"
	// "time"
	"testing"
	"strings"
	"github.com/opencost/opencost-integration-tests/pkg/api"
)

func assertPodInformation(apiSummary api.AllocationResponseItemProperties, apiCompute *api.AllocationResponseItemProperties) (bool) {
	status := true
	if apiSummary.Cluster != apiCompute.Cluster {
		status = false
	}
	if apiSummary.Container != apiCompute.Container {
		status = false
	}
	if apiSummary.Namespace != apiCompute.Namespace {
		status = false
	}
	if apiSummary.Node != apiCompute.Node {
		status = false
	}
	return status
}
func TestAllocationAndAllocationSummary(t *testing.T) {
	apiObj := api.NewAPI()

	testCases := []struct {
		name		string
		window      string
		aggregate   string
		accumulate  string
	}{
		{
			name: "Yesterday",
			window: "1d",
			aggregate: "pod",
			accumulate: "false",
		},
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			// API Client
			apiResponse, err := apiObj.GetAllocation(api.AllocationRequest{
				Window: tc.window,
				Aggregate: tc.aggregate,
			})

			if err != nil {
				t.Fatalf("Error while calling Allocation API %v", err)
			}
			if apiResponse.Code != 200 {
				t.Errorf("API returned non-200 code")
			}

			apiSummaryResponse, err := apiObj.GetAllocationSummary(api.AllocationRequest{
				Window: tc.window,
				Aggregate: "pods", // Summary API is not working as expected
			})

			if err != nil {
				t.Fatalf("Error while calling Allocation API %v", err)
			}
			if apiSummaryResponse.Code != 200 {
				t.Errorf("API returned non-200 code")
			}
			
			// Extract Key Information from Summary
			var allPods map[string]api.AllocationResponseItemProperties = make(map[string]api.AllocationResponseItemProperties)
			for key, _ := range apiSummaryResponse.Data.Sets[0].Allocations {
				parts := strings.Split(key, "/")
				if len(parts) == 5 {
					cluster := parts[0]
					node := parts[1]
					namespace := parts[2]
					pod := parts[3]
					container := parts[4]
					allPods[pod] = api.AllocationResponseItemProperties{
						Cluster: cluster,
						Node: node,
						Namespace: namespace,
						Container: container,
					}
				}
			}
			// Ignoring step parameter
			for pod, allocationResponseItem := range apiResponse.Data[0] {
				allocationSummary4Pod, podPresent := allPods[pod]
				if !podPresent {
					t.Errorf("%s missing from /allocation", pod)
					continue
				}
				status := assertPodInformation(allocationSummary4Pod, allocationResponseItem.Properties)
				if status != true {
					t.Errorf("/allocation/summary and /allocation do not match for %s", pod)
				}
			}
		})
	}
}
