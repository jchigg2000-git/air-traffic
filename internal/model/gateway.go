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
	RequestID string `json:"request_id"`
	Route     string `json:"route"`
	Action    string `json:"action"` // pass | mask | block | detect
	Model     string `json:"model,omitempty"`
	// Principal attribution, from the keystore key that authenticated the
	// request. Legacy GATEWAY_CLIENT_KEYS callers report app_id "env".
	//
	// Subject is the one field here that can hold a human identifier, and it
	// is the first such field in this type — everything else is types,
	// offsets and counts. It is safe in a different way than the rest: the
	// owner authored it at issuance, so it is a deliberate label rather than
	// content extracted from traffic. Treat it as such when choosing what to
	// put in it.
	AppID   string `json:"app_id,omitempty"`
	KeyID   string `json:"key_id,omitempty"`
	Subject string `json:"subject,omitempty"`
	// Baseline names which baseline decided Action, so a per-app posture is
	// visible in the feed rather than having to be inferred from app_id.
	Baseline        string             `json:"baseline,omitempty"`
	Redactions      []GatewayRedaction `json:"redactions,omitempty"`
	DetectorErrors  []string           `json:"detector_errors,omitempty"`
	FailModeTripped bool               `json:"fail_mode_tripped,omitempty"`
	Stream          bool               `json:"stream,omitempty"`
	UpstreamStatus  int                `json:"upstream_status,omitempty"`
	// GatewayStatus is the HTTP status the gateway wrote itself, set on every
	// exit that answered the caller without relaying an upstream response
	// (policy block, fail-closed trip, and every Error below). Exactly one of
	// UpstreamStatus and GatewayStatus is set on a report.
	GatewayStatus int `json:"gateway_status,omitempty"`
	// Error names why the gateway could not complete the request — one of the
	// GatewayErr* codes, never free text. Empty on a relayed response and on a
	// deliberate refusal (block, fail-closed), which Action already names.
	// Without it a route failing every request looks idle: the failure leaves
	// no report and no metric.
	Error string `json:"error,omitempty"`
	// TokensIn/TokensOut are what the vendor reported for THIS request, not a
	// gateway estimate: absent usage stays zero rather than being guessed at.
	// There is deliberately no cost field — see docs/plans/TODO-gateway-deferred.md;
	// pricing belongs to whoever owns the contract, not to the proxy.
	TokensIn       int64     `json:"tokens_in,omitempty"`
	TokensOut      int64     `json:"tokens_out,omitempty"`
	LatencyMS      int64     `json:"latency_ms"`
	AddedLatencyMS int64     `json:"added_latency_ms"`
	At             time.Time `json:"at"`
}

// Error codes a GatewayRequestReport can carry: the ways a proxied request
// fails inside the gateway after the caller authenticated. Authentication
// failures are counted, not reported — see the gateway's metrics.
const (
	GatewayErrNoUpstream          = "no_upstream"
	GatewayErrBodyUnreadable      = "body_unreadable"
	GatewayErrInvalidJSON         = "invalid_json"
	GatewayErrRewriteFailed       = "rewrite_failed"
	GatewayErrCredential          = "credential_unavailable"
	GatewayErrUpstreamURL         = "upstream_url_unusable"
	GatewayErrRequestBuild        = "request_build_failed"
	GatewayErrUpstreamUnreachable = "upstream_unreachable"
)

// Bounds on a report's variable-size fields. Applied on both sides of the
// spine: the gateway clamps before a report enters its ring, so one oversized
// proxied request can never produce a push the control plane's 2 MB decoder
// rejects; the control plane clamps again on ingest, so a hostile or buggy
// gateway cannot balloon the ring. Truncation only — Clamp never adds, so the
// metadata-only guarantee is unaffected.
const (
	maxReportIDLen        = 128
	maxReportModelLen     = 256
	maxReportLabelLen     = 64  // route, app_id, key_id, baseline, error
	maxReportSubjectLen   = 200 // matches the keystore's issuance bound
	maxReportDetectorErrs = 5
	maxReportDetectorErr  = 300
	maxReportRedactions   = 500
	maxReportPathLen      = 200
)

// Clamp bounds string and slice growth in place.
func (r *GatewayRequestReport) Clamp() {
	r.RequestID = truncate(r.RequestID, maxReportIDLen)
	r.Model = truncate(r.Model, maxReportModelLen)
	r.Route = truncate(r.Route, maxReportLabelLen)
	r.AppID = truncate(r.AppID, maxReportLabelLen)
	r.KeyID = truncate(r.KeyID, maxReportLabelLen)
	r.Baseline = truncate(r.Baseline, maxReportLabelLen)
	r.Subject = truncate(r.Subject, maxReportSubjectLen)
	r.Error = truncate(r.Error, maxReportLabelLen)
	if len(r.DetectorErrors) > maxReportDetectorErrs {
		r.DetectorErrors = r.DetectorErrors[:maxReportDetectorErrs]
	}
	for i, e := range r.DetectorErrors {
		r.DetectorErrors[i] = truncate(e, maxReportDetectorErr)
	}
	if len(r.Redactions) > maxReportRedactions {
		r.Redactions = r.Redactions[:maxReportRedactions]
	}
	for i := range r.Redactions {
		r.Redactions[i].Path = truncate(r.Redactions[i].Path, maxReportPathLen)
	}
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// GatewayHeartbeatInterval is how often a gateway announces itself, and
// GatewayStaleAfter is how old a heartbeat may be before the control plane
// stops believing it: one missed beat is jitter, three is an outage. Both
// halves of the spine and the harness read these, so the window the gateway
// promises and the window the control plane enforces cannot drift apart.
const (
	GatewayHeartbeatInterval = 15 * time.Second
	GatewayStaleAfter        = 3 * GatewayHeartbeatInterval
)

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
	// KindAllowList suppresses a span whose exact text is a known false
	// positive for that type. It is the only kind that REMOVES a detection —
	// every other kind adds one — and it exists because score thresholds
	// cannot separate a real hit from a false one when the NER model returns
	// the same confidence for both (spaCy returns 0.85 for every PERSON and
	// LOCATION it finds, true or not).
	KindAllowList = "allow_list"
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
	AllowList  []string  `json:"allow_list,omitempty"`
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
// upstream (/synthetic/{vendor}/v1/messages). Bodies are stored as received
// (bounded to the upstream's capture byte cap) because harness traffic is
// synthetic by construction — NEVER point real traffic at the synthetic
// upstream. The credential itself is never stored, only a SHA-256 fingerprint
// prefix for swap verification.
type InferenceCapture struct {
	ID               int64     `json:"id"`
	AdapterID        string    `json:"adapter_id"`
	GatewayRequestID string    `json:"gateway_request_id"`
	Path             string    `json:"path"`
	Body             string    `json:"body"`
	Truncated        bool      `json:"truncated,omitempty"` // Body was cut at the synthetic upstream's capture byte cap
	AuthFingerprint  string    `json:"auth_fingerprint"`
	Stream           bool      `json:"stream"`
	ReceivedAt       time.Time `json:"received_at"`
}
