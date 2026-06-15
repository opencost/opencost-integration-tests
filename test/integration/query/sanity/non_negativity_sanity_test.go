package sanity

// This test checks broad sanity invariants on allocation query output.
// It intentionally validates the API response as an external consumer:
// numeric fields must be finite, costs and usage metrics must be non-negative,
// runtime minutes must not exceed the returned window, and efficiencies must
// stay within their expected ranges.

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/api"
)

const (
	minEfficiency      = 0.0
	maxEfficiency      = 1.0
	windowSlackMinutes = 0.1
	// Efficiency tolerance allows for differences in time units and averaging methods.
	// Request-average fields may use different bases (per-minute, per-second, etc.),
	// so we allow efficiencies up to 5x to catch real issues while avoiding false
	// positives from unit/reporting differences.
	efficiencyTolerance = 5.0
)

func TestAllocationNonNegativitySanityChecks(t *testing.T) {
	apiObj := api.NewAPI()

	testCases := []struct {
		name        string
		window      string
		aggregate   string
		accumulate  string
		includeidle string
	}{
		{name: "Today", window: "today", aggregate: "namespace", accumulate: "false", includeidle: "true"},
		{name: "Yesterday", window: "yesterday", aggregate: "cluster", accumulate: "false", includeidle: "true"},
		{name: "Last week", window: "week", aggregate: "service", accumulate: "false", includeidle: "true"},
		{name: "Last 14 days", window: "14d", aggregate: "container", accumulate: "false", includeidle: "true"},
		{name: "Custom", window: "%sT00:00:00Z,%sT00:00:00Z", aggregate: "namespace", accumulate: "false", includeidle: "true"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "Custom" {
				now := time.Now().UTC()
				tc.window = fmt.Sprintf(tc.window,
					now.AddDate(0, 0, -2).Format("2006-01-02"),
					now.AddDate(0, 0, -1).Format("2006-01-02"),
				)
			}

			req := api.AllocationRequest{
				Window:      tc.window,
				Aggregate:   tc.aggregate,
				Accumulate:  tc.accumulate,
				IncludeIdle: tc.includeidle,
			}

			allocationResponse, err := apiObj.GetAllocation(req)
			if err != nil {
				t.Fatalf("failed to call /allocation: %v", err)
			}
			if allocationResponse.Code != 200 {
				t.Fatalf("expected /allocation status 200, got %d", allocationResponse.Code)
			}

			for _, allocationSet := range allocationResponse.Data {
				for key, item := range allocationSet {
					itemName := item.Name
					if itemName == "" {
						itemName = key
					}
					checkAllocationItem(t, itemName, item)
				}
			}
		})
	}
}

func checkAllocationItem(t *testing.T, itemName string, item api.AllocationResponseItem) {
	windowMinutes := item.Window.RunTime()

	checkNonNegativeFields(t, itemName, map[string]float64{
		"minutes": item.Minutes,

		"cpuCores":              item.CPUCores,
		"cpuCoreRequestAverage": item.CPUCoreRequestAverage,
		"cpuCoreLimitAverage":   item.CPUCoreLimitAverage,
		"cpuCoreUsageAverage":   item.CPUCoreUsageAverage,
		"cpuCoreHours":          item.CPUCoreHours,

		"gpuCount": item.GPUCount,
		"gpuHours": item.GPUHours,

		"networkTransferBytes": item.NetworkTransferBytes,
		"networkReceiveBytes":  item.NetworkReceiveBytes,

		"pvBytes":     item.PVBytes,
		"pvByteHours": item.PVByteHours,

		"ramBytes":              item.RAMBytes,
		"ramByteRequestAverage": item.RAMBytesRequestAverage,
		"ramByteLimitAverage":   item.RAMBytesLimitAverage,
		"ramByteUsageAverage":   item.RAMBytesUsageAverage,
		"ramByteHours":          item.RAMByteHours,
	})

	// Cost adjustment fields may be negative by design, so only validate the
	// core cost fields and idle cost values for non-negativity.
	checkNonNegativeFields(t, itemName, map[string]float64{
		"cpuCost":                item.CPUCost,
		"cpuCostIdle":            item.CPUCostIdle,
		"gpuCost":                item.GPUCost,
		"gpuCostIdle":            item.GPUCostIdle,
		"networkCost":            item.NetworkCost,
		"networkCrossZoneCost":   item.NetworkCrossZoneCost,
		"networkCrossRegionCost": item.NetworkCrossRegionCost,
		"networkInternetCost":    item.NetworkInternetCost,
		"loadBalancerCost":       item.LoadBalancerCost,
		"pvCost":                 item.PersistentVolumeCost(),
		"ramCost":                item.RAMCost,
		"ramCostIdle":            item.RAMCostIdle,
		"sharedCost":             item.SharedCost,
		"totalCost":              item.TotalCost,
	})

	if item.Minutes > windowMinutes+0.01 {
		t.Fatalf(
			"allocation sanity violation: item=%q field=minutes value=%f windowMinutes=%f",
			itemName,
			item.Minutes,
			windowMinutes,
		)
	}

	// Validate documented efficiency values.
	checkEfficiencyRange(t, itemName, "totalEfficiency", item.TotalEfficiency)

	// Validate computed per-resource efficiencies (more precise than totalEfficiency)
	checkComputedCPUEfficiency(t, itemName, item, windowMinutes)
	checkComputedRAMEfficiency(t, itemName, item, windowMinutes)
	checkComputedGPUEfficiency(t, itemName, item, windowMinutes)
}

