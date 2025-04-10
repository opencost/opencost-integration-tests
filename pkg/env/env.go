package env

import (
	"os"
	"strconv"
	"strings"

	"github.com/opencost/opencost-integration-tests/pkg/log"
)

const defaultURL = "http://localhost:9003"
const defaultApproxThreshold = 0.0001 // 0.01%

func GetDefaultURL() string {
	url := defaultURL

	if os.Getenv("OPENCOST_URL") != "" {
		url = os.Getenv("OPENCOST_URL")
	}

	return strings.TrimRight(url, "/")
}

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
