package harness

// The flywheel's raw-score evidence source: a minimal Presidio /analyze
// client asking for every span the engine saw, however weakly (threshold 0,
// no pack recognizers) — evidence, not enforcement. Deliberately NOT the
// gateway's detect package: the control-plane binary must never link gateway
// packages (G0 dependency isolation, enforced by cmd/air-traffic-server's
// depisolation test). The shared vocabulary lives in internal/model so probe
// and enforcement stay in step.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"air-traffic/internal/model"
)

type presidioProbe struct {
	url  string
	http *http.Client
}

// probeSpan is one raw engine observation in gateway vocabulary and byte
// offsets. Values are never stored — offsets and scores only.
type probeSpan struct {
	Start, End int
	Type       string
	Score      float64
}

func newPresidioProbe(url string) *presidioProbe {
	return &presidioProbe{url: url, http: &http.Client{}}
}

func (p *presidioProbe) analyzeRaw(ctx context.Context, text string) ([]probeSpan, error) {
	body, err := json.Marshal(map[string]any{
		"text": text, "language": "en", "score_threshold": 0,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url+"/analyze", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("presidio analyze returned %d", resp.StatusCode)
	}
	var results []struct {
		EntityType string  `json:"entity_type"`
		Start      int     `json:"start"`
		End        int     `json:"end"`
		Score      float64 `json:"score"`
	}
	// Bound the sidecar read, matching the enforcement-side Presidio client.
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&results); err != nil {
		return nil, fmt.Errorf("presidio response: %w", err)
	}

	runeToByte := model.RuneToByteOffsets(text)
	var spans []probeSpan
	for _, r := range results {
		typ, mapped := model.PresidioEntityMap[r.EntityType]
		if !mapped {
			typ = r.EntityType
		}
		if typ == "" {
			continue
		}
		if r.Start < 0 || r.End > len(runeToByte)-1 || r.Start >= r.End {
			continue
		}
		spans = append(spans, probeSpan{
			Start: runeToByte[r.Start], End: runeToByte[r.End],
			Type: typ, Score: r.Score,
		})
	}
	return spans, nil
}
