package allocation

// Description
// Check Pod Labels from API Match results from Promethues

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
			aggregate:                 "pod",
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
			// Pod Running Time
			// avg(avg_over_time(kube_pod_container_status_running{%s}[%s])) by (pod)
			// -------------------------------
			client := prometheus.NewClient()
			promPodRunningInfoInput := prometheus.PrometheusInput{}
			promPodRunningInfoInput.Metric = "kube_pod_container_status_running"
			promPodRunningInfoInput.Function = []string{"avg_over_time", "avg"}
			promPodRunningInfoInput.QueryWindow = tc.window
			promPodRunningInfoInput.AggregateBy = []string{"pod"}
			promPodRunningInfoInput.Time = &endTime

			promPodRunningInfo, err := client.RunPromQLQuery(promPodRunningInfoInput)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			podRunningStatus := make(map[string]int)

			for _, promPodRunningInfoItem := range promPodRunningInfo.Data.Result {
				pod := promPodRunningInfoItem.Metric.Pod
				runningStatus := int(promPodRunningInfoItem.Value.Value)

				// kube_pod_labels and kube_nodespace_labels might hold labels for dead pods as well
				// filter the ones that are running because allocation filters for that
				podRunningStatus[pod] = runningStatus
			}


			// -------------------------------
			// Pod Labels
			// avg_over_time(kube_pod_labels{%s}[%s])
			// -------------------------------
			promLabelInfoInput := prometheus.PrometheusInput{}
			promLabelInfoInput.Metric = "kube_pod_labels"
			promLabelInfoInput.Function = []string{"avg_over_time"}
			promLabelInfoInput.QueryWindow = tc.window
			promLabelInfoInput.Time = &endTime

			promlabelInfo, err := client.RunPromQLQuery(promLabelInfoInput)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			// Store Results in a Pod Map
			type PodData struct {
				Pod         string
				PromLabels  map[string]string
				AllocLabels map[string]string
			}

			podMap := make(map[string]*PodData)

			// Store Prometheus Pod Prometheus Results
			for _, promlabel := range promlabelInfo.Data.Result {
				pod := promlabel.Metric.Pod
				labels := promlabel.Metric.Labels

				// Skip Dead Pods
				if podRunningStatus[pod] == 0 {
					continue
				}

				podMap[pod] = &PodData{
					Pod:        pod,
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

			// Store Allocation Pod Label Results
			for pod, allocationResponseItem := range apiResponse.Data[0] {
				podLabels, ok := podMap[pod]
				if !ok {
					t.Logf("Pod Information Missing from Prometheus %s", pod)
					continue
				}
				podLabels.AllocLabels = allocationResponseItem.Properties.Labels
			}

			// Compare Results
			for pod, podLabels := range podMap {
				t.Logf("Pod: %s", pod)

				// Prometheus Result will have fewer labels.
				// Allocation has oracle and feature related labels
				for promLabel, promLabelValue := range podLabels.PromLabels {
					allocLabelValue, ok := podLabels.AllocLabels[promLabel]
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
