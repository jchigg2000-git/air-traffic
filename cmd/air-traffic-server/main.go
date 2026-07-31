// Command air-traffic-server boots the Air-Traffic control-plane API, the synthetic
// vendor surfaces, and the background ops-observation-batch/v1 emitter.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"air-traffic/internal/emitter"
	"air-traffic/internal/harness"
	"air-traffic/internal/policy"
	"air-traffic/internal/server"
	"air-traffic/internal/store"
)

func main() {
	addr := env("AIRTRAFFIC_ADDR", "127.0.0.1:8122")
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	st := store.New()
	app := server.New(st, log)

	// Shared key for the gateway spine routes (/api/gateway/leaks,
	// /enforcement, /patterns). Unset keeps the loopback-only dev posture.
	gatewayKey := env("AIRTRAFFIC_GATEWAY_KEY", "gwk-demo")
	spineKey := os.Getenv("AIRTRAFFIC_SPINE_KEY")
	app.SetSpineKey(spineKey)
	warnUnrotatedKeys(log, gatewayKey, spineKey)

	// The gateway harness engine: drives synthetic traffic through the
	// gateway and feeds the flywheel. Durable state (ratchet, corpus,
	// pattern pack) lives under AIRTRAFFIC_DATA_DIR. The Presidio URL feeds
	// the flywheel's raw-score probe (threshold-proposal evidence); if the
	// sidecar is down the probe degrades gracefully.
	hr, err := harness.NewRunner(st, log,
		env("AIRTRAFFIC_DATA_DIR", "data/harness"),
		gatewayKey,
		env("AIRTRAFFIC_PRESIDIO_URL", "http://127.0.0.1:8126"))
	if err != nil {
		log.Error("harness init failed", "error", err)
		os.Exit(1)
	}
	app.SetHarness(hr)

	emitCtx, stopEmit := context.WithCancel(context.Background())
	defer stopEmit()
	if env("AIRTRAFFIC_EMIT", "on") != "off" {
		interval := emitInterval()
		em := emitter.New(st, log, interval)
		em.AddTickHook(func(ts time.Time) { policy.RefreshDrift(st, ts) })
		em.Seed()
		policy.RefreshDrift(st, time.Now().UTC())
		go em.Run(emitCtx)
		log.Info("synthetic emitter running", "interval", interval.String())
	}

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           app.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("air-traffic listening", "addr", addr)
		errCh <- httpServer.ListenAndServe()
	}()

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-stopCh:
		log.Info("shutdown requested", "signal", sig.String())
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			log.Error("server failed", "error", err)
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

// devDefaultKeys are the throwaway values docker-compose falls back to so the
// demo comes up with one command. They are fine on a laptop and wrong
// anywhere else, so a boot that still carries one says so out loud.
var devDefaultKeys = map[string]bool{"gwk-demo": true, "spine-dev-insecure": true}

func warnUnrotatedKeys(log *slog.Logger, gatewayKey, spineKey string) {
	if devDefaultKeys[gatewayKey] {
		log.Warn("AIRTRAFFIC_GATEWAY_KEY is the dev default; rotate before any shared deployment (scripts/dev-env.sh)")
	}
	switch {
	case spineKey == "":
		log.Warn("AIRTRAFFIC_SPINE_KEY unset: /api/gateway/{leaks,enforcement,patterns} accept loopback callers only")
	case devDefaultKeys[spineKey]:
		log.Warn("AIRTRAFFIC_SPINE_KEY is the dev default; rotate before any shared deployment (scripts/dev-env.sh)")
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func emitInterval() time.Duration {
	if n, err := strconv.Atoi(os.Getenv("AIRTRAFFIC_EMIT_INTERVAL_SECONDS")); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return 5 * time.Second
}
