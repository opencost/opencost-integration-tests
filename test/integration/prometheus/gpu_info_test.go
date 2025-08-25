package prometheus

// Description - Assert GPU Information

import (
	// "fmt"
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
	"testing"
	"time"
)

const Tolerance = 0.05

func TestGPUInfo(t *testing.T) {
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
			aggregate:  "pod,container",
			accumulate: "false",
		},
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			type PodGPUData struct {
				Pod       string
				Namespace string
				Device    string
				ModelName string
				UUID      string
				// PCIBusID  			string
				// DCGMFIDriverVersion string
			}
			// To store GPU Specific Information
			podPromGPUMap := make(map[string]*PodGPUData)

			queryEnd := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
			endTime := queryEnd.Unix()

			////////////////////////////////////////////////////////////////////////////
			// GPU Info
			// avg(avg_over_time(DCGM_FI_DEV_DEC_UTIL
			//     container!="", %s}[%s]))
			// by
			// (container, pod, namespace, device, modelName, UUID, %s)
			////////////////////////////////////////////////////////////////////////////
			client := prometheus.NewClient()
			promInput := prometheus.PrometheusInput{
				Metric: "DCGM_FI_DEV_DEC_UTIL",
			}
			ignoreFilters := map[string][]string{
				"container": {""},
			}
			promInput.Function = []string{"avg_over_time", "avg"}
			promInput.QueryWindow = tc.window
			promInput.IgnoreFilters = ignoreFilters
			promInput.AggregateBy = []string{"container", "pod", "namespace", "device", "modelName", "UUID"}
			promInput.Time = &endTime

			promResponse, err := client.RunPromQLQuery(promInput)
			// Do we need container_name and pod_name
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			for _, promResponseItem := range promResponse.Data.Result {

				podPromGPUMap[promResponseItem.Metric.Container] = &PodGPUData{
					Pod:       promResponseItem.Metric.Pod,
					Namespace: promResponseItem.Metric.Namespace,
					Device:    promResponseItem.Metric.Device,
					ModelName: promResponseItem.Metric.ModelName,
					UUID:      promResponseItem.Metric.UUID,
				}
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

			podAllocGPUMap := make(map[string]*PodGPUData)
			for _, allocationResponseItem := range apiResponse.Data[0] {
				_, ok := allocationResponseItem.Properties.Labels["nvidia_com_gpu_count"]
				if !ok {
					continue
				}
				// Is there a better way to check if GPU's were assigned to a pod?
				if allocationResponseItem.GPUAllocation.GPUDevice == "" {
					continue
				}
				podAllocGPUMap[allocationResponseItem.Properties.Container] = &PodGPUData{
					Pod:       allocationResponseItem.Properties.Pod,
					Namespace: allocationResponseItem.Properties.Namespace,
					Device:    allocationResponseItem.GPUAllocation.GPUDevice,
					ModelName: allocationResponseItem.GPUAllocation.GPUModel,
					UUID:      allocationResponseItem.GPUAllocation.GPUUUID,
				}
			}

			// Optionally print the missing containers, How about a verbose flag?
			if len(podPromGPUMap) != len(podAllocGPUMap) {
				t.Errorf("Prometheus GPU Containers:\n%v\nAllocation GPU Containers:\n%v", podPromGPUMap, podAllocGPUMap)
				t.Fatalf("Number of Containers from Prometheus (%d) and Allocation (%d) don't match.", len(podPromGPUMap), len(podAllocGPUMap))
			}

			for container, containerPromGPUInfo := range podPromGPUMap {
				containerAllocGPUInfo, ok := podAllocGPUMap[container]
				if !ok {
					t.Fatalf("Container %s is present in Prometheus result but not in Allocation", container)
				}
				// Device Name will not Match "nvidia0" vs "nvidia"
				t.Logf("Container %s", container)
				t.Logf("Pod %s", containerPromGPUInfo.Pod)
				t.Logf("Namespace %s", containerPromGPUInfo.Namespace)
				if containerAllocGPUInfo.ModelName != containerPromGPUInfo.ModelName {
					t.Errorf("  - [Fail] Prom ModelName %s != Alloc ModelName %s", containerPromGPUInfo.ModelName, containerAllocGPUInfo.ModelName)
				} else {
					t.Logf("  - [Pass] ModelName: %s", containerPromGPUInfo.ModelName)
				}
				if containerAllocGPUInfo.UUID != containerPromGPUInfo.UUID {
					t.Errorf("  - [Fail] Prom UUID %s != Alloc UUID %s", containerPromGPUInfo.UUID, containerAllocGPUInfo.UUID)
				} else {
					t.Logf("  - [Pass] UUID: %s", containerPromGPUInfo.UUID)
				}
			}
		})
	}
}
