package validation

import (
	"fmt"
	"testing"
)

func TestEfficiencyMetricsValidity(t *testing.T) {

	allocResp, err := FetchAllocationData(true)
	if err != nil {
		t.Fatalf("Failed to fetch allocation data: %v", err)
	}

	var invalidEfficiencies []string

	const maxReasonableEfficiency = 5.0

	for _, allocSet := range allocResp.Data {

		for namespaceName, alloc := range allocSet {

			if alloc.CPUEfficiency < 0 || alloc.CPUEfficiency > maxReasonableEfficiency {
				invalidEfficiencies = append(invalidEfficiencies,
					fmt.Sprintf("%s: cpuEfficiency (%.2f) outside reasonable range [0, %.1f]",
						namespaceName, alloc.CPUEfficiency, maxReasonableEfficiency))
			}

			if alloc.RAMEfficiency < 0 || alloc.RAMEfficiency > maxReasonableEfficiency {
				invalidEfficiencies = append(invalidEfficiencies,
					fmt.Sprintf("%s: ramEfficiency (%.2f) outside reasonable range [0, %.1f]",
						namespaceName, alloc.RAMEfficiency, maxReasonableEfficiency))
			}

			if alloc.GPUEfficiency < 0 || alloc.GPUEfficiency > maxReasonableEfficiency {
				invalidEfficiencies = append(invalidEfficiencies,
					fmt.Sprintf("%s: gpuEfficiency (%.2f) outside reasonable range [0, %.1f]",
						namespaceName, alloc.GPUEfficiency, maxReasonableEfficiency))
			}

			if alloc.TotalEfficiency < 0 || alloc.TotalEfficiency > maxReasonableEfficiency {
				invalidEfficiencies = append(invalidEfficiencies,
					fmt.Sprintf("%s: totalEfficiency (%.2f) outside reasonable range [0, %.1f]",
						namespaceName, alloc.TotalEfficiency, maxReasonableEfficiency))
			}
		}
	}

	if len(invalidEfficiencies) > 0 {
		t.Errorf("Found %d efficiency metrics outside reasonable ranges: %v",
			len(invalidEfficiencies), invalidEfficiencies)
	}
}
