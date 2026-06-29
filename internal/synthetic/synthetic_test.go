package synthetic

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"air-traffic/internal/model"
	"air-traffic/internal/store"
)

func newHandler() *Handler {
	return New(store.New(), slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func do(t *testing.T, h *Handler, method, path string) (int, any) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var body any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s %s: invalid JSON: %v", method, path, err)
		}
	}
	return rec.Code, body
}

func obj(t *testing.T, body any) map[string]any {
	t.Helper()
	m, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected JSON object, got %T", body)
	}
	return m
}

func hasKeys(t *testing.T, m map[string]any, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			t.Errorf("missing expected key %q (have %v)", k, keysOf(m))
		}
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// ---- byte-identical success envelopes ----

func TestOpenAIFidelity(t *testing.T) {
	h := newHandler()
	code, body := do(t, h, "GET", "/synthetic/openai/admin/organization/users")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	m := obj(t, body)
	hasKeys(t, m, "object", "data", "first_id", "last_id", "has_more")
	if m["object"] != "list" {
		t.Errorf("openai list object should be \"list\", got %v", m["object"])
	}

	_, usage := do(t, h, "GET", "/synthetic/openai/v1/organization/usage")
	um := obj(t, usage)
	hasKeys(t, um, "object", "data", "has_more", "next_page")
	if um["object"] != "page" {
		t.Errorf("usage object should be \"page\", got %v", um["object"])
	}
}

func TestAnthropicFidelity(t *testing.T) {
	h := newHandler()
	_, body := do(t, h, "GET", "/synthetic/anthropic/v1/organizations/workspaces")
	m := obj(t, body)
	hasKeys(t, m, "data", "has_more", "first_id", "last_id")
	if _, ok := m["object"]; ok {
		t.Error("anthropic list must NOT carry an OpenAI-style \"object\" key")
	}
}

func TestBedrockFidelity(t *testing.T) {
	h := newHandler()
	_, body := do(t, h, "GET", "/synthetic/bedrock/guardrails")
	m := obj(t, body)
	hasKeys(t, m, "guardrails", "nextToken")
}

func TestAzureFidelity(t *testing.T) {
	h := newHandler()
	_, body := do(t, h, "GET", "/synthetic/azure_openai/subscriptions/x/locations/eastus/usages")
	m := obj(t, body)
	hasKeys(t, m, "value")
}

func TestVertexFidelity(t *testing.T) {
	h := newHandler()
	_, body := do(t, h, "GET", "/synthetic/vertex/v1/projects/acme/timeSeries")
	m := obj(t, body)
	hasKeys(t, m, "timeSeries")
}

func TestGitHubFidelity(t *testing.T) {
	h := newHandler()
	_, seats := do(t, h, "GET", "/synthetic/github_copilot/enterprises/acme/copilot/billing/seats")
	sm := obj(t, seats)
	hasKeys(t, sm, "total_seats", "seats")

	_, metrics := do(t, h, "GET", "/synthetic/github_copilot/enterprises/acme/copilot/metrics")
	if _, ok := metrics.([]any); !ok {
		t.Errorf("github metrics must be a JSON array, got %T", metrics)
	}
}

// ---- vendor-shaped error envelopes ----

func TestErrorEnvelopesPerVendor(t *testing.T) {
	h := newHandler()
	scenario := "401"

	cases := map[string]func(m map[string]any) bool{
		"openai": func(m map[string]any) bool {
			e, ok := m["error"].(map[string]any)
			return ok && e["type"] != nil && e["code"] != nil
		},
		"anthropic": func(m map[string]any) bool {
			return m["type"] == "error" && m["error"] != nil
		},
		"bedrock": func(m map[string]any) bool {
			return m["__type"] != nil && m["message"] != nil
		},
		"vertex": func(m map[string]any) bool {
			e, ok := m["error"].(map[string]any)
			return ok && e["status"] != nil
		},
		"github_copilot": func(m map[string]any) bool {
			return m["message"] != nil && m["documentation_url"] != nil
		},
	}

	for vendor, check := range cases {
		if _, err := h.store.SetScenario(vendor, scenario); err != nil {
			t.Fatalf("set scenario for %s: %v", vendor, err)
		}
		code, body := do(t, h, "GET", "/synthetic/"+vendor+"/anything")
		if code != http.StatusUnauthorized {
			t.Errorf("%s: expected 401, got %d", vendor, code)
		}
		if !check(obj(t, body)) {
			t.Errorf("%s: error envelope shape mismatch: %v", vendor, body)
		}
	}
}

func TestDisabledReturnsVendorShaped503(t *testing.T) {
	h := newHandler()
	enabled := false
	if _, err := h.store.PatchAdapter("openai", model.AdapterPatch{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	code, body := do(t, h, "GET", "/synthetic/openai/admin/organization/users")
	if code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", code)
	}
	if _, ok := obj(t, body)["error"]; !ok {
		t.Error("expected openai error envelope on 503")
	}
}

func TestHarnessManifest(t *testing.T) {
	h := newHandler()
	code, body := do(t, h, "GET", "/synthetic/openai/_harness/manifest")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	m := obj(t, body)
	hasKeys(t, m, "adapter_id", "vendor", "mode", "supported_modes", "supported_scenarios", "capabilities")
}

func TestCallsRecorded(t *testing.T) {
	h := newHandler()
	do(t, h, "GET", "/synthetic/openai/admin/organization/users")
	calls := h.store.ListCalls("openai", 10)
	if len(calls) == 0 {
		t.Fatal("expected a recorded call")
	}
	// Authorization header should be redacted if present (none sent here, but ensure no panic).
	if calls[0].AdapterID != "openai" {
		t.Errorf("wrong adapter on call: %s", calls[0].AdapterID)
	}
}

func TestUnknownAdapter404(t *testing.T) {
	h := newHandler()
	code, _ := do(t, h, "GET", "/synthetic/nope/x")
	if code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown adapter, got %d", code)
	}
}
