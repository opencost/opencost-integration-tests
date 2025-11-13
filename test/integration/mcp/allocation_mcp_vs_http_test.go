package mcp

// Integration tests to compare MCP get_allocation_costs tool results with HTTP API /allocation endpoint results
// This ensures that the MCP tool returns the same data as the HTTP API for allocation queries
//
// Note: All tests use historical time windows (yesterday and earlier) to ensure consistent, 
// reproducible results that don't change as new data arrives in the present.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
	"github.com/opencost/opencost-integration-tests/pkg/api"
)

// MCPAllocationRequest represents the request structure for get_allocation_costs MCP tool
type MCPAllocationRequest struct {
	Window                                string `json:"window"`
	Aggregate                             string `json:"aggregate"`
	Step                                  string `json:"step,omitempty"`
	Accumulate                            bool   `json:"accumulate,omitempty"`
	ShareIdle                             bool   `json:"share_idle,omitempty"`
	IncludeIdle                           bool   `json:"include_idle,omitempty"`
	IdleByNode                            bool   `json:"idle_by_node,omitempty"`
	IncludeProportionalAssetResourceCosts bool   `json:"include_proportional_asset_resource_costs,omitempty"`
	IncludeAggregatedMetadata             bool   `json:"include_aggregated_metadata,omitempty"`
	ShareLB                               bool   `json:"share_lb,omitempty"`
	Filter                                string `json:"filter,omitempty"`
}

// MCPToolRequest wraps the tool call in MCP format (JSON-RPC 2.0)
type MCPToolRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	ID      int    `json:"id"`
	Params  struct {
		Name      string               `json:"name"`
		Arguments MCPAllocationRequest `json:"arguments"`
	} `json:"params"`
}

// MCPAllocationData matches the MCP tool response format
type MCPAllocationData struct {
	Data struct {
		Allocations map[string]struct {
			Name        string `json:"name"`
			Allocations []struct {
				Name         string    `json:"name"`
				CPUCost      float64   `json:"cpuCost"`
				GPUCost      float64   `json:"gpuCost"`
				RAMCost      float64   `json:"ramCost"`
				PVCost       float64   `json:"pvCost"`
				NetworkCost  float64   `json:"networkCost"`
				SharedCost   float64   `json:"sharedCost"`
				ExternalCost float64   `json:"externalCost"`
				TotalCost    float64   `json:"totalCost"`
				CPUCoreHours float64   `json:"cpuCoreHours"`
				RAMByteHours float64   `json:"ramByteHours"`
				GPUHours     float64   `json:"gpuHours"`
				PVByteHours  float64   `json:"pvByteHours"`
				Start        time.Time `json:"start"`
				End          time.Time `json:"end"`
			} `json:"allocations"`
		} `json:"allocations"`
	} `json:"data"`
	QueryInfo struct {
		QueryID        string  `json:"queryId"`
		Timestamp      string  `json:"timestamp"`
		ProcessingTime float64 `json:"processingTime"`
	} `json:"queryInfo"`
}

// callMCPTool calls the MCP get_allocation_costs tool
func callMCPTool(req MCPAllocationRequest) (*MCPAllocationData, error) {
	// Initialize session first
	sessionID, err := initializeMCPSession()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize MCP session: %w", err)
	}

	mcpReq := MCPToolRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		ID:      1,
	}
	mcpReq.Params.Name = "get_allocation_costs"
	mcpReq.Params.Arguments = req

	jsonData, err := json.Marshal(mcpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal MCP request: %w", err)
	}

	url := getMCPURL() + "/mcp"

	// Create request with proper headers for MCP server
	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	httpReq.Header.Set("Mcp-Session-Id", sessionID)

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call MCP tool: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("MCP tool returned status %d: %s", resp.StatusCode, string(body))
	}

	var mcpResp MCPResponse
	if err := json.NewDecoder(resp.Body).Decode(&mcpResp); err != nil {
		return nil, fmt.Errorf("failed to decode MCP response: %w", err)
	}

	// Check for errors
	if mcpResp.Error != nil {
		return nil, fmt.Errorf("MCP error (code %d): %s", mcpResp.Error.Code, mcpResp.Error.Message)
	}

	// Extract the actual data from the content array
	if len(mcpResp.Result.Content) == 0 {
		return nil, fmt.Errorf("empty content in MCP response")
	}

	// The text field contains a JSON string, so we need to unmarshal it twice
	// First unmarshal to get the string, then unmarshal the string to get the data
	var textStr string
	if err := json.Unmarshal(mcpResp.Result.Content[0].Text, &textStr); err != nil {
		// If it's not a string, try unmarshaling directly
		var mcpData MCPAllocationData
		if err2 := json.Unmarshal(mcpResp.Result.Content[0].Text, &mcpData); err2 != nil {
			return nil, fmt.Errorf("failed to decode MCP allocation data (tried both string and direct): %w, %w", err, err2)
		}
		return &mcpData, nil
	}

	// Now unmarshal the JSON string into the actual data structure
	var mcpData MCPAllocationData
	if err := json.Unmarshal([]byte(textStr), &mcpData); err != nil {
		return nil, fmt.Errorf("failed to decode MCP allocation data from string: %w", err)
	}

	return &mcpData, nil
}

