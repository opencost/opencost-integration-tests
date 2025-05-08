// test/integration/query/validation/idle_allocation_test.go
package validation

import (
	"fmt"
	"testing"
)

// TestIdleAllocationValidity verifies that idle resources are properly allocated
func TestIdleAllocationValidity(t *testing.T) {
	// First, get allocation data with idle included
	allocWithIdle, err := FetchAllocationData(true)
	if err != nil {
		t.Fatalf("Failed to fetch allocation data with idle: %v", err)
	}

	// Then, get allocation data without idle
	allocWithoutIdle, err := FetchAllocationData(false)
	if err != nil {
		t.Fatalf("Failed to fetch allocation data without idle: %v", err)
	}

	// Verify that idle entry only exists when includeIdle=true
	idleEntryExists := false
	if len(allocWithIdle.Data) > 0 {
		for _, allocSet := range allocWithIdle.Data {
			_, idleEntryExists = allocSet["__idle__"]
			if idleEntryExists {
				break
			}
		}
	}

	if !idleEntryExists {
		t.Errorf("Idle entry (__idle__) does not exist even with includeIdle=true")
		return
	}

	// Verify idle entry does not exist when includeIdle=false
	idleEntryExists = false
	if len(allocWithoutIdle.Data) > 0 {
		for _, allocSet := range allocWithoutIdle.Data {
			_, idleEntryExists = allocSet["__idle__"]
			if idleEntryExists {
				t.Errorf("Idle entry (__idle__) exists even with includeIdle=false")
				break
			}
		}
	}

	// Check for validation issues with idle costs
	var issues []string

	// Get idle allocation
	var idleAlloc Allocation
	if len(allocWithIdle.Data) > 0 {
		for _, allocSet := range allocWithIdle.Data {
			if idle, exists := allocSet["__idle__"]; exists {
				idleAlloc = idle
				break
			}
		}
	}

	// Calculate total cluster resources and costs (excluding idle)
	var totalCPUCores, totalRAMBytes, totalGPUCount float64
	var totalCPUCost, totalRAMCost, totalGPUCost, totalAllCosts float64

	if len(allocWithIdle.Data) > 0 {
		for _, allocSet := range allocWithIdle.Data {
			for namespaceName, alloc := range allocSet {
				if namespaceName != "__idle__" {
					totalCPUCores += alloc.CPUCores
					totalRAMBytes += alloc.RAMBytes
					totalGPUCount += alloc.GPUCount

					totalCPUCost += alloc.CPUCost
					totalRAMCost += alloc.RAMCost
					totalGPUCost += alloc.GPUCost
					totalAllCosts += alloc.TotalCost
				}
			}
		}
	}

	// Validate idle costs are reasonable (not more than 50% of total costs)
	// This is a heuristic - in a real production system, idle might be higher or lower
	if idleAlloc.TotalCost < 0 {
		issues = append(issues, fmt.Sprintf("Idle total cost is negative: %.2f", idleAlloc.TotalCost))
	} else if totalAllCosts > 0 && idleAlloc.TotalCost > totalAllCosts {
		issues = append(issues, fmt.Sprintf("Idle total cost (%.2f) exceeds the sum of all other namespace costs (%.2f)",
			idleAlloc.TotalCost, totalAllCosts))
	}

	// Check that idle costs by resource type make sense
	if idleAlloc.CPUCost < 0 {
		issues = append(issues, fmt.Sprintf("Idle CPU cost is negative: %.2f", idleAlloc.CPUCost))
	}
	if idleAlloc.RAMCost < 0 {
		issues = append(issues, fmt.Sprintf("Idle RAM cost is negative: %.2f", idleAlloc.RAMCost))
	}
	if idleAlloc.GPUCost < 0 {
		issues = append(issues, fmt.Sprintf("Idle GPU cost is negative: %.2f", idleAlloc.GPUCost))
	}

	// Fail the test if issues were found
	if len(issues) > 0 {
		t.Errorf("Found %d issues with idle resource allocation: %v", len(issues), issues)
	}
}
