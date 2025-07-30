package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

const (
	MCPServerURL = "http://localhost:9003"
	MCPEndpoint  = "/mcp"
	Timeout      = 30 * time.Second
)

// JSON-RPC 2.0 request structure
type MCPRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// JSON-RPC 2.0 response structure
type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
}

type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Test helper to make HTTP requests to MCP server
func makeMCPRequest(t *testing.T, method string, params interface{}) (*MCPResponse, error) {
	request := MCPRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
		Params:  params,
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	client := &http.Client{Timeout: Timeout}
	resp, err := client.Post(MCPServerURL+MCPEndpoint, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var mcpResp MCPResponse
	if err := json.Unmarshal(body, &mcpResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &mcpResp, nil
}

// Test helper to check if MCP server is running
func isMCPServerRunning(t *testing.T) bool {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(MCPServerURL + "/health")
	if err != nil {
		// Try ping method as backup
		_, err := makeMCPRequest(t, "ping", nil)
		return err == nil
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Test 1: MCP Server Status Test
func TestMCPServerStatus(t *testing.T) {
	if !isMCPServerRunning(t) {
		t.Skip("MCP server is not running - start the server and run tests again")
	}

	// Test ping method
	resp, err := makeMCPRequest(t, "ping", nil)
	if err != nil {
		t.Fatalf("Failed to ping MCP server: %v", err)
	}

	if resp.Error != nil {
		t.Errorf("Ping request returned error: %v", resp.Error)
	}

	if resp.Result == nil {
		t.Error("Ping request returned nil result")
	}

	// Verify result contains expected fields
	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Ping result is not a map: %T", resp.Result)
	}

	if status, exists := result["status"]; !exists || status != "ok" {
		t.Errorf("Expected status 'ok', got %v", status)
	}

	if _, exists := result["timestamp"]; !exists {
		t.Error("Expected timestamp in ping response")
	}

	if _, exists := result["version"]; !exists {
		t.Error("Expected version in ping response")
	}

	t.Log("✓ MCP server status test passed")
}

// Test 2: JSON-RPC Protocol Tests
func TestJSONRPCProtocol(t *testing.T) {
	if !isMCPServerRunning(t) {
		t.Skip("MCP server is not running")
	}

	t.Run("Initialize", func(t *testing.T) {
		params := map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"clientInfo": map[string]interface{}{
				"name":    "test-client",
				"version": "1.0.0",
			},
		}

		resp, err := makeMCPRequest(t, "initialize", params)
		if err != nil {
			t.Fatalf("Initialize request failed: %v", err)
		}

		if resp.Error != nil {
			t.Errorf("Initialize returned error: %v", resp.Error)
		}

		if resp.Result == nil {
			t.Fatal("Initialize returned nil result")
		}

		// Verify result structure
		result, ok := resp.Result.(map[string]interface{})
		if !ok {
			t.Fatalf("Initialize result is not a map: %T", resp.Result)
		}

		if protocolVersion, exists := result["protocolVersion"]; !exists || protocolVersion != "2024-11-05" {
			t.Errorf("Expected protocolVersion '2024-11-05', got %v", protocolVersion)
		}

		if serverInfo, exists := result["serverInfo"]; !exists {
			t.Error("Expected serverInfo in initialize response")
		} else {
			serverInfoMap := serverInfo.(map[string]interface{})
			if name, exists := serverInfoMap["name"]; !exists || !strings.Contains(name.(string), "opencost") {
				t.Errorf("Expected server name to contain 'opencost', got %v", name)
			}
		}

		t.Log("✓ Initialize test passed")
	})

	t.Run("ToolsList", func(t *testing.T) {
		resp, err := makeMCPRequest(t, "tools/list", nil)
		if err != nil {
			t.Fatalf("tools/list request failed: %v", err)
		}

		if resp.Error != nil {
			t.Errorf("tools/list returned error: %v", resp.Error)
		}

		if resp.Result == nil {
			t.Fatal("tools/list returned nil result")
		}

		// Verify result structure
		result, ok := resp.Result.(map[string]interface{})
		if !ok {
			t.Fatalf("tools/list result is not a map: %T", resp.Result)
		}

		tools, exists := result["tools"]
		if !exists {
			t.Fatal("Expected 'tools' field in result")
		}

		toolsList, ok := tools.([]interface{})
		if !ok {
			t.Fatalf("Expected tools to be an array: %T", tools)
		}

		if len(toolsList) == 0 {
			t.Error("Expected at least one tool in the list")
		}

		// Verify expected tools are present
		expectedTools := []string{"query_allocations", "query_assets", "query_cloud_costs"}
		foundTools := make(map[string]bool)

		for _, tool := range toolsList {
			toolMap, ok := tool.(map[string]interface{})
			if !ok {
				continue
			}
			if name, exists := toolMap["name"]; exists {
				foundTools[name.(string)] = true
			}
		}

		for _, expectedTool := range expectedTools {
			if !foundTools[expectedTool] {
				t.Errorf("Expected tool '%s' not found in tools list", expectedTool)
			}
		}

		t.Logf("✓ Tools list test passed - found %d tools", len(toolsList))
	})

	t.Run("InvalidMethod", func(t *testing.T) {
		resp, err := makeMCPRequest(t, "invalid/method", nil)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		if resp.Error == nil {
			t.Error("Expected error for invalid method, got nil")
		}

		if resp.Error.Code == 0 {
			t.Error("Expected non-zero error code for invalid method")
		}

		t.Log("✓ Invalid method test passed")
	})
}

// Test 3: MCP Tool Functionality Tests
func TestMCPToolFunctionality(t *testing.T) {
	if !isMCPServerRunning(t) {
		t.Skip("MCP server is not running")
	}

	// First initialize the server
	initParams := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"clientInfo": map[string]interface{}{
			"name":    "test-client",
			"version": "1.0.0",
		},
	}
	_, err := makeMCPRequest(t, "initialize", initParams)
	if err != nil {
		t.Fatalf("Failed to initialize server: %v", err)
	}

	t.Run("QueryAllocations", func(t *testing.T) {
		params := map[string]interface{}{
			"name": "query_allocations",
			"arguments": map[string]interface{}{
				"query":  "Show me namespace costs for the last day",
				"window": "1d",
			},
		}

		resp, err := makeMCPRequest(t, "tools/call", params)
		if err != nil {
			t.Fatalf("query_allocations call failed: %v", err)
		}

		// Should not return JSON-RPC error (but tool might return error content)
		if resp.Error != nil {
			t.Errorf("query_allocations returned JSON-RPC error: %v", resp.Error)
		}

		if resp.Result == nil {
			t.Fatal("query_allocations returned nil result")
		}

		// Verify result structure
		result, ok := resp.Result.(map[string]interface{})
		if !ok {
			t.Fatalf("Result is not a map: %T", resp.Result)
		}

		content, exists := result["content"]
		if !exists {
			t.Fatal("Expected 'content' field in result")
		}

		contentList, ok := content.([]interface{})
		if !ok {
			t.Fatalf("Expected content to be an array: %T", content)
		}

		if len(contentList) == 0 {
			t.Error("Expected at least one content item")
		}

		t.Log("✓ query_allocations tool test passed")
	})

	t.Run("QueryAssets", func(t *testing.T) {
		params := map[string]interface{}{
			"name": "query_assets",
			"arguments": map[string]interface{}{
				"query":  "Show me node utilization",
				"window": "1d",
			},
		}

		resp, err := makeMCPRequest(t, "tools/call", params)
		if err != nil {
			t.Fatalf("query_assets call failed: %v", err)
		}

		if resp.Error != nil {
			t.Errorf("query_assets returned JSON-RPC error: %v", resp.Error)
		}

		if resp.Result == nil {
			t.Fatal("query_assets returned nil result")
		}

		// Verify result structure
		result, ok := resp.Result.(map[string]interface{})
		if !ok {
			t.Fatalf("Result is not a map: %T", resp.Result)
		}

		if _, exists := result["content"]; !exists {
			t.Fatal("Expected 'content' field in result")
		}

		t.Log("✓ query_assets tool test passed")
	})

	t.Run("QueryCloudCosts", func(t *testing.T) {
		params := map[string]interface{}{
			"name": "query_cloud_costs",
			"arguments": map[string]interface{}{
				"query":  "Show me cloud spending by service",
				"window": "7d",
			},
		}

		resp, err := makeMCPRequest(t, "tools/call", params)
		if err != nil {
			t.Fatalf("query_cloud_costs call failed: %v", err)
		}

		if resp.Error != nil {
			t.Errorf("query_cloud_costs returned JSON-RPC error: %v", resp.Error)
		}

		if resp.Result == nil {
			t.Fatal("query_cloud_costs returned nil result")
		}

		// Verify result structure
		result, ok := resp.Result.(map[string]interface{})
		if !ok {
			t.Fatalf("Result is not a map: %T", resp.Result)
		}

		if _, exists := result["content"]; !exists {
			t.Fatal("Expected 'content' field in result")
		}

		t.Log("✓ query_cloud_costs tool test passed")
	})

	t.Run("InvalidTool", func(t *testing.T) {
		params := map[string]interface{}{
			"name": "invalid_tool",
			"arguments": map[string]interface{}{
				"query": "test",
			},
		}

		resp, err := makeMCPRequest(t, "tools/call", params)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		// Should return JSON-RPC error for invalid tool
		if resp.Error == nil {
			t.Error("Expected error for invalid tool, got nil")
		}

		t.Log("✓ Invalid tool test passed")
	})
}

// Test 4: HTTP Protocol Compliance
func TestHTTPProtocolCompliance(t *testing.T) {
	if !isMCPServerRunning(t) {
		t.Skip("MCP server is not running")
	}

	t.Run("ContentType", func(t *testing.T) {
		// Test with wrong content type
		client := &http.Client{Timeout: Timeout}
		req, _ := http.NewRequest("POST", MCPServerURL+MCPEndpoint, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "text/plain")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			t.Error("Expected non-200 status for wrong content type")
		}

		t.Log("✓ Content-Type validation test passed")
	})

	t.Run("HTTPMethod", func(t *testing.T) {
		// Test with GET method
		client := &http.Client{Timeout: Timeout}
		resp, err := client.Get(MCPServerURL + MCPEndpoint)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			t.Error("Expected non-200 status for GET method")
		}

		t.Log("✓ HTTP method validation test passed")
	})

	t.Run("JSONRPCVersion", func(t *testing.T) {
		request := MCPRequest{
			JSONRPC: "1.0", // Wrong version
			ID:      1,
			Method:  "ping",
		}

		jsonData, _ := json.Marshal(request)
		client := &http.Client{Timeout: Timeout}
		resp, err := client.Post(MCPServerURL+MCPEndpoint, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		var mcpResp MCPResponse
		json.Unmarshal(body, &mcpResp)

		if mcpResp.Error == nil {
			t.Error("Expected error for wrong JSON-RPC version")
		}

		t.Log("✓ JSON-RPC version validation test passed")
	})
}

