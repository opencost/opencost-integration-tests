package chaos

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	defaultRetryInterval  = 2 * time.Second
	defaultReadyTimeout   = 2 * time.Minute
	defaultRequestTimeout = 20 * time.Second

	scenarioKillOpenCost        = "kill-opencost"
	scenarioKillPrometheus      = "kill-prometheus"
	scenarioPartitionPrometheus = "partition-prometheus"
	scenarioLatencyPrometheus   = "latency-prometheus"
)

var requiredBrokerScenarios = []string{
	scenarioKillOpenCost,
	scenarioKillPrometheus,
	scenarioPartitionPrometheus,
	scenarioLatencyPrometheus,
}

// ChaosEnvironment holds configuration for chaos injection.
type ChaosEnvironment struct {
	Enabled   bool
	DryRun    bool
	BrokerURL string
	Token     string
}

// LoadChaosEnv loads chaos environment from OS variables.
func LoadChaosEnv() ChaosEnvironment {
	return ChaosEnvironment{
		Enabled:   os.Getenv("CHAOS_ENABLED") != "",
		DryRun:    os.Getenv("CHAOS_DRY_RUN") != "",
		BrokerURL: strings.TrimRight(os.Getenv("OPENCOST_BROKER_URL"), "/"),
		Token:     os.Getenv("OPENCOST_BROKER_TOKEN"),
	}
}

func (c ChaosEnvironment) ValidateBrokerConfig() error {
	if c.BrokerURL == "" {
		return fmt.Errorf("OPENCOST_BROKER_URL is required for broker-driven chaos tests")
	}
	if c.Token == "" {
		return fmt.Errorf("OPENCOST_BROKER_TOKEN is required for broker-driven chaos tests")
	}
	return nil
}
