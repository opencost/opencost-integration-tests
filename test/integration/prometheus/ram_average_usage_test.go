package prometheus

// Description - Checks for RAM Average Usage from prometheus and /allocation are the same

import (
	// "fmt"
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
	"github.com/opencost/opencost-integration-tests/pkg/utils"
	"testing"
	"time"
)

const Resolution = "1m"
const Tolerance = 0.07
const negligibleUsage = 0.01

func TestRAMAvgUsage(t *testing.T) {
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
		{
			name:       "Last Two Days",
			window:     "48h",
			aggregate:  "namespace",
			accumulate: "false",
		},
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			// Use this information to find start and end time of pod
			queryEnd := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
			// Get Time Duration
			timeNumericVal, _ := utils.ExtractNumericPrefix(tc.window)
			// Assume the minumum unit is an hour
			negativeDuration := time.Duration(timeNumericVal*float64(time.Hour)) * -1
			queryStart := queryEnd.Add(negativeDuration)
			window24h := api.Window{
				Start: queryStart,
				End:   queryEnd,
			}
			resolutionNumericVal, _ := utils.ExtractNumericPrefix(Resolution)
			resolution := time.Duration(int(resolutionNumericVal) * int(time.Minute))
			endTime := queryEnd.Unix()

			windowRange := prometheus.GetOffsetAdjustedQueryWindow(tc.window, Resolution)

			client := prometheus.NewClient()
			// Pod Info
			promPodInfoInput := prometheus.PrometheusInput{}
			promPodInfoInput.Metric = "kube_pod_container_status_running"
			promPodInfoInput.MetricNotEqualTo = "0"
			promPodInfoInput.AggregateBy = []string{"container", "pod", "namespace", "node"}
			promPodInfoInput.Function = []string{"avg"}
			promPodInfoInput.AggregateWindow = windowRange
			promPodInfoInput.AggregateResolution = Resolution
			promPodInfoInput.Time = &endTime

			podInfo, err := client.RunPromQLQuery(promPodInfoInput)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			type PodData struct {
				Pod       string
				Namespace string
				Window    *api.Window
			}

			podMap := make(map[string]*PodData)

			for _, podInfoResponseItem := range podInfo.Data.Result {

				s, e := prometheus.CalculateStartAndEnd(podInfoResponseItem.Values, resolution, window24h)
				podMap[podInfoResponseItem.Metric.Pod] = &PodData{
					Pod:       podInfoResponseItem.Metric.Pod,
					Namespace: podInfoResponseItem.Metric.Namespace,
					Window: &api.Window{
						Start: s,
						End:   e,
					},
				}
			}

			type RAMUsageAvgAggregate struct {
				AllocationUsageAvg float64
				PrometheusUsageAvg float64
				Window             *api.Window
			}
			ramUsageAvgNamespaceMap := make(map[string]*RAMUsageAvgAggregate)

			////////////////////////////////////////////////////////////////////////////
			// RAMAvgUsage Calculation
			// avg(avg_over_time(container_memory_working_set_bytes{
			//     container!="", container_name!="POD", container!="POD", %s}[%s]))
			// by
			// (container_name, container, pod_name, pod, namespace, node, instance, %s)[24h:5m]
			////////////////////////////////////////////////////////////////////////////

			// Collect Namespace results from Prometheus
			promInput := prometheus.PrometheusInput{
				Metric: "container_memory_working_set_bytes",
			}
			ignoreFilters := map[string][]string{
				"container": {"", "POD"},
				"node":      {""},
			}
			promInput.Function = []string{"avg_over_time", "avg"}
			promInput.QueryWindow = windowRange
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
				// Get containerRunTime by getting the pod's (parent object) runtime.
				pod, ok := podMap[promResponseItem.Metric.Pod]
				if !ok {
					continue
				}

				containerRunTime := pod.Window.RunTime()

				ramUsageAvgPod, ok := ramUsageAvgNamespaceMap[promResponseItem.Metric.Namespace]
				if !ok {
					ramUsageAvgNamespaceMap[promResponseItem.Metric.Namespace] = &RAMUsageAvgAggregate{
						PrometheusUsageAvg: promResponseItem.Value.Value * containerRunTime,
						AllocationUsageAvg: 0.0,
						Window: &api.Window{
							Start: pod.Window.Start,
							End:   pod.Window.End,
						},
					}
					continue
				}
				ramUsageAvgPod.PrometheusUsageAvg += promResponseItem.Value.Value * containerRunTime
				ramUsageAvgPod.Window = api.ExpandTimeRange(ramUsageAvgPod.Window, pod.Window)
			}

			// windowRunTime := queryEnd.Sub(queryStart).Minutes()
			for _, ramUsageAvgProm := range ramUsageAvgNamespaceMap {
				ramUsageAvgProm.PrometheusUsageAvg = ramUsageAvgProm.PrometheusUsageAvg / ramUsageAvgProm.Window.RunTime()
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
				ramUsageAvgPod, ok := ramUsageAvgNamespaceMap[namespace]
				if !ok {
					ramUsageAvgNamespaceMap[namespace] = &RAMUsageAvgAggregate{
						PrometheusUsageAvg: 0,
						AllocationUsageAvg: allocationResponseItem.RAMBytesUsageAverage,
					}
					continue
				}
				ramUsageAvgPod.AllocationUsageAvg += allocationResponseItem.RAMBytesUsageAverage
			}

			seenUsage := false
			t.Logf("\nAvg Values for Namespaces.\n")
			// Windows are not accurate for prometheus and allocation
			for namespace, ramAvgUsageValues := range ramUsageAvgNamespaceMap {
				if ramAvgUsageValues.AllocationUsageAvg < negligibleUsage {
					continue
				} else {
					seenUsage = true
				}
				t.Logf("Namespace %s", namespace)
				withinRange, diff_percent := utils.AreWithinPercentage(ramAvgUsageValues.PrometheusUsageAvg, ramAvgUsageValues.AllocationUsageAvg, tolerance)
				if !withinRange {
					t.Errorf("RAMUsageAvg[Fail]: DifferencePercent %0.2f, Prometheus: %0.2f, /allocation: %0.2f", diff_percent, ramAvgUsageValues.PrometheusUsageAvg, ramAvgUsageValues.AllocationUsageAvg)
				} else {
					t.Logf("RAMUsageAvg[Pass]: ~ %v", ramAvgUsageValues.PrometheusUsageAvg)
				}
			}
			if seenUsage == false {
				t.Logf("All Costs were Negligible and cannot be tested. Failing Test")
			}
		})
	}
}
