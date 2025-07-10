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

const tolerance = 0.05

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
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			// Use this information to find start and end time of pod
			queryEnd := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
			queryStart := queryEnd.Add(-24 * time.Hour)
			window24h := api.Window{
				Start: queryStart,
				End:   queryEnd,
			}
			resolution := 5 * time.Minute
			endTime := queryEnd.Unix()

			client := prometheus.NewClient()
			// Pod Info
			promPodInfoInput := prometheus.PrometheusInput{}
			promPodInfoInput.Metric = "kube_pod_container_status_running"
			promPodInfoInput.MetricNotEqualTo = "0"
			promPodInfoInput.AggregateBy = []string{"container", "pod", "namespace", "node"}
			promPodInfoInput.Function = []string{"avg"}
			promPodInfoInput.AggregateWindow = tc.window
			promPodInfoInput.AggregateResolution = "5m"
			promPodInfoInput.Time = &endTime

			podInfo, err := client.RunPromQLQuery(promPodInfoInput)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			type PodData struct {
				Pod        string
				Namespace  string
				RunTime    float64
			}

			podMap := make(map[string]*PodData)

			for _, podInfoResponseItem := range podInfo.Data.Result {

				s, e := prometheus.CalculateStartAndEnd(podInfoResponseItem.Values, resolution, window24h)
				podMap[podInfoResponseItem.Metric.Pod] = &PodData{
					Pod:        podInfoResponseItem.Metric.Pod,
					Namespace:  podInfoResponseItem.Metric.Namespace,
					RunTime:    e.Sub(s).Minutes(),
				}
			}

			type RAMUsageAvgAggregate struct {
				AllocationUsageAvg float64
				PrometheusUsageAvg float64
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
				// Get containerRunTime by getting the pod's (parent object) runtime. 
				containerRunTime := podMap[promResponseItem.Metric.Pod].RunTime

				ramUsageAvgPod, ok := ramUsageAvgNamespaceMap[promResponseItem.Metric.Namespace]
				if !ok {
					ramUsageAvgNamespaceMap[promResponseItem.Metric.Namespace] = &RAMUsageAvgAggregate{
						PrometheusUsageAvg: promResponseItem.Value.Value * containerRunTime,
						AllocationUsageAvg: 0.0,
					}
					continue
				}
				ramUsageAvgPod.PrometheusUsageAvg += promResponseItem.Value.Value * containerRunTime
			}

			windowRunTime := queryEnd.Sub(queryStart).Minutes()
			for _, ramUsageAvgProm := range ramUsageAvgNamespaceMap {
				ramUsageAvgProm.PrometheusUsageAvg = ramUsageAvgProm.PrometheusUsageAvg / windowRunTime
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

			t.Logf("\nAvg Values for Namespaces.\n")
			// Windows are not accurate for prometheus and allocation
			for namespace, ramAvgUsageValues := range ramUsageAvgNamespaceMap {
				t.Logf("Namespace %s", namespace)
				withinRange, diff_percent := utils.AreWithinPercentage(ramAvgUsageValues.PrometheusUsageAvg, ramAvgUsageValues.AllocationUsageAvg, tolerance)
				if !withinRange {
					t.Errorf("RAMUsageAvg[Fail]: DifferencePercent %0.2f, Prometheus: %0.2f, /allocation: %0.2f", diff_percent, ramAvgUsageValues.PrometheusUsageAvg, ramAvgUsageValues.AllocationUsageAvg)
				} else {
					t.Logf("RAMUsageAvg[Pass]: ~ %v", ramAvgUsageValues.PrometheusUsageAvg)
				}
			}
		})
	}
}
