package assets

// ### Description
// Check Spot Nodes from Assets API Match results from Promethues

import (
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
	"time"
	"testing"
)

func TestSpotNodes(t *testing.T) {
	apiObj := api.NewAPI()

	testCases := []struct {
		name        				string
		window      				string
		assetType					string
	}{
		{
			name:        "Today",
			window:      "24h",
			assetType:   "node",
		},
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
	
			queryEnd := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
			endTime := queryEnd.Unix()

			// -------------------------------
			// Node Labels
			// avg_over_time(kubecost_node_is_spot{%s}[%s])
			// -------------------------------
			client := prometheus.NewClient()
			promSpotNodeInfoInput := prometheus.PrometheusInput{}
			promSpotNodeInfoInput.Metric = "kubecost_node_is_spot"
			promSpotNodeInfoInput.Function = []string{"avg_over_time"}
			promSpotNodeInfoInput.QueryWindow = tc.window
			promSpotNodeInfoInput.Time = &endTime

			promSpotNodeInfo, err := client.RunPromQLQuery(promSpotNodeInfoInput)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			// Store Results in a Node Map
			type SpotNodeData struct {
				Node					string
				IsSpotNodeAsset			bool
				IsSpotNodeProm			bool
				IsNodeAvailableInBoth	bool
			}

			spotNodeMap := make(map[string]*SpotNodeData)

			// Store Prometheus Node Results
			for _, promSpotNode := range promSpotNodeInfo.Data.Result {
				node := promSpotNode.Metric.Node
				isSpotNode := promSpotNode.Value.Value

				var isSpot bool
				
				if isSpotNode > 0.0 {
					isSpot = true
				} else {
					isSpot = false
				}

				spotNodeMap[node] = &SpotNodeData{
					Node: node,
					IsSpotNodeProm: isSpot,
					IsNodeAvailableInBoth: false, // We set it to True if we see the node in assets API
				}
			}

			// API Response
			apiResponse, err := apiObj.GetAssets(api.AssetsRequest{
				Window:     tc.window,
				Filter:		tc.assetType,
			})

			if err != nil {
				t.Fatalf("Error while calling Allocation API %v", err)
			}
			if apiResponse.Code != 200 {
				t.Errorf("API returned non-200 code")
			}

			// Store Allocation Pod Label Results
			for _, assetResponseItem := range apiResponse.Data {
				
				node, ok := assetResponseItem.Properties.Name
				if !ok {
					continue
				}
				spotNode, ok := spotNodeMap[node]
				if !ok {
					t.Logf("Node Information Missing in Prometheus %s", node)
					continue
				}

				var isSpot bool
				
				if assetResponseItem.Preemptible == 1 {
					isSpot = true
				} else {
					isSpot = false
				}
				spotNode.IsSpotNodeAsset = isSpot
				spotNode.IsNodeAvailableInBoth = true
			}

			if len(spotNodeMap) == 0 {
				t.Fatalf("No nodes found in Assets and Prometheus")
			}

			// Compare Results
			for node, spotNodeValues := range spotNodeMap{
				if spotNodeValues.IsNodeAvailableInBoth == false {
					t.Errorf("Node %s Information missing in Assets", node)
				}
				t.Logf("Node: %s", node)
				if spotNodeValues.IsSpotNodeAsset == spotNodeValues.IsSpotNodeProm {
					t.Logf("  - [Pass]: Is it SpotNode?: %t", spotNodeValues.IsSpotNodeProm)
				} else {
					t.Errorf("  - [Fail]: Alloc %t != Prom %t", spotNodeValues.IsSpotNodeAsset, spotNodeValues.IsSpotNodeProm)
				}
			}
		})
	}
}
