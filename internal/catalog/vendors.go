package catalog

import "github.com/jchigg2000-git/air-traffic/internal/model"

const (
	dw = model.PlaneDeveloperWorkflow
	dp = model.PlaneDataPolicy
	bg = model.PlaneBudget
	ob = model.PlaneObservability

	vn = model.DispVendorNative
	em = model.DispEnvManaged
	pe = model.DispProxyEnforced
	mo = model.DispMonitorOnly
	uv = model.DispUnverified
)

// ---- Tier 1: deep, byte-identical admin surfaces ----

func openAI() Definition {
	caps := []model.Capability{
		cap("user_management", "User / Invite Management", dw, vn, "Admin API", "/admin/organization/users"),
		cap("project_management", "Project Management", dw, vn, "Admin API", "/admin/organization/projects"),
		cap("model_access", "Per-Project Model Access", dw, vn, "model_permissions allow/deny", "/admin/organization/projects/{id}/model_permissions"),
		cap("spend_alerts", "Spend Alerts (soft)", bg, vn, "thresholds in cents", "/admin/organization/projects/{id}/spend_alerts"),
		cap("rate_limits", "Per-Project Rate Limits", bg, vn, "set per-model ≤ org", "/admin/organization/projects/{id}/rate_limits"),
		cap("usage_metering", "Usage & Cost Metering", bg, vn, "daily granularity, USD", "/v1/organization/usage"),
		cap("audit_logs", "Audit Logs (51 event types)", ob, vn, "cursor-paginated, Enterprise", "/admin/organization/audit_logs"),
		note(cap("hard_spend_cap", "Hard Spend Cap", bg, pe, "org budget no longer hard-stops (2026)", ""), "optional gateway required; else monitor_only"),
		cap("zdr", "Zero Data Retention", dp, mo, "Enterprise-negotiated; store=false", ""),
		cap("training_opt_out", "Training Opt-Out", dp, vn, "contractual default (API)", ""),
		cap("data_residency", "Data Residency (10 regions)", dp, vn, "project region, immutable", ""),
		cap("content_safety", "Org Content-Safety Policy", dp, pe, "no org-level safety API", ""),
		cap("request_logs", "Request/Response Content Logs", ob, mo, "not customer-retrievable via API", ""),
	}
	caps = append(caps, agenticEnvCaps()...)
	m := budgetMetrics("openai", "https://api.openai.com/v1/organization/usage")
	m = append(m,
		stateMetric("training_opt_out", "Training Opt-Out", dp, vn),
		stateMetric("model_access", "Model Access Policy", dw, vn),
		stateMetric("mcp_allow", "MCP Allow-List", dw, em),
	)
	return Definition{ID: "openai", Vendor: "OpenAI", DisplayName: "OpenAI", Family: "api-platform", APIVersion: "2026-03-10", Tier: 1, BAASigned: true, Capabilities: caps, Metrics: m,
		EnvPlatforms: []EnvPlatform{{Platform: "claude_code", Enforcement: model.EnforcementMDMLocked}, {Platform: "github", Enforcement: model.EnforcementServerSide}}}
}