// Test 5: Performance and Load Test
func TestMCPServerPerformance(t *testing.T) {
	if !isMCPServerRunning(t) {
		t.Skip("MCP server is not running")
	}

	// Initialize server
	initParams := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"clientInfo": map[string]interface{}{
			"name":    "test-client",
			"version": "1.0.0",
		},
	}
	_, err := makeMCPRequest(t, "initialize", initParams)
	if err != nil {
		t.Fatalf("Failed to initialize server: %v", err)
	}

	t.Run("ResponseTime", func(t *testing.T) {
		start := time.Now()
		_, err := makeMCPRequest(t, "ping", nil)
		duration := time.Since(start)

		if err != nil {
			t.Fatalf("Ping request failed: %v", err)
		}

		if duration > 5*time.Second {
			t.Errorf("Ping response took too long: %v", duration)
		}

		t.Logf("✓ Response time test passed - ping took %v", duration)
	})

	t.Run("ConcurrentRequests", func(t *testing.T) {
		const numRequests = 5
		results := make(chan error, numRequests)

		for i := 0; i < numRequests; i++ {
			go func() {
				_, err := makeMCPRequest(t, "ping", nil)
				results <- err
			}()
		}

		for i := 0; i < numRequests; i++ {
			if err := <-results; err != nil {
				t.Errorf("Concurrent request %d failed: %v", i, err)
			}
		}

		t.Log("✓ Concurrent requests test passed")
	})
}

