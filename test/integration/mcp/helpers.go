package mcp

// Shared helper functions and types for MCP integration tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"github.com/opencost/opencost-integration-tests/pkg/env"
)

// MCPResponse represents the MCP server response
type MCPResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Result  struct {
		Content []struct {
			Type string          `json:"type"`
			Text json.RawMessage `json:"text,omitempty"`
		} `json:"content"`
	} `json:"result,omitempty"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// getMCPURL returns the MCP HTTP server URL
func getMCPURL() string {
	mcpURL := env.GetMCPURL()
	if mcpURL == "" {
		// Default to port 8081 if not set
		return "http://localhost:8081"
	}
	return mcpURL
}

// initializeMCPSession initializes an MCP session and returns the session ID
func initializeMCPSession() (string, error) {
	url := getMCPURL() + "/mcp"

	initReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "initialize",
		"id":      1,
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]string{
				"name":    "opencost-integration-test",
				"version": "1.0.0",
			},
		},
	}

	jsonData, err := json.Marshal(initReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal init request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to initialize MCP session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("MCP init returned status %d: %s", resp.StatusCode, string(body))
	}

	// Extract session ID from response header
	sessionID := resp.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		return "", fmt.Errorf("no session ID returned from MCP server")
	}

	// Send initialized notification with session ID
	notifyReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	}

	jsonData, err = json.Marshal(notifyReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal notify request: %w", err)
	}

	httpReq, err = http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create notify request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	httpReq.Header.Set("Mcp-Session-Id", sessionID)

	resp, err = client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to send initialized notification: %w", err)
	}
	defer resp.Body.Close()

	return sessionID, nil
}

// abs returns absolute value of a float64
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
