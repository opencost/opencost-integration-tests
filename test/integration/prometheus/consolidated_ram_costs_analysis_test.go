package prometheus

// Description: Checks all RAM related Costs
// - RAMBytes
// - RAMByteHours
// - RAMMaxUsage
// Also processes RAMByteRequestAverage and RAMByteAllocated for RAMByteHours calculation.

// Testing Methodology
// 1. Query RAMAllocated and RAMRequested in prometheus
//     1.1 Use the current "time" as upperbound in promql
//     1.2 Query by (container, pod, namespace)
// 2. Consolidate containers based on pods
//     2.1 RAMByte is the max of RAMByteAllocated and RAMByteRequested
//     2.2 Query [24h:5m] to get 288 (1440/5) points. 
// 	   2.3 Assumption 1: Identify the time range for the pod (all containers within the pod have the same time range)
// 3. Consolidate RAMBytes based on pod and then based on namespace
// 4. Fetch /allocation data aggregated by namespace
// 5. Compare results with a 2% error margin.


import (
	// "fmt"
	"time"
	"testing"
	// "strconv"
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
)

func ConvertToHours(minutes float64) (float64) {
	// Convert Time from Minutes to Hours
	return minutes / 60
}

func TestRAMByteCosts(t *testing.T) {
	apiObj := api.NewAPI()

	// test for more windows
	testCases := []struct {
		name		string
		window      string
		aggregate   string
		accumulate  string
	}{
		{
			name: "Yesterday",
			window: "24h",
			aggregate: "namespace",
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
				Window: tc.window,
				Aggregate: tc.aggregate,
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
					"resource": "memory",
					"unit": "byte",
					"namespace": namespace,
				}
				promRAMRequestedInput.IgnoreFilters = map[string][]string{
					"container": {"", "POD"},
					"node": {""},
				}
				promRAMRequestedInput.AggregateBy = []string{"container", "pod", "namespace", "node"}
				promRAMRequestedInput.Function = []string{"avg_over_time", "avg"}
				promRAMRequestedInput.QueryWindow = tc.window

				requestedRAM, err := client.RunPromQLQuery(promRAMRequestedInput)
				if err != nil {
					t.Fatalf("Error while calling Prometheus API %v", err)
				}

				// Metric: RAMAllocated
				// avg(avg_over_time(
				// 		kube_pod_container_resource_requests{
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
					"unit": "byte",
					"namespace": namespace,
				}
				promRAMAllocatedInput.IgnoreFilters = map[string][]string{
					"container": {"", "POD"},
					"node": {""},
				}
				promRAMAllocatedInput.AggregateBy = []string{"container", "pod", "namespace", "node"}
				promRAMAllocatedInput.Function = []string{"avg_over_time", "avg"}
				promRAMAllocatedInput.QueryWindow = tc.window
				
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
				promPodInfoInput.MetricNotEqualTo = "0"
				promPodInfoInput.AggregateBy = []string{"container", "pod", "namespace", "node"}
				promPodInfoInput.Function = []string{"avg"}
				promPodInfoInput.AggregateWindow = tc.window
				promPodInfoInput.AggregateResolution = "5m"

				podInfo, err := client.RunPromQLQuery(promPodInfoInput)
				if err != nil {
					t.Fatalf("Error while calling Prometheus API %v", err)
				}

				// Use this information to find start and end time of pod
				queryEnd := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
				queryStart := queryEnd.Add(-24 * time.Hour)
				window24h := api.Window{
					Start: queryStart, 
					End: queryEnd,
				}
				resolution := 5 * time.Minute

				// Define Local Struct for PoD Data Consolidation
				// PoD is composed of multiple containers, we want to represent all that information succinctly here
				type ContainerRAMData struct {
					Container string
					RAMBytesRequestAverage float64
					RAMBytes float64
					RAMByteHours float64
				}

				type PodData struct {
					Pod string
					Namespace string
					Start time.Time
					End time.Time
					Minutes float64
					Containers map[string]*ContainerRAMData
				}

				// ----------------------------------------------
				// Identify All Pods and Containers for that Pod
				// ----------------------------------------------
				
				// Pointers to modify in place
				podMap := make(map[string]*PodData)
				for _, podInfoResponseItem := range podInfo.Data.Result {
					// Calculate Start and End Time for Pod
					// out of the samples, we are interested in the first and last sample
					s, e := prometheus.CalculateStartAndEnd(podInfoResponseItem.Values, resolution, window24h)

					// Add a key in the podMap for current Pod
					podMap[podInfoResponseItem.Metric.Pod] = &PodData{
						Pod: podInfoResponseItem.Metric.Pod,
						Namespace: namespace,
						Start: s,
						End: e,
						Minutes: e.Sub(s).Minutes(),
						Containers: make(map[string]*ContainerRAMData),
					}
				}


				// ----------------------------------------------
				// Gather RAMBytes (RAMAllocated) for every container in a Pod
				// ----------------------------------------------

				for _, ramAllocatedItem := range allocatedRAM.Data.Result {
					container := ramAllocatedItem.Metric.Container
					if container == "" {
						t.Logf("Skipping RAM allocation for empty container in pod %s in namespace: %s", ramAllocatedItem.Metric.Pod, ramAllocatedItem.Metric.Namespace)
						continue
					}
					podData, ok := podMap[container]
					if !ok {
						t.Logf("Failed to find namespace: %s and pod: %s in RAM allocated results", ramAllocatedItem.Metric.Namespace, ramAllocatedItem.Metric.Pod)
						continue
					}
					valueArray, _ := ramAllocatedItem.Value.(interface{})
					ramBytes, _ := valueArray[1].(int64)
					runMinutes := podData.Minutes
					if runMinutes <= 0 {
						t.Logf("Namespace: %s, Pod %s has a run duration of 0 minutes, skipping RAM allocation calculation", podData.Namespace, podData.Pod)
						continue
					}
					
					runHours := ConvertToHours(runMinutes)
					podData.Containers[container] = &ContainerRAMData{
						Container: container,
						RAMByteHours: ramBytes * runHours,
						RAMBytes: ramBytes,
						RAMBytesRequestAverage: 0.0,
					}
				}


				// ----------------------------------------------
				// Gather RAMRequestAverage (RAMRequested) for every container in a Pod
				// ----------------------------------------------
				for _, ramRequestedItem := range requestedRAM.Data.Result {
					container := ramRequestedItem.Metric.Container
					if container == "" {
						t.Logf("Skipping RAM allocation for empty container in pod %s in namespace: %s", ramRequestedItem.Metric.Pod, ramRequestedItem.Metric.Namespace)
						continue
					}
					podData, ok := podMap[container]
					if !ok {
						t.Logf("Failed to find namespace: %s and pod: %s in RAM allocated results", ramRequestedItem.Metric.Namespace, ramRequestedItem.Metric.Pod)
						continue
					}
					valueArray, _ := ramRequestedItem.Value.(interface{})
					ramBytesRequestedAverage, _ := valueArray[1].(int64)
			
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
							containerData.RAMByteHours = ramBytesRequestedAverage * runHours
						}
					} else {
						podData.Containers[container] = &ContainerRAMData{
							Container:     container,
							RAMByteHours: ramBytesRequestedAverage * runHours,
						}
					}

					podData.Containers[container].RAMBytesRequestAverage = ramBytesRequestedAverage
				}

				// ----------------------------------------------
				// Aggregate Container results to get Pod Output and Aggregate Pod Output to get Namespace results
				// ----------------------------------------------


				nsRAMByteRequest := 0.0
				nsRAMByteHours := 0.0
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
						ramByteHours += containerData.RAMByteHours
						ramByteRequest += containerData.RAMBytesRequestAverage
					}

					// Sum up Pod Values
					nsRAMByteRequest += (ramByteRequest * minutes + nsRAMByteRequest * nsMinutes)
					nsRAMByteHours += ramByteHours
				
					// only the first time
					if nsStart.IsZero() && nsEnd.IsZero() {
						nsStart = start
						nsEnd = end		
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
						nsRAMBytes = nsRAMByteHours / nsHours
						nsRAMByteRequest = nsRAMByteRequest / nsMinutes
					}
				}

				// ----------------------------------------------
				// Compare Results with Allocation
				// ----------------------------------------------
				if nsRAMBytes != allocationResponseItem.RAMBytes {
					t.Errorf("RAM Byte Hours Sum does not match for prometheus %f and /allocation %f for namespace %s", nsRAMBytes, allocationResponseItem.RAMBytes, namespace)	
				} else {
					t.Logf("RAM Byte Hours Match for namespace %s", namespace)	
				}
				if nsRAMByteHours != allocationResponseItem.RAMByteHours {
					t.Errorf("RAM Byte Hours Sum does not match for prometheus %f and /allocation %f for namespace %s", nsRAMByteHours, allocationResponseItem.RAMByteHours, namespace)	
				} else {
					t.Logf("RAM Byte Hours Match for namespace %s", namespace)	
				}
			}
		})
	}
}
