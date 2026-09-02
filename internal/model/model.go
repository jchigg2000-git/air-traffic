// Package model holds the domain types for the Air-Traffic control plane.
// Mirrors the model package of it-scorecard (the author's earlier, unpublished predecessor project; no third-party code), extended for AI-vendor control surfaces.
package model

import "time"

// ObservationContract is the emission contract carried over from the predecessor project.
const ObservationContract = "ops-observation-batch/v1"

// Mode selects how an adapter behaves.
type Mode string

const (
	ModeDisabled  Mode = "disabled"
	ModeSynthetic Mode = "synthetic"
	ModeProxy     Mode = "proxy"
)

// ProxyNotNormalizedCode / ProxyNotNormalizedMessage are the 501 body the
// synthetic vendor replica serves for ModeProxy and the error entry the
// emitter records for it. The replica never normalizes proxy traffic; inline
// enforcement is the inference gateway (cmd/air-traffic-gateway).
const (
	ProxyNotNormalizedCode    = "proxy_not_normalized"
	ProxyNotNormalizedMessage = "proxy mode is not served by the synthetic vendor replica; inline enforcement is the inference gateway (cmd/air-traffic-gateway)"
)

// Disposition is the truthfulness layer: how a control is actually enforced.
type Disposition string

const (
	DispVendorNative  Disposition = "vendor_native"
	DispEnvManaged    Disposition = "env_managed"
	DispProxyEnforced Disposition = "proxy_enforced"
	DispMonitorOnly   Disposition = "monitor_only"
	DispUnverified    Disposition = "unverified"
	DispUnsupported   Disposition = "unsupported"
)

// Plane is one of the three control planes plus observability.
type Plane string

const (
	PlaneDeveloperWorkflow Plane = "developer_workflow"
	PlaneDataPolicy        Plane = "data_policy"
	PlaneBudget            Plane = "budget"
	PlaneObservability     Plane = "observability"
)

// Enforcement is the env_managed confidence tier.
type Enforcement string

const (
	EnforcementServerSide Enforcement = "server_side" // Tier A — non-bypassable
	EnforcementMDMLocked  Enforcement = "mdm_locked"  // Tier B — enforced only on MDM devices
	EnforcementSeedOnly   Enforcement = "seed_only"   // Tier C — drift-detect only, user can override
)

// Capability is one control a vendor surface exposes, with its disposition.
type Capability struct {
	Key              string      `json:"key"`
	Name             string      `json:"name"`
	Plane            Plane       `json:"plane"`
	Disposition      Disposition `json:"disposition"`
	Enforcement      Enforcement `json:"enforcement,omitempty"`
	Mechanism        string      `json:"mechanism"`
	Endpoint         string      `json:"endpoint,omitempty"`
	DocumentationURL string      `json:"documentation_url,omitempty"`
	Retryable        bool        `json:"retryable"`
	Note             string      `json:"note,omitempty"`
}

// Status is an adapter's last-checked health.
type Status struct {
	State     string    `json:"state"` // unknown | healthy | degraded | error
	Message   string    `json:"message"`
	CheckedAt time.Time `json:"checked_at"`
}

