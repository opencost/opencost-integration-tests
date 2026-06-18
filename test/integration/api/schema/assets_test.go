package schema

import (
	"fmt"
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/api"
)

// Description - Assert that /assets responses retain their expected public
// JSON fields, nested objects, and important field types.
//
// Implementation Details
// - Fields shared by every asset are validated separately from fields that are
//   specific to Node assets.
// - Additional response fields and non-Node asset types are allowed.
// - The test is skipped when the endpoint returns no assets.
//
// Passing Criteria
// - The endpoint returns a successful response.
// - Every asset contains the common required fields.
// - Node assets contain their Node-specific fields and properties.

var assetCommonRequiredFields = []string{
	"type",
	"properties",
	"window",
	"start",
	"end",
	"minutes",
	"totalCost",
}

var assetCommonPropertiesRequiredFields = []string{
	"category",
	"provider",
	"service",
}

var assetNodeRequiredFields = []string{
	"cpuCost",
	"ramCost",
	"gpuCost",
	"nodeType",
	"cpuCores",
	"ramBytes",
}

var assetNodePropertiesRequiredFields = []string{
	"name",
	"providerID",
}

func TestAssetsResponseSchemaStability(t *testing.T) {
	apiClient := api.NewAPI()

	resp := fetchRawEndpoint(t, apiClient, "/assets", api.AutocompleteRequest{
		Window: "1h",
	})

	requireSuccessfulResponse(t, "/assets response", resp)

	data := requireMap(t, "/assets data", resp["data"])
	if len(data) == 0 {
		t.Skip("/assets returned no asset items")
	}

	for assetName, rawAsset := range data {
		itemContext := fmt.Sprintf("/assets item %q", assetName)
		asset := requireMap(t, itemContext, rawAsset)

		requireFields(t, itemContext, asset, assetCommonRequiredFields)
		requireNumber(t, itemContext+" totalCost", asset["totalCost"])

		windowContext := itemContext + " window"
		window := requireMap(t, windowContext, asset["window"])
		requireFields(t, windowContext, window, windowRequiredFields)
		requireString(t, windowContext+" start", window["start"])
		requireString(t, windowContext+" end", window["end"])

		propertiesContext := itemContext + " properties"
		properties := requireMap(t, propertiesContext, asset["properties"])
		requireFields(t, propertiesContext, properties, assetCommonPropertiesRequiredFields)

		assetType := requireString(t, itemContext+" type", asset["type"])
		if assetType == "Node" {
			requireFields(t, itemContext, asset, assetNodeRequiredFields)
			requireFields(t, propertiesContext, properties, assetNodePropertiesRequiredFields)
		}
	}
}