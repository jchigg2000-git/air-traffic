package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// spineRoutes are every route the gateway drives. /api/gateway/status is
// deliberately absent: it is the browser's liveness view and carries no terms.
var spineRoutes = []struct {
	method, path, body string
}{
	{"POST", "/api/gateway/leaks", `{"reports":[{"request_id":"gw-a","route":"anthropic","action":"mask","latency_ms":1,"added_latency_ms":1,"at":"2026-07-02T12:00:00Z"}]}`},
	{"POST", "/api/gateway/enforcement", `{"gateway_id":"gw@test","base_url":"http://127.0.0.1:8125","action":"mask"}`},
	{"GET", "/api/gateway/patterns", ""},
}

// Without a shared key the spine is host-local: a remote caller cannot push
// forged enforcement evidence, and — since the G6 config-knob slice put
// deny-list terms in the pattern pack — cannot read the pack either.
func TestSpineRoutesRejectRemoteCallersWithoutKey(t *testing.T) {
	_, _, h := newTestServer(t)
	for _, rt := range spineRoutes {
		req := httptest.NewRequest(rt.method, rt.path, strings.NewReader(rt.body))
		req.RemoteAddr = "203.0.113.7:44321"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s from remote = %d, want 401", rt.method, rt.path, rec.Code)
		}
	}
}

// With a key configured, loopback stops being a free pass — the key is the
// only credential, so a compose sidecar and a laptop curl are treated alike.
func TestSpineRoutesRequireKeyWhenConfigured(t *testing.T) {
	srv, _, _ := newTestServer(t)
	srv.SetSpineKey("spine-test-key")
	h := srv.Routes()

	for _, rt := range spineRoutes {
		unauth := spineReq(rt.method, rt.path, strings.NewReader(rt.body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, unauth)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without key = %d, want 401", rt.method, rt.path, rec.Code)
		}

		wrong := spineReq(rt.method, rt.path, strings.NewReader(rt.body))
		wrong.Header.Set("Authorization", "Bearer wrong-key")
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, wrong)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with wrong key = %d, want 401", rt.method, rt.path, rec.Code)
		}

		// remote + correct key is the whole point: the gateway is a peer, not
		// a neighbour on the loopback interface.
		ok := httptest.NewRequest(rt.method, rt.path, strings.NewReader(rt.body))
		ok.RemoteAddr = "203.0.113.7:44321"
		ok.Header.Set("Authorization", "Bearer spine-test-key")
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, ok)
		if rec.Code >= 300 {
			t.Errorf("%s %s with key = %d: %s", rt.method, rt.path, rec.Code, rec.Body.String())
		}
	}
}

// The x-api-key header is accepted too, matching the gateway's own client-key
// convention (proxy.go requireClientKey).
func TestSpineAcceptsHeaderAlias(t *testing.T) {
	srv, _, _ := newTestServer(t)
	srv.SetSpineKey("spine-test-key")
	req := httptest.NewRequest("GET", "/api/gateway/patterns", nil)
	req.Header.Set("X-Air-Traffic-Key", "spine-test-key")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("patterns with X-Air-Traffic-Key = %d", rec.Code)
	}
}

// The status route reports the posture rather than implying enforcement that
// isn't there — the honesty model, applied to our own control surface.
func TestGatewayStatusReportsSpinePosture(t *testing.T) {
	_, _, h := newTestServer(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/gateway/status", nil))
	if !strings.Contains(rec.Body.String(), `"spine_auth": "loopback_only"`) {
		t.Errorf("status body = %s", rec.Body.String())
	}

	srv, _, _ := newTestServer(t)
	srv.SetSpineKey("spine-dev-insecure")
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest("GET", "/api/gateway/status", nil))
	body := rec.Body.String()
	if !strings.Contains(body, `"spine_auth": "shared_key"`) || !strings.Contains(body, `"spine_key_unrotated": true`) {
		t.Errorf("dev-default key not reported: %s", body)
	}
}

func TestIsLoopback(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:8122": true,
		"[::1]:8122":     true,
		"::1":            true,
		"192.0.2.1:1234": false,
		"172.18.0.4:443": false, // a compose sidecar is NOT local
		"":               false, // unparseable fails closed
		"garbage":        false,
	}
	for addr, want := range cases {
		if got := isLoopback(addr); got != want {
			t.Errorf("isLoopback(%q) = %v, want %v", addr, got, want)
		}
	}
}
