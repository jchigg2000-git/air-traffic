package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// writeRoutes are the state-changing control-plane routes the admin key gates.
// Bodies are deliberately valid: a 400 would prove nothing about the gate.
var writeRoutes = []struct {
	method, path, body string
}{
	{"PATCH", "/api/adapters/openai", `{"enabled":true}`},
	{"PUT", "/api/policies", `{"baseline":"fintech"}`},
	{"POST", "/api/credentials", `{"name":"n","provider":"openai","type":"api_key","secret_ref":"env:X"}`},
	{"POST", "/api/harness/runs", `{"count":1}`},
	{"POST", "/api/harness/proposals", `{"kind":"allow_list","type":"PERSON_NAME","allow_list":["acme"]}`},
}

// readRoutes must never be gated: the observability surfaces are the product,
// and a mode that hides them protects nothing a curl cannot reach anyway.
var readRoutes = []string{
	"/api/health", "/api/adapters", "/api/baselines", "/api/policies",
	"/api/audit", "/api/drift", "/api/observations", "/api/activity",
}

// The compatibility bar: with no key configured the control plane behaves
// exactly as it did before the tier existed. This is the posture the
// one-command compose demo runs in, where the browser reaches the SPA over the
// Docker bridge and is never loopback.
func TestWritesAreOpenWithoutAdminKey(t *testing.T) {
	_, _, h := newTestServer(t)
	for _, rt := range writeRoutes {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(rt.method, rt.path, strings.NewReader(rt.body)))
		if rec.Code == http.StatusUnauthorized {
			t.Errorf("%s %s = 401 with no admin key configured; the unset posture must stay open", rt.method, rt.path)
		}
	}
}

func TestWritesRequireAdminKeyWhenConfigured(t *testing.T) {
	srv, _, _ := newTestServer(t)
	srv.SetAdminKey("adm-test-key")
	h := srv.Routes()

	for _, rt := range writeRoutes {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(rt.method, rt.path, strings.NewReader(rt.body)))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without key = %d, want 401", rt.method, rt.path, rec.Code)
		}

		wrong := httptest.NewRequest(rt.method, rt.path, strings.NewReader(rt.body))
		wrong.Header.Set("X-Air-Traffic-Admin-Key", "not-it")
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, wrong)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with wrong key = %d, want 401", rt.method, rt.path, rec.Code)
		}

		ok := httptest.NewRequest(rt.method, rt.path, strings.NewReader(rt.body))
		ok.Header.Set("X-Air-Traffic-Admin-Key", "adm-test-key")
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, ok)
		if rec.Code == http.StatusUnauthorized {
			t.Errorf("%s %s with the right key = 401: %s", rt.method, rt.path, rec.Body.String())
		}
	}
}

// Loopback is not a free pass once a key exists — otherwise anything running on
// the host, including the browser in the bare `go run` dev flow, silently keeps
// write access the operator believes they have taken away.
func TestAdminKeyAppliesToLoopbackToo(t *testing.T) {
	srv, _, _ := newTestServer(t)
	srv.SetAdminKey("adm-test-key")
	req := httptest.NewRequest("PUT", "/api/policies", strings.NewReader(`{"baseline":"fintech"}`))
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("loopback write without key = %d, want 401", rec.Code)
	}
}

func TestReadsStayOpenWithAdminKeyConfigured(t *testing.T) {
	srv, _, _ := newTestServer(t)
	srv.SetAdminKey("adm-test-key")
	h := srv.Routes()
	for _, path := range readRoutes {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200; reads are never gated", path, rec.Code)
		}
	}
}

// The gateway pushes observation batches with the spine key, not the admin key.
// Gating that route on the admin key alone would break the data plane.
func TestObservationIngestAcceptsEitherCredential(t *testing.T) {
	srv, _, _ := newTestServer(t)
	srv.SetAdminKey("adm-test-key")
	srv.SetSpineKey("spine-test-key")
	h := srv.Routes()

	batch := `{"contract":"ops-observation-batch/v1","batch_id":"b","connector":{"type":"ai-vendor","instance":"openai","api_version":"x"},"complete":true}`
	for _, key := range []string{"adm-test-key", "spine-test-key"} {
		req := httptest.NewRequest("POST", "/api/observations", strings.NewReader(batch))
		req.RemoteAddr = "203.0.113.7:44321" // a gateway is a peer, not a neighbour
		req.Header.Set("Authorization", "Bearer "+key)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Errorf("ingest with %q = %d, want 202: %s", key, rec.Code, rec.Body.String())
		}
	}
}

