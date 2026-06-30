package main

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	kfake "k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func newFakeK8s(objs ...runtime.Object) (*K8sClient, *fake.FakeDynamicClient) {
	scheme := runtime.NewScheme()
	gvrToListKind := map[schema.GroupVersionResource]string{
		networkChaosResource: "NetworkChaosList",
		podChaosResource:     "PodChaosList",
	}
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, objs...)
	return &K8sClient{dynamic: dyn, chaosNamespace: "opencost"}, dyn
}

// CleanupChaos returns only after the chaos CR fully disappears (finalizer done).
func TestCleanupChaosWaitsForDrain(t *testing.T) {
	c, dyn := newFakeK8s()

	// Delete succeeds; the CR then lingers (in-progress finalizer) and only
	// clears on the 3rd Get, mimicking netem rules draining asynchronously.
	dyn.PrependReactor("delete", "networkchaos", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	gets := 0
	obj := chaosObject("NetworkChaos", scenarioLatencyPrometheus, "opencost", map[string]any{})
	dyn.PrependReactor("get", "networkchaos", func(ktesting.Action) (bool, runtime.Object, error) {
		gets++
		if gets >= 3 {
			return true, nil, apierrors.NewNotFound(networkChaosResource.GroupResource(), "x")
		}
		return true, obj, nil // still present
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.CleanupChaos(ctx, scenarioLatencyPrometheus); err != nil {
		t.Fatalf("CleanupChaos: %v", err)
	}
	if gets < 3 {
		t.Fatalf("expected to poll until gone, got %d gets", gets)
	}
}

// CleanupChaos errors (not hangs) if the resource never drains within ctx.
func TestCleanupChaosTimesOut(t *testing.T) {
	c, dyn := newFakeK8s()
	dyn.PrependReactor("delete", "networkchaos", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	obj := chaosObject("NetworkChaos", scenarioLatencyPrometheus, "opencost", map[string]any{})
	dyn.PrependReactor("get", "networkchaos", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, obj, nil // never clears
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	if err := c.CleanupChaos(ctx, scenarioLatencyPrometheus); err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

// Deleting an already-absent scenario is a no-op.
func TestCleanupChaosNoopWhenAbsent(t *testing.T) {
	c, _ := newFakeK8s()
	if err := c.CleanupChaos(context.Background(), scenarioLatencyPrometheus); err != nil {
		t.Fatalf("expected nil for absent scenario, got %v", err)
	}
}

func newFakeTypedK8s(objs ...runtime.Object) *K8sClient {
	return &K8sClient{
		client:        kfake.NewSimpleClientset(objs...),
		namespace:     "opencost",
		deploy:        "opencost",
		selector:      "app.kubernetes.io/name=opencost",
		logNamespaces: []string{"opencost"},
	}
}

func TestDeploymentReadinessReportsReady(t *testing.T) {
	replicas := int32(2)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "opencost", Namespace: "opencost"},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status:     appsv1.DeploymentStatus{ReadyReplicas: 2, UpdatedReplicas: 2, Replicas: 2},
	}
	c := newFakeTypedK8s(deploy)

	info, err := c.DeploymentReadiness(context.Background(), "opencost", "opencost")
	if err != nil {
		t.Fatalf("DeploymentReadiness: %v", err)
	}
	if !info.Ready || info.ReadyReplicas != 2 || info.DesiredReplicas != 2 {
		t.Fatalf("unexpected readiness: %+v", info)
	}
}

func TestDeploymentReadinessNotReadyWhenReplicasMissing(t *testing.T) {
	replicas := int32(1)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "opencost", Namespace: "opencost"},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status:     appsv1.DeploymentStatus{ReadyReplicas: 0},
	}
	c := newFakeTypedK8s(deploy)

	info, err := c.DeploymentReadiness(context.Background(), "opencost", "opencost")
	if err != nil {
		t.Fatalf("DeploymentReadiness: %v", err)
	}
	if info.Ready {
		t.Fatalf("expected not ready, got %+v", info)
	}
}

// Mid-restart, the outgoing pod keeps readyReplicas == desired while the new
// ReplicaSet has not finished rolling out. DeploymentReadiness must report NOT
// ready until updatedReplicas/replicas converge (the bug a live restart exposed).
func TestDeploymentReadinessNotReadyMidRollout(t *testing.T) {
	replicas := int32(1)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "opencost", Namespace: "opencost", Generation: 2},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 2,
			ReadyReplicas:      1, // old pod still ready
			Replicas:           2, // old + new during surge
			UpdatedReplicas:    0, // new pod not ready yet
		},
	}
	c := newFakeTypedK8s(deploy)

	info, err := c.DeploymentReadiness(context.Background(), "opencost", "opencost")
	if err != nil {
		t.Fatalf("DeploymentReadiness: %v", err)
	}
	if info.Ready {
		t.Fatalf("expected NOT ready mid-rollout, got %+v", info)
	}
}

