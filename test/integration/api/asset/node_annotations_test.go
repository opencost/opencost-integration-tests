package assets

// Description
// Check Node Annotations from Assets API Match results from Promethues

import (
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
	"time"
	"testing"
)

func TestAnnotations(t *testing.T) {
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
			// Node Annotations
			// avg_over_time(kube_node_annotations{%s}[%s])
			// -------------------------------
			client := prometheus.NewClient()
			promAnnotationInfoInput := prometheus.PrometheusInput{}
			promAnnotationInfoInput.Metric = "kube_node_annotations"
			promAnnotationInfoInput.Function = []string{"avg_over_time"}
			promAnnotationInfoInput.QueryWindow = tc.window
			promAnnotationInfoInput.Time = &endTime

			promAnnotationInfo, err := client.RunPromQLQuery(promAnnotationInfoInput)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			// Store Results in a Node Map
			type NodeData struct {
				Node		string
				PromAnnotations	map[string]string
				AllocAnnotations	map[string]string
			}

			nodeMap := make(map[string]*NodeData)

			// Store Prometheus Pod Prometheus Results
			for _, promAnnotation := range promAnnotationInfo.Data.Result {
				node := promAnnotation.Metric.Node
				annotations := promAnnotation.Metric.Annotations
				nodeMap[node] = &NodeData{
					Node: node,
					PromAnnotations: annotations,
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

			// Store Allocation Pod Annotation Results
			for _, assetResponseItem := range apiResponse.Data {
				node := assetResponseItem.Properties.Name
				nodeAnnotations, ok := nodeMap[node]
				if !ok {
					t.Logf("Node Information Missing from Prometheus %s", node)
					continue
				}
				nodeAnnotations.AllocAnnotations = assetResponseItem.Annotations
			}

			// Compare Results
			for node, nodeAnnotations := range nodeMap{
				t.Logf("Node: %s", node)

				// Prometheus Result will have fewer annotations.
				// Allocation has oracle and feature related annotations
				for promAnnotation, promAnnotationValue := range nodeAnnotations.PromAnnotations {
					allocAnnotationValue, ok := nodeAnnotations.AllocAnnotations[promAnnotation]
					if !ok {
						t.Errorf("  - [Fail]: Prometheus Annotation %s not found in Allocation", promAnnotation)
						continue
					}
					if allocAnnotationValue != promAnnotationValue {
						t.Logf("  - [Fail]: Alloc %s != Prom %s", allocAnnotationValue, promAnnotationValue)
					} else {
						t.Logf("  - [Pass]: Annotation: %s", promAnnotation)
					}
				}
			}
		})
	}
}
