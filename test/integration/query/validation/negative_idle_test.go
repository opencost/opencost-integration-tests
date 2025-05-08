// test/integration/query/validation/negative_idle_test.go
package validation

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"testing"
)

type AllocationResponse struct {
	Code   int                     `json:"code"`
	Status string                  `json:"status"`
	Data   []map[string]Allocation `json:"data"`
}

type Allocation struct {
	Name       string                 `json:"name"`
	Properties map[string]interface{} `json:"properties"`
	Window     map[string]string      `json:"window"`
	Start      string                 `json:"start"`
	End        string                 `json:"end"`
	Minutes    int                    `json:"minutes"`
	CPUCores   float64                `json:"cpuCores"`
	// Many other fields...
	CPUCost   float64 `json:"cpuCost"`
	RAMCost   float64 `json:"ramCost"`
	GPUCost   float64 `json:"gpuCost"`
	TotalCost float64 `json:"totalCost"`
}

func TestNegativeIdleCosts(t *testing.T) {
	opencostURL := os.Getenv("OPENCOST_URL")
	if opencostURL == "" {
		t.Fatal("OPENCOST_URL environment variable is not set")
	}

	endpoint := fmt.Sprintf("%s/allocation/compute", opencostURL)

	// Add query parameters
	window := "1d"
	aggregate := "namespace"
	includeIdle := "true"
	step := "1d"
	accumulate := "false"

	url := fmt.Sprintf("%s?window=%s&aggregate=%s&includeIdle=%s&step=%s&accumulate=%s",
		endpoint, window, aggregate, includeIdle, step, accumulate)

	// Make the request
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Check for successful response
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Request failed with status code: %d", resp.StatusCode)
	}

	// Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	// Parse the JSON response
	var allocResp AllocationResponse
	if err := json.Unmarshal(body, &allocResp); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	var foundNegativeValues bool
	var negativeFields []string

	for _, allocSet := range allocResp.Data {
		for namespaceName, alloc := range allocSet {
			if alloc.CPUCost < 0 {
				negativeFields = append(negativeFields, fmt.Sprintf("%s.cpuCost = %.5f", namespaceName, alloc.CPUCost))
				foundNegativeValues = true
			}
			if alloc.RAMCost < 0 {
				negativeFields = append(negativeFields, fmt.Sprintf("%s.ramCost = %.5f", namespaceName, alloc.RAMCost))
				foundNegativeValues = true
			}
			if alloc.GPUCost < 0 {
				negativeFields = append(negativeFields, fmt.Sprintf("%s.gpuCost = %.5f", namespaceName, alloc.GPUCost))
				foundNegativeValues = true
			}
			if alloc.TotalCost < 0 {
				negativeFields = append(negativeFields, fmt.Sprintf("%s.totalCost = %.5f", namespaceName, alloc.TotalCost))
				foundNegativeValues = true
			}
		}
	}

	// Fail the test if negative values were found
	if foundNegativeValues {
		t.Errorf("Found negative cost values in the API response: %v", negativeFields)
	}
}
