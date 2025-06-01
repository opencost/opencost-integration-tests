//This test checks consistency between pod-level and namespace-level cost aggregations over the last 24 hours. It fetches aggregated cost data at both levels, sums up the total costs (excluding idle entries), and verifies that the difference between the pod-level total and namespace-level total is within an acceptable margin, ensuring data aggregation integrity across hierarchy levels.
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

type aggregate struct {
    TotalCost float64 `json:"totalCost"`
}

type aggregateResponse struct {
    Code   int                       `json:"code"`
    Status string                    `json:"status"`
    Data   []map[string]CostEntry   `json:"data"`
}

func TestAggregateConsistency(t *testing.T) {
    now := time.Now().UTC()
    yesterday := now.Add(-24 * time.Hour)

    qPod := url.Values{}
    qPod.Set("window", fmt.Sprintf("%s,%s", yesterday.Format(time.RFC3339), now.Format(time.RFC3339)))
    qPod.Set("aggregate", "pod")
    qPod.Set("accumulate", "true")
    qPod.Set("step", "1d")

    podURL := fmt.Sprintf("https://demo.infra.opencost.io/model/allocation/compute?%s", qPod.Encode())

    respPod, err := http.Get(podURL)
    if err != nil {
        t.Fatalf("Failed to fetch pod-level data: %v", err)
    }
    defer respPod.Body.Close()

    var podResult CostResponse
    bodyPod, _ := io.ReadAll(respPod.Body)
    json.Unmarshal(bodyPod, &podResult)

    totalPodCost := 0.0
    for _, data := range podResult.Data {
        for name, entry := range data {
            if name == "__idle__" {
                continue
            }
            totalPodCost += entry.TotalCost
        }
    }

    qNs := url.Values{}
    qNs.Set("window", fmt.Sprintf("%s,%s", yesterday.Format(time.RFC3339), now.Format(time.RFC3339)))
    qNs.Set("aggregate", "namespace")
    qNs.Set("accumulate", "true")
    qNs.Set("step", "1d")

    nsURL := fmt.Sprintf("https://demo.infra.opencost.io/model/allocation/compute?%s", qNs.Encode())

    respNs, err := http.Get(nsURL)
    if err != nil {
        t.Fatalf("Failed to fetch namespace-level data: %v", err)
    }
    defer respNs.Body.Close()

    var nsResult CostResponse
    bodyNs, _ := io.ReadAll(respNs.Body)
    json.Unmarshal(bodyNs, &nsResult)

    totalNsCost := 0.0
    for _, data := range nsResult.Data {
        for name, entry := range data {
            if name == "__idle__" {
                continue
            }
            totalNsCost += entry.TotalCost
        }
    }

    diff := totalNsCost - totalPodCost
    if diff < 0 {
        diff = -diff
    }

    t.Logf("Total Namespace Cost: %.4f, Total Pod Cost: %.4f, Diff: %.4f", totalNsCost, totalPodCost, diff)

    if diff > 0.01 {
        t.Errorf("Mismatch between pod-level and namespace-level aggregation: ns=%.4f, pod=%.4f, diff=%.4f",
            totalNsCost, totalPodCost, diff)
    }
}

// Output

// === RUN   TestAggregateConsistency
//     f:\opencost-integration-tests\test\aggregate_consistency_test.go:89: Total Namespace Cost: 61.9037, Total Pod Cost: 61.9037, Diff: 0.0000
// --- PASS: TestAggregateConsistency (4.46s)
// PASS
// ok      github.com/opencost/opencost-integration-tests/test 
