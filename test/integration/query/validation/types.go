// test/integration/query/validation/types.go
package validation

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
)

// AllocationResponse represents the structure of the API response
type AllocationResponse struct {
	Code   int                     `json:"code"`
	Status string                  `json:"status"`
	Data   []map[string]Allocation `json:"data"`
}

// Allocation represents the structure of each allocation entry
type Allocation struct {
	Name                       string                 `json:"name"`
	Properties                 map[string]interface{} `json:"properties"`
	Window                     map[string]string      `json:"window"`
	Start                      string                 `json:"start"`
	End                        string                 `json:"end"`
	Minutes                    int                    `json:"minutes"`
	CPUCores                   float64                `json:"cpuCores"`
	CPUCoreRequestAverage      float64                `json:"cpuCoreRequestAverage"`
	CPUCoreUsageAverage        float64                `json:"cpuCoreUsageAverage"`
	CPUCoreHours               float64                `json:"cpuCoreHours"`
	CPUCost                    float64                `json:"cpuCost"`
	CPUCostAdjustment          float64                `json:"cpuCostAdjustment"`
	CPUCostIdle                float64                `json:"cpuCostIdle"`
	CPUEfficiency              float64                `json:"cpuEfficiency"`
	GPUCount                   float64                `json:"gpuCount"`
	GPUHours                   float64                `json:"gpuHours"`
	GPUCost                    float64                `json:"gpuCost"`
	GPUCostAdjustment          float64                `json:"gpuCostAdjustment"`
	GPUCostIdle                float64                `json:"gpuCostIdle"`
	GPUEfficiency              float64                `json:"gpuEfficiency"`
	RAMBytes                   float64                `json:"ramBytes"`
	RAMByteRequestAverage      float64                `json:"ramByteRequestAverage"`
	RAMByteUsageAverage        float64                `json:"ramByteUsageAverage"`
	RAMByteHours               float64                `json:"ramByteHours"`
	RAMCost                    float64                `json:"ramCost"`
	RAMCostAdjustment          float64                `json:"ramCostAdjustment"`
	RAMCostIdle                float64                `json:"ramCostIdle"`
	RAMEfficiency              float64                `json:"ramEfficiency"`
	NetworkCost                float64                `json:"networkCost"`
	NetworkCrossZoneCost       float64                `json:"networkCrossZoneCost"`
	NetworkCrossRegionCost     float64                `json:"networkCrossRegionCost"`
	NetworkInternetCost        float64                `json:"networkInternetCost"`
	NetworkCostAdjustment      float64                `json:"networkCostAdjustment"`
	LoadBalancerCost           float64                `json:"loadBalancerCost"`
	LoadBalancerCostAdjustment float64                `json:"loadBalancerCostAdjustment"`
	PVBytes                    float64                `json:"pvBytes"`
	PVByteHours                float64                `json:"pvByteHours"`
	PVCost                     float64                `json:"pvCost"`
	PVCostAdjustment           float64                `json:"pvCostAdjustment"`
	ExternalCost               float64                `json:"externalCost"`
	SharedCost                 float64                `json:"sharedCost"`
	TotalCost                  float64                `json:"totalCost"`
	TotalEfficiency            float64                `json:"totalEfficiency"`
}

// Helper function to fetch allocation data from the API
func FetchAllocationData(includeIdle bool) (AllocationResponse, error) {
	var allocResp AllocationResponse

	// Get the OpenCost URL from the environment
	opencostURL := os.Getenv("OPENCOST_URL")
	if opencostURL == "" {
		return allocResp, fmt.Errorf("OPENCOST_URL environment variable is not set")
	}

	// Format the API endpoint for allocation data
	endpoint := fmt.Sprintf("%s/allocation/compute", opencostURL)

	// Add query parameters
	window := "1d"
	aggregate := "namespace"
	includeIdleStr := "false"
	if includeIdle {
		includeIdleStr = "true"
	}
	step := "1d"
	accumulate := "false"

	url := fmt.Sprintf("%s?window=%s&aggregate=%s&includeIdle=%s&step=%s&accumulate=%s",
		endpoint, window, aggregate, includeIdleStr, step, accumulate)

	// Make the request
	resp, err := http.Get(url)
	if err != nil {
		return allocResp, fmt.Errorf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Check for successful response
	if resp.StatusCode != http.StatusOK {
		return allocResp, fmt.Errorf("request failed with status code: %d", resp.StatusCode)
	}

	// Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return allocResp, fmt.Errorf("failed to read response body: %v", err)
	}

	// Parse the JSON response
	if err := json.Unmarshal(body, &allocResp); err != nil {
		return allocResp, fmt.Errorf("failed to parse JSON response: %v", err)
	}

	return allocResp, nil
}
