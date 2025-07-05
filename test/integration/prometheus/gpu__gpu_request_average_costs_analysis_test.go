package prometheus

// Description: Checks all GPU related Costs
// - GPUCores
// - GPUCoresHours
// - GPUCoresRequestAverage
// - GPUMaxUsage

// Testing Methodology
// 1. Query GPUAllocated and GPURequested in prometheus
//     1.1 Use the current "time" as upperbound in promql
//     1.2 Query by (container, pod, node, namespace)
// 2. Consolidate containers based on pods
//     2.1 GPUCore is the max of GPUCoreAllocated and GPUCoreRequested for each container
//     2.2 Query [24h:5m] to get 288 (1440/5) points to identify the StartTime and EndTime DataPoint Timestamp.
// 	   2.3 Assumption 1: Identify the time range for the pod (all containers within the pod have the same time range)
// 3. Consolidate GPUCores based on pod and then based on namespace
// 4. Fetch /allocation data aggregated by namespace
// 5. Compare results with a 5% error margin.

import (
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
	"github.com/opencost/opencost-integration-tests/pkg/utils"
	"testing"
	"time"
)

const tolerance = 0.05

func ConvertToHours(minutes float64) float64 {
	// Convert Time from Minutes to Hours
	return minutes / 60
}

func TestGPUCosts(t *testing.T) {
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
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			// ----------------------------------------------
			// Allocation API Data Collection
			// ----------------------------------------------
			// /compute/allocation: GPU Costs for all namespaces
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
				queryStart := queryEnd.Add(-24 * time.Hour)
				window24h := api.Window{
					Start: queryStart,
					End:   queryEnd,
				}
				// Note that in the Pod Query, we use a 5m resolution [THIS IS THE DEFAULT VALUE IN OPENCOST]
				resolution := 5 * time.Minute

				// Query End Time for all Queries
				endTime := queryEnd.Unix()

				// Metric: GPURequests
				// avg(avg_over_time(
				// 		kube_pod_container_resource_requests{
				// 			resource=""nvidia_com_gpu", container!="", container!="POD", node!=""
				// 		}[24h])
				// )
				// by
				// (container, pod, namespace, node)

				// Q) What about Cluster Filter and Cluster Label?
				promGPURequestedInput := prometheus.PrometheusInput{}

				promGPURequestedInput.Metric = "kube_pod_container_resource_requests"
				promGPURequestedInput.Filters = map[string]string{
					// "job": "opencost", Averaging all results negates this process
					"resource":  "nvidia_com_gpu",
					"namespace": namespace,
				}
				promGPURequestedInput.IgnoreFilters = map[string][]string{
					"container": {"", "POD"},
					"node":      {""},
				}
				promGPURequestedInput.AggregateBy = []string{"container", "pod", "namespace", "node"}
				promGPURequestedInput.Function = []string{"avg_over_time", "avg"}
				promGPURequestedInput.QueryWindow = tc.window
				promGPURequestedInput.Time = &endTime

				requestedGPU, err := client.RunPromQLQuery(promGPURequestedInput)
				if err != nil {
					t.Fatalf("Error while calling Prometheus API %v", err)
				}

				// Metric: GPUAllocated
				// avg(avg_over_time(
				// 		container_gpu_allocation{
				// 			container!="", container!="POD", node!=""
				// 		}[24h])
				// )
				// by
				// (container, pod, namespace, node)

				// Q) What about Cluster Filter and Cluster Label?
				promGPUAllocatedInput := prometheus.PrometheusInput{}

				promGPUAllocatedInput.Metric = "container_gpu_allocation"
				promGPUAllocatedInput.Filters = map[string]string{
					// "job": "opencost", Averaging all results negates this process
					"namespace": namespace,
				}
				promGPUAllocatedInput.IgnoreFilters = map[string][]string{
					"container": {"", "POD"},
					"node":      {""},
				}
				promGPUAllocatedInput.AggregateBy = []string{"container", "pod", "namespace", "node"}
				promGPUAllocatedInput.Function = []string{"avg_over_time", "avg"}
				promGPUAllocatedInput.QueryWindow = tc.window
				promGPUAllocatedInput.Time = &endTime

				allocatedGPU, err := client.RunPromQLQuery(promGPUAllocatedInput)
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
				promPodInfoInput.AggregateWindow = tc.window
				promPodInfoInput.AggregateResolution = "5m"
				promPodInfoInput.Time = &endTime

				podInfo, err := client.RunPromQLQuery(promPodInfoInput)
				if err != nil {
					t.Fatalf("Error while calling Prometheus API %v", err)
				}

				// Define Local Struct for PoD Data Consolidation
				// PoD is composed of multiple containers, we want to represent all that information succinctly here
				type ContainerGPUData struct {
					Container              string
					GPUCoresRequestAverage float64
					GPUCores               float64
					GPUCoresHours          float64
				}

				type PodData struct {
					Pod        string
					Namespace  string
					Start      time.Time
					End        time.Time
					Minutes    float64
					Containers map[string]*ContainerGPUData
				}

				// ----------------------------------------------
				// Identify All Pods and Containers for that Pod

				// Create a map of PodData for each pod, and calculate the runtime.
				// The query we make for pods is _NOT_ an average over time "range" vector. Instead, it's a range vector
				// that returns _ALL_ samples for the pod over the last 24 hours. So, when you see <metric>[24h:5m] the
				// number after the ':' (5m) is the resolution of the samples. So for the pod query, we get 24h / 5m
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
						Containers: make(map[string]*ContainerGPUData),
					}
				}
				// ----------------------------------------------
				// Gather GPUCores (GPUAllocated) for every container in a Pod

				// Iterate over results and group by pod
				// ----------------------------------------------

				for _, GPUAllocatedItem := range allocatedGPU.Data.Result {
					container := GPUAllocatedItem.Metric.Container
					pod := GPUAllocatedItem.Metric.Pod
					if container == "" {
						t.Logf("Skipping GPU allocation for empty container in pod %s in namespace: %s", pod, GPUAllocatedItem.Metric.Namespace)
						continue
					}
					podData, ok := podMap[pod]
					if !ok {
						t.Logf("Failed to find namespace: %s and pod: %s in GPU allocated results", GPUAllocatedItem.Metric.Namespace, pod)
						continue
					}
					GPUCores := GPUAllocatedItem.Value.Value
					runMinutes := podData.Minutes
					if runMinutes <= 0 {
						t.Logf("Namespace: %s, Pod %s has a run duration of 0 minutes, skipping GPU allocation calculation", podData.Namespace, podData.Pod)
						continue
					}

					runHours := ConvertToHours(runMinutes)
					podData.Containers[container] = &ContainerGPUData{
						Container:              container,
						GPUCoresHours:          GPUCores * runHours,
						GPUCores:               GPUCores,
						GPUCoresRequestAverage: 0.0,
					}
				}

				// ----------------------------------------------
				// Gather GPURequestAverage (GPURequested) for every container in a Pod

				// Iterate over results and group by pod
				// ----------------------------------------------
				for _, GPURequestedItem := range requestedGPU.Data.Result {
					container := GPURequestedItem.Metric.Container
					pod := GPURequestedItem.Metric.Pod
					if container == "" {
						t.Logf("Skipping GPU allocation for empty container in pod %s in namespace: %s", pod, GPURequestedItem.Metric.Namespace)
						continue
					}
					podData, ok := podMap[pod]
					if !ok {
						t.Logf("Failed to find namespace: %s and pod: %s in GPU allocated results", GPURequestedItem.Metric.Namespace, pod)
						continue
					}

					GPUCoresRequestedAverage := GPURequestedItem.Value.Value

					runMinutes := podData.Minutes
					if runMinutes <= 0 {
						t.Logf("Namespace: %s, Pod %s has a run duration of 0 minutes, skipping GPU allocation calculation", podData.Namespace, podData.Pod)
						continue
					}

					runHours := ConvertToHours(runMinutes)

					// if the container exists, you need to apply the opencost cost specification
					if containerData, ok := podData.Containers[container]; ok {
						if containerData.GPUCores < GPUCoresRequestedAverage {
							containerData.GPUCores = GPUCoresRequestedAverage
							containerData.GPUCoresHours = GPUCoresRequestedAverage * runHours
						}
					} else {
						podData.Containers[container] = &ContainerGPUData{
							Container:     container,
							GPUCoresHours: GPUCoresRequestedAverage * runHours,
						}
					}

					podData.Containers[container].GPUCoresRequestAverage = GPUCoresRequestedAverage
				}

				// ----------------------------------------------
				// Aggregate Container results to get Pod Output and Aggregate Pod Output to get Namespace results

				// Aggregating the AVG GPU values is not as simple as just summing them up because we have to consider that
				// each pod's average GPU data is relative to that same pod's lifetime. So, in order to aggregate the data
				// together, we have to expand the averages back into their pure Core values, merge the run times, sum the
				// raw values, and then REAPPLY the merged run time. See core/pkg/opencost/allocation.go "add()" line #1225
				// NOTE: This is only for the average GPU values, GPUCoreHours can be summed directly.
				// ----------------------------------------------
				nsGPUCoresRequest := 0.0
				nsGPUCoresHours := 0.0
				// nsGPUCores := 0.0
				nsMinutes := 0.0
				var nsStart, nsEnd time.Time

				for _, podData := range podMap {

					start := podData.Start
					end := podData.End
					minutes := podData.Minutes

					GPUCoreRequest := 0.0
					GPUCoreHours := 0.0

					for _, containerData := range podData.Containers {
						GPUCoreHours += containerData.GPUCoresHours
						GPUCoreRequest += containerData.GPUCoresRequestAverage
					}
					// t.Logf("Pod %s, GPUCoreHours %v", podData.Pod, GPUCoreHours)
					// Sum up Pod Values
					nsGPUCoresRequest += (GPUCoreRequest*minutes + nsGPUCoresRequest*nsMinutes)
					nsGPUCoresHours += GPUCoreHours

					// only the first time
					if nsStart.IsZero() && nsEnd.IsZero() {
						nsStart = start
						nsEnd = end
						nsMinutes = nsEnd.Sub(nsStart).Minutes()
						// nsHours := ConvertToHours(nsMinutes)
						// nsGPUCores = nsGPUCoresHours / nsHours
						nsGPUCoresRequest = nsGPUCoresRequest / nsMinutes
						continue
					} else {
						if start.Before(nsStart) {
							nsStart = start
						}
						if end.After(nsEnd) {
							nsEnd = end
						}
						nsMinutes = nsEnd.Sub(nsStart).Minutes()
						// nsHours := ConvertToHours(nsMinutes)
						// nsGPUCores = nsGPUCoresHours / nsHours
						nsGPUCoresRequest = nsGPUCoresRequest / nsMinutes
					}
				}

				// ----------------------------------------------
				// Compare Results with Allocation
				// ----------------------------------------------
				t.Logf("Namespace: %s", namespace)
				// 5 % Tolerance
				withinRange, diff_percent := utils.AreWithinPercentage(nsGPUCoresHours, allocationResponseItem.GPUHours, tolerance)
				if withinRange {
					t.Logf("    - GPUCoreHours[Pass]: ~%.2f", nsGPUCoresHours)
				} else {
					t.Errorf("    - GPUCoreHours[Fail]: DifferencePercent: %0.2f, Prom Results: %.2f, API Results: %.2f", diff_percent, nsGPUCoresHours, allocationResponseItem.GPUHours)
				}
				withinRange, diff_percent = utils.AreWithinPercentage(nsGPUCoresRequest, allocationResponseItem.GPUAllocation.GPURequestAverage, tolerance)
				if withinRange {
					t.Logf("    - GPUCoreRequestAverage[Pass]: ~%.2f", nsGPUCoresRequest)
				} else {
					t.Errorf("    - GPUCoreRequestAverage[Fail]: DifferencePercent: %0.2f, Prom Results: %.2f, API Results: %.2f", diff_percent, nsGPUCoresRequest, allocationResponseItem.GPUAllocation.GPURequestAverage)
				}
			}
		})
	}
}
