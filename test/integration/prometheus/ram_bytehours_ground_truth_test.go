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
				Window: "1d",
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
				Metric: "kube_pod_container_resource_requests",
			}
			//Get CPU Core hours from each namespace
			for namespace, allocationResponseItem := range apiResponse.Data[0] {
				// Namespace Sum Count
				promNamespaceRAMByte := 0.0
				// Collect Namespace results from Prometheus
				filters := map[string]string{
					"job": "opencost",
					"resource": "memory",
					"namespace": namespace,
				}
				promInput.Filters = filters
				promInput.Function = "avg_over_time"
				promInput.Window = tc.window
				promResponse, err := client.RunPromQLQuery(promInput)

				if err != nil {
					t.Fatalf("Error while calling Prometheus API %v", err)
				}
				// Add all post CPUHours
				for _, promResponseItem := range promResponse.Data.Result {
					valueStr, _ := promResponseItem.Values[1].(string)
					// Convert the string value to float64
					floatVal, err := strconv.ParseFloat(valueStr, 64)
					if err != nil {
						t.Errorf("Error parsing metric value '%s' to float64 for pod %s, namespace %s: %v",
							valueStr, promResponseItem.Metric.Pod, namespace, err)
						// Decide how to handle this error (e.g., continue, log, set to 0)
					} else {
						promNamespaceRAMByte += floatVal
					}
				}
				// RamByte is the average value
				if promNamespaceRAMByte != allocationResponseItem.RAMByteHours {
					t.Errorf("RAM Byte Hours Sum does not match for prometheus %f and /allocation %f for namespace %s", promNamespaceRAMByte, allocationResponseItem.RAMBytehours, namespace)	
				} else {
					t.Logf("RAM Byte Hours Match for namespace %s", namespace)	
				}
			}
		})
	}
}
