package allocation

import (
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/api"
)

func validateNonNegativeCosts(t *testing.T, aggregate string, window string) {

	client := api.NewAPI()
	req := api.AllocationRequest{
		Window:    window,
		Aggregate: aggregate,
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
	//fmt.Println(resp.Data)

	for _, allocations := range resp.Data {
		for _, entry := range allocations {

			costChecks := []struct {
				name  string
				value float64
			}{
				// Cost values we are checking if they are negative or not
				{"total cost", entry.TotalCost},
				{"CPU cost", entry.CPUCost},
				{"RAM cost", entry.RAMCost},
				{"GPU cost", entry.GPUCost},
			}

			for _, check := range costChecks {
				if check.value < 0 {
					t.Errorf("Found negative %s in %s allocation: %f", check.name, entry.Name, check.value)
				}
			}
		}
	}
}

func TestNonNegativeCostValidation(t *testing.T) {

	windows := []string{"10m", "1h", "12h", "1d", "2d", "7d", "30d"}
	aggregates := []string{"namespace", "cluster", "service", "pod", "node"}

	for _, window := range windows {
		for _, aggregate := range aggregates {
			name := "Window: " + window + " Aggregate: " + aggregate
			t.Run(name, func(t *testing.T) {
				validateNonNegativeCosts(t, aggregate, window)
			})
		}
	}
}
