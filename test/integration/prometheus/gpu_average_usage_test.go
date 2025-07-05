package prometheus

// Description - Checks for GPU Average Usage from prometheus and /allocation are the same

import (
	// "fmt"
	"time"
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
	"github.com/opencost/opencost-integration-tests/pkg/utils"
	"testing"
)

const tolerance = 0.05

func TestGPUAvgUsage(t *testing.T) {
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
			aggregate:  "namespace",
			accumulate: "false",
		},
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			type GPUUsageAvgAggregate struct {
				AllocationUsageAvg float64
				PrometheusUsageAvg float64
			}

			gpuUsageAvgNamespaceMap := make(map[string]*GPUUsageAvgAggregate)

			////////////////////////////////////////////////////////////////////////////
			// GPUAvgUsage Calculation

			// avg(avg_over_time(DCGM_FI_PROF_GR_ENGINE_ACTIVE{container!=""}[%s])) by (container, pod, namespace, %s)
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
			promInput.Function = []string{"avg_over_time", "avg"}
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
				gpuUsageAvgPod, ok := gpuUsageAvgNamespaceMap[promResponseItem.Metric.Namespace]
				if !ok {
					gpuUsageAvgNamespaceMap[promResponseItem.Metric.Namespace] = &GPUUsageAvgAggregate{
						PrometheusUsageAvg: promResponseItem.Value.Value,
						AllocationUsageAvg: 0.0,
					}
					continue
				}
				gpuUsageAvgPod.PrometheusUsageAvg += promResponseItem.Value.Value
			}

			/////////////////////////////////////////////
			// API Client
			/////////////////////////////////////////////

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

			for namespace, allocationResponseItem := range apiResponse.Data[0] {
				gpuUsageAvgPod, ok := gpuUsageAvgNamespaceMap[namespace]
				if !ok {
					gpuUsageAvgNamespaceMap[namespace] = &GPUUsageAvgAggregate{
						AllocationUsageAvg: allocationResponseItem.GPUAllocation.GPUUsageAverage,
					}
					continue
				}
				gpuUsageAvgPod.AllocationUsageAvg += allocationResponseItem.GPUAllocation.GPUUsageAverage
			}

			t.Logf("\nAvg Values for Namespaces.\n")
			// Windows are not accurate for prometheus and allocation
			for namespace, gpuAvgUsageValues := range gpuUsageAvgNamespaceMap {
				t.Logf("Namespace %s", namespace)
				withinRange, diff_percent := utils.AreWithinPercentage(gpuAvgUsageValues.PrometheusUsageAvg, gpuAvgUsageValues.AllocationUsageAvg, tolerance)
				if !withinRange {
					t.Errorf("GPUUsageAvg[Fail]: DifferencePercent %0.2f, Prometheus: %0.2f, /allocation: %0.2f", diff_percent, gpuAvgUsageValues.PrometheusUsageAvg, gpuAvgUsageValues.AllocationUsageAvg)
				} else {
					t.Logf("GPUUsageAvg[Pass]: ~ %v", gpuAvgUsageValues.PrometheusUsageAvg)
				}
			}
		})
	}
}
