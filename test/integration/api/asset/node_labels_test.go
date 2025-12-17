package assets

// Description
// Check Node Labels from Assets API Match results from Promethues

import (
	"testing"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
)

func TestLabels(t *testing.T) {
	apiObj := api.NewAPI()

	testCases := []struct {
		name      string
		window    string
		assetType string
	}{
		{
			name:      "Today",
			window:    "24h",
			assetType: "node",
		},
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			queryEnd := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
			endTime := queryEnd.Unix()

			// -------------------------------
			// Node Labels
			// avg_over_time(kube_node_labels{%s}[%s])
			// -------------------------------
			client := prometheus.NewClient()
			promLabelInfoInput := prometheus.PrometheusInput{}
			promLabelInfoInput.Metric = "kube_node_labels"
			promLabelInfoInput.Function = []string{"avg_over_time"}
			promLabelInfoInput.QueryWindow = tc.window
			promLabelInfoInput.Time = &endTime

			promLabelInfo, err := client.RunPromQLQuery(promLabelInfoInput, t)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			// Store Results in a Node Map
			type NodeData struct {
				Node        string
				PromLabels  map[string]string
				AllocLabels map[string]string
			}

			nodeMap := make(map[string]*NodeData)

			// Store Prometheus Pod Prometheus Results
			for _, promlabel := range promLabelInfo.Data.Result {
				node := promlabel.Metric.Node
				labels := promlabel.Metric.Labels
				nodeMap[node] = &NodeData{
					Node:       node,
					PromLabels: labels,
				}
			}

			// API Response
			apiResponse, err := apiObj.GetAssets(api.AssetsRequest{
				Window: tc.window,
				Filter: tc.assetType,
			})

			if err != nil {
				t.Fatalf("Error while calling Allocation API %v", err)
			}
			if apiResponse.Code != 200 {
				t.Errorf("API returned non-200 code")
			}

			// Store Allocation Pod Label Results
			for _, assetResponseItem := range apiResponse.Data {
				node := assetResponseItem.Properties.Name
				nodeLabels, ok := nodeMap[node]
				if !ok {
					t.Logf("Node Information Missing from Prometheus %s", node)
					continue
				}
				nodeLabels.AllocLabels = assetResponseItem.Labels
			}

			// Compare Results
			for node, nodeLabels := range nodeMap {
				t.Logf("Node: %s", node)

				// Prometheus Result will have fewer labels.
				// Allocation has oracle and feature related labels
				for promLabel, promLabelValue := range nodeLabels.PromLabels {
					allocLabelValue, ok := nodeLabels.AllocLabels[promLabel]
					if !ok {
						t.Errorf("  - [Fail]: Prometheus Label %s not found in Allocation", promLabel)
						continue
					}
					if allocLabelValue != promLabelValue {
						t.Errorf("  - [Fail]: Alloc %s != Prom %s", allocLabelValue, promLabelValue)
					} else {
						t.Logf("  - [Pass]: Label: %s", promLabel)
					}
				}
			}
		})
	}
}
