package assert

// Description - Check Promethues PV Pricing Information matches Oracle Billing API Details

import (
	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/utils"

	// "time"
	"testing"
)

const tolerance = 0.05

func OraclePVCost(partNumber string) (float64, error) {

	oracleAPIObj := api.NewOracleBillingAPI()
	// ---------------- --------------------
	// Get Oracle Billing Information
	// -------------------------------------
	oracleCost := 0.0

	// Oracle can return a instance cost by monthly or hourly usage
	pvreq := api.OracleRequest{
		CurrencyCode: "USD",
		PartNumber:   partNumber,
	}
	// PV Costs
	pvresp, err := oracleAPIObj.GetOracleBillingInformation(pvreq)

	oracleCost = pvresp.Items[0].CurrencyCodeLocalizations[0].Prices[0].Value
	return oracleCost, err

}
func TestOraclePVNodePricing(t *testing.T) {
	t.Skip("Skipping Oracle PV Node Pricing Test")
	testCases := []struct {
		name      string
		window    string
		assetType string
	}{
		{
			name:      "Today",
			window:    "24h",
			assetType: "disk",
		},
		{
			name:      "Last Two Days",
			window:    "48h",
			assetType: "disk",
		},
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			// queryEnd := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
			// endTime := queryEnd.Unix()

			// Store Results in a Node Map
			type PVData struct {
				PVPartNumber     string
				OraclePVCost     float64
				AssetPVTotalCost float64
			}

			pvMap := make(map[string]*PVData)

			// API Response
			apiObj := api.NewAPI()
			apiResponse, err := apiObj.GetAssets(api.AssetsRequest{
				Window: tc.window,
				Filter: tc.assetType,
			})

			if err != nil {
				t.Fatalf("Error while calling Allocation API %v", err)
			}
			if apiResponse.Code != 200 {
				t.Errorf("API returned non-200 code")
			}

			// Store Assets and Oracle Result
			for _, assetResponseItem := range apiResponse.Data {

				pv := assetResponseItem.Properties.Name
				byteHours := assetResponseItem.ByteHours
				local := assetResponseItem.Local

				if local > 0 {
					continue
				}

				// Get Oracle Prices
				// Resolve this to depend on Disk Information when there are different disk types
				// Disk Cost is monthlt
				pvPartNumber := "B91961" // The only disk type used
				OracleCostPerHr, err := OraclePVCost(pvPartNumber)
				if err != nil {
					t.Errorf("Oracle Billing Information Missing")
					continue
				}

				oracleTotalCosts := (byteHours / 1024 / 1024 / 1024) * (OracleCostPerHr / 24 / 31)

				pvMap[pv] = &PVData{
					PVPartNumber:     pvPartNumber,
					OraclePVCost:     oracleTotalCosts,
					AssetPVTotalCost: assetResponseItem.TotalCost,
				}
			}

			// Compare Results
			for pv, pvInfo := range pvMap {
				t.Logf("PV: %s", pv)
				t.Logf("Details: %s", pvInfo.PVPartNumber)

				// Verify Assets API with Oracle API
				withinRange, diff_percent := utils.AreWithinPercentage(pvInfo.AssetPVTotalCost, pvInfo.OraclePVCost, tolerance)
				if withinRange {
					t.Logf("    - NodeTotalOracleCost[Pass]: ~%0.2f", pvInfo.AssetPVTotalCost)
				} else {
					t.Errorf("    - NodeTotalOracleCost[Fail]: DifferencePercent: %0.2f, Oracle Results: %0.4f, API Results: %0.4f", diff_percent, pvInfo.OraclePVCost, pvInfo.AssetPVTotalCost)
				}

			}
		})
	}
}
