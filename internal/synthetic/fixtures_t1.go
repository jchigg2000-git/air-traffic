package synthetic

import (
	"strings"
	"time"

	"air-traffic/internal/model"
)

func init() {
	register("openai", openaiFixture)
	register("anthropic", anthropicFixture)
	register("bedrock", bedrockFixture)
	register("azure_openai", azureFixture)
	register("vertex", vertexFixture)
	register("github_copilot", githubFixture)
	register("mistral", mistralFixture)
}

func nowUnix() int64                { return time.Now().UTC().Unix() }
func nowRFC() string                { return time.Now().UTC().Format(time.RFC3339) }
func agoRFC(d time.Duration) string { return time.Now().UTC().Add(-d).Format(time.RFC3339) }

// ---- OpenAI: Admin API + Usage/Costs (byte-identical envelopes) ----
func openaiFixture(a model.Adapter, method, path string, q map[string][]string) (int, any, map[string]string) {
	switch {
	case strings.Contains(path, "model_permissions"):
		return 200, map[string]any{
			"object": "organization.project.model_permission",
			"mode":   "allow_list",
			"data":   []any{"gpt-4o", "gpt-4o-mini", "o3-mini"},
		}, nil
	case strings.Contains(path, "spend_alerts"):
		return 200, map[string]any{
			"object":     "organization.project.spend_alert",
			"id":         "spend_alert_" + model.NewUUID()[:8],
			"threshold":  500000,
			"interval":   "monthly",
			"created_at": nowUnix(),
		}, nil
	case strings.Contains(path, "rate_limits"):
		return 200, openaiList([]any{
			map[string]any{"object": "project.rate_limit", "id": "rl-gpt-4o", "model": "gpt-4o", "max_requests_per_1_minute": 10000, "max_tokens_per_1_minute": 2000000},
		}), nil
	case strings.Contains(path, "audit_logs"):
		return 200, openaiList([]any{
			map[string]any{"id": "audit_log-" + model.NewUUID()[:12], "type": "api_key.created", "effective_at": nowUnix(),
				"actor":   map[string]any{"type": "session", "session": map[string]any{"user": map[string]any{"id": "user-42", "email": "admin@acme.com"}, "ip_address": "203.0.113.7"}},
				"project": map[string]any{"id": "proj_acme", "name": "acme-prod"}},
			map[string]any{"id": "audit_log-" + model.NewUUID()[:12], "type": "project.updated", "effective_at": nowUnix() - 3600,
				"actor": map[string]any{"type": "session", "session": map[string]any{"user": map[string]any{"id": "user-7", "email": "sec@acme.com"}}}},
		}), nil
	case strings.Contains(path, "/projects"):
		return 200, openaiList([]any{
			map[string]any{"id": "proj_acme", "object": "organization.project", "name": "acme-prod", "created_at": nowUnix() - 86400*30, "archived_at": nil, "status": "active"},
			map[string]any{"id": "proj_lab", "object": "organization.project", "name": "acme-research", "created_at": nowUnix() - 86400*9, "archived_at": nil, "status": "active"},
		}), nil
	case strings.Contains(path, "/users") || strings.Contains(path, "/invites"):
		return 200, openaiList([]any{
			map[string]any{"object": "organization.user", "id": "user-42", "name": "Ada Admin", "email": "admin@acme.com", "role": "owner", "added_at": nowUnix() - 86400*120},
			map[string]any{"object": "organization.user", "id": "user-7", "name": "Sec Eng", "email": "sec@acme.com", "role": "member", "added_at": nowUnix() - 86400*60},
		}), nil
	case strings.Contains(path, "/costs"):
		if field := requestedGroupField(q, "group_by"); field != "" {
			if f, ok := facetFor("openai", field, "project_id", "api_key_id", "line_item"); ok {
				return 200, openaiPage([]any{openaiCostsBucket(f)}), nil
			}
		}
		return 200, openaiPage([]any{openaiBucket("organization.costs.result", map[string]any{
			"amount": map[string]any{"value": 1182.44, "currency": "usd"}, "line_item": nil, "project_id": "proj_acme"})}), nil
	case strings.Contains(path, "/usage"):
		if field := requestedGroupField(q, "group_by"); field != "" {
			if f, ok := facetFor("openai", field, "project_id", "model", "user_id", "api_key_id", "service_tier"); ok {
				return 200, openaiPage([]any{openaiUsageBucket(f)}), nil
			}
		}
		return 200, openaiPage([]any{openaiBucket("organization.usage.completions.result", map[string]any{
			"input_tokens": 142500, "output_tokens": 58210, "input_cached_tokens": 38110, "num_model_requests": 9123, "project_id": "proj_acme", "model": "gpt-4o"})}), nil
	default:
		return 200, openaiList([]any{}), nil
	}
}

