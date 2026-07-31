package gateway

// The upward half of the spine contract (build plan §4.2): observations and
// request reports push to the control plane, enforcement heartbeats announce
// what this gateway actually enforces. Fire-and-forget with re-queue — a down
// control plane never blocks the data plane.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"air-traffic/internal/model"
)

const heartbeatInterval = 15 * time.Second

// RunSpine drives the push loops until ctx is cancelled. A failed heartbeat
// retries on a short fuse instead of waiting a full interval — at boot the
// control plane may come up seconds after the gateway, and proxy_enforced
// should flip as soon as it does.
func (s *Server) RunSpine(ctx context.Context) {
	obsT := time.NewTicker(s.cfg.ObsPushInterval)
	defer obsT.Stop()
	hb := time.NewTimer(0) // announce immediately
	defer hb.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-obsT.C:
			s.pushObservations(ctx)
			s.pushReports(ctx)
		case <-hb.C:
			delay := heartbeatInterval
			if err := s.pushHeartbeat(ctx); err != nil {
				delay = 2 * time.Second
			}
			hb.Reset(delay)
		}
	}
}

func (s *Server) gatewayID() string { return "gw@" + s.cfg.ListenAddr }

func (s *Server) pushObservations(ctx context.Context) {
	snap := s.metrics.drain()
	if snap.Requests == 0 {
		return
	}
	dims := map[string]any{}
	obs := []any{
		gwObs("gw_requests", snap.Requests, "count", dims),
		gwObs("gw_block_rate", ratio(snap.Blocked, snap.Requests), "ratio", dims),
		gwObs("gw_masked", snap.Masked, "count", dims),
		gwObs("gw_fail_mode_trips", snap.FailTrips, "count", dims),
		gwObs("gw_added_latency_ms_p50", snap.AddedP50, "ms", dims),
		gwObs("gw_added_latency_ms_p95", snap.AddedP95, "ms", dims),
		gwObs("gw_added_latency_ms_p99", snap.AddedP99, "ms", dims),
		gwObs("gw_latency_ms_p95", snap.TotalP95, "ms", dims),
		gwObs("tokens_in", snap.TokensIn, "tokens", dims),
		gwObs("tokens_out", snap.TokensOut, "tokens", dims),
	}
	for typ, n := range snap.RedactionsByType {
		obs = append(obs, gwObs("gw_redactions", n, "count", map[string]any{"pii_type": typ}))
	}
	batch := map[string]any{
		"contract": model.ObservationContract,
		"batch_id": model.NewUUID(),
		"connector": map[string]any{
			"type":        "gateway",
			"instance":    s.gatewayID(),
			"api_version": "gateway/v0",
		},
		"collected_at": time.Now().UTC().Format(time.RFC3339),
		"window": map[string]any{
			"from": time.Now().Add(-s.cfg.ObsPushInterval).UTC().Format(time.RFC3339),
			"to":   time.Now().UTC().Format(time.RFC3339),
		},
		"cursor":       map[string]any{"input": nil, "output": nil},
		"complete":     true,
		"observations": obs,
		"errors":       []any{},
	}
	if err := s.postJSON(ctx, "/api/observations", batch); err != nil {
		s.log.Warn("observation push failed", "error", err)
	}
}

// gwObs shapes one observation entry with the gateway's standard dimensions.
func gwObs(name string, value any, unit string, extraDims map[string]any) any {
	return model.Obs("metric", name, value, unit, "green", "info",
		model.PlaneObservability, "anthropic", model.DispProxyEnforced, extraDims, "gateway", "")
}

func ratio(num, den int64) float64 {
	if den == 0 {
		return 0
	}
	return float64(num) / float64(den)
}

func (s *Server) pushReports(ctx context.Context) {
	reports := s.audits.drain()
	if len(reports) == 0 {
		return
	}
	for start := 0; start < len(reports); start += 1000 {
		end := min(start+1000, len(reports))
		chunk := reports[start:end]
		if err := s.postJSON(ctx, "/api/gateway/leaks", map[string]any{"reports": chunk}); err != nil {
			s.log.Warn("report push failed; requeueing", "error", err, "count", len(reports)-start)
			s.audits.requeue(reports[start:])
			return
		}
	}
}

func (s *Server) pushHeartbeat(ctx context.Context) error {
	action := s.currentAction()
	vendors := map[string][]string{}
	// detect-only is monitoring, not enforcement — the honesty model demands
	// the heartbeat only claim capabilities that actually gate traffic.
	if action == actionMask || action == actionBlock {
		vendors["anthropic"] = []string{"pii_redaction"}
	}
	rep := model.EnforcementReport{
		GatewayID: s.gatewayID(),
		BaseURL:   s.cfg.AdvertiseURL,
		Action:    action,
		Detectors: s.cfg.Detectors,
		Vendors:   vendors,
	}
	if err := s.postJSON(ctx, "/api/gateway/enforcement", rep); err != nil {
		s.log.Warn("enforcement heartbeat failed", "error", err)
		return err
	}
	return nil
}

// authSpine stamps the shared spine key on a control-plane request. A gateway
// with no key configured sends none and is accepted only over loopback.
func (s *Server) authSpine(req *http.Request) {
	if s.spineKey == "" {
		return
	}
	req.Header.Set("Authorization", "Bearer "+s.spineKey)
}

func (s *Server) postJSON(ctx context.Context, path string, body any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.ControlPlaneURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	s.authSpine(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s returned %d", path, resp.StatusCode)
	}
	return nil
}
