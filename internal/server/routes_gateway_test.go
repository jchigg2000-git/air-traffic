package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jchigg2000-git/air-traffic/internal/model"
	"github.com/jchigg2000-git/air-traffic/internal/store"
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

	id, fresh, everSeen := st.GatewayEnforcement("anthropic", "pii_redaction", model.GatewayStaleAfter)
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

// The heartbeat's pulled-state fields are stored and returned on the status
// view; a heartbeat without them (an older gateway) still decodes and the
// fields stay absent rather than reading as zero.
func TestGatewayStatusReturnsPulledVersions(t *testing.T) {
	_, _, h := newTestServer(t)
	for _, body := range []string{
		`{"gateway_id":"gw@new","base_url":"http://127.0.0.1:8125","action":"mask","vendors":{},` +
			`"policy_baseline":"fintech","pack_version":0,"keystore_version":7,"detector_ran":false}`,
		`{"gateway_id":"gw@old","base_url":"http://127.0.0.1:8126","action":"mask","vendors":{}}`,
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, spineReq("POST", "/api/gateway/enforcement", strings.NewReader(body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("enforcement %s → %d: %s", body, rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/gateway/status", nil))
	var out struct {
		Gateways []map[string]any `json:"gateways"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	byID := map[string]map[string]any{}
	for _, g := range out.Gateways {
		byID[g["gateway_id"].(string)] = g
	}
	got := byID["gw@new"]
	if got["policy_baseline"] != "fintech" || got["pack_version"] != float64(0) ||
		got["keystore_version"] != float64(7) || got["detector_ran"] != false {
		t.Errorf("new gateway status = %v", got)
	}
	for _, k := range []string{"policy_baseline", "pack_version", "keystore_version", "detector_ran"} {
		if _, ok := byID["gw@old"][k]; ok {
			t.Errorf("old heartbeat reports %s = %v, want absent", k, byID["gw@old"][k])
		}
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

// The harness hands the client key and prompt bodies to base_url, so a
// heartbeat cannot point it anywhere but an absolute http(s) host.
func TestGatewayEnforcementRejectsBadBaseURL(t *testing.T) {
	_, _, h := newTestServer(t)
	for _, body := range []string{
		`{"gateway_id":"gw@test","base_url":"evil:9"}`,
		`{"gateway_id":"gw@test","base_url":"http://u:p@evil:9"}`,
		`{"gateway_id":"gw@test","base_url":"ftp://evil:9"}`,
		`{"gateway_id":"gw@test","base_url":"/relative"}`,
		`{"gateway_id":"gw@test"}`,
		`{"gateway_id":"` + strings.Repeat("g", 129) + `","base_url":"http://127.0.0.1:8125"}`,
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, spineReq("POST", "/api/gateway/enforcement", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s → %d, want 400", body, rec.Code)
		}
	}
}

// The traffic feed backs the Gateway Traffic page: newest first, a bounded
// page, and [] rather than null so the UI never has to special-case empty.
func TestGatewayRequestsFeed(t *testing.T) {
	_, st, h := newTestServer(t)

	get := func(query string) (int, map[string]json.RawMessage) {
		t.Helper()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/gateway/requests"+query, nil))
		var out map[string]json.RawMessage
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatalf("decode %q: %v", query, err)
			}
		}
		return rec.Code, out
	}
	ids := func(raw json.RawMessage) []string {
		t.Helper()
		var reps []model.GatewayRequestReport
		if err := json.Unmarshal(raw, &reps); err != nil {
			t.Fatalf("decode requests: %v", err)
		}
		out := make([]string, len(reps))
		for i, r := range reps {
			out[i] = r.RequestID
		}
		return out
	}

	code, body := get("")
	if code != http.StatusOK {
		t.Fatalf("empty store status = %d", code)
	}
	if got := string(body["requests"]); got != "[]" {
		t.Errorf("empty store requests = %s, want []", got)
	}
	if got := string(body["limit"]); got != "100" {
		t.Errorf("default limit = %s, want 100", got)
	}

	st.AddGatewayReports([]model.GatewayRequestReport{
		{RequestID: "gw-a", Route: "anthropic", Action: "pass"},
		{RequestID: "gw-b", Route: "anthropic", Action: "mask"},
		{RequestID: "gw-c", Route: "openai", Action: "block"},
	})

	_, body = get("")
	if got := strings.Join(ids(body["requests"]), ","); got != "gw-c,gw-b,gw-a" {
		t.Errorf("order = %s, want newest first gw-c,gw-b,gw-a", got)
	}

	_, body = get("?limit=2")
	if got := strings.Join(ids(body["requests"]), ","); got != "gw-c,gw-b" {
		t.Errorf("limit=2 requests = %s, want gw-c,gw-b", got)
	}

	_, body = get("?limit=999999")
	if got := string(body["limit"]); got != "1000" {
		t.Errorf("limit=999999 reported limit = %s, want clamp to 1000", got)
	}

	for _, bad := range []string{"0", "-1", "abc"} {
		if code, _ := get("?limit=" + bad); code != http.StatusBadRequest {
			t.Errorf("limit=%s status = %d, want 400", bad, code)
		}
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/gateway/requests", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST status = %d, want 405", rec.Code)
	}
}
