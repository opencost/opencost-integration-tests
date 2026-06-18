package assets

// Validates that assets api handles invalid parameters correctly.
// Passing Criteria:
// Returns a HTTP 400 error instead of a response with no error or an HTTP 500 error.

import (
	"strings"
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/api"
)

func TestAssetInvalidParameters(t *testing.T) {
	apiObj := api.NewAPI()

	invalidParameterTestCases := []struct {
		name      string
		window    string
		assetType string
	}{
		{
			name:      "ReverseWindow",
			window:    "2026-06-16T00:00:00Z,2026-06-15T00:00:00Z",
			assetType: "node",
		},
		{
			name:      "InvalidWindowFormat",
			window:    "invalid-window-format",
			assetType: "node",
		},
		{
			name:      "InvalidFilter",
			window:    "24h",
			assetType: "invalid-filter",
		},
	}

	for _, tc := range invalidParameterTestCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Testing: %s", tc.name)

			_, err := apiObj.GetAssets(api.AssetsRequest{
				Window: tc.window,
				Filter: tc.assetType,
			})

			if err == nil {
				t.Fatalf("Expected an error for invalid input, but got a successful response")
			}

			if !strings.Contains(err.Error(), "HTTP 400") {
				t.Fatalf("expected HTTP 400 error, got %v", err)
			}
		})
	}
}
