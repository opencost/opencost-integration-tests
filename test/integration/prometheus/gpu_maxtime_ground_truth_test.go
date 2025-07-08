package prometheus

// Description - Checks for Ram Byte Hours from prometheus and /allocation are the same

import (
	// "fmt"
	"time"
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
	"github.com/opencost/opencost-integration-tests/pkg/utils"
	"testing"
)

const tolerance = 0.05

func TestGPUMax(t *testing.T) {
	apiObj := api.NewAPI()

	testCases := []struct {
		name       string
		window     string
		aggregate  string
		accumulate string
	}{
		{
			name:       "Yesterday",
			window:     "24h",
			aggregate:  "container,pod",
			accumulate: "false",
		},
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			// Any data that is in a "raw allocation only" is not valid in any
			// sort of cumulative Allocation (like one that is added).

			type GPUUsageMaxAggregate struct {
				AllocationUsageMax float64
				PrometheusUsageMax float64
				Namespace          string
			}
			gpuUsageMaxPodMap := make(map[string]*GPUUsageMaxAggregate)

			////////////////////////////////////////////////////////////////////////////
			// GPUMaxUsage Calculation

			// max(max_over_time(DCGM_FI_PROF_GR_ENGINE_ACTIVE{container!=""}[%s])) by (container, pod, namespace, %s)`
			////////////////////////////////////////////////////////////////////////////

			queryEnd := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
			endTime := queryEnd.Unix()
			// Collect Namespace results from Prometheus
			client := prometheus.NewClient()
			promInput := prometheus.PrometheusInput{
				Metric: "DCGM_FI_PROF_GR_ENGINE_ACTIVE",
			}
			ignoreFilters := map[string][]string{
				"container": {""},
			}
			promInput.Function = []string{"max_over_time", "max"}
			promInput.QueryWindow = tc.window
			promInput.IgnoreFilters = ignoreFilters
			promInput.AggregateBy = []string{"container", "pod", "namespace"}
			promInput.Time = &endTime
			
			promResponse, err := client.RunPromQLQuery(promInput)
			// Do we need container_name and pod_name
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			for _, promResponseItem := range promResponse.Data.Result {
				if promResponseItem.Metric.Container == "" {
					continue
				}
				gpuUsageMaxPod, ok := gpuUsageMaxPodMap[promResponseItem.Metric.Pod]
				if !ok {
					gpuUsageMaxPodMap[promResponseItem.Metric.Pod] = &GPUUsageMaxAggregate{
						PrometheusUsageMax: promResponseItem.Value.Value,
						AllocationUsageMax: 0.0,
						Namespace:          promResponseItem.Metric.Namespace,
					}
					continue
				}
				gpuUsageMaxPod.PrometheusUsageMax = max(gpuUsageMaxPod.PrometheusUsageMax, promResponseItem.Value.Value)
			}

			/////////////////////////////////////////////
			// API Client
			/////////////////////////////////////////////

			// Why doesn't allocation work on Namespace aggregate?
			apiResponse, err := apiObj.GetAllocation(api.AllocationRequest{
				Window:     tc.window,
				Aggregate:  tc.aggregate,
				Accumulate: tc.accumulate,
			})

			if err != nil {
				t.Fatalf("Error while calling Allocation API %v", err)
			}
			if apiResponse.Code != 200 {
				t.Errorf("API returned non-200 code")
			}

			for _, allocationResponseItem := range apiResponse.Data[0] {
				gpuUsageMaxPod, ok := gpuUsageMaxPodMap[allocationResponseItem.Properties.Pod]
				if !ok {
					gpuUsageMaxPodMap[allocationResponseItem.Properties.Pod] = &GPUUsageMaxAggregate{
						PrometheusUsageMax: 0.0,
						AllocationUsageMax: allocationResponseItem.RawAllocationsOnly.GPUUsageMax,
					}
					continue
				}
				gpuUsageMaxPod.AllocationUsageMax = max(gpuUsageMaxPod.AllocationUsageMax, allocationResponseItem.RawAllocationsOnly.GPUUsageMax)
			}

			// Windows are not accurate for prometheus and allocation
			// for pod, gpuMaxUsageValues := range gpuUsageMaxPodMap {
			// 	t.Logf("Pod %s", pod)
			// 	// Ignore Zero Max Value Pods
			// 	if gpuMaxUsageValues.AllocationUsageMax == 0{
			// 		continue
			// 	}
			// 	withinTolerance, diff_percent := utils.AreWithinPercentage(gpuMaxUsageValues.PrometheusUsageMax, gpuMaxUsageValues.AllocationUsageMax, tolerance)
			// 	if !withinTolerance {
			// 		t.Errorf("GPUUsageMax[Fail]: DifferencePercent %0.2f, Prometheus: %0.2f, /allocation: %0.2f", diff_percent, gpuMaxUsageValues.PrometheusUsageMax, gpuMaxUsageValues.AllocationUsageMax)
			// 	} else {
			// 		t.Logf("GPUUsageMax[Pass]: ~ %v", gpuMaxUsageValues.PrometheusUsageMax)
			// 	}
			// }

			gpuUsageMaxNamespaceMap := make(map[string]*GPUUsageMaxAggregate)
			for _, gpuMaxUsageValues := range gpuUsageMaxPodMap {
				gpuUsageNamespaceValue, ok := gpuUsageMaxNamespaceMap[gpuMaxUsageValues.Namespace]
				if !ok {
					gpuUsageMaxNamespaceMap[gpuMaxUsageValues.Namespace] = &GPUUsageMaxAggregate{
						PrometheusUsageMax: gpuMaxUsageValues.PrometheusUsageMax,
						AllocationUsageMax: gpuMaxUsageValues.AllocationUsageMax,
						Namespace:          gpuMaxUsageValues.Namespace, // Adds no value, Just for Clarity
					}
					continue
				}
				gpuUsageNamespaceValue.AllocationUsageMax = max(gpuUsageNamespaceValue.AllocationUsageMax, gpuMaxUsageValues.AllocationUsageMax)
				gpuUsageNamespaceValue.PrometheusUsageMax = max(gpuUsageNamespaceValue.PrometheusUsageMax, gpuMaxUsageValues.PrometheusUsageMax)
			}
			// t.Logf("\nMax Values for Namespaces.\n")
			// Windows are not accurate for prometheus and allocation
			for namespace, gpuMaxUsageValues := range gpuUsageMaxNamespaceMap {
				t.Logf("Namespace %s", namespace)
				withinRange, diff_percent := utils.AreWithinPercentage(gpuMaxUsageValues.PrometheusUsageMax, gpuMaxUsageValues.AllocationUsageMax, tolerance)
				if !withinRange {
					t.Errorf("GPUUsageMax[Fail]: DifferencePercent %0.2f, Prometheus: %0.2f, /allocation: %0.2f", diff_percent, gpuMaxUsageValues.PrometheusUsageMax, gpuMaxUsageValues.AllocationUsageMax)
				} else {
					t.Logf("GPUUsageMax[Pass]: ~ %v", gpuMaxUsageValues.PrometheusUsageMax)
				}
			}
		})
	}
}