func anthropic() Definition {
	caps := []model.Capability{
		cap("user_management", "User / Invite Management", dw, vn, "Admin API; 5 org roles", "/v1/organizations/users"),
		cap("workspace_management", "Workspace Management", dw, vn, "max 100; archive irreversible", "/v1/organizations/workspaces"),
		cap("workspace_members", "Workspace Member Management", dw, vn, "Admin API", "/v1/organizations/workspaces/{id}/members"),
		cap("rate_limits", "Rate Limits (read-only)", dw, vn, "changes require Console", "/v1/organizations/workspaces/{id}/rate_limits"),
		cap("inference_geo", "Inference Geo Controls", dp, vn, "us 1.1x; Opus/Sonnet 4.6+", "/v1/messages"),
		cap("usage_metering", "Usage Report", bg, vn, "filter: key/workspace/model/geo", "/v1/organizations/usage_report/messages"),
		cap("cost_report", "Cost Report", bg, vn, "USD, daily, by workspace", "/v1/organizations/cost_report"),
		cap("compliance_activities", "Compliance Activity Feed", ob, vn, "30+ events, 180-day", "/v1/compliance/activities"),
		cap("compliance_content", "Chat/File/Project Retrieval", ob, vn, "actual content (Enterprise)", "/v1/compliance/chats"),
		note(cap("model_access", "Per-Workspace Model Access", dw, pe, "no workspace model-permission API", ""), "biggest gap vs OpenAI; env_managed for agentic"),
		cap("workspace_spend_limit", "Workspace Spend Limits", bg, mo, "Console-only; no SET endpoint", ""),
		cap("hard_spend_cap", "Hard Spend Cap", bg, pe, "no hard-cutoff API", ""),
		cap("spend_alerting", "Spend Alerting", bg, uv, "no confirmed creation API", ""),
		cap("training_opt_out", "Training Opt-Out", dp, vn, "contractual default ON", ""),
		cap("zdr", "Zero Data Retention", dp, mo, "eligible orgs; not self-serve", ""),
		cap("retention", "API Log Retention (7-day)", dp, mo, "30-day via DPA only", ""),
		cap("content_safety", "Org Content-Safety Policy", dp, pe, "system prompt only", ""),
		note(cap("pii_redaction", "PII/PHI Redaction (inline)", dp, pe, "inline gateway detector; mask/block pre-forward", "/v1/messages"), "realized by the air-traffic-gateway data plane"),
	}
	caps = append(caps, agenticEnvCaps()...)
	m := budgetMetrics("anthropic", "https://api.anthropic.com/v1/organizations/usage_report/messages")
	m = append(m,
		stateMetric("training_opt_out", "Training Opt-Out", dp, vn),
		stateMetric("inference_geo", "Inference Geo", dp, vn),
		stateMetric("mcp_allow", "MCP Allow-List", dw, em),
		stateMetric("compliance_feed", "Compliance Feed", ob, vn),
	)
	return Definition{ID: "anthropic", Vendor: "Anthropic", DisplayName: "Anthropic", Family: "api-platform", APIVersion: "2026-04-01", Tier: 1, BAASigned: true, Capabilities: caps, Metrics: m,
		EnvPlatforms: []EnvPlatform{{Platform: "claude_code", Enforcement: model.EnforcementMDMLocked}, {Platform: "github", Enforcement: model.EnforcementServerSide}}}
}

func bedrock() Definition {
	caps := []model.Capability{
		cap("model_access", "Model Access (IAM/SCP)", dw, vn, "model-access page retired 2026", ""),
		cap("abac", "Tag-Based Policies (ABAC)", dw, vn, "IAM conditions", ""),
		cap("agentcore_policy", "AgentCore Tool Policy", dw, vn, "default-deny, compiled to Cedar", ""),
		cap("guardrails", "Bedrock Guardrails", dp, vn, "content filters + PII REDACT/ANONYMIZE/BLOCK", ""),
		cap("org_guardrails", "Cross-Account Guardrails", dp, vn, "BEDROCK_POLICY via Organizations", "DescribeEffectivePolicy"),
		cap("invocation_logging", "Request/Response Logging", dp, vn, "S3 + CloudWatch; identity.arn", "PutModelInvocationLoggingConfiguration"),
		cap("audit_logs", "CloudTrail Audit", ob, vn, "all admin + data events", ""),
		cap("usage_metering", "CloudWatch Usage Metrics", bg, vn, "aws/bedrock namespace", ""),
		cap("cost_attribution", "Per-Principal Cost Attribution", bg, vn, "CUR 2.0 / Cost Explorer", ""),
		cap("soft_alerts", "AWS Budgets Alerts", bg, vn, "tag-filtered SNS/email", ""),
		note(cap("hard_spend_cap", "Hard Spend Cap", bg, pe, "no native cap; AWS prescribes proxy", ""), "API-GW/Lambda+DynamoDB proxy; else monitor_only"),
		cap("training_opt_out", "Training Opt-Out", dp, vn, "contractual default", ""),
		cap("data_residency", "Data Residency", dp, vn, "region-scoped, VPC endpoints, CMEK", ""),
	}
	caps = append(caps, agenticEnvCaps()...)
	m := budgetMetrics("bedrock", "https://monitoring.amazonaws.com/aws/bedrock")
	m = append(m,
		stateMetric("guardrails", "Guardrails Active", dp, vn),
		stateMetric("org_guardrails", "Org Guardrail Policy", dp, vn),
		stateMetric("invocation_logging", "Invocation Logging", dp, vn),
	)
	return Definition{ID: "bedrock", Vendor: "AWS Bedrock", DisplayName: "AWS Bedrock", Family: "hyperscaler", APIVersion: "2023-09-30", Tier: 1, BAASigned: true, Capabilities: caps, Metrics: m,
		EnvPlatforms: []EnvPlatform{{Platform: "github", Enforcement: model.EnforcementServerSide}}}
}

