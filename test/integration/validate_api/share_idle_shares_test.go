package validate_api

// Description
// Validate if when idle costs are spread across resources, the net sum of the split to all resources is equal to the idle cost for that resource when in a separate namespace __idle__
// This is a Proof of Concept Test. So I decided to test this only for 3 resources - RAM, CPU, GPU. This can easily be extended to other resources
// The test does not take into account the proportion of split to each namespace. That should another test altogether

// Test Pass Criteria
// If the rounded value upto 2 decimal points of Idle cost for a resoruce and calculated Idle Costs match.


import (
	"testing"
	"math"
	"github.com/opencost/opencost-integration-tests/pkg/api"
)


func RoundUpToTwoDecimals(num float64) float64 {

	temp := num * 100
	roundedTemp := math.Round(temp)
	return roundedTemp / 100
}


func TestIdleSharingWorkflow(t *testing.T) {
	apiObj := api.NewAPI()

	// Sharing Idle values across other namespaces
	shared_idle_response, err := apiObj.GetAllocation(api.AllocationRequest{
		Window: "today",
		Aggregate: "namespace",
		Accumulate: "false",
		ShareIdle: "true",
	})

	if err != nil {
		t.Fatalf("Error while calling Allocation API %v", err)
	}

	if shared_idle_response.Code != 200 {
		t.Errorf("API returned non-200 code")
	}

	// Retrieve Idle values in __idle__ namespace
	separate_idle_response, err := apiObj.GetAllocation(api.AllocationRequest{
		Window: "today",
		Aggregate: "namespace",
		Accumulate: "false",
		IncludeIdle: "true",
	})

	if err != nil {
		t.Fatalf("Error while calling Allocation API %v", err)
	}

	if separate_idle_response.Code != 200 {
		t.Errorf("API returned non-200 code")
	}

	// Sum of all CPU Usage Costs without idle costs
	shared_idle_all_cpu_sum := 0.0
	shared_idle_all_gpu_sum := 0.0
	shared_idle_all_ram_sum := 0.0

	// Proof Of Concept to only check it on CPU, GPU and RAM Resource
	for _, allocationRequestObjMap := range shared_idle_response.Data {
		for mapkey, allocationRequestObj := range allocationRequestObjMap {
			t.Logf("Namespace: %v\n", mapkey)
			shared_idle_all_cpu_sum += allocationRequestObj.CPUCost
			shared_idle_all_gpu_sum += allocationRequestObj.GPUCost
			shared_idle_all_ram_sum += allocationRequestObj.RAMCost
	}

	// Sum of all CPU Usage Costs with idle costs
	separate_idle_all_cpu_sum := 0.0
	separate_idle_all_gpu_sum := 0.0
	separate_idle_all_ram_sum := 0.0

	for _, allocationRequestObjMap := range separate_idle_response.Data {
		for mapkey, allocationRequestObj := range allocationRequestObjMap {
			t.Logf("Namespace: %v\n", mapkey)
			separate_idle_all_cpu_sum += allocationRequestObj.CPUCost
			separate_idle_all_gpu_sum += allocationRequestObj.GPUCost
			separate_idle_all_ram_sum += allocationRequestObj.RAMCost
		}
	}

	calculated_idle_costs_cpu :=  RoundUpToTwoDecimals(separate_idle_all_cpu_sum - shared_idle_all_cpu_sum)
	calculated_idle_costs_gpu :=  RoundUpToTwoDecimals(separate_idle_all_gpu_sum - shared_idle_all_gpu_sum)
	calculated_idle_costs_ram :=  RoundUpToTwoDecimals(separate_idle_all_ram_sum - shared_idle_all_ram_sum)

	if calculated_idle_costs_cpu  ==  RoundUpToTwoDecimals(separate_idle_response.Data[0]["__idle__"].CPUCost) {
		t.Logf("Idle Values are Completely distributed amoung other namespace CPU resources")
	} else {
		t.Errorf("Sum of Idle values distributed %v do not match original idle_value %v for CPU", calculated_idle_costs_cpu, RoundUpToTwoDecimals(separate_idle_response.Data[0]["__idle__"].CPUCost))
	}
	if calculated_idle_costs_gpu  ==  RoundUpToTwoDecimals(separate_idle_response.Data[0]["__idle__"].GPUCost) {
		t.Logf("Idle Values are Completely distributed amoung other namespace GPU resources")
	} else {
		t.Errorf("Sum of Idle values distributed %v do not match original idle_value %v for GPU", calculated_idle_costs_gpu, RoundUpToTwoDecimals(separate_idle_all_gpu_sum - shared_idle_all_gpu_sum))
	}
	if calculated_idle_costs_ram  ==  RoundUpToTwoDecimals(separate_idle_response.Data[0]["__idle__"].RAMCost) {
		t.Logf("Idle Values are Completely distributed amoung other namespace RAM resources")
	} else {
		t.Errorf("Sum of Idle values distributed %v do not match original idle_value %v for RAM", calculated_idle_costs_ram, RoundUpToTwoDecimals(separate_idle_all_ram_sum - shared_idle_all_ram_sum))
	}

}
}
