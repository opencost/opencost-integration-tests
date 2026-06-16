package allocation

import (
    "testing"

    "github.com/opencost/opencost-integration-tests/pkg/api"
)

// TestAllocationEmptyWindow validates that allocation API returns a clean, well-formed
// empty response for valid but data-less windows.
//
// Expected response shape for empty windows:
// - HTTP Code: 200 (success)
// - Data: empty slice [] or list of empty allocations
// - No panic or 5xx errors
func TestAllocationEmptyWindow(t *testing.T) {
    apiObj := api.NewAPI()

    testCases := []struct {
        name      string
        window    string
        aggregate string
    }{
        {
            name:      "FarPastWindow",
            window:    "1970-01-01T00:00:00Z,1970-01-02T00:00:00Z",
			aggregate: "namespace",
        },
        {
            name:      "FutureWindow",
            window:    "2099-01-01T00:00:00Z,2099-01-02T00:00:00Z",
            aggregate: "namespace",
        },
        {
            name:      "BeyondRetentionWindow",
            window:    "2000-01-01T00:00:00Z,2000-01-02T00:00:00Z",
            aggregate: "cluster",
        },
        {
            name:      "TestPodEmptyWindow",
            window:    "1970-01-01T00:00:00Z,1970-01-02T00:00:00Z",
            aggregate: "pod",
        },
        {
            name:      "TestContainerEmptyWindow",
            window:    "1970-01-01T00:00:00Z,1970-01-02T00:00:00Z",
            aggregate: "container",
        },
    }

    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            t.Logf("Testing: %s - %s", tc.name, tc.window)

            response, err := apiObj.GetAllocation(api.AllocationRequest{
                Window:    tc.window,
                Aggregate: tc.aggregate,
            })

            // Validate no errors occurred
            if err != nil {
                t.Fatalf("Unexpected error calling allocation API: %v", err)
            }

            // Validate HTTP 200
            if response.Code != 200 {
                t.Fatalf("Expected HTTP 200, got %d", response.Code)
            }

            // Validate response structure is well-formed
            if response.Data == nil {
                t.Fatalf("Expected non-nil data slice, got nil")
            }

            // Empty window should have empty data or a single empty map
            // The exact format depends on the step parameter, but we should
            // have at least one "window" entry even if no allocations
            if len(response.Data) == 0 {
                t.Logf("Data is empty slice (expected for empty window): %v", response.Data)
            } else {
                // If data has entries, each should be a map
                for i, dataMap := range response.Data {
                    if len(dataMap) == 0 {
                        t.Logf("Data[%d] is empty map (expected for empty window)", i)
                    } else {
                        t.Fatalf("expected empty allocation data for empty window, data[%d] contained allocations: %v", i, dataMap)
                    }
                }
            }
        })
    }
}

