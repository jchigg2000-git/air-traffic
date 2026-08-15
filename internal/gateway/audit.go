package gateway

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"air-traffic/internal/model"
)

// RequestAudit is the per-request redaction record: types, offsets, counts —
// never values ("don't log what you redact", design §10). Offsets are into
// the named field's text, which lets the harness join detections against its
// ground-truth spans without either side exchanging raw values. The type is
// the shared model contract so the spine pusher ships it verbatim.
type RequestAudit = model.GatewayRequestReport

// Redaction locates one detection. No value, ever.
type Redaction = model.GatewayRedaction

const auditRingMax = 5000

// auditRing buffers per-request audits until the spine pusher drains them
// upward. In-memory only — the durable home is the control plane's audit
// stream.
type auditRing struct {
	mu  sync.Mutex
	buf []RequestAudit
}

func (a *auditRing) add(r RequestAudit) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.buf = append(a.buf, r)
	if len(a.buf) > auditRingMax {
		a.buf = a.buf[len(a.buf)-auditRingMax:]
	}
}

func (a *auditRing) drain() []RequestAudit {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := a.buf
	a.buf = nil
	return out
}

// requeue puts reports back at the front after a failed push so the harness
// join never loses them to a transient control-plane outage.
func (a *auditRing) requeue(reports []RequestAudit) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.buf = append(reports, a.buf...)
	if len(a.buf) > auditRingMax {
		a.buf = a.buf[:auditRingMax]
	}
}

// record stores the audit, feeds the metrics window, and emits the
// redaction-safe structured log line.
func (s *Server) record(a RequestAudit, tokensIn, tokensOut int64) {
	// Attach the counts here rather than at each call site, so the ring the
	// spine drains and the metrics window can never disagree about a request.
	a.TokensIn, a.TokensOut = tokensIn, tokensOut
	s.audits.add(a)
	s.metrics.observe(a, tokensIn, tokensOut)
	s.log.Info("gateway.request",
		"request_id", a.RequestID,
		"route", a.Route,
		"action", a.Action,
		"model", a.Model,
		"tokens_in", tokensIn,
		"tokens_out", tokensOut,
		"redactions", len(a.Redactions),
		"types", typeSummary(a.Redactions),
		"detector_errors", len(a.DetectorErrors),
		"fail_mode_tripped", a.FailModeTripped,
		"upstream_status", a.UpstreamStatus,
		"latency_ms", a.LatencyMS,
		"added_latency_ms", a.AddedLatencyMS,
	)
}

// typeSummary renders "SSN×2, EMAIL×1" — counts only, safe for logs and
// block messages.
func typeSummary(reds []Redaction) string {
	if len(reds) == 0 {
		return ""
	}
	counts := map[string]int{}
	for _, r := range reds {
		counts[r.Type]++
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s×%d", k, counts[k]))
	}
	return strings.Join(parts, ", ")
}
