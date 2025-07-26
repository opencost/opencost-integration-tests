package prometheus

// Description - Compares Network Zone Internet from Prometheus and Allocation

import (
	// "fmt"
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
	"github.com/opencost/opencost-integration-tests/pkg/utils"
	"testing"
	"time"
)

const tolerance = 0.05

func TestNetworkInternetCosts(t *testing.T) {
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
				PromNetworkInternetGiB		float64
				AllocNetworkInternetGiB 	float64
			}

			networkCostsPodMap := make(map[string]*NetworkCostsAggregate)

			queryEnd := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
			endTime := queryEnd.Unix()
			// Collect Namespace results from Prometheus
			client := prometheus.NewClient()

			////////////////////////////////////////////////////////////////////////////
			// Network Internet GiB

			// sum(increase(kubecost_pod_network_egress_bytes_total{internet="true"}[24h:5m])) by (pod_name, namespace) / 1024 / 1024 / 1024
			// Apply Division by 1024^3 when you are looping over the response
			////////////////////////////////////////////////////////////////////////////

			promNetworkInternetInput := prometheus.PrometheusInput{
				Metric: "kubecost_pod_network_egress_bytes_total",
			}
			promNetworkInternetInput.Filters = map[string]string{
				"internet": "true",
			}
			promNetworkInternetInput.Function = []string{"increase", "sum"}
			promNetworkInternetInput.QueryWindow = tc.window
			promNetworkInternetInput.QueryResolution = "5m"
			promNetworkInternetInput.AggregateBy = []string{"pod_name", "namespace"}
			promNetworkInternetInput.Time = &endTime

			promNetworkInternetResponse, err := client.RunPromQLQuery(promNetworkInternetInput)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			////////////////////////////////////////////////////////////////////////////
			// Network Internet price per GiB

			// avg(avg_over_time(kubecost_network_internet_egress_cost{%s}[%s])) by (%s)
			////////////////////////////////////////////////////////////////////////////

			promNetworkInternetCostInput := prometheus.PrometheusInput{
				Metric: "kubecost_network_internet_egress_cost",
			}
			promNetworkInternetCostInput.Function = []string{"avg_over_time", "avg"}
			promNetworkInternetCostInput.QueryWindow = tc.window
			promNetworkInternetCostInput.Time = &endTime

			promNetworkInternetCostResponse, err := client.RunPromQLQuery(promNetworkInternetCostInput)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			// --------------------------------
			// Network Internet Cost for all Pods
			// --------------------------------

			networkInternetCost := promNetworkInternetCostResponse.Data.Result[0].Value.Value

			// --------------------------------
			// Assign Network Costs to Pods and Cumulate based on Namespace
			// --------------------------------
			
			// Form a key based on namespace and pod name

			for _, promNetworkInternetItem := range promNetworkInternetResponse.Data.Result {
				// namespace := promNetworkInternetItem.Metric.Namespace
				pod := promNetworkInternetItem.Metric.PodName
				gib := promNetworkInternetItem.Value.Value


				networkCostsPodMap[pod] = &NetworkCostsAggregate{
					PromNetworkInternetGiB: (gib / 1024 / 1024 / 1024) * networkInternetCost,
					AllocNetworkInternetGiB: 0.0,
				}

				// networkCostsNamespace, ok := networkCostsPodMap[namespace]
				// if !ok {
				// 	networkCostsPodMap[pod] = &NetworkCostsAggregate{
				// 		PromNetworkInternetGiB: (gib / 1024 / 1024 / 1024) * networkInternetCost,
				// 		AllocNetworkInternetGiB: 0.0,
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
						PromNetworkInternetGiB: 0.0,
						AllocNetworkInternetGiB: allocationResponseItem.NetworkInternetCost,
					}
					continue
				}
				networkCostsPod.AllocNetworkInternetGiB = allocationResponseItem.NetworkInternetCost
			}

			noNegligibleCosts := false
			negligilbleCost := 0.01  // 1 Cent of a Dollar
			for pod, networkCostValues := range networkCostsPodMap {
				if networkCostValues.AllocNetworkInternetGiB < negligilbleCost {
					continue
				} else {
					noNegligibleCosts = true
				}
				t.Logf("Pod %s", pod)
				withinRange, diff_percent := utils.AreWithinPercentage(networkCostValues.AllocNetworkInternetGiB, networkCostValues.PromNetworkInternetGiB, tolerance)
				if !withinRange {
					t.Errorf("  - NetworkInternetCost[Fail]: DifferencePercent: %0.2f, Prometheus: %0.9f, /allocation: %0.9f", diff_percent, networkCostValues.PromNetworkInternetGiB, networkCostValues.AllocNetworkInternetGiB)
				} else {
					t.Logf("  - NetworkInternetCost[Pass]: ~ %0.5f", networkCostValues.PromNetworkInternetGiB)
				}
			}
			if !noNegligibleCosts {
				t.Errorf("NetWork Internet Costs for all Pods are below 1 cent and hence cannot be considered as costs from resource usage and validated.")
			}
		})
	}
}
