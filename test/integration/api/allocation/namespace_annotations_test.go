package allocation

// Description
// Check Namespace Annotations from API Match results from Promethues

import (
	"testing"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
)

func TestNamespaceAnnotations(t *testing.T) {
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
		{
			name:                      "Last Two Days",
			window:                    "48h",
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
			// namespace Annotations
			// avg_over_time(kube_namespace_annotations{%s}[%s])
			// -------------------------------
			client := prometheus.NewClient()
			promAnnotationInfoInput := prometheus.PrometheusInput{}
			promAnnotationInfoInput.Metric = "kube_namespace_annotations"
			promAnnotationInfoInput.Function = []string{"avg_over_time"}
			promAnnotationInfoInput.QueryWindow = tc.window
			promAnnotationInfoInput.Time = &endTime

			promannotationInfo, err := client.RunPromQLQuery(promAnnotationInfoInput, t)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			// Store Results in a NamespaceMap
			type NamespaceData struct {
				Namespace        string
				PromAnnotations  map[string]string
				AllocAnnotations map[string]string
			}

			namespaceMap := make(map[string]*NamespaceData)

			// Store Prometheus namespace Prometheus Results
			for _, promannotation := range promannotationInfo.Data.Result {
				namespace := promannotation.Metric.Namespace
				annotations := promannotation.Metric.Annotations

				namespaceMap[namespace] = &NamespaceData{
					Namespace:       namespace,
					PromAnnotations: annotations,
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

			// Store Allocation namespace Annotation Results
			for namespace, allocationResponseItem := range apiResponse.Data[0] {
				namespaceAnnotations, ok := namespaceMap[namespace]
				if !ok && allocationResponseItem.Properties.NamespaceAnnotations == nil {
					t.Logf("[Skipped] - No Annotations for Namespace: %s", namespace)
					continue
				}
				namespaceAnnotations.AllocAnnotations = allocationResponseItem.Properties.NamespaceAnnotations
			}

			seenAnnotations := false

			// Compare Results
			for namespace, namespaceAnnotations := range namespaceMap {
				t.Logf("namespace: %s", namespace)

				// Prometheus Result will have fewer annotations.
				// Allocation has oracle and feature related annotations
				for promAnnotation, promAnnotationValue := range namespaceAnnotations.PromAnnotations {
					allocAnnotationValue, ok := namespaceAnnotations.AllocAnnotations[promAnnotation]
					if !ok {
						t.Errorf("  - [Fail]: Prometheus Annotation %s not found in Allocation", promAnnotation)
						continue
					}

					seenAnnotations = true

					if allocAnnotationValue != promAnnotationValue {
						t.Errorf("  - [Fail]: Alloc %s != Prom %s", allocAnnotationValue, promAnnotationValue)
					} else {
						t.Logf("  - [Pass]: Annotation: %s", promAnnotation)
					}
				}
			}
			if !seenAnnotations {
				t.Fatalf("No Namespace Annotations")
			}
		})
	}
}
