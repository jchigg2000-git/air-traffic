package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const sseSample = "event: message_start\n" +
	`data: {"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":41,"output_tokens":1}}}` + "\n\n" +
	"event: content_block_delta\n" +
	`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}` + "\n\n" +
	"event: message_delta\n" +
	`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":17}}` + "\n\n" +
	"event: message_stop\n" +
	`data: {"type":"message_stop"}` + "\n\n"

func TestCopyStreamExtractsUsageByteFaithfully(t *testing.T) {
	rec := httptest.NewRecorder()
	in, out, err := copyStream(rec, strings.NewReader(sseSample))
	if err != nil {
		t.Fatalf("copyStream: %v", err)
	}
	if in != 41 || out != 17 {
		t.Errorf("usage = (%d, %d), want (41, 17)", in, out)
	}
	if rec.Body.String() != sseSample {
		t.Errorf("stream was not relayed byte-faithfully:\n%q", rec.Body.String())
	}
}

// Events split across read boundaries are the normal case on a real socket —
// the scanner must not lose a count that straddles two chunks.
func TestCopyStreamHandlesSplitEvents(t *testing.T) {
	rec := httptest.NewRecorder()
	in, out, err := copyStream(rec, iotestChunks(sseSample, 7))
	if err != nil {
		t.Fatalf("copyStream: %v", err)
	}
	if in != 41 || out != 17 {
		t.Errorf("usage across chunk boundaries = (%d, %d), want (41, 17)", in, out)
	}
	if rec.Body.String() != sseSample {
		t.Error("chunked relay was not byte-faithful")
	}
}

// A final event with no trailing newline still counts.
func TestCopyStreamFlushesTrailingLine(t *testing.T) {
	body := `data: {"type":"message_delta","usage":{"output_tokens":9}}`
	_, out, err := copyStream(httptest.NewRecorder(), strings.NewReader(body))
	if err != nil {
		t.Fatalf("copyStream: %v", err)
	}
	if out != 9 {
		t.Errorf("tokensOut = %d, want 9", out)
	}
}

// Junk, oversized lines, and non-JSON payloads degrade to "no usage", never to
// a broken relay or unbounded memory.
func TestCopyStreamToleratesJunk(t *testing.T) {
	junk := ": ping\n" + "data: not json\n" + "data: " + strings.Repeat("x", maxSSELineBytes+1024) + "\n" +
		`data: {"type":"message_delta","usage":{"output_tokens":4}}` + "\n"
	rec := httptest.NewRecorder()
	in, out, err := copyStream(rec, iotestChunks(junk, 4096))
	if err != nil {
		t.Fatalf("copyStream: %v", err)
	}
	if in != 0 || out != 4 {
		t.Errorf("usage = (%d, %d), want (0, 4)", in, out)
	}
	if rec.Body.Len() != len(junk) {
		t.Errorf("relayed %d bytes, want %d", rec.Body.Len(), len(junk))
	}
}

// End to end: a streamed request must report tokens on the spine like a
// non-streaming one, not silently drop to zero.
func TestStreamedRequestReportsUsageToMetrics(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		for _, ev := range strings.SplitAfter(sseSample, "\n\n") {
			if ev == "" {
				continue
			}
			_, _ = io.WriteString(w, ev)
			fl.Flush()
		}
	}))
	defer upstream.Close()

	gw := newTestGatewayServer(t, upstream.URL, discardLogger())
	srv := httptest.NewServer(gw.Routes())
	defer srv.Close()

	resp := postMessages(t, srv.URL, `{"stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	snap := gw.metrics.drain()
	if snap.TokensIn != 41 || snap.TokensOut != 17 {
		t.Errorf("streamed usage = (%d, %d), want (41, 17)", snap.TokensIn, snap.TokensOut)
	}
}

// iotestChunks returns a reader that hands out s in fixed-size pieces.
func iotestChunks(s string, size int) io.Reader { return &chunkReader{data: s, size: size} }

type chunkReader struct {
	data string
	size int
	pos  int
}

func (c *chunkReader) Read(p []byte) (int, error) {
	if c.pos >= len(c.data) {
		return 0, io.EOF
	}
	end := min(c.pos+min(c.size, len(p)), len(c.data))
	n := copy(p, c.data[c.pos:end])
	c.pos += n
	return n, nil
}
