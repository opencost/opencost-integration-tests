// test/integration/query/validation/efficiency_metrics_test.go
package validation

import (
	"fmt"
	"testing"
)

// TestEfficiencyMetricsValidity checks that efficiency metrics are within valid ranges
func TestEfficiencyMetricsValidity(t *testing.T) {
	// Fetch allocation data with idle included
	allocResp, err := FetchAllocationData(true)
	if err != nil {
		t.Fatalf("Failed to fetch allocation data: %v", err)
	}

	// Check for efficiency metric validity
	var invalidEfficiencies []string

	// Define reasonable limits for efficiency values
	// Typically efficiency should be between 0 and 1, but values slightly
	// above 1 might be okay in some cases (like over-provisioning)
	const maxReasonableEfficiency = 5.0

	// Iterate through each allocation set in the data array
	for _, allocSet := range allocResp.Data {
		// Check each allocation entry
		for namespaceName, alloc := range allocSet {
			// Skip entries with zero resources (efficiency might not be applicable)

			// Check CPU efficiency
			if alloc.CPUEfficiency < 0 || alloc.CPUEfficiency > maxReasonableEfficiency {
				invalidEfficiencies = append(invalidEfficiencies,
					fmt.Sprintf("%s: cpuEfficiency (%.2f) outside reasonable range [0, %.1f]",
						namespaceName, alloc.CPUEfficiency, maxReasonableEfficiency))
			}

			// Check RAM efficiency
			if alloc.RAMEfficiency < 0 || alloc.RAMEfficiency > maxReasonableEfficiency {
				invalidEfficiencies = append(invalidEfficiencies,
					fmt.Sprintf("%s: ramEfficiency (%.2f) outside reasonable range [0, %.1f]",
						namespaceName, alloc.RAMEfficiency, maxReasonableEfficiency))
			}

			// Check GPU efficiency (if applicable)
			if alloc.GPUEfficiency < 0 || alloc.GPUEfficiency > maxReasonableEfficiency {
				invalidEfficiencies = append(invalidEfficiencies,
					fmt.Sprintf("%s: gpuEfficiency (%.2f) outside reasonable range [0, %.1f]",
						namespaceName, alloc.GPUEfficiency, maxReasonableEfficiency))
			}

			// Check total efficiency
			if alloc.TotalEfficiency < 0 || alloc.TotalEfficiency > maxReasonableEfficiency {
				invalidEfficiencies = append(invalidEfficiencies,
					fmt.Sprintf("%s: totalEfficiency (%.2f) outside reasonable range [0, %.1f]",
						namespaceName, alloc.TotalEfficiency, maxReasonableEfficiency))
			}
		}
	}

	// Fail the test if invalid efficiencies were found
	if len(invalidEfficiencies) > 0 {
		t.Errorf("Found %d efficiency metrics outside reasonable ranges: %v",
			len(invalidEfficiencies), invalidEfficiencies)
	}
}
