package harness

// One ad-hoc prompt through the gateway — the harness UI's try-a-prompt box.
// Deliberately OUTSIDE the run lifecycle: user-typed content has no ground
// truth, so nothing here scores, promotes to the corpus, or feeds proposals
// (that would corrupt the ratchet). It may run at any time, including while
// a harness run is in progress — it touches no runner state.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"air-traffic/internal/model"
)

const (
	maxSampleBytes    = 8 << 10
	sampleSendTimeout = 15 * time.Second
	sampleJoinTimeout = 8 * time.Second
)

// RunSample sends content through the freshest gateway exactly like run
// traffic (same auth, same request-id join), then briefly polls for the
// gateway's report and the synthetic upstream's capture. A missing join is
// reported honestly (ReportJoined=false), not guessed at.
func (r *Runner) RunSample(content string) (model.SampleResult, error) {
	if strings.TrimSpace(content) == "" {
		return model.SampleResult{}, fmt.Errorf("content is empty")
	}
	if len(content) > maxSampleBytes {
		return model.SampleResult{}, fmt.Errorf("content exceeds %d bytes", maxSampleBytes)
	}
	gwURL, chain, err := r.freshGateway()
	if err != nil {
		return model.SampleResult{}, err
	}
	out := model.SampleResult{
		RequestID:     "sample-" + model.NewUUID()[:8],
		PackVersion:   r.store.GetPatternPack().Version,
		DetectorChain: chain,
	}

	body, err := json.Marshal(map[string]any{
		"model":    "claude-harness",
		"stream":   false,
		"messages": []map[string]any{{"role": "user", "content": content}},
	})
	if err != nil {
		return out, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), sampleSendTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gwURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.gatewayKey)
	req.Header.Set("X-Gateway-Request-Id", out.RequestID)

	start := time.Now()
	resp, err := r.httpc.Do(req)
	out.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return out, err
	}
	out.HTTPStatus = resp.StatusCode
	out.ResponseText = responseText(string(respBody))

	// The gateway pushes reports on its own cadence — poll briefly.
	deadline := time.Now().Add(sampleJoinTimeout)
	for {
		if rep, ok := r.store.GatewayReportByRequestID(out.RequestID); ok {
			out.Action = rep.Action
			out.Redactions = rep.Redactions
			out.ReportJoined = true
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if !out.ReportJoined && out.HTTPStatus == http.StatusBadRequest {
		out.Action = "block" // the block response is synchronous even when the report is late
	}
	// Upstream capture exists only on the synthetic upstream; against a real
	// vendor there is deliberately no capture to show.
	if capRec, ok := r.store.InferenceCaptureByRequestID(out.RequestID); ok {
		out.UpstreamText = upstreamContent(capRec.Body)
	}
	return out, nil
}
