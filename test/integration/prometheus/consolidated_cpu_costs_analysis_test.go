package prometheus

// Description - Checks for CPU core hours from prometheus and /allocation are the same

import (
	// "fmt"
	// "time"
	"testing"
	"strconv"
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
)

func TestCpuAllocation(t *testing.T) {
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
				Metric: "container_cpu_allocation",
			}

			//Get CPU Core hours from each namespace
			for namespace, allocationResponseItem := range apiResponse.Data[0] {
				// Namespace Sum Count
				promNamespaceCPUSum := 0.0
				// Collect Namespace results from Prometheus
				filters := map[string]string{
					"job": "opencost",
					"namespace": namespace, 
				}
				promInput.Filters = filters
				// promInput.Function = "avg_over_time"
				promInput.Window = tc.window
				promResponse, err := client.RunPromQLQuery(promInput)

				if err != nil {
					t.Fatalf("Error while calling Prometheus API %v", err)
				}
				// Add all post CPUHours
				for _, promResponseItem := range promResponse.Data.Result {
					for _, promCPUNodeValue := range promResponseItem.Values {
						valueArray, _ := promCPUNodeValue.([]interface{})
						valueStr, _ := valueArray[1].(string)
						// Convert the string value to float64
						floatVal, err := strconv.ParseFloat(valueStr, 64)
						if err != nil {
							t.Errorf("Error parsing metric value '%s' to float64 for pod %s, namespace %s: %v",
								valueStr, promResponseItem.Metric.Pod, namespace, err)
							// Decide how to handle this error (e.g., continue, log, set to 0)
						} else {
							promNamespaceCPUSum += floatVal
						}
					}
				}
				if promNamespaceCPUSum != allocationResponseItem.CPUCoreHours {
					t.Errorf("CPU Core Hours Sum does not match for prometheus %f and /allocation %f for namespace %s", promNamespaceCPUSum, allocationResponseItem.CPUCoreHours, namespace)	
				} else {
					t.Logf("CPU Core Hours Match for namespace %s", namespace)	
				}
			}
		})
	}
}
