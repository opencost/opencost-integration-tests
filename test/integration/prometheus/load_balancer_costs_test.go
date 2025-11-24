package prometheus

// Description - Load Balancer Cost Analysis
// To - Do
// Need to confirm if cost per minute matches pricing API (where do I get the API?)

import (
	// "fmt"
	"testing"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
	"github.com/opencost/opencost-integration-tests/pkg/utils"
)

const loadBalancerCostTolerance = 0.05

func TestLoadBalancerCost(t *testing.T) {
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
		{
			name:       "Last 2 Days",
			window:     "48h",
			aggregate:  "namespace",
			accumulate: "false",
		},
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			// Use this information to find start and end time of pod
			queryEnd := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
			// Get Time Duration
			timeMumericVal, _ := utils.ExtractNumericPrefix(tc.window)
			// Assume the minumum unit is an hour
			negativeDuration := time.Duration(timeMumericVal*float64(time.Hour)) * -1
			queryStart := queryEnd.Add(negativeDuration)
			window24h := api.Window{
				Start: queryStart,
				End:   queryEnd,
			}
			resolution := 5 * time.Minute
			endTime := queryEnd.Unix()

			////////////////////////////////////////////////////////////////////////////
			// Load Balancer Price Per Hour

			// avg(kubecost_load_balancer_cost{%s}[%s])
			// by
			// (namespace, service_name, ingress_ip, %s)[%s:%d]
			////////////////////////////////////////////////////////////////////////////
			client := prometheus.NewClient()
			promLBInfoInput := prometheus.PrometheusInput{}
			promLBInfoInput.Metric = "kubecost_load_balancer_cost"
			promLBInfoInput.AggregateBy = []string{"namespace", "service_name", "ingress_ip"}
			promLBInfoInput.Function = []string{"avg"}
			promLBInfoInput.AggregateWindow = tc.window
			promLBInfoInput.AggregateResolution = "5m"
			promLBInfoInput.Time = &endTime

			promLBInfo, err := client.RunPromQLQuery(promLBInfoInput)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			type LBServiceData struct {
				ServicePricePerHr float64
				RunTime           float64
			}
			type LoadBalancerData struct {
				AllocLBCost float64
				PromLBCost  float64
				Services    map[string]*LBServiceData
			}

			// Namespace
			LBNamespaceMap := make(map[string]*LoadBalancerData)
			// Service
			LBServiceMap := make(map[string]*LoadBalancerData)

			// Get Run Times for each service under a namespace
			for _, promLBInfoResponseItem := range promLBInfo.Data.Result {
				namespace := promLBInfoResponseItem.Metric.Namespace
				LBNamespace, ok := LBNamespaceMap[namespace]
				s, e := prometheus.CalculateStartAndEnd(promLBInfoResponseItem.Values, resolution, window24h)
				serviceRunTime := e.Sub(s).Minutes()
				serviceName := promLBInfoResponseItem.Metric.ServiceName
				if !ok {
					LBNamespaceMap[namespace] = &LoadBalancerData{
						AllocLBCost: 0.0,
						PromLBCost:  0.0,
						Services:    make(map[string]*LBServiceData),
					}
					LBNamespaceMap[namespace].Services[namespace+"/"+serviceName] = &LBServiceData{
						RunTime:           serviceRunTime,
						ServicePricePerHr: 0.0,
					}
					continue
				}
				LBNamespace.Services[namespace+"/"+serviceName] = &LBServiceData{
					RunTime:           serviceRunTime,
					ServicePricePerHr: 0.0,
				}
			}

			////////////////////////////////////////////////////////////////////////////
			// Load Balancer Price Per Hour

			// avg(avg_over_time(kubecost_load_balancer_cost{%s}[%s]))
			// by
			// (namespace, service_name, ingress_ip, %s)
			////////////////////////////////////////////////////////////////////////////
			promLBInput := prometheus.PrometheusInput{
				Metric: "kubecost_load_balancer_cost",
			}
			promLBInput.Function = []string{"avg_over_time", "avg"}
			promLBInput.QueryWindow = tc.window
			promLBInput.AggregateBy = []string{"namespace", "service_name", "ingress_ip"}
			promLBInput.Time = &endTime

			promLBResponse, err := client.RunPromQLQuery(promLBInput)
			// Do we need container_name and pod_name
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			// Append ServicePerHr Cost Details to the Map
			for _, promLBResponseItem := range promLBResponse.Data.Result {

				// Get containerRunTime by getting the pod's (parent object) runtime.
				servicePricePerHr := promLBResponseItem.Value.Value
				namespace := promLBResponseItem.Metric.Namespace
				serviceName := promLBResponseItem.Metric.ServiceName

				// Service Name must already exist under namespace, otherwise it would be impossible to calculate Cost without runtime
				LBNamespaceItem, ok := LBNamespaceMap[namespace]
				if !ok {
					t.Errorf("Namespace missing from Prom Response %s", namespace)
				}
				LBServiceItem, ok := LBNamespaceItem.Services[namespace+"/"+serviceName]
				if !ok {
					t.Errorf("Service missing from Prom Response %s", serviceName)
				}
				LBServiceItem.ServicePricePerHr = servicePricePerHr
			}

			// Calculate LB cost for namespace using weighted average method considering runtime
			// This is the aggregate result from Prometheus
			for _, promLBNamespaceItem := range LBNamespaceMap {
				namespaceLBCost := 0.0
				for serviceName, serviceLBItem := range promLBNamespaceItem.Services {
					serviceCost := serviceLBItem.ServicePricePerHr * serviceLBItem.RunTime
					namespaceLBCost += serviceCost

					LBServiceMap[serviceName] = &LoadBalancerData{
						PromLBCost:  serviceCost / 60,
						AllocLBCost: 0.0,
					}
				}
				// Might be necessary in the future to account for window discreprancies or LoadBalancer provision time differences
				// promLBNamespaceItem.PromLBCost = namespaceLBCost * ScalingFactor
				promLBNamespaceItem.PromLBCost = namespaceLBCost / 60
			}

			/////////////////////////////////////////////
			// API Client
			/////////////////////////////////////////////

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
				// For Namespace
				allocLBItem, ok := LBNamespaceMap[namespace]
				if !ok {
					LBNamespaceMap[namespace] = &LoadBalancerData{
						PromLBCost:  0.0,
						AllocLBCost: allocationResponseItem.LoadBalancerCost,
						Services:    make(map[string]*LBServiceData),
					}
					continue
				}
				allocLBItem.AllocLBCost = allocationResponseItem.LoadBalancerCost

				// For Service
				for _, loadBalancerItem := range allocationResponseItem.LoadBalancerAllocations {
					service := loadBalancerItem.Service
					allocLBItem, ok := LBServiceMap[service]
					if !ok {
						LBServiceMap[service] = &LoadBalancerData{
							PromLBCost:  0.0,
							AllocLBCost: loadBalancerItem.Cost,
						}
						continue
					}
					allocLBItem.AllocLBCost = loadBalancerItem.Cost
				}
			}

			// By Namespace
			t.Logf("Load Balancer Costs by Namespace")
			for namespace, LBNamespaceItem := range LBNamespaceMap {
				t.Logf("Namespace %s", namespace)
				withinRange, diff_percent := utils.AreWithinPercentage(LBNamespaceItem.PromLBCost, LBNamespaceItem.AllocLBCost, loadBalancerCostTolerance)
				if !withinRange {
					t.Errorf("  - LoadBalancerCost[Fail]: DifferencePercent %0.2f, Prometheus: %0.2f, /allocation: %0.2f", diff_percent, LBNamespaceItem.PromLBCost, LBNamespaceItem.AllocLBCost)
				} else {
					t.Logf("  - LoadBalancerCost[Pass]: ~ %0.2f", LBNamespaceItem.PromLBCost)
				}
			}

			// By Services
			t.Logf("Load Balancer Costs by Services")
			for service, LBServiceItem := range LBServiceMap {
				t.Logf("Service %s", service)
				withinRange, diff_percent := utils.AreWithinPercentage(LBServiceItem.PromLBCost, LBServiceItem.AllocLBCost, loadBalancerCostTolerance)
				if !withinRange {
					t.Errorf("  - LoadBalancerCost[Fail]: DifferencePercent %0.2f, Prometheus: %0.2f, /allocation: %0.2f", diff_percent, LBServiceItem.PromLBCost, LBServiceItem.AllocLBCost)
				} else {
					t.Logf("  - LoadBalancerCost[Pass]: ~ %0.2f", LBServiceItem.PromLBCost)
				}
			}
		})
	}
}
