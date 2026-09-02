package synthetic

// The synthetic inference upstream: an Anthropic-Messages-shaped endpoint the
// gateway forwards to during harness runs and tests. It records every request
// it receives (body bounded to captureKeepBytes — harness traffic is synthetic
// by construction; NEVER point real traffic here) so the harness can prove
// behaviorally that seeded PII was redacted before the "vendor" saw it.
//
// Harness controls (request headers):
//   X-Harness-Echo: input   — assistant reply echoes the user text back
//   X-Harness-Straddle: on  — SSE deltas split at hostile boundaries (mid-SSN)

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jchigg2000-git/air-traffic/internal/model"
)

const (
	inferenceMaxBody = 10 << 20 // read cap; matches the gateway's GATEWAY_MAX_BODY_BYTES default
	captureKeepBytes = 64 << 10 // bytes of body retained per capture; see handleInference
)

func isInferencePath(vendorID, nativePath string) bool {
	return vendorID == "anthropic" && nativePath == "/v1/messages"
}

type messagesRequest struct {
	Model    string `json:"model"`
	Stream   bool   `json:"stream"`
	System   any    `json:"system"`
	Messages []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"messages"`
}

func (h *Handler) handleInference(w http.ResponseWriter, r *http.Request, a model.Adapter, nativePath string) {
	start := time.Now()
	if r.Method != http.MethodPost {
		s, b := vendorError(a.ID, http.StatusMethodNotAllowed, "invalid_request_error", "use POST")
		writeJSON(w, s, b)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, inferenceMaxBody))
	if err != nil {
		s, b := vendorError(a.ID, http.StatusBadRequest, "invalid_request_error", "unreadable body")
		writeJSON(w, s, b)
		return
	}

	var req messagesRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		s, b := vendorError(a.ID, http.StatusBadRequest, "invalid_request_error", "body is not valid JSON")
		writeJSON(w, s, b)
		return
	}

	// Keep a bounded prefix: the ring is count-capped (store ringMax), so an
	// unbounded body would let anyone reaching this port hold 5000 x 10 MB in
	// memory. Harness prompts are <= 8 KB, so scoring never sees the cut.
	body, truncated := string(raw), false
	if len(body) > captureKeepBytes {
		body, truncated = body[:captureKeepBytes], true
	}
	h.store.RecordInferenceCapture(model.InferenceCapture{
		AdapterID:        a.ID,
		GatewayRequestID: r.Header.Get("X-Gateway-Request-Id"),
		Path:             nativePath,
		Body:             body,
		Truncated:        truncated,
		AuthFingerprint:  authFingerprint(r),
		Stream:           req.Stream,
		ReceivedAt:       time.Now().UTC(),
	})
	defer func() {
		h.store.RecordCall(model.CallRecord{
			AdapterID: a.ID, Method: r.Method, Path: nativePath,
			Scenario: a.Scenario, StatusCode: http.StatusOK,
			DurationMS: time.Since(start).Milliseconds(),
		})
	}()

	// Scenario overrides are honored deliberately: scenario 500/timeout lets
	// the harness exercise the gateway's upstream-error handling for free.
	if handled := h.writeScenario(w, a); handled {
		return
	}

	reply := "Synthetic response from the air-traffic mock upstream."
	if r.Header.Get("X-Harness-Echo") == "input" {
		if echoed := userText(req); echoed != "" {
			reply = echoed
		}
	}
	modelName := req.Model
	if modelName == "" {
		modelName = "claude-synthetic"
	}
	usageIn, usageOut := len(raw)/4, len(reply)/4

	if req.Stream {
		h.streamInference(w, r, modelName, reply, usageIn, usageOut)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":            fmt.Sprintf("msg_synth_%d", time.Now().UnixNano()),
		"type":          "message",
		"role":          "assistant",
		"model":         modelName,
		"content":       []map[string]any{{"type": "text", "text": reply}},
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"usage":         map[string]int{"input_tokens": usageIn, "output_tokens": usageOut},
	})
}

// streamInference emits a well-formed Anthropic SSE event sequence. With
// X-Harness-Straddle: on, text deltas are cut every few bytes so any seeded
// value is guaranteed to split across chunks — the signature streaming test.
func (h *Handler) streamInference(w http.ResponseWriter, r *http.Request, modelName, reply string, usageIn, usageOut int) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "streaming unsupported"})
		return
	}
	chunk := 48
	if r.Header.Get("X-Harness-Straddle") == "on" {
		chunk = 5
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	event := func(name string, payload any) {
		b, _ := json.Marshal(payload)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, b)
		fl.Flush()
	}

	id := fmt.Sprintf("msg_synth_%d", time.Now().UnixNano())
	event("message_start", map[string]any{"type": "message_start", "message": map[string]any{
		"id": id, "type": "message", "role": "assistant", "model": modelName,
		"content": []any{}, "usage": map[string]int{"input_tokens": usageIn, "output_tokens": 0},
	}})
	event("content_block_start", map[string]any{"type": "content_block_start", "index": 0,
		"content_block": map[string]any{"type": "text", "text": ""}})
	for i := 0; i < len(reply); i += chunk {
		end := min(i+chunk, len(reply))
		event("content_block_delta", map[string]any{"type": "content_block_delta", "index": 0,
			"delta": map[string]any{"type": "text_delta", "text": reply[i:end]}})
	}
	event("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
	event("message_delta", map[string]any{"type": "message_delta",
		"delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil},
		"usage": map[string]int{"output_tokens": usageOut}})
	event("message_stop", map[string]any{"type": "message_stop"})
}

// userText flattens the request's user-visible text (string contents and text
// content blocks) for the echo control.
func userText(req messagesRequest) string {
	var parts []string
	for _, m := range req.Messages {
		if m.Role != "user" {
			continue
		}
		var asString string
		if err := json.Unmarshal(m.Content, &asString); err == nil {
			parts = append(parts, asString)
			continue
		}
		var blocks []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(m.Content, &blocks); err == nil {
			for _, b := range blocks {
				if b.Type == "text" && b.Text != "" {
					parts = append(parts, b.Text)
				}
			}
		}
	}
	return strings.Join(parts, "\n")
}

// authFingerprint identifies which credential reached the upstream without
// ever storing the credential: SHA-256 prefix only.
func authFingerprint(r *http.Request) string {
	cred := r.Header.Get("x-api-key")
	if cred == "" {
		cred = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	if cred == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(cred))
	return "sha256:" + hex.EncodeToString(sum[:])[:16]
}