func openaiList(data []any) map[string]any {
	first, last := any(nil), any(nil)
	if len(data) > 0 {
		if m, ok := data[0].(map[string]any); ok {
			first = m["id"]
		}
		if m, ok := data[len(data)-1].(map[string]any); ok {
			last = m["id"]
		}
	}
	return map[string]any{"object": "list", "data": data, "first_id": first, "last_id": last, "has_more": false}
}
func openaiPage(data []any) map[string]any {
	return map[string]any{"object": "page", "data": data, "has_more": false, "next_page": nil}
}
func openaiBucket(resultObject string, result map[string]any) map[string]any {
	result["object"] = resultObject
	return map[string]any{"object": "bucket", "start_time": nowUnix() - 86400, "end_time": nowUnix(), "results": []any{result}}
}

// ---- Anthropic: Admin + Usage/Cost + Compliance ----
func anthropicFixture(a model.Adapter, method, path string, q map[string][]string) (int, any, map[string]string) {
	switch {
	case strings.Contains(path, "/workspaces"):
		return 200, anthropicList([]any{
			map[string]any{"type": "workspace", "id": "wrkspc_" + model.NewUUID()[:16], "name": "Production", "created_at": agoRFC(720 * time.Hour), "archived_at": nil, "display_color": "#0891B2"},
			map[string]any{"type": "workspace", "id": "wrkspc_" + model.NewUUID()[:16], "name": "Research", "created_at": agoRFC(220 * time.Hour), "archived_at": nil, "display_color": "#D97757"},
		}), nil
	case strings.Contains(path, "/users") || strings.Contains(path, "/invites"):
		return 200, anthropicList([]any{
			map[string]any{"type": "user", "id": "user_" + model.NewUUID()[:16], "email": "admin@acme.com", "name": "Ada Admin", "role": "admin", "added_at": agoRFC(2880 * time.Hour)},
			map[string]any{"type": "user", "id": "user_" + model.NewUUID()[:16], "email": "dev@acme.com", "name": "Dev One", "role": "developer", "added_at": agoRFC(1440 * time.Hour)},
		}), nil
	case strings.Contains(path, "rate_limits"):
		return 200, anthropicList([]any{
			map[string]any{"type": "rate_limit", "model_tier": "claude-opus", "requests_per_minute": 4000, "input_tokens_per_minute": 400000, "output_tokens_per_minute": 80000},
		}), nil
	case strings.Contains(path, "usage_report"):
		if field := requestedGroupField(q, "group_by"); field != "" {
			if f, ok := facetFor("anthropic", field, "model", "workspace_id", "api_key_id", "service_tier"); ok {
				return 200, anthropicUsageGrouped(f), nil
			}
		}
		return 200, map[string]any{"data": []any{map[string]any{
			"starting_at": agoRFC(24 * time.Hour), "ending_at": nowRFC(),
			"results": []any{map[string]any{"uncached_input_tokens": 142500, "cache_read_input_tokens": 38110, "output_tokens": 58210, "api_key_id": "apikey_01", "workspace_id": "wrkspc_prod", "model": "claude-opus-4-6", "service_tier": "standard"}},
		}}, "has_more": false, "next_page": nil}, nil
	case strings.Contains(path, "cost_report"):
		if field := requestedGroupField(q, "group_by"); field != "" {
			if f, ok := facetFor("anthropic", field, "workspace_id", "description"); ok {
				return 200, anthropicCostGrouped(f), nil
			}
		}
		return 200, map[string]any{"data": []any{map[string]any{
			"starting_at": agoRFC(24 * time.Hour), "ending_at": nowRFC(),
			"results": []any{map[string]any{"amount": "284.10", "currency": "USD", "workspace_id": "wrkspc_prod", "description": "Claude API usage"}},
		}}, "has_more": false, "next_page": nil}, nil
	case strings.Contains(path, "/compliance/activities"):
		return 200, map[string]any{"data": []any{
			map[string]any{"id": "activity_" + model.NewUUID()[:16], "type": "conversation.created", "timestamp": nowRFC(), "actor": map[string]any{"type": "user", "id": "user_01", "email": "dev@acme.com"}},
			map[string]any{"id": "activity_" + model.NewUUID()[:16], "type": "org_config.updated", "timestamp": agoRFC(2 * time.Hour), "actor": map[string]any{"type": "user", "id": "user_admin", "email": "admin@acme.com"}},
		}, "has_more": false, "first_id": nil, "last_id": nil}, nil
	default:
		return 200, anthropicList([]any{}), nil
	}
}
func anthropicList(data []any) map[string]any {
	return map[string]any{"data": data, "has_more": false, "first_id": nil, "last_id": nil}
}

