package allocation

// Tests AllocationAPI returns a valid Window.start and Window.end 
// This is PoC test, there are lot of cases like "lastweek", "30m" and Unix timestamp values
// Since I don't find the end date calculation consistent in the response, I have chosen to hardcode based on simple examples.

// Test Case Pass Criteria
// If Window.Start and Window.End correspond to the same time window as Input Window Argument

import (
	"time"
	"testing"
	"github.com/opencost/opencost-integration-tests/pkg/api"
)

//Calculate time window given a time string
func calculateTimeWindow(window string, apiObj api.Window) (bool, api.Window) {
	now := time.Now().UTC()
	var startTime, endTime time.Time
	windowComparisonStatus := false

	//Ex 2025-05-17T00:00:00Z
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	switch window {
		case "today":
			startTime = todayStart
			endTime = todayStart.Add(24 * time.Hour)
		case "yesterday":
			startTime = todayStart.AddDate(0, 0, -1) 
			endTime = todayStart           
		case "7d":
			startTime = todayStart.AddDate(0, 0, -6) // or todayStart.Add(24 * time.Hour).AddDate(0, 0, -7)
			endTime = todayStart.Add(24 * time.Hour)
		default:
			return false, api.Window{}
		}

		calcualtedWindow := api.Window{
			Start: startTime,
			End: endTime,
		}
		// Compare and set status
		if calcualtedWindow.Start == apiObj.Start && calcualtedWindow.End == apiObj.End {
			windowComparisonStatus = true
		}

		return windowComparisonStatus, calcualtedWindow
}

func TestResponseWindowandInput(t *testing.T) {
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
		},
		{
			name: "Yesterday",
			window: "yesterday",
			aggregate: "cluster",
			accumulate: "false",
		},
		{
			name: "week",
			window: "7d",
			aggregate: "namespace",
			accumulate: "false",
		},
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
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
				t.Logf("Response Data Step Index %d:\n", i+1) // There is only one index as step = window by default
				for mapkey, allocationRequestObj := range allocationRequestObjMap {
					t.Logf("Name: %v\n", mapkey)
					matchStatus, calcualtedWindow := calculateTimeWindow(tc.window, allocationRequestObj.Window)
					if matchStatus == true {
						t.Logf("Input Window matches Response Window")
					} else {
						t.Errorf("Response Window %v does not match Input Windows %v", calcualtedWindow, allocationRequestObj.Window)
					}
				}
		}
		})
	}
}
