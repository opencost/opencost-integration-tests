package chaos

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

var (
	opencostNamespace     = envOrDefault("OPENCOST_NAMESPACE", "opencost")
	opencostPodLabel      = envOrDefault("OPENCOST_POD_LABEL", "app=opencost")
	prometheusNamespace   = envOrDefault("PROMETHEUS_NAMESPACE", "prometheus")
	prometheusPodLabel    = envOrDefault("PROMETHEUS_POD_LABEL", "app.kubernetes.io/name=prometheus")
	prometheusServiceName = envOrDefault("PROMETHEUS_SERVICE_NAME", "prometheus")
)

const (
	defaultRetryInterval   = 2 * time.Second
	networkPartitionDelay  = "5000ms" // 5s latency
	networkPartitionLoss   = "50%"    // 50% packet loss
	networkPartitionJitter = "100ms"  // ±100ms jitter
)

// ChaosEnvironment holds configuration for chaos injection
type ChaosEnvironment struct {
	KubernetesContext   string
	OpencostNamespace   string
	PrometheusNamespace string
	Enabled             bool
	DryRun              bool
}

// LoadChaosEnv loads chaos environment from OS variables
func LoadChaosEnv() ChaosEnvironment {
	return ChaosEnvironment{
		KubernetesContext:   os.Getenv("KUBE_CONTEXT"),
		OpencostNamespace:   getOrDefault("OPENCOST_NAMESPACE", opencostNamespace),
		PrometheusNamespace: getOrDefault("PROMETHEUS_NAMESPACE", prometheusNamespace),
		Enabled:             os.Getenv("CHAOS_ENABLED") != "",
		DryRun:              os.Getenv("CHAOS_DRY_RUN") != "",
	}
}

// ExecuteCommand runs a shell command and returns output
func ExecuteCommand(cmd string, args ...string) (string, error) {
	command := exec.Command(cmd, args...)
	output, err := command.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

// ExecuteKubectl runs a kubectl command
func ExecuteKubectl(args ...string) (string, error) {
	return ExecuteCommand("kubectl", args...)
}

// CheckPrerequisites verifies required tools are available
func CheckPrerequisites() error {
	required := []string{"kubectl", "tc"}
	for _, tool := range required {
		_, err := ExecuteCommand("which", tool)
		if err != nil {
			return fmt.Errorf("required tool not found: %s", tool)
		}
	}
	return nil
}

// PortForward establishes a kubectl port-forward session
func PortForward(namespace, pod, localPort, remotePort string, duration time.Duration) error {
	cmd := exec.Command(
		"kubectl", "port-forward",
		fmt.Sprintf("pod/%s", pod),
		fmt.Sprintf("%s:%s", localPort, remotePort),
		"-n", namespace,
	)

	go func() {
		_ = cmd.Run()
	}()

	time.Sleep(500 * time.Millisecond)
	return nil
}

func envOrDefault(key string, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getOrDefault(key, defaultVal string) string {
	return envOrDefault(key, defaultVal)
}
