package main

import (
	"context"
	"encoding/json"
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
	logNamespaces       []string
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
		logNamespaces:       cfg.LogNamespaces,
	}, nil
}

// inputError signals caller-supplied input was malformed; handlers map it to 400.
type inputError string

func (e inputError) Error() string { return string(e) }

// notAllowedError signals the caller targeted something outside the broker's
// allowlist (e.g. a non-pinned deployment or namespace); handlers map it to 404.
type notAllowedError string

func (e notAllowedError) Error() string { return string(e) }

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

// DeploymentReadinessInfo is the trimmed deployment readiness view the broker
// returns — the `kubectl rollout status` analogue for the pinned deployment.
type DeploymentReadinessInfo struct {
	Name            string `json:"name"`
	Ready           bool   `json:"ready"`
	ReadyReplicas   int32  `json:"readyReplicas"`
	UpdatedReplicas int32  `json:"updatedReplicas"`
	DesiredReplicas int32  `json:"desiredReplicas"`
}

// DeploymentReadiness reports readiness for the pinned OpenCost deployment. The
// name and namespace must match the broker's fixed target; anything else is
// rejected rather than read, so the caller can never probe arbitrary workloads.
//
// Ready follows the same rules as `kubectl rollout status`: the controller has
// observed the latest spec (observedGeneration), every replica is the updated
// one (updatedReplicas == desired), no old replicas linger (replicas == desired),
// and all are ready. Checking only readyReplicas would report Ready mid-restart,
// while the outgoing pod is still serving.
func (c *K8sClient) DeploymentReadiness(ctx context.Context, name, namespace string) (DeploymentReadinessInfo, error) {
	if name != c.deploy || namespace != c.namespace {
		return DeploymentReadinessInfo{}, notAllowedError(fmt.Sprintf(
			"deployment %s/%s is not the pinned target", namespace, name))
	}

	deploy, err := c.client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return DeploymentReadinessInfo{}, fmt.Errorf("getting deployment %s/%s: %w", namespace, name, err)
	}

	desired := int32(1)
	if deploy.Spec.Replicas != nil {
		desired = *deploy.Spec.Replicas
	}
	st := deploy.Status
	ready := desired > 0 &&
		st.ObservedGeneration >= deploy.Generation &&
		st.UpdatedReplicas == desired &&
		st.Replicas == desired &&
		st.ReadyReplicas == desired
	return DeploymentReadinessInfo{
		Name:            deploy.Name,
		Ready:           ready,
		ReadyReplicas:   st.ReadyReplicas,
		UpdatedReplicas: st.UpdatedReplicas,
		DesiredReplicas: desired,
	}, nil
}

const (
	// defaultLogTailLines is returned when the caller omits tailLines.
	defaultLogTailLines = 200
	// maxLogTailLines caps how many lines a single /v1/logs call may return.
	maxLogTailLines = 2000
)

// PodLogs returns trimmed log lines for pods matching selector in an allowlisted
// namespace. The namespace must be allowlisted and the selector must be a valid
// key=value set; tailLines is clamped to [1, maxLogTailLines]. Only log lines are
// returned — never pod specs, env, or secrets.
func (c *K8sClient) PodLogs(ctx context.Context, namespace, selector, container string, tailLines int) ([]string, error) {
	if !c.logNamespaceAllowed(namespace) {
		return nil, notAllowedError(fmt.Sprintf("namespace %q is not allowlisted for logs", namespace))
	}
	if _, err := selectorToMap(selector); err != nil {
		return nil, inputError(fmt.Sprintf("log selector: %v", err))
	}
	if tailLines <= 0 {
		tailLines = defaultLogTailLines
	}
	if tailLines > maxLogTailLines {
		tailLines = maxLogTailLines
	}

	pods, err := c.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("listing pods for logs in %s: %w", namespace, err)
	}

	tail := int64(tailLines)
	lines := []string{}
	for _, p := range pods.Items {
		// The k8s API requires a container name when a pod has more than one
		// container. If the caller did not pin one, read every container so
		// multi-container pods (e.g. OpenCost + its UI sidecar) don't 502.
		containers := []string{container}
		if container == "" {
			containers = containers[:0]
			for _, c := range p.Spec.Containers {
				containers = append(containers, c.Name)
			}
		}
		for _, cn := range containers {
			opts := &corev1.PodLogOptions{TailLines: &tail}
			if cn != "" {
				opts.Container = cn
			}
			raw, err := c.client.CoreV1().Pods(namespace).GetLogs(p.Name, opts).DoRaw(ctx)
			if err != nil {
				return nil, fmt.Errorf("reading logs for pod %s/%s container %q: %w", namespace, p.Name, cn, err)
			}
			for _, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
				if line != "" {
					lines = append(lines, line)
				}
			}
		}
	}
	return lines, nil
}

