package prometheus

// Description: Checks all RAM related Costs
// - RAMBytes
// - RAMByteHours
// - RAMMaxUsage
// Also processes RAMByteRequestAverage and RAMByteAllocated for RAMByteHours calculation.

// Testing Methodology
// 1. Query RAMAllocated and RAMRequested in prometheus
//     1.1 Use the current "time" as upperbound in promql
//     1.2 Query by (container, pod, namespace)
// 2. Consolidate containers based on pods
//     2.1 RAMByte is the max of RAMByteAllocated and RAMByteRequested
//     2.2 Query [24h:5m] to get 288 (1440/5) points. 
// 	   2.3 Assumption 1: Identify the time range for the pod (all containers within the pod have the same time range)
// 3. Consolidate RAMBytes based on pod and then based on namespace
// 4. Fetch /allocation data aggregated by namespace
// 5. Compare results with a 2% error margin.


import (
	// "fmt"
	// "time"
	"testing"
	"strconv"
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
)

func TestRAMByteCosts(t *testing.T) {
	apiObj := api.NewAPI()

	// test for more windows
	testCases := []struct {
		name		string
		window      string
		aggregate   string
		accumulate  string
	}{
		{
			name: "Yesterday",
			window: "24h",
			aggregate: "namespace",
			accumulate: "false",
		},
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			// ----------------------------------------------
			// Allocation API Data Collection
			// ----------------------------------------------
			// /compute/allocation
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

			// ----------------------------------------------
			// Prometheus Data Collection
			// ----------------------------------------------
			client := prometheus.NewClient()

			// Get RAM costs from each namespace
			for namespace, allocationResponseItem := range apiResponse.Data[0] {
				// Namespace Sum Count
				// promNamespaceRAMByte := 0.0

				// Query to construct
				// avg(avg_over_time(
				// 		kube_pod_container_resource_requests{
				// 			resource="memory", unit="byte", container!="", container!="POD", node!=""
				// 		}[24h])
				// )
				// by 
				// (container, pod, namespace, node) 
				
				// Q) What about Cluster Filter and Cluster Label?
				promInput := prometheus.PrometheusInput{}

				metric := "kube_pod_container_resource_requests",
				filters := map[string]string{
					// "job": "opencost", Averaging all results negates this process
					"resource": "memory",
					"unit": "byte",
					"namespace": namespace,
				}
				ignoreFilters := map[string][]string{
					"container": {"", "POD"},
					"node": {""},
				}
				aggregateBy := []string{"container", "pod", "namespace", "node"}

				promInput.Metric = metric
				promInput.Filters = filters
				promInput.Function = []string{"avg_over_time", "avg"}
				promInput.Window = tc.window
				promInput.IgnoreFilters = ignoreFilters
				promInput.AggregateBy = aggregateBy

				promResponse, err := client.RunPromQLQuery(promInput)

				if err != nil {
					t.Fatalf("Error while calling Prometheus API %v", err)
				}

				for _, promResponseItem := range promResponse.Data.Result {
					valueStr, _ := promResponseItem.Value[1].(string)
					floatVal, err := strconv.ParseFloat(valueStr, 64)
					if err != nil {
						t.Errorf("Error parsing metric value '%s' to float64 for pod %s, namespace %s: %v",
							valueStr, promResponseItem.Metric.Pod, namespace, err)
						// Decide how to handle this error (e.g., continue, log, set to 0)
					} else {
						promNamespaceRAMByte += floatVal
					}
				}
				// for _, promResponseItem := range promResponse.Data.Result {
				// 	// t.Logf("%v", promResponseItem.Values[0])
				// 	for _, promRAMNodeValue := range promResponseItem.Values {
				// 		// Convert the string value to float64
				// 		valueArray, _ := promRAMNodeValue.([]interface{})
				// 		valueStr, _ := valueArray[1].(string)
				// 		floatVal, err := strconv.ParseFloat(valueStr, 64)
				// 		if err != nil {
				// 			t.Errorf("Error parsing metric value '%s' to float64 for pod %s, namespace %s: %v",
				// 				valueStr, promResponseItem.Metric.Pod, namespace, err)
				// 			// Decide how to handle this error (e.g., continue, log, set to 0)
				// 		} else {
				// 			promNamespaceRAMByte += floatVal
				// 		}
				// 	}
				// }
				// RamByte is the average value
				if promNamespaceRAMByte != allocationResponseItem.RAMBytes {
					t.Errorf("RAM Byte Hours Sum does not match for prometheus %f and /allocation %f for namespace %s", promNamespaceRAMByte, allocationResponseItem.RAMBytes, namespace)	
				} else {
					t.Logf("RAM Byte Hours Match for namespace %s", namespace)	
				}
			}
		})
	}
}
