package allocation

// Description - Verify allocation window parsing behavior for keyword windows
// and equivalent explicit RFC3339 ranges.
//
// Implementation Details
// - Each keyword window is queried first.
// - The response window from that keyword query is then used to build an
//   equivalent explicit RFC3339 range.
// - The explicit range query should return valid allocation JSON and a
//   consistent total cost for the same window.
// - A completed historical window is inherently more stable, but "today" is
//   still tested because it is a supported keyword window.
//
// Passing Criteria
// - Each keyword window returns a successful Allocation API response.
// - Each equivalent explicit RFC3339 range returns a successful Allocation API response.
// - Keyword and explicit range totals match within the documented tolerance.
// - Failures identify the window form and the observed difference.

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/api"
)

const (
	windowEquivalenceAbsoluteTolerance = 0.01
	windowEquivalenceRelativeTolerance = 0.001
)

type windowEquivalenceCase struct {
	name   string
	window string
}

var windowEquivalenceCases = []windowEquivalenceCase{
	{
		name:   "Today",
		window: "today",
	},
	{
		name:   "Yesterday",
		window: "yesterday",
	},
	{
		name:   "Week",
		window: "week",
	},
}

func TestAllocationKeywordAndExplicitWindowsEquivalent(t *testing.T) {
	apiClient := api.NewAPI()

	for _, tc := range windowEquivalenceCases {
		t.Run(tc.name, func(t *testing.T) {
			keywordResponse := fetchAllocationForWindow(t, apiClient, tc.window)
			keywordTotal := sumAllocationWindowTotalCost(t, keywordResponse)
			keywordWindow := firstAllocationWindow(t, keywordResponse)

			explicitWindow := formatExplicitRFC3339Window(keywordWindow)
			explicitResponse := fetchAllocationForWindow(t, apiClient, explicitWindow)
			explicitTotal := sumAllocationWindowTotalCost(t, explicitResponse)

			if !withinWindowEquivalenceTolerance(keywordTotal, explicitTotal) {
				absoluteDiff := math.Abs(keywordTotal - explicitTotal)
				relativeDiff := relativeWindowDiff(keywordTotal, explicitTotal)

				t.Errorf(
					"window=%q explicitWindow=%q totals diverged: keywordTotal=%.6f explicitTotal=%.6f diff=%.6f relativeDiff=%.4f%%",
					tc.window,
					explicitWindow,
					keywordTotal,
					explicitTotal,
					absoluteDiff,
					relativeDiff*100,
				)
			}

			t.Logf(
				"window=%q explicitWindow=%q keywordTotal=%.6f explicitTotal=%.6f",
				tc.window,
				explicitWindow,
				keywordTotal,
				explicitTotal,
			)
		})
	}
}

func fetchAllocationForWindow(t *testing.T, apiClient *api.API, window string) *api.AllocationResponse {
	t.Helper()

	response, err := apiClient.GetAllocation(api.AllocationRequest{
		Window:      window,
		Aggregate:   "namespace",
		Accumulate:  "true",
		IncludeIdle: "true",
	})
	if err != nil {
		t.Fatalf("allocation request failed for window %q: %v", window, err)
	}

	if response.Code != 200 {
		t.Fatalf("allocation request for window %q returned non-200 code: %d", window, response.Code)
	}

	return response
}

func sumAllocationWindowTotalCost(t *testing.T, response *api.AllocationResponse) float64 {
	t.Helper()

	total := 0.0
	itemCount := 0

	for _, allocationSet := range response.Data {
		for _, item := range allocationSet {
			total += item.TotalCost
			itemCount++
		}
	}

	if itemCount == 0 {
		t.Fatal("allocation response did not contain any allocation items")
	}

	return total
}

func firstAllocationWindow(t *testing.T, response *api.AllocationResponse) api.Window {
	t.Helper()

	for _, allocationSet := range response.Data {
		for _, item := range allocationSet {
			if item.Window.Start.IsZero() || item.Window.End.IsZero() {
				t.Fatalf("allocation item returned invalid window: start=%v end=%v", item.Window.Start, item.Window.End)
			}

			return item.Window
		}
	}

	t.Fatal("allocation response did not contain a window")
	return api.Window{}
}

func formatExplicitRFC3339Window(window api.Window) string {
	return fmt.Sprintf(
		"%s,%s",
		window.Start.UTC().Format(time.RFC3339),
		window.End.UTC().Format(time.RFC3339),
	)
}

func withinWindowEquivalenceTolerance(expected, actual float64) bool {
	absoluteDiff := math.Abs(expected - actual)
	if absoluteDiff <= windowEquivalenceAbsoluteTolerance {
		return true
	}

	denominator := math.Max(math.Abs(expected), math.Abs(actual))
	if denominator == 0 {
		return true
	}

	return absoluteDiff/denominator <= windowEquivalenceRelativeTolerance
}

func relativeWindowDiff(expected, actual float64) float64 {
	denominator := math.Max(math.Abs(expected), math.Abs(actual))
	if denominator == 0 {
		return 0
	}

	return math.Abs(expected-actual) / denominator
}