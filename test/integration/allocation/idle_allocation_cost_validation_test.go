package allocation

import (
	"encoding/json"
	"fmt"
	"io"

	"net/http"
	"net/url"
	"testing"
)

type AllocationResponse struct {
	Code   int                    `json:"code"`
	Status string                 `json:"status"`
	Data   []map[string]CostEntry `json:"data"`
}

type CostEntry struct {
	Name      string  `json:"name"`
	CPUCost   float64 `json:"cpuCost"`
	RAMCost   float64 `json:"ramCost"`
	GPUCost   float64 `json:gpuCost`
	TotalCost float64 `json:TotalCost`
}

func validateNonNegativeIdleCosts(t *testing.T, aggregate string, window string) {

	baseURL := "https://demo.infra.opencost.io/model/allocation/compute"

	params := url.Values{}
	params.Set("window", window)
	params.Set("aggregate", aggregate)
	params.Set("includeIdle", "true")
	params.Set("step", "1d")
	params.Set("accumulate", "false")

	fullurl := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	resp, err := http.Get(fullurl)
	if err != nil {
		t.Fatalf("Failed to query OpenCost allocation API with window %s: %v", window, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("OpenCost allocation API returned error for window %s: code=%d", window, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %s", err)
	}
	if len(body) == 0 {
		t.Fatalf("No allocation data returned for window %s", window)
	}

	var parsed AllocationResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %s", err)
	}

	data := &parsed

	foundIdle := false
	for _, entry := range data.Data {
		if idle, ok := entry["__idle__"]; ok {
			foundIdle = true
			costChecks := []struct {
				name  string
				value float64
			}{
				{"total cost", idle.TotalCost},
				{"CPU cost", idle.CPUCost},
				{"RAM cost", idle.RAMCost},
				{"GPU cost", idle.GPUCost},
			}

			for _, check := range costChecks {
				if check.value < 0 {
					t.Errorf("Found negative %s in idle allocation: %f", check.name, check.value)
				}
			}
		}
	}

	if !foundIdle {
		t.Skipf("No __idle__ entries found for window :%s aggregate: %s.", window, aggregate)
	}
}

func TestIdleResourceCostValidation(t *testing.T) {

	windows := []string{"10m", "1h", "12h", "1d", "2d", "7d", "30d"}
	aggregates := []string{"namespace", "cluster"}

	for _, window := range windows {
		for _, aggregate := range aggregates {
			t.Run(window, func(t *testing.T) {
				validateNonNegativeIdleCosts(t, aggregate, window)
			})
		}
	}
}
