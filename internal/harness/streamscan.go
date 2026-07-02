package harness

import (
	"encoding/json"
	"strings"
)

// reassembleSSE concatenates text_delta payloads from a raw SSE body — the
// harness-side reassembly that proves a value split across chunks is still
// found (design §10's signature streaming case). Chunk boundaries cannot hide
// a value from a scan over the reassembled stream.
func reassembleSSE(raw string) string {
	var b strings.Builder
	for _, line := range strings.Split(raw, "\n") {
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		var ev struct {
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err == nil && ev.Delta.Type == "text_delta" {
			b.WriteString(ev.Delta.Text)
		}
	}
	return b.String()
}

// responseText extracts the assistant text from a non-streaming Messages
// response body.
func responseText(raw string) string {
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return ""
	}
	var b strings.Builder
	for _, c := range resp.Content {
		if c.Type == "text" {
			b.WriteString(c.Text)
		}
	}
	return b.String()
}
