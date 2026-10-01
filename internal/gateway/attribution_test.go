package gateway

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jchigg2000-git/air-traffic/internal/gateway/config"
)

// A gateway fronting both dialects must attribute each aggregate to the route
// that served it. Before, every observation was stamped "anthropic", so
// OpenAI-dialect traffic was counted against the wrong route.
func TestAggregatesAreAttributedToTheServingRoute(t *testing.T) {
	anth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"type":"message","usage":{"input_tokens":10,"output_tokens":2}}`)
	}))
	defer anth.Close()
	oai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"id":"c1","usage":{"prompt_tokens":5,"completion_tokens":7}}`)
	}))
	defer oai.Close()

	fcp := &fakeControlPlane{policyJSON: `{"policy":null}`, packJSON: `{"pack":{"version":0,"rules":[]}}`}
	cp := httptest.NewServer(fcp.handler(t))
	defer cp.Close()
	t.Setenv("GATEWAY_CONTROL_PLANE_URL", cp.URL)
	t.Setenv("GATEWAY_REDACT_ACTION", "mask")
	gw := newDualRouteServer(t, anth.URL, oai.URL+"/v1")
	srv := httptest.NewServer(gw.Routes())
	defer srv.Close()

	for range 2 {
		postMessages(t, srv.URL, piiBody).Body.Close()
	}
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set("Authorization", "Bearer "+testClientKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	resp.Body.Close()
	// A refused caller on the OpenAI route is that route's auth failure.
	bad, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/chat/completions", strings.NewReader(`{}`))
	bad.Header.Set("Authorization", "Bearer wrong")
	if resp, err = http.DefaultClient.Do(bad); err != nil {
		t.Fatalf("do: %v", err)
	}
	resp.Body.Close()

	gw.pushObservations(context.Background())

	fcp.mu.Lock()
	defer fcp.mu.Unlock()
	if len(fcp.observations) != 1 {
		t.Fatalf("observation pushes = %d, want 1", len(fcp.observations))
	}
	byVendor := map[string]map[string]float64{}
	for _, o := range fcp.observations[0]["observations"].([]any) {
		om := o.(map[string]any)
		dims := om["dimensions"].(map[string]any)
		vendor := dims["vendor"].(string)
		if dims["route"] != vendor {
			t.Errorf("observation vendor %q but route %v: the two must name the same serving route", vendor, dims["route"])
		}
		sig := om["signal"].(map[string]any)
		if byVendor[vendor] == nil {
			byVendor[vendor] = map[string]float64{}
		}
		if _, isType := dims["pii_type"]; !isType {
			byVendor[vendor][sig["name"].(string)] = sig["value"].(float64)
		}
	}
	for vendor, want := range map[string]struct{ requests, tokensIn, authFailures float64 }{
		"anthropic": {2, 20, 0},
		"openai":    {1, 5, 1},
	} {
		got := byVendor[vendor]
		if got["gw_requests"] != want.requests || got["tokens_in"] != want.tokensIn || got["gw_auth_failures"] != want.authFailures {
			t.Errorf("%s aggregate = %+v, want requests=%v tokens_in=%v auth_failures=%v",
				vendor, got, want.requests, want.tokensIn, want.authFailures)
		}
	}
	if len(byVendor) != 2 {
		t.Errorf("observation vendors = %v, want exactly anthropic and openai", byVendor)
	}
}

// The heartbeat must not claim a vendor whose route has no upstream behind
// it: a gateway wired to an OpenAI-compatible router only would otherwise tell
// the control plane it enforces redaction for Anthropic traffic it answers
// with a 502.
func TestHeartbeatClaimsOnlyRoutesThatHaveAnUpstream(t *testing.T) {
	fcp := &fakeControlPlane{policyJSON: `{"policy":null}`, packJSON: `{"pack":{"version":0,"rules":[]}}`}
	cp := httptest.NewServer(fcp.handler(t))
	defer cp.Close()
	t.Setenv("GATEWAY_CONTROL_PLANE_URL", cp.URL)
	t.Setenv("GATEWAY_REDACT_ACTION", "mask")
	t.Setenv("GATEWAY_UPSTREAMS", fmt.Sprintf(`{"openai":{"base_url":%q,"credential_ref":"env:TEST_OPENAI_CRED"}}`, "http://unused.invalid/v1"))
	t.Setenv("TEST_OPENAI_CRED", "openai-secret-cred")
	t.Setenv("GATEWAY_CLIENT_KEYS_REF", "env:TEST_CLIENT_KEYS")
	t.Setenv("TEST_CLIENT_KEYS", testClientKey)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	gw, err := New(cfg, discardLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := gw.pushHeartbeat(context.Background()); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	fcp.mu.Lock()
	defer fcp.mu.Unlock()
	if len(fcp.heartbeats) != 1 {
		t.Fatalf("heartbeats = %d", len(fcp.heartbeats))
	}
	if vendors := fcp.heartbeats[0]["vendors"].(map[string]any); len(vendors) != 0 {
		t.Errorf("openai-only gateway claimed enforcement: %v", vendors)
	}
}
