package allocation

// Validates that allocation api handles invalid parameters correctly.
// Passing Criteria:
// Returns a HTTP 400 error instead of a response with no error or an HTTP 500 error.

import (
	"strings"
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/api"
)

func TestAllocationInvalidParameters(t *testing.T) {
	apiObj := api.NewAPI()

	invalidParameterTestCases := []struct {
		name        string
		window      string
		filter      string
		aggregate   string
		accumulate  string
		includeidle string
	}{
		{
			name:      "ReverseWindow",
			window:    "2026-06-16T00:00:00Z,2026-06-15T00:00:00Z",
			aggregate: "namespace",
		},
		{
			name:      "InvalidWindowFormat",
			window:    "invalid-window-format",
			aggregate: "namespace",
		},
		{
			name:        "InvalidAggregate",
			window:      "24h",
			aggregate:   "invalid-aggregate",
			accumulate:  "true",
			includeidle: "true",
		},
		{
			name:        "InvalidFilter",
			window:      "24h",
			filter:      "invalid-filter",
			accumulate:  "true",
			includeidle: "true",
			aggregate:   "namespace",
		},
		{
			name:        "InvalidAccumulate",
			window:      "24h",
			accumulate:  "invalid-accumulate",
			includeidle: "true",
			aggregate:   "namespace",
		},
		{
			name:        "InvalidIncludeIdle",
			window:      "24h",
			accumulate:  "true",
			includeidle: "invalid-include-idle",
			aggregate:   "namespace",
		},
	}

	for _, tc := range invalidParameterTestCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Testing: %s", tc.name)

			_, err := apiObj.GetAllocation(api.AllocationRequest{
				Window:      tc.window,
				Filter:      tc.filter,
				Accumulate:  tc.accumulate,
				IncludeIdle: tc.includeidle,
				Aggregate:   tc.aggregate,
			})

			// Assert that an error was returned since input is invalid.
			if err == nil {
				t.Fatalf("Expected an error for invalid input, but got a successful response")
			}

			// Assert that it is a 400 error.
			if !strings.Contains(err.Error(), "HTTP 400") {
				t.Fatalf("expected HTTP 400 error, got %v", err)
			}
		})
	}
}
