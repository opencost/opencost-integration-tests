package validate_api

// Tests AllocationAPI returns a valid output for TotalEfficiency 
// The test assumes a valid efficiency value (based on the literal definition of efficiency) to be between 0% and 100%.
// API returns TotalEfficiency as a float64 value between 0.0 and 1.0. a 1.0 value signifies INF (Infinite) Efficieny in the dashboard.


// Test Cases
// Designed to test various payload combinations in the API
// Available filters in the dasboard that are tested include "Date Range" and "Breakdown"
// API Payload combinations include varying "aggregate" and "window" values. Ex: aggregate=namespace&window=today, aggregate=service&window=yesterday
// Currency just seems to prepend the respective currency symbol, so not testing it here


// Pass Criteria
// If all items identified by their name have valid TotalEfficiency
import (
	"fmt"
	"time"
	"testing"
	"github.com/opencost/opencost-integration-tests/pkg/api"
)

func TestTotalEfficiencyValue(t *testing.T) {
	apiObj := api.NewAPI()

	testCases := []struct {
		name		string
		window      string
		aggregate   string
		accumulate  string
		includeidle string
	}{
		{
			name: "Today",
			window: "today",
			aggregate: "namespace",
			accumulate: "false",
			includeidle: "true",
		},
		{
			name: "Yesterday",
			window: "yesterday",
			aggregate: "cluster",
			accumulate: "false",
			includeidle: "true",
		},
		{
			name: "Last week",
			window: "week",
			aggregate: "container",
			accumulate: "false",
			includeidle: "true",
		},
		{
			name: "Last 14 days",
			window: "14d",
			aggregate: "service",
			accumulate: "false",
			includeidle: "true",
		},
		{
			name: "Custom",
			window: "%sT00:00:00Z,%sT00:00:00Z", // This can be generated dynamically based on the running time
			aggregate: "namespace",
			accumulate: "false",
			includeidle: "true",
		},
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			if tc.name == "Custom" {
				// Dynamically generate the "Custom" window
				now := time.Now()
				tc.window = fmt.Sprintf(tc.window,
					now.AddDate(0, 0, -2).Format("2006-01-02"),
					now.AddDate(0, 0, -1).Format("2006-01-02"),
				)
			}
			response, err := apiObj.GetAllocation(api.AllocationRequest{
				Window: tc.window,
				Aggregate: tc.aggregate,
				Accumulate: tc.accumulate,
				IncludeIdle: tc.includeidle,
			})
			// Catch API Request Errors
			if err != nil {
				t.Fatalf("Error while calling Allocation API %v", err)
			}
			if response.Code != 200 {
				t.Errorf("API returned non-200 code")
			}
			t.Logf("Breakdown %v", tc.aggregate)
			for i, allocationRequestObjMap := range response.Data {
				t.Logf("Response Data Slice Index %d:\n", i+1) // There is only one index in the slice all the time for my observation
				for mapkey, allocationRequestObj := range allocationRequestObjMap {
					t.Logf("Name: %v\n", mapkey)
					// Really not sure about the upper limit as some values cross 100% in the demo dashboard
					if allocationRequestObj.TotalEfficiency >= 0.0 && allocationRequestObj.TotalEfficiency <= 1.0 {
						t.Logf("Total Efficiency for %v is a Valid Value", mapkey)
					} else {
						t.Errorf("Total Efficiency for %v is an Invalid Value: %v", mapkey, allocationRequestObj.TotalEfficiency)
					}
				}
		}
		})
	}
}
