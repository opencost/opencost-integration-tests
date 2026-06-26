package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// K8sClient is a connection to the cluster the broker pod already runs in
// (in-cluster SA token, or KUBECONFIG for local dev). It does NOT create a
// cluster — it just talks to the existing one through a scoped clientset.
type K8sClient struct {
	client              kubernetes.Interface
	dynamic             dynamic.Interface
	namespace           string
	deploy              string
	selector            string
	prometheusNamespace string
	prometheusSelector  string
	chaosNamespace      string
}

func NewK8sClient(cfg Config) (*K8sClient, error) {
	var restCfg *rest.Config
	var err error
	if cfg.Kubeconfig != "" {
		// Local dev: talk to the cluster in KUBECONFIG.
		restCfg, err = clientcmd.BuildConfigFromFlags("", cfg.Kubeconfig)
	} else {
		// In-cluster: reads the auto-mounted SA token + CA + API host.
		restCfg, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("building k8s rest config: %w", err)
	}

	client, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("building k8s client: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("building dynamic k8s client: %w", err)
	}

	return &K8sClient{
		client:              client,
		dynamic:             dynamicClient,
		namespace:           cfg.Namespace,
		deploy:              cfg.OpenCostDeployment,
		selector:            cfg.OpenCostSelector,
		prometheusNamespace: cfg.PrometheusNamespace,
		prometheusSelector:  cfg.PrometheusSelector,
		chaosNamespace:      cfg.ChaosNamespace,
	}, nil
}

// RestartOpenCost triggers a rolling restart of the OpenCost deployment by
// stamping the pod-template restart annotation — the same mechanism as
// `kubectl rollout restart`. The target is fixed by config, never the caller.
func (c *K8sClient) RestartOpenCost(ctx context.Context) error {
	patch := fmt.Sprintf(
		`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":%q}}}}}`,
		time.Now().UTC().Format(time.RFC3339),
	)
	_, err := c.client.AppsV1().Deployments(c.namespace).Patch(
		ctx, c.deploy, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("restarting deployment %s/%s: %w", c.namespace, c.deploy, err)
	}
	return nil
}

// PodInfo is the trimmed pod view the broker returns (not raw k8s objects).
type PodInfo struct {
	Name         string `json:"name"`
	Phase        string `json:"phase"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restartCount"`
}

// PodStatus lists OpenCost pods so tests can wait for readiness after a restart.
func (c *K8sClient) PodStatus(ctx context.Context) ([]PodInfo, error) {
	pods, err := c.client.CoreV1().Pods(c.namespace).List(
		ctx, metav1.ListOptions{LabelSelector: c.selector},
	)
	if err != nil {
		return nil, fmt.Errorf("listing pods in %s: %w", c.namespace, err)
	}

	out := make([]PodInfo, 0, len(pods.Items))
	for _, p := range pods.Items {
		out = append(out, PodInfo{
			Name:         p.Name,
			Phase:        string(p.Status.Phase),
			Ready:        podReady(p),
			RestartCount: totalRestarts(p),
		})
	}
	return out, nil
}

func podReady(p corev1.Pod) bool {
	for _, cond := range p.Status.Conditions {
		if cond.Type == corev1.PodReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

func totalRestarts(p corev1.Pod) int32 {
	var n int32
	for _, cs := range p.Status.ContainerStatuses {
		n += cs.RestartCount
	}
	return n
}

const (
	scenarioKillOpenCost        = "kill-opencost"
	scenarioKillPrometheus      = "kill-prometheus"
	scenarioPartitionPrometheus = "partition-prometheus"
	scenarioLatencyPrometheus   = "latency-prometheus"

	// chaosCleanupPollInterval is how often CleanupChaos re-checks whether the
	// deleted chaos CR has fully disappeared (finalizer done == netem reverted).
	chaosCleanupPollInterval = 500 * time.Millisecond
)

var (
	podChaosResource = schema.GroupVersionResource{
		Group:    "chaos-mesh.org",
		Version:  "v1alpha1",
		Resource: "podchaos",
	}
	networkChaosResource = schema.GroupVersionResource{
		Group:    "chaos-mesh.org",
		Version:  "v1alpha1",
		Resource: "networkchaos",
	}
)

type ChaosScenario struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Engine      string `json:"engine"`
}

func SupportedChaosScenarios() []ChaosScenario {
	return []ChaosScenario{
		{
			ID:          scenarioKillOpenCost,
			Description: "Kill one allowlisted OpenCost pod",
			Engine:      "chaos-mesh",
		},
		{
			ID:          scenarioKillPrometheus,
			Description: "Kill one allowlisted Prometheus pod",
			Engine:      "chaos-mesh",
		},
		{
			ID:          scenarioPartitionPrometheus,
			Description: "Partition OpenCost from Prometheus",
			Engine:      "chaos-mesh",
		},
		{
			ID:          scenarioLatencyPrometheus,
			Description: "Add latency between OpenCost and Prometheus",
			Engine:      "chaos-mesh",
		},
	}
}

func (c *K8sClient) InjectChaos(ctx context.Context, scenario string) error {
	obj, resource, err := c.chaosObject(scenario)
	if err != nil {
		return err
	}

	_, err = c.dynamic.Resource(resource).Namespace(c.chaosNamespace).Create(ctx, obj, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("creating chaos scenario %q: %w", scenario, err)
	}
	return nil
}

// CleanupChaos deletes a chaos scenario and blocks until the resource is fully
// gone. Chaos Mesh reverts the injected tc/netem rules asynchronously via a
// finalizer, so a bare Delete returns while the rules are still live on the
// target pods. We delete with foreground propagation and poll until the object
// is NotFound, so cleanup only returns once the rules have actually drained.
// The caller's ctx bounds the wait.
func (c *K8sClient) CleanupChaos(ctx context.Context, scenario string) error {
	resource, err := chaosResourceForScenario(scenario)
	if err != nil {
		return err
	}

	name := chaosResourceName(scenario)
	ri := c.dynamic.Resource(resource).Namespace(c.chaosNamespace)

	foreground := metav1.DeletePropagationForeground
	err = ri.Delete(ctx, name, metav1.DeleteOptions{PropagationPolicy: &foreground})
	if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("deleting chaos scenario %q: %w", scenario, err)
	}

	ticker := time.NewTicker(chaosCleanupPollInterval)
	defer ticker.Stop()
	for {
		_, err := ri.Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("waiting for chaos scenario %q to clear: %w", scenario, err)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for chaos scenario %q to clear: %w", scenario, ctx.Err())
		case <-ticker.C:
		}
	}
}

