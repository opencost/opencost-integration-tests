package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestChaosScenariosEndpoint(t *testing.T) {
	mux := newMux(Config{AuthToken: "test-token"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/chaos", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var response struct {
		Scenarios []ChaosScenario `json:"scenarios"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Scenarios) != 4 {
		t.Fatalf("len(scenarios) = %d, want 4", len(response.Scenarios))
	}
}

func TestNodesEndpoint(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			},
		},
	}
	mux := newMux(Config{AuthToken: "test-token"}, newFakeTypedK8s(node))

	req := httptest.NewRequest(http.MethodGet, "/v1/nodes", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Nodes []NodeInfo `json:"nodes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(response.Nodes) != 1 || response.Nodes[0].Name != "node-1" {
		t.Fatalf("unexpected nodes: %+v", response.Nodes)
	}
}

func TestDisksEndpoint(t *testing.T) {
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "pv-1"},
		Spec: corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("5Gi")},
		},
		Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeAvailable},
	}
	mux := newMux(Config{AuthToken: "test-token"}, newFakeTypedK8s(pv))

	req := httptest.NewRequest(http.MethodGet, "/v1/disks", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Disks []DiskInfo `json:"disks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(response.Disks) != 1 || response.Disks[0].Name != "pv-1" {
		t.Fatalf("unexpected disks: %+v", response.Disks)
	}
}

func TestDeploymentReadinessEndpoint(t *testing.T) {
	replicas := int32(1)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "opencost", Namespace: "opencost"},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status:     appsv1.DeploymentStatus{ReadyReplicas: 1, UpdatedReplicas: 1, Replicas: 1},
	}
	mux := newMux(Config{AuthToken: "test-token"}, newFakeTypedK8s(deploy))

	req := httptest.NewRequest(http.MethodGet, "/v1/deployments/opencost?namespace=opencost", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var response DeploymentReadinessInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !response.Ready {
		t.Fatalf("expected ready, got %+v", response)
	}
}

func TestDeploymentReadinessEndpointRequiresNamespace(t *testing.T) {
	mux := newMux(Config{AuthToken: "test-token"}, newFakeTypedK8s())

	req := httptest.NewRequest(http.MethodGet, "/v1/deployments/opencost", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestLogsEndpoint(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opencost-abc",
			Namespace: "opencost",
			Labels:    map[string]string{"app.kubernetes.io/name": "opencost"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "opencost"}},
		},
	}
	mux := newMux(Config{AuthToken: "test-token"}, newFakeTypedK8s(pod))

	req := httptest.NewRequest(http.MethodGet, "/v1/logs?namespace=opencost&selector=app.kubernetes.io/name=opencost", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(response.Lines) == 0 {
		t.Fatalf("expected log lines, got none")
	}
}

func TestLogsEndpointRequiresSelector(t *testing.T) {
	mux := newMux(Config{AuthToken: "test-token"}, newFakeTypedK8s())

	req := httptest.NewRequest(http.MethodGet, "/v1/logs?namespace=opencost", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestConfigApplyEndpoint(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "opencost", Namespace: "opencost"},
	}
	mux := newMux(Config{AuthToken: "test-token"}, newFakeTypedK8s(deploy))

	req := httptest.NewRequest(http.MethodPost, "/v1/config", strings.NewReader(`{"fixtureId":"pricing-fixed-v1"}`))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if response["applied"] != true || response["fixtureId"] != "pricing-fixed-v1" {
		t.Fatalf("unexpected response: %v", response)
	}
}

func TestConfigEndpointRejectsMissingFixtureID(t *testing.T) {
	mux := newMux(Config{AuthToken: "test-token"}, newFakeTypedK8s())

	req := httptest.NewRequest(http.MethodPost, "/v1/config", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestConfigEndpointRejectsUnknownFixture(t *testing.T) {
	mux := newMux(Config{AuthToken: "test-token"}, newFakeTypedK8s())

	req := httptest.NewRequest(http.MethodPost, "/v1/config", strings.NewReader(`{"fixtureId":"nope"}`))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuthedEndpointRejectsMissingTokenWithJSON(t *testing.T) {
	mux := newMux(Config{AuthToken: "test-token"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/chaos", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rec.Body.String())
	}
	if response["error"] != "unauthorized" {
		t.Fatalf("error = %q, want unauthorized", response["error"])
	}
}
