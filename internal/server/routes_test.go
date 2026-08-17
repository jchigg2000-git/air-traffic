package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"air-traffic/internal/model"
	"air-traffic/internal/store"
)

func testServer() http.Handler {
	st := store.New()
	return New(st, slog.New(slog.NewTextHandler(io.Discard, nil))).Routes()
}

func req(t *testing.T, h http.Handler, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var r *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		r = httptest.NewRequest(method, path, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	var out map[string]any
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return rec.Code, out
}

func TestHealth(t *testing.T) {
	h := testServer()
	code, body := req(t, h, "GET", "/api/health", nil)
	if code != 200 || body["ok"] != true {
		t.Fatalf("health failed: %d %v", code, body)
	}
}

func TestAdaptersList(t *testing.T) {
	h := testServer()
	code, body := req(t, h, "GET", "/api/adapters", nil)
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	arr, ok := body["adapters"].([]any)
	if !ok || len(arr) < 16 {
		t.Fatalf("expected >=16 adapters, got %v", len(arr))
	}
}

func TestAdapterCRUDAndManifest(t *testing.T) {
	h := testServer()
	code, _ := req(t, h, "GET", "/api/adapters/openai", nil)
	if code != 200 {
		t.Fatalf("get adapter: %d", code)
	}
	code, body := req(t, h, "GET", "/api/adapters/openai/manifest", nil)
	if code != 200 || body["capabilities"] == nil {
		t.Fatalf("manifest: %d %v", code, body)
	}
	mode := model.ModeDisabled
	code, body = req(t, h, "PATCH", "/api/adapters/openai", model.AdapterPatch{Mode: &mode})
	if code != 200 {
		t.Fatalf("patch: %d", code)
	}
	code, body = req(t, h, "POST", "/api/adapters/openai/test", nil)
	if code != 200 || body["status"] == nil {
		t.Fatalf("test: %d %v", code, body)
	}
}

func TestBaselinesRoute(t *testing.T) {
	h := testServer()
	code, body := req(t, h, "GET", "/api/baselines", nil)
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	if arr, ok := body["baselines"].([]any); !ok || len(arr) != 4 {
		t.Fatalf("expected 4 baselines, got %v", body["baselines"])
	}
}

func TestPolicyApplyReturnsCoverage(t *testing.T) {
	h := testServer()
	code, body := req(t, h, "PUT", "/api/policies", map[string]any{"baseline": "fintech"})
	if code != 200 {
		t.Fatalf("put policy: %d %v", code, body)
	}
	cov, ok := body["coverage"].(map[string]any)
	if !ok || cov["rows"] == nil || cov["summary"] == nil {
		t.Fatalf("expected coverage report, got %v", body)
	}
	// unknown baseline rejected
	code, _ = req(t, h, "PUT", "/api/policies", map[string]any{"baseline": "bogus"})
	if code != 400 {
		t.Errorf("expected 400 for unknown baseline, got %d", code)
	}
}

func TestCredentialsRejectPlaintext(t *testing.T) {
	h := testServer()
	code, _ := req(t, h, "POST", "/api/credentials", map[string]any{"name": "x", "api_key": "sk-secret-value"})
	if code != 400 {
		t.Errorf("expected 400 rejecting plaintext secret, got %d", code)
	}
	code, body := req(t, h, "POST", "/api/credentials", map[string]any{"name": "x", "secret_ref": "vault://kv/openai"})
	if code != 201 || body["credential"] == nil {
		t.Errorf("expected 201 for secret_ref credential, got %d %v", code, body)
	}
}

// reqLocal is req from a loopback caller. httptest's default RemoteAddr
// (192.0.2.1) is deliberately non-local, so any test touching an ingest or
// admin route has to say which side of that line it is on.
func reqLocal(t *testing.T, h http.Handler, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var r *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		r = httptest.NewRequest(method, path, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	r.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	var out map[string]any
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return rec.Code, out
}

func TestObservationsIngestAndList(t *testing.T) {
	h := testServer()
	batch := map[string]any{
		"contract":  model.ObservationContract,
		"batch_id":  "test",
		"connector": map[string]any{"type": "ai-vendor", "instance": "openai", "api_version": "x"},
		"complete":  true,
	}
	code, body := reqLocal(t, h, "POST", "/api/observations", batch)
	if code != 202 || body["observation"] == nil {
		t.Fatalf("ingest: %d %v", code, body)
	}
	// wrong contract rejected
	code, _ = reqLocal(t, h, "POST", "/api/observations", map[string]any{"contract": "nope"})
	if code != 400 {
		t.Errorf("expected 400 for wrong contract, got %d", code)
	}
	// Reads stay open — the observability surfaces are the product.
	code, body = req(t, h, "GET", "/api/observations", nil)
	if code != 200 || body["observations"] == nil {
		t.Fatalf("list obs: %d %v", code, body)
	}
}

// Observation ingest is a write, and writes from off-host need a credential.
// With no keys configured the route accepts loopback only, the same posture
// the other three spine ingest routes have had since GATEWAY-2; observations
// was simply the one that was never gated.
func TestObservationsIngestRejectsUnauthenticatedRemoteWriter(t *testing.T) {
	h := testServer()
	code, _ := req(t, h, "POST", "/api/observations", map[string]any{
		"contract": model.ObservationContract, "batch_id": "remote",
	})
	if code != http.StatusUnauthorized {
		t.Errorf("remote unauthenticated ingest = %d, want 401", code)
	}
}

func TestAuditAndSIEM(t *testing.T) {
	h := testServer()
	code, body := req(t, h, "GET", "/api/audit", nil)
	if code != 200 {
		t.Fatalf("audit: %d", code)
	}
	if arr, ok := body["audit"].([]any); !ok || len(arr) == 0 {
		t.Fatal("expected seeded audit events")
	}
	code, body = req(t, h, "GET", "/api/audit?format=siem", nil)
	if code != 200 || body["records"] == nil {
		t.Fatalf("siem export: %d %v", code, body)
	}
}

func TestDriftAndEnvConfig(t *testing.T) {
	h := testServer()
	// applying a policy refreshes drift
	req(t, h, "PUT", "/api/policies", map[string]any{"baseline": "fintech"})
	code, body := req(t, h, "GET", "/api/drift", nil)
	if code != 200 {
		t.Fatalf("drift: %d", code)
	}
	if arr, ok := body["drift"].([]any); !ok || len(arr) == 0 {
		t.Fatal("expected drift records after policy apply")
	}
	code, body = req(t, h, "GET", "/api/envconfig", nil)
	if code != 200 || body["artifacts"] == nil {
		t.Fatalf("envconfig: %d %v", code, body)
	}
}
