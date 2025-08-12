package api

import (
	"fmt"
	"strings"
	"time"
)

// GetAssets requests GET /assets
func (api *API) GetAssets(req AssetsRequest) (*AssetsResponse, error) {
	resp := &AssetsResponse{}

	err := api.GET("/assets", req, resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

type AssetsRequest struct {
	Window string
	Filter string
}

func (ar AssetsRequest) QueryString() string {
	params := []string{}

	params = append(params, fmt.Sprintf("window=%s", ar.Window))
	if ar.Filter != "" {
		params = append(params, fmt.Sprintf("filter=assetType:\"%s\"", ar.Filter))
	}

	return fmt.Sprintf("?%s", strings.Join(params, "&"))
}

type AssetsResponse struct {
	Code int                           `json:"code"`
	Data map[string]AssetsResponseItem `json:"data"`
}

type AssetsResponseItem struct {
	Properties   *AssetsResponseItemProperties `json:"properties"`
	Labels       map[string]string             `json:"labels"`
	Window       Window                        `json:"window"`
	Start        time.Time                     `json:"start"`
	End          time.Time                     `json:"end"`
	Minutes      float64                       `json:"minutes"`
	Adjustment   float64                       `json:"adjustment"`
	CPUCoreHours float64                       `json:"cpuCoreHours`
	RAMByteHours float64                       `json:"ramByteHours`
  GPUCount     float64						           `json:"gpuCount"`
	GPUHours     float64                       `json:"GPUHours`
	CPUCores     float64                       `json:"cpuCores`
	RAMBytes     float64                       `json:"ramBytes`
  RAMCost	     float64			                 `json:"ramCost"`
	CPUCost	     float64			                 `json:"cpuCost"`
	GPUCost	     float64			                 `json:"gpuCost"`
	TotalCost    float64                       `json:"totalCost"`
	Local        float64                       `json:"local"`
	ByteHours    float64                       `json:"byteHours"`
	NodeType     string                        `json:"nodeType"`
}

type AssetsResponseItemProperties struct {
	Category   string `json:"category"`
	Provider   string `json:"provider"`
	Account    string `json:"account"`
	Project    string `json:"project"`
	Service    string `json:"service"`
	Cluster    string `json:"cluster"`
	Name       string `json:"name"`
	ProviderID string `json:"providerID"`
  Node	     string `json:"node"`
}