func TestDeploymentReadinessRejectsNonPinnedTarget(t *testing.T) {
	c := newFakeTypedK8s()
	_, err := c.DeploymentReadiness(context.Background(), "other", "opencost")
	if _, ok := err.(notAllowedError); !ok {
		t.Fatalf("expected notAllowedError, got %v", err)
	}
}

func TestPodLogsReturnsLines(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opencost-abc",
			Namespace: "opencost",
			Labels:    map[string]string{"app.kubernetes.io/name": "opencost"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "opencost"}, {Name: "opencost-ui"}},
		},
	}
	c := newFakeTypedK8s(pod)

	lines, err := c.PodLogs(context.Background(), "opencost", "app.kubernetes.io/name=opencost", "", 0)
	if err != nil {
		t.Fatalf("PodLogs: %v", err)
	}
	if len(lines) == 0 {
		t.Fatalf("expected at least one log line, got none")
	}
}

func TestPodLogsRejectsNonAllowlistedNamespace(t *testing.T) {
	c := newFakeTypedK8s()
	_, err := c.PodLogs(context.Background(), "kube-system", "app=x", "", 0)
	if _, ok := err.(notAllowedError); !ok {
		t.Fatalf("expected notAllowedError, got %v", err)
	}
}

func TestPodLogsRejectsInvalidSelector(t *testing.T) {
	c := newFakeTypedK8s()
	_, err := c.PodLogs(context.Background(), "opencost", "not a selector", "", 0)
	if _, ok := err.(inputError); !ok {
		t.Fatalf("expected inputError, got %v", err)
	}
}

func TestNodeFactsReturnsTrimmedCapacity(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("16Gi"),
			},
		},
	}
	c := newFakeTypedK8s(node)

	nodes, err := c.NodeFacts(context.Background())
	if err != nil {
		t.Fatalf("NodeFacts: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("len(nodes) = %d, want 1", len(nodes))
	}
	if nodes[0].Name != "node-1" || nodes[0].CPU != "4" || nodes[0].RAM != "16Gi" {
		t.Fatalf("unexpected node: %+v", nodes[0])
	}
}

