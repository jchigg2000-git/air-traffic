package detect

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
)

// Presidio calls a self-hosted presidio-analyzer sidecar over HTTP — the
// heavy NER tier that catches what regex can't (names, addresses, free-text
// PHI). Self-hosted deliberately: payloads never leave the local boundary
// (design §6, §11). Approved pattern-pack rules ride along per call as
// ad_hoc_recognizers, so the flywheel extends Presidio without a container
// restart.
type Presidio struct {
	url  string
	http *http.Client
	pack atomic.Pointer[[]PatternRule]
}

func NewPresidio(url string) *Presidio {
	p := &Presidio{url: url, http: &http.Client{}}
	empty := []PatternRule{}
	p.pack.Store(&empty)
	return p
}

func (p *Presidio) Name() string { return "presidio" }

func (p *Presidio) SetPatternPack(rules []PatternRule) {
	p.pack.Store(&rules)
}

// presidioTypeMap normalizes Presidio entity types onto the gateway's
// vocabulary. Types mapping to "" are dropped (DATE_TIME is FP noise against
// the semver/order-number traps).
var presidioTypeMap = map[string]string{
	"PERSON":        "PERSON_NAME",
	"LOCATION":      "ADDRESS",
	"EMAIL_ADDRESS": "EMAIL",
	"PHONE_NUMBER":  "PHONE",
	"US_SSN":        "SSN",
	"CREDIT_CARD":   "CREDIT_CARD",
	"IP_ADDRESS":    "IP",
	"IBAN_CODE":     "IBAN",
	"MEDICAL_LICENSE": "MRN",
	"DATE_TIME":     "",
	"URL":           "",
	"NRP":           "",
}

type presidioResult struct {
	EntityType string  `json:"entity_type"`
	Start      int     `json:"start"`
	End        int     `json:"end"`
	Score      float64 `json:"score"`
}

func (p *Presidio) Detect(ctx context.Context, text string) ([]Span, error) {
	payload := map[string]any{
		"text":            text,
		"language":        "en",
		"score_threshold": 0.4,
	}
	if rules := *p.pack.Load(); len(rules) > 0 {
		var adhoc []map[string]any
		for _, r := range rules {
			adhoc = append(adhoc, map[string]any{
				"name":               "pack-" + r.ID,
				"supported_language": "en",
				"supported_entity":   r.Type,
				"patterns": []map[string]any{
					{"name": r.ID, "regex": r.Regex, "score": r.Confidence},
				},
			})
		}
		payload["ad_hoc_recognizers"] = adhoc
	}
	body, err := json.Marshal(payload)
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
	var results []presidioResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("presidio response: %w", err)
	}

	// Presidio offsets are unicode codepoints; Go spans are byte offsets.
	runeToByte := runeOffsets(text)
	var spans []Span
	for _, r := range results {
		typ, mapped := presidioTypeMap[r.EntityType]
		if !mapped {
			typ = r.EntityType // ad_hoc pack types come back verbatim
		}
		if typ == "" {
			continue
		}
		if r.Start < 0 || r.End > len(runeToByte)-1 || r.Start >= r.End {
			continue
		}
		spans = append(spans, Span{
			Start: runeToByte[r.Start], End: runeToByte[r.End],
			Type: typ, Confidence: r.Score, Detector: "presidio",
		})
	}
	return spans, nil
}

// runeOffsets maps each rune index (plus one past the end) to its byte offset.
func runeOffsets(s string) []int {
	offs := make([]int, 0, len(s)+1)
	for i := range s {
		offs = append(offs, i)
	}
	offs = append(offs, len(s))
	return offs
}