// The keystore's loopback default holds, and the admin key is the alternative
// that makes it reachable from a browser at all (GATEWAY-7a).
func TestKeystoreAdminAcceptsAdminKeyFromRemote(t *testing.T) {
	srv, _, _ := newTestServer(t)
	remote := func() *http.Request {
		r := httptest.NewRequest("GET", "/api/apps", nil)
		r.RemoteAddr = "203.0.113.7:44321"
		return r
	}
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, remote())
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("remote keystore read without a key = %d, want 401", rec.Code)
	}

	srv.SetAdminKey("adm-test-key")
	req := remote()
	req.Header.Set("X-Air-Traffic-Admin-Key", "adm-test-key")
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("remote keystore read with the admin key = %d, want 200", rec.Code)
	}
}

// The posture is reported, not implied — "open" is a real state and the status
// surface has to be able to say so.
func TestStatusReportsAdminPosture(t *testing.T) {
	_, _, h := newTestServer(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/health", nil))
	if !strings.Contains(rec.Body.String(), `"admin_auth": "open"`) {
		t.Errorf("health body = %s", rec.Body.String())
	}

	srv, _, _ := newTestServer(t)
	srv.SetAdminKey("adm-test-key")
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, httptest.NewRequest("GET", "/api/gateway/status", nil))
	if !strings.Contains(rec.Body.String(), `"admin_auth": "admin_key"`) {
		t.Errorf("status body = %s", rec.Body.String())
	}
}

// The synthetic vendor replica is mounted bare, so its two mutating /_harness
// control paths are the one place a state change could reach the shared adapter
// record without passing requireAdminWrite. Both write exactly what
// PATCH /api/adapters/{id} writes, so both carry the same key.
//
// The GET case is the one a method-based gate would miss: /_harness/reset
// mutates on any method, not just POST.
var syntheticWriteRoutes = []struct{ method, path string }{
	{"PUT", "/synthetic/openai/_harness/scenario/429-retry-after"},
	{"POST", "/synthetic/openai/_harness/reset"},
	{"GET", "/synthetic/openai/_harness/reset"},
}

// Reads on the same prefix, plus the vendor surface itself, must stay open —
// the replica holds nothing real and answering anyone is the point.
var syntheticReadRoutes = []string{
	"/synthetic/openai/_harness/manifest",
	"/synthetic/openai/_harness/calls",
	"/synthetic/openai/admin/organization/users",
}

func TestSyntheticHarnessWritesRequireAdminKey(t *testing.T) {
	srv, _, _ := newTestServer(t)
	srv.SetAdminKey("adm-test-key")
	h := srv.Routes()

	for _, rt := range syntheticWriteRoutes {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(rt.method, rt.path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without key = %d, want 401 (it writes the adapter record)", rt.method, rt.path, rec.Code)
		}

		ok := httptest.NewRequest(rt.method, rt.path, nil)
		ok.Header.Set("X-Air-Traffic-Admin-Key", "adm-test-key")
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, ok)
		if rec.Code == http.StatusUnauthorized {
			t.Errorf("%s %s with the right key = 401: %s", rt.method, rt.path, rec.Body.String())
		}
	}
}

func TestSyntheticReadsStayOpenWithAdminKey(t *testing.T) {
	srv, _, _ := newTestServer(t)
	srv.SetAdminKey("adm-test-key")
	h := srv.Routes()
	for _, path := range syntheticReadRoutes {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code == http.StatusUnauthorized {
			t.Errorf("GET %s = 401; the replica and its read paths are never gated", path)
		}
	}
}

// Unset key keeps the pre-existing posture on these paths too.
func TestSyntheticHarnessWritesOpenWithoutAdminKey(t *testing.T) {
	_, _, h := newTestServer(t)
	for _, rt := range syntheticWriteRoutes {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(rt.method, rt.path, nil))
		if rec.Code == http.StatusUnauthorized {
			t.Errorf("%s %s = 401 with no admin key configured; the unset posture must stay open", rt.method, rt.path)
		}
	}
}
