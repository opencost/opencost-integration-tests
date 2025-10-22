package prometheus

// Description - Checks for GPU Average Usage from prometheus and /allocation are the same

import (
	// "fmt"
	"testing"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
	"github.com/opencost/opencost-integration-tests/pkg/utils"
)

const gpuAverageUsageResolution = "1m"
const gpuAverageUsageTolerance = 0.05
const gpuAverageUsageNegligibleUsage = 0.01

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
		//TODO
		// {
		// 	name:       "Last Two Days",
		// 	window:     "48h",
		// 	aggregate:  "namespace",
		// 	accumulate: "false",
		// },
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

			resolutionNumericVal, _ := utils.ExtractNumericPrefix(gpuAverageUsageResolution)
			resolution := time.Duration(int(resolutionNumericVal) * int(time.Minute))
			endTime := queryEnd.Unix()

			windowRange := prometheus.GetOffsetAdjustedQueryWindow(tc.window, gpuAverageUsageResolution)

			client := prometheus.NewClient()
			// Pod Info
			promPodInfoInput := prometheus.PrometheusInput{}
			promPodInfoInput.Metric = "kube_pod_container_status_running"
			promPodInfoInput.MetricNotEqualTo = "0"
			promPodInfoInput.AggregateBy = []string{"container", "pod", "namespace", "node"}
			promPodInfoInput.Function = []string{"avg"}
			promPodInfoInput.AggregateWindow = windowRange
			promPodInfoInput.AggregateResolution = gpuAverageUsageResolution
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

			type GPUUsageAvgAggregate struct {
				AllocationUsageAvg float64
				PrometheusUsageAvg float64
				Window             *api.Window
			}
			GPUUsageAvgNamespaceMap := make(map[string]*GPUUsageAvgAggregate)

			////////////////////////////////////////////////////////////////////////////
			// GPUAvgUsage Calculation

			// avg(avg_over_time(DCGM_FI_PROF_GR_ENGINE_ACTIVE{container!=""}[%s])) by (container, pod, namespace, %s)
			////////////////////////////////////////////////////////////////////////////

			promInput := prometheus.PrometheusInput{
				Metric: "DCGM_FI_PROF_GR_ENGINE_ACTIVE",
			}
			ignoreFilters := map[string][]string{
				"container": {""},
			}
			promInput.Function = []string{"avg_over_time", "avg"}
			promInput.QueryWindow = windowRange
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
				// Get containerRunTime by getting the pod's (parent object) runtime.
				pod, ok := podMap[promResponseItem.Metric.Pod]
				if !ok {
					continue
				}

				containerRunTime := pod.Window.RunTime()

				GPUUsageAvgPod, ok := GPUUsageAvgNamespaceMap[promResponseItem.Metric.Namespace]
				if !ok {
					GPUUsageAvgNamespaceMap[promResponseItem.Metric.Namespace] = &GPUUsageAvgAggregate{
						PrometheusUsageAvg: promResponseItem.Value.Value * containerRunTime,
						AllocationUsageAvg: 0.0,
						Window: &api.Window{
							Start: pod.Window.Start,
							End:   pod.Window.End,
						},
					}
					continue
				}
				GPUUsageAvgPod.PrometheusUsageAvg += promResponseItem.Value.Value * containerRunTime
				GPUUsageAvgPod.Window = api.ExpandTimeRange(GPUUsageAvgPod.Window, pod.Window)
			}

			for _, GPUUsageAvgProm := range GPUUsageAvgNamespaceMap {
				GPUUsageAvgProm.PrometheusUsageAvg = GPUUsageAvgProm.PrometheusUsageAvg / GPUUsageAvgProm.Window.RunTime()
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
				GPUUsageAvgPod, ok := GPUUsageAvgNamespaceMap[namespace]
				if !ok {
					GPUUsageAvgNamespaceMap[namespace] = &GPUUsageAvgAggregate{
						PrometheusUsageAvg: 0,
						AllocationUsageAvg: allocationResponseItem.GPUAllocation.GPUUsageAverage,
					}
					continue
				}
				GPUUsageAvgPod.AllocationUsageAvg += allocationResponseItem.GPUAllocation.GPUUsageAverage
			}

			seenUsage := false
			t.Logf("\nAvg Values for Namespaces.\n")
			// Windows are not accurate for prometheus and allocation
			for namespace, GPUAvgUsageValues := range GPUUsageAvgNamespaceMap {
				if GPUAvgUsageValues.AllocationUsageAvg < gpuAverageUsageNegligibleUsage {
					continue
				} else {
					seenUsage = true
				}
				t.Logf("Namespace %s", namespace)
				withinRange, diff_percent := utils.AreWithinPercentage(GPUAvgUsageValues.PrometheusUsageAvg, GPUAvgUsageValues.AllocationUsageAvg, gpuAverageUsageTolerance)
				if !withinRange {
					t.Errorf("GPUUsageAvg[Fail]: DifferencePercent %0.2f, Prometheus: %0.2f, /allocation: %0.2f", diff_percent, GPUAvgUsageValues.PrometheusUsageAvg, GPUAvgUsageValues.AllocationUsageAvg)
				} else {
					t.Logf("GPUUsageAvg[Pass]: ~ %v", GPUAvgUsageValues.PrometheusUsageAvg)
				}
			}
			if seenUsage == false {
				t.Logf("All Costs were Negligible and cannot be tested. Failing Test")
			}
		})
	}
}
