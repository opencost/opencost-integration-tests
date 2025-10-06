package assert

// Description - Check Promethues Node Pricing Information matches Oracle Billing API Details

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/api"
	"github.com/opencost/opencost-integration-tests/pkg/prometheus"
	"github.com/opencost/opencost-integration-tests/pkg/utils"
)

const tolerance = 0.05

type ProductPartNumber struct {
	OCPU   string `json:"OCPU"`
	Memory string `json:"Memory"`
	GPU    string `json:"GPU"`
	Disk   string `json:"Disk"`
}

// Represents partnumbers.json structure
type InstanceSKU map[string]ProductPartNumber

// loadJSONFile reads a JSON file from the given filePath and unmarshals it to the target
func loadJSONFile(filePath string, target interface{}) error {
	// Read the file content
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("error reading file %s: %w", filePath, err)
	}

	// Unmarshal the JSON data into the target struct
	err = json.Unmarshal(data, target)
	if err != nil {
		return fmt.Errorf("error unmarshaling JSON from %s: %w", filePath, err)
	}

	return nil
}

type OracleCosts struct {
	CPU    float64
	Memory float64
	GPU    float64
}

func CalCulateTimeCoeff(MetricName string) float64 {
	if strings.Contains(MetricName, "Per Hour") {
		return 1.0
	} else if strings.Contains(MetricName, "Per Month") {
		return 24 * 31
	}
	return 1
}

func OracleNodeCosts(SKU ProductPartNumber) (OracleCosts, error) {

	oracleAPIObj := api.NewOracleBillingAPI()
	// ---------------- --------------------
	// Get Oracle Billing Information
	// -------------------------------------

	// Oracle can return a instance cost by monthly or hourly usage
	var OracleNodeCost OracleCosts
	var err error

	if SKU.OCPU != "" {
		cpureq := api.OracleRequest{
			CurrencyCode: "USD",
			PartNumber:   SKU.OCPU,
		}
		// CPU Costs
		cpuresp, err := oracleAPIObj.GetOracleBillingInformation(cpureq)
		if err != nil {
			fmt.Sprintf("Error while calling Oracle API %v", err)
			return OracleNodeCost, err
		}
		// In Oracle, 1 Oracle-CPU = 2 Virtual-CPU
		// We want to calculate costs per V-CPU
		OracleNodeCost.CPU = (cpuresp.Items[0].CurrencyCodeLocalizations[0].Prices[0].Value / CalCulateTimeCoeff(cpuresp.Items[0].MetricName)) / 2

	} else {
		OracleNodeCost.CPU = 0.0
	}

	if SKU.Memory != "" {
		memreq := api.OracleRequest{
			CurrencyCode: "USD",
			PartNumber:   SKU.Memory,
		}
		// Memory Costs
		memresp, err := oracleAPIObj.GetOracleBillingInformation(memreq)
		if err != nil {
			fmt.Sprintf("Error while calling Oracle API %v", err)
			return OracleNodeCost, err
		}
		OracleNodeCost.Memory = memresp.Items[0].CurrencyCodeLocalizations[0].Prices[0].Value / CalCulateTimeCoeff(memresp.Items[0].MetricName)
	} else {
		OracleNodeCost.Memory = 0.0
	}

	if SKU.GPU != "" {
		gpureq := api.OracleRequest{
			CurrencyCode: "USD",
			PartNumber:   SKU.GPU,
		}
		// GPU Costs
		gpuresp, err := oracleAPIObj.GetOracleBillingInformation(gpureq)
		if err != nil {
			fmt.Sprintf("Error while calling Oracle API %v", err)
			return OracleNodeCost, err
		}
		OracleNodeCost.GPU = gpuresp.Items[0].CurrencyCodeLocalizations[0].Prices[0].Value / CalCulateTimeCoeff(gpuresp.Items[0].MetricName)
	} else {
		OracleNodeCost.GPU = 0.0
	}

	return OracleNodeCost, err

}

func loadRemoteJSON(t *testing.T, url string, target interface{}) error {
	// Create an HTTP client with a timeout
	client := &http.Client{
		Timeout: 30 * time.Second, // Increased timeout for network requests
	}

	// Create a new GET request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Errorf("Error creating request for URL %s: %v", url, err)
		return fmt.Errorf("error creating request: %w", err)
	}

	// Perform the request
	resp, err := client.Do(req)
	if err != nil {
		t.Errorf("Error performing request to %s: %v", url, err)
		return fmt.Errorf("error performing request: %w", err)
	}
	defer resp.Body.Close() // Ensure the response body is closed

	// Check for a successful HTTP status code
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Received non-OK HTTP status from %s: %s", url, resp.Status)
		return fmt.Errorf("received non-OK HTTP status: %s", resp.Status)
	}

	// Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("Error reading response body from %s: %v", url, err)
		return fmt.Errorf("error reading response body: %w", err)
	}

	// Unmarshal the JSON response into the target struct
	err = json.Unmarshal(body, target)
	if err != nil {
		t.Errorf("Error unmarshaling JSON from %s: %v", url, err)
		return fmt.Errorf("error unmarshaling JSON: %w", err)
	}

	return nil
}

