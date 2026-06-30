package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/opencost/opencost-integration-tests/pkg/log"
)

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	k8s, err := NewK8sClient(cfg)
	if err != nil {
		log.Fatalf("k8s: %v", err)
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           newMux(cfg, k8s),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Infof("ops-broker listening on %s (namespace=%s)", cfg.Addr, cfg.Namespace)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
