package gateway

// /healthz and /readyz. docker-compose gates the gateway container's health
// on GET /readyz, so its 200/503 contract and reasons list are pinned here.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

type healthBody struct {
	Status  string   `json:"status"`
	Reasons []string `json:"reasons"`
}

func getHealth(t *testing.T, h http.Handler, method, path string) (*httptest.ResponseRecorder, healthBody) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	var body healthBody
	if rec.Code == http.StatusOK || rec.Code == http.StatusServiceUnavailable {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s %s: decode %q: %v", method, path, rec.Body.String(), err)
		}
	}
	return rec, body
}

func TestHealthzOK(t *testing.T) {
	gw := newTestGatewayServer(t, "http://upstream.invalid", discardLogger())
	rec, body := getHealth(t, gw.Routes(), http.MethodGet, "/healthz")
	if rec.Code != http.StatusOK || body.Status != "ok" {
		t.Fatalf("GET /healthz = %d %+v, want 200 ok", rec.Code, body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestHealthzRejectsPost(t *testing.T) {
	gw := newTestGatewayServer(t, "http://upstream.invalid", discardLogger())
	rec, _ := getHealth(t, gw.Routes(), http.MethodPost, "/healthz")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /healthz = %d, want 405", rec.Code)
	}
}

func TestReadyzReadyWhenConfigured(t *testing.T) {
	gw := newTestGatewayServer(t, "http://upstream.invalid", discardLogger())
	rec, body := getHealth(t, gw.Routes(), http.MethodGet, "/readyz")
	if rec.Code != http.StatusOK || body.Status != "ready" || len(body.Reasons) != 0 {
		t.Fatalf("GET /readyz = %d %+v, want 200 ready", rec.Code, body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestReadyzNotReadyWithoutUpstreams(t *testing.T) {
	gw := newTestGatewayServer(t, "http://upstream.invalid", discardLogger())
	gw.cfg.Upstreams = nil
	rec, body := getHealth(t, gw.Routes(), http.MethodGet, "/readyz")
	if rec.Code != http.StatusServiceUnavailable || body.Status != "not_ready" {
		t.Fatalf("GET /readyz = %d %+v, want 503 not_ready", rec.Code, body)
	}
	if !slices.Contains(body.Reasons, "no upstreams configured") {
		t.Errorf("reasons = %q, want \"no upstreams configured\"", body.Reasons)
	}
	if slices.Contains(body.Reasons, "no client keys resolved") {
		t.Errorf("reasons = %q, client keys are resolved", body.Reasons)
	}
}

// New refuses zero client keys, so the empty-keys server is built directly.
func TestReadyzNotReadyWithoutClientKeys(t *testing.T) {
	gw := newTestGatewayServer(t, "http://upstream.invalid", discardLogger())
	s := &Server{cfg: gw.cfg, log: discardLogger(), clientKeys: map[string]struct{}{}, metrics: newRouteMetrics()}
	rec, body := getHealth(t, s.Routes(), http.MethodGet, "/readyz")
	if rec.Code != http.StatusServiceUnavailable || body.Status != "not_ready" {
		t.Fatalf("GET /readyz = %d %+v, want 503 not_ready", rec.Code, body)
	}
	if !slices.Contains(body.Reasons, "no client keys resolved") {
		t.Errorf("reasons = %q, want \"no client keys resolved\"", body.Reasons)
	}
	if slices.Contains(body.Reasons, "no upstreams configured") {
		t.Errorf("reasons = %q, upstreams are configured", body.Reasons)
	}
}
