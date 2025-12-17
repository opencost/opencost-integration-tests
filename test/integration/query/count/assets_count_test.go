package count

// Description - Check if prometheus costs match allocation

import (
	// "fmt"
	"testing"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
	"github.com/opencost/opencost-integration-tests/pkg/utils"
)

const tolerance = 0.07

func TestQueryAssets(t *testing.T) {
	// API Client
	apiObj := api.NewAPI()
	// Prometheus Client
	client := prometheus.NewClient()

	testCases := []struct {
		name   string
		window string
	}{
		{
			name:   "Yesterday",
			window: "24h",
		},
		{
			name:   "Last 2 Days",
			window: "48h",
		},
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			// Node Response
			apiNodeResponse, err := apiObj.GetAssets(api.AssetsRequest{
				Window: tc.window,
				Filter: "node",
			})

			if err != nil {
				t.Fatalf("Error while calling Allocation API %v", err)
			}
			if apiNodeResponse.Code != 200 {
				t.Errorf("API returned non-200 code")
			}

			// LoadBalancer Response
			apiLBResponse, err := apiObj.GetAssets(api.AssetsRequest{
				Window: tc.window,
				Filter: "loadbalancer",
			})

			if err != nil {
				t.Fatalf("Error while calling Allocation API %v", err)
			}
			if apiLBResponse.Code != 200 {
				t.Errorf("API returned non-200 code")
			}

			// Persistent Volume
			apiPVResponse, err := apiObj.GetAssets(api.AssetsRequest{
				Window: tc.window,
				Filter: "disk",
			})

			if err != nil {
				t.Fatalf("Error while calling Allocation API %v", err)
			}
			if apiPVResponse.Code != 200 {
				t.Errorf("API returned non-200 code")
			}

			queryEnd := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
			endTime := queryEnd.Unix()

			// Node Prom Output
			promNodeInput := prometheus.PrometheusInput{
				Metric:      "node_total_hourly_cost",
				Function:    []string{"avg_over_time"},
				QueryWindow: tc.window,
				Time:        &endTime,
			}

			// LoadBalancer Prom Output
			promNodeResponse, err := client.RunPromQLQuery(promNodeInput, t)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			promLBInput := prometheus.PrometheusInput{
				Metric:      "kubecost_load_balancer_cost",
				Function:    []string{"avg_over_time"},
				QueryWindow: tc.window,
				Time:        &endTime,
			}

			promLBResponse, err := client.RunPromQLQuery(promLBInput, t)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			promPVInput := prometheus.PrometheusInput{
				Metric:      "pv_hourly_cost",
				Function:    []string{"avg_over_time"},
				QueryWindow: tc.window,
				Time:        &endTime,
			}

			promPVResponse, err := client.RunPromQLQuery(promPVInput, t)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			// Store Asset Results
			type NodeAggregation struct {
				AssetNodeHrCost float64
				PromNodeHrCost  float64
			}
			type PVAggregation struct {
				AssetPVHrCost float64
				PromPVHrCost  float64
			}
			type LBAggregation struct {
				AssetLBHrCost float64
				PromLBHrCost  float64
			}

			// Node
			var assetNodeCount = make(map[string]*NodeAggregation)
			for _, apiNodeResponeItem := range apiNodeResponse.Data {
				name := apiNodeResponeItem.Properties.Name
				totalCost := apiNodeResponeItem.TotalCost
				runTime := apiNodeResponeItem.Minutes

				nodeHrCost := totalCost / runTime

				assetNodeCount[name] = &NodeAggregation{
					AssetNodeHrCost: nodeHrCost,
				}
			}

			for _, promNodeResponseItem := range promNodeResponse.Data.Result {
				node := promNodeResponseItem.Metric.Node
				totalCost := promNodeResponseItem.Value.Value

				assetNodeCountItem, ok := assetNodeCount[node]

				if !ok {
					t.Errorf("Node Counts do not match. Missing %s", node)
				} else {
					assetNodeCountItem.PromNodeHrCost = totalCost
				}

				t.Logf("Node: %s", node)
				withinRange, diff_percent := utils.AreWithinPercentage(assetNodeCountItem.PromNodeHrCost, assetNodeCountItem.AssetNodeHrCost, tolerance)
				if !withinRange {
					t.Logf("  - NodeCosts[Pass]: ~ %v", assetNodeCountItem.AssetNodeHrCost)
				} else {
					t.Errorf("  - NodeCosts[Fail]: DifferencePercent %0.2f, Prometheus: %0.2f, /assets: %0.2f", diff_percent, assetNodeCountItem.PromNodeHrCost, assetNodeCountItem.AssetNodeHrCost)
				}

			}

			// LB
			var assetLBCount = make(map[string]*LBAggregation)
			for _, apiLBResponeItem := range apiLBResponse.Data {
				name := apiLBResponeItem.Properties.Name
				totalCost := apiLBResponeItem.TotalCost
				runTime := apiLBResponeItem.Minutes

				lbHrCost := totalCost / runTime

				assetLBCount[name] = &LBAggregation{
					AssetLBHrCost: lbHrCost,
				}
			}

			for _, promLBResponseItem := range promLBResponse.Data.Result {
				namespace := promLBResponseItem.Metric.Namespace
				serviceName := promLBResponseItem.Metric.ServiceName

				name := namespace + "/" + serviceName
				cost := promLBResponseItem.Value.Value

				assetLBCountItem, ok := assetLBCount[name]
				if !ok {
					t.Errorf("Load Balancer Counts do not match. Missing %s", name)
				} else {
					assetLBCountItem.PromLBHrCost = cost
				}

				t.Logf("Load Balancer: %s", name)
				withinRange, diff_percent := utils.AreWithinPercentage(assetLBCountItem.PromLBHrCost, assetLBCountItem.AssetLBHrCost, tolerance)
				if !withinRange {
					t.Logf("  - LBCosts[Pass]: ~ %v", assetLBCountItem.AssetLBHrCost)
				} else {
					t.Errorf("  - LBCosts[Fail]: DifferencePercent %0.2f, Prometheus: %0.2f, /assets: %0.2f", diff_percent, assetLBCountItem.PromLBHrCost, assetLBCountItem.AssetLBHrCost)
				}
			}

			// PV
			var assetPVCount = make(map[string]*PVAggregation)
			for _, apiPVResponseItem := range apiPVResponse.Data {
				name := apiPVResponseItem.Properties.Name
				totalCost := apiPVResponseItem.TotalCost
				runTime := apiPVResponseItem.Minutes

				local := apiPVResponseItem.Local

				// This is not a Persistent Volume
				if local > 0 {
					continue
				}

				pvHrCost := totalCost / runTime

				assetPVCount[name] = &PVAggregation{
					AssetPVHrCost: pvHrCost,
				}
			}

			for _, promPVResponseItem := range promPVResponse.Data.Result {
				volumeName := promPVResponseItem.Metric.VolumeName
				cost := promPVResponseItem.Value.Value

				assetPVCountItem, ok := assetPVCount[volumeName]

				if !ok {
					t.Errorf("Persistent Volume Counts do not match. Missing %s", volumeName)
				} else {
					assetPVCountItem.PromPVHrCost = cost
				}

				t.Logf("Persistent Volume: %s", volumeName)
				withinRange, diff_percent := utils.AreWithinPercentage(assetPVCountItem.PromPVHrCost, assetPVCountItem.AssetPVHrCost, tolerance)
				if !withinRange {
					t.Logf("  - PVCosts[Pass]: ~ %v", assetPVCountItem.AssetPVHrCost)
				} else {
					t.Errorf("  - PVCosts[Fail]: DifferencePercent %0.2f, Prometheus: %0.2f, /assets: %0.2f", diff_percent, assetPVCountItem.PromPVHrCost, assetPVCountItem.AssetPVHrCost)
				}
			}
		})
	}
}
