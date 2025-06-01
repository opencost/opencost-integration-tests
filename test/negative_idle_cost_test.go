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

type NamespaceEntry struct {
    Name       string  `json:"name"`
    CPUCost    float64 `json:"cpuCost"`
    RAMCost    float64 `json:"ramCost"`
    GPUCost    float64 `json:"gpuCost"`
    PVCost     float64 `json:"pvCost"`
    TotalCost  float64 `json:"totalCost"`
    Efficiency float64 `json:"totalEfficiency"`
    Start      string  `json:"start"`
    End        string  `json:"end"`
}

type AllocationData map[string]NamespaceEntry

type AllocationResponse struct {
    Code   int              `json:"code"`
    Status string           `json:"status"`
    Data   []AllocationData `json:"data"`
}


func TestNegativeIdleCosts(t *testing.T) {
    baseURL := "https://demo.infra.opencost.io/model/allocation/compute"

    now := time.Now().UTC()
    yesterday := now.Add(-24 * time.Hour)

    q := url.Values{}
    q.Set("window", fmt.Sprintf("%s,%s", yesterday.Format(time.RFC3339), now.Format(time.RFC3339)))
    q.Set("aggregate", "namespace")
    q.Set("includeIdle", "true")
    q.Set("accumulate", "false")
    q.Set("step", "1d")

    fullURL := fmt.Sprintf("%s?%s", baseURL, q.Encode())

    resp, err := http.Get(fullURL)
    if err != nil {
        t.Fatalf("Failed to fetch allocation data: %v", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        t.Fatalf("Expected HTTP 200 OK, got %d", resp.StatusCode)
    }

    bodyBytes, err := io.ReadAll(resp.Body)
    if err != nil {
        t.Fatalf("Failed to read response body: %v", err)
    }

    var result AllocationResponse
    if err := json.Unmarshal(bodyBytes, &result); err != nil {
        t.Fatalf("Failed to unmarshal JSON: %v\nRaw body:\n%s", err, string(bodyBytes))
    }

    foundNegative := false

    for _, allocation := range result.Data {
        if idleEntry, exists := allocation["__idle__"]; exists {
            t.Logf("Inspecting __idle__ entry: Total=$%.2f, CPU=$%.2f, RAM=$%.2f, GPU=$%.2f, PV=$%.2f",
                idleEntry.TotalCost, idleEntry.CPUCost, idleEntry.RAMCost, idleEntry.GPUCost, idleEntry.PVCost)

            negativeChecks := []struct {
                name  string
                value float64
            }{
                {"TotalCost", idleEntry.TotalCost},
                {"CPUCost", idleEntry.CPUCost},
                {"RAMCost", idleEntry.RAMCost},
                {"GPUCost", idleEntry.GPUCost},
                {"PVCost", idleEntry.PVCost},
            }

            for _, check := range negativeChecks {
                if check.value < 0 {
                    t.Errorf("__idle__ entry has negative %s: $%.4f", check.name, check.value)
                    foundNegative = true
                }
            }
        } else {
            t.Log("No __idle__ entry found in allocation — skipping.")
        }
    }

    if !foundNegative {
        t.Log("No negative values found in __idle__ entry — test passed.")
    }
}