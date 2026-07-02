package synthetic

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"air-traffic/internal/store"
)

func newInferenceHandler(t *testing.T) (*Handler, *store.Store) {
	t.Helper()
	st := store.New()
	return New(st, slog.New(slog.NewTextHandler(io.Discard, nil))), st
}

func TestInferenceCapturesRequest(t *testing.T) {
	h, st := newInferenceHandler(t)
	body := `{"model":"claude-test","messages":[{"role":"user","content":"hello SSN 123-45-6789"}]}`
	req := httptest.NewRequest("POST", "/synthetic/anthropic/v1/messages", strings.NewReader(body))
	req.Header.Set("X-Gateway-Request-Id", "gw-abc123")
	req.Header.Set("x-api-key", "upstream-cred-value")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Type    string `json:"type"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if resp.Type != "message" || len(resp.Content) == 0 {
		t.Errorf("unexpected shape: %s", rec.Body.String())
	}

	cap, ok := st.InferenceCaptureByRequestID("gw-abc123")
	if !ok {
		t.Fatal("capture not recorded")
	}
	if cap.Body != body {
		t.Errorf("capture body = %q", cap.Body)
	}
	sum := sha256.Sum256([]byte("upstream-cred-value"))
	if want := "sha256:" + hex.EncodeToString(sum[:])[:16]; cap.AuthFingerprint != want {
		t.Errorf("fingerprint = %q, want %q", cap.AuthFingerprint, want)
	}
	if strings.Contains(cap.AuthFingerprint, "upstream-cred-value") {
		t.Error("credential stored raw")
	}
}

func TestInferenceEchoStraddleSSE(t *testing.T) {
	h, _ := newInferenceHandler(t)
	body := `{"model":"claude-test","stream":true,"messages":[{"role":"user","content":"patient SSN is 123-45-6789 thanks"}]}`
	req := httptest.NewRequest("POST", "/synthetic/anthropic/v1/messages", strings.NewReader(body))
	req.Header.Set("X-Harness-Echo", "input")
	req.Header.Set("X-Harness-Straddle", "on")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q", ct)
	}
	out := rec.Body.String()
	for _, ev := range []string{"message_start", "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop"} {
		if !strings.Contains(out, "event: "+ev) {
			t.Errorf("missing event %s", ev)
		}
	}
	// Straddle proof: no single delta line carries the whole SSN…
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "123-45-6789") {
			t.Errorf("SSN not straddled; whole value in one frame: %s", line)
		}
	}
	// …but the reassembled deltas do.
	var text strings.Builder
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err == nil && ev.Delta.Type == "text_delta" {
			text.WriteString(ev.Delta.Text)
		}
	}
	if !strings.Contains(text.String(), "123-45-6789") {
		t.Errorf("reassembled deltas missing the echoed SSN: %q", text.String())
	}
}

func TestInferenceHonorsScenario(t *testing.T) {
	h, st := newInferenceHandler(t)
	if _, err := st.SetScenario("anthropic", "500"); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/synthetic/anthropic/v1/messages", strings.NewReader(`{"messages":[]}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 500 {
		t.Errorf("status = %d, want scenario 500", rec.Code)
	}
}
