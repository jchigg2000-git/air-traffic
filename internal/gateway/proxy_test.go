package gateway

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"air-traffic/internal/gateway/config"
)

const (
	testClientKey    = "gwk-test-key"
	testUpstreamCred = "upstream-secret-cred"
)

func newTestGateway(t *testing.T, upstreamURL string) http.Handler {
	t.Helper()
	t.Setenv("GATEWAY_UPSTREAMS", fmt.Sprintf(`{"anthropic":{"base_url":%q,"credential_ref":"env:TEST_UPSTREAM_CRED"}}`, upstreamURL))
	t.Setenv("TEST_UPSTREAM_CRED", testUpstreamCred)
	t.Setenv("GATEWAY_CLIENT_KEYS_REF", "env:TEST_CLIENT_KEYS")
	t.Setenv("TEST_CLIENT_KEYS", testClientKey)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	gw, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return gw.Routes()
}

func TestRoundTripByteFaithfulAndCredentialSwap(t *testing.T) {
	upstreamBody := `{"id":"msg_1","type":"message","content":[{"type":"text","text":"hi"}]}`
	var seenAuth, seenBearer, seenBody string
	var seenHeaders http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		seenBody = string(b)
		seenAuth = r.Header.Get("x-api-key")
		seenBearer = r.Header.Get("Authorization")
		seenHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Anthropic-Request-Id", "req_upstream_1")
		_, _ = io.WriteString(w, upstreamBody)
	}))
	defer upstream.Close()

	gw := httptest.NewServer(newTestGateway(t, upstream.URL))
	defer gw.Close()

	reqBody := `{"model":"claude-test","messages":[{"role":"user","content":"hello"}]}`
	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/messages", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+testClientKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)

	if seenBody != reqBody {
		t.Errorf("upstream body = %q, want byte-faithful %q", seenBody, reqBody)
	}
	if seenAuth != testUpstreamCred {
		t.Errorf("upstream x-api-key = %q, want resolved credential", seenAuth)
	}
	if seenBearer != "" {
		t.Errorf("client Authorization leaked upstream: %q", seenBearer)
	}
	for k, vs := range seenHeaders {
		for _, v := range vs {
			if strings.Contains(v, testClientKey) {
				t.Errorf("client key leaked upstream in header %s", k)
			}
		}
	}
	if string(got) != upstreamBody {
		t.Errorf("response = %q, want byte-faithful %q", got, upstreamBody)
	}
	if resp.Header.Get("Anthropic-Request-Id") != "req_upstream_1" {
		t.Errorf("upstream headers not forwarded")
	}
	if resp.Header.Get("X-Gateway-Request-Id") == "" {
		t.Errorf("missing X-Gateway-Request-Id on response")
	}
	if strings.Contains(string(got), testUpstreamCred) {
		t.Errorf("upstream credential leaked to client")
	}
}

func TestRejectsBadClientKey(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream must not be reached on auth failure")
	}))
	defer upstream.Close()
	gw := httptest.NewServer(newTestGateway(t, upstream.URL))
	defer gw.Close()

	for _, auth := range []string{"", "Bearer wrong-key"} {
		req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/messages", strings.NewReader(`{}`))
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("do: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("auth %q: status = %d, want 401", auth, resp.StatusCode)
		}
	}
}

func TestSSEPassThroughPreservesFraming(t *testing.T) {
	events := "event: message_start\ndata: {\"type\":\"message_start\"}\n\n" +
		"event: content_block_delta\ndata: {\"delta\":{\"text\":\"hi\"}}\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		for _, ev := range strings.SplitAfter(events, "\n\n") {
			if ev == "" {
				continue
			}
			_, _ = io.WriteString(w, ev)
			fl.Flush()
		}
	}))
	defer upstream.Close()
	gw := httptest.NewServer(newTestGateway(t, upstream.URL))
	defer gw.Close()

	req, _ := http.NewRequest(http.MethodPost, gw.URL+"/v1/messages", strings.NewReader(`{"stream":true}`))
	req.Header.Set("Authorization", "Bearer "+testClientKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q", ct)
	}
	got, _ := io.ReadAll(resp.Body)
	if string(got) != events {
		t.Errorf("SSE framing altered:\ngot  %q\nwant %q", got, events)
	}
}
