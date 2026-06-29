package model

import "time"

// Baseline is a named industry-rigor profile (General SaaS / Fintech / Healthcare / Gov).
type Baseline struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Rigor          string `json:"rigor"` // 🔒 | 🔒🔒 | 🔒🔒🔒
	Description    string `json:"description"`
	ModelAccess    string `json:"model_access"`
	TrainingOptOut bool   `json:"training_opt_out"`
	ZDR            string `json:"zdr"` // off | where_native | enforced
	PIIRedaction   string `json:"pii_redaction"`
	ContentSafety  string `json:"content_safety"`
	RetentionDays  int    `json:"retention_days"` // 0 = zero retention
	OrgCapUSD      int    `json:"org_cap_usd"`
	UserCapUSD     int    `json:"user_cap_usd"`
	CodeReview     string `json:"code_review"`
	BAAOnly        bool   `json:"baa_only"`
}

// Policy is the resolved policy-as-code intent (baseline ⊕ overrides).
type Policy struct {
	Baseline       string                    `json:"baseline"`
	VendorDefaults map[string]any            `json:"vendor_defaults"`
	Vendors        map[string]map[string]any `json:"vendors"`
	Agentic        map[string]map[string]any `json:"agentic"`
	Budget         map[string]any            `json:"budget"`
	AppliedAt      time.Time                 `json:"applied_at,omitempty"`
}

// CoverageRow classifies one (vendor, capability) after reconcile.
type CoverageRow struct {
	Vendor      string      `json:"vendor"`
	Capability  string      `json:"capability"`
	Plane       Plane       `json:"plane"`
	Disposition Disposition `json:"disposition"`
	Enforcement Enforcement `json:"enforcement,omitempty"`
	Outcome     string      `json:"outcome"` // applied_native | applied_env | proxy_needed | monitored | unverified | unsupported
	Signal      Signal      `json:"signal"`
}

// CoverageReport is returned by PUT /api/policies.
type CoverageReport struct {
	Baseline  string         `json:"baseline"`
	AppliedAt time.Time      `json:"applied_at"`
	Rows      []CoverageRow  `json:"rows"`
	Summary   map[string]int `json:"summary"` // outcome → count
}