func TestApplyConfigCreatesConfigMapAndRestarts(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "opencost", Namespace: "opencost"},
	}
	c := newFakeTypedK8s(deploy)

	if err := c.ApplyConfig(context.Background(), "pricing-fixed-v1"); err != nil {
		t.Fatalf("ApplyConfig: %v", err)
	}

	// OpenCost reads custom pricing from the ConfigMap named "custom-pricing-model"
	// (PRICING_CONFIGMAP_NAME), with flat field keys (no default.json wrapper).
	cm, err := c.client.CoreV1().ConfigMaps("opencost").Get(context.Background(), "custom-pricing-model", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected fixture ConfigMap to exist: %v", err)
	}
	if cm.Labels[fixtureOwnerLabel] != "pricing-fixed-v1" {
		t.Fatalf("expected broker-owned label, got %v", cm.Labels)
	}
	if cm.Data["CPU"] == "" || cm.Data["provider"] != "custom" {
		t.Fatalf("expected flat custom-pricing fields, got %v", cm.Data)
	}
	// Nothing pre-existed, so there is no snapshot to restore on cleanup.
	if _, ok := cm.Annotations[fixtureSnapshotAnnotation]; ok {
		t.Fatalf("did not expect a snapshot annotation when the ConfigMap was created fresh")
	}

	// Restart must have stamped the deployment's pod-template annotation.
	got, err := c.client.AppsV1().Deployments("opencost").Get(context.Background(), "opencost", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if got.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] == "" {
		t.Fatalf("expected restart annotation after ApplyConfig")
	}
}

func TestApplyConfigRejectsUnknownFixture(t *testing.T) {
	c := newFakeTypedK8s()
	err := c.ApplyConfig(context.Background(), "does-not-exist")
	if _, ok := err.(notAllowedError); !ok {
		t.Fatalf("expected notAllowedError, got %v", err)
	}
}

func TestDeleteConfigRemovesOwnedConfigMap(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "opencost", Namespace: "opencost"},
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "custom-pricing-model",
			Namespace: "opencost",
			Labels:    map[string]string{fixtureOwnerLabel: "pricing-fixed-v1"},
		},
	}
	c := newFakeTypedK8s(deploy, cm)

	if err := c.DeleteConfig(context.Background(), "pricing-fixed-v1"); err != nil {
		t.Fatalf("DeleteConfig: %v", err)
	}
	_, err := c.client.CoreV1().ConfigMaps("opencost").Get(context.Background(), "custom-pricing-model", metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected fixture ConfigMap deleted, got err=%v", err)
	}
}

func TestDeleteConfigNoopWhenAbsent(t *testing.T) {
	c := newFakeTypedK8s()
	if err := c.DeleteConfig(context.Background(), "pricing-fixed-v1"); err != nil {
		t.Fatalf("expected nil for absent fixture, got %v", err)
	}
}

func TestDeleteConfigRejectsUnownedConfigMap(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "custom-pricing-model", Namespace: "opencost"},
	}
	c := newFakeTypedK8s(cm)

	err := c.DeleteConfig(context.Background(), "pricing-fixed-v1")
	if _, ok := err.(notAllowedError); !ok {
		t.Fatalf("expected notAllowedError for unowned ConfigMap, got %v", err)
	}
}

// originalPricingCM is a pre-existing, non-broker-owned custom pricing ConfigMap.
func originalPricingCM() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "custom-pricing-model",
			Namespace:   "opencost",
			Labels:      map[string]string{"app.kubernetes.io/managed-by": "Helm"},
			Annotations: map[string]string{"meta.helm.sh/release-name": "opencost"},
		},
		Data: map[string]string{"CPU": "9.9", "RAM": "9.9", "provider": "aws"},
	}
}

func TestApplyConfigSnapshotsExistingConfigMap(t *testing.T) {
	deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "opencost", Namespace: "opencost"}}
	c := newFakeTypedK8s(deploy, originalPricingCM())

	if err := c.ApplyConfig(context.Background(), "pricing-fixed-v1"); err != nil {
		t.Fatalf("ApplyConfig: %v", err)
	}

	cm, err := c.client.CoreV1().ConfigMaps("opencost").Get(context.Background(), "custom-pricing-model", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get configmap: %v", err)
	}
	// Fixture values now in place...
	if cm.Data["CPU"] != "1.0" || cm.Labels[fixtureOwnerLabel] != "pricing-fixed-v1" {
		t.Fatalf("expected fixture content, got data=%v labels=%v", cm.Data, cm.Labels)
	}
	// ...and the original snapshotted for restore.
	snap, ok := cm.Annotations[fixtureSnapshotAnnotation]
	if !ok {
		t.Fatalf("expected a snapshot annotation of the original ConfigMap")
	}
	restored, err := restoredConfigMap(cm, snap)
	if err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if restored.Data["CPU"] != "9.9" || restored.Data["provider"] != "aws" {
		t.Fatalf("snapshot did not capture the original data: %v", restored.Data)
	}
}

