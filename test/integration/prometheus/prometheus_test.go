package prometheus

import (
	"fmt"
	"testing"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/log"
)

// Ensure we do not regress when querying Prometheus to extract a resource's
// (e.g. pod, node, loadbalancer, disk) start and end time.
// https://github.com/opencost/opencost/pull/3094
//
// Since integration tests run without a real Prometheus instance, we verify
// correctness by checking if at least a majority of resource start/end times
// match the query window. This approach assumes stable infrastructure
// (long-running nodes and disks) and may need improvement in the future.
func TestDatasourceStartAndEndTime(t *testing.T) {
	a := api.NewAPI()

	testCases := []struct {
		name      string
		windowFmt string
	}{
		{
			name:      "Full day window",
			windowFmt: "%sT00:00:00Z,%sT00:00:00Z",
		},
		{
			name:      "Partial day window",
			windowFmt: "%sT01:00:00Z,%sT02:00:00Z",
		},
		{
			name:      "Partial day window with offset",
			windowFmt: "%sT03:25:00Z,%sT06:10:00Z",
		},
	}

	log.Infof("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now()
			window := fmt.Sprintf(tc.windowFmt,
				now.AddDate(0, 0, -2).Format("2006-01-02"),
				now.AddDate(0, 0, -1).Format("2006-01-02"))

			log.Infof("window: %s", window)

			resp, err := a.GetAssets(api.AssetsRequest{
				Window: window,
			})
			if err != nil {
				t.Fatalf("Failed to get assets: %v", err)
			}

			if resp.Code != 200 {
				t.Fatalf("Expected status code 200, got %d", resp.Code)
			}

			if len(resp.Data) == 0 {
				t.Fatal("Expected non-empty data response")
			}

			var matchingCount, totalCount int
			for _, asset := range resp.Data {
				totalCount++
				if asset.Start.Equal(asset.Window.Start) && asset.End.Equal(asset.Window.End) {
					matchingCount++
				}
			}

			if matchPercentage := float64(matchingCount) / float64(totalCount) * 100; matchPercentage < 50.0 {
				t.Errorf("Expected at least 50%% of assets to have matching window times, got %.2f%% (%d/%d assets)",
					matchPercentage, matchingCount, totalCount)
			}
		})
	}
}
