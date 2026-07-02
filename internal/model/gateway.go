package model

import "time"

// GatewayRedaction locates one gateway detection: offsets into the named
// request field. No value, ever.
type GatewayRedaction struct {
	Path       string  `json:"path"`
	Type       string  `json:"type"`
	Start      int     `json:"start"`
	End        int     `json:"end"`
	Detector   string  `json:"detector"`
	Confidence float64 `json:"confidence"`
}

// GatewayRequestReport is the gateway's per-request redaction audit record —
// types, offsets, counts, never values. Shared contract: the gateway emits
// it, the control plane ingests it at POST /api/gateway/leaks, the harness
// joins it against ground truth by RequestID.
type GatewayRequestReport struct {
	RequestID       string             `json:"request_id"`
	Route           string             `json:"route"`
	Action          string             `json:"action"` // pass | mask | block | detect
	Redactions      []GatewayRedaction `json:"redactions,omitempty"`
	DetectorErrors  []string           `json:"detector_errors,omitempty"`
	FailModeTripped bool               `json:"fail_mode_tripped,omitempty"`
	Stream          bool               `json:"stream,omitempty"`
	UpstreamStatus  int                `json:"upstream_status,omitempty"`
	LatencyMS       int64              `json:"latency_ms"`
	AddedLatencyMS  int64              `json:"added_latency_ms"`
	At              time.Time          `json:"at"`
}

// EnforcementReport is the gateway's periodic heartbeat: which vendor
// capabilities it is actively enforcing right now. Freshness is what flips
// proxy_enforced from label to truth; staleness raises drift.
type EnforcementReport struct {
	GatewayID string              `json:"gateway_id"`
	BaseURL   string              `json:"base_url"`
	Action    string              `json:"action"`
	Detectors []string            `json:"detectors,omitempty"`
	Vendors   map[string][]string `json:"vendors"`
	At        time.Time           `json:"at,omitempty"`
}

// Pattern kinds: what shape of detector artifact a rule or proposal carries.
// An empty kind means regex (rules persisted before kinds existed).
const (
	KindRegex     = "regex"
	KindDenyList  = "deny_list"
	KindThreshold = "threshold"
)

// PatternRule is one flywheel-approved detector addition, distributed to
// gateways via GET /api/gateway/patterns. Exactly one artifact is set,
// selected by Kind: Regex (with optional Context words for Presidio's
// context enhancer), DenyList terms, or a per-type score Threshold.
type PatternRule struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	Kind       string    `json:"kind,omitempty"` // "" = regex
	Regex      string    `json:"regex,omitempty"`
	DenyList   []string  `json:"deny_list,omitempty"`
	Threshold  float64   `json:"threshold,omitempty"`
	Context    []string  `json:"context,omitempty"`
	Confidence float64   `json:"confidence,omitempty"`
	Rationale  string    `json:"rationale,omitempty"`
	AddedAt    time.Time `json:"added_at,omitempty"`
}

// PatternPack is the versioned active rule set; gateways hot-reload on a
// version bump, no restart.
type PatternPack struct {
	Version   int           `json:"version"`
	UpdatedAt time.Time     `json:"updated_at,omitempty"`
	Rules     []PatternRule `json:"rules"`
}

// InferenceCapture records one request that reached the synthetic inference
// upstream (/synthetic/{vendor}/v1/messages). Bodies are stored raw because
// harness traffic is synthetic by construction — NEVER point real traffic at
// the synthetic upstream. The credential itself is never stored, only a
// SHA-256 fingerprint prefix for swap verification.
type InferenceCapture struct {
	ID               int64     `json:"id"`
	AdapterID        string    `json:"adapter_id"`
	GatewayRequestID string    `json:"gateway_request_id"`
	Path             string    `json:"path"`
	Body             string    `json:"body"`
	AuthFingerprint  string    `json:"auth_fingerprint"`
	Stream           bool      `json:"stream"`
	ReceivedAt       time.Time `json:"received_at"`
}