// Adapter is one vendor control surface (data-driven, mirrors the predecessor project's Connector).
type Adapter struct {
	ID          string `json:"id"`
	Vendor      string `json:"vendor"`
	DisplayName string `json:"display_name"`
	Family      string `json:"family"` // api-platform | hyperscaler | coding-assistant | productivity
	APIVersion  string `json:"api_version"`
	Tier        int    `json:"tier"` // 1 deep | 2 core | 3 manifest-only
	Mode        Mode   `json:"mode"`
	Enabled     bool   `json:"enabled"`
	Emit        bool   `json:"emit"`
	BasePath    string `json:"base_path"`
	UpstreamURL string `json:"upstream_url"`
	// EndpointConfig holds per-vendor proxy-mode config (non-secret params + secret
	// *references*, never raw secrets): region, project, api_version, org_id, and
	// *_ref keys pointing at a secret store. Captured for the (future) proxy impl.
	EndpointConfig map[string]string `json:"endpoint_config,omitempty"`
	Scenario       string            `json:"scenario"`
	BAASigned      bool              `json:"baa_signed"`
	Capabilities   []Capability      `json:"capabilities"`
	Status         Status            `json:"status"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

// AdapterPatch is the mutable subset for PATCH /api/adapters/{id}.
type AdapterPatch struct {
	Enabled        *bool              `json:"enabled"`
	Emit           *bool              `json:"emit"`
	Mode           *Mode              `json:"mode"`
	Scenario       *string            `json:"scenario"`
	UpstreamURL    *string            `json:"upstream_url"`
	EndpointConfig *map[string]string `json:"endpoint_config"`
}

// Signal accompanies every control-application result.
type Signal struct {
	Disposition Disposition `json:"disposition"`
	Code        string      `json:"code"` // success | not_supported | credential_missing | rate_limit | drift | proxy_required
	Message     string      `json:"message"`
	Retryable   bool        `json:"retryable"`
}

// ManagedArtifact is a rendered env_managed config payload.
type ManagedArtifact struct {
	Platform        string         `json:"platform"`
	Identifier      string         `json:"identifier"`
	Artifact        map[string]any `json:"artifact"`
	Enforcement     Enforcement    `json:"enforcement"`
	DistributionURL string         `json:"distribution_url"`
	RenderedAt      time.Time      `json:"rendered_at"`
}

// EnvState is the read-back effective state of an env_managed surface.
type EnvState struct {
	Platform       string         `json:"platform"`
	Identifier     string         `json:"identifier"`
	ActualSettings map[string]any `json:"actual_settings"`
	Source         string         `json:"source"` // locked | user_override | default
	DriftDetected  bool           `json:"drift_detected"`
	DriftMessage   string         `json:"drift_message,omitempty"`
	ReadAt         time.Time      `json:"read_at"`
}

// ObservationRecord wraps one emitted ops-observation-batch/v1 batch (wire-compatible with the predecessor project).
type ObservationRecord struct {
	ID                int64          `json:"id"`
	ReceivedAt        time.Time      `json:"received_at"`
	Contract          string         `json:"contract"`
	ConnectorType     string         `json:"connector_type"`
	ConnectorInstance string         `json:"connector_instance"`
	Complete          bool           `json:"complete"`
	ObservationCount  int            `json:"observation_count"`
	ErrorCount        int            `json:"error_count"`
	Body              map[string]any `json:"body"`
}

// CallRecord is one recorded synthetic-surface request (redacted).
type CallRecord struct {
	ID         int64               `json:"id"`
	AdapterID  string              `json:"adapter_id"`
	Method     string              `json:"method"`
	Path       string              `json:"path"`
	Query      map[string][]string `json:"query"`
	Headers    map[string][]string `json:"headers"`
	Scenario   string              `json:"scenario"`
	StatusCode int                 `json:"status_code"`
	DurationMS int64               `json:"duration_ms"`
	RecordedAt time.Time           `json:"recorded_at"`
}

// AuditEvent is one normalized cross-vendor audit entry (OTel GenAI semconv shaped).
type AuditEvent struct {
	ID             string         `json:"id"`
	Timestamp      time.Time      `json:"timestamp"`
	Actor          string         `json:"actor"`
	Action         string         `json:"action"`
	Resource       string         `json:"resource"`
	Plane          Plane          `json:"plane"`
	Vendor         string         `json:"vendor"`
	ControlSurface Disposition    `json:"control_surface"`
	Before         map[string]any `json:"before,omitempty"`
	After          map[string]any `json:"after,omitempty"`
	RequestID      string         `json:"request_id"`
}

// ActivityEvent is a coarse system activity log entry.
type ActivityEvent struct {
	ID        int64     `json:"id"`
	Source    string    `json:"source"`
	Message   string    `json:"message"`
	Detail    string    `json:"detail,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Credential is stored by reference only — never plaintext.
type Credential struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Provider  string            `json:"provider"`
	Type      string            `json:"type"`
	SecretRef string            `json:"secret_ref"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Status    Status            `json:"status"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// DriftRecord is a single intent-vs-actual divergence surfaced at /api/drift.
type DriftRecord struct {
	ID         int64       `json:"id"`
	Vendor     string      `json:"vendor"`
	Capability string      `json:"capability"`
	Plane      Plane       `json:"plane"`
	Surface    Disposition `json:"control_surface"`
	Declared   string      `json:"declared"`
	Actual     string      `json:"actual"`
	Severity   string      `json:"severity"` // info | warning | error
	Message    string      `json:"message"`
	DetectedAt time.Time   `json:"detected_at"`
}