// TestMCPAllocationVsHTTP compares MCP tool results with HTTP API results
// Uses only historical time windows (excluding present) to ensure consistent results
func TestMCPAllocationVsHTTP(t *testing.T) {
	apiObj := api.NewAPI()

	// Generate historical time windows (yesterday and earlier)
	now := time.Now().UTC()
	yesterday := now.AddDate(0, 0, -1)
	twoDaysAgo := now.AddDate(0, 0, -2)
	threeDaysAgo := now.AddDate(0, 0, -3)
	sevenDaysAgo := now.AddDate(0, 0, -7)

	testCases := []struct {
		name       string
		window     string
		aggregate  string
		accumulate bool
		filter     string
	}{
		{
			name:       "Yesterday namespace aggregation",
			window:     "yesterday",
			aggregate:  "namespace",
			accumulate: false,
		},
		{
			name:       "2 days ago cluster aggregation",
			window:     fmt.Sprintf("%s,%s", twoDaysAgo.Format("2006-01-02T15:04:05Z"), yesterday.Format("2006-01-02T15:04:05Z")),
			aggregate:  "cluster",
			accumulate: false,
		},
		{
			name:       "Last 3 days (historical) pod aggregation",
			window:     fmt.Sprintf("%s,%s", threeDaysAgo.Format("2006-01-02T15:04:05Z"), yesterday.Format("2006-01-02T15:04:05Z")),
			aggregate:  "pod",
			accumulate: false,
		},
		{
			name:       "Yesterday namespace with accumulate",
			window:     "yesterday",
			aggregate:  "namespace",
			accumulate: true,
		},
		{
			name:       "Last 7 days (historical) controller aggregation",
			window:     fmt.Sprintf("%s,%s", sevenDaysAgo.Format("2006-01-02T15:04:05Z"), yesterday.Format("2006-01-02T15:04:05Z")),
			aggregate:  "controller",
			accumulate: false,
		},
		{
			name:       "Last week (7-14 days ago) namespace aggregation",
			window:     fmt.Sprintf("%s,%s", now.AddDate(0, 0, -14).Format("2006-01-02T15:04:05Z"), sevenDaysAgo.Format("2006-01-02T15:04:05Z")),
			aggregate:  "namespace",
			accumulate: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Get HTTP API response
			httpResp, err := apiObj.GetAllocation(api.AllocationRequest{
				Window:     tc.window,
				Aggregate:  tc.aggregate,
				Accumulate: fmt.Sprintf("%t", tc.accumulate),
				Filter:     tc.filter,
			})
			if err != nil {
				t.Fatalf("Failed to get HTTP API response: %v", err)
			}

			if httpResp.Code != 200 {
				t.Fatalf("HTTP API returned non-200 code: %d", httpResp.Code)
			}

			// Get MCP tool response
			mcpResp, err := callMCPTool(MCPAllocationRequest{
				Window:     tc.window,
				Aggregate:  tc.aggregate,
				Accumulate: tc.accumulate,
				Filter:     tc.filter,
			})
			if err != nil {
				t.Fatalf("Failed to get MCP tool response: %v", err)
			}

			// Compare results
			compareMCPWithHTTP(t, mcpResp, httpResp)
		})
	}
}

