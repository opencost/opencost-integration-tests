package allocation

import (
	"testing"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/api"
)

func mustSummary(t *testing.T, req api.AllocationRequest) *api.AllocationSummaryResponse {
	t.Helper()

	client := api.NewAPI()
	resp, err := client.GetAllocationSummary(req)
	if err != nil {
		t.Fatalf("error querying /allocation/summary: %v", err)
	}

	return resp
}

func lastSundayUTC(ts time.Time) time.Time {
	d := int(ts.Weekday())
	dayStart := ts.UTC().Truncate(24 * time.Hour)
	return dayStart.AddDate(0, 0, -d)
}

func firstOfMonthUTC(ts time.Time) time.Time {
	utc := ts.UTC()
	return time.Date(utc.Year(), utc.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func requireNonEmptyAllocationsInEachSet(t *testing.T, sets []api.AllocationSummaryDataItem) {
	t.Helper()
	for i, set := range sets {
		if len(set.Allocations) == 0 {
			t.Fatalf("expected non-empty allocations in set %d (%s to %s)", i, set.Window.Start, set.Window.End)
		}
	}
}

func TestAccumulateLegacyTruthyValues(t *testing.T) {
	tests := []string{"true", "1", "t", "TRUE"}

	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			resp := mustSummary(t, api.AllocationRequest{
				Window:     "14d",
				Aggregate:  "namespace",
				Accumulate: value,
			})
			if resp.Code != 200 {
				t.Fatalf("expected 200, got %d", resp.Code)
			}
			if len(resp.Data.Sets) != 1 {
				t.Fatalf("expected one accumulated set for accumulate=%q, got %d", value, len(resp.Data.Sets))
			}
			requireNonEmptyAllocationsInEachSet(t, resp.Data.Sets)
		})
	}
}

func TestAccumulateCalendarBucketCounts(t *testing.T) {
	now := time.Now().UTC()
	dayEnd := now.Truncate(24 * time.Hour)
	weekEnd := lastSundayUTC(dayEnd)
	monthEnd := firstOfMonthUTC(dayEnd)

	tests := []struct {
		name          string
		windowStart   time.Time
		windowEnd     time.Time
		accumulate    string
		expectedCount int
	}{
		{
			name:          "day over 2 days",
			windowStart:   dayEnd.AddDate(0, 0, -2),
			windowEnd:     dayEnd,
			accumulate:    "day",
			expectedCount: 2,
		},
		{
			name:          "week over 2 weeks",
			windowStart:   weekEnd.AddDate(0, 0, -14),
			windowEnd:     weekEnd,
			accumulate:    "week",
			expectedCount: 2,
		},
		{
			name:          "month over 2 months",
			windowStart:   monthEnd.AddDate(0, -2, 0),
			windowEnd:     monthEnd,
			accumulate:    "month",
			expectedCount: 2,
		},
		{
			name:          "quarter over 2 quarters",
			windowStart:   monthEnd.AddDate(0, -6, 0),
			windowEnd:     monthEnd,
			accumulate:    "quarter",
			expectedCount: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			window := tc.windowStart.Format(time.RFC3339) + "," + tc.windowEnd.Format(time.RFC3339)
			resp := mustSummary(t, api.AllocationRequest{
				Window:     window,
				Aggregate:  "namespace",
				Accumulate: tc.accumulate,
			})
			if resp.Code != 200 {
				t.Fatalf("expected 200, got %d", resp.Code)
			}
			if len(resp.Data.Sets) != tc.expectedCount {
				t.Fatalf("expected %d sets, got %d (window=%s, accumulate=%s)", tc.expectedCount, len(resp.Data.Sets), window, tc.accumulate)
			}
			requireNonEmptyAllocationsInEachSet(t, resp.Data.Sets)
		})
	}
}

func TestAccumulateByPrecedence(t *testing.T) {
	now := time.Now().UTC()
	weekEnd := lastSundayUTC(now)
	window := weekEnd.AddDate(0, 0, -14).Format(time.RFC3339) + "," + weekEnd.Format(time.RFC3339)

	t.Run("accumulateBy overrides false accumulate", func(t *testing.T) {
		resp := mustSummary(t, api.AllocationRequest{
			Window:       window,
			Aggregate:    "namespace",
			Accumulate:   "false",
			AccumulateBy: "week",
		})
		if resp.Code != 200 {
			t.Fatalf("expected 200, got %d", resp.Code)
		}
		if len(resp.Data.Sets) != 2 {
			t.Fatalf("expected 2 weekly sets from accumulateBy override, got %d", len(resp.Data.Sets))
		}
		requireNonEmptyAllocationsInEachSet(t, resp.Data.Sets)
	})

	t.Run("accumulateBy all overrides accumulate week", func(t *testing.T) {
		resp := mustSummary(t, api.AllocationRequest{
			Window:       window,
			Aggregate:    "namespace",
			Accumulate:   "week",
			AccumulateBy: "all",
		})
		if resp.Code != 200 {
			t.Fatalf("expected 200, got %d", resp.Code)
		}
		if len(resp.Data.Sets) != 1 {
			t.Fatalf("expected 1 set for accumulateBy=all override, got %d", len(resp.Data.Sets))
		}
		requireNonEmptyAllocationsInEachSet(t, resp.Data.Sets)
	})
}

func TestAccumulateStepParameterVariants(t *testing.T) {
	now := time.Now().UTC()
	weekEnd := lastSundayUTC(now)
	window := weekEnd.AddDate(0, 0, -14).Format(time.RFC3339) + "," + weekEnd.Format(time.RFC3339)

	validSteps := []string{"week", "168h", "month"}
	for _, step := range validSteps {
		t.Run("step="+step, func(t *testing.T) {
			resp := mustSummary(t, api.AllocationRequest{
				Window:     window,
				Aggregate:  "namespace",
				Accumulate: "week",
				Step:       step,
			})
			if resp.Code != 200 {
				t.Fatalf("expected 200 for step=%s, got %d", step, resp.Code)
			}
			requireNonEmptyAllocationsInEachSet(t, resp.Data.Sets)
		})
	}
}

func TestAccumulateInvalidParamsReturnBadRequest(t *testing.T) {
	t.Run("invalid accumulateBy", func(t *testing.T) {
		resp := mustSummary(t, api.AllocationRequest{
			Window:       "14d",
			Aggregate:    "namespace",
			AccumulateBy: "not-valid",
		})
		if resp.Code == 200 {
			t.Fatalf("expected non-200 for invalid accumulateBy")
		}
	})

	t.Run("invalid step", func(t *testing.T) {
		resp := mustSummary(t, api.AllocationRequest{
			Window:     "14d",
			Aggregate:  "namespace",
			Accumulate: "week",
			Step:       "not-a-duration",
		})
		if resp.Code == 200 {
			t.Fatalf("expected non-200 for invalid step")
		}
	})
}
