package api

import (
	"fmt"
	"strings"
	"github.com/opencost/opencost-integration-tests/pkg/env"
)

// -----------------------------------------------------
// Sample API Response
// {
//     "items": [
//         {
//             "partNumber": "B88298",
//             "displayName": "Oracle WebCenter Portal Cloud Service",
//             "metricName": "OCPU Per Hour",
//             "serviceCategory": "WebCenter Portal",
//             "currencyCodeLocalizations": [
//                 {
//                     "currencyCode": "USD",
//                     "prices": [
//                         {
//                             "model": "PAY_AS_YOU_GO",
//                             "value": 0.7742
//                         }
//                     ]
//                 }
//             ]
//         }
//     ]
// }

type OracleResponse struct {
	Items []struct {
		PartNumber                string `json:"partNumber"`
		DisplayName               string `json:"displayName"`
		MetricName                string `json:"metricName"`
		ServiceCategory           string `json:"serviceCategory"`
		CurrencyCodeLocalizations []struct {
			CurrencyCode string `json:"currencyCode"`
			Prices       []struct {
				Model string  `json:"model"`
				Value float64 `json:"value"`
			} `json:"prices"`
		} `json:"currencyCodeLocalizations"`
	} `json:"items`
}

type OracleRequest struct {
	CurrencyCode string
	PartNumber   string
}

func (or OracleRequest) QueryString() string {
	params := []string{}

	if or.CurrencyCode != "" {
		params = append(params, fmt.Sprintf("currencyCode=%s", or.CurrencyCode))
	}
	if or.PartNumber != "" {
		params = append(params, fmt.Sprintf("partNumber=%s", or.PartNumber))
	}
	if len(params) == 0 {
		return ""
	}
	return fmt.Sprintf("?%s", strings.Join(params, "&"))
}


func NewOracleBillingAPI() *API {
	return &API{
		url: env.GetDefaultOracleBillingURL(),
	}
}

// Billing URL
// https://apexapps.oracle.com/pls/apex/cetools/api/v1/products/
func (api *API) GetOracleBillingInformation(req OracleRequest) (*OracleResponse, error) {
	resp := &OracleResponse{}

	err := api.GET("/pls/apex/cetools/api/v1/products", req, resp)

	if err != nil {
		return nil, err
	}

	return resp, nil
}
