package cluster

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientHealthzDoesNotSendAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathHealthz {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("Authorization header = %q, want empty", got)
		}
		writeJSON(t, w, healthResponse{Status: "ok"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	if err := client.Healthz(context.Background()); err != nil {
		t.Fatalf("Healthz() error = %v", err)
	}
}

func TestClientRestartOpenCostSendsBearerToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathRestart {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization header = %q, want bearer token", got)
		}
		writeJSON(t, w, restartResponse{Status: "restart triggered"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	if err := client.RestartOpenCost(context.Background()); err != nil {
		t.Fatalf("RestartOpenCost() error = %v", err)
	}
}

func TestClientPodsDecodesTrimmedPodList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathPods {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization header = %q, want bearer token", got)
		}
		writeJSON(t, w, podsResponse{
			Pods: []Pod{
				{Name: "opencost-abc", Phase: "Running", Ready: true, RestartCount: 1},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	pods, err := client.Pods(context.Background())
	if err != nil {
		t.Fatalf("Pods() error = %v", err)
	}
	if len(pods) != 1 {
		t.Fatalf("len(pods) = %d, want 1", len(pods))
	}
	if pods[0].Name != "opencost-abc" || pods[0].Phase != "Running" || !pods[0].Ready || pods[0].RestartCount != 1 {
		t.Fatalf("unexpected pod: %+v", pods[0])
	}
}

func TestClientChaosScenariosDecodesScenarioList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathChaos {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization header = %q, want bearer token", got)
		}
		writeJSON(t, w, chaosScenariosResponse{
			Scenarios: []ChaosScenario{
				{ID: "kill-opencost", Description: "Kill OpenCost", Engine: "chaos-mesh"},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	scenarios, err := client.ChaosScenarios(context.Background())
	if err != nil {
		t.Fatalf("ChaosScenarios() error = %v", err)
	}
	if len(scenarios) != 1 {
		t.Fatalf("len(scenarios) = %d, want 1", len(scenarios))
	}
	if scenarios[0].ID != "kill-opencost" || scenarios[0].Engine != "chaos-mesh" {
		t.Fatalf("unexpected scenario: %+v", scenarios[0])
	}
}

func TestClientInjectAndCleanupChaos(t *testing.T) {
	requests := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization header = %q, want bearer token", got)
		}

		requests[r.Method+" "+r.URL.Path]++
		switch {
		case r.Method == http.MethodPost && r.URL.Path == chaosScenarioPath("kill-opencost"):
			writeJSON(t, w, chaosInjectResponse{Injected: true, Scenario: "kill-opencost"})
		case r.Method == http.MethodDelete && r.URL.Path == chaosScenarioPath("kill-opencost"):
			writeJSON(t, w, chaosCleanupResponse{Deleted: true, Scenario: "kill-opencost"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	if err := client.InjectChaos(context.Background(), "kill-opencost"); err != nil {
		t.Fatalf("InjectChaos() error = %v", err)
	}
	if err := client.CleanupChaos(context.Background(), "kill-opencost"); err != nil {
		t.Fatalf("CleanupChaos() error = %v", err)
	}

	if requests[http.MethodPost+" "+chaosScenarioPath("kill-opencost")] != 1 {
		t.Fatalf("expected one inject request, got %d", requests[http.MethodPost+" "+chaosScenarioPath("kill-opencost")])
	}
	if requests[http.MethodDelete+" "+chaosScenarioPath("kill-opencost")] != 1 {
		t.Fatalf("expected one cleanup request, got %d", requests[http.MethodDelete+" "+chaosScenarioPath("kill-opencost")])
	}
}

func TestClientNodesDecodesTrimmedList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathNodes {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization header = %q, want bearer token", got)
		}
		writeJSON(t, w, nodesResponse{Nodes: []NodeInfo{{Name: "node-1", CPU: "4", RAM: "16Gi"}}})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	nodes, err := client.Nodes(context.Background())
	if err != nil {
		t.Fatalf("Nodes() error = %v", err)
	}
	if len(nodes) != 1 || nodes[0].Name != "node-1" || nodes[0].CPU != "4" || nodes[0].RAM != "16Gi" {
		t.Fatalf("unexpected nodes: %+v", nodes)
	}
}

func TestClientDisksDecodesTrimmedList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathDisks {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization header = %q, want bearer token", got)
		}
		writeJSON(t, w, disksResponse{Disks: []DiskInfo{
			{Name: "pv-1", Capacity: "10Gi", StorageClass: "standard", Phase: "Bound", ClaimNamespace: "opencost", ClaimName: "data"},
		}})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	disks, err := client.Disks(context.Background())
	if err != nil {
		t.Fatalf("Disks() error = %v", err)
	}
	if len(disks) != 1 || disks[0].Name != "pv-1" || disks[0].ClaimName != "data" {
		t.Fatalf("unexpected disks: %+v", disks)
	}
}

func TestClientApplyAndDeleteConfig(t *testing.T) {
	requests := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathConfig {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization header = %q, want bearer token", got)
		}

		var body ConfigRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body.FixtureID != "pricing-fixed-v1" {
			t.Fatalf("fixtureId = %q, want pricing-fixed-v1", body.FixtureID)
		}

		requests[r.Method]++
		switch r.Method {
		case http.MethodPost:
			writeJSON(t, w, configApplyResponse{Applied: true, FixtureID: body.FixtureID})
		case http.MethodDelete:
			writeJSON(t, w, configDeleteResponse{Deleted: true, FixtureID: body.FixtureID})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	if err := client.ApplyConfig(context.Background(), "pricing-fixed-v1"); err != nil {
		t.Fatalf("ApplyConfig() error = %v", err)
	}
	if err := client.DeleteConfig(context.Background(), "pricing-fixed-v1"); err != nil {
		t.Fatalf("DeleteConfig() error = %v", err)
	}
	if requests[http.MethodPost] != 1 || requests[http.MethodDelete] != 1 {
		t.Fatalf("expected one apply and one delete, got %v", requests)
	}
}

func TestClientDeploymentDecodesReadiness(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != deploymentPath("opencost") {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("namespace"); got != "opencost" {
			t.Fatalf("namespace = %q, want opencost", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization header = %q, want bearer token", got)
		}
		writeJSON(t, w, DeploymentReadiness{Name: "opencost", Ready: true, ReadyReplicas: 1, DesiredReplicas: 1})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	info, err := client.Deployment(context.Background(), "opencost", "opencost")
	if err != nil {
		t.Fatalf("Deployment() error = %v", err)
	}
	if !info.Ready || info.ReadyReplicas != 1 || info.DesiredReplicas != 1 {
		t.Fatalf("unexpected readiness: %+v", info)
	}
}

func TestClientLogsSendsQueryAndDecodesLines(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathLogs {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("namespace") != "opencost" || q.Get("selector") != "app=opencost" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		if q.Get("tailLines") != "50" {
			t.Fatalf("tailLines = %q, want 50", q.Get("tailLines"))
		}
		writeJSON(t, w, logsResponse{Lines: []string{"line 1", "panic: boom"}})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	lines, err := client.Logs(context.Background(), LogsRequest{
		Namespace: "opencost",
		Selector:  "app=opencost",
		TailLines: 50,
	})
	if err != nil {
		t.Fatalf("Logs() error = %v", err)
	}
	if len(lines) != 2 || lines[1] != "panic: boom" {
		t.Fatalf("unexpected lines: %+v", lines)
	}
}

func TestWaitForDeploymentReady(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		ready := requests >= 2
		replicas := 1
		readyReplicas := 0
		if ready {
			readyReplicas = 1
		}
		writeJSON(t, w, DeploymentReadiness{
			Name:            "opencost",
			Ready:           ready,
			ReadyReplicas:   readyReplicas,
			DesiredReplicas: replicas,
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	info, err := client.WaitForDeploymentReady(ctx, "opencost", "opencost", time.Millisecond)
	if err != nil {
		t.Fatalf("WaitForDeploymentReady() error = %v", err)
	}
	if !info.Ready {
		t.Fatalf("info = %+v, want ready", info)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

func TestClientReportsBrokerJSONError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		writeJSON(t, w, brokerError{Error: "listing pods: unavailable"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	_, err := client.Pods(context.Background())
	if err == nil {
		t.Fatal("Pods() error = nil, want broker error")
	}
}

func TestNewClientFromEnv(t *testing.T) {
	t.Setenv(EnvBrokerURL, "http://broker.example.test/")
	t.Setenv(EnvBrokerToken, "test-token")

	client, err := NewClientFromEnv()
	if err != nil {
		t.Fatalf("NewClientFromEnv() error = %v", err)
	}
	if client.baseURL != "http://broker.example.test" {
		t.Fatalf("baseURL = %q, want trimmed URL", client.baseURL)
	}
	if client.token != "test-token" {
		t.Fatalf("token = %q, want test-token", client.token)
	}
}

func TestWaitForOpenCostReady(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			writeJSON(t, w, podsResponse{
				Pods: []Pod{{Name: "opencost-abc", Phase: "Running", Ready: false}},
			})
			return
		}
		writeJSON(t, w, podsResponse{
			Pods: []Pod{{Name: "opencost-abc", Phase: "Running", Ready: true}},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	pods, err := client.WaitForOpenCostReady(ctx, time.Millisecond)
	if err != nil {
		t.Fatalf("WaitForOpenCostReady() error = %v", err)
	}
	if len(pods) != 1 || !pods[0].Ready {
		t.Fatalf("pods = %+v, want one ready pod", pods)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("write JSON: %v", err)
	}
}
