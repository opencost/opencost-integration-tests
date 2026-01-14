package prometheus

// Description - Compares Network Region Costs from Prometheus and Allocation

import (
	// "fmt"
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
	"github.com/opencost/opencost-integration-tests/pkg/utils"
	"testing"
	"time"
)

const tolerance = 0.05

func TestNetworkRegionCosts(t *testing.T) {
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
			aggregate:  "pod",
			accumulate: "false",
		},
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			// Any data that is in a "raw allocation only" is not valid in any
			// sort of cumulative Allocation (like one that is added).

			type NetworkCostsAggregate struct {
				PromNetworkRegionGiB		float64
				AllocNetworkRegionGiB 	float64
			}

			networkCostsPodMap := make(map[string]*NetworkCostsAggregate)

			queryEnd := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
			endTime := queryEnd.Unix()
			// Collect Namespace results from Prometheus
			client := prometheus.NewClient()

			////////////////////////////////////////////////////////////////////////////
			// Network Region GiB

			// sum(increase(kubecost_pod_network_egress_bytes_total{internet="false", same_zone="false", same_region="false", %s}[%s:%dm])) by (pod_name, namespace, %s) / 1024 / 1024 / 1024
			// Apply Division by 1024^3 when you are looping over the response
			////////////////////////////////////////////////////////////////////////////

			promNetworkRegionInput := prometheus.PrometheusInput{
				Metric: "kubecost_pod_network_egress_bytes_total",
			}
			promNetworkRegionInput.Filters = map[string]string{
				"internet": "false",
				"same_Region": "false",
				"same_region": "false",
			}
			promNetworkRegionInput.Function = []string{"increase", "sum"}
			promNetworkRegionInput.QueryWindow = tc.window
			promNetworkRegionInput.QueryResolution = "5m"
			promNetworkRegionInput.AggregateBy = []string{"pod_name", "namespace"}
			promNetworkRegionInput.Time = &endTime

			promNetworkRegionResponse, err := client.RunPromQLQuery(promNetworkRegionInput)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			////////////////////////////////////////////////////////////////////////////
			// Network Region price per GiB

			// avg(avg_over_time(kubecost_network_region_egress_cost{%s}[%s])) by (%s)
			////////////////////////////////////////////////////////////////////////////

			promNetworkRegionCostInput := prometheus.PrometheusInput{
				Metric: "kubecost_network_region_egress_cost",
			}
			promNetworkRegionCostInput.Function = []string{"avg_over_time", "avg"}
			promNetworkRegionCostInput.QueryWindow = tc.window
			promNetworkRegionCostInput.Time = &endTime

			promNetworkRegionCostResponse, err := client.RunPromQLQuery(promNetworkRegionCostInput)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			// --------------------------------
			// Network Region Cost for all Pods
			// --------------------------------

			networkRegionCost := promNetworkRegionCostResponse.Data.Result[0].Value.Value

			// --------------------------------
			// Assign Network Costs to Pods and Cumulate based on Namespace
			// --------------------------------
			
			// Form a key based on namespace and pod name

			for _, promNetworkRegionItem := range promNetworkRegionResponse.Data.Result {
				// namespace := promNetworkRegionItem.Metric.Namespace
				pod := promNetworkRegionItem.Metric.PodName
				gib := promNetworkRegionItem.Value.Value


				networkCostsPodMap[pod] = &NetworkCostsAggregate{
					PromNetworkRegionGiB: (gib / 1024 / 1024 / 1024) * networkRegionCost,
					AllocNetworkRegionGiB: 0.0,
				}

				// networkCostsNamespace, ok := networkCostsPodMap[namespace]
				// if !ok {
				// 	networkCostsPodMap[pod] = &NetworkCostsAggregate{
				// 		PromNetworkRegionGiB: (gib / 1024 / 1024 / 1024) * networkRegionCost,
				// 		AllocNetworkRegionGiB: 0.0,
				// 	}
				// 	continue
				// }
			}


			/////////////////////////////////////////////
			// API Client
			/////////////////////////////////////////////

			// Why doesn't allocation work on Namespace aggregate?
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

			for pod, allocationResponseItem := range apiResponse.Data[0] {
				networkCostsPod, ok := networkCostsPodMap[pod]
				if !ok {
					networkCostsPodMap[pod] = &NetworkCostsAggregate{
						PromNetworkRegionGiB: 0.0,
						AllocNetworkRegionGiB: allocationResponseItem.NetworkCrossRegionCost,
					}
					continue
				}
				networkCostsPod.AllocNetworkRegionGiB = allocationResponseItem.NetworkCrossRegionCost
			}

			validCostsSeen := false
			negligilbleCost := 0.01  // 1 Cent of a Dollar
			for pod, networkCostValues := range networkCostsPodMap {
				if networkCostValues.AllocNetworkRegionGiB < negligilbleCost {
					continue
				} else {
					validCostsSeen = true
				}
				t.Logf("Pod %s", pod)
				withinRange, diff_percent := utils.AreWithinPercentage(networkCostValues.AllocNetworkRegionGiB, networkCostValues.PromNetworkRegionGiB, tolerance)
				if !withinRange {
					t.Errorf("  - NetworkRegionCost[Fail]: DifferencePercent: %0.2f, Prometheus: %0.9f, /allocation: %0.9f", diff_percent, networkCostValues.PromNetworkRegionGiB, networkCostValues.AllocNetworkRegionGiB)
				} else {
					t.Logf("  - NetworkRegionCost[Pass]: ~ %0.5f", networkCostValues.PromNetworkRegionGiB)
				}
			}
			if !validCostsSeen {
				t.Errorf("NetWork Region Costs for all Pods are below 1 cent and hence cannot be considered as costs from resource usage and validated.")
			}
		})
	}
}