// compareMCPWithHTTP compares MCP data with HTTP API data
func compareMCPWithHTTP(t *testing.T, mcpData *MCPAllocationData, httpData *api.AllocationResponse) {
	// Get the allocations from MCP response
	mcpAllocations := mcpData.Data.Allocations["allocations"]
	if len(mcpAllocations.Allocations) == 0 {
		t.Log("Warning: MCP response has no allocations")
	}

	// HTTP API returns array of maps, MCP returns single allocation set
	if len(httpData.Data) == 0 {
		t.Log("Warning: HTTP response has no data")
	}

	// Build maps for comparison
	mcpMap := make(map[string]float64)
	for _, alloc := range mcpAllocations.Allocations {
		mcpMap[alloc.Name] = alloc.TotalCost
	}

	httpMap := make(map[string]float64)
	for _, dataMap := range httpData.Data {
		for name, item := range dataMap {
			httpMap[name] = item.TotalCost
		}
	}

	// Compare allocation names
	for name := range mcpMap {
		if _, exists := httpMap[name]; !exists {
			t.Errorf("Allocation '%s' exists in MCP but not in HTTP API", name)
		}
	}

	for name := range httpMap {
		if _, exists := mcpMap[name]; !exists {
			t.Errorf("Allocation '%s' exists in HTTP API but not in MCP", name)
		}
	}

	// Compare costs with tolerance for floating point differences
	const tolerance = 0.01
	for name, mcpCost := range mcpMap {
		if httpCost, exists := httpMap[name]; exists {
			diff := abs(mcpCost - httpCost)
			percentDiff := 0.0
			if httpCost != 0 {
				percentDiff = (diff / httpCost) * 100
			}

			if diff > tolerance && percentDiff > 0.1 {
				t.Errorf("Cost mismatch for '%s': MCP=%.4f, HTTP=%.4f (diff=%.4f, %.2f%%)",
					name, mcpCost, httpCost, diff, percentDiff)
			} else {
				t.Logf("Cost match for '%s': MCP=%.4f, HTTP=%.4f", name, mcpCost, httpCost)
			}
		}
	}

	// Log summary
	t.Logf("Comparison complete: %d MCP allocations, %d HTTP allocations",
		len(mcpMap), len(httpMap))
}

// TestMCPAllocationWithFilters tests MCP allocation queries with various filters
// Uses only historical time windows (excluding present) to ensure consistent results
func TestMCPAllocationWithFilters(t *testing.T) {
	apiObj := api.NewAPI()

	testCases := []struct {
		name      string
		window    string
		aggregate string
		filter    string
	}{
		{
			name:      "Filter by namespace (yesterday)",
			window:    "yesterday",
			aggregate: "pod",
			filter:    "namespace:\"kube-system\"",
		},
		{
			name:      "Filter by cluster (yesterday)",
			window:    "yesterday",
			aggregate: "namespace",
			filter:    "cluster:\"tilt-cluster\"",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Get HTTP API response
			httpResp, err := apiObj.GetAllocation(api.AllocationRequest{
				Window:    tc.window,
				Aggregate: tc.aggregate,
				Filter:    tc.filter,
			})
			if err != nil {
				t.Fatalf("Failed to get HTTP API response: %v", err)
			}

			if httpResp.Code != 200 {
				t.Fatalf("HTTP API returned non-200 code: %d", httpResp.Code)
			}

			// Get MCP tool response
			mcpResp, err := callMCPTool(MCPAllocationRequest{
				Window:    tc.window,
				Aggregate: tc.aggregate,
				Filter:    tc.filter,
			})
			if err != nil {
				t.Fatalf("Failed to get MCP tool response: %v", err)
			}

			// Compare results
			compareMCPWithHTTP(t, mcpResp, httpResp)
		})
	}
}

// TestMCPAllocationWithIdleAndShare tests MCP allocation queries with idle and share options
// Uses only historical time windows (excluding present) to ensure consistent results
func TestMCPAllocationWithIdleAndShare(t *testing.T) {
	apiObj := api.NewAPI()

	testCases := []struct {
		name        string
		window      string
		aggregate   string
		includeIdle bool
		shareIdle   bool
	}{
		{
			name:        "Include idle (yesterday)",
			window:      "yesterday",
			aggregate:   "namespace",
			includeIdle: true,
			shareIdle:   false,
		},
		{
			name:        "Share idle (yesterday)",
			window:      "yesterday",
			aggregate:   "pod",
			includeIdle: false,
			shareIdle:   true,
		},
		{
			name:        "Include and share idle (yesterday)",
			window:      "yesterday",
			aggregate:   "namespace",
			includeIdle: true,
			shareIdle:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Get HTTP API response
			httpResp, err := apiObj.GetAllocation(api.AllocationRequest{
				Window:      tc.window,
				Aggregate:   tc.aggregate,
				IncludeIdle: fmt.Sprintf("%t", tc.includeIdle),
				ShareIdle:   fmt.Sprintf("%t", tc.shareIdle),
			})
			if err != nil {
				t.Fatalf("Failed to get HTTP API response: %v", err)
			}

			if httpResp.Code != 200 {
				t.Fatalf("HTTP API returned non-200 code: %d", httpResp.Code)
			}

			// Get MCP tool response
			mcpResp, err := callMCPTool(MCPAllocationRequest{
				Window:      tc.window,
				Aggregate:   tc.aggregate,
				IncludeIdle: tc.includeIdle,
				ShareIdle:   tc.shareIdle,
			})
			if err != nil {
				t.Fatalf("Failed to get MCP tool response: %v", err)
			}

			// Compare results
			compareMCPWithHTTP(t, mcpResp, httpResp)
		})
	}
}

