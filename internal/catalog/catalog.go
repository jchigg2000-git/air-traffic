// Package catalog is the deep, static collection of every vendor control surface
// Air-Traffic governs: per-vendor capabilities (with dispositions + enforcement
// tiers) and the emitter metric definitions. This is the "surface collection"
// deliverable of Phase 1, grounded in docs/air-traffic-analysis.md §3.
package catalog

import "air-traffic/internal/model"

// MetricDef drives the synthetic emitter's mean-reverting random walk for one metric.
type MetricDef struct {
	Key       string
	Name      string
	Unit      string
	Plane     model.Plane
	Surface   model.Disposition
	Kind      string // metric | state
	Baseline  float64
	Min       float64
	Max       float64
	Step      float64
	Green     float64
	Amber     float64
	Polarity  string // higher | lower
	Decimals  int
	Model     string // optional model dimension
	SourceURL string
}

// EnvPlatform is an env_managed distribution target a vendor adapter renders config for.
type EnvPlatform struct {
	Platform    string // claude_code | github | vs_code | cursor
	Enforcement model.Enforcement
}

// CostFacet declares ONE cost/usage drill-down dimension assessed against a vendor's
// REAL admin/billing API. Supported facets (Supported && len(Members)>0) drive both the
// byte-identical synthetic grouped responses AND the emitter's per-member cost breakdown,
// from this single source of truth. Unsupported facets carry an honest Reason and are
// surfaced (greyed) in the UI — Air-Traffic never fabricates a breakdown a vendor can't do.
type CostFacet struct {
	Dimension     string        `json:"dimension"`      // user|model|repo|project|workspace|api_key|team|region|service_tier|sku|language|deployment
	Label         string        `json:"label"`          // "By User"
	Supported     bool          `json:"supported"`      // does the REAL vendor API group cost/usage by this?
	RealParam     string        `json:"real_param"`     // the real group-by mechanism (e.g. group_by=user_id)
	Endpoint      string        `json:"endpoint"`       // the real endpoint/method that serves it
	ResponseField string        `json:"response_field"` // the JSON field the grouped key appears under (byte-identical)
	Reason        string        `json:"reason"`         // caveat (if supported) or why-not (if unsupported)
	Members       []FacetMember `json:"members"`        // canonical members; empty if unsupported
}

// FacetMember is one breakdown row (e.g. user-42 / gpt-4o / acme-payments). Weight is the
// member's relative share of the vendor's spend, normalized at read time so the breakdown
// always sums to the vendor's aggregate cost.
type FacetMember struct {
	Key    string  `json:"key"`    // value as the vendor returns it (user-42, gpt-4o, acme/payments)
	Label  string  `json:"label"`  // display label
	Weight float64 `json:"weight"` // relative spend share
}

// Definition is the full static description of one vendor adapter.
type Definition struct {
	ID           string
	Vendor       string
	DisplayName  string
	Family       string
	APIVersion   string
	Tier         int
	BAASigned    bool
	Capabilities []model.Capability
	Metrics      []MetricDef
	EnvPlatforms []EnvPlatform
	CostFacets   []CostFacet
}

// compact capability constructor
func cap(key, name string, plane model.Plane, disp model.Disposition, mech, endpoint string) model.Capability {
	return model.Capability{Key: key, Name: name, Plane: plane, Disposition: disp, Mechanism: mech, Endpoint: endpoint}
}

func capEnv(key, name string, plane model.Plane, enf model.Enforcement, mech, endpoint string) model.Capability {
	return model.Capability{Key: key, Name: name, Plane: plane, Disposition: model.DispEnvManaged, Enforcement: enf, Mechanism: mech, Endpoint: endpoint}
}

func note(c model.Capability, n string) model.Capability { c.Note = n; return c }

// standard env_managed agentic capabilities shared by API-platform vendors
func agenticEnvCaps() []model.Capability {
	return []model.Capability{
		capEnv("mcp_allow_deny", "MCP Server Allow/Deny", model.PlaneDeveloperWorkflow, model.EnforcementMDMLocked, "Claude Code managed-settings.json", ""),
		capEnv("shared_hooks", "Shared Hooks (pre-commit/test/scan)", model.PlaneDeveloperWorkflow, model.EnforcementMDMLocked, "managed-settings.json hooks", ""),
		capEnv("code_review_gate", "Agentic Code-Review Gate", model.PlaneDeveloperWorkflow, model.EnforcementServerSide, "GitHub branch protection + required status checks", ""),
		capEnv("cron_controls", "Scheduled-Job Controls", model.PlaneDeveloperWorkflow, model.EnforcementSeedOnly, "environment managed config", ""),
	}
}

// shared budget/usage metric defs
func budgetMetrics(vendor string, src string) []MetricDef {
	return []MetricDef{
		{Key: "tokens_in", Name: "Input Tokens", Unit: "tokens", Plane: model.PlaneBudget, Surface: model.DispVendorNative, Kind: "metric", Baseline: 142000, Min: 60000, Max: 240000, Step: 9000, Green: 200000, Amber: 230000, Polarity: "lower", Decimals: 0, SourceURL: src},
		{Key: "tokens_out", Name: "Output Tokens", Unit: "tokens", Plane: model.PlaneBudget, Surface: model.DispVendorNative, Kind: "metric", Baseline: 58000, Min: 20000, Max: 120000, Step: 5000, Green: 95000, Amber: 110000, Polarity: "lower", Decimals: 0, SourceURL: src},
		{Key: "cost_usd", Name: "Spend (rolling)", Unit: "usd", Plane: model.PlaneBudget, Surface: model.DispVendorNative, Kind: "metric", Baseline: 1180, Min: 400, Max: 2600, Step: 90, Green: 2000, Amber: 2400, Polarity: "lower", Decimals: 2, SourceURL: src},
		{Key: "cap_utilization", Name: "Org Cap Utilization", Unit: "%", Plane: model.PlaneBudget, Surface: model.DispMonitorOnly, Kind: "metric", Baseline: 47, Min: 20, Max: 99, Step: 3, Green: 80, Amber: 92, Polarity: "lower", Decimals: 1, SourceURL: src},
		{Key: "p95_latency", Name: "Admin API P95", Unit: "ms", Plane: model.PlaneObservability, Surface: model.DispVendorNative, Kind: "metric", Baseline: 220, Min: 110, Max: 900, Step: 40, Green: 400, Amber: 700, Polarity: "lower", Decimals: 0, SourceURL: src},
	}
}

func stateMetric(key, name string, plane model.Plane, surface model.Disposition) MetricDef {
	return MetricDef{Key: key, Name: name, Unit: "bool", Plane: plane, Surface: surface, Kind: "state", Baseline: 1, Min: 1, Max: 1, Step: 0, Green: 1, Amber: 0, Polarity: "higher", Decimals: 0}
}

// All returns every vendor definition (the surface collection), with each vendor's
// cost drill-down facets attached from the single registry in cost_facets.go.
func All() []Definition {
	defs := []Definition{
		openAI(), anthropic(), bedrock(), azureOpenAI(), vertex(), githubCopilot(),
		m365Copilot(), mistral(), databricks(), perplexity(), cohere(), together(),
		groq(), xai(), amazonQ(), watsonx(),
	}
	for i := range defs {
		defs[i].CostFacets = costFacetsByID[defs[i].ID]
	}
	return defs
}

// ByID returns the definition with the given id and whether it exists.
func ByID(id string) (Definition, bool) {
	for _, d := range All() {
		if d.ID == id {
			return d, true
		}
	}
	return Definition{}, false
}
