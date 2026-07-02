// Command air-traffic-gateway boots the inference-gateway data plane: a
// vendor-dialect reverse proxy with inline PII detection. It is deliberately
// a separate process from air-traffic-server — it scales with inference
// traffic, the control plane scales with governance reads (build plan §2).
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"air-traffic/internal/gateway"
	"air-traffic/internal/gateway/config"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		log.Error("config rejected", "error", err)
		os.Exit(1)
	}
	gw, err := gateway.New(cfg, log)
	if err != nil {
		log.Error("gateway init failed", "error", err)
		os.Exit(1)
	}

	spineCtx, stopSpine := context.WithCancel(context.Background())
	defer stopSpine()
	go gw.RunSpine(spineCtx)      // observations + reports up, heartbeat out
	go gw.RunPolicyPull(spineCtx) // policy + pattern pack down

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           gw.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("air-traffic-gateway listening", "addr", cfg.ListenAddr, "detectors", cfg.Detectors, "fail_mode", cfg.FailMode)
		errCh <- httpServer.ListenAndServe()
	}()

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-stopCh:
		log.Info("shutdown requested", "signal", sig.String())
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			log.Error("gateway failed", "error", err)
			os.Exit(1)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
}
