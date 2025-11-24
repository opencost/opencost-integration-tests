package mcp

// Integration tests to compare MCP get_asset_costs tool results with HTTP API /assets endpoint results
// This ensures that the MCP tool returns the same data as the HTTP API for asset queries
//
// Note: All tests use historical time windows (yesterday and earlier) to ensure consistent,
// reproducible results that don't change as new data arrives in the present.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
	"github.com/opencost/opencost-integration-tests/pkg/api"
)

// MCPAssetRequest represents the request structure for get_asset_costs MCP tool
type MCPAssetRequest struct {
	Window string `json:"window"`
}

// MCPAssetData matches the MCP tool response format for assets
type MCPAssetData struct {
	Data struct {
		Assets map[string]struct {
			Name   string `json:"name"`
			Assets []struct {
				Type       string            `json:"type"`
				Properties map[string]string `json:"properties"`
				Labels     map[string]string `json:"labels,omitempty"`
				CPUCost    float64           `json:"cpuCost"`
				RAMCost    float64           `json:"ramCost"`
				GPUCost    float64           `json:"gpuCost"`
				TotalCost  float64           `json:"totalCost"`
				Start      time.Time         `json:"start"`
				End        time.Time         `json:"end"`
			} `json:"assets"`
		} `json:"assets"`
	} `json:"data"`
	QueryInfo struct {
		QueryID        string  `json:"queryId"`
		Timestamp      string  `json:"timestamp"`
		ProcessingTime float64 `json:"processingTime"`
	} `json:"queryInfo"`
}

// callMCPAssetTool calls the MCP get_asset_costs tool
func callMCPAssetTool(req MCPAssetRequest) (*MCPAssetData, error) {
	sessionID, err := initializeMCPSession()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize MCP session: %w", err)
	}

	mcpReq := struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		ID      int    `json:"id"`
		Params  struct {
			Name      string          `json:"name"`
			Arguments MCPAssetRequest `json:"arguments"`
		} `json:"params"`
	}{
		JSONRPC: "2.0",
		Method:  "tools/call",
		ID:      1,
	}
	mcpReq.Params.Name = "get_asset_costs"
	mcpReq.Params.Arguments = req

	jsonData, err := json.Marshal(mcpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal MCP request: %w", err)
	}

	url := getMCPURL() + "/mcp"
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

	if mcpResp.Error != nil {
		return nil, fmt.Errorf("MCP error (code %d): %s", mcpResp.Error.Code, mcpResp.Error.Message)
	}

	if len(mcpResp.Result.Content) == 0 {
		return nil, fmt.Errorf("empty content in MCP response")
	}

	var textStr string
	if err := json.Unmarshal(mcpResp.Result.Content[0].Text, &textStr); err != nil {
		var mcpData MCPAssetData
		if err2 := json.Unmarshal(mcpResp.Result.Content[0].Text, &mcpData); err2 != nil {
			return nil, fmt.Errorf("failed to decode MCP asset data: %w, %w", err, err2)
		}
		return &mcpData, nil
	}

	var mcpData MCPAssetData
	if err := json.Unmarshal([]byte(textStr), &mcpData); err != nil {
		return nil, fmt.Errorf("failed to decode MCP asset data from string: %w", err)
	}

	return &mcpData, nil
}

// TestMCPAssetVsHTTP compares MCP tool results with HTTP API results
func TestMCPAssetVsHTTP(t *testing.T) {
	apiObj := api.NewAPI()

	now := time.Now().UTC()
	yesterday := now.AddDate(0, 0, -1)
	threeDaysAgo := now.AddDate(0, 0, -3)
	sevenDaysAgo := now.AddDate(0, 0, -7)

	testCases := []struct {
		name   string
		window string
	}{
		{
			name:   "Yesterday assets",
			window: "yesterday",
		},
		{
			name:   "Last 3 days (historical) assets",
			window: fmt.Sprintf("%s,%s", threeDaysAgo.Format("2006-01-02T15:04:05Z"), yesterday.Format("2006-01-02T15:04:05Z")),
		},
		{
			name:   "Last 7 days (historical) assets",
			window: fmt.Sprintf("%s,%s", sevenDaysAgo.Format("2006-01-02T15:04:05Z"), yesterday.Format("2006-01-02T15:04:05Z")),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpResp, err := apiObj.GetAssets(api.AssetsRequest{
				Window: tc.window,
			})
			if err != nil {
				t.Fatalf("Failed to get HTTP API response: %v", err)
			}

			if httpResp.Code != 200 {
				t.Fatalf("HTTP API returned non-200 code: %d", httpResp.Code)
			}

			mcpResp, err := callMCPAssetTool(MCPAssetRequest{
				Window: tc.window,
			})
			if err != nil {
				t.Fatalf("Failed to get MCP tool response: %v", err)
			}

			compareMCPAssetWithHTTP(t, mcpResp, httpResp)
		})
	}
}

