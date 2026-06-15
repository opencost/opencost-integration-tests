package schema

import (
	"fmt"
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/api"
)

// Description - Assert that /cloudCost responses retain their expected public
// JSON fields, nested objects, and important field types.
//
// Implementation Details
// - Each CloudCost item, properties object, window, and nested cost object is
//   validated using raw JSON.
// - Empty sets are ignored while other returned sets continue to be validated.
// - The test is skipped when no CloudCost items are returned.
//
// Passing Criteria
// - The endpoint returns a successful response.
// - Every CloudCost item contains the required fields and properties.
// - Each supported nested cost object contains numeric cost and
//   kubernetesPercent fields.

var cloudCostPropertiesRequiredFields = []string{
	"provider",
	"accountID",
	"accountName",
	"invoiceEntityID",
	"invoiceEntityName",
	"service",
	"category",
}

var cloudCostCostObjectRequiredFields = []string{
	"cost",
	"kubernetesPercent",
}

var cloudCostFields = []string{
	"netCost",
	"amortizedCost",
	"amortizedNetCost",
	"invoicedCost",
	"listCost",
}

var cloudCostItemRequiredFields = append(
	[]string{
		"properties",
		"window",
	},
	cloudCostFields...,
)

func TestCloudCostResponseSchemaStability(t *testing.T) {
	apiClient := api.NewAPI()

	resp := fetchRawEndpoint(t, apiClient, "/cloudCost", api.AutocompleteRequest{
		Window: "1d",
	})

	requireSuccessfulResponse(t, "/cloudCost response", resp)

	data := requireMap(t, "/cloudCost data", resp["data"])
	requireFields(t, "/cloudCost data", data, []string{"sets"})

	sets := requireArray(t, "/cloudCost data.sets", data["sets"])
	if len(sets) == 0 {
		t.Skip("/cloudCost returned no sets")
	}

	validatedItems := 0

	for setIndex, rawSet := range sets {
		setContext := fmt.Sprintf("/cloudCost set[%d]", setIndex)
		set := requireMap(t, setContext, rawSet)

		requireFields(t, setContext, set, []string{"cloudCosts"})

		cloudCostsContext := setContext + ".cloudCosts"
		cloudCosts := requireMap(t, cloudCostsContext, set["cloudCosts"])

		for itemName, rawItem := range cloudCosts {
			validatedItems++

			itemContext := fmt.Sprintf("/cloudCost item %q", itemName)
			item := requireMap(t, itemContext, rawItem)

			requireFields(t, itemContext, item, cloudCostItemRequiredFields)

			windowContext := itemContext + " window"
			window := requireMap(t, windowContext, item["window"])
			requireFields(t, windowContext, window, windowRequiredFields)
			requireString(t, windowContext+" start", window["start"])
			requireString(t, windowContext+" end", window["end"])

			propertiesContext := itemContext + " properties"
			properties := requireMap(t, propertiesContext, item["properties"])
			requireFields(t, propertiesContext, properties, cloudCostPropertiesRequiredFields)

			for _, costField := range cloudCostFields {
				costContext := itemContext + " " + costField
				costObject := requireMap(t, costContext, item[costField])

				requireFields(t, costContext, costObject, cloudCostCostObjectRequiredFields)
				requireNumber(t, costContext+" cost", costObject["cost"])
				requireNumber(t, costContext+" kubernetesPercent", costObject["kubernetesPercent"])
			}
		}
	}

	if validatedItems == 0 {
		t.Skip("/cloudCost returned no cloud cost items")
	}
}