package count

// Description - Checks if the aggregate of pods for each namespace is the same for a prometheus request
// and allocation API request

import (
	// "fmt"
	// "time"
	"testing"
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
)

func TestQueryAllocation(t *testing.T) {
	apiObj := api.NewAPI()

	testCases := []struct {
		name		string
		window      string
		aggregate   string
		accumulate  string
	}{
		{
			name: "Yesterday",
			window: "24h",
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
				Accumulate: tc.accumulate,
			})

			if err != nil {
				t.Fatalf("Error while calling Allocation API %v", err)
			}
			if apiResponse.Code != 200 {
				t.Errorf("API returned non-200 code")
			}

			// Prometheus Client
			client := prometheus.NewClient()
			metric := "kube_pod_container_status_running"
			// kube-state-metrics is another job type
			filters := map[string]string{
				"job": "opencost",
			}
			promResponse, err := client.RunPromQLQuery(metric, filters, tc.window)

			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}
			
			// Calculate Number of Pods per Aggregate for API Object
			var apiAggregateCount = make(map[string]int)
			apiAllocations := apiResponse.Data[0]

			for _, allocationResponeItem := range apiAllocations {
				podNamespace := allocationResponeItem.Properties.Namespace
				_, namespacePresent := apiAggregateCount[podNamespace]
				if !namespacePresent {
					apiAggregateCount[podNamespace] = 1
				} else {
					apiAggregateCount[podNamespace] += 1
				}
			}
            
			// Calculate Number of Pods per Aggregate for Prom Object
			var promAggregateCount = make(map[string]int)
			for _, metric := range promResponse.Data.Result {
				podNamespace := metric.Metric.Namespace
				_, namespacePresent := promAggregateCount[podNamespace]
				if !namespacePresent {
					promAggregateCount[podNamespace] = 1
				} else {
					promAggregateCount[podNamespace] += 1
				}
			}

			var largerMapCount map[string]int
			if len(promAggregateCount) > len(apiAggregateCount) {
				largerMapCount = promAggregateCount
			} else {
				largerMapCount = apiAggregateCount
			}

			for key, _ := range largerMapCount {
				apiNamespaceCount, apiNamespacePresent := apiAggregateCount[key]
				promNamespaceCount, promNamespacePresent := promAggregateCount[key]
				if apiNamespacePresent && promNamespacePresent {
					if apiNamespaceCount != promNamespaceCount {
						t.Errorf("Aggregate count mismatch for Namespace %s", key)
					} else {
						t.Logf("Aggregate count matches for Namespace %s", key)
					}
				} else {
					t.Errorf("Namespace %s availablility in prometheus %v, allocation API %v", key, promNamespaceCount, apiNamespaceCount)
				}
			}
		})
	}
}
