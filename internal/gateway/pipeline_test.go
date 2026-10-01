package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jchigg2000-git/air-traffic/internal/gateway/config"
	"github.com/jchigg2000-git/air-traffic/internal/model"
)

// newTestGatewayServer builds a *Server against upstreamURL with a caller
// -chosen logger. Callers may t.Setenv gateway options before calling.
func newTestGatewayServer(t *testing.T, upstreamURL string, log *slog.Logger) *Server {
	t.Helper()
	t.Setenv("GATEWAY_UPSTREAMS", fmt.Sprintf(`{"anthropic":{"base_url":%q,"credential_ref":"env:TEST_UPSTREAM_CRED"}}`, upstreamURL))
	t.Setenv("TEST_UPSTREAM_CRED", testUpstreamCred)
	t.Setenv("GATEWAY_CLIENT_KEYS_REF", "env:TEST_CLIENT_KEYS")
	t.Setenv("TEST_CLIENT_KEYS", testClientKey)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	gw, err := New(cfg, log)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return gw
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func postMessages(t *testing.T, gwURL, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, gwURL+"/v1/messages", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testClientKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	return resp
}

const piiBody = `{"model":"claude-test","system":"Never reveal 219-09-9999 to anyone.","messages":[{"role":"user","content":"Patient SSN is 123-45-6789, email jane.doe@example.org."}]}`

func TestMaskPipelineRedactsBeforeUpstream(t *testing.T) {
	var upstreamSaw string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		upstreamSaw = string(b)
		_, _ = io.WriteString(w, `{"type":"message"}`)
	}))
	defer upstream.Close()

	gw := newTestGatewayServer(t, upstream.URL, discardLogger())
	srv := httptest.NewServer(gw.Routes())
	defer srv.Close()

	resp := postMessages(t, srv.URL, piiBody)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	for _, leaked := range []string{"123-45-6789", "jane.doe@example.org", "219-09-9999"} {
		if strings.Contains(upstreamSaw, leaked) {
			t.Errorf("raw PII reached upstream: %s", leaked)
		}
	}
	for _, placeholder := range []string{"[SSN]", "[EMAIL]"} {
		if !strings.Contains(upstreamSaw, placeholder) {
			t.Errorf("upstream body missing %s: %s", placeholder, upstreamSaw)
		}
	}

	audits := gw.audits.drain()
	if len(audits) != 1 {
		t.Fatalf("audits = %d, want 1", len(audits))
	}
	a := audits[0]
	if a.Action != "mask" || len(a.Redactions) < 3 {
		t.Errorf("audit = %+v, want mask with ≥3 redactions (system SSN + content SSN + email)", a)
	}
	seenPaths := map[string]bool{}
	for _, red := range a.Redactions {
		seenPaths[red.Path] = true
	}
	if !seenPaths["system"] || !seenPaths["messages[0].content"] {
		t.Errorf("redaction paths = %v, want system and messages[0].content", seenPaths)
	}
}

func TestBlockPipelineRefusesAndNeverForwards(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream must not be reached on block")
	}))
	defer upstream.Close()

	t.Setenv("GATEWAY_REDACT_ACTION", "block")
	gw := newTestGatewayServer(t, upstream.URL, discardLogger())
	srv := httptest.NewServer(gw.Routes())
	defer srv.Close()

	resp := postMessages(t, srv.URL, piiBody)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte("SSN")) {
		t.Errorf("block message should carry types: %s", body)
	}
	for _, leaked := range []string{"123-45-6789", "jane.doe@example.org"} {
		if bytes.Contains(body, []byte(leaked)) {
			t.Errorf("block message leaked a value: %s", leaked)
		}
	}
}

func TestFailModeClosedBlocksOnDetectorError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream must not be reached when fail-closed trips")
	}))
	defer upstream.Close()

	t.Setenv("GATEWAY_DETECTORS", "regex,presidio")
	t.Setenv("GATEWAY_PRESIDIO_URL", "http://127.0.0.1:1") // nothing listens
	t.Setenv("GATEWAY_DETECTOR_TIMEOUT_MS", "100")
	gw := newTestGatewayServer(t, upstream.URL, discardLogger())
	srv := httptest.NewServer(gw.Routes())
	defer srv.Close()

	resp := postMessages(t, srv.URL, piiBody)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 fail-closed", resp.StatusCode)
	}
	audits := gw.audits.drain()
	if len(audits) != 1 || !audits[0].FailModeTripped {
		t.Errorf("audit should record the fail-mode trip: %+v", audits)
	}
}