func azureOpenAI() Definition {
	caps := []model.Capability{
		cap("rbac", "Model Access (RBAC)", dw, vn, "4 built-in roles via ARM", ""),
		cap("deployment_management", "Deployment + PTU", dw, vn, "ProvisionedManaged SKU", ""),
		cap("content_filters", "Content Filters", dp, vn, "per-deployment, 4 categories × thresholds", ""),
		cap("usage_quota", "TPM Quota / Usage", bg, vn, "Usages API currentValue/limit", "/locations/{loc}/usages"),
		cap("request_logging", "Request/Response Logging", dp, vn, "Diagnostic RequestResponseLog; off by default", ""),
		cap("usage_metering", "Token Metrics", ob, vn, "TokensProcessed; x-ratelimit-* headers", ""),
		cap("cost_budgets", "Cost Management Budgets", bg, vn, "action groups; no native OAI cap", ""),
		cap("training_opt_out", "Training Opt-Out", dp, vn, "Azure DPA / OST", ""),
		cap("data_residency", "Data Residency", dp, vn, "region + Azure Policy", ""),
		note(cap("per_user_routing", "Per-User Model Routing", dw, pe, "RBAC controls mgmt, not runtime model", ""), "gateway required"),
		cap("pii_redaction", "PII Redaction", dp, pe, "separate AI Content Safety / proxy", ""),
		cap("audit_logs", "Activity Log Audit", ob, vn, "control-plane operations", ""),
		cap("agentic_logs", "Agentic Action Logs", ob, uv, "Foundry tracing exists; enterprise API unconfirmed", ""),
	}
	caps = append(caps, agenticEnvCaps()...)
	m := budgetMetrics("azure_openai", "https://management.azure.com/.../usages")
	m = append(m,
		stateMetric("content_filters", "Content Filters", dp, vn),
		stateMetric("request_logging", "Request Logging", dp, vn),
		stateMetric("training_opt_out", "Training Opt-Out", dp, vn),
	)
	return Definition{ID: "azure_openai", Vendor: "Azure OpenAI", DisplayName: "Azure OpenAI Service", Family: "hyperscaler", APIVersion: "2024-10-01", Tier: 1, BAASigned: true, Capabilities: caps, Metrics: m,
		EnvPlatforms: []EnvPlatform{{Platform: "vs_code", Enforcement: model.EnforcementMDMLocked}, {Platform: "github", Enforcement: model.EnforcementServerSide}}}
}

