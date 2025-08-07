package allocation

// Description
// Check Namespace Labels from API Match results from Promethues

import (
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
	"testing"
	"time"
)

func TestLabels(t *testing.T) {
	apiObj := api.NewAPI()

	testCases := []struct {
		name                      string
		window                    string
		aggregate                 string
		accumulate                string
		includeAggregatedMetadata string
	}{
		{
			name:                      "Today",
			window:                    "24h",
			aggregate:                 "namespace",
			accumulate:                "true",
			includeAggregatedMetadata: "true",
		},
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			queryEnd := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
			endTime := queryEnd.Unix()

			// -------------------------------
			// namespace Labels
			// avg_over_time(kube_namespace_labels{%s}[%s])
			// -------------------------------
			client := prometheus.NewClient()
			promLabelInfoInput := prometheus.PrometheusInput{}
			promLabelInfoInput.Metric = "kube_namespace_labels"
			promLabelInfoInput.Function = []string{"avg_over_time"}
			promLabelInfoInput.QueryWindow = tc.window
			promLabelInfoInput.Time = &endTime

			promlabelInfo, err := client.RunPromQLQuery(promLabelInfoInput)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			// Store Results in a NamespaceMap
			type NamespaceData struct {
				Namespace   string
				PromLabels  map[string]string
				AllocLabels map[string]string
			}

			namespaceMap := make(map[string]*NamespaceData)

			// Store Prometheus namespace Prometheus Results
			for _, promlabel := range promlabelInfo.Data.Result {
				namespace := promlabel.Metric.Namespace
				labels := promlabel.Metric.Labels

				namespaceMap[namespace] = &NamespaceData{
					Namespace:  namespace,
					PromLabels: labels,
				}
			}

			// API Response
			apiResponse, err := apiObj.GetAllocation(api.AllocationRequest{
				Window:                    tc.window,
				Aggregate:                 tc.aggregate,
				Accumulate:                tc.accumulate,
				IncludeAggregatedMetadata: tc.includeAggregatedMetadata,
			})

			if err != nil {
				t.Fatalf("Error while calling Allocation API %v", err)
			}
			if apiResponse.Code != 200 {
				t.Errorf("API returned non-200 code")
			}

			// Store Allocation namespace Label Results
			for namespace, allocationResponseItem := range apiResponse.Data[0] {
				namespaceLabels, ok := namespaceMap[namespace]
				if !ok {
					t.Logf("Namespace Information Missing from Prometheus %s", namespace)
					continue
				}
				namespaceLabels.AllocLabels = allocationResponseItem.Properties.Labels
			}

			// Compare Results
			for namespace, namespaceLabels := range namespaceMap {
				t.Logf("namespace: %s", namespace)

				// Prometheus Result will have fewer labels.
				// Allocation has oracle and feature related labels
				for promLabel, promLabelValue := range namespaceLabels.PromLabels {
					allocLabelValue, ok := namespaceLabels.AllocLabels[promLabel]
					if !ok {
						t.Errorf("  - [Fail]: Prometheus Label %s not found in Allocation", promLabel)
						continue
					}
					if allocLabelValue != promLabelValue {
						t.Logf("  - [Fail]: Alloc %s != Prom %s", allocLabelValue, promLabelValue)
					} else {
						t.Logf("  - [Pass]: Label: %s", promLabel)
					}
				}
			}
		})
	}
}
