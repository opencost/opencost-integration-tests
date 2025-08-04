package prometheus

// Description - Checks for cpu Average Usage from prometheus and /allocation are the same

import (
	// "fmt"
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
	"github.com/opencost/opencost-integration-tests/pkg/utils"
	"testing"
	"time"
)

const tolerance = 0.07
const negligibleUsage = 0.01

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
				Pod       string
				Namespace string
				RunTime   float64
			}

			podMap := make(map[string]*PodData)

			for _, podInfoResponseItem := range podInfo.Data.Result {

				s, e := prometheus.CalculateStartAndEnd(podInfoResponseItem.Values, resolution, window24h)
				podMap[podInfoResponseItem.Metric.Pod] = &PodData{
					Pod:       podInfoResponseItem.Metric.Pod,
					Namespace: podInfoResponseItem.Metric.Namespace,
					RunTime:   e.Sub(s).Minutes(),
				}
			}

			type cpuUsageAvgAggregate struct {
				AllocationUsageAvg float64
				PrometheusUsageAvg float64
			}
			cpuUsageAvgNamespaceMap := make(map[string]*cpuUsageAvgAggregate)

			////////////////////////////////////////////////////////////////////////////
			// CPUAvgUsage Calculation
			// avg(rate(container_cpu_usage_seconds_total{
			//     container!="", container_name!="POD", container!="POD", %s}[%s]))
			// by
			// (container_name, container, pod_name, pod, namespace, node, instance, %s)
			////////////////////////////////////////////////////////////////////////////
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
				// Get containerRunTime by getting the pod's (parent object) runtime.
				pod, ok := podMap[promResponseItem.Metric.Pod]
				if !ok {
					continue
				}
				containerRunTime := pod.RunTime

				cpuUsageAvgPod, ok := cpuUsageAvgNamespaceMap[promResponseItem.Metric.Namespace]
				if !ok {
					cpuUsageAvgNamespaceMap[promResponseItem.Metric.Namespace] = &cpuUsageAvgAggregate{
						PrometheusUsageAvg: promResponseItem.Value.Value * containerRunTime,
						AllocationUsageAvg: 0.0,
					}
					continue
				}
				cpuUsageAvgPod.PrometheusUsageAvg += promResponseItem.Value.Value * containerRunTime
			}

			windowRunTime := queryEnd.Sub(queryStart).Minutes()
			for _, cpuUsageAvgProm := range cpuUsageAvgNamespaceMap {
				cpuUsageAvgProm.PrometheusUsageAvg = cpuUsageAvgProm.PrometheusUsageAvg / windowRunTime
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
					cpuUsageAvgNamespaceMap[namespace] = &cpuUsageAvgAggregate{
						PrometheusUsageAvg: 0,
						AllocationUsageAvg: allocationResponseItem.CPUCoreUsageAverage,
					}
					continue
				}
				cpuUsageAvgPod.AllocationUsageAvg += allocationResponseItem.CPUCoreUsageAverage
			}

			seenUsage := false
			t.Logf("\nAvg Values for Namespaces.\n")
			// Windows are not accurate for prometheus and allocation
			for namespace, cpuAvgUsageValues := range cpuUsageAvgNamespaceMap {
				if cpuAvgUsageValues.AllocationUsageAvg < negligibleUsage {
					continue
				} else {
					seenUsage = true
				}
				t.Logf("Namespace %s", namespace)
				withinRange, diff_percent := utils.AreWithinPercentage(cpuAvgUsageValues.PrometheusUsageAvg, cpuAvgUsageValues.AllocationUsageAvg, tolerance)
				if !withinRange {
					t.Errorf("cpuUsageAvg[Fail]: DifferencePercent %0.2f, Prometheus: %0.2f, /allocation: %0.2f", diff_percent, cpuAvgUsageValues.PrometheusUsageAvg, cpuAvgUsageValues.AllocationUsageAvg)
				} else {
					t.Logf("cpuUsageAvg[Pass]: ~ %v", cpuAvgUsageValues.PrometheusUsageAvg)
				}
			}
			if seenUsage == false {
				t.Logf("All Costs were Negligible and cannot be tested. Failing Test")
			}
		})
	}
}
