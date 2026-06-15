package schema

import (
	"fmt"
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/api"
)

// Description - Assert that /allocation responses retain their expected public
// JSON fields, nested objects, and important field types.
//
// Implementation Details
// - Raw JSON is inspected instead of typed API structs so removed or renamed
//   fields are detected.
// - Additional response fields are allowed to preserve compatible API extensions.
// - The test is skipped when the endpoint returns no allocation items.
//
// Passing Criteria
// - The endpoint returns a successful response.
// - Every allocation item contains the required fields.
// - Required window, properties, name, and totalCost values have expected types.

var allocationRequiredFields = []string{
	"name",
	"cpuCost",
	"ramCost",
	"gpuCost",
	"pvCost",
	"networkCost",
	"loadBalancerCost",
	"sharedCost",
	"externalCost",
	"totalCost",
	"minutes",
	"window",
	"properties",
}

func TestAllocationResponseSchemaStability(t *testing.T) {
	apiClient := api.NewAPI()

	resp := fetchRawEndpoint(t, apiClient, "/allocation", api.AutocompleteRequest{
		Window: "1h",
	})

	requireSuccessfulResponse(t, "/allocation response", resp)

	data := requireArray(t, "/allocation data", resp["data"])
	if len(data) == 0 {
		t.Skip("/allocation returned no data sets")
	}

	validatedItems := 0

	for setIndex, rawSet := range data {
		setContext := fmt.Sprintf("/allocation data[%d]", setIndex)
		set := requireMap(t, setContext, rawSet)

		for itemName, rawItem := range set {
			validatedItems++

			itemContext := fmt.Sprintf("/allocation item %q", itemName)
			item := requireMap(t, itemContext, rawItem)

			requireFields(t, itemContext, item, allocationRequiredFields)
			requireString(t, itemContext+" name", item["name"])
			requireNumber(t, itemContext+" totalCost", item["totalCost"])

			windowContext := itemContext + " window"
			window := requireMap(t, windowContext, item["window"])
			requireFields(t, windowContext, window, windowRequiredFields)
			requireString(t, windowContext+" start", window["start"])
			requireString(t, windowContext+" end", window["end"])

			requireMap(t, itemContext+" properties", item["properties"])
		}
	}

	if validatedItems == 0 {
		t.Skip("/allocation returned no allocation items")
	}
}