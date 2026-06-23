package reliability

import (
	"encoding/json"
	"os/exec"
	"testing"

	"github.com/opencost/opencost-integration-tests/pkg/env"
)

type podList struct {
	Items []pod `json:"items"`
}

type pod struct {
	Metadata podMetadata `json:"metadata"`
	Status   podStatus   `json:"status"`
}

type podMetadata struct {
	Name string `json:"name"`
}

type podStatus struct {
	ContainerStatuses []containerStatus `json:"containerStatuses"`
}

type containerStatus struct {
	Name         string `json:"name"`
	RestartCount int32  `json:"restartCount"`
}

func requireKubectl(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("kubectl"); err != nil { //chwcks PATH to see if kubectl is installed
		t.Skipf("kubectl not found: %v", err)
	}

	if err := exec.Command("kubectl", "cluster-info").Run(); err != nil { //checks if the kubernetes cluster is reachable
		t.Skipf("kubernetes cluster not reachable: %v", err)
	}
}

func dumpPodDiagnostics(t *testing.T, podName, containerName string) {
	t.Helper()
	ns := env.GetOpenCostNamespace()

	describe, _ := exec.Command("kubectl", "describe", "pod", "-n", ns, podName).CombinedOutput()
	t.Logf("kubectl describe pod %s:\n%s", podName, describe)

	logs, _ := exec.Command(
		"kubectl", "logs", "-n", ns, podName,
		"-c", containerName, "--previous",
	).CombinedOutput()
	t.Logf("kubectl logs --previous %s/%s:\n%s", podName, containerName, logs)
}

func TestNoOpenCostPodRestarts(t *testing.T) {
	requireKubectl(t)

	out, err := exec.Command(
		"kubectl", "get", "pods",
		"-n", env.GetOpenCostNamespace(),
		"-l", env.GetOpenCostLabelSelector(),
		"-o", "json",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("kubectl get pods: %v\n%s", err, out)
	}

	var pods podList
	if err := json.Unmarshal(out, &pods); err != nil {
		t.Fatalf("decode pods json: %v", err)
	}

	if len(pods.Items) == 0 {
		t.Fatalf("no OpenCost pods found in namespace %q", env.GetOpenCostNamespace())
	}

	for _, p := range pods.Items {
		for _, cs := range p.Status.ContainerStatuses {
			if cs.RestartCount != 0 {
				t.Errorf("pod %s container %s restartCount=%d, want 0",
					p.Metadata.Name, cs.Name, cs.RestartCount)
				dumpPodDiagnostics(t, p.Metadata.Name, cs.Name)
			}
		}
	}
}
