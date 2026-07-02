package gateway

import (
	"io"
	"net/http"
)

// copyStream relays an SSE body to the caller flushing per read, so event
// framing and pacing survive the proxy hop (design §9: stream bytes as they
// arrive; the caller must not wait for the full response).
func copyStream(w http.ResponseWriter, body io.Reader) error {
	fl, canFlush := w.(http.Flusher)
	buf := make([]byte, 32<<10)
	for {
		n, err := body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
			if canFlush {
				fl.Flush()
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
