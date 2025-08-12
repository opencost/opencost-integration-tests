package prometheus

// Description - Checks for GPU Count

import (
	// "fmt"
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
	"testing"
	"time"
)

const tolerance = 0.05

func TestGPUCount(t *testing.T) {

	testCases := []struct {
		name       string
		window     string
		aggregate  string
		accumulate string
	}{
		{
			name:       "Yesterday",
			window:     "24h",
			aggregate:  "node",
			accumulate: "false",
		},
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			// Use this information to find start and end time of pod
			queryEnd := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
			endTime := queryEnd.Unix()

			client := prometheus.NewClient()
			// Node GPU Count
			promNodeGPUCountInput := prometheus.PrometheusInput{}
			promNodeGPUCountInput.Metric = "node_gpu_count"
			promNodeGPUCountInput.Function = []string{"avg_over_time"}
			promNodeGPUCountInput.QueryWindow = tc.window
			promNodeGPUCountInput.Time = &endTime

			nodeGPUCount, err := client.RunPromQLQuery(promNodeGPUCountInput)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			type NodeData struct {
				Node       		string
				PromGPUCount 	float64
				AssetGPUCount	float64
				AllocGPUCount	float64
			}

			nodeMap := make(map[string]*NodeData)

			for _, nodeGPUCountResponseItem := range nodeGPUCount.Data.Result {
				node := nodeGPUCountResponseItem.Metric.Node
				count := nodeGPUCountResponseItem.Value.Value

				if node == "" {
					continue
				}

				nodeMap[node] = &NodeData{
					Node: node,
					PromGPUCount: count,
				}
			}

			/////////////////////////////////////////////
			// API Client
			/////////////////////////////////////////////

			apiObj := api.NewAPI()
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

			for node, allocationResponseItem := range apiResponse.Data[0] {
				if node == "" {
					continue
				}
				nodeItem, ok := nodeMap[node]
				if !ok {
					t.Logf("Node %s missing from Allocation", node)
					continue
				}
				nodeItem.AllocGPUCount = allocationResponseItem.GPUCount
			}

			assetResponse, err := apiObj.GetAssets(api.AssetsRequest{
				Window:     tc.window,
				Filter:		tc.aggregate,
			})

			if err != nil {
				t.Fatalf("Error while calling Allocation API %v", err)
			}
			if assetResponse.Code != 200 {
				t.Errorf("API returned non-200 code")
			}

			for _, assetResponseItem := range assetResponse.Data {
				node := assetResponseItem.Properties.Name
				if node == "" {
					continue
				}
				nodeItem, ok := nodeMap[node]
				if !ok {
					t.Logf("Node %s missing from Allocation", node)
					continue
				}
				nodeItem.AssetGPUCount = assetResponseItem.GPUCount
			}

			t.Logf("\nGPU Count for Node.\n")
			// Windows are not accurate for prometheus and allocation
			for node, nodegpuCountInfo := range nodeMap {
				t.Logf("Node %s", node)
				
				if (nodegpuCountInfo.PromGPUCount == nodegpuCountInfo.AssetGPUCount) && (nodegpuCountInfo.PromGPUCount == nodegpuCountInfo.AllocGPUCount) {
					t.Logf("  - NodeGPUCount[Pass]: %f", nodegpuCountInfo.PromGPUCount)
				} else {
					t.Errorf("  - NodeGPUCount[Fail]: Prom - %f, Alloc - %f, Asset - %f", nodegpuCountInfo.PromGPUCount, nodegpuCountInfo.AssetGPUCount, nodegpuCountInfo.AllocGPUCount)
				}
			}
		})
	}
}