func (c *K8sClient) logNamespaceAllowed(namespace string) bool {
	for _, ns := range c.logNamespaces {
		if ns == namespace {
			return true
		}
	}
	return false
}

// NodeInfo is the trimmed node view the broker returns for asset ground-truth
// tests — name and capacity only, never labels, addresses, or provider IDs.
type NodeInfo struct {
	Name string `json:"name"`
	CPU  string `json:"cpu"`
	RAM  string `json:"ram"`
}

// NodeFacts lists cluster nodes with their CPU/RAM capacity.
func (c *K8sClient) NodeFacts(ctx context.Context) ([]NodeInfo, error) {
	nodes, err := c.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}

	out := make([]NodeInfo, 0, len(nodes.Items))
	for _, n := range nodes.Items {
		out = append(out, NodeInfo{
			Name: n.Name,
			CPU:  n.Status.Capacity.Cpu().String(),
			RAM:  n.Status.Capacity.Memory().String(),
		})
	}
	return out, nil
}

// DiskInfo is the trimmed PV view the broker returns. It is PV-centric: the
// bound claim (namespace/name) comes from the PV's ClaimRef, so the broker never
// needs to list namespaced PVCs — keeping RBAC to cluster-scoped PVs only.
type DiskInfo struct {
	Name           string `json:"name"`
	Capacity       string `json:"capacity"`
	StorageClass   string `json:"storageClass"`
	Phase          string `json:"phase"`
	ClaimNamespace string `json:"claimNamespace,omitempty"`
	ClaimName      string `json:"claimName,omitempty"`
}

// DiskFacts lists persistent volumes with capacity, class, phase, and bound claim.
func (c *K8sClient) DiskFacts(ctx context.Context) ([]DiskInfo, error) {
	pvs, err := c.client.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing persistent volumes: %w", err)
	}

	out := make([]DiskInfo, 0, len(pvs.Items))
	for _, pv := range pvs.Items {
		info := DiskInfo{
			Name:         pv.Name,
			Capacity:     pv.Spec.Capacity.Storage().String(),
			StorageClass: pv.Spec.StorageClassName,
			Phase:        string(pv.Status.Phase),
		}
		if ref := pv.Spec.ClaimRef; ref != nil {
			info.ClaimNamespace = ref.Namespace
			info.ClaimName = ref.Name
		}
		out = append(out, info)
	}
	return out, nil
}

