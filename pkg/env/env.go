package env

import (
	"os"
	"strconv"
	"strings"

	"github.com/opencost/opencost-integration-tests/pkg/log"
)

const defaultURL = "http://localhost:9003"
const defaultMCPURL = "http://localhost:8081"
const defaultApproxThreshold = 0.0001 // 0.01%
const defaultOracleBillingURL = "https://apexapps.oracle.com/"
const defaultDataResolutionMinutes = 1 // demo.infra.opencost.io sets queryResolutionSeconds: 60
const defaultOpenCostNamespace = "opencost"
const defaultOpenCostLabelSelector = "app.kubernetes.io/name=opencost"

func GetDefaultURL() string {
	url := defaultURL

	if os.Getenv("OPENCOST_URL") != "" {
		url = os.Getenv("OPENCOST_URL")
	}

	return strings.TrimRight(url, "/")
}

// checks if OPENCOST_URL is set, if not, use the default URL
func GetDefaultOracleBillingURL() string {
	url := defaultOracleBillingURL

	if os.Getenv("ORACLE_BILLING_URL") != "" {
		url = os.Getenv("ORACLE_BILLING_URL")
	}

	return strings.TrimRight(url, "/")
}

// checks if COMPARISON_OPENCOST_URL is set, if not, use the default URL
func GetComparisonURL() string {
	url := defaultURL

	if os.Getenv("COMPARISON_OPENCOST_URL") != "" {
		url = os.Getenv("COMPARISON_OPENCOST_URL")
	}

	return strings.TrimRight(url, "/")
}

// checks if APPROX_THRESHOLD is set, if not, use the default threshold
func GetApproxThreshold() float64 {
	approxThreshold := defaultApproxThreshold

	if os.Getenv("APPROX_THRESHOLD") != "" {
		at, err := strconv.ParseFloat(os.Getenv("APPROX_THRESHOLD"), 64)
		if err == nil && at > 0.0 {
			approxThreshold = at
		} else {
			log.Errorf("invalid APPROX_THRESHOLD: %s", os.Getenv("APPROX_THRESHOLD"))
		}
	}

	return approxThreshold
}

// checks if OPENCOST_MCP_URL is set, if not, use the default MCP URL
func GetMCPURL() string {
	url := defaultMCPURL

	if os.Getenv("OPENCOST_MCP_URL") != "" {
		url = os.Getenv("OPENCOST_MCP_URL")
	}

	return strings.TrimRight(url, "/")
}

// checks if OPENCOST_DATA_RESOLUTION_MINUTES is set, if not, use the default data resolution minutes
func GetDataResolutionMinutes() int {
	minutes := defaultDataResolutionMinutes

	if os.Getenv("OPENCOST_DATA_RESOLUTION_MINUTES") != "" {
		m, err := strconv.Atoi(os.Getenv("OPENCOST_DATA_RESOLUTION_MINUTES"))
		if err == nil && m > 0 {
			minutes = m
		} else {
			log.Errorf("invalid OPENCOST_DATA_RESOLUTION_MINUTES: %s", os.Getenv("OPENCOST_DATA_RESOLUTION_MINUTES"))
		}
	}

	return minutes
}

// checks if SHOW_DIFF is set, if not, use the default show diff
func GetShowDiff() bool {
	value := os.Getenv("SHOW_DIFF")
	if value != "" {
		v, err := strconv.ParseBool(value)
		if err == nil {
			return v
		} else {
			log.Errorf("invalid SHOW_DIFF: %s", os.Getenv("SHOW_DIFF"))
		}
	}

	return false
}

func GetOpenCostNamespace() string {
	if ns := os.Getenv("OPENCOST_NAMESPACE"); ns != "" { //looks outside of th eprogram and check the host computer's system settings for an environment variables names OPENCOST_NAMESPACE
		return ns
	}
	return defaultOpenCostNamespace
}

func GetOpenCostLabelSelector() string {
	if sel := os.Getenv("OPENCOST_LABEL_SELECTOR"); sel != "" { //looks outside of th eprogram and check the host computer's system settings for an environment variables names OPENCOST_LABEL_SELECTOR
		return sel
	}
	return defaultOpenCostLabelSelector
}
