// Package detect is the gateway's pluggable PII/PHI detection seam: one
// interface, engines behind it (in-process regex now, Presidio sidecar,
// managed DLP later), composed into an ordered chain with span merging.
package detect

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// Span is one detected sensitive value: offsets into the scanned text plus
// type and provenance. Never carries the value itself.
type Span struct {
	Start      int     `json:"start"`
	End        int     `json:"end"`
	Type       string  `json:"type"`
	Confidence float64 `json:"confidence"`
	Detector   string  `json:"detector"`
}

// Detector is implemented by every engine.
type Detector interface {
	Name() string
	Detect(ctx context.Context, text string) ([]Span, error)
}

// Chain runs engines in order (fast deterministic floor first), each under
// its own timeout, and merges overlapping spans. Engine errors are collected,
// not fatal — the caller decides what an error means via FAIL_MODE.
type Chain struct {
	Detectors []Detector
	Timeout   time.Duration
}

// typeGuards are engine-independent validators applied to every span by
// claimed type: an engine claiming SSN inside ORD-123-45-6789 is overruled
// the same way whichever engine claimed it (Presidio's context boosting has
// no hyphen-adjacency notion; ours does).
var typeGuards = map[string]func(text string, start, end int) bool{
	"SSN":         notHyphenAdjacent,
	"CREDIT_CARD": luhnValid,
	"IP":          validOctets,
	"IBAN":        ibanMod97,
}

// Run returns merged, guard-validated spans plus one error per failed engine.
func (c *Chain) Run(ctx context.Context, text string) ([]Span, []error) {
	var all []Span
	var errs []error
	for _, d := range c.Detectors {
		dctx, cancel := context.WithTimeout(ctx, c.Timeout)
		spans, err := d.Detect(dctx, text)
		cancel()
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", d.Name(), err))
			continue
		}
		for _, sp := range spans {
			if guard, ok := typeGuards[sp.Type]; ok && !guard(text, sp.Start, sp.End) {
				continue
			}
			all = append(all, sp)
		}
	}
	return Merge(all), errs
}

// Merge sorts spans and collapses overlaps: the union wins the extent, the
// higher confidence wins type and provenance.
func Merge(spans []Span) []Span {
	if len(spans) < 2 {
		return spans
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].Start != spans[j].Start {
			return spans[i].Start < spans[j].Start
		}
		return spans[i].End > spans[j].End
	})
	out := spans[:1]
	for _, sp := range spans[1:] {
		last := &out[len(out)-1]
		if sp.Start >= last.End {
			out = append(out, sp)
			continue
		}
		if sp.End > last.End {
			last.End = sp.End
		}
		if sp.Confidence > last.Confidence {
			last.Type, last.Detector, last.Confidence = sp.Type, sp.Detector, sp.Confidence
		}
	}
	return out
}
