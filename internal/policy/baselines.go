// Package policy holds the industry-rigor baselines, the policy-as-code resolver,
// the reconcile/coverage engine, and drift detection.
package policy

import "github.com/jchigg2000-git/air-traffic/internal/model"

// Baselines returns the four named industry-rigor profiles (analysis §6).
func Baselines() []model.Baseline {
	return []model.Baseline{
		{
			ID: "general_saas", Name: "General SaaS", Rigor: "🔒",
			Description: "Balanced defaults for typical SaaS engineering orgs.",
			ModelAccess: "open", TrainingOptOut: true, ZDR: "off", PIIRedaction: "off",
			ContentSafety: "standard", RetentionDays: 30, OrgCapUSD: 10000, UserCapUSD: 200,
			CodeReview: "agentic-reviewer (customer-facing)", BAAOnly: false,
		},
		{
			ID: "fintech", Name: "Fintech", Rigor: "🔒🔒",
			Description: "Elevated controls for regulated financial workloads.",
			ModelAccess: "restricted (prod-tier only)", TrainingOptOut: true, ZDR: "where_native", PIIRedaction: "on",
			ContentSafety: "elevated", RetentionDays: 7, OrgCapUSD: 50000, UserCapUSD: 500,
			CodeReview: "human-required (financial flows)", BAAOnly: false,
		},
		{
			ID: "healthcare", Name: "Healthcare", Rigor: "🔒🔒🔒",
			Description: "Maximum rigor; BAA-signed vendors only; zero retention.",
			ModelAccess: "BAA-only allowlist", TrainingOptOut: true, ZDR: "enforced", PIIRedaction: "on+phi",
			ContentSafety: "maximum", RetentionDays: 0, OrgCapUSD: 0, UserCapUSD: 100,
			CodeReview: "human-required (all)", BAAOnly: true,
		},
		{
			ID: "gov_infra", Name: "Gov / Infra", Rigor: "🔒🔒🔒",
			Description: "FedRAMP/on-prem only; audit-controlled retention.",
			ModelAccess: "FedRAMP/on-prem only", TrainingOptOut: true, ZDR: "enforced", PIIRedaction: "on",
			ContentSafety: "maximum", RetentionDays: 0, OrgCapUSD: 0, UserCapUSD: 0,
			CodeReview: "human-required (all)", BAAOnly: true,
		},
	}
}

// BaselineByID returns the baseline with the given id.
func BaselineByID(id string) (model.Baseline, bool) {
	for _, b := range Baselines() {
		if b.ID == id {
			return b, true
		}
	}
	return model.Baseline{}, false
}
