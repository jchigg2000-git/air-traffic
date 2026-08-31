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
	"strings"
	"syscall"
	"time"

	"github.com/jchigg2000-git/air-traffic/internal/gateway"
	"github.com/jchigg2000-git/air-traffic/internal/gateway/config"
	"github.com/jchigg2000-git/air-traffic/internal/gateway/credbroker"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		log.Error("config rejected", "error", err)
		os.Exit(1)
	}
	warnUnrotatedKeys(log, cfg)

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

// devDefaultKeys are the throwaway values docker-compose falls back to so the
// demo comes up with one command. The control plane says so out loud when it
// still carries one (cmd/air-traffic-server/main.go); the data plane holds the
// other half of the same two secrets, so it says so too.
var devDefaultKeys = map[string]bool{"gwk-demo": true, "spine-dev-insecure": true}

// warnUnrotatedKeys resolves the two references that can carry a compose
// default and names the one still in use. Resolution failures are not reported
// here: an unusable client-key ref is fatal in gateway.New, and a spine ref
// that resolves to nothing is the loopback-only posture New already warns
// about.
func warnUnrotatedKeys(log *slog.Logger, cfg config.Config) {
	creds := credbroker.New()
	if raw, err := creds.Resolve(cfg.ClientKeysRef); err == nil {
		for _, k := range strings.Split(raw, ",") {
			if k = strings.TrimSpace(k); devDefaultKeys[k] {
				log.Warn("client key is the dev default; rotate before any shared deployment (scripts/dev-env.sh)",
					"ref", cfg.ClientKeysRef, "key", k)
			}
		}
	}
	if k, err := creds.Resolve(cfg.ControlPlaneKeyRef); err == nil && devDefaultKeys[k] {
		log.Warn("control-plane spine key is the dev default; rotate before any shared deployment (scripts/dev-env.sh)",
			"ref", cfg.ControlPlaneKeyRef, "key", k)
	}
}
