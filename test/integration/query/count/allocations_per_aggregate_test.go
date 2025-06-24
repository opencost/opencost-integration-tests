package count

// Description - Checks for the aggregate count of pods for each namespace from prometheus request
// and allocation API request are the same

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
			promInput = prometheus.PrometheusInput{
				Metric: metric
				Filters: filters
				Window: tc.window
			}
			promResponse, err := client.RunPromQLQuery(promInput)

			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}
			
			// Calculate Number of Pods per Aggregate for API Object
			type podAggregation struct  {
				Pods []string
				NumberOfPods int
			}
			var apiAggregateCount = make(map[string]podAggregation)
			apiAllocations := apiResponse.Data[0]
			for _, allocationResponeItem := range apiAllocations {
				podNamespace := allocationResponeItem.Properties.Namespace
				apiAggregateItem, namespacePresent := apiAggregateCount[podNamespace]
				if !namespacePresent {
					apiAggregateItem.NumberOfPods = 1
				} else {
					apiAggregateItem.NumberOfPods += 1
				}
				apiAggregateItem.Pods = append(apiAggregateItem.Pods, allocationResponeItem.Name)
				apiAggregateCount[podNamespace] = apiAggregateItem
			}
            
			// Calculate Number of Pods per Aggregate for Prom Object
			var promAggregateCount = make(map[string]podAggregation)
			for _, metric := range promResponse.Data.Result {
				podNamespace := metric.Metric.Namespace
				promAggregateItem, namespacePresent := promAggregateCount[podNamespace]
				if !namespacePresent {
					promAggregateItem.NumberOfPods = 1
				} else {
					promAggregateItem.NumberOfPods += 1
				}
				promAggregateItem.Pods = append(promAggregateItem.Pods, metric.Metric.Pod)
				promAggregateCount[podNamespace] = promAggregateItem
			}

			var largerMapCount map[string]podAggregation
			if len(promAggregateCount) > len(apiAggregateCount) {
				largerMapCount = promAggregateCount
			} else {
				largerMapCount = apiAggregateCount
			}
			for key, _ := range largerMapCount {
				apiNamespaceCount, apiNamespacePresent := apiAggregateCount[key]
				promNamespaceCount, promNamespacePresent := promAggregateCount[key]
				if apiNamespacePresent && promNamespacePresent {
					if apiNamespaceCount.NumberOfPods != promNamespaceCount.NumberOfPods {
						t.Errorf("Aggregate count from API != Prometheus mismatch for Namespace %s\n%v (%d)\n%v (%d)", key, apiNamespaceCount.Pods, apiNamespaceCount.NumberOfPods, promNamespaceCount.Pods, promNamespaceCount.NumberOfPods)

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
