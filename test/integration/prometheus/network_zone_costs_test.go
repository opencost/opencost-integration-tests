package prometheus

// Description - Compares Network Zone Costs from Prometheus and Allocation

import (
	// "fmt"
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
	"github.com/opencost/opencost-integration-tests/pkg/utils"
	"testing"
	"time"
)

const tolerance = 0.05

func TestNetworkZoneCosts(t *testing.T) {
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
				PromNetworkZoneGiB		float64
				AllocNetworkZoneGiB 	float64
			}

			networkCostsPodMap := make(map[string]*NetworkCostsAggregate)

			queryEnd := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
			endTime := queryEnd.Unix()
			// Collect Namespace results from Prometheus
			client := prometheus.NewClient()

			////////////////////////////////////////////////////////////////////////////
			// Network Zone GiB

			// sum(increase(kubecost_pod_network_egress_bytes_total{internet="false", same_zone="false", same_region="true", %s}[%s:%dm])) by (pod_name, namespace, %s) / 1024 / 1024 / 1024
			// Apply Division by 1024^3 when you are looping over the response
			////////////////////////////////////////////////////////////////////////////

			promNetworkZoneInput := prometheus.PrometheusInput{
				Metric: "kubecost_pod_network_egress_bytes_total",
			}
			promNetworkZoneInput.Filters = map[string]string{
				"internet": "false",
				"same_zone": "false",
				"same_region": "true",
			}
			promNetworkZoneInput.Function = []string{"increase", "sum"}
			promNetworkZoneInput.QueryWindow = tc.window
			promNetworkZoneInput.QueryResolution = "5m"
			promNetworkZoneInput.AggregateBy = []string{"pod_name", "namespace"}
			promNetworkZoneInput.Time = &endTime

			promNetworkZoneResponse, err := client.RunPromQLQuery(promNetworkZoneInput)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			////////////////////////////////////////////////////////////////////////////
			// Network Zone price per GiB

			// avg(avg_over_time(kubecost_network_zone_egress_cost{%s}[%s])) by (%s)
			////////////////////////////////////////////////////////////////////////////

			promNetworkZoneCostInput := prometheus.PrometheusInput{
				Metric: "kubecost_network_zone_egress_cost",
			}
			promNetworkZoneCostInput.Function = []string{"avg_over_time", "avg"}
			promNetworkZoneCostInput.QueryWindow = tc.window
			promNetworkZoneCostInput.Time = &endTime

			promNetworkZoneCostResponse, err := client.RunPromQLQuery(promNetworkZoneCostInput)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			// --------------------------------
			// Network Zone Cost for all Pods
			// --------------------------------

			networkZoneCost := promNetworkZoneCostResponse.Data.Result[0].Value.Value

			// --------------------------------
			// Assign Network Costs to Pods and Cumulate based on Namespace
			// --------------------------------
			
			// Form a key based on namespace and pod name

			for _, promNetworkZoneItem := range promNetworkZoneResponse.Data.Result {
				// namespace := promNetworkZoneItem.Metric.Namespace
				pod := promNetworkZoneItem.Metric.PodName
				gib := promNetworkZoneItem.Value.Value


				networkCostsPodMap[pod] = &NetworkCostsAggregate{
					PromNetworkZoneGiB: (gib / 1024 / 1024 / 1024) * networkZoneCost,
					AllocNetworkZoneGiB: 0.0,
				}

				// networkCostsNamespace, ok := networkCostsPodMap[namespace]
				// if !ok {
				// 	networkCostsPodMap[pod] = &NetworkCostsAggregate{
				// 		PromNetworkZoneGiB: (gib / 1024 / 1024 / 1024) * networkZoneCost,
				// 		AllocNetworkZoneGiB: 0.0,
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
						PromNetworkZoneGiB: 0.0,
						AllocNetworkZoneGiB: allocationResponseItem.NetworkCrossZoneCost,
					}
					continue
				}
				networkCostsPod.AllocNetworkZoneGiB = allocationResponseItem.NetworkCrossZoneCost
			}

			validCostsSeen := false
			negligilbleCost := 0.01  // 1 Cent of a Dollar
			for pod, networkCostValues := range networkCostsPodMap {
				if networkCostValues.AllocNetworkZoneGiB < negligilbleCost {
					continue
				} else {
					validCostsSeen = true
				}
				t.Logf("Pod %s", pod)
				withinRange, diff_percent := utils.AreWithinPercentage(networkCostValues.AllocNetworkZoneGiB, networkCostValues.PromNetworkZoneGiB, tolerance)
				if !withinRange {
					t.Errorf("  - NetworkZoneCost[Fail]: DifferencePercent: %0.2f, Prometheus: %0.9f, /allocation: %0.9f", diff_percent, networkCostValues.PromNetworkZoneGiB, networkCostValues.AllocNetworkZoneGiB)
				} else {
					t.Logf("  - NetworkZoneCost[Pass]: ~ %0.5f", networkCostValues.PromNetworkZoneGiB)
				}
			}
			if !validCostsSeen {
				t.Errorf("NetWork Zone Costs for all Pods are below 1 cent and hence cannot be considered as costs from resource usage and validated.")
			}
		})
	}
}
