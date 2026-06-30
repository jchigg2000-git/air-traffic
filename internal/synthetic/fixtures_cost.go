package synthetic

// fixtures_cost.go holds the byte-identical GROUPED cost/usage responses. When a request
// carries a vendor's real group-by param, the vendor fixture fans its single result row
// out into one row per catalog facet member — same envelope, same field names, just N
// rows with the grouped field populated. Reads the same catalog.CostFacets the emitter
// attributes spend against, so the synthetic surface and the drill-down never disagree.

import (
	"fmt"
	"time"

	"air-traffic/internal/catalog"
)

func round2(v float64) float64 { return float64(int64(v*100+0.5)) / 100 }

// facetFor resolves a requested group-by field to a vendor facet, but only if the field is
// valid for the calling endpoint (allowed) — so e.g. group_by=model on the Costs API (which
// the real API rejects) falls through to the ungrouped response instead of fabricating one.
func facetFor(vendorID, field string, allowed ...string) (catalog.CostFacet, bool) {
	for _, a := range allowed {
		if a == field {
			return facetByField(vendorID, field)
		}
	}
	return catalog.CostFacet{}, false
}

// ---- OpenAI: Usage API (/usage/completions) and Costs API (/costs) ----

// openaiUsageBucket builds a byte-identical "bucket" whose results[] carries one
// organization.usage.completions.result per facet member, the grouped field populated and
// the other group keys null (exactly as the real grouped Usage API returns).
func openaiUsageBucket(f catalog.CostFacet) map[string]any {
	results := make([]any, 0, len(f.Members))
	for _, ms := range shares(f) {
		r := map[string]any{
			"object":              "organization.usage.completions.result",
			"input_tokens":        int64(142500*ms.Share + 0.5),
			"output_tokens":       int64(58210*ms.Share + 0.5),
			"input_cached_tokens": int64(38110*ms.Share + 0.5),
			"num_model_requests":  int64(9123*ms.Share + 0.5),
			"project_id":          nil,
			"user_id":             nil,
			"api_key_id":          nil,
			"model":               nil,
			"service_tier":        nil,
			"batch":               nil,
		}
		r[f.ResponseField] = ms.Member.Key
		results = append(results, r)
	}
	return map[string]any{"object": "bucket", "start_time": nowUnix() - 86400, "end_time": nowUnix(), "results": results}
}

// openaiCostsBucket builds a byte-identical costs bucket: one organization.costs.result per
// member with amount{value,currency} scaled by share and the grouped field populated.
func openaiCostsBucket(f catalog.CostFacet) map[string]any {
	results := make([]any, 0, len(f.Members))
	for _, ms := range shares(f) {
		r := map[string]any{
			"object":     "organization.costs.result",
			"amount":     map[string]any{"value": round2(1182.44 * ms.Share), "currency": "usd"},
			"line_item":  nil,
			"project_id": nil,
			"api_key_id": nil,
		}
		r[f.ResponseField] = ms.Member.Key
		results = append(results, r)
	}
	return map[string]any{"object": "bucket", "start_time": nowUnix() - 86400, "end_time": nowUnix(), "results": results}
}

// ---- Anthropic: usage_report/messages and cost_report ----

func anthropicUsageGrouped(f catalog.CostFacet) map[string]any {
	results := make([]any, 0, len(f.Members))
	for _, ms := range shares(f) {
		r := map[string]any{
			"uncached_input_tokens":   int64(142500*ms.Share + 0.5),
			"cache_read_input_tokens": int64(38110*ms.Share + 0.5),
			"output_tokens":           int64(58210*ms.Share + 0.5),
			"api_key_id":              nil,
			"workspace_id":            nil,
			"model":                   nil,
			"service_tier":            nil,
		}
		r[f.ResponseField] = ms.Member.Key
		results = append(results, r)
	}
	return map[string]any{"data": []any{map[string]any{
		"starting_at": agoRFC(24 * time.Hour), "ending_at": nowRFC(), "results": results,
	}}, "has_more": false, "next_page": nil}
}

func anthropicCostGrouped(f catalog.CostFacet) map[string]any {
	results := make([]any, 0, len(f.Members))
	for _, ms := range shares(f) {
		r := map[string]any{
			"amount":       fmt.Sprintf("%.2f", round2(284.10*ms.Share)),
			"currency":     "USD",
			"workspace_id": nil,
			"description":  nil,
		}
		r[f.ResponseField] = ms.Member.Key
		results = append(results, r)
	}
	return map[string]any{"data": []any{map[string]any{
		"starting_at": agoRFC(24 * time.Hour), "ending_at": nowRFC(), "results": results,
	}}, "has_more": false, "next_page": nil}
}

// ---- GitHub Copilot: inherent multi-row breakdowns (no group_by param) ----

// githubUsageItems returns one Enhanced-Billing usageItems[] row per repo facet member —
// the real /settings/billing/usage shape, where repo attribution is one line item each.
func githubUsageItems() []any {
	f, ok := facetByField("github_copilot", "repositoryName")
	if !ok {
		return []any{}
	}
	date := time.Now().UTC().Format("2006-01-02")
	items := make([]any, 0, len(f.Members))
	for _, ms := range shares(f) {
		items = append(items, map[string]any{
			"date": date, "product": "copilot", "sku": "copilot_premium_request",
			"quantity": int64(1830*ms.Share + 0.5), "unitType": "AI Credit",
			"netAmount": round2(18.30 * ms.Share), "organizationName": "acme",
			"repositoryName": ms.Member.Key,
		})
	}
	return items
}

// githubMetrics returns the Copilot metrics array with languages[] and editors[].models[]
// fanned out from the language and model facets (the real, pre-broken-down nesting).
func githubMetrics() []any {
	langs := []any{}
	if f, ok := facetByField("github_copilot", "languages[].name"); ok {
		for _, ms := range shares(f) {
			langs = append(langs, map[string]any{"name": ms.Member.Key, "total_engaged_users": int64(360*ms.Share + 0.5)})
		}
	}
	models := []any{}
	if f, ok := facetByField("github_copilot", "editors[].models[].name"); ok {
		for _, ms := range shares(f) {
			models = append(models, map[string]any{"name": ms.Member.Key, "total_engaged_users": int64(340*ms.Share + 0.5)})
		}
	}
	return []any{map[string]any{
		"date": time.Now().UTC().Format("2006-01-02"), "total_active_users": 412, "total_engaged_users": 388,
		"copilot_ide_code_completions": map[string]any{
			"total_engaged_users": 360,
			"languages":           langs,
			"editors":             []any{map[string]any{"name": "vscode", "total_engaged_users": 360, "models": models}},
		},
	}}
}

// githubSeats returns one seats[] row per user facet member (the real per-assignee shape).
func githubSeats() map[string]any {
	f, ok := facetByField("github_copilot", "seats[].assignee.login")
	seats := []any{}
	if ok {
		for _, ms := range shares(f) {
			seats = append(seats, map[string]any{
				"created_at": agoRFC(2400 * time.Hour), "updated_at": nowRFC(),
				"last_activity_at": nowRFC(), "last_activity_editor": "vscode/1.99.0",
				"assignee": map[string]any{"login": ms.Member.Key, "id": 90210, "type": "User"},
			})
		}
	}
	return map[string]any{"total_seats": 412, "seats": seats}
}
