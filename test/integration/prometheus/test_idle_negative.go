package main

import (
	"fmt"
	"encoding/json"
	"github.com/opencost/opencost-integration-tests/pkg/api"
)

func printAsJSON(data interface{}) {
	jsonBytes, err := json.MarshalIndent(data, "", "  ") // Use MarshalIndent for pretty-printing
	if err != nil {
		fmt.Println("Error marshalling to JSON:", err)
		return
	}
	fmt.Println(string(jsonBytes))
}

func checkNegativeCosts(m api.AllocationResponseItem) {
	isNegative := false // Flag to track if any negative value is found
	var negativeFields []string // To store names of negative fields

	// Check each field individually
	if m.CPUCost < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "CPUCost")
	}
	if m.GPUCost < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "GPUCost")
	}
	if m.NetworkCost < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "NetworkCost")
	}
	if m.LoadBalancerCost < 0 {
		isNegative = true
		negativeFields = append(negativeFields, "LoadBalancerCost")
	}

	if isNegative {
		fmt.Println("  WARNING: One or more cost values are negative!")
		fmt.Printf("  Negative fields: %v\n", negativeFields)
	} else {
		fmt.Println("  All cost values are non-negative.")
	}
}

func main() {
	apiObj := api.NewAPI()
	// API Documentation - https://opencost.io/docs/integrations/api/
	response, err := apiObj.GetAllocation(api.AllocationRequest{
		Window: "1d",
		Aggregate: "namespace",
		Accumulate: "false",
		// ShareIdle: "true",
		IncludeIdle: "true",
	})
	fmt.Printf("Errors in Route %v", err)
	for i, allocationRequestObjMap := range response.Data {
		fmt.Printf("Data Entry %d (map of time ranges to items):\n", i+1)
		checkNegativeCosts(allocationRequestObjMap["__idle__"])
	}

}