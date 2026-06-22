package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

type MCPEfficiencyRequest struct {
	Window           string   `json:"window"`
	Aggregate        string   `json:"aggregate,omitempty"`
	Filter           string   `json:"filter,omitempty"`
	BufferMultiplier *float64 `json:"buffer_multiplier,omitempty"`
	Step             string   `json:"step,omitempty"`
}

type MCPEfficiencyData struct {
	Data struct {
		Efficiencies []struct {
			Name                      string    `json:"name"`
			CPUEfficiency             float64   `json:"cpuEfficiency"`
			MemoryEfficiency          float64   `json:"memoryEfficiency"`
			CPUCoresRequested         float64   `json:"cpuCoresRequested"`
			CPUCoresUsed              float64   `json:"cpuCoresUsed"`
			RAMBytesRequested         float64   `json:"ramBytesRequested"`
			RAMBytesUsed              float64   `json:"ramBytesUsed"`
			RecommendedCPURequest     float64   `json:"recommendedCpuRequest"`
			RecommendedRAMRequest     float64   `json:"recommendedRamRequest"`
			ResultingCPUEfficiency    float64   `json:"resultingCpuEfficiency"`
			ResultingMemoryEfficiency float64   `json:"resultingMemoryEfficiency"`
			CurrentTotalCost          float64   `json:"currentTotalCost"`
			RecommendedCost           float64   `json:"recommendedCost"`
			CostSavings               float64   `json:"costSavings"`
			CostSavingsPercent        float64   `json:"costSavingsPercent"`
			Start                     time.Time `json:"start"`
			End                       time.Time `json:"end"`
		} `json:"efficiencies"`
	} `json:"data"`
}

func callMCPEfficiencyTool(req MCPEfficiencyRequest) (*MCPEfficiencyData, error) {
	sessionID, err := initializeMCPSession()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize MCP session: %w", err)
	}

	mcpReq := struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		ID      int    `json:"id"`
		Params  struct {
			Name      string               `json:"name"`
			Arguments MCPEfficiencyRequest `json:"arguments"`
		} `json:"params"`
	}{
		JSONRPC: "2.0",
		Method:  "tools/call",
		ID:      1,
	}
	mcpReq.Params.Name = "get_efficiency"
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
		var data MCPEfficiencyData
		if err2 := json.Unmarshal(mcpResp.Result.Content[0].Text, &data); err2 != nil {
			return nil, fmt.Errorf("failed to decode MCP efficiency data: %w, %w", err, err2)
		}
		return &data, nil
	}

	var data MCPEfficiencyData
	if err := json.Unmarshal([]byte(textStr), &data); err != nil {
		return nil, fmt.Errorf("failed to decode MCP efficiency data from string: %w", err)
	}

	return &data, nil
}

// TestMCPEfficiencyStepConsistency verifies that the step parameter produces
// functionally equivalent results to a no-step (full-window) query.
// Uses historical windows to ensure consistent, reproducible results.
func TestMCPEfficiencyStepConsistency(t *testing.T) {
	testCases := []struct {
		name      string
		window    string
		aggregate string
		step      string
	}{
		{
			name:      "Yesterday namespace with 1h step",
			window:    "yesterday",
			aggregate: "namespace",
			step:      "1h",
		},
		{
			name:      "Yesterday pod with 6h step",
			window:    "yesterday",
			aggregate: "pod",
			step:      "6h",
		},
		{
			name:      "3d namespace with 1h step",
			window:    fmt.Sprintf("%s,%s", time.Now().UTC().AddDate(0, 0, -4).Format("2006-01-02T15:04:05Z"), time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02T15:04:05Z")),
			aggregate: "namespace",
			step:      "1h",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			noStepResp, err := callMCPEfficiencyTool(MCPEfficiencyRequest{
				Window:    tc.window,
				Aggregate: tc.aggregate,
			})
			if err != nil {
				t.Fatalf("Failed to get efficiency without step: %v", err)
			}

			steppedResp, err := callMCPEfficiencyTool(MCPEfficiencyRequest{
				Window:    tc.window,
				Aggregate: tc.aggregate,
				Step:      tc.step,
			})
			if err != nil {
				t.Fatalf("Failed to get efficiency with step=%s: %v", tc.step, err)
			}

			compareEfficiencyResults(t, noStepResp, steppedResp, tc.step)
		})
	}
}

// TestMCPEfficiencyStepDoesNotOOM verifies that large-window queries with step
// complete successfully. This is the primary use case for the step parameter.
func TestMCPEfficiencyStepDoesNotOOM(t *testing.T) {
	now := time.Now().UTC()
	windowStart := now.AddDate(0, 0, -8)
	windowEnd := now.AddDate(0, 0, -1)

	testCases := []struct {
		name      string
		window    string
		aggregate string
		step      string
	}{
		{
			name:      "7d namespace with default step",
			window:    fmt.Sprintf("%s,%s", windowStart.Format("2006-01-02T15:04:05Z"), windowEnd.Format("2006-01-02T15:04:05Z")),
			aggregate: "namespace",
		},
		{
			name:      "7d container with 6h step",
			window:    fmt.Sprintf("%s,%s", windowStart.Format("2006-01-02T15:04:05Z"), windowEnd.Format("2006-01-02T15:04:05Z")),
			aggregate: "cluster,container",
			step:      "6h",
		},
		{
			name:      "7d container with 1d step",
			window:    fmt.Sprintf("%s,%s", windowStart.Format("2006-01-02T15:04:05Z"), windowEnd.Format("2006-01-02T15:04:05Z")),
			aggregate: "cluster,container",
			step:      "1d",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := callMCPEfficiencyTool(MCPEfficiencyRequest{
				Window:    tc.window,
				Aggregate: tc.aggregate,
				Step:      tc.step,
			})
			if err != nil {
				t.Fatalf("Efficiency query failed (possible OOM): %v", err)
			}

			if len(resp.Data.Efficiencies) == 0 {
				t.Error("Expected at least one efficiency result")
			}

			t.Logf("Successfully returned %d efficiency results for window=%s aggregate=%s step=%s",
				len(resp.Data.Efficiencies), tc.window, tc.aggregate, tc.step)
		})
	}
}

