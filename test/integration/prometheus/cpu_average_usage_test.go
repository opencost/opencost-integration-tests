package prometheus

// Description - Checks for CPU Average Usage from prometheus and /allocation are the same

import (
	// "fmt"
	"time"
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
	"github.com/opencost/opencost-integration-tests/pkg/utils"
	"testing"
)

// This is a bit of a hack to work around garbage data from cadvisor
// Ideally you cap each pod to the max CPU on its node, but that involves a bit more complexity, as it it would need to be done when allocations joins with asset data.
const CPU_SANITY_LIMIT = 512
const tolerance = 0.05

func TestCPUAvgUsage(t *testing.T) {
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

			type CPUUsageAvgAggregate struct {
				AllocationUsageAvg float64
				PrometheusUsageAvg float64
			}

			cpuUsageAvgNamespaceMap := make(map[string]*CPUUsageAvgAggregate)

			////////////////////////////////////////////////////////////////////////////
			// CPUAvgUsage Calculation
			// avg(rate(container_cpu_usage_seconds_total{
			//     container!="", container_name!="POD", container!="POD", %s}[%s]))
			// by
			// (container_name, container, pod_name, pod, namespace, node, instance, %s)
			////////////////////////////////////////////////////////////////////////////

			queryEnd := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
			endTime := queryEnd.Unix()
			// Collect Namespace results from Prometheus
			client := prometheus.NewClient()
			promInput := prometheus.PrometheusInput{
				Metric: "container_cpu_usage_seconds_total",
			}
			ignoreFilters := map[string][]string{
				"container": {"", "POD"},
				"node":      {""},
			}
			promInput.Function = []string{"rate", "avg"}
			promInput.QueryWindow = tc.window
			promInput.IgnoreFilters = ignoreFilters
			promInput.AggregateBy = []string{"container", "pod", "namespace", "node", "instance"}
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
				if promResponseItem.Value.Value >= CPU_SANITY_LIMIT {
					continue
				}
				cpuUsageAvgPod, ok := cpuUsageAvgNamespaceMap[promResponseItem.Metric.Namespace]
				if !ok {
					cpuUsageAvgNamespaceMap[promResponseItem.Metric.Namespace] = &CPUUsageAvgAggregate{
						PrometheusUsageAvg: promResponseItem.Value.Value,
						AllocationUsageAvg: 0.0,
					}
					continue
				}
				cpuUsageAvgPod.PrometheusUsageAvg += promResponseItem.Value.Value
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
				cpuUsageAvgPod, ok := cpuUsageAvgNamespaceMap[namespace]
				if !ok {
					cpuUsageAvgNamespaceMap[namespace] = &CPUUsageAvgAggregate{
						AllocationUsageAvg: allocationResponseItem.CPUCoreUsageAverage,
					}
					continue
				}
				cpuUsageAvgPod.AllocationUsageAvg += allocationResponseItem.CPUCoreUsageAverage
			}

			t.Logf("\nAvg Values for Namespaces.\n")
			// Windows are not accurate for prometheus and allocation
			for namespace, cpuAvgUsageValues := range cpuUsageAvgNamespaceMap {
				t.Logf("Namespace %s", namespace)
				withinRange, diff_percent := utils.AreWithinPercentage(cpuAvgUsageValues.PrometheusUsageAvg, cpuAvgUsageValues.AllocationUsageAvg, tolerance)
				if !withinRange {
					t.Errorf("CPUUsageAvg[Fail]: DifferencePercent %0.2f, Prometheus: %0.2f, /allocation: %0.2f", diff_percent, cpuAvgUsageValues.PrometheusUsageAvg, cpuAvgUsageValues.AllocationUsageAvg)
				} else {
					t.Logf("CPUUsageAvg[Pass]: ~ %v", cpuAvgUsageValues.PrometheusUsageAvg)
				}
			}
		})
	}
}
