package prometheus

// Description: Checks all PV related Costs
// - PVBytes
// - PVBytesHours
// - PVBytesRequestAverage
// - PVMaxUsage

// Testing Methodology
// 1. Query PVAllocated and PVRequested in prometheus
//     1.1 Use the current "time" as upperbound in promql
//     1.2 Query by (container, pod, node, namespace)
// 2. Consolidate containers based on pods
//     2.1 PVByte is the max of PVByteAllocated and PVByteRequested for each container
//     2.2 Query [24h:5m] to get 288 (1440/5) points to identify the StartTime and EndTime DataPoint Timestamp.
// 	   2.3 Assumption 1: Identify the time range for the pod (all containers within the pod have the same time range)
// 3. Consolidate PVBytes based on pod and then based on namespace
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

func TestPVCosts(t *testing.T) {
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
			// /compute/allocation: PV Costs for all namespaces
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

				// Metric: PVRequests
				// avg(avg_over_time(pod_pvc_allocation{%s}[%s])) 
				// by 
				// (persistentvolume, persistentvolumeclaim, pod, namespace, %s)

				// Q) What about Cluster Filter and Cluster Label?
				promPVRequestedInput := prometheus.PrometheusInput{}

				promPVRequestedInput.Metric = "pod_pvc_allocation"
				promPVRequestedInput.Filters = map[string]string{
					"namespace": namespace,
				}
				promPVRequestedInput.AggregateBy = []string{"persistentvolume", "persistentvolumeclaim", "pod", "namespace"}
				promPVRequestedInput.Function = []string{"avg_over_time", "avg"}
				promPVRequestedInput.QueryWindow = tc.window
				promPVRequestedInput.Time = &endTime

				requestedPV, err := client.RunPromQLQuery(promPVRequestedInput)
				if err != nil {
					t.Fatalf("Error while calling Prometheus API %v", err)
				}

				// Metric: PVAllocated
				// avg(avg_over_time(kube_persistentvolumeclaim_resource_requests_storage_bytes{%s}[%s])) 
				// by 
				// (persistentvolumeclaim, namespace, %s)

				// Q) What about Cluster Filter and Cluster Label?
				promPVAllocatedInput := prometheus.PrometheusInput{}

				promPVAllocatedInput.Metric = "kube_persistentvolumeclaim_resource_requests_storage_bytes"
				promPVAllocatedInput.Filters = map[string]string{
					// "job": "opencost", Averaging all results negates this process
					"namespace": namespace,
				}
				promPVAllocatedInput.AggregateBy = []string{"persistentvolumeclaim", "pod", "namespace"}
				promPVAllocatedInput.Function = []string{"avg_over_time", "avg"}
				promPVAllocatedInput.QueryWindow = tc.window
				promPVAllocatedInput.Time = &endTime

				allocatedPV, err := client.RunPromQLQuery(promPVAllocatedInput)
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
				type ContainerPVData struct {
					Container              string
					PVBytesRequestAverage float64
					PVBytes               float64
					PVBytesHours          float64
				}

				type PodData struct {
					Pod        string
					Namespace  string
					Start      time.Time
					End        time.Time
					Minutes    float64
					Containers map[string]*ContainerPVData
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
						Containers: make(map[string]*ContainerPVData),
					}
				}
				// ----------------------------------------------
				// Gather PVBytes (PVAllocated) for every container in a Pod

				// Iterate over results and group by pod
				// ----------------------------------------------

				for _, pvAllocatedItem := range allocatedPV.Data.Result {
					container := pvAllocatedItem.Metric.Container
					pod := pvAllocatedItem.Metric.Pod
					if container == "" {
						t.Logf("Skipping PV allocation for empty container in pod %s in namespace: %s", pod, pvAllocatedItem.Metric.Namespace)
						continue
					}
					podData, ok := podMap[pod]
					if !ok {
						t.Logf("Failed to find namespace: %s and pod: %s in PV allocated results", pvAllocatedItem.Metric.Namespace, pod)
						continue
					}
					pvBytes := pvAllocatedItem.Value.Value
					runMinutes := podData.Minutes
					if runMinutes <= 0 {
						t.Logf("Namespace: %s, Pod %s has a run duration of 0 minutes, skipping PV allocation calculation", podData.Namespace, podData.Pod)
						continue
					}

					runHours := ConvertToHours(runMinutes)
					podData.Containers[container] = &ContainerPVData{
						Container:              container,
						PVBytesHours:          pvBytes * runHours,
						PVBytes:               pvBytes,
						PVBytesRequestAverage: 0.0,
					}
				}

				// ----------------------------------------------
				// Gather PVRequestAverage (PVRequested) for every container in a Pod

				// Iterate over results and group by pod
				// ----------------------------------------------
				for _, pvRequestedItem := range requestedPV.Data.Result {
					container := pvRequestedItem.Metric.Container
					pod := pvRequestedItem.Metric.Pod
					if container == "" {
						t.Logf("Skipping PV allocation for empty container in pod %s in namespace: %s", pod, pvRequestedItem.Metric.Namespace)
						continue
					}
					podData, ok := podMap[pod]
					if !ok {
						t.Logf("Failed to find namespace: %s and pod: %s in PV allocated results", pvRequestedItem.Metric.Namespace, pod)
						continue
					}

					pvBytesRequestedAverage := pvRequestedItem.Value.Value

					runMinutes := podData.Minutes
					if runMinutes <= 0 {
						t.Logf("Namespace: %s, Pod %s has a run duration of 0 minutes, skipping PV allocation calculation", podData.Namespace, podData.Pod)
						continue
					}

					runHours := ConvertToHours(runMinutes)

					// if the container exists, you need to apply the opencost cost specification
					if containerData, ok := podData.Containers[container]; ok {
						if containerData.PVBytes < pvBytesRequestedAverage {
							containerData.PVBytes = pvBytesRequestedAverage
							containerData.PVBytesHours = pvBytesRequestedAverage * runHours
						}
					} else {
						podData.Containers[container] = &ContainerPVData{
							Container:     container,
							PVBytesHours: pvBytesRequestedAverage * runHours,
						}
					}

					podData.Containers[container].PVBytesRequestAverage = pvBytesRequestedAverage
				}

				// ----------------------------------------------
				// Aggregate Container results to get Pod Output and Aggregate Pod Output to get Namespace results

				// Aggregating the AVG PV values is not as simple as just summing them up because we have to consider that
				// each pod's average pv data is relative to that same pod's lifetime. So, in order to aggregate the data
				// together, we have to expand the averages back into their pure byte values, merge the run times, sum the
				// raw values, and then REAPPLY the merged run time. See core/pkg/opencost/allocation.go "add()" line #1225
				// NOTE: This is only for the average PV values, PVByteHours can be summed directly.
				// ----------------------------------------------
				nsPVBytesRequest := 0.0
				nsPVBytesHours := 0.0
				nsPVBytes := 0.0
				nsMinutes := 0.0
				var nsStart, nsEnd time.Time

				for _, podData := range podMap {

					start := podData.Start
					end := podData.End
					minutes := podData.Minutes

					pvByteRequest := 0.0
					pvByteHours := 0.0

					for _, containerData := range podData.Containers {
						pvByteHours += containerData.PVBytesHours
						pvByteRequest += containerData.PVBytesRequestAverage
					}
					// t.Logf("Pod %s, pvByteHours %v", podData.Pod, pvByteHours)
					// Sum up Pod Values
					nsPVBytesRequest += (pvByteRequest*minutes + nsPVBytesRequest*nsMinutes)
					nsPVBytesHours += pvByteHours

					// only the first time
					if nsStart.IsZero() && nsEnd.IsZero() {
						nsStart = start
						nsEnd = end
						nsMinutes = nsEnd.Sub(nsStart).Minutes()
						nsHours := ConvertToHours(nsMinutes)
						nsPVBytes = nsPVBytesHours / nsHours
						nsPVBytesRequest = nsPVBytesRequest / nsMinutes
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
						nsPVBytes = nsPVBytesHours / nsHours
						nsPVBytesRequest = nsPVBytesRequest / nsMinutes
					}
				}

				// ----------------------------------------------
				// Compare Results with Allocation
				// ----------------------------------------------
				t.Logf("Namespace: %s", namespace)
				// 5 % Tolerance
				withinRange, diff_percent := utils.AreWithinPercentage(nsPVBytes, allocationResponseItem.PVBytes, tolerance)
				if withinRange {
					t.Logf("    - PVBytes[Pass]: ~%.2f", nsPVBytes)
				} else {
					t.Errorf("    - PVBytes[Fail]: DifferencePercent: %0.2f, Prom Results: %.2f, API Results: %.2f", diff_percent, nsPVBytes, allocationResponseItem.PVBytes)
				}
				withinRange, diff_percent = utils.AreWithinPercentage(nsPVBytesHours, allocationResponseItem.PVByteHours, tolerance)
				if withinRange {
					t.Logf("    - PVByteHours[Pass]: ~%.2f", nsPVBytesHours)
				} else {
					t.Errorf("    - PVByteHours[Fail]: DifferencePercent: %0.2f, Prom Results: %.2f, API Results: %.2f", diff_percent, nsPVBytesHours, allocationResponseItem.PVByteHours)
				}
			}
		})
	}
}