func TestOracleNodePricing(t *testing.T) {
	t.Skip("Skipping Oracle Node Pricing Test")
	testCases := []struct {
		name      string
		window    string
		assetType string
	}{
		{
			name:      "Today",
			window:    "24h",
			assetType: "node",
		},
		{
			name:      "Last Two Days",
			window:    "48h",
			assetType: "node",
		},
	}

	t.Logf("testCases: %v", testCases)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			// ----------------------------
			// Load Local JSON File
			// ----------------------------
			var instanceSKUs InstanceSKU

			remoteJSONURL := "https://raw.githubusercontent.com/opencost/opencost/develop/pkg/cloud/oracle/partnumbers/shape_part_numbers.json"
			err := loadRemoteJSON(t, remoteJSONURL, &instanceSKUs)

			if err != nil {
				t.Errorf("Failed to GET file partnumbers.json: %v\n", err)
				return
			}

			queryEnd := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
			endTime := queryEnd.Unix()

			// -------------------------------
			// Node Total Hourly Cost Pr Hr
			// avg_over_time(node_total_hourly_cost{%s}[%s])
			// -------------------------------
			client := prometheus.NewClient()
			promNodeTotalCostHrInput := prometheus.PrometheusInput{}
			promNodeTotalCostHrInput.Metric = "node_total_hourly_cost"
			promNodeTotalCostHrInput.Function = []string{"avg_over_time"}
			promNodeTotalCostHrInput.QueryWindow = tc.window
			promNodeTotalCostHrInput.Time = &endTime

			promNodeTotalCostHr, err := client.RunPromQLQuery(promNodeTotalCostHrInput)
			if err != nil {
				t.Fatalf("Error while calling Prometheus API %v", err)
			}

			// Store Results in a Node Map
			type NodeData struct {
				NodePartNumber     string
				PromNodeCost       float64
				OracleNodeCost     float64
				AssetNodeTotalCost float64
			}

			nodeMap := make(map[string]*NodeData)

			// Store Prometheus Pod Prometheus Results
			for _, promNodeTotalCostHrItem := range promNodeTotalCostHr.Data.Result {
				node := promNodeTotalCostHrItem.Metric.Node
				instanceType := promNodeTotalCostHrItem.Metric.InstanceType
				cost := promNodeTotalCostHrItem.Value.Value

				// Default Node
				if node == "" {
					continue
				}

				queryWindow, _ := utils.ExtractNumericPrefix(tc.window)
				promCost := cost * queryWindow

				nodeMap[node] = &NodeData{
					NodePartNumber: instanceType,
					PromNodeCost:   promCost,
				}
			}

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

				node := assetResponseItem.Properties.Name
				nodeInfo, ok := nodeMap[node]
				if !ok {
					t.Logf("Node Information Missing from Prometheus %s", node)
					continue
				}

				// Get Oracle Prices
				OracleCostPerHr, err := OracleNodeCosts(instanceSKUs[nodeInfo.NodePartNumber])
				if err != nil {
					t.Errorf("Oracle Billing Information Missing")
					continue
				}

				oracleTotalCosts := 0.0
				// CPU
				oracleTotalCosts += OracleCostPerHr.CPU * assetResponseItem.CPUCoreHours
				// Memory
				oracleTotalCosts += OracleCostPerHr.Memory * (assetResponseItem.RAMByteHours / 1024 / 1024 / 1024)
				// GPU
				oracleTotalCosts += OracleCostPerHr.GPU * assetResponseItem.GPUHours

				nodeInfo.AssetNodeTotalCost = assetResponseItem.TotalCost
				nodeInfo.OracleNodeCost = oracleTotalCosts
			}

			// Compare Results
			for node, nodeInfo := range nodeMap {
				t.Logf("Node: %s", node)
				t.Logf("Details: %s", nodeInfo.NodePartNumber)

				// Verify Assets API with Prometheus
				withinRange, diff_percent := utils.AreWithinPercentage(nodeInfo.AssetNodeTotalCost, nodeInfo.PromNodeCost, tolerance)
				if withinRange {
					t.Logf("    - NodeTotalPromCost[Pass]: ~%0.2f", nodeInfo.AssetNodeTotalCost)
				} else {
					t.Errorf("    - NodeTotalPromCost[Fail]: DifferencePercent: %0.2f, Prom Results: %0.4f, API Results: %0.4f", diff_percent, nodeInfo.AssetNodeTotalCost, nodeInfo.PromNodeCost)
				}

				// Verify Assets API with Oracle API
				withinRange, diff_percent = utils.AreWithinPercentage(nodeInfo.AssetNodeTotalCost, nodeInfo.OracleNodeCost, tolerance)
				if withinRange {
					t.Logf("    - NodeTotalOracleCost[Pass]: ~%0.2f", nodeInfo.AssetNodeTotalCost)
				} else {
					t.Errorf("    - NodeTotalOracleCost[Fail]: DifferencePercent: %0.2f, Oracle Results: %0.4f, API Results: %0.4f", diff_percent, nodeInfo.OracleNodeCost, nodeInfo.AssetNodeTotalCost)
				}

			}
		})
	}
}