func vertex() Definition {
	caps := []model.Capability{
		cap("iam_roles", "Model Access (IAM)", dw, vn, "aiplatform.* roles + ABAC", ""),
		cap("agent_engine", "Agent Engine Deployment", dw, vn, "VPC-SC compliant", ""),
		cap("tool_governance", "Tool Governance (API Registry)", dw, vn, "org allow/deny for agent tools", ""),
		cap("model_armor", "Model Armor Safety", dp, vn, "prompt/response + injection screening (GA 2026)", ""),
		cap("zdr", "Zero Data Retention", dp, vn, "store=false self-serve", ""),
		note(cap("data_access_logs", "Data Access Audit Logs", ob, vn, "DISABLED by default — common misconfig", ""), "admins must enable Data Read logs"),
		cap("spend_caps", "Project Hard Spend Caps", bg, vn, "Next '26; pauses traffic", ""),
		cap("soft_alerts", "Cloud Billing Budgets", bg, vn, "Pub/Sub or email", ""),
		cap("usage_metering", "Cloud Monitoring AI Metrics", bg, vn, "token/latency/error per model", ""),
		cap("cost_metering", "Billing Export (BigQuery)", bg, vn, "per-label attribution", ""),
		cap("audit_logs", "Cloud Audit Logs", ob, vn, "Admin always-on", ""),
		note(cap("per_user_caps", "Per-User Token Caps", bg, pe, "Spend Caps project-level only", ""), "gateway required"),
		cap("training_opt_out", "Training Opt-Out", dp, vn, "contractual", ""),
	}
	caps = append(caps, agenticEnvCaps()...)
	m := budgetMetrics("vertex", "https://monitoring.googleapis.com/v3/projects")
	m = append(m,
		stateMetric("model_armor", "Model Armor", dp, vn),
		stateMetric("data_access_logs", "Data Access Logs", ob, vn),
		stateMetric("zdr", "Zero Data Retention", dp, vn),
	)
	return Definition{ID: "vertex", Vendor: "Google Vertex", DisplayName: "Google Vertex AI / Gemini", Family: "hyperscaler", APIVersion: "v1", Tier: 1, BAASigned: true, Capabilities: caps, Metrics: m,
		EnvPlatforms: []EnvPlatform{{Platform: "github", Enforcement: model.EnforcementServerSide}}}
}

func githubCopilot() Definition {
	caps := []model.Capability{
		cap("seat_management", "Seat Management", dw, vn, "assign/revoke", "/enterprises/{e}/copilot/billing/seats"),
		cap("content_exclusion", "Content Exclusions", dw, vn, "file/repo glob patterns", "/enterprises/{e}/copilot/content_exclusion"),
		cap("usage_billing", "AI Credit Usage Billing", bg, vn, "1 credit = $0.01", "/enterprises/{e}/billing/usage"),
		cap("metrics", "Copilot Metrics", bg, vn, "28-day rolling, 1-yr history", "/enterprises/{e}/copilot/metrics"),
		cap("audit_logs", "Audit Log + Agent Control Plane", ob, vn, "actor_is_agent, agent_session.task (GA Feb 2026)", ""),
		note(cap("feature_policy", "Feature/Model/MCP Policy", dw, mo, "AI Controls tab; portal-only", ""), "no documented REST API for toggles"),
		capEnv("code_review_gate", "Agentic Code-Review Gate", dw, model.EnforcementServerSide, "GitHub branch protection + required status checks", ""),
		cap("training_opt_out", "Training Opt-Out", dp, vn, "Business/Enterprise default", ""),
		cap("public_code_filter", "Public Code Filter", dp, vn, "duplicate detection policy", ""),
		cap("hard_spend_cap", "Hard Spend Cap / Rate Limit", bg, pe, "no native cap on AI credit spend", ""),
		cap("content_logs", "Prompt/Response Content", ob, mo, "audit metadata only", ""),
		cap("data_residency", "Data Residency", dp, uv, "Copilot-specific residency unclear", ""),
	}
	caps = append(caps,
		capEnv("mcp_allow_deny", "MCP Server Registry", dw, model.EnforcementServerSide, "Copilot MCP registry-only allowlist", ""),
		capEnv("shared_hooks", "Shared Hooks", dw, model.EnforcementMDMLocked, "managed config", ""),
	)
	m := budgetMetrics("github_copilot", "https://api.github.com/enterprises/acme/copilot/metrics")
	m = append(m,
		stateMetric("content_exclusion", "Content Exclusions", dw, vn),
		stateMetric("code_review_gate", "Code-Review Gate", dw, em),
		stateMetric("agent_audit", "Agent Control Plane Audit", ob, vn),
	)
	return Definition{ID: "github_copilot", Vendor: "GitHub Copilot", DisplayName: "GitHub Copilot", Family: "coding-assistant", APIVersion: "2026-03-10", Tier: 1, BAASigned: false, Capabilities: caps, Metrics: m,
		EnvPlatforms: []EnvPlatform{{Platform: "github", Enforcement: model.EnforcementServerSide}, {Platform: "vs_code", Enforcement: model.EnforcementMDMLocked}}}
}

