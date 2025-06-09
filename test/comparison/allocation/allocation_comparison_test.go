package allocation

import (
	"fmt"
	"math"
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/api"
)

// compareValues compares two float64 values with a percentage tolerance
func compareValues(t *testing.T, fieldName string, value1, value2 float64, tolerancePercent float64) bool {
	if value1 == 0 && value2 == 0 {
		return true
	}

	// Calculate the percentage difference
	diff := (value1 / value2) - 1
	percentDiff := math.Abs(diff) * 100

	if percentDiff > tolerancePercent {
		t.Errorf("%s values differ by %.2f%% (value1: %.2f, value2: %.2f)",
			fieldName, percentDiff, value1, value2)
		return false
	}
	return true
}

// compareAllocationResponses compares two allocation responses with percentage tolerance
func compareAllocationResponses(t *testing.T, resp1, resp2 *api.AllocationResponse, tolerancePercent float64) {
	if resp1 == nil || resp2 == nil {
		t.Fatalf("One or both responses are nil")
	}

	// Check for non-zero data lengths
	if len(resp1.Data) == 0 {
		t.Fatalf("First response has no data")
	}
	if len(resp2.Data) == 0 {
		t.Fatalf("Second response has no data")
	}

	// Log the number of entries in each response
	t.Logf("First response has %d entries", len(resp1.Data))
	t.Logf("Second response has %d entries", len(resp2.Data))

	if len(resp1.Data) != len(resp2.Data) {
		t.Errorf("Different number of entries in responses: resp1=%d, resp2=%d",
			len(resp1.Data), len(resp2.Data))
		return
	}

	// Track number of pods compared
	podsCompared := 0

	// Compare each allocation item
	for _, allocMap1 := range resp1.Data {
		for podName, alloc1 := range allocMap1 {
			// Find matching allocation in second response
			var alloc2 api.AllocationResponseItem
			found := false
			for _, allocMap2 := range resp2.Data {
				if a2, exists := allocMap2[podName]; exists {
					alloc2 = a2
					found = true
					break
				}
			}

			if !found {
				t.Errorf("Pod %s exists in first response but not in second", podName)
				continue
			}

			// Compare CPU metrics
			compareValues(t, fmt.Sprintf("%s CPU Core Hours", podName),
				alloc1.CPUCoreHours, alloc2.CPUCoreHours, tolerancePercent)
			compareValues(t, fmt.Sprintf("%s CPU Core Request Average", podName),
				alloc1.CPUCoreRequestAverage, alloc2.CPUCoreRequestAverage, tolerancePercent)
			compareValues(t, fmt.Sprintf("%s CPU Core Usage Average", podName),
				alloc1.CPUCoreUsageAverage, alloc2.CPUCoreUsageAverage, tolerancePercent)
			compareValues(t, fmt.Sprintf("%s CPU Cost", podName),
				alloc1.CPUCost, alloc2.CPUCost, tolerancePercent)

			// Compare Memory metrics
			compareValues(t, fmt.Sprintf("%s RAM Byte Hours", podName),
				alloc1.RAMByteHours, alloc2.RAMByteHours, tolerancePercent)
			compareValues(t, fmt.Sprintf("%s RAM Bytes Request Average", podName),
				alloc1.RAMBytesRequestAverage, alloc2.RAMBytesRequestAverage, tolerancePercent)
			compareValues(t, fmt.Sprintf("%s RAM Bytes Usage Average", podName),
				alloc1.RAMBytesUsageAverage, alloc2.RAMBytesUsageAverage, tolerancePercent)
			compareValues(t, fmt.Sprintf("%s RAM Cost", podName),
				alloc1.RAMCost, alloc2.RAMCost, tolerancePercent)

			// Compare GPU metrics if present
			if alloc1.GPUHours > 0 || alloc2.GPUHours > 0 {
				compareValues(t, fmt.Sprintf("%s GPU Hours", podName),
					alloc1.GPUHours, alloc2.GPUHours, tolerancePercent)
				compareValues(t, fmt.Sprintf("%s GPU Cost", podName),
					alloc1.GPUCost, alloc2.GPUCost, tolerancePercent)
			}

			// Compare Network metrics
			compareValues(t, fmt.Sprintf("%s Network Transfer Bytes", podName),
				alloc1.NetworkTransferBytes, alloc2.NetworkTransferBytes, tolerancePercent)
			compareValues(t, fmt.Sprintf("%s Network Receive Bytes", podName),
				alloc1.NetworkReceiveBytes, alloc2.NetworkReceiveBytes, tolerancePercent)
			compareValues(t, fmt.Sprintf("%s Network Cost", podName),
				alloc1.NetworkCost, alloc2.NetworkCost, tolerancePercent)

			// Compare PV metrics
			compareValues(t, fmt.Sprintf("%s PV Cost", podName),
				alloc1.PersistentVolumeCost(), alloc2.PersistentVolumeCost(), tolerancePercent)

			// Compare Total Cost
			compareValues(t, fmt.Sprintf("%s Total Cost", podName),
				alloc1.TotalCost, alloc2.TotalCost, tolerancePercent)

			podsCompared++
		}
	}

	// Verify minimum number of pods were compared
	if podsCompared < 3 {
		t.Errorf("Not enough pods were compared. Expected at least 3, got %d", podsCompared)
	}
	t.Logf("Successfully compared %d pods", podsCompared)
}

func TestAllocationAPIComparison(t *testing.T) {
	// Initialize both API clients
	api1 := api.NewAPI()
	api2 := api.NewAPI()

	// Set up the allocation request with 10-minute window
	req := api.AllocationRequest{
		Window:     "10m",
		Aggregate:  "pod",
		Accumulate: "true",
	}

	// Get responses from both APIs
	resp1, err := api1.GetAllocation(req)
	if err != nil {
		t.Fatalf("Failed to get allocation from first API: %v", err)
	}

	resp2, err := api2.GetAllocation(req)
	if err != nil {
		t.Fatalf("Failed to get allocation from second API: %v", err)
	}

	// Compare responses with 5% tolerance
	compareAllocationResponses(t, resp1, resp2, 5.0)
}