// configMapSnapshot captures the mutable parts of a ConfigMap the fixture
// overwrote, so cleanup can restore the original exactly.
type configMapSnapshot struct {
	Data        map[string]string `json:"data,omitempty"`
	BinaryData  map[string][]byte `json:"binaryData,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ApplyConfig applies an allowlisted fixture ConfigMap into the OpenCost
// namespace and restarts OpenCost so it reloads with the fixed config. The
// caller supplies only the fixture ID; the manifest is embedded in the broker
// image and the namespace is forced broker-side.
//
// If a ConfigMap already exists at the target name, its original contents are
// snapshotted into an annotation so DeleteConfig can restore them rather than
// leaving the cluster without its pricing config.
func (c *K8sClient) ApplyConfig(ctx context.Context, fixtureID string) error {
	cm, err := loadFixtureConfigMap(fixtureID)
	if err != nil {
		return err
	}
	cm.Namespace = c.namespace
	if cm.Annotations == nil {
		cm.Annotations = map[string]string{}
	}

	cms := c.client.CoreV1().ConfigMaps(c.namespace)
	existing, err := cms.Get(ctx, cm.Name, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		// Nothing pre-existed; no snapshot. Cleanup will delete the ConfigMap.
		if _, err := cms.Create(ctx, cm, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("creating fixture configmap %s/%s: %w", c.namespace, cm.Name, err)
		}
	case err != nil:
		return fmt.Errorf("checking fixture configmap %s/%s: %w", c.namespace, cm.Name, err)
	default:
		if existing.Labels[fixtureOwnerLabel] == fixtureID {
			// Re-apply: the existing object is already the fixture and carries the
			// ORIGINAL snapshot. Carry it forward — never snapshot the fixture
			// over itself.
			if snap, ok := existing.Annotations[fixtureSnapshotAnnotation]; ok {
				cm.Annotations[fixtureSnapshotAnnotation] = snap
			}
		} else {
			// First apply over a real ConfigMap: snapshot the original.
			snap, err := encodeConfigMapSnapshot(existing)
			if err != nil {
				return fmt.Errorf("snapshotting configmap %s/%s: %w", c.namespace, cm.Name, err)
			}
			cm.Annotations[fixtureSnapshotAnnotation] = snap
		}
		cm.ResourceVersion = existing.ResourceVersion
		if _, err := cms.Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("updating fixture configmap %s/%s: %w", c.namespace, cm.Name, err)
		}
	}

	if err := c.RestartOpenCost(ctx); err != nil {
		return fmt.Errorf("restarting OpenCost after applying fixture %q: %w", fixtureID, err)
	}
	return nil
}

// DeleteConfig reverses a previously applied fixture and restarts OpenCost. It
// only touches objects carrying the broker-owned label for this fixture, never
// an arbitrary resource. If the fixture overwrote a pre-existing ConfigMap, the
// original is restored from its snapshot; if the fixture created the ConfigMap,
// it is deleted. Cleaning up an already-absent fixture is a no-op.
func (c *K8sClient) DeleteConfig(ctx context.Context, fixtureID string) error {
	cm, err := loadFixtureConfigMap(fixtureID)
	if err != nil {
		return err
	}

	cms := c.client.CoreV1().ConfigMaps(c.namespace)
	existing, err := cms.Get(ctx, cm.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking fixture configmap %s/%s: %w", c.namespace, cm.Name, err)
	}
	if existing.Labels[fixtureOwnerLabel] != fixtureID {
		return notAllowedError(fmt.Sprintf("configmap %s/%s is not owned by fixture %q", c.namespace, cm.Name, fixtureID))
	}

	if snap, ok := existing.Annotations[fixtureSnapshotAnnotation]; ok {
		// The fixture overwrote a pre-existing ConfigMap — restore it exactly.
		restored, err := restoredConfigMap(existing, snap)
		if err != nil {
			return fmt.Errorf("decoding snapshot for configmap %s/%s: %w", c.namespace, cm.Name, err)
		}
		if _, err := cms.Update(ctx, restored, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("restoring configmap %s/%s: %w", c.namespace, cm.Name, err)
		}
	} else {
		// The fixture created the ConfigMap — remove it.
		if err := cms.Delete(ctx, cm.Name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("deleting fixture configmap %s/%s: %w", c.namespace, cm.Name, err)
		}
	}

	if err := c.RestartOpenCost(ctx); err != nil {
		return fmt.Errorf("restarting OpenCost after removing fixture %q: %w", fixtureID, err)
	}
	return nil
}

func encodeConfigMapSnapshot(cm *corev1.ConfigMap) (string, error) {
	raw, err := json.Marshal(configMapSnapshot{
		Data:        cm.Data,
		BinaryData:  cm.BinaryData,
		Labels:      cm.Labels,
		Annotations: cm.Annotations,
	})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// restoredConfigMap rebuilds the original ConfigMap from a snapshot, keeping the
// live object's identity (name/namespace/resourceVersion) so the Update replaces
// the fixture in place and strips the broker's label + snapshot annotation.
func restoredConfigMap(existing *corev1.ConfigMap, snapshot string) (*corev1.ConfigMap, error) {
	var snap configMapSnapshot
	if err := json.Unmarshal([]byte(snapshot), &snap); err != nil {
		return nil, err
	}
	restored := existing.DeepCopy()
	restored.Data = snap.Data
	restored.BinaryData = snap.BinaryData
	restored.Labels = snap.Labels
	restored.Annotations = snap.Annotations
	return restored, nil
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