// ---- Tier 2: core surfaces + manifest ----

func m365Copilot() Definition {
	caps := []model.Capability{
		cap("license_assignment", "License Assignment", dw, vn, "Graph group-based licensing", "/groups/{id}/assignLicense"),
		cap("usage_reports", "Usage / Adoption Reports", bg, vn, "Graph Reports API", "/reports/microsoft365CopilotUsageUserDetail"),
		cap("unified_audit", "Purview Unified Audit Log", ob, vn, "AIAppInteraction, AgentId; 180-day", ""),
		cap("dlp_ai", "DLP for AI Interactions", dp, vn, "Purview endpoint DLP", ""),
		cap("sensitivity_labels", "Sensitivity Label Enforcement", dp, vn, "automatic", ""),
		cap("data_residency", "Advanced Data Residency", dp, vn, "ADR + Multi-Geo", ""),
		cap("training_opt_out", "Training Opt-Out", dp, vn, "Microsoft Product Terms", ""),
		cap("spend_cap", "Token-Level Spend Cap", bg, mo, "license-priced; no budgeting", ""),
		cap("app_toggle", "Per-App Copilot Toggle", dw, mo, "Admin Center; portal-only", ""),
		cap("mcp_controls", "MCP Integration Controls", dw, uv, "no org-level MCP policy API", ""),
	}
	m := budgetMetrics("m365_copilot", "https://graph.microsoft.com/v1.0/reports")
	m = append(m, stateMetric("dlp_ai", "DLP for AI", dp, vn), stateMetric("unified_audit", "Unified Audit", ob, vn))
	return Definition{ID: "m365_copilot", Vendor: "Microsoft 365 Copilot", DisplayName: "Microsoft 365 Copilot", Family: "productivity", APIVersion: "v1.0", Tier: 2, BAASigned: true, Capabilities: caps, Metrics: m}
}

func mistral() Definition {
	caps := []model.Capability{
		cap("workspaces", "Organizations + Workspaces", dw, vn, "Admin API (Preview)", "/beta/admin/workspaces"),
		cap("user_management", "User Management + Invites", dw, vn, "Admin API", "/beta/admin/users"),
		cap("api_keys", "API Key Management", dw, vn, "workspace-scoped", "/beta/admin/api-keys"),
		cap("mcp_connectors", "Per-Workspace MCP Connectors", dw, vn, "rare native MCP control", ""),
		cap("billing", "Billing API", bg, vn, "Preview", "/beta/admin/billing"),
		cap("analytics", "Usage / Analytics API", bg, vn, "Preview", "/beta/admin/analytics"),
		cap("spend_limits", "Per-Workspace Spend Limits", bg, vn, "Admin Panel + billing endpoint", ""),
		cap("training_opt_out", "Training Opt-Out", dp, vn, "Teams/Enterprise default", ""),
		cap("retention", "Chat Retention Policy", dp, vn, "30-90d / 180d / 1yr", ""),
		note(cap("model_access", "Per-Workspace Model Access", dw, pe, "no model-level allow/deny", ""), "confirmed negative"),
		cap("audit_logs", "Audit Logs (panel)", ob, mo, "no export / retrieval API", ""),
	}
	m := budgetMetrics("mistral", "https://api.mistral.ai/beta/admin/analytics")
	m = append(m, stateMetric("mcp_connectors", "MCP Connectors", dw, vn), stateMetric("training_opt_out", "Training Opt-Out", dp, vn))
	return Definition{ID: "mistral", Vendor: "Mistral AI", DisplayName: "Mistral AI", Family: "api-platform", APIVersion: "beta-admin", Tier: 2, BAASigned: false, Capabilities: caps, Metrics: m}
}

