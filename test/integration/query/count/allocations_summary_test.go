package count

// Description - Checks if the allocation summary of pods for each namespace is the same for a prometheus request
// and allocation/summary API request

import (
        // "fmt"
        // "time"
        "github.com/opencost/opencost-integration-tests/pkg/api"
        "github.com/opencost/opencost-integration-tests/pkg/prometheus"
        "testing"
        "sort"

        "github.com/pmezard/go-difflib/difflib"
)

func TestQueryAllocationSummary(t *testing.T) {
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

                        // API Client
                        apiResponse, err := apiObj.GetAllocationSummary(api.AllocationRequest{
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

                        // Prometheus Client
                        client := prometheus.NewClient()
                        metric := "kube_pod_container_status_running"
                        // kube-state-metrics is another job type
                        filters := map[string]string{
                                "job": "opencost",
                        }
                        promResponse, err := client.RunPromQLQuery(metric, filters, tc.window)

                        if err != nil {
                                t.Fatalf("Error while calling Prometheus API %v", err)
                        }

                        apiAllocationsSummaryCount := len(apiResponse.Data.Sets[0].Allocations)
                        promAllocationsSummaryCount := len(promResponse.Data.Result)

                        var apiAllocationPodNames []string
                        for podName, _ := range apiResponse.Data.Sets[0].Allocations {
                                apiAllocationPodNames = append(apiAllocationPodNames, podName)
                        }

                        var promPodNames []string
                        for _, promItem := range promResponse.Data.Result {
                                promPodNames = append(promPodNames, promItem.Metric.Pod)
                        }
                        // sort the strings
                        sort.Strings(promPodNames)
                        sort.Strings(apiAllocationPodNames)

                        // How to Tackle: Count might be equal the pods might not be the same
                        if apiAllocationsSummaryCount != promAllocationsSummaryCount {
                                diff := difflib.UnifiedDiff{
                                        A:        promPodNames,
                                        B:        apiAllocationPodNames,
                                        FromFile: "Prometheus",
                                        ToFile:   "AllocationSummary",
                                        Context:  3,
                                }
                                podNamesDiff, _ := difflib.GetUnifiedDiffString(diff)
                                t.Errorf("Number of Allocations Responses from Prometheus %d and /allocation/summary %d did not match\n Unified Diff:\n %s", promAllocationsSummaryCount, apiAllocationsSummaryCount, podNamesDiff)
                        } else {
                                t.Logf("Number of Allocations from Promtheus and /allocation/summary match")
                        }

                })
        }
}