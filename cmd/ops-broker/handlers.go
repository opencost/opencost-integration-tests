package main

import (
	"context"
	"encoding/json"
	"net/http"
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

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeChaosError(w http.ResponseWriter, err error) {
	if _, ok := err.(unknownScenarioError); ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
}