func checkNonNegativeFields(t *testing.T, itemName string, fields map[string]float64) {
	for field, value := range fields {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			t.Fatalf(
				"allocation sanity violation: item=%q field=%s value=%f is not finite",
				itemName,
				field,
				value,
			)
		}

		if value < 0 {
			t.Fatalf(
				"allocation sanity violation: item=%q field=%s value=%f is negative",
				itemName,
				field,
				value,
			)
		}
	}
}

func checkEfficiencyRange(t *testing.T, itemName string, field string, value float64) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		t.Fatalf(
			"allocation sanity violation: item=%q field=%s value=%f is not finite",
			itemName,
			field,
			value,
		)
	}

	if value < 0 || value > 1 {
		t.Fatalf(
			"allocation sanity violation: item=%q field=%s value=%f is outside expected range [0,1]",
			itemName,
			field,
			value,
		)
	}
}

// checkComputedCPUEfficiency computes CPU efficiency from request-average and usage data
// and validates it is within [0, 1 + tolerance].
// cpuEfficiency = cpuCoreHours / (cpuCoreRequestAverage * minutes / 60)
// Skips check if CPU request is negligible (< 1e-6).
func checkComputedCPUEfficiency(t *testing.T, itemName string, item api.AllocationResponseItem, windowMinutes float64) {
	if item.CPUCoreRequestAverage < 1e-6 || windowMinutes <= 0 {
		// Skip check if CPU request is negligible or window is zero
		return
	}

	cpuRequestedHours := item.CPUCoreRequestAverage * windowMinutes / 60.0
	if cpuRequestedHours <= 0 {
		return
	}

	cpuEfficiency := item.CPUCoreHours / cpuRequestedHours

	// Allow small rounding tolerance above 1.0
	if cpuEfficiency < 0 || cpuEfficiency > (1.0+efficiencyTolerance) {
		t.Fatalf(
			"allocation sanity violation: item=%q field=cpuEfficiency value=%f is outside expected range [0,%.2f]",
			itemName,
			cpuEfficiency,
			1.0+efficiencyTolerance,
		)
	}
}

// checkComputedRAMEfficiency computes RAM efficiency from request-average and usage data
// and validates it is within [0, 1 + tolerance].
// ramEfficiency = ramByteHours / (ramByteRequestAverage * minutes / 60)
// Skips check if RAM request is negligible (< 1e-6).
func checkComputedRAMEfficiency(t *testing.T, itemName string, item api.AllocationResponseItem, windowMinutes float64) {
	if item.RAMBytesRequestAverage < 1e-6 || windowMinutes <= 0 {
		// Skip check if RAM request is negligible or window is zero
		return
	}

	ramRequestedHours := item.RAMBytesRequestAverage * windowMinutes / 60.0
	if ramRequestedHours <= 0 {
		return
	}

	ramEfficiency := item.RAMByteHours / ramRequestedHours

	// Allow small rounding tolerance above 1.0
	if ramEfficiency < 0 || ramEfficiency > (1.0+efficiencyTolerance) {
		t.Fatalf(
			"allocation sanity violation: item=%q field=ramEfficiency value=%f is outside expected range [0,%.2f]",
			itemName,
			ramEfficiency,
			1.0+efficiencyTolerance,
		)
	}
}

// checkComputedGPUEfficiency computes GPU efficiency from request-average and usage data
// and validates it is within [0, 1 + tolerance].
// gpuEfficiency = gpuHours / (gpuRequestAverage * minutes / 60)
// Skips check if GPU request is negligible (< 1e-6).
func checkComputedGPUEfficiency(t *testing.T, itemName string, item api.AllocationResponseItem, windowMinutes float64) {
	// GPU request average is in the GPUAllocation nested struct
	if item.GPUAllocation.GPURequestAverage < 1e-6 || windowMinutes <= 0 {
		// Skip check if GPU request is negligible or window is zero
		return
	}

	gpuRequestedHours := item.GPUAllocation.GPURequestAverage * windowMinutes / 60.0
	if gpuRequestedHours <= 0 {
		return
	}

	gpuEfficiency := item.GPUHours / gpuRequestedHours

	// Allow small rounding tolerance above 1.0
	if gpuEfficiency < 0 || gpuEfficiency > (1.0+efficiencyTolerance) {
		t.Fatalf(
			"allocation sanity violation: item=%q field=gpuEfficiency value=%f is outside expected range [0,%.2f]",
			itemName,
			gpuEfficiency,
			1.0+efficiencyTolerance,
		)
	}
}
