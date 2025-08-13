package api

// GetAllocation requests GET /allocation
func (api *API) GetAllocationSummary(req AllocationRequest) (*AllocationSummaryResponse, error) {
	resp := &AllocationSummaryResponse{}

	err := api.GET("/allocation/summary", req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

type AllocationSummaryResponse struct {
	Code int                   `json:"code"`
	Data AllocationSummaryData `json:"data"`
}

type AllocationSummaryData struct {
	Step int                         `json:"step"`
	Sets []AllocationSummaryDataItem `json:"sets`
}

type AllocationSummaryDataItem struct {
	Allocations map[string]AllocationResponseItem `json:"allocations"`
	Window      Window                            `json:"window`
}