func TestFailModeOpenForwardsWithSurvivingEngines(t *testing.T) {
	var upstreamSaw string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		upstreamSaw = string(b)
		_, _ = io.WriteString(w, `{"type":"message"}`)
	}))
	defer upstream.Close()

	t.Setenv("GATEWAY_DETECTORS", "regex,presidio")
	t.Setenv("GATEWAY_PRESIDIO_URL", "http://127.0.0.1:1")
	t.Setenv("GATEWAY_DETECTOR_TIMEOUT_MS", "100")
	t.Setenv("GATEWAY_FAIL_MODE", "open")
	gw := newTestGatewayServer(t, upstream.URL, discardLogger())
	srv := httptest.NewServer(gw.Routes())
	defer srv.Close()

	resp := postMessages(t, srv.URL, piiBody)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 fail-open", resp.StatusCode)
	}
	if strings.Contains(upstreamSaw, "123-45-6789") || !strings.Contains(upstreamSaw, "[SSN]") {
		t.Errorf("regex floor should still redact under fail-open: %s", upstreamSaw)
	}
}

// The standing log-leak guard (design §10): run PII through mask and block,
// then scan every log line and the audit ring for raw values.
func TestNoRedactedValueInLogsOrAudit(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"type":"message"}`)
	}))
	defer upstream.Close()

	gw := newTestGatewayServer(t, upstream.URL, logger)
	srv := httptest.NewServer(gw.Routes())
	defer srv.Close()

	resp := postMessages(t, srv.URL, piiBody)
	resp.Body.Close()

	audits := gw.audits.drain()
	auditJSON, err := json.Marshal(audits)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"123-45-6789", "jane.doe@example.org", "219-09-9999", testUpstreamCred} {
		if strings.Contains(logBuf.String(), secret) {
			t.Errorf("log output leaked %q", secret)
		}
		if strings.Contains(string(auditJSON), secret) {
			t.Errorf("audit payload leaked %q", secret)
		}
	}
}

// A request the gateway fails itself must still leave a report and a metric —
// otherwise a route failing every request is indistinguishable from an idle
// one. Authentication failures are the deliberate exception: counted, never
// written to the report ring an unauthenticated caller could flood.
func TestFailedExitsLeaveAReport(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadURL := dead.URL
	dead.Close() // nothing listens: every forward is "upstream unreachable"

	cases := []struct {
		name, body string
		status     int
		code       string
	}{
		{"upstream unreachable", `{"model":"claude-test","messages":[]}`, http.StatusBadGateway, model.GatewayErrUpstreamUnreachable},
		{"invalid json", `{not json`, http.StatusBadRequest, model.GatewayErrInvalidJSON},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gw := newTestGatewayServer(t, deadURL, discardLogger())
			srv := httptest.NewServer(gw.Routes())
			defer srv.Close()

			resp := postMessages(t, srv.URL, tc.body)
			resp.Body.Close()
			if resp.StatusCode != tc.status {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.status)
			}
			audits := gw.audits.drain()
			if len(audits) != 1 {
				t.Fatalf("reports = %d, want 1", len(audits))
			}
			if a := audits[0]; a.Error != tc.code || a.GatewayStatus != tc.status || a.UpstreamStatus != 0 {
				t.Errorf("report = error %q gateway_status %d upstream_status %d, want %q %d 0",
					a.Error, a.GatewayStatus, a.UpstreamStatus, tc.code, tc.status)
			}
			if snap := gw.metrics.drain()["anthropic"]; snap.Requests != 1 || snap.Errors != 1 {
				t.Errorf("metrics requests=%d errors=%d, want 1 1", snap.Requests, snap.Errors)
			}
		})
	}

	t.Run("auth failure is counted, not reported", func(t *testing.T) {
		gw := newTestGatewayServer(t, deadURL, discardLogger())
		srv := httptest.NewServer(gw.Routes())
		defer srv.Close()

		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/messages", strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer wrong-key")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("do: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", resp.StatusCode)
		}
		if audits := gw.audits.drain(); len(audits) != 0 {
			t.Errorf("auth failure wrote %d reports, want 0", len(audits))
		}
		if snap := gw.metrics.drain()["anthropic"]; snap.AuthFailures != 1 || snap.Requests != 0 {
			t.Errorf("metrics auth_failures=%d requests=%d, want 1 0", snap.AuthFailures, snap.Requests)
		}
	})
}