func (c *K8sClient) chaosObject(scenario string) (*unstructured.Unstructured, schema.GroupVersionResource, error) {
	switch scenario {
	case scenarioKillOpenCost:
		labelSelectors, err := selectorToMap(c.selector)
		if err != nil {
			return nil, schema.GroupVersionResource{}, fmt.Errorf("OpenCost selector: %w", err)
		}
		return podKillObject(scenario, c.chaosNamespace, c.namespace, labelSelectors), podChaosResource, nil
	case scenarioKillPrometheus:
		labelSelectors, err := selectorToMap(c.prometheusSelector)
		if err != nil {
			return nil, schema.GroupVersionResource{}, fmt.Errorf("Prometheus selector: %w", err)
		}
		return podKillObject(scenario, c.chaosNamespace, c.prometheusNamespace, labelSelectors), podChaosResource, nil
	case scenarioPartitionPrometheus:
		return c.networkChaosObject(scenario, "partition")
	case scenarioLatencyPrometheus:
		return c.networkChaosObject(scenario, "delay")
	default:
		return nil, schema.GroupVersionResource{}, unknownScenarioError(scenario)
	}
}

func (c *K8sClient) networkChaosObject(scenario string, action string) (*unstructured.Unstructured, schema.GroupVersionResource, error) {
	openCostSelectors, err := selectorToMap(c.selector)
	if err != nil {
		return nil, schema.GroupVersionResource{}, fmt.Errorf("OpenCost selector: %w", err)
	}
	prometheusSelectors, err := selectorToMap(c.prometheusSelector)
	if err != nil {
		return nil, schema.GroupVersionResource{}, fmt.Errorf("Prometheus selector: %w", err)
	}

	spec := map[string]any{
		"action": action,
		"mode":   "all",
		"selector": map[string]any{
			"namespaces":     []any{c.namespace},
			"labelSelectors": openCostSelectors,
		},
		"direction": "to",
		"target": map[string]any{
			"mode": "all",
			"selector": map[string]any{
				"namespaces":     []any{c.prometheusNamespace},
				"labelSelectors": prometheusSelectors,
			},
		},
	}
	if action == "delay" {
		spec["delay"] = map[string]any{
			"latency":     "5s",
			"correlation": "0",
			"jitter":      "100ms",
		}
	}

	return chaosObject("NetworkChaos", scenario, c.chaosNamespace, spec), networkChaosResource, nil
}

func podKillObject(scenario string, chaosNamespace string, targetNamespace string, labelSelectors map[string]any) *unstructured.Unstructured {
	spec := map[string]any{
		"action": "pod-kill",
		"mode":   "one",
		"selector": map[string]any{
			"namespaces":     []any{targetNamespace},
			"labelSelectors": labelSelectors,
		},
	}
	return chaosObject("PodChaos", scenario, chaosNamespace, spec)
}

func chaosObject(kind string, scenario string, namespace string, spec map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "chaos-mesh.org/v1alpha1",
			"kind":       kind,
			"metadata": map[string]any{
				"name":      chaosResourceName(scenario),
				"namespace": namespace,
				"labels": map[string]any{
					"opencost.io/broker-owned": "true",
					"opencost.io/scenario":     scenario,
				},
			},
			"spec": spec,
		},
	}
}

func chaosResourceForScenario(scenario string) (schema.GroupVersionResource, error) {
	switch scenario {
	case scenarioKillOpenCost, scenarioKillPrometheus:
		return podChaosResource, nil
	case scenarioPartitionPrometheus, scenarioLatencyPrometheus:
		return networkChaosResource, nil
	default:
		return schema.GroupVersionResource{}, unknownScenarioError(scenario)
	}
}

func chaosResourceName(scenario string) string {
	return "opencost-" + scenario
}

type unknownScenarioError string

func (e unknownScenarioError) Error() string {
	return fmt.Sprintf("unknown chaos scenario %q", string(e))
}

func selectorToMap(selector string) (map[string]any, error) {
	out := map[string]any{}
	for _, part := range strings.Split(selector, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok || key == "" || value == "" || strings.ContainsAny(key, " !()<>") || strings.ContainsAny(value, " !()<>") {
			return nil, fmt.Errorf("only comma-separated key=value selectors are supported, got %q", selector)
		}
		out[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("selector must contain at least one key=value pair")
	}
	return out, nil
}
