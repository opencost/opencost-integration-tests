package allocation

import (
	"fmt"
	"testing"
	"time"

	util "github.com/opencost/opencost-integration-tests/test/comparison/helper"

	"github.com/opencost/opencost-integration-tests/pkg/api"
)

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

			// Compare Times
			start1 := alloc1.Start
			start2 := alloc2.Start
			startDiff := start1.Sub(start2).Minutes()
			if startDiff > 1 {
				t.Errorf("")
			}
			end1 := alloc1.End
			end2 := alloc2.End
			endDiff := end1.Sub(end2).Minutes()
			if endDiff > 1 {
				t.Errorf("Difference Between End is greater than the resolution")
			}

			// Compare Properties

			// Compare CPU metrics
			util.CompareValues(t, fmt.Sprintf("%s CPU Core Hours", podName),
				alloc1.CPUCoreHours, alloc2.CPUCoreHours, tolerancePercent)
			util.CompareValues(t, fmt.Sprintf("%s CPU Core Request Average", podName),
				alloc1.CPUCoreRequestAverage, alloc2.CPUCoreRequestAverage, tolerancePercent)
			util.CompareValues(t, fmt.Sprintf("%s CPU Core Usage Average", podName),
				alloc1.CPUCoreUsageAverage, alloc2.CPUCoreUsageAverage, tolerancePercent)
			util.CompareValues(t, fmt.Sprintf("%s CPU Cost", podName),
				alloc1.CPUCost, alloc2.CPUCost, tolerancePercent)
			//
			//// Compare Memory metrics
			util.CompareValues(t, fmt.Sprintf("%s RAM Byte Hours", podName),
				alloc1.RAMByteHours, alloc2.RAMByteHours, tolerancePercent)
			util.CompareValues(t, fmt.Sprintf("%s RAM Bytes Request Average", podName),
				alloc1.RAMBytesRequestAverage, alloc2.RAMBytesRequestAverage, tolerancePercent)
			util.CompareValues(t, fmt.Sprintf("%s RAM Bytes Usage Average", podName),
				alloc1.RAMBytesUsageAverage, alloc2.RAMBytesUsageAverage, tolerancePercent)
			util.CompareValues(t, fmt.Sprintf("%s RAM Cost", podName),
				alloc1.RAMCost, alloc2.RAMCost, tolerancePercent)
			//
			// Compare GPU metrics if present
			if alloc1.GPUHours > 0 || alloc2.GPUHours > 0 {
				util.CompareValues(t, fmt.Sprintf("%s GPU Hours", podName),
					alloc1.GPUHours, alloc2.GPUHours, tolerancePercent)
				util.CompareValues(t, fmt.Sprintf("%s GPU Cost", podName),
					alloc1.GPUCost, alloc2.GPUCost, tolerancePercent)
			}

			// Compare Network metrics
			util.CompareValues(t, fmt.Sprintf("%s Network Transfer Bytes", podName),
				alloc1.NetworkTransferBytes, alloc2.NetworkTransferBytes, tolerancePercent)
			util.CompareValues(t, fmt.Sprintf("%s Network Receive Bytes", podName),
				alloc1.NetworkReceiveBytes, alloc2.NetworkReceiveBytes, tolerancePercent)
			util.CompareValues(t, fmt.Sprintf("%s Network Cost", podName),
				alloc1.NetworkCost, alloc2.NetworkCost, tolerancePercent)

			// Compare PV metrics
			util.CompareValues(t, fmt.Sprintf("%s PV Cost", podName),
				alloc1.PersistentVolumeCost(), alloc2.PersistentVolumeCost(), tolerancePercent)

			// Compare LB metrics
			util.CompareValues(t, fmt.Sprintf("%s LB Cost", podName),
				alloc1.LoadBalancerCost, alloc2.LoadBalancerCost, tolerancePercent)

			// Compare Total Cost
			util.CompareValues(t, fmt.Sprintf("%s Total Cost", podName),
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
	api2 := api.NewComparisonAPI()

	end := time.Now().UTC().Truncate(10 * time.Minute)
	start := end.Add(-10 * time.Minute)
	win := fmt.Sprintf("%s,%s", start.Format(time.RFC3339), end.Format(time.RFC3339))

	// Set up the allocation request with 10-minute window
	req := api.AllocationRequest{
		Window:     win,
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

	// Compare responses with 10% tolerance
	compareAllocationResponses(t, resp1, resp2, 10.0)
}
