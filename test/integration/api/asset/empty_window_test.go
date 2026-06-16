package assets

import (
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/api"
)

// TestAssetsEmptyWindow validates that assets API returns a clean, well-formed
// empty response for valid but data-less windows.
//
// Expected response shape for empty windows:
// - HTTP Code: 200 (success)
// - Data: empty map {} (no assets in the window)
// - No panic or 5xx errors
func TestAssetsEmptyWindow(t *testing.T) {
	apiObj := api.NewAPI()

	testCases := []struct {
		name      string
		window    string
		assetType string
	}{
		{
			name:      "FarPastWindow",
			window:    "1970-01-01T00:00:00Z,1970-01-02T00:00:00Z",
			assetType: "node",
		},
		{
			name:      "FutureWindow",
			window:    "2099-01-01T00:00:00Z,2099-01-02T00:00:00Z",
			assetType: "node",
		},
		{
			name:      "BeyondRetentionWindow",
			window:    "2000-01-01T00:00:00Z,2000-01-02T00:00:00Z",
			assetType: "disk",
		},
		{
			name:      "EmptyWindowPVC",
			window:    "1970-01-01T00:00:00Z,1970-01-02T00:00:00Z",
			assetType: "pvc",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Testing: %s - %s", tc.name, tc.window)

			response, err := apiObj.GetAssets(api.AssetsRequest{
				Window: tc.window,
				Filter: tc.assetType,
			})

			// Validate no errors occurred
			if err != nil {
				t.Fatalf("Unexpected error calling assets API: %v", err)
			}

			// Validate HTTP 200
			if response.Code != 200 {
				t.Fatalf("Expected HTTP 200, got %d", response.Code)
			}

			// Validate response structure is well-formed
			if response.Data == nil {
				t.Fatalf("Expected non-nil data map, got nil")
			}

			// Empty window should return empty map
			if len(response.Data) == 0 {
				t.Logf("Data is empty map (expected for empty window)")
			} else {
				t.Fatalf("Data has %d entries (filter: %s)", len(response.Data), tc.assetType)
			}
		})
	}
}