package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"air-traffic/internal/model"
	"air-traffic/internal/store"
)

func newTestServer(t *testing.T) (*Server, *store.Store, http.Handler) {
	t.Helper()
	st := store.New()
	srv := New(st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return srv, st, srv.Routes()
}

// spineReq builds a request from a loopback caller — the default posture for
// the gateway spine routes when no shared key is configured. httptest's
// default RemoteAddr (192.0.2.1) is deliberately non-local, so spine tests
// must say which side of that line they are on.
func spineReq(method, path string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, path, body)
	req.RemoteAddr = "127.0.0.1:54321"
	return req
}

func TestGatewayLeaksIngestAndAudit(t *testing.T) {
	_, st, h := newTestServer(t)
	body := `{"reports":[
		{"request_id":"gw-1","route":"anthropic","action":"mask",
		 "redactions":[{"path":"messages[0].content","type":"SSN","start":10,"end":21,"detector":"regex","confidence":0.9}],
		 "latency_ms":12,"added_latency_ms":1,"at":"2026-07-02T12:00:00Z"},
		{"request_id":"gw-2","route":"anthropic","action":"block",
		 "redactions":[{"path":"messages[0].content","type":"SSN","start":4,"end":15,"detector":"regex","confidence":0.9}],
		 "latency_ms":3,"added_latency_ms":1,"at":"2026-07-02T12:00:01Z"}
	]}`
	req := spineReq("POST", "/api/gateway/leaks", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	if _, ok := st.GatewayReportByRequestID("gw-1"); !ok {
		t.Error("mask report not stored")
	}
	// mask stays in the report ring; block surfaces on the audit stream
	foundBlock, foundMask := false, false
	for _, e := range st.ListAudit(50) {
		if e.Action == "gateway.block" && e.RequestID == "gw-2" {
			foundBlock = true
		}
		if e.Action == "gateway.mask" {
			foundMask = true
		}
	}
	if !foundBlock {
		t.Error("block report did not reach the audit stream")
	}
	if foundMask {
		t.Error("routine mask traffic should not flood the audit stream")
	}
}

// The strict decoder is the metadata-only guarantee: a gateway (or attacker)
// smuggling a raw value field gets a 400, not a stored value.
func TestGatewayLeaksRejectsValueField(t *testing.T) {
	_, _, h := newTestServer(t)
	body := `{"reports":[{"request_id":"gw-3","route":"anthropic","action":"mask",
		"redactions":[{"path":"p","type":"SSN","start":0,"end":11,"detector":"regex","confidence":0.9,"value":"123-45-6789"}],
		"latency_ms":1,"added_latency_ms":1,"at":"2026-07-02T12:00:00Z"}]}`
	req := spineReq("POST", "/api/gateway/leaks", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for smuggled value field", rec.Code)
	}
}

func TestGatewayEnforcementAndStatus(t *testing.T) {
	_, st, h := newTestServer(t)
	req := spineReq("POST", "/api/gateway/enforcement",
		strings.NewReader(`{"gateway_id":"gw@test","base_url":"http://127.0.0.1:8125","action":"mask","vendors":{"anthropic":["pii_redaction"]}}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("enforcement status = %d: %s", rec.Code, rec.Body.String())
	}

	id, fresh, everSeen := st.GatewayEnforcement("anthropic", "pii_redaction", 45*time.Second)
	if id != "gw@test" || !fresh || !everSeen {
		t.Errorf("enforcement lookup = (%q, %v, %v)", id, fresh, everSeen)
	}

	statusReq := httptest.NewRequest("GET", "/api/gateway/status", nil)
	statusRec := httptest.NewRecorder()
	h.ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusOK || !strings.Contains(statusRec.Body.String(), `"fresh": true`) {
		t.Errorf("status = %d body %s", statusRec.Code, statusRec.Body.String())
	}
}

func TestGatewayPatternsServesPack(t *testing.T) {
	_, st, h := newTestServer(t)
	st.SetPatternPack(model.PatternPack{Version: 3, Rules: []model.PatternRule{{ID: "r1", Type: "SSN", Regex: `\d{9}`, Confidence: 0.7}}})
	req := spineReq("GET", "/api/gateway/patterns", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"version": 3`) {
		t.Errorf("patterns = %d body %s", rec.Code, rec.Body.String())
	}
}
