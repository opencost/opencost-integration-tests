package assert

// Description - Asserts Cost values from /allocation for each pod match cost values from /allocation/summary the same pod

import (
	"fmt"
	"testing"
	"reflect"
	"math"

	"github.com/opencost/opencost-integration-tests/pkg/api"
)

func areCostsWithinTolerance(cost1, cost2 float64) bool {
	tolerance := .05

	if cost1 == cost2 { //handles null case
		return true
	}

	

	// variancePercentage := math.Min(cost1, cost2) / math.Max(cost1, cost2)
	diff := math.Abs(cost1 - cost2)
	variancePercentage := diff / math.Max(cost1, cost2)
	return variancePercentage < tolerance
}


func assertNamespaceInformation(apiSummary api.AllocationResponseItem, apiCompute api.AllocationResponseItem) (bool, []string) {
	status := true
	failedMetrics := []string{}

	// List of float64 fields to compare using areCostsWithinTolerance
	floatFields := []string{
		"CPUCoreRequestAverage",
		"CPUCoreUsageAverage",
		"CPUCost",
		"CPUCostIdle",
		"GPUCost",
		"GPUCostIdle",
		"NetworkCost",
		"LoadBalancerCost",
		"RAMBytesRequestAverage",
		"RAMBytesUsageAverage",
		"RAMCost",
		"RAMCostIdle",
		"SharedCost",
	}
	// Use reflection to loop through float64 fields
	summaryValReflect := reflect.ValueOf(apiSummary)
	computeValReflect := reflect.ValueOf(apiCompute)

	for _, fieldName := range floatFields {
		summaryField := summaryValReflect.FieldByName(fieldName)
		computeField := computeValReflect.FieldByName(fieldName)

		if !summaryField.IsValid() || !computeField.IsValid() {
			failedMetrics = append(failedMetrics, fmt.Sprintf("Internal Error: Missing or invalid field %s for comparison\n", fieldName))
			status = false
		}
		// Check if the field is actually a float64
		if summaryField.Kind() != reflect.Float64 || computeField.Kind() != reflect.Float64 {
			failedMetrics = append(failedMetrics, fmt.Sprintf("Internal Error: Field %s is not a float64 type\n", fieldName))
			status = false
		}

		val1 := summaryField.Float()
		val2 := computeField.Float()

		if !areCostsWithinTolerance(val1, val2) {
			failedMetrics = append(failedMetrics, fmt.Sprintf("Value Mismatch Error: /allocation cost value %f does not match allocation/summary cost value %f for field %s\n", val1, val2, fieldName))
			status = false
		}
	}

	return status, failedMetrics
}
func TestAllocationAndAllocationSummary(t *testing.T) {
	apiObj := api.NewAPI()

	testCases := []struct {
		name		string
		window      string
		aggregate   string
		accumulate  string
	}{
		{
			name: "Yesterday",
			window: "1d",
			aggregate: "namespace",
			accumulate: "false",
		},
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			// API Client
			apiResponse, err := apiObj.GetAllocation(api.AllocationRequest{
				Window: tc.window,
				Aggregate: tc.aggregate,
			})

			if err != nil {
				t.Fatalf("Error while calling Allocation API %v", err)
			}
			if apiResponse.Code != 200 {
				t.Errorf("API returned non-200 code")
			}

			apiSummaryResponse, err := apiObj.GetAllocationSummary(api.AllocationRequest{
				Window: tc.window,
				Aggregate: tc.aggregate, // Summary API is not working as expecte
			})

			if err != nil {
				t.Fatalf("Error while calling Allocation API %v", err)
			}
			if apiSummaryResponse.Code != 200 {
				t.Errorf("API returned non-200 code")
			}

			// Assumes /allocation will not omit any namespace, should be the other way around as well
			for namespace, allocationResponseItem := range apiResponse.Data[0] {
				allocationSummaryResponseItem, namespacePresent := apiSummaryResponse.Data.Sets[0].Allocations[namespace]
				if !namespacePresent {
					t.Errorf("Namespace %s not present in allocation/summary", namespace)
					continue
				} 
				comparisonStatus, failedMetrics := assertNamespaceInformation(allocationResponseItem, allocationSummaryResponseItem)
				if comparisonStatus != true {
					t.Errorf("Cost Information for resources don't match for namespace %s:\n %v", namespace, failedMetrics)
				}
				if comparisonStatus == true {
					t.Logf("Cost Information for resources match for namespace %s", namespace)
				} else {

				}
			}
		})
	}
}
