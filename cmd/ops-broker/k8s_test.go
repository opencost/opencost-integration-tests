package main

import (
	"context"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
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
