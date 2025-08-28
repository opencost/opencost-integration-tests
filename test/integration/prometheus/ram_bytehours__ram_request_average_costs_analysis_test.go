package prometheus

// Description: Checks all RAM related Costs
// - RAMBytes
// - RAMBytesHours
// - RAMBytesRequestAverage
// - RAMMaxUsage

// Testing Methodology
// 1. Query RAMAllocated and RAMRequested in prometheus
//     1.1 Use the current "time" as upperbound in promql
//     1.2 Query by (container, pod, node, namespace)
// 2. Consolidate containers based on pods
//     2.1 RAMByte is the max of RAMByteAllocated and RAMByteRequested for each container
//     2.2 Query [24h:5m] to get 288 (1440/5) points to identify the StartTime and EndTime DataPoint Timestamp.
// 	   2.3 Assumption 1: Identify the time range for the pod (all containers within the pod have the same time range)
// 3. Consolidate RAMBytes based on pod and then based on namespace
// 4. Fetch /allocation data aggregated by namespace
// 5. Compare results with a 5% error margin.

import (
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
	"github.com/opencost/opencost-integration-tests/pkg/utils"
	"testing"
	"time"
)

// 10 Minutes
const ShortLivedPodsRunTime = 60
const Resolution = "1m"
const Tolerance = 0.05

func ConvertToHours(minutes float64) float64 {
	// Convert Time from Minutes to Hours
	return minutes / 60
}

