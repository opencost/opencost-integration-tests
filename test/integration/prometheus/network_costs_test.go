package prometheus

// Description - Compares Network Costs from Prometheus and Allocation

import (
	// "fmt"
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
	"github.com/opencost/opencost-integration-tests/pkg/utils"
	"slices"
	"testing"
	"time"
)

const tolerance = 0.05

func TestNetworkCosts(t *testing.T) {
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
			aggregate:  "namespace",
			accumulate: "false",
		},
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			// Any data that is in a "raw allocation only" is not valid in any
			// sort of cumulative Allocation (like one that is added).

			type NetworkCostsAggregate struct {
				PromNetworkTransferBytes  float64
				PromNetworkReceiveBytes   float64
				Pods				      []string
				AllocNetworkTransferBytes float64
				AllocNetworkReceiveBytes  float64
			}
			networkCostsNamespaceMap := make(map[string]*NetworkCostsAggregate)

			queryEnd := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
			endTime := queryEnd.Unix()
			// Collect Namespace results from Prometheus
			client := prometheus.NewClient()

			////////////////////////////////////////////////////////////////////////////
			// Network Receive Bytes

			// sum(increase(container_network_receive_bytes_total{pod!=""}[24h:5m])) by (pod, namespace)`
			////////////////////////////////////////////////////////////////////////////

			promNetworkReceiveInput := prometheus.PrometheusInput{
				Metric: "container_network_receive_bytes_total",
			}
			promNetworkReceiveInput.IgnoreFilters = map[string][]string{
				"pod": {""},
			}
			promNetworkReceiveInput.Function = []string{"increase", "sum"}
			promNetworkReceiveInput.QueryWindow = tc.window
			promNetworkReceiveInput.QueryResolution = "5m"
			promNetworkReceiveInput.AggregateBy = []string{"pod", "namespace"}
			promNetworkReceiveInput.Time = &endTime

			promNetworkReceiveResponse, err := client.RunPromQLQuery(promNetworkReceiveInput)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			////////////////////////////////////////////////////////////////////////////
			// Network Transfer Bytes

			// sum(increase(container_network_transmit_bytes_total{pod!="", %s}[%s:%dm])) by (pod_name, pod, namespace, %s)`
			////////////////////////////////////////////////////////////////////////////

			promNetworkTransferInput := prometheus.PrometheusInput{
				Metric: "container_network_transmit_bytes_total",
			}
			promNetworkTransferInput.IgnoreFilters = map[string][]string{
				"pod": {""},
			}
			promNetworkTransferInput.Function = []string{"increase", "sum"}
			promNetworkTransferInput.QueryWindow = tc.window
			promNetworkTransferInput.QueryResolution = "5m"
			promNetworkTransferInput.AggregateBy = []string{"pod", "namespace"}
			promNetworkTransferInput.Time = &endTime

			promNetworkTransferResponse, err := client.RunPromQLQuery(promNetworkTransferInput)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			// Network Receive Bytes
			for _, promNetworkReceiveResponse := range promNetworkReceiveResponse.Data.Result {
				namespace := promNetworkReceiveResponse.Metric.Namespace
				pod := promNetworkReceiveResponse.Metric.Pod
				networkReceiveBytesPod := promNetworkReceiveResponse.Value.Value
				networkCostsNamespace, ok := networkCostsNamespaceMap[namespace]
				if !ok {
					networkCostsNamespaceMap[namespace] = &NetworkCostsAggregate{
						PromNetworkReceiveBytes: networkReceiveBytesPod,
						PromNetworkTransferBytes: 0.0,
						AllocNetworkReceiveBytes: 0.0,
						AllocNetworkTransferBytes: 0.0,
						Pods: []string{pod},
					}
					continue
				}
			
				networkCostsNamespace.Pods = append(networkCostsNamespace.Pods, pod)
				networkCostsNamespace.PromNetworkReceiveBytes += networkReceiveBytesPod
			}

			// Network Transfer Bytes
			for _, promNetworkTransferResponse := range promNetworkTransferResponse.Data.Result {
				namespace := promNetworkTransferResponse.Metric.Namespace
				pod := promNetworkTransferResponse.Metric.Pod
				networkTransferBytesPod := promNetworkTransferResponse.Value.Value
				networkCostsNamespace, ok := networkCostsNamespaceMap[namespace]
				if !ok {
					networkCostsNamespaceMap[namespace] = &NetworkCostsAggregate{
						PromNetworkReceiveBytes: networkTransferBytesPod,
						PromNetworkTransferBytes: networkTransferBytesPod,
						AllocNetworkReceiveBytes: 0.0,
						AllocNetworkTransferBytes: 0.0,
						Pods: []string{pod},
					}
					continue
				}
				if !slices.Contains(networkCostsNamespace.Pods, pod) {
					networkCostsNamespace.Pods = append(networkCostsNamespace.Pods, pod)
				}
				networkCostsNamespace.PromNetworkTransferBytes += networkTransferBytesPod
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

			for namespace, allocationResponseItem := range apiResponse.Data[0] {
				networkCostsNamespace, ok := networkCostsNamespaceMap[namespace]
				if !ok {
					networkCostsNamespaceMap[namespace] = &NetworkCostsAggregate{
						PromNetworkReceiveBytes: 0.0,
						PromNetworkTransferBytes: 0.0,
						AllocNetworkReceiveBytes: allocationResponseItem.NetworkReceiveBytes,
						AllocNetworkTransferBytes: allocationResponseItem.NetworkTransferBytes,
					}
					continue
				}
				networkCostsNamespace.AllocNetworkReceiveBytes = allocationResponseItem.NetworkReceiveBytes
				networkCostsNamespace.AllocNetworkTransferBytes = allocationResponseItem.NetworkTransferBytes
			}

			for namespace, networkCostValues := range networkCostsNamespaceMap {
				t.Logf("Namespace %s", namespace)
				withinRange, diff_percent := utils.AreWithinPercentage(networkCostValues.AllocNetworkTransferBytes, networkCostValues.PromNetworkTransferBytes, tolerance)
				if !withinRange {
					t.Errorf("  - NetworkTransferBytes[Fail]: DifferencePercent: %0.2f, Prometheus: %0.2f, /allocation: %0.2f", diff_percent, networkCostValues.PromNetworkTransferBytes, networkCostValues.AllocNetworkTransferBytes)
				} else {
					t.Logf("  - NetworkTransferBytes[Pass]: ~ %0.2f", networkCostValues.PromNetworkTransferBytes)
				}
				if !withinRange {
					t.Errorf("  - NetworkReceiveBytes[Fail]: DifferencePercent: %0.2f, Prometheus: %0.2f, /allocation: %0.2f", diff_percent, networkCostValues.PromNetworkReceiveBytes, networkCostValues.AllocNetworkReceiveBytes)
				} else {
					t.Logf("  - NetworkReceiveBytes[Pass]: ~ %0.2f", networkCostValues.PromNetworkReceiveBytes)
				}
			}
		})
	}
}
