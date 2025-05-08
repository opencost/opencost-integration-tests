// test/integration/query/validation/negative_idle_test.go
package validation

import (
	"fmt"
	"testing"
)

// TestNegativeIdleCosts checks for negative values in cost-related fields
func TestNegativeIdleCosts(t *testing.T) {
	// Fetch allocation data with idle included
	allocResp, err := FetchAllocationData(true)
	if err != nil {
		t.Fatalf("Failed to fetch allocation data: %v", err)
	}

	// Check for negative values in the response
	var foundNegativeValues bool
	var negativeFields []string

	// Iterate through each allocation set in the data array
	for _, allocSet := range allocResp.Data {
		// Check each allocation entry
		for namespaceName, alloc := range allocSet {
			// Check for negative cost values
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

	// Fail the test if negative values were found
	if foundNegativeValues {
		t.Errorf("Found negative cost values in the API response: %v", negativeFields)
	}
}