func TestDeleteConfigRestoresSnapshot(t *testing.T) {
	deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "opencost", Namespace: "opencost"}}
	c := newFakeTypedK8s(deploy, originalPricingCM())

	// Apply (snapshots original) then delete (must restore it).
	if err := c.ApplyConfig(context.Background(), "pricing-fixed-v1"); err != nil {
		t.Fatalf("ApplyConfig: %v", err)
	}
	if err := c.DeleteConfig(context.Background(), "pricing-fixed-v1"); err != nil {
		t.Fatalf("DeleteConfig: %v", err)
	}

	cm, err := c.client.CoreV1().ConfigMaps("opencost").Get(context.Background(), "custom-pricing-model", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected ConfigMap to still exist after restore: %v", err)
	}
	if cm.Data["CPU"] != "9.9" || cm.Data["provider"] != "aws" {
		t.Fatalf("expected original data restored, got %v", cm.Data)
	}
	if _, ok := cm.Labels[fixtureOwnerLabel]; ok {
		t.Fatalf("broker label should be gone after restore, got %v", cm.Labels)
	}
	if _, ok := cm.Annotations[fixtureSnapshotAnnotation]; ok {
		t.Fatalf("snapshot annotation should be gone after restore, got %v", cm.Annotations)
	}
	if cm.Labels["app.kubernetes.io/managed-by"] != "Helm" {
		t.Fatalf("expected original labels restored, got %v", cm.Labels)
	}
}

func TestApplyConfigReapplyPreservesOriginalSnapshot(t *testing.T) {
	deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "opencost", Namespace: "opencost"}}
	c := newFakeTypedK8s(deploy, originalPricingCM())

	// Apply twice; the second apply must not snapshot the fixture over itself.
	if err := c.ApplyConfig(context.Background(), "pricing-fixed-v1"); err != nil {
		t.Fatalf("ApplyConfig #1: %v", err)
	}
	if err := c.ApplyConfig(context.Background(), "pricing-fixed-v1"); err != nil {
		t.Fatalf("ApplyConfig #2: %v", err)
	}

	cm, err := c.client.CoreV1().ConfigMaps("opencost").Get(context.Background(), "custom-pricing-model", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get configmap: %v", err)
	}
	restored, err := restoredConfigMap(cm, cm.Annotations[fixtureSnapshotAnnotation])
	if err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if restored.Data["CPU"] != "9.9" {
		t.Fatalf("re-apply clobbered the original snapshot: %v", restored.Data)
	}
}

func TestDiskFactsReturnsPVsWithClaim(t *testing.T) {
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "pv-1"},
		Spec: corev1.PersistentVolumeSpec{
			Capacity:         corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")},
			StorageClassName: "standard",
			ClaimRef:         &corev1.ObjectReference{Namespace: "opencost", Name: "data-claim"},
		},
		Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound},
	}
	c := newFakeTypedK8s(pv)

	disks, err := c.DiskFacts(context.Background())
	if err != nil {
		t.Fatalf("DiskFacts: %v", err)
	}
	if len(disks) != 1 {
		t.Fatalf("len(disks) = %d, want 1", len(disks))
	}
	d := disks[0]
	if d.Name != "pv-1" || d.Capacity != "10Gi" || d.StorageClass != "standard" ||
		d.Phase != "Bound" || d.ClaimNamespace != "opencost" || d.ClaimName != "data-claim" {
		t.Fatalf("unexpected disk: %+v", d)
	}
}
