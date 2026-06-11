package allocation

import (
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/env"
)

func TestAllocationSmoke(t *testing.T) {
	apiObj := api.NewAPI()

	// logging incase we can trace potential env issues with go test -v
	t.Logf("Smoke target (OPENCOST_URL or default): %s", env.GetDefaultURL())

	testCases := []struct {
		name   string
		window string
	}{
		{
			name:   "SimpleValidQuery_1d",
			window: "1d",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			response, err := apiObj.GetAllocation(api.AllocationRequest{
				Window: tc.window,
			})

			if err != nil {
				t.Fatalf("Error while calling Allocation API %v", err)
			}

			if response.Code != 200 {
				t.Errorf("/allocation returned non-200 code: %d", response.Code)
			}

			itemCount := 0
			for _, step := range response.Data {
				itemCount += len(step)
			}
			t.Logf("/allocation OK: code=%d, steps=%d, allocation items=%d",
				response.Code, len(response.Data), itemCount)
		})
	}
}