func TestRAMCosts(t *testing.T) {
	apiObj := api.NewAPI()

	// test for more windows
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

			// ----------------------------------------------
			// Allocation API Data Collection
			// ----------------------------------------------
			// /compute/allocation: RAM Costs for all namespaces
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

			// ----------------------------------------------
			// Prometheus Data Collection
			// ----------------------------------------------
			client := prometheus.NewClient()

			// Loop over namespaces
			for namespace, allocationResponseItem := range apiResponse.Data[0] {

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
				// Note that in the Pod Query, we use a 5m resolution [THIS IS THE DEFAULT VALUE IN OPENCOST]
				resolutionNumericVal, _ := utils.ExtractNumericPrefix(Resolution)
				resolution := time.Duration(int(resolutionNumericVal) * int(time.Minute))

				// Query End Time for all Queries
				endTime := queryEnd.Unix()

				windowRange := prometheus.GetOffsetAdjustedQueryWindow(tc.window, Resolution)

				// Metric: RAMRequests
				// avg(avg_over_time(
				// 		kube_pod_container_resource_requests{
				// 			resource="memory", unit="byte", container!="", container!="POD", node!=""
				// 		}[24h])
				// )
				// by
				// (container, pod, namespace, node)

				// Q) What about Cluster Filter and Cluster Label?
				promRAMRequestedInput := prometheus.PrometheusInput{}

				promRAMRequestedInput.Metric = "kube_pod_container_resource_requests"
				promRAMRequestedInput.Filters = map[string]string{
					// "job": "opencost", Averaging all results negates this process
					"resource":  "memory",
					"unit":      "byte",
					"namespace": namespace,
				}
				promRAMRequestedInput.IgnoreFilters = map[string][]string{
					"container": {"", "POD"},
					"node":      {""},
				}
				promRAMRequestedInput.AggregateBy = []string{"container", "pod", "namespace", "node"}
				promRAMRequestedInput.Function = []string{"avg_over_time", "avg"}
				promRAMRequestedInput.QueryWindow = windowRange
				promRAMRequestedInput.Time = &endTime

				requestedRAM, err := client.RunPromQLQuery(promRAMRequestedInput)
				if err != nil {
					t.Fatalf("Error while calling Prometheus API %v", err)
				}

				// Metric: RAMAllocated
				// avg(avg_over_time(
				// 		container_memory_allocation_bytes{
				// 			container!="", container!="POD", node!=""
				// 		}[24h])
				// )
				// by
				// (container, pod, namespace, node)

				// Q) What about Cluster Filter and Cluster Label?
				promRAMAllocatedInput := prometheus.PrometheusInput{}

				promRAMAllocatedInput.Metric = "container_memory_allocation_bytes"
				promRAMAllocatedInput.Filters = map[string]string{
					// "job": "opencost", Averaging all results negates this process
					"namespace": namespace,
				}
				promRAMAllocatedInput.IgnoreFilters = map[string][]string{
					"container": {"", "POD"},
					"node":      {""},
				}
				promRAMAllocatedInput.AggregateBy = []string{"container", "pod", "namespace", "node"}
				promRAMAllocatedInput.Function = []string{"avg_over_time", "avg"}
				promRAMAllocatedInput.QueryWindow = windowRange
				promRAMAllocatedInput.Time = &endTime

				allocatedRAM, err := client.RunPromQLQuery(promRAMAllocatedInput)
				if err != nil {
					t.Fatalf("Error while calling Prometheus API %v", err)
				}

				// Query all running pod information
				// avg(kube_pod_container_status_running{} != 0)
				// by
				// (container, pod, namespace, node)[24h:5m]

				// Q) != 0 is not necessary I suppose?
				promPodInfoInput := prometheus.PrometheusInput{}

				promPodInfoInput.Metric = "kube_pod_container_status_running"
				promPodInfoInput.Filters = map[string]string{
					"namespace": namespace,
				}
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

				// Define Local Struct for PoD Data Consolidation
				// PoD is composed of multiple containers, we want to represent all that information succinctly here
				type ContainerRAMData struct {
					Container              string
					RAMBytesRequestAverage float64
					RAMBytes               float64
					RAMBytesHours          float64
				}

				type PodData struct {
					Pod        string
					Namespace  string
					Start      time.Time
					End        time.Time
					Minutes    float64
					Containers map[string]*ContainerRAMData
				}

				// ----------------------------------------------
				// Identify All Pods and Containers for that Pod

				// Create a map of PodData for each pod, and calculate the runtime.
				// The query we make for pods is _NOT_ an average over time "range" vector. Instead, it's a range vector
				// that returns _ALL_ samples for the pod over the last 24 hours. So, when you see <metric>[24h:5m] the
				// number after the ':' (1m) is the resolution of the samples. So for the pod query, we get 24h / 1m
				// ----------------------------------------------

				// Pointers to modify in place
				podMap := make(map[string]*PodData)

				for _, podInfoResponseItem := range podInfo.Data.Result {
					// The summary of this method is that we grab the _last_ sample time and _first_ sample time
					// from the pod's metrics, which roughly represents the start and end of the pod's lifetime
					// WITHIN the query window (24h in this case).
					s, e := prometheus.CalculateStartAndEnd(podInfoResponseItem.Values, resolution, window24h)

					// Add a key in the podMap for current Pod
					podMap[podInfoResponseItem.Metric.Pod] = &PodData{
						Pod:        podInfoResponseItem.Metric.Pod,
						Namespace:  namespace,
						Start:      s,
						End:        e,
						Minutes:    e.Sub(s).Minutes(),
						Containers: make(map[string]*ContainerRAMData),
					}
				}
				// ----------------------------------------------
				// Gather RAMBytes (RAMAllocated) for every container in a Pod

				// Iterate over results and group by pod
				// ----------------------------------------------

				for _, ramAllocatedItem := range allocatedRAM.Data.Result {
					container := ramAllocatedItem.Metric.Container
					pod := ramAllocatedItem.Metric.Pod
					if container == "" {
						t.Logf("Skipping RAM allocation for empty container in pod %s in namespace: %s", pod, ramAllocatedItem.Metric.Namespace)
						continue
					}
					podData, ok := podMap[pod]
					if !ok {
						t.Logf("Failed to find namespace: %s and pod: %s in RAM allocated results", ramAllocatedItem.Metric.Namespace, pod)
						continue
					}
					ramBytes := ramAllocatedItem.Value.Value
					runMinutes := podData.Minutes
					if runMinutes <= 0 {
						t.Logf("Namespace: %s, Pod %s has a run duration of 0 minutes, skipping RAM allocation calculation", podData.Namespace, podData.Pod)
						continue
					}

					runHours := ConvertToHours(runMinutes)
					podData.Containers[container] = &ContainerRAMData{
						Container:              container,
						RAMBytesHours:          ramBytes * runHours,
						RAMBytes:               ramBytes,
						RAMBytesRequestAverage: 0.0,
					}
				}

				// ----------------------------------------------
				// Gather RAMRequestAverage (RAMRequested) for every container in a Pod

				// Iterate over results and group by pod
				// ----------------------------------------------
				for _, ramRequestedItem := range requestedRAM.Data.Result {
					container := ramRequestedItem.Metric.Container
					pod := ramRequestedItem.Metric.Pod
					if container == "" {
						t.Logf("Skipping RAM allocation for empty container in pod %s in namespace: %s", pod, ramRequestedItem.Metric.Namespace)
						continue
					}
					podData, ok := podMap[pod]
					if !ok {
						t.Logf("Failed to find namespace: %s and pod: %s in RAM allocated results", ramRequestedItem.Metric.Namespace, pod)
						continue
					}

					ramBytesRequestedAverage := ramRequestedItem.Value.Value

					runMinutes := podData.Minutes
					if runMinutes <= 0 {
						t.Logf("Namespace: %s, Pod %s has a run duration of 0 minutes, skipping RAM allocation calculation", podData.Namespace, podData.Pod)
						continue
					}

					runHours := ConvertToHours(runMinutes)

					// if the container exists, you need to apply the opencost cost specification
					if containerData, ok := podData.Containers[container]; ok {
						if containerData.RAMBytes < ramBytesRequestedAverage {
							containerData.RAMBytes = ramBytesRequestedAverage
							containerData.RAMBytesHours = ramBytesRequestedAverage * runHours
						}
					} else {
						podData.Containers[container] = &ContainerRAMData{
							Container:     container,
							RAMBytesHours: ramBytesRequestedAverage * runHours,
						}
					}

					podData.Containers[container].RAMBytesRequestAverage = ramBytesRequestedAverage
				}

				// ----------------------------------------------
				// Aggregate Container results to get Pod Output and Aggregate Pod Output to get Namespace results

				// Aggregating the AVG RAM values is not as simple as just summing them up because we have to consider that
				// each pod's average ram data is relative to that same pod's lifetime. So, in order to aggregate the data
				// together, we have to expand the averages back into their pure byte values, merge the run times, sum the
				// raw values, and then REAPPLY the merged run time. See core/pkg/opencost/allocation.go "add()" line #1225
				// NOTE: This is only for the average RAM values, RAMByteHours can be summed directly.
				// ----------------------------------------------
				nsRAMBytesRequest := 0.0
				nsRAMBytesHours := 0.0
				nsRAMBytes := 0.0
				nsMinutes := 0.0
				var nsStart, nsEnd time.Time

				for _, podData := range podMap {

					start := podData.Start
					end := podData.End
					minutes := podData.Minutes

					ramByteRequest := 0.0
					ramByteHours := 0.0

					for _, containerData := range podData.Containers {
						ramByteHours += containerData.RAMBytesHours
						ramByteRequest += containerData.RAMBytesRequestAverage
					}
					// t.Logf("Pod %s, ramByteHours %v", podData.Pod, ramByteHours)
					// Sum up Pod Values
					nsRAMBytesRequest += (ramByteRequest*minutes + nsRAMBytesRequest*nsMinutes)
					nsRAMBytesHours += ramByteHours

					// only the first time
					if nsStart.IsZero() && nsEnd.IsZero() {
						nsStart = start
						nsEnd = end
						nsMinutes = nsEnd.Sub(nsStart).Minutes()
						nsHours := ConvertToHours(nsMinutes)
						nsRAMBytes = nsRAMBytesHours / nsHours
						nsRAMBytesRequest = nsRAMBytesRequest / nsMinutes
						continue
					} else {
						if start.Before(nsStart) {
							nsStart = start
						}
						if end.After(nsEnd) {
							nsEnd = end
						}
						nsMinutes = nsEnd.Sub(nsStart).Minutes()
						nsHours := ConvertToHours(nsMinutes)
						nsRAMBytes = nsRAMBytesHours / nsHours
						nsRAMBytesRequest = nsRAMBytesRequest / nsMinutes
					}
				}

				if nsMinutes < ShortLivedPodsRunTime {
					// Too short of a run time to assert results. ByteHours is very sensitive to run time.
					continue
				}
				// ----------------------------------------------
				// Compare Results with Allocation
				// ----------------------------------------------
				t.Logf("Namespace: %s", namespace)
				// 5 % Tolerance
				withinRange, diff_percent := utils.AreWithinPercentage(nsRAMBytes, allocationResponseItem.RAMBytes, Tolerance)
				if withinRange {
					t.Logf("    - RAMBytes[Pass]: ~%.2f", nsRAMBytes)
				} else {
					t.Errorf("    - RAMBytes[Fail]: DifferencePercent: %0.2f, Prom Results: %.2f, API Results: %.2f", diff_percent, nsRAMBytes, allocationResponseItem.RAMBytes)
				}
				withinRange, diff_percent = utils.AreWithinPercentage(nsRAMBytesHours, allocationResponseItem.RAMByteHours, Tolerance)
				if withinRange {
					t.Logf("    - RAMByteHours[Pass]: ~%.2f", nsRAMBytesHours)
				} else {
					t.Errorf("    - RAMByteHours[Fail]: DifferencePercent: %0.2f, Prom Results: %.2f, API Results: %.2f", diff_percent, nsRAMBytesHours, allocationResponseItem.RAMByteHours)
				}
				withinRange, diff_percent = utils.AreWithinPercentage(nsRAMBytesRequest, allocationResponseItem.RAMBytesRequestAverage, Tolerance)
				if withinRange {
					t.Logf("    - RAMByteRequestAverage[Pass]: ~%.2f", nsRAMBytesRequest)
				} else {
					t.Errorf("    - RAMByteRequestAverage[Fail]: DifferencePercent: %0.2f, Prom Results: %.2f, API Results: %.2f", diff_percent, nsRAMBytesRequest, allocationResponseItem.RAMBytesRequestAverage)
				}
			}
		})
	}
}
