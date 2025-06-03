package allocation

import (
	"math"
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/api"
)

func isTotalSumEqualTotalCost(entry api.AllocationResponseItem) bool {
	sum := 0.0
	sum += entry.CPUCost + entry.GPUCost + entry.RAMCost + entry.NetworkCost + entry.NetworkCrossZoneCost + entry.NetworkCrossRegionCost + entry.NetworkInternetCost + entry.LoadBalancerCost + entry.RAMCost + entry.SharedCost
	for _, persistentVolume := range entry.PersistentVolumes {
		sum += persistentVolume.Cost
	}
	return math.Round(sum) == math.Round(entry.TotalCost)
}

func validatesum(t *testing.T, aggregate string, window string) {

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

	for _, allocations := range resp.Data {
		for _, entry := range allocations {
			if isTotalSumEqualTotalCost(entry) == false {
				t.Errorf("TotalCost and sum of costs not matching for entry %s", entry.Name)
			}
		}
	}
}

func TestToalCostValidation(t *testing.T) {

	windows := []string{"10m", "1h", "12h", "1d", "2d", "7d", "30d"}
	aggregates := []string{"namespace", "cluster", "service", "pod", "node"}

	for _, window := range windows {
		for _, aggregate := range aggregates {
			name := "Window: " + window + " Aggregate: " + aggregate
			t.Run(name, func(t *testing.T) {
				validatesum(t, aggregate, window)
			})
		}
	}
}
