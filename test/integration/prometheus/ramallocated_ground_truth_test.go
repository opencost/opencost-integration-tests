package prometheus

// Description - Checks for Ram Byte Hours from prometheus and /allocation are the same

import (
	// "fmt"
	// "time"
	"testing"
	"strconv"
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
)

func TestRAMByteHours(t *testing.T) {
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
			aggregate: "namespace",
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
			promInput := prometheus.PrometheusInput{
				Metric: "container_memory_allocation_bytes",
			}
			//Get CPU Core hours from each namespace
			for namespace, allocationResponseItem := range apiResponse.Data[0] {

				////////////////////////////////////////////////////////////////////////////
				// RAMByteAllocated Calculation
				promNamespaceRAMByteAllocated := 0.0
				// Collect Namespace results from Prometheus
				filters := map[string]string{
					"job": "opencost",
					"namespace": namespace,
					"unit": "byte",
				}
				ignoreFilters := map[string][]string{
					"container": {"", "POD"},
					"node": {""},
				}
				promInput.Filters = filters
				promInput.Function = "avg_over_time"
				promInput.Window = tc.window
				promInput.IgnoreFilters = ignoreFilters
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
						promNamespaceRAMByteAllocated += floatVal
					}
				}

				////////////////////////////////////////////////////////////////////////////
				// RAMByteRequestedAverage
				promNamespaceRAMByteRequested := 0.0
				filters = map[string]string{
					"resource": "memory",
					"job": "opencost",
					"namespace": namespace,
				}
				promInput.Metric = "kube_pod_container_resource_requests"
				promInput.Filters = filters
				// RAMByte Comparison did not work with "avg_over_time(kube_pod_container_resource_requests[24h])"
				promInput.Function = "avg_over_time"
				promInput.Window = tc.window
				promInput.IgnoreFilters = ignoreFilters
				promResponse, err = client.RunPromQLQuery(promInput)
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
						promNamespaceRAMByteRequested += floatVal
					}
				}
				// https://github.com/opencost/opencost/blob/develop/core/pkg/opencost/allocation.go#L1175C1-L1182C1
				// Looks like the runtime of the pod is calculated based on the window
				// How do I take into account time when the pod was down, from the looks of the above snippet, I don't think opencost math takes that into consideration
				// This needs to be dynamically updated based on the testcase window, hardcording it as there is only one test case
				podRuntime := 1440 / 60
				promNamespaceRAMByteHours := 0.0
				if promNamespaceRAMByteAllocated < promNamespaceRAMByteRequested {
					promNamespaceRAMByteHours = promNamespaceRAMByteRequested * float64(podRuntime)
				} else {
					promNamespaceRAMByteHours = promNamespaceRAMByteAllocated * float64(podRuntime)
				}
				t.Logf("%v %v", promNamespaceRAMByteAllocated, promNamespaceRAMByteRequested)
				
				if promNamespaceRAMByteHours != allocationResponseItem.RAMByteHours {
					t.Errorf("RAM Byte Hours Sum does not match for prometheus %f and /allocation %f for namespace %s", promNamespaceRAMByteHours, allocationResponseItem.RAMByteHours, namespace)	
				} else {
					t.Logf("RAM Byte Hours Match for namespace %s", namespace)	
				}
			}
		})
	}
}


// Get RamBytes as the allocated Bytes - Done
// Get ramBytesRequestAverage - Done
// update ramBytes to requestAverage it is smaller - Done
// Get PoD Runtime
// multiply ramBytes * podRuntime / 60 to get RamByteHours