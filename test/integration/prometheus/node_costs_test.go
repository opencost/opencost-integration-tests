package prometheus

// Description - Test Node Codes

import (
	// "fmt"

	"testing"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
	"github.com/opencost/opencost-integration-tests/pkg/utils"
)

const nodeCostTolerance = 0.05
const nodeCostNegligibleCost = 0.01

func TestNodeInfo(t *testing.T) {
	apiObj := api.NewAPI()

	testCases := []struct {
		name        string
		window      string
		aggregate   string
		accumulate  string
		assetfilter string
	}{
		{
			name:        "Yesterday",
			window:      "24h",
			aggregate:   "pod",
			accumulate:  "false",
			assetfilter: "node",
		},
		{
			name:        "Last 2 Days",
			window:      "48h",
			aggregate:   "pod",
			accumulate:  "false",
			assetfilter: "node",
		},
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			client := prometheus.NewClient()
			queryEnd := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
			endTime := queryEnd.Unix()

			////////////////////////////////////////////////////////////////////////////
			// Node CPU Hourly Cost
			//
			// avg(avg_over_time(node_cpu_hourly_cost{%s}[%s])) by (node, %s, instance_type, provider_id)
			////////////////////////////////////////////////////////////////////////////

			promNodeCPUCostInput := prometheus.PrometheusInput{
				Metric: "node_cpu_hourly_cost",
			}
			promNodeCPUCostInput.Function = []string{"avg_over_time", "avg"}
			promNodeCPUCostInput.QueryWindow = tc.window
			promNodeCPUCostInput.AggregateBy = []string{"node", "instance_type", "provider_id"}
			promNodeCPUCostInput.Time = &endTime

			promNodeCPUCostResponse, err := client.RunPromQLQuery(promNodeCPUCostInput)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			////////////////////////////////////////////////////////////////////////////
			// Node GPU Hourly Cost
			//
			// avg(avg_over_time(node_gpu_hourly_cost{%s}[%s])) by (node, %s, instance_type, provider_id)
			////////////////////////////////////////////////////////////////////////////

			promNodeGPUCostInput := prometheus.PrometheusInput{
				Metric: "node_gpu_hourly_cost",
			}
			promNodeGPUCostInput.Function = []string{"avg_over_time", "avg"}
			promNodeGPUCostInput.QueryWindow = tc.window
			promNodeGPUCostInput.AggregateBy = []string{"node", "instance_type", "provider_id"}
			promNodeGPUCostInput.Time = &endTime

			promNodeGPUCostResponse, err := client.RunPromQLQuery(promNodeGPUCostInput)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			////////////////////////////////////////////////////////////////////////////
			// Node RAM Hourly Cost
			//
			// avg(avg_over_time(node_ram_hourly_cost{%s}[%s])) by (node, %s, instance_type, provider_id)
			////////////////////////////////////////////////////////////////////////////

			promNodeRAMCostInput := prometheus.PrometheusInput{
				Metric: "node_ram_hourly_cost",
			}
			promNodeRAMCostInput.Function = []string{"avg_over_time", "avg"}
			promNodeRAMCostInput.QueryWindow = tc.window
			promNodeRAMCostInput.AggregateBy = []string{"node", "instance_type", "provider_id"}
			promNodeRAMCostInput.Time = &endTime

			promNodeRAMCostResponse, err := client.RunPromQLQuery(promNodeRAMCostInput)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
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

			type NodeData struct {
				CPUCostPerHr float64
				GPUCostPerHr float64
				RAMCostPerHr float64
				ProviderID   string
				InstanceType string
			}

			nodeMap := make(map[string]*NodeData)

			// Node RAM Costs
			for _, promNodeRAMCostItem := range promNodeRAMCostResponse.Data.Result {

				node := promNodeRAMCostItem.Metric.Node
				providerID := promNodeRAMCostItem.Metric.ProviderID
				instanceType := promNodeRAMCostItem.Metric.InstanceType

				nodeRAMCostPerHr := promNodeRAMCostItem.Value.Value

				if _, ok := nodeMap[node]; !ok {
					nodeMap[node] = &NodeData{
						RAMCostPerHr: nodeRAMCostPerHr,
						GPUCostPerHr: 0.0,
						CPUCostPerHr: 0.0,
						ProviderID:   providerID,
						InstanceType: instanceType,
					}
				}
			}

			// Node CPU Costs
			for _, promNodeCPUCostItem := range promNodeCPUCostResponse.Data.Result {

				node := promNodeCPUCostItem.Metric.Node
				nodeCPUCostPerHr := promNodeCPUCostItem.Value.Value

				nodeItem, ok := nodeMap[node]
				if !ok {
					t.Errorf("Node Missing from Average Hourly Node CPU Cost Prometheus Response: %s", node)
				}

				nodeItem.CPUCostPerHr = nodeCPUCostPerHr
			}

			// Node GPU Costs
			for _, promNodeGPUCostItem := range promNodeGPUCostResponse.Data.Result {

				node := promNodeGPUCostItem.Metric.Node
				nodeGPUCostPerHr := promNodeGPUCostItem.Value.Value

				nodeItem, ok := nodeMap[node]
				if !ok {
					t.Errorf("Node Missing from Average Hourly Node CPU Cost Prometheus Response: %s", node)
				}

				nodeItem.GPUCostPerHr = nodeGPUCostPerHr
			}

			// Get Node Running Time and Total Cost
			// Ex: CPUCost and CPUCoreHours
			for pod, allocationResponseItem := range apiResponse.Data[0] {

				node := allocationResponseItem.Properties.Node
				if node == "" {
					continue
				}

				nodeItem, ok := nodeMap[node]
				if !ok {
					t.Logf("Node Information missing from Allocation API: %s", node)
					continue
				}

				providerID := allocationResponseItem.Properties.ProviderID
				if providerID != nodeItem.ProviderID {
					t.Logf("Node Provider IDs do not match. Prom (%s) != API (%s)", nodeItem.ProviderID, providerID)
				}

				// CPU Costs
				cpuCost := allocationResponseItem.CPUCoreHours * nodeItem.CPUCostPerHr

				// GPU Costs
				gpuCost := allocationResponseItem.GPUHours * nodeItem.GPUCostPerHr

				// RAM Costs
				ramCost := (allocationResponseItem.RAMByteHours / 1024 / 1024 / 1024) * nodeItem.RAMCostPerHr

				t.Logf("Pod: %s", pod)

				seenCost := false

				if allocationResponseItem.RAMCost > nodeCostNegligibleCost {
					seenCost = true
					withinRange, diff_percent := utils.AreWithinPercentage(ramCost, allocationResponseItem.RAMCost, nodeCostTolerance)
					if !withinRange {
						t.Errorf("  - RAMCost[Fail]: DifferencePercent %0.2f, Prometheus: %0.4f, /allocation: %0.4f", diff_percent, ramCost, allocationResponseItem.RAMCost)
					} else {
						t.Logf("  - RAMCost[Pass]: ~ %0.2f", ramCost)
					}
				}
				if allocationResponseItem.CPUCost > nodeCostNegligibleCost {
					seenCost = true
					withinRange, diff_percent := utils.AreWithinPercentage(cpuCost, allocationResponseItem.CPUCost, nodeCostTolerance)
					if !withinRange {
						t.Errorf("  - CPUCost[Fail]: DifferencePercent %0.2f, Prometheus: %0.4f, /allocation: %0.4f", diff_percent, cpuCost, allocationResponseItem.CPUCost)
					} else {
						t.Logf("  - CPUCost[Pass]: ~ %0.2f", cpuCost)
					}
				}
				if allocationResponseItem.GPUCost > nodeCostNegligibleCost {
					seenCost = true
					withinRange, diff_percent := utils.AreWithinPercentage(gpuCost, allocationResponseItem.GPUCost, nodeCostTolerance)
					if !withinRange {
						t.Errorf("  - GPUCost[Fail]: DifferencePercent %0.2f, Prometheus: %0.4f, /allocation: %0.4f", diff_percent, gpuCost, allocationResponseItem.GPUCost)
					} else {
						t.Logf("  - GPUCost[Pass]: ~ %0.2f", gpuCost)
					}
				}
				if seenCost == false {
					t.Logf("  - No Costs Found[Skipped]")
				}
			}
			t.Logf("\n\n")
			t.Logf("Checking Node Costs from Assets API")
			apiAssetResponse, err := apiObj.GetAssets(api.AssetsRequest{
				Window: tc.window,
				Filter: tc.assetfilter,
			})

			if err != nil {
				t.Fatalf("Error while calling Allocation API %v", err)
			}
			if apiAssetResponse.Code != 200 {
				t.Errorf("API returned non-200 code")
			}

			for _, assetsResponseItem := range apiAssetResponse.Data {
				node := assetsResponseItem.Properties.Node
				cpuCost := assetsResponseItem.CPUCost
				gpuCost := assetsResponseItem.GPUCost
				ramCost := assetsResponseItem.RAMCost
				totalCost := assetsResponseItem.TotalCost

				if totalCost < nodeCostNegligibleCost {
					continue
				}

				calculatedTotalCost := cpuCost + gpuCost + ramCost

				t.Logf("Node: %s", node)
				withinRange, diff_percent := utils.AreWithinPercentage(calculatedTotalCost, totalCost, nodeCostTolerance)
				if withinRange {
					t.Logf("  - TotalNodeCost[Pass]: ~ %0.2f", totalCost)
				} else {
					t.Errorf("  - TotalNodeCost[Fail]: DifferencePercent %0.2f, AssetValue: %0.4f, CalculatedValue: %0.4f", diff_percent, totalCost, calculatedTotalCost)
				}
			}
		})
	}
}
