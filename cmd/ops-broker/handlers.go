package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// chaosCleanupTimeout caps how long the DELETE handler waits for netem rules to
// drain. Kept under the broker client's 30s HTTP timeout so the broker returns a
// structured error instead of the caller's connection timing out.
const chaosCleanupTimeout = 25 * time.Second

// newMux wires the frozen HTTP contract. Each authed route is a named button;
// none accept a target from the caller — targets are fixed by broker config.
//
//	GET  /healthz   — unauthenticated liveness
//	GET  /v1/pods   — list OpenCost pods (wait-for-ready)
//	GET  /v1/nodes  — trimmed node facts (asset ground truth)
//	GET  /v1/disks  — trimmed PV facts (asset ground truth)
//	GET  /v1/deployments/{name} — pinned deployment readiness (rollout status)
//	GET  /v1/logs   — trimmed pod logs (panic/error checks)
//	POST /v1/config — apply one allowlisted fixture config (assets)
//	DELETE /v1/config — remove one allowlisted fixture config (assets)
//	POST /v1/restart — trigger rolling restart of OpenCost
//	GET  /v1/chaos — list allowlisted chaos scenarios
//	POST /v1/chaos/{scenario} — inject one allowlisted chaos scenario
//	DELETE /v1/chaos/{scenario} — cleanup one allowlisted chaos scenario
func newMux(cfg Config, k8s *K8sClient) *http.ServeMux {
	mux := http.NewServeMux()

	// Unauthenticated liveness probe.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /v1/pods", requireToken(cfg.AuthToken,
		func(w http.ResponseWriter, r *http.Request) {
			pods, err := k8s.PodStatus(r.Context())
			if err != nil {
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"pods": pods})
		}))

	mux.HandleFunc("GET /v1/nodes", requireToken(cfg.AuthToken,
		func(w http.ResponseWriter, r *http.Request) {
			nodes, err := k8s.NodeFacts(r.Context())
			if err != nil {
				writeOpError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"nodes": nodes})
		}))

	mux.HandleFunc("GET /v1/disks", requireToken(cfg.AuthToken,
		func(w http.ResponseWriter, r *http.Request) {
			disks, err := k8s.DiskFacts(r.Context())
			if err != nil {
				writeOpError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"disks": disks})
		}))

	mux.HandleFunc("GET /v1/deployments/{name}", requireToken(cfg.AuthToken,
		func(w http.ResponseWriter, r *http.Request) {
			name := r.PathValue("name")
			namespace := r.URL.Query().Get("namespace")
			if namespace == "" {
				writeError(w, http.StatusBadRequest, "INVALID_PARAM", "namespace query parameter is required")
				return
			}
			info, err := k8s.DeploymentReadiness(r.Context(), name, namespace)
			if err != nil {
				writeOpError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, info)
		}))

	mux.HandleFunc("GET /v1/logs", requireToken(cfg.AuthToken,
		func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			namespace := q.Get("namespace")
			selector := q.Get("selector")
			if namespace == "" || selector == "" {
				writeError(w, http.StatusBadRequest, "INVALID_PARAM", "namespace and selector query parameters are required")
				return
			}
			tailLines := 0
			if raw := q.Get("tailLines"); raw != "" {
				n, err := strconv.Atoi(raw)
				if err != nil || n < 1 {
					writeError(w, http.StatusBadRequest, "INVALID_PARAM", "tailLines must be a positive integer")
					return
				}
				tailLines = n
			}
			lines, err := k8s.PodLogs(r.Context(), namespace, selector, q.Get("container"), tailLines)
			if err != nil {
				writeOpError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"lines": lines})
		}))

	mux.HandleFunc("POST /v1/config", requireToken(cfg.AuthToken,
		func(w http.ResponseWriter, r *http.Request) {
			req, err := decodeConfigRequest(r)
			if err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_PARAM", err.Error())
				return
			}
			if err := k8s.ApplyConfig(r.Context(), req.FixtureID); err != nil {
				writeOpError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"applied": true, "fixtureId": req.FixtureID})
		}))

	mux.HandleFunc("DELETE /v1/config", requireToken(cfg.AuthToken,
		func(w http.ResponseWriter, r *http.Request) {
			req, err := decodeConfigRequest(r)
			if err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_PARAM", err.Error())
				return
			}
			if err := k8s.DeleteConfig(r.Context(), req.FixtureID); err != nil {
				writeOpError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "fixtureId": req.FixtureID})
		}))

	mux.HandleFunc("POST /v1/restart", requireToken(cfg.AuthToken,
		func(w http.ResponseWriter, r *http.Request) {
			if err := k8s.RestartOpenCost(r.Context()); err != nil {
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "restart triggered"})
		}))

	mux.HandleFunc("GET /v1/chaos", requireToken(cfg.AuthToken,
		func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]any{"scenarios": SupportedChaosScenarios()})
		}))

	mux.HandleFunc("POST /v1/chaos/{scenario}", requireToken(cfg.AuthToken,
		func(w http.ResponseWriter, r *http.Request) {
			scenario := r.PathValue("scenario")
			if err := k8s.InjectChaos(r.Context(), scenario); err != nil {
				writeChaosError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"injected": true,
				"scenario": scenario,
			})
		}))

	mux.HandleFunc("DELETE /v1/chaos/{scenario}", requireToken(cfg.AuthToken,
		func(w http.ResponseWriter, r *http.Request) {
			scenario := r.PathValue("scenario")
			ctx, cancel := context.WithTimeout(r.Context(), chaosCleanupTimeout)
			defer cancel()
			if err := k8s.CleanupChaos(ctx, scenario); err != nil {
				writeChaosError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"deleted":  true,
				"scenario": scenario,
			})
		}))

	return mux
}

// configRequest is the JSON body of /v1/config: an allowlisted fixture ID only.
type configRequest struct {
	FixtureID string `json:"fixtureId"`
}

func decodeConfigRequest(r *http.Request) (configRequest, error) {
	var req configRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return configRequest{}, fmt.Errorf("invalid JSON body: %v", err)
	}
	if req.FixtureID == "" {
		return configRequest{}, fmt.Errorf("fixtureId is required")
	}
	return req, nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a structured broker error: {"error": msg, "code": code}.
func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"error": msg, "code": code})
}

// writeOpError maps a k8s-operation error to the frozen contract status codes:
// invalid input → 400, target outside the allowlist → 404, upstream cluster
// failure → 502.
func writeOpError(w http.ResponseWriter, err error) {
	switch err.(type) {
	case inputError:
		writeError(w, http.StatusBadRequest, "INVALID_PARAM", err.Error())
	case notAllowedError, unknownScenarioError:
		writeError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
	default:
		writeError(w, http.StatusBadGateway, "UPSTREAM", err.Error())
	}
}

func writeChaosError(w http.ResponseWriter, err error) {
	writeOpError(w, err)
}