// extractAssetName extracts the simplified asset name from a fully-qualified HTTP API asset path
// For example: "custom/__undefined__/__undefined__/Compute/tilt-cluster/Node/Kubernetes/ocid1.../10.0.151.228" -> "10.0.151.228"
// Special cases:
// - Management assets ending with "__undefined__" map to the asset type (e.g., "ClusterManagement")
// - LoadBalancer assets have the format: .../LoadBalancer/Kubernetes/IP/namespace/service-name
func extractAssetName(fullPath string) string {
	parts := strings.Split(fullPath, "/")
	if len(parts) == 0 {
		return fullPath
	}

	lastPart := parts[len(parts)-1]

	// Special case: Management assets that end with __undefined__
	// These should use the asset type name (e.g., ClusterManagement)
	if lastPart == "__undefined__" && len(parts) >= 6 {
		for i := len(parts) - 2; i >= 0; i-- {
			if parts[i] != "__undefined__" && parts[i] != "Kubernetes" {
				return parts[i]
			}
		}
	}

	// Special case: LoadBalancer assets
	// Path format: .../Network/.../LoadBalancer/Kubernetes/IP/namespace/service-name
	// We want to return "namespace/service-name"
	for i, part := range parts {
		if part == "LoadBalancer" && i+4 < len(parts) {
			// Return namespace/service-name format (skip IP, use namespace/service-name)
			return parts[i+3] + "/" + parts[i+4]
		}
	}

	return lastPart
}

// compareMCPAssetWithHTTP compares MCP asset data with HTTP API asset data
func compareMCPAssetWithHTTP(t *testing.T, mcpData *MCPAssetData, httpData *api.AssetsResponse) {
	mcpAssets := mcpData.Data.Assets["assets"]
	if len(mcpAssets.Assets) == 0 {
		t.Log("Warning: MCP response has no assets")
	}

	if len(httpData.Data) == 0 {
		t.Log("Warning: HTTP response has no data")
	}

	mcpMap := make(map[string]float64)
	for _, asset := range mcpAssets.Assets {
		name := asset.Properties["name"]
		if name == "" {
			name = asset.Type
		}
		mcpMap[name] = asset.TotalCost
	}

	// Build HTTP map with extracted simplified names
	httpMap := make(map[string]float64)
	httpFullPathMap := make(map[string]string) // Maps simplified name to full path for logging
	for fullPath, item := range httpData.Data {
		simplifiedName := extractAssetName(fullPath)
		httpMap[simplifiedName] = item.TotalCost
		httpFullPathMap[simplifiedName] = fullPath
	}

	for name := range mcpMap {
		if _, exists := httpMap[name]; !exists {
			t.Errorf("Asset '%s' exists in MCP but not in HTTP API", name)
		}
	}

	for name := range httpMap {
		if _, exists := mcpMap[name]; !exists {
			t.Errorf("Asset '%s' exists in HTTP API but not in MCP (full path: %s)", name, httpFullPathMap[name])
		}
	}

	const tolerance = 0.01
	matchCount := 0
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
				matchCount++
				t.Logf("Cost match for '%s': MCP=%.4f, HTTP=%.4f", name, mcpCost, httpCost)
			}
		}
	}

	t.Logf("Comparison complete: %d MCP assets, %d HTTP assets, %d matches",
		len(mcpMap), len(httpMap), matchCount)
}

