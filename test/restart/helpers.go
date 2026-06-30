package restart

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	defaultReadyTimeout   = 3 * time.Minute
	defaultRetryInterval  = 3 * time.Second
	defaultRequestTimeout = 20 * time.Second
	defaultLogTailLines   = 500

	// historyTolerance bounds how much a fixed past window's cost may drift
	// across a restart. Past data should be essentially identical; the small
	// allowance covers churn-resampling jitter, not lost history.
	historyTolerance = 0.05
)

// RestartEnvironment holds configuration for the restart-recovery suite. Like
// the chaos suite, it talks to OpenCost only through the trusted ops-broker.
type RestartEnvironment struct {
	Enabled            bool
	DryRun             bool
	BrokerURL          string
	Token              string
	OpenCostNamespace  string
	OpenCostDeployment string
	OpenCostSelector   string
}

// LoadRestartEnv loads the restart environment from OS variables.
func LoadRestartEnv() RestartEnvironment {
	return RestartEnvironment{
		Enabled:            os.Getenv("RESTART_ENABLED") != "",
		DryRun:             os.Getenv("RESTART_DRY_RUN") != "",
		BrokerURL:          strings.TrimRight(os.Getenv("OPENCOST_BROKER_URL"), "/"),
		Token:              os.Getenv("OPENCOST_BROKER_TOKEN"),
		OpenCostNamespace:  getEnv("OPENCOST_NAMESPACE", "opencost"),
		OpenCostDeployment: getEnv("OPENCOST_DEPLOYMENT", "opencost"),
		OpenCostSelector:   getEnv("OPENCOST_SELECTOR", "app.kubernetes.io/name=opencost"),
	}
}

func (e RestartEnvironment) ValidateBrokerConfig() error {
	if e.BrokerURL == "" {
		return fmt.Errorf("OPENCOST_BROKER_URL is required for broker-driven restart tests")
	}
	if e.Token == "" {
		return fmt.Errorf("OPENCOST_BROKER_TOKEN is required for broker-driven restart tests")
	}
	return nil
}

// FindPanic returns the first log line that looks like a Go panic or fatal
// crash, or "" if none are present. Used to assert OpenCost came back cleanly
// after a restart.
func FindPanic(lines []string) string {
	for _, line := range lines {
		l := strings.ToLower(line)
		if strings.Contains(l, "panic:") ||
			strings.Contains(l, "fatal error") ||
			strings.Contains(l, "runtime error") {
			return line
		}
	}
	return ""
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