func databricks() Definition {
	caps := []model.Capability{
		cap("model_policies", "Foundation Model Access Policies", dw, vn, "by provider/country/approval/tags", ""),
		cap("service_policies", "Contextual Service Policies", dw, vn, "allow/deny/approve agentic actions (Beta)", ""),
		cap("mcp_governance", "MCP Service Governance", dw, vn, "Unity Catalog managed integrations", ""),
		cap("pii_injection", "PII + Injection Detection", dp, vn, "policy enforcement layer", ""),
		cap("hard_spend_cap", "Hard Spend Caps w/ Auto-Stop", bg, vn, "Data+AI Summit 2026", ""),
		cap("tracing", "Unified Tracing (model + MCP)", ob, vn, "end-to-end agent traces", ""),
	}
	m := budgetMetrics("databricks", "https://accounts.cloud.databricks.com/api")
	m = append(m, stateMetric("mcp_governance", "MCP Governance", dw, vn), stateMetric("hard_spend_cap", "Hard Spend Cap", bg, vn))
	return Definition{ID: "databricks", Vendor: "Databricks", DisplayName: "Databricks Unity AI Gateway", Family: "hyperscaler", APIVersion: "2.0", Tier: 2, BAASigned: true, Capabilities: caps, Metrics: m}
}

func perplexity() Definition {
	caps := []model.Capability{
		cap("webhook_audit", "Real-Time Webhook Audit Log", ob, vn, "HTTP POST; SIEM-ready; 50+ seats", ""),
		cap("agentic_audit", "Agentic Action / Connector Audit", ob, vn, "step visibility", ""),
		cap("sso", "SSO / SAML 2.0", dw, vn, "Okta, Azure AD, Google", ""),
		cap("scim", "SCIM Provisioning", dw, vn, "directory sync", ""),
		cap("training_opt_out", "No Training on Enterprise Data", dp, vn, "default", ""),
	}
	m := budgetMetrics("perplexity", "https://api.perplexity.ai/enterprise")
	m = append(m, stateMetric("webhook_audit", "Webhook Audit", ob, vn))
	return Definition{ID: "perplexity", Vendor: "Perplexity", DisplayName: "Perplexity Enterprise Pro", Family: "api-platform", APIVersion: "enterprise", Tier: 2, BAASigned: false, Capabilities: caps, Metrics: m}
}

func cohere() Definition {
	caps := []model.Capability{
		cap("training_opt_out", "Training Opt-Out", dp, mo, "Dashboard: Data Controls", ""),
		cap("retention", "30-Day Log Retention", dp, vn, "policy-enforced auto-delete", ""),
		cap("spend_limit", "Monthly Spending Limit", bg, mo, "Dashboard: Billing", ""),
		cap("zdr", "Zero Data Retention", dp, mo, "Enterprise; email support", ""),
		note(cap("per_key_cap", "Per-Key Spend Cap", bg, pe, "org-level only", ""), "no per-key limits"),
	}
	m := budgetMetrics("cohere", "https://api.cohere.com/v1")
	m = append(m, stateMetric("training_opt_out", "Training Opt-Out", dp, mo))
	return Definition{ID: "cohere", Vendor: "Cohere", DisplayName: "Cohere", Family: "api-platform", APIVersion: "v1", Tier: 2, BAASigned: false, Capabilities: caps, Metrics: m}
}

func together() Definition {
	caps := []model.Capability{
		cap("scoped_keys", "Project-Scoped API Keys", dw, vn, "key in project A → project A only", ""),
		cap("key_expiry", "Key Expiration Dates", dw, vn, "settable per-key", ""),
		cap("zdr", "Zero Data Retention", dp, vn, "self-serve toggle", ""),
		cap("training_opt_out", "Training Opt-Out", dp, vn, "opt-in, off by default", ""),
		cap("cost_attribution", "Cost by api_key_id", bg, vn, "returned on inference", ""),
		cap("org_limits", "Org-Level Usage Limits", bg, vn, "apply at org level", ""),
		note(cap("per_key_cap", "Per-Key Spend Cap", bg, pe, "can't cap/rate-limit individual keys", ""), "explicit negative"),
		cap("audit_logs", "Audit Logs", ob, mo, "not documented", ""),
	}
	m := budgetMetrics("together", "https://api.together.xyz/v1")
	m = append(m, stateMetric("zdr", "Zero Data Retention", dp, vn))
	return Definition{ID: "together", Vendor: "Together AI", DisplayName: "Together AI", Family: "api-platform", APIVersion: "v1", Tier: 2, BAASigned: false, Capabilities: caps, Metrics: m}
}

