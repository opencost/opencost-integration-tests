package count

// Description - Checks if the allocation summary of pods for each namespace is the same for a prometheus request
// and allocation/summary API request

import (
        // "fmt"
        // "time"
        "github.com/opencost/opencost-integration-tests/pkg/api"
        "github.com/opencost/opencost-integration-tests/pkg/prometheus"
        "testing"
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

                        if apiAllocationsSummaryCount != promAllocationsSummaryCount {
                                t.Errorf("Number of Allocations Responses from Prometheus %d and /allocation/summary %d did not match.", promAllocationsSummaryCount, apiAllocationsSummaryCount)
                        } else {
                                t.Logf("Number of Allocations from Promtheus and /allocation/summary match")
                        }

                })
        }
}