// ---- AWS Bedrock: operation-specific JSON ----
func bedrockFixture(a model.Adapter, method, path string, q map[string][]string) (int, any, map[string]string) {
	switch {
	case strings.Contains(path, "logging"):
		return 200, map[string]any{"loggingConfig": map[string]any{
			"cloudWatchConfig":        map[string]any{"logGroupName": "/aws/bedrock/modelinvocations", "roleArn": "arn:aws:iam::123456789012:role/bedrock-logging"},
			"s3Config":                map[string]any{"bucketName": "acme-bedrock-logs", "keyPrefix": "invocations/"},
			"textDataDeliveryEnabled": true, "imageDataDeliveryEnabled": true, "embeddingDataDeliveryEnabled": true,
		}}, nil
	case strings.Contains(path, "effective"):
		return 200, map[string]any{"policy": map[string]any{"policyArn": "arn:aws:organizations::123456789012:policy/o-acme/service_control_policy/p-bedrock",
			"guardrailIdentifier": "g-12345abc", "effectiveGuardrailVersion": "DRAFT", "appliedAcrossAccounts": true}}, nil
	case strings.Contains(path, "guardrail"):
		return 200, map[string]any{"guardrails": []any{map[string]any{
			"id": "g-12345abc", "arn": "arn:aws:bedrock:us-east-1:123456789012:guardrail/g-12345abc", "name": "acme-pii-and-safety",
			"status": "READY", "version": "DRAFT", "createdAt": nowRFC(), "updatedAt": nowRFC()}}, "nextToken": nil}, nil
	default:
		return 200, map[string]any{"modelSummaries": []any{
			map[string]any{"modelArn": "arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-opus-4-6", "modelId": "anthropic.claude-opus-4-6", "providerName": "Anthropic", "modelLifecycle": map[string]any{"status": "ACTIVE"}},
		}}, nil
	}
}

// ---- Azure OpenAI: ARM + Usages ----
func azureFixture(a model.Adapter, method, path string, q map[string][]string) (int, any, map[string]string) {
	switch {
	case strings.Contains(path, "usages"):
		return 200, map[string]any{"value": []any{
			map[string]any{"name": map[string]any{"value": "OpenAI.Standard.gpt-4o", "localizedValue": "Tokens Per Minute (thousands) - gpt-4o"}, "currentValue": 320.0, "limit": 450.0, "unit": "Count"},
			map[string]any{"name": map[string]any{"value": "OpenAI.Standard.o3-mini", "localizedValue": "Tokens Per Minute (thousands) - o3-mini"}, "currentValue": 90.0, "limit": 250.0, "unit": "Count"},
		}}, nil
	case strings.Contains(path, "raiPolicies") || strings.Contains(path, "ContentFilter"):
		return 200, map[string]any{"value": []any{map[string]any{
			"name": "acme-elevated", "type": "Microsoft.CognitiveServices/accounts/raiPolicies",
			"properties": map[string]any{"mode": "Blocking", "contentFilters": []any{
				map[string]any{"name": "hate", "blocking": true, "enabled": true, "severityThreshold": "Medium", "source": "Prompt"},
				map[string]any{"name": "violence", "blocking": true, "enabled": true, "severityThreshold": "Medium", "source": "Completion"},
			}}}}}, nil
	case strings.Contains(path, "deployments"):
		return 200, map[string]any{"value": []any{map[string]any{
			"id":   "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.CognitiveServices/accounts/acme/deployments/gpt-4o",
			"name": "gpt-4o", "type": "Microsoft.CognitiveServices/accounts/deployments",
			"properties": map[string]any{"model": map[string]any{"format": "OpenAI", "name": "gpt-4o", "version": "2024-11-20"}, "provisioningState": "Succeeded"},
			"sku":        map[string]any{"name": "ProvisionedManaged", "capacity": 200}}}}, nil
	default:
		return 200, map[string]any{"value": []any{}}, nil
	}
}

