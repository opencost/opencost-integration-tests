package validation

import (
	"fmt"
	"testing"
)

func TestIdleAllocationValidity(t *testing.T) {
	allocWithIdle, err := FetchAllocationData(true)
	if err != nil {
		t.Fatalf("Failed to fetch allocation data with idle: %v", err)
	}

	allocWithoutIdle, err := FetchAllocationData(false)
	if err != nil {
		t.Fatalf("Failed to fetch allocation data without idle: %v", err)
	}

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

	var issues []string

	var idleAlloc Allocation
	if len(allocWithIdle.Data) > 0 {
		for _, allocSet := range allocWithIdle.Data {
			if idle, exists := allocSet["__idle__"]; exists {
				idleAlloc = idle
				break
			}
		}
	}

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

	if idleAlloc.TotalCost < 0 {
		issues = append(issues, fmt.Sprintf("Idle total cost is negative: %.2f", idleAlloc.TotalCost))
	} else if totalAllCosts > 0 && idleAlloc.TotalCost > totalAllCosts {
		issues = append(issues, fmt.Sprintf("Idle total cost (%.2f) exceeds the sum of all other namespace costs (%.2f)",
			idleAlloc.TotalCost, totalAllCosts))
	}

	if idleAlloc.CPUCost < 0 {
		issues = append(issues, fmt.Sprintf("Idle CPU cost is negative: %.2f", idleAlloc.CPUCost))
	}
	if idleAlloc.RAMCost < 0 {
		issues = append(issues, fmt.Sprintf("Idle RAM cost is negative: %.2f", idleAlloc.RAMCost))
	}
	if idleAlloc.GPUCost < 0 {
		issues = append(issues, fmt.Sprintf("Idle GPU cost is negative: %.2f", idleAlloc.GPUCost))
	}

	if len(issues) > 0 {
		t.Errorf("Found %d issues with idle resource allocation: %v", len(issues), issues)
	}
}