// Test 6: Integration Test
func TestMCPIntegration(t *testing.T) {
	if !isMCPServerRunning(t) {
		t.Skip("MCP server is not running")
	}

	t.Run("FullWorkflow", func(t *testing.T) {
		// 1. Initialize
		initParams := map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"clientInfo": map[string]interface{}{
				"name":    "integration-test",
				"version": "1.0.0",
			},
		}
		initResp, err := makeMCPRequest(t, "initialize", initParams)
		if err != nil || initResp.Error != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		// 2. List tools
		toolsResp, err := makeMCPRequest(t, "tools/list", nil)
		if err != nil || toolsResp.Error != nil {
			t.Fatalf("Tools list failed: %v", err)
		}

		// 3. Call a tool
		callParams := map[string]interface{}{
			"name": "query_allocations",
			"arguments": map[string]interface{}{
				"query":  "Integration test query",
				"window": "1h",
			},
		}
		callResp, err := makeMCPRequest(t, "tools/call", callParams)
		if err != nil || callResp.Error != nil {
			t.Fatalf("Tool call failed: %v", err)
		}

		// 4. Ping to verify server is still responsive
		pingResp, err := makeMCPRequest(t, "ping", nil)
		if err != nil || pingResp.Error != nil {
			t.Fatalf("Final ping failed: %v", err)
		}

		t.Log("✓ Full workflow integration test passed")
	})
}