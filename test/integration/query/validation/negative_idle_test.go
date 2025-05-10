package validation

import (
	"fmt"
	"testing"
)

func TestNegativeIdleCosts(t *testing.T) {
	allocResp, err := FetchAllocationData(true)
	if err != nil {
		t.Fatalf("Failed to fetch allocation data: %v", err)
	}

	var foundNegativeValues bool
	var negativeFields []string

	for _, allocSet := range allocResp.Data {
		for namespaceName, alloc := range allocSet {
			if alloc.CPUCost < 0 {
				negativeFields = append(negativeFields, fmt.Sprintf("%s.cpuCost = %.5f", namespaceName, alloc.CPUCost))
				foundNegativeValues = true
			}
			if alloc.RAMCost < 0 {
				negativeFields = append(negativeFields, fmt.Sprintf("%s.ramCost = %.5f", namespaceName, alloc.RAMCost))
				foundNegativeValues = true
			}
			if alloc.GPUCost < 0 {
				negativeFields = append(negativeFields, fmt.Sprintf("%s.gpuCost = %.5f", namespaceName, alloc.GPUCost))
				foundNegativeValues = true
			}
			if alloc.TotalCost < 0 {
				negativeFields = append(negativeFields, fmt.Sprintf("%s.totalCost = %.5f", namespaceName, alloc.TotalCost))
				foundNegativeValues = true
			}
		}
	}

	if foundNegativeValues {
		t.Errorf("Found negative cost values in the API response: %v", negativeFields)
	}
}