// ---- Tier 3: manifest + emit only ----

func groq() Definition {
	caps := []model.Capability{
		cap("spend_cap", "Monthly Org Spend Cap", bg, mo, "Dashboard UI only", ""),
		cap("alert_thresholds", "Alert Thresholds (50/75/90%)", bg, mo, "UI only", ""),
		cap("rate_limits", "Org-Level Rate Limits", bg, vn, "shared across keys", ""),
		cap("training_opt_out", "Training Opt-Out", dp, uv, "not documented", ""),
	}
	return Definition{ID: "groq", Vendor: "Groq", DisplayName: "Groq", Family: "api-platform", APIVersion: "v1", Tier: 3, BAASigned: false, Capabilities: caps, Metrics: budgetMetrics("groq", "https://api.groq.com/openai/v1")}
}

func xai() Definition {
	caps := []model.Capability{
		cap("team_management", "Team Management", dw, vn, "Grok Business", ""),
		cap("sso", "SSO/SAML + SCIM", dw, vn, "Enterprise tier", ""),
		cap("training_opt_out", "No Training on Business Data", dp, vn, "Business/Enterprise default", ""),
		cap("rate_limits", "Custom Rate Limits", bg, vn, "Enterprise negotiated", ""),
		cap("audit_logs", "Audit Logging", ob, uv, "programmatic API unconfirmed", ""),
	}
	return Definition{ID: "xai", Vendor: "xAI", DisplayName: "xAI Grok", Family: "api-platform", APIVersion: "v1", Tier: 3, BAASigned: false, Capabilities: caps, Metrics: budgetMetrics("xai", "https://api.x.ai/v1")}
}

func amazonQ() Definition {
	caps := []model.Capability{
		cap("identity_center", "IAM Identity Center", dw, vn, "trusted identity propagation", ""),
		cap("guardrails", "Admin Guardrails", dp, vn, "configurable", ""),
		cap("cmek", "KMS CMEK", dp, vn, "customer-managed keys", ""),
		cap("audit_logs", "CloudTrail Audit", ob, vn, "propagated user identity", ""),
		cap("spend_cap", "Per-Token Cap", bg, mo, "subscription tiers; proxy for per-token", ""),
	}
	return Definition{ID: "amazon_q", Vendor: "Amazon Q", DisplayName: "Amazon Q", Family: "hyperscaler", APIVersion: "2024", Tier: 3, BAASigned: true, Capabilities: caps, Metrics: budgetMetrics("amazon_q", "https://docs.aws.amazon.com/amazonq/latest/qbusiness-ug/security-iam.html")}
}

func watsonx() Definition {
	caps := []model.Capability{
		cap("model_config", "Provider/Model/Policy Config", dw, vn, "watsonx.ai", ""),
		cap("agent_catalog", "Agent Catalog Publishing", dw, vn, "watsonx Orchestrate", ""),
		cap("multi_tenancy", "Isolated Tenants", dp, vn, "watsonx.ai v2.4", ""),
		cap("governance", "GRC Governance Framework", ob, vn, "watsonx.governance", ""),
		cap("usage_controls", "Usage Controls", bg, mo, "admin UI", ""),
	}
	return Definition{ID: "watsonx", Vendor: "IBM watsonx", DisplayName: "IBM watsonx", Family: "hyperscaler", APIVersion: "2.4", Tier: 3, BAASigned: true, Capabilities: caps, Metrics: budgetMetrics("watsonx", "https://us-south.ml.cloud.ibm.com")}
}
