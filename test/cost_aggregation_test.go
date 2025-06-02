//This test verifies that for each namespace, the total cost reported matches the sum of its individual resource cost components (CPU, RAM, GPU, Persistent Volume). It fetches cost data aggregated by namespace for the last 24 hours, calculates the sum of individual costs, and compares it to the reported total cost, logging any discrepancies beyond a small tolerance.

package tests

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "testing"
    "time"
)

type CostEntry struct {
    CPUCost   float64 `json:"cpuCost"`
    RAMCost   float64 `json:"ramCost"`
    GPUCost   float64 `json:"gpuCost"`
    PVCost    float64 `json:"pvCost"`
    TotalCost float64 `json:"totalCost"`
}

type CostResponse struct {
    Code   int                       `json:"code"`
    Status string                    `json:"status"`
    Data   []map[string]CostEntry   `json:"data"`
}

func TestCostAggregationBreakdown(t *testing.T) {
    now := time.Now().UTC()
    yesterday := now.Add(-24 * time.Hour)

    q := url.Values{}
    q.Set("window", fmt.Sprintf("%s,%s", yesterday.Format(time.RFC3339), now.Format(time.RFC3339)))
    q.Set("aggregate", "namespace")
    q.Set("accumulate", "true")
    q.Set("step", "1d")

    endpoint := fmt.Sprintf("https://demo.infra.opencost.io/model/allocation/compute?%s", q.Encode())

    resp, err := http.Get(endpoint)
    if err != nil {
        t.Fatalf("Failed to fetch cost data: %v", err)
    }
    defer resp.Body.Close()

    var result CostResponse
    body, _ := io.ReadAll(resp.Body)
    json.Unmarshal(body, &result)

    for _, data := range result.Data {
        for name, entry := range data {
            if name == "__idle__" {
                continue
            }

            calculated := entry.CPUCost + entry.RAMCost + entry.GPUCost + entry.PVCost
            diff := entry.TotalCost - calculated
            if diff < 0 {
                diff = -diff
            }

            t.Logf("Namespace '%s': Total=%.4f, Sum=%.4f, Diff=%.4f", name, entry.TotalCost, calculated, diff)

            if diff > 0.01 {
                t.Errorf("Cost mismatch in '%s': total=%.4f, sum=%.4f, diff=%.4f",
                    name, entry.TotalCost, calculated, diff)
            }
        }
    }
}


// Output 

// === RUN   TestCostAggregationBreakdown
//     f:\opencost-integration-tests\test\cost_aggregation_test.go:63: Namespace 'cert-manager': Total=0.0051, Sum=0.0051, Diff=0.0000
//     f:\opencost-integration-tests\test\cost_aggregation_test.go:63: Namespace 'folding-at-home': Total=55.9653, Sum=55.9653, Diff=0.0000
//     f:\opencost-integration-tests\test\cost_aggregation_test.go:63: Namespace 'kube-system': Total=1.1124, Sum=1.1124, Diff=0.0000
//     f:\opencost-integration-tests\test\cost_aggregation_test.go:63: Namespace 'load-generator': Total=1.6815, Sum=1.6815, Diff=0.0000
//     f:\opencost-integration-tests\test\cost_aggregation_test.go:63: Namespace 'opencost': Total=0.0179, Sum=0.0179, Diff=0.0000
//     f:\opencost-integration-tests\test\cost_aggregation_test.go:63: Namespace 'sealed-secrets': Total=0.0006, Sum=0.0006, Diff=0.0000
//     f:\opencost-integration-tests\test\cost_aggregation_test.go:63: Namespace 'argo': Total=0.0524, Sum=0.0525, Diff=0.0000
//     f:\opencost-integration-tests\test\cost_aggregation_test.go:63: Namespace 'gpu-operator': Total=0.3392, Sum=0.3392, Diff=0.0000
//     f:\opencost-integration-tests\test\cost_aggregation_test.go:63: Namespace 'ingress-nginx': Total=0.3390, Sum=0.0677, Diff=0.2712
//     f:\opencost-integration-tests\test\cost_aggregation_test.go:66: Cost mismatch in 'ingress-nginx': total=0.3390, sum=0.0677, diff=0.2712
//     f:\opencost-integration-tests\test\cost_aggregation_test.go:63: Namespace 'prometheus-system': Total=2.3897, Sum=2.3897, Diff=0.0000
// --- FAIL: TestCostAggregationBreakdown (1.81s)
// FAIL
// FAIL    github.com/opencost/opencost-integration-tests/test     3.784s