package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeControlPlane captures spine pushes and serves policy/baselines/patterns.
type fakeControlPlane struct {
	mu           sync.Mutex
	observations []map[string]any
	leakPushes   []map[string]any
	heartbeats   []map[string]any
	policyJSON   string
	packJSON     string
}

func (f *fakeControlPlane) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	capture := func(dst *[]map[string]any) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			b, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(b, &body); err != nil {
				t.Errorf("bad push body: %v", err)
			}
			f.mu.Lock()
			*dst = append(*dst, body)
			f.mu.Unlock()
			w.WriteHeader(http.StatusAccepted)
		}
	}
	mux.HandleFunc("/api/observations", capture(&f.observations))
	mux.HandleFunc("/api/gateway/leaks", capture(&f.leakPushes))
	mux.HandleFunc("/api/gateway/enforcement", capture(&f.heartbeats))
	mux.HandleFunc("/api/policies", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, f.policyJSON)
	})
	mux.HandleFunc("/api/baselines", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"baselines":[
			{"id":"general_saas","zdr":"off","pii_redaction":"off"},
			{"id":"fintech","zdr":"where_native","pii_redaction":"on"},
			{"id":"healthcare","zdr":"enforced","pii_redaction":"on+phi"}
		]}`)
	})
	mux.HandleFunc("/api/gateway/patterns", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, f.packJSON)
	})
	return mux
}

func newSpineGateway(t *testing.T, upstreamURL string) (*Server, *fakeControlPlane) {
	t.Helper()
	fcp := &fakeControlPlane{policyJSON: `{"policy":null}`, packJSON: `{"pack":{"version":0,"rules":[]}}`}
	cp := httptest.NewServer(fcp.handler(t))
	t.Cleanup(cp.Close)
	t.Setenv("GATEWAY_CONTROL_PLANE_URL", cp.URL)
	t.Setenv("GATEWAY_REDACT_ACTION", "per_policy")
	return newTestGatewayServer(t, upstreamURL, discardLogger()), fcp
}

func TestSpinePushObservationsReportsHeartbeat(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"type":"message","usage":{"input_tokens":42,"output_tokens":7}}`)
	}))
	defer upstream.Close()
	gw, fcp := newSpineGateway(t, upstream.URL)
	srv := httptest.NewServer(gw.Routes())
	defer srv.Close()

	resp := postMessages(t, srv.URL, piiBody) // default action per_policy → mask
	resp.Body.Close()

	ctx := context.Background()
	gw.pushObservations(ctx)
	gw.pushReports(ctx)
	gw.pushHeartbeat(ctx)

	fcp.mu.Lock()
	defer fcp.mu.Unlock()
	if len(fcp.observations) != 1 {
		t.Fatalf("observation pushes = %d", len(fcp.observations))
	}
	batch := fcp.observations[0]
	if batch["contract"] != "ops-observation-batch/v1" {
		t.Errorf("contract = %v", batch["contract"])
	}
	conn := batch["connector"].(map[string]any)
	if conn["type"] != "gateway" {
		t.Errorf("connector.type = %v", conn["type"])
	}
	obsJSON, _ := json.Marshal(batch["observations"])
	for _, want := range []string{"gw_requests", "gw_redactions", "gw_added_latency_ms_p95", "tokens_in"} {
		if !strings.Contains(string(obsJSON), want) {
			t.Errorf("observations missing %s", want)
		}
	}

	if len(fcp.leakPushes) != 1 {
		t.Fatalf("leak pushes = %d", len(fcp.leakPushes))
	}
	reports := fcp.leakPushes[0]["reports"].([]any)
	if len(reports) != 1 {
		t.Errorf("reports = %d", len(reports))
	}

	if len(fcp.heartbeats) != 1 {
		t.Fatalf("heartbeats = %d", len(fcp.heartbeats))
	}
	hb := fcp.heartbeats[0]
	vendors := hb["vendors"].(map[string]any)
	if _, ok := vendors["anthropic"]; !ok {
		t.Errorf("heartbeat should claim anthropic while masking: %v", hb)
	}
}

func TestPolicyPullDerivesPreCoverageGate(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()
	gw, fcp := newSpineGateway(t, upstream.URL)
	ctx := context.Background()

	// healthcare, ZDR not attested → block (the pre-coverage gate)
	fcp.policyJSON = `{"policy":{"baseline":"healthcare","vendor_defaults":{},"vendors":{},"agentic":{},"budget":{}}}`
	gw.pullOnce(ctx)
	got, _ := gw.actionFor(principal{})
	if got != actionBlock {
		t.Errorf("healthcare unattested action = %q, want block", got)
	}

	// attest ZDR → downgrade to mask, no restart
	fcp.policyJSON = `{"policy":{"baseline":"healthcare","vendor_defaults":{},"vendors":{"anthropic":{"zdr_attested":true}},"agentic":{},"budget":{}}}`
	gw.pullOnce(ctx)
	got, _ = gw.actionFor(principal{})
	if got != actionMask {
		t.Errorf("healthcare attested action = %q, want mask", got)
	}

	// general_saas → detect-only (monitoring, not enforcement)
	fcp.policyJSON = `{"policy":{"baseline":"general_saas","vendor_defaults":{},"vendors":{},"agentic":{},"budget":{}}}`
	gw.pullOnce(ctx)
	got, _ = gw.actionFor(principal{})
	if got != actionDetect {
		t.Errorf("general_saas action = %q, want detect", got)
	}
	gw.pushHeartbeat(ctx)
	fcp.mu.Lock()
	last := fcp.heartbeats[len(fcp.heartbeats)-1]
	fcp.mu.Unlock()
	if vendors := last["vendors"].(map[string]any); len(vendors) != 0 {
		t.Errorf("detect-only heartbeat must not claim enforcement: %v", vendors)
	}
}

