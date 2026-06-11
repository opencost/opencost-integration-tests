package assets

import (
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/env"
)

func TestAssetsSmoke(t *testing.T) {

	apiClient := api.NewAPI()

	// logging in case we can trace potential env issues with go test -v
	t.Logf("Assets Smoke target (OPENCOST_URL or default): %s", env.GetDefaultURL())

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

			ar := api.AssetsRequest{
				Window: tc.window,
			}
			response, err := apiClient.GetAssets(ar)
			if err != nil {
				t.Fatalf("/assets?window=%s request failed: %v", ar.Window, err)
			}

			if response.Code != 200 {
				t.Errorf("/assets?window=%s returned non-200 code: %d", ar.Window, response.Code)
			}

			if len(response.Data) == 0 {
				t.Logf("warning: /assets returned 200 but zero assets")
			}

			t.Logf("/assets: code=%d, assets returned: %d", response.Code, len(response.Data))
		})
	}
}