// ---- Google Vertex: Cloud Monitoring / Logging / models ----
func vertexFixture(a model.Adapter, method, path string, q map[string][]string) (int, any, map[string]string) {
	switch {
	case strings.Contains(path, "timeSeries"):
		return 200, map[string]any{"timeSeries": []any{map[string]any{
			"metric":   map[string]any{"type": "aiplatform.googleapis.com/prediction/online/token_count", "labels": map[string]any{"model_id": "gemini-2.5-pro"}},
			"resource": map[string]any{"type": "aiplatform.googleapis.com/Endpoint", "labels": map[string]any{"project_id": "acme-prod", "location": "us-central1"}},
			"points":   []any{map[string]any{"interval": map[string]any{"startTime": agoRFC(time.Hour), "endTime": nowRFC()}, "value": map[string]any{"int64Value": "142500"}}},
		}}, "unit": "1"}, nil
	case strings.Contains(path, "entries") || strings.Contains(path, "logging"):
		return 200, map[string]any{"entries": []any{map[string]any{
			"logName":      "projects/acme-prod/logs/cloudaudit.googleapis.com%2Fdata_access",
			"resource":     map[string]any{"type": "audited_resource", "labels": map[string]any{"project_id": "acme-prod"}},
			"timestamp":    nowRFC(),
			"protoPayload": map[string]any{"@type": "type.googleapis.com/google.cloud.audit.AuditLog", "serviceName": "aiplatform.googleapis.com", "methodName": "google.cloud.aiplatform.v1.PredictionService.GenerateContent", "authenticationInfo": map[string]any{"principalEmail": "svc-agent@acme-prod.iam.gserviceaccount.com"}},
		}}, "nextPageToken": ""}, nil
	default:
		return 200, map[string]any{"models": []any{map[string]any{
			"name": "projects/acme-prod/locations/us-central1/models/gemini-2.5-pro", "displayName": "Gemini 2.5 Pro", "versionId": "1"}}, "nextPageToken": ""}, nil
	}
}

// ---- GitHub Copilot: REST shapes (arrays + objects) ----
func githubFixture(a model.Adapter, method, path string, q map[string][]string) (int, any, map[string]string) {
	switch {
	case strings.Contains(path, "content_exclusion"):
		return 200, map[string]any{"*": []any{".env", "secrets/**"}, "acme/payments": []any{"**/*.key"}}, nil
	case strings.Contains(path, "billing/usage"):
		// Enhanced Billing Platform returns one usageItems[] row per repo (real shape).
		return 200, map[string]any{"usageItems": githubUsageItems()}, nil
	case strings.Contains(path, "/metrics"):
		// Copilot metrics nest languages[] and editors[].models[] (pre-broken-down).
		return 200, githubMetrics(), nil
	case strings.Contains(path, "audit") || strings.Contains(path, "audit-log"):
		return 200, []any{
			map[string]any{"action": "copilot.agent_session.start", "actor": "Copilot", "actor_is_agent": true, "created_at": nowUnix() * 1000, "user": "dev-one", "agent_session": map[string]any{"task": "implement-feature"}},
			map[string]any{"action": "business.update_copilot_business_policy", "actor": "ada-admin", "actor_is_agent": false, "created_at": (nowUnix() - 7200) * 1000},
		}, nil
	default: // seats — one seats[] row per assignee
		return 200, githubSeats(), nil
	}
}

// ---- Mistral: OpenAI-shaped admin (Tier 2 depth sample) ----
func mistralFixture(a model.Adapter, method, path string, q map[string][]string) (int, any, map[string]string) {
	switch {
	case strings.Contains(path, "workspaces"):
		return 200, map[string]any{"object": "list", "data": []any{map[string]any{
			"id": "ws_" + model.NewUUID()[:12], "object": "workspace", "name": "acme-prod", "created_at": nowRFC(), "mcp_connectors_enabled": true}}}, nil
	case strings.Contains(path, "analytics") || strings.Contains(path, "billing"):
		return 200, map[string]any{"object": "list", "data": []any{map[string]any{
			"date": time.Now().UTC().Format("2006-01-02"), "workspace_id": "ws_prod", "total_tokens": 200710, "cost": map[string]any{"amount": 142.5, "currency": "EUR"}}}}, nil
	default:
		return 200, map[string]any{"object": "list", "data": []any{}}, nil
	}
}