// The control plane's spine routes are key-gated for non-loopback callers, so
// a configured gateway must present the key on every push AND every pull.
func TestSpineRequestsCarryTheSharedKey(t *testing.T) {
	const key = "spine-test-key"
	var mu sync.Mutex
	seen := map[string]string{}
	cp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.URL.Path] = r.Header.Get("Authorization")
		mu.Unlock()
		switch r.URL.Path {
		case "/api/gateway/patterns":
			_, _ = io.WriteString(w, `{"pack":{"version":0,"rules":[]}}`)
		case "/api/policies":
			_, _ = io.WriteString(w, `{"policy":null}`)
		default:
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	defer cp.Close()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()

	t.Setenv("GATEWAY_CONTROL_PLANE_URL", cp.URL)
	t.Setenv("GATEWAY_CONTROL_PLANE_KEY_REF", "env:TEST_SPINE_KEY")
	t.Setenv("TEST_SPINE_KEY", key)
	gw := newTestGatewayServer(t, upstream.URL, discardLogger())

	ctx := context.Background()
	gw.pushHeartbeat(ctx)
	gw.pullOnce(ctx)

	mu.Lock()
	defer mu.Unlock()
	for _, path := range []string{"/api/gateway/enforcement", "/api/gateway/patterns"} {
		if got := seen[path]; got != "Bearer "+key {
			t.Errorf("%s Authorization = %q, want bearer spine key", path, got)
		}
	}
}

// No key configured is a supported (loopback-only) posture, not a boot error:
// the gateway comes up and simply sends no credential.
func TestSpineKeyOptional(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()
	t.Setenv("GATEWAY_CONTROL_PLANE_KEY_REF", "env:TEST_ABSENT_SPINE_KEY")
	gw := newTestGatewayServer(t, upstream.URL, discardLogger())
	if gw.spineKey != "" {
		t.Errorf("spineKey = %q, want empty", gw.spineKey)
	}
}

func TestPatternPullHotReloads(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()
	gw, fcp := newSpineGateway(t, upstream.URL)
	ctx := context.Background()

	text := "the SSN is 123456789 ok"
	spans, _ := gw.regexDet.Detect(ctx, text)
	if len(spans) != 0 {
		t.Fatalf("bare SSN should miss before the pack: %+v", spans)
	}

	fcp.packJSON = `{"pack":{"version":1,"rules":[{"id":"ssn-bare","type":"SSN","regex":"(?i)\\bSSN is (\\d{9})\\b","confidence":0.7}]}}`
	gw.pullOnce(ctx)
	spans, _ = gw.regexDet.Detect(ctx, text)
	if len(spans) == 0 {
		t.Error("pack v1 rule not live after pull")
	}
	if gw.packVersion.Load() != 1 {
		t.Errorf("packVersion = %d", gw.packVersion.Load())
	}
}

// A pack the regex engine refuses must install nothing — not even the
// chain-level allow-list, which used to go in first and stayed in force with
// the recognizers it shipped with never loaded — and must not re-fail on every
// pull for as long as the control plane keeps serving it.
func TestRejectedPatternPackInstallsNothing(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()
	gw, fcp := newSpineGateway(t, upstream.URL)
	ctx := context.Background()

	text := "ssn 123-45-6789 here"
	fcp.packJSON = `{"pack":{"version":4,"rules":[` +
		`{"id":"bad","type":"MRN","regex":"(unclosed"},` +
		`{"id":"hush","type":"SSN","kind":"allow_list","allow_list":["123-45-6789"]}]}}`
	if err := gw.pullPatterns(ctx); err == nil {
		t.Fatal("want the uncompilable rule to reject the pack")
	}
	if spans, _ := gw.chain.Run(ctx, text); len(spans) == 0 {
		t.Error("rejected pack's allow-list suppressed a span it never earned")
	}
	if err := gw.pullPatterns(ctx); err != nil {
		t.Errorf("rejected pack re-fires on the next pull: %v", err)
	}

	// The fixed pack arrives as a new version and reloads normally.
	fcp.packJSON = `{"pack":{"version":5,"rules":[{"id":"hush","type":"SSN","kind":"allow_list","allow_list":["123-45-6789"]}]}}`
	if err := gw.pullPatterns(ctx); err != nil {
		t.Fatalf("pull v5: %v", err)
	}
	if spans, _ := gw.chain.Run(ctx, text); len(spans) != 0 {
		t.Errorf("allow-list not live after the fixed pack: %+v", spans)
	}
}
