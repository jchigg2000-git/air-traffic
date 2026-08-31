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

// usageScanner reads token counts off a relayed SSE stream. Implementations
// see only a copy of the bytes already on their way to the caller: a scanner
// can neither alter nor delay the stream, which is why a dialect it cannot
// parse degrades to zero counts rather than to a broken response.
type usageScanner interface {
	feed(chunk []byte)
	flush()
	totals() (tokensIn, tokensOut int64)
}

// copyStream relays an Anthropic SSE body with the Anthropic usage scanner.
// Test-only wrapper: both proxy routes call copyStreamWith directly with the
// scanner their own dialect supplies (proxy.go).
func copyStream(w http.ResponseWriter, body io.Reader) (tokensIn, tokensOut int64, err error) {
	return copyStreamWith(w, body, newAnthropicUsageScanner())
}

// copyStreamWith relays an SSE body to the caller flushing per read, so event
// framing and pacing survive the proxy hop (design §9: stream bytes as they
// arrive; the caller must not wait for the full response). While relaying it
// tees the bytes through the given usage scanner, so streamed requests report
// token counts like non-streaming ones do.
func copyStreamWith(w http.ResponseWriter, body io.Reader, usage usageScanner) (tokensIn, tokensOut int64, err error) {
	fl, canFlush := w.(http.Flusher)
	buf := make([]byte, 32<<10)
	for {
		n, rerr := body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				in, out := usage.totals()
				return in, out, werr
			}
			if canFlush {
				fl.Flush()
			}
			usage.feed(buf[:n])
		}
		if rerr == io.EOF {
			usage.flush()
			in, out := usage.totals()
			return in, out, nil
		}
		if rerr != nil {
			in, out := usage.totals()
			return in, out, rerr
		}
	}
}

// sseLineReader splits a relayed SSE stream into lines, carrying the partial
// tail across reads. Both dialect scanners embed it and supply scan.
type sseLineReader struct {
	partial []byte
	scan    func(line []byte)
}

func (r *sseLineReader) feed(chunk []byte) {
	rest := chunk
	for {
		i := bytes.IndexByte(rest, '\n')
		if i < 0 {
			break
		}
		line := rest[:i]
		if len(r.partial) > 0 {
			line = append(r.partial, line...)
			r.partial = nil
		}
		r.scan(line)
		rest = rest[i+1:]
	}
	if len(rest) == 0 {
		return
	}
	if len(r.partial)+len(rest) > maxSSELineBytes {
		r.partial = nil // oversized event: resync at the next newline
		return
	}
	r.partial = append(r.partial, rest...)
}

// flush scans a trailing line that arrived without a terminating newline.
func (r *sseLineReader) flush() {
	if len(r.partial) > 0 {
		r.scan(r.partial)
		r.partial = nil
	}
}

// sseData returns the JSON payload of a `data:` line, or nil for anything else
// (comments, event: lines, the terminal `data: [DONE]`).
func sseData(line []byte) []byte {
	line = bytes.TrimSuffix(bytes.TrimSpace(line), []byte("\r"))
	if !bytes.HasPrefix(line, []byte("data:")) {
		return nil
	}
	payload := bytes.TrimSpace(line[len("data:"):])
	if len(payload) == 0 || payload[0] != '{' {
		return nil
	}
	return payload
}

// tokenTally keeps the high-water mark of each count. Both dialects report
// output tokens cumulatively across events, so the largest value seen is the
// total; taking the max also makes a duplicated or replayed event harmless.
type tokenTally struct {
	tokensIn  int64
	tokensOut int64
}

func (t *tokenTally) take(in, out int64) {
	if in > t.tokensIn {
		t.tokensIn = in
	}
	if out > t.tokensOut {
		t.tokensOut = out
	}
}

func (t *tokenTally) totals() (int64, int64) { return t.tokensIn, t.tokensOut }

// anthropicUsageScanner extracts Anthropic token counts from a streamed
// response: message_start carries input_tokens, and message_delta carries the
// running output_tokens.
type anthropicUsageScanner struct {
	sseLineReader
	tokenTally
}

func newAnthropicUsageScanner() *anthropicUsageScanner {
	s := &anthropicUsageScanner{}
	s.scan = s.scanLine
	return s
}

func (s *anthropicUsageScanner) scanLine(line []byte) {
	payload := sseData(line)
	if payload == nil {
		return
	}
	var ev struct {
		Type    string `json:"type"`
		Message struct {
			Usage anthropicUsage `json:"usage"`
		} `json:"message"`
		Usage anthropicUsage `json:"usage"`
	}
	if json.Unmarshal(payload, &ev) != nil {
		return
	}
	switch ev.Type {
	case "message_start":
		s.take(ev.Message.Usage.InputTokens, ev.Message.Usage.OutputTokens)
	case "message_delta":
		s.take(ev.Usage.InputTokens, ev.Usage.OutputTokens)
	}
}

type anthropicUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

// openAIUsageScanner extracts OpenAI chat-completions token counts. Unlike
// Anthropic's, these arrive only if the caller asked for them: a terminal
// chunk carrying a non-null usage object, emitted when the request set
// stream_options.include_usage. Without that flag the counts are legitimately
// absent and this scanner reports zero rather than guessing.
type openAIUsageScanner struct {
	sseLineReader
	tokenTally
}

func newOpenAIUsageScanner() *openAIUsageScanner {
	s := &openAIUsageScanner{}
	s.scan = s.scanLine
	return s
}

func (s *openAIUsageScanner) scanLine(line []byte) {
	payload := sseData(line)
	if payload == nil {
		return
	}
	var ev struct {
		Usage *openAIUsage `json:"usage"`
	}
	if json.Unmarshal(payload, &ev) != nil || ev.Usage == nil {
		return
	}
	s.take(ev.Usage.PromptTokens, ev.Usage.CompletionTokens)
}

type openAIUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
}
