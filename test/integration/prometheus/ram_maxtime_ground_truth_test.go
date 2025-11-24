package prometheus

// Description - Checks for Ram Byte Hours from prometheus and /allocation are the same

// RawAllocationOnlyData is information that only belong in "raw" Allocations,
// those which have not undergone aggregation, accumulation, or any other form
// of combination to produce a new Allocation from other Allocations.
//
// Max usage data belongs here because computing the overall maximum from two
// or more Allocations is a non-trivial operation that cannot be defined without
// maintaining a large amount of state. Consider the following example:
// _______________________________________________
//
// A1 Using 3 CPU    ----      -----     ------
// A2 Using 2 CPU      ----      -----      ----
// A3 Using 1 CPU         ---       --
// _______________________________________________
//
//	Time ---->
//
// The logical maximum CPU usage is 5, but this cannot be calculated iteratively,
// which is how we calculate aggregations and accumulations of Allocations currently.
// This becomes a problem I could call "maximum sum of overlapping intervals" and is
// essentially a variant of an interval scheduling algorithm.
//
// If we had types to differentiate between regular Allocations and AggregatedAllocations
// then this type would be unnecessary and its fields would go into the regular Allocation
// and not in the AggregatedAllocation.

import (
	// "fmt"
	"testing"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
	"github.com/opencost/opencost-integration-tests/pkg/utils"
)

const ramMaxTimeTolerance = 0.07

func TestRAMMax(t *testing.T) {
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
		{
			name:       "Last Two Days",
			window:     "48h",
			aggregate:  "container,pod",
			accumulate: "false",
		},
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			// Any data that is in a "raw allocation only" is not valid in any
			// sort of cumulative Allocation (like one that is added).

			type RAMUsageMaxAggregate struct {
				AllocationUsageMax float64
				PrometheusUsageMax float64
				Namespace          string
			}
			ramUsageMaxPodMap := make(map[string]*RAMUsageMaxAggregate)

			////////////////////////////////////////////////////////////////////////////
			// RAMMaxUsage Calculation

			// max(max_over_time(container_memory_working_set_bytes{
			//     container!="", container_name!="POD", container!="POD", %s}[%s]))
			// by
			// (container_name, container, pod_name, pod, namespace, node, instance, %s)
			////////////////////////////////////////////////////////////////////////////

			queryEnd := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
			endTime := queryEnd.Unix()
			// Collect Namespace results from Prometheus
			client := prometheus.NewClient()
			promInput := prometheus.PrometheusInput{
				Metric: "container_memory_working_set_bytes",
			}
			ignoreFilters := map[string][]string{
				"container": {"", "POD"},
				"node":      {""},
			}
			promInput.Function = []string{"max_over_time", "max"}
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
				ramUsageMaxPod, ok := ramUsageMaxPodMap[promResponseItem.Metric.Pod]
				if !ok {
					ramUsageMaxPodMap[promResponseItem.Metric.Pod] = &RAMUsageMaxAggregate{
						PrometheusUsageMax: promResponseItem.Value.Value,
						AllocationUsageMax: 0.0,
						Namespace:          promResponseItem.Metric.Namespace,
					}
					continue
				}
				ramUsageMaxPod.PrometheusUsageMax = max(ramUsageMaxPod.PrometheusUsageMax, promResponseItem.Value.Value)
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
				ramUsageMaxPod, ok := ramUsageMaxPodMap[allocationResponseItem.Properties.Pod]
				if !ok {
					ramUsageMaxPodMap[allocationResponseItem.Properties.Pod] = &RAMUsageMaxAggregate{
						PrometheusUsageMax: 0.0,
						AllocationUsageMax: allocationResponseItem.RawAllocationsOnly.RAMByteUsageMax,
					}
					continue
				}
				ramUsageMaxPod.AllocationUsageMax = max(ramUsageMaxPod.AllocationUsageMax, allocationResponseItem.RawAllocationsOnly.RAMByteUsageMax)
			}

			// Windows are not accurate for prometheus and allocation
			for pod, ramMaxUsageValues := range ramUsageMaxPodMap {
				t.Logf("Pod %s", pod)
				// Ignore Zero Max Value Pods
				if ramMaxUsageValues.AllocationUsageMax == 0 {
					continue
				}
				withinTolerance, diff_percent := utils.AreWithinPercentage(ramMaxUsageValues.PrometheusUsageMax, ramMaxUsageValues.AllocationUsageMax, ramMaxTimeTolerance)
				if !withinTolerance {
					t.Errorf("RAMUsageMax[Fail]: DifferencePercent %0.2f, Prometheus: %0.2f, /allocation: %0.2f", diff_percent, ramMaxUsageValues.PrometheusUsageMax, ramMaxUsageValues.AllocationUsageMax)
				} else {
					t.Logf("RAMUsageMax[Pass]: ~ %v", ramMaxUsageValues.PrometheusUsageMax)
				}
			}

			ramUsageMaxNamespaceMap := make(map[string]*RAMUsageMaxAggregate)
			for _, ramMaxUsageValues := range ramUsageMaxPodMap {
				ramUsageNamespaceValue, ok := ramUsageMaxNamespaceMap[ramMaxUsageValues.Namespace]
				if !ok {
					ramUsageMaxNamespaceMap[ramMaxUsageValues.Namespace] = &RAMUsageMaxAggregate{
						PrometheusUsageMax: ramMaxUsageValues.PrometheusUsageMax,
						AllocationUsageMax: ramMaxUsageValues.AllocationUsageMax,
						Namespace:          ramMaxUsageValues.Namespace, // Adds no value, Just for Clarity
					}
					continue
				}
				ramUsageNamespaceValue.AllocationUsageMax = max(ramUsageNamespaceValue.AllocationUsageMax, ramMaxUsageValues.AllocationUsageMax)
				ramUsageNamespaceValue.PrometheusUsageMax = max(ramUsageNamespaceValue.PrometheusUsageMax, ramMaxUsageValues.PrometheusUsageMax)
			}

			t.Logf("\nMax Values for Namespaces.\n")
			// Windows are not accurate for prometheus and allocation
			for namespace, ramMaxUsageValues := range ramUsageMaxNamespaceMap {
				t.Logf("Namespace %s", namespace)
				withinRange, diff_percent := utils.AreWithinPercentage(ramMaxUsageValues.PrometheusUsageMax, ramMaxUsageValues.AllocationUsageMax, ramMaxTimeTolerance)
				if !withinRange {
					t.Errorf("RAMUsageMax[Fail]: DifferencePercent %0.2f, Prometheus: %0.2f, /allocation: %0.2f", diff_percent, ramMaxUsageValues.PrometheusUsageMax, ramMaxUsageValues.AllocationUsageMax)
				} else {
					t.Logf("RAMUsageMax[Pass]: ~ %v", ramMaxUsageValues.PrometheusUsageMax)
				}
			}
		})
	}
}
