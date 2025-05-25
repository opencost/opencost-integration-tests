package allocation

import (
	"fmt"
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/api"
)

func validateNonNegativeIdleCosts(t *testing.T, aggregate string, window string) {

	client := api.NewAPI()
	req := api.AllocationRequest{
		Window:     window,
		Aggregate:  aggregate,
		Idle:       "true",
		Accumulate: "false",
	}

	resp, err := client.GetAllocation(req)
	if err != nil {
		t.Fatalf("Failed to query OpenCost allocation API with window %s: %v", window, err)
	}
	if resp.Code != 200 {
		t.Fatalf("OpenCost allocation API returned error for window %s: code=%d", window, resp.Code)
	}
	if len(resp.Data) == 0 {
		t.Fatalf("No allocation data returned for window %s", window)
	}

	foundIdle := false
	fmt.Println(resp.Data)

	for _, allocations := range resp.Data {
		if idle, exists := allocations["__idle__"]; exists {
			foundIdle = true

			// instead for putting which values to check inside code, seeing them in one place might be better
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
		t.Skipf("Idle Allocation not found for window %s, Verification skipped", window)
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