// TestMCPEfficiencyInvalidStep verifies that invalid step values return errors.
func TestMCPEfficiencyInvalidStep(t *testing.T) {
	testCases := []struct {
		name string
		step string
	}{
		{name: "Negative step", step: "-1h"},
		{name: "Zero step", step: "0s"},
		{name: "Invalid format", step: "notaduration"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := callMCPEfficiencyTool(MCPEfficiencyRequest{
				Window:    "yesterday",
				Aggregate: "namespace",
				Step:      tc.step,
			})
			if err == nil {
				t.Errorf("Expected error for step=%s, got nil", tc.step)
			} else {
				t.Logf("Correctly rejected step=%s: %v", tc.step, err)
			}
		})
	}
}

func compareEfficiencyResults(t *testing.T, baseline, stepped *MCPEfficiencyData, step string) {
	t.Helper()

	baseMap := make(map[string]int)
	for i, e := range baseline.Data.Efficiencies {
		baseMap[e.Name] = i
	}

	stepMap := make(map[string]int)
	for i, e := range stepped.Data.Efficiencies {
		stepMap[e.Name] = i
	}

	for name := range baseMap {
		if _, exists := stepMap[name]; !exists {
			t.Errorf("Workload '%s' exists in baseline but not in stepped (step=%s)", name, step)
		}
	}
	for name := range stepMap {
		if _, exists := baseMap[name]; !exists {
			t.Errorf("Workload '%s' exists in stepped (step=%s) but not in baseline", name, step)
		}
	}

	const costTolerance = 0.01
	const costPctThreshold = 0.1
	// Efficiency ratios can differ by a few percent when batched because
	// per-step weighted averages of usage/request are not identical to a
	// single-window computation. 5% relative with a 0.01 absolute floor
	// catches real bugs while accepting expected averaging differences.
	const efficiencyPctThreshold = 5.0
	const efficiencyAbsFloor = 0.01
	matchCount := 0

	for name, baseIdx := range baseMap {
		stepIdx, exists := stepMap[name]
		if !exists {
			continue
		}

		b := baseline.Data.Efficiencies[baseIdx]
		s := stepped.Data.Efficiencies[stepIdx]

		costDiff := abs(b.CurrentTotalCost - s.CurrentTotalCost)
		if costDiff > costTolerance {
			if b.CurrentTotalCost == 0 {
				t.Errorf("TotalCost mismatch for '%s': baseline=0, stepped=%.6f (diff=%.6f)",
					name, s.CurrentTotalCost, costDiff)
			} else if costPctDiff := (costDiff / abs(b.CurrentTotalCost)) * 100; costPctDiff > costPctThreshold {
				t.Errorf("TotalCost mismatch for '%s': baseline=%.6f, stepped=%.6f (diff=%.6f, %.4f%%)",
					name, b.CurrentTotalCost, s.CurrentTotalCost, costDiff, costPctDiff)
			}
		}

		checkEfficiency(t, name, "CPUEfficiency", b.CPUEfficiency, s.CPUEfficiency, efficiencyAbsFloor, efficiencyPctThreshold)
		checkEfficiency(t, name, "MemoryEfficiency", b.MemoryEfficiency, s.MemoryEfficiency, efficiencyAbsFloor, efficiencyPctThreshold)

		savingsDiff := abs(b.CostSavings - s.CostSavings)
		if savingsDiff > costTolerance {
			if b.CostSavings == 0 {
				t.Errorf("CostSavings mismatch for '%s': baseline=0, stepped=%.6f (diff=%.6f)",
					name, s.CostSavings, savingsDiff)
			} else if savingsPctDiff := (savingsDiff / abs(b.CostSavings)) * 100; savingsPctDiff > costPctThreshold {
				t.Errorf("CostSavings mismatch for '%s': baseline=%.6f, stepped=%.6f (diff=%.6f, %.4f%%)",
					name, b.CostSavings, s.CostSavings, savingsDiff, savingsPctDiff)
			}
		}

		matchCount++
	}

	t.Logf("Comparison complete: %d baseline, %d stepped, %d matched (step=%s)",
		len(baseMap), len(stepMap), matchCount, step)
}

func checkEfficiency(t *testing.T, workload, metric string, baseline, stepped, absFloor, pctThreshold float64) {
	t.Helper()
	diff := abs(baseline - stepped)
	if diff <= absFloor {
		return
	}
	if baseline == 0 {
		t.Errorf("%s mismatch for '%s': baseline=0, stepped=%.6f (diff=%.6f)",
			metric, workload, stepped, diff)
		return
	}
	pctDiff := (diff / abs(baseline)) * 100
	if pctDiff > pctThreshold {
		t.Errorf("%s mismatch for '%s': baseline=%.6f, stepped=%.6f (diff=%.6f, %.2f%%)",
			metric, workload, baseline, stepped, diff, pctDiff)
	}
}
