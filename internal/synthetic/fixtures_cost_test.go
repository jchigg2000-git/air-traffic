package synthetic

import "testing"

// arr asserts body[key] is a JSON array and returns it.
func arr(t *testing.T, m map[string]any, key string) []any {
	t.Helper()
	a, ok := m[key].([]any)
	if !ok {
		t.Fatalf("expected %q to be an array, got %T", key, m[key])
	}
	return a
}

// TestOpenAIGroupedUsage: group_by=model keeps the page/bucket envelope and fans results
// out, one per facet member, with the grouped field populated and other group keys null.
func TestOpenAIGroupedUsage(t *testing.T) {
	h := newHandler()
	code, body := do(t, h, "GET", "/synthetic/openai/v1/organization/usage?group_by=model")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	m := obj(t, body)
	hasKeys(t, m, "object", "data", "has_more", "next_page")
	if m["object"] != "page" {
		t.Fatalf("envelope drifted: want page, got %v", m["object"])
	}
	bucket := obj(t, arr(t, m, "data")[0])
	results := arr(t, bucket, "results")
	if len(results) < 2 {
		t.Fatalf("expected multiple grouped rows, got %d", len(results))
	}
	for _, r := range results {
		rm := obj(t, r)
		if rm["object"] != "organization.usage.completions.result" {
			t.Errorf("result object drifted: %v", rm["object"])
		}
		if rm["model"] == nil {
			t.Error("group_by=model must populate the model field")
		}
		if rm["user_id"] != nil {
			t.Error("non-grouped key user_id must be null")
		}
		if _, ok := rm["input_tokens"]; !ok {
			t.Error("grouped usage row missing input_tokens")
		}
	}
}

// TestOpenAIGroupedCosts: group_by=project_id on the Costs API.
func TestOpenAIGroupedCosts(t *testing.T) {
	h := newHandler()
	_, body := do(t, h, "GET", "/synthetic/openai/v1/organization/costs?group_by=project_id")
	bucket := obj(t, arr(t, obj(t, body), "data")[0])
	results := arr(t, bucket, "results")
	if len(results) < 2 {
		t.Fatalf("expected multiple cost rows, got %d", len(results))
	}
	for _, r := range results {
		rm := obj(t, r)
		if rm["object"] != "organization.costs.result" {
			t.Errorf("cost result object drifted: %v", rm["object"])
		}
		if rm["project_id"] == nil {
			t.Error("group_by=project_id must populate project_id")
		}
		if _, ok := rm["amount"].(map[string]any)["value"]; !ok {
			t.Error("cost row missing amount.value")
		}
	}
}

// TestOpenAIUngroupedUnchanged: no group_by → the original single-result shape is intact.
func TestOpenAIUngroupedUnchanged(t *testing.T) {
	h := newHandler()
	_, body := do(t, h, "GET", "/synthetic/openai/v1/organization/usage")
	bucket := obj(t, arr(t, obj(t, body), "data")[0])
	if got := len(arr(t, bucket, "results")); got != 1 {
		t.Fatalf("ungrouped usage should have exactly 1 result, got %d", got)
	}
}

// TestAnthropicGroupedUsage: group_by=workspace_id; no OpenAI-style top-level object key.
func TestAnthropicGroupedUsage(t *testing.T) {
	h := newHandler()
	_, body := do(t, h, "GET", "/synthetic/anthropic/v1/organizations/usage_report/messages?group_by=workspace_id")
	m := obj(t, body)
	if _, ok := m["object"]; ok {
		t.Error("anthropic grouped response must NOT carry an OpenAI-style object key")
	}
	results := arr(t, obj(t, arr(t, m, "data")[0]), "results")
	if len(results) < 2 {
		t.Fatalf("expected multiple grouped rows, got %d", len(results))
	}
	for _, r := range results {
		rm := obj(t, r)
		if rm["workspace_id"] == nil {
			t.Error("group_by=workspace_id must populate workspace_id")
		}
		if rm["model"] != nil {
			t.Error("non-grouped key model must be null")
		}
	}
}

// TestAnthropicGroupedCost: cost_report group_by=description, amount stays a string.
func TestAnthropicGroupedCost(t *testing.T) {
	h := newHandler()
	_, body := do(t, h, "GET", "/synthetic/anthropic/v1/organizations/cost_report?group_by[]=description")
	results := arr(t, obj(t, arr(t, obj(t, body), "data")[0]), "results")
	if len(results) < 2 {
		t.Fatalf("expected multiple cost rows, got %d", len(results))
	}
	for _, r := range results {
		rm := obj(t, r)
		if _, ok := rm["amount"].(string); !ok {
			t.Errorf("anthropic cost amount must be a string, got %T", rm["amount"])
		}
		if rm["description"] == nil {
			t.Error("group_by=description must populate description")
		}
	}
}

// TestGitHubRepoBreakdown: billing/usage returns one usageItems[] row per repo.
func TestGitHubRepoBreakdown(t *testing.T) {
	h := newHandler()
	_, body := do(t, h, "GET", "/synthetic/github_copilot/enterprises/acme/settings/billing/usage")
	items := arr(t, obj(t, body), "usageItems")
	if len(items) < 2 {
		t.Fatalf("expected multiple usageItems (one per repo), got %d", len(items))
	}
	for _, it := range items {
		im := obj(t, it)
		if im["repositoryName"] == nil {
			t.Error("usageItems row missing repositoryName")
		}
		if _, ok := im["netAmount"]; !ok {
			t.Error("usageItems row missing netAmount")
		}
	}
}
