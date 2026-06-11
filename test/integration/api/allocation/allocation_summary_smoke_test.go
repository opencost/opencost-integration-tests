package allocation

import (
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/env"
)

func TestAllocationSummarySmoke(t *testing.T) {

	apiClient := api.NewAPI()

	// logging in case we can trace potential env issues with go test -v
	t.Logf("Smoke target (OPENCOST_URL or default): %s", env.GetDefaultURL())

	testCases := []struct {
		name   string
		window string
	}{
		{
			name:   "ValidQueryTest",
			window: "1d",
		},
	}

	for _, tc := range testCases {

		t.Run(tc.name, func(t *testing.T) {

			ar := api.AllocationRequest{
				Window: tc.window,
			}
			response, err := apiClient.GetAllocationSummary(ar)
			if err != nil {
				t.Fatalf("/allocation/summary window=%s request failed: %v", ar.Window, err)
			}

			if response.Code != 200 {
				t.Errorf("/allocation/summary window=%s returned non-200 code: %d", ar.Window, response.Code)
			}

			t.Logf("/allocation/summary: code=%d, allocation sets returned: %d",
				response.Code, len(response.Data.Sets))
		})
	}
}
