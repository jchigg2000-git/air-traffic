package gateway

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

// maxSSELineBytes bounds the partial-line buffer the usage scanner keeps. A
// stream that never emits a newline must not grow it without limit; past the
// cap the scanner drops the fragment and resyncs at the next newline. Relaying
// is unaffected — bytes go to the caller as they arrive either way.
const maxSSELineBytes = 1 << 20

// copyStream relays an SSE body to the caller flushing per read, so event
// framing and pacing survive the proxy hop (design §9: stream bytes as they
// arrive; the caller must not wait for the full response). While relaying it
// tees the bytes through a usage scanner, so streamed requests report token
// counts like non-streaming ones do.
func copyStream(w http.ResponseWriter, body io.Reader) (tokensIn, tokensOut int64, err error) {
	fl, canFlush := w.(http.Flusher)
	var usage sseUsageScanner
	buf := make([]byte, 32<<10)
	for {
		n, rerr := body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return usage.tokensIn, usage.tokensOut, werr
			}
			if canFlush {
				fl.Flush()
			}
			usage.feed(buf[:n])
		}
		if rerr == io.EOF {
			usage.flush()
			return usage.tokensIn, usage.tokensOut, nil
		}
		if rerr != nil {
			return usage.tokensIn, usage.tokensOut, rerr
		}
	}
}

// sseUsageScanner extracts Anthropic token counts from a streamed response:
// message_start carries input_tokens, and message_delta carries the running
// output_tokens (the last one seen is the final count). It only ever reads the
// relayed bytes — it can neither alter nor delay them.
type sseUsageScanner struct {
	partial   []byte
	tokensIn  int64
	tokensOut int64
}

func (s *sseUsageScanner) feed(chunk []byte) {
	rest := chunk
	for {
		i := bytes.IndexByte(rest, '\n')
		if i < 0 {
			break
		}
		line := rest[:i]
		if len(s.partial) > 0 {
			line = append(s.partial, line...)
			s.partial = nil
		}
		s.scanLine(line)
		rest = rest[i+1:]
	}
	if len(rest) == 0 {
		return
	}
	if len(s.partial)+len(rest) > maxSSELineBytes {
		s.partial = nil // oversized event: resync at the next newline
		return
	}
	s.partial = append(s.partial, rest...)
}

// flush scans a trailing line that arrived without a terminating newline.
func (s *sseUsageScanner) flush() {
	if len(s.partial) > 0 {
		s.scanLine(s.partial)
		s.partial = nil
	}
}

func (s *sseUsageScanner) scanLine(line []byte) {
	line = bytes.TrimSuffix(bytes.TrimSpace(line), []byte("\r"))
	if !bytes.HasPrefix(line, []byte("data:")) {
		return
	}
	payload := bytes.TrimSpace(line[len("data:"):])
	if len(payload) == 0 || payload[0] != '{' {
		return
	}
	var ev struct {
		Type    string `json:"type"`
		Message struct {
			Usage sseUsage `json:"usage"`
		} `json:"message"`
		Usage sseUsage `json:"usage"`
	}
	if json.Unmarshal(payload, &ev) != nil {
		return
	}
	switch ev.Type {
	case "message_start":
		s.take(ev.Message.Usage)
	case "message_delta":
		s.take(ev.Usage)
	}
}

type sseUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

// take records the counts an event carried. output_tokens is cumulative across
// message_delta events, so the largest value seen is the total; input_tokens
// arrives once, on message_start.
func (s *sseUsageScanner) take(u sseUsage) {
	if u.InputTokens > s.tokensIn {
		s.tokensIn = u.InputTokens
	}
	if u.OutputTokens > s.tokensOut {
		s.tokensOut = u.OutputTokens
	}
}
