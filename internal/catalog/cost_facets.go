package catalog

// cost_facets.go is the single source of truth for per-vendor cost/usage drill-down
// dimensions. Both the byte-identical synthetic grouped responses (internal/synthetic)
// and the emitter's per-member cost breakdown (internal/emitter) read from this map, so
// the two surfaces can never disagree.
//
// Each vendor lists every candidate dimension it was assessed against. Supported ones
// carry the real group-by param, endpoint, the exact JSON response field the grouped key
// appears under, and canonical members. Unsupported ones carry Supported:false + an honest
// Reason — surfaced (greyed) in the Cost Explorer so the product never implies a breakdown
// a vendor's real API can't actually produce.
//
// Grounded in docs/air-traffic-analysis.md §3 vendor research (24-agent study verified
// against live admin/billing APIs, June 2026; adversarial-verify corrections applied).

// cf builds a supported cost facet.
func cf(dim, label, realParam, endpoint, respField, reason string, members ...FacetMember) CostFacet {
	return CostFacet{Dimension: dim, Label: label, Supported: true, RealParam: realParam, Endpoint: endpoint, ResponseField: respField, Reason: reason, Members: members}
}

// uf builds an unsupported cost facet (honest "no native breakdown" note).
func uf(dim, label, reason string) CostFacet {
	return CostFacet{Dimension: dim, Label: label, Supported: false, Reason: reason}
}

// fm builds one facet member (weight is a relative spend share, normalized at read time).
func fm(key, label string, weight float64) FacetMember {
	return FacetMember{Key: key, Label: label, Weight: weight}
}

var costFacetsByID = map[string][]CostFacet{
	"openai": {
		cf("project", "By Project", "group_by[]=project_id", "/v1/organization/costs", "project_id", "Primary attribution unit; the only dimension the Costs API breaks USD spend down by, alongside line_item.",
			fm("proj_acme", "acme-prod", 70), fm("proj_lab", "acme-research", 30)),
		cf("model", "By Model", "group_by[]=model", "/v1/organization/usage/completions", "model", "Usage API only (tokens/requests); model appears in Costs only inside the line_item string.",
			fm("gpt-4o", "gpt-4o", 55), fm("gpt-4o-mini", "gpt-4o-mini", 30), fm("o3-mini", "o3-mini", 15)),
		cf("user", "By User", "group_by[]=user_id", "/v1/organization/usage/completions", "user_id", "Usage API only; null for service-account keys, so per-user USD spend is not native.",
			fm("user-42", "Ada Admin", 60), fm("user-7", "Sec Eng", 40)),
		cf("api_key", "By API Key", "group_by[]=api_key_id", "/v1/organization/usage/completions", "api_key_id", "Groupable on both usage and costs reports, so per-key USD spend is native.",
			fm("key_acme_prod_a1b2", "acme-prod key", 65), fm("key_lab_ci_c3d4", "lab-ci key", 35)),
		cf("sku", "By SKU / Line Item", "group_by[]=line_item", "/v1/organization/costs", "line_item", "Finest USD-cost breakdown OpenAI exposes; line_item embeds model + token-type as a billing SKU.",
			fm("gpt-4o, input", "gpt-4o input", 50), fm("gpt-4o, output", "gpt-4o output", 30), fm("gpt-4o-mini, input", "gpt-4o-mini input", 20)),
		cf("service_tier", "By Service Tier", "group_by[]=service_tier", "/v1/organization/usage/completions", "service_tier", "A genuine Usage group_by enum (default/flex/priority).",
			fm("default", "default", 70), fm("flex", "flex", 20), fm("priority", "priority", 10)),
		uf("workspace", "By Workspace", "OpenAI's org sub-unit is the project, not a workspace; there is no workspace_id group_by."),
		uf("repo", "By Repo", "The Usage/Costs API has no source-repository concept."),
		uf("team", "By Team / Tag", "group_by accepts only a fixed enum; there is no team or cost-allocation-tag dimension."),
		uf("region", "By Region", "A project's data-residency region is immutable metadata, not a group_by dimension."),
	},
	"anthropic": {
		cf("workspace", "By Workspace", "group_by[]=workspace_id", "/v1/organizations/cost_report", "workspace_id", "The only dimension groupable on both usage and cost reports; Anthropic's primary cost-attribution unit.",
			fm("wrkspc_prod", "Production", 65), fm("wrkspc_research", "Research", 35)),
		cf("model", "By Model", "group_by[]=model", "/v1/organizations/usage_report/messages", "model", "Usage report only; the cost report folds model into the free-text description.",
			fm("claude-opus-4-6", "claude-opus-4-6", 60), fm("claude-sonnet-4-6", "claude-sonnet-4-6", 40)),
		cf("api_key", "By API Key", "group_by[]=api_key_id", "/v1/organizations/usage_report/messages", "api_key_id", "Usage report only; the cost report groups solely by workspace_id and description.",
			fm("apikey_01", "apikey_01", 60), fm("apikey_02", "apikey_02", 40)),
		cf("service_tier", "By Service Tier", "group_by[]=service_tier", "/v1/organizations/usage_report/messages", "service_tier", "Usage report only; standard vs priority vs batch throughput tiers.",
			fm("standard", "standard", 70), fm("priority", "priority", 20), fm("batch", "batch", 10)),
		cf("sku", "By SKU / Line Item", "group_by[]=description", "/v1/organizations/cost_report", "description", "The cost report's per-line-item descriptor (model + token-type), the closest real SKU.",
			fm("Claude Opus 4.6 - Input Tokens", "Opus 4.6 Input", 45), fm("Claude Sonnet 4.6 - Output Tokens", "Sonnet 4.6 Output", 30), fm("Cache Read", "Cache Read", 25)),
		uf("user", "By User", "Usage/cost attribute to API keys and workspaces, never to individual user identities."),
		uf("project", "By Project", "The hierarchy is org→workspace→API key; there is no project entity (workspace_id is the equivalent)."),
		uf("repo", "By Repo", "Anthropic's API platform has no source-repository concept."),
		uf("region", "By Region", "Inference-geo is a runtime control on /v1/messages, not a result-level group_by."),
		uf("team", "By Team / Tag", "No arbitrary tag/team cost-allocation concept beyond workspace on either report."),
	},
	"bedrock": {
		cf("model", "By Model", "Dimensions:[{Name:ModelId}] (AWS/Bedrock namespace)", "CloudWatch GetMetricData", "MetricDataResults[].Label", "ModelId is first-class in CloudWatch for tokens/invocations, though not a standalone Cost Explorer dimension.",
			fm("anthropic.claude-opus-4-6", "claude-opus-4-6", 60), fm("anthropic.claude-sonnet-4-6", "claude-sonnet-4-6", 40)),
		cf("workspace", "By Account", "GroupBy:[{Type:DIMENSION,Key:LINKED_ACCOUNT}]", "Cost Explorer GetCostAndUsage", "Groups[].Keys", "The AWS analog of a workspace is an Organizations member account (the native LINKED_ACCOUNT dimension).",
			fm("123456789012", "acme-prod", 65), fm("210987654321", "acme-research", 35)),
		cf("project", "By Project", "GroupBy:[{Type:TAG,Key:Project}]", "Cost Explorer GetCostAndUsage", "Groups[].Keys", "Via an activated user-defined cost-allocation tag named Project, not a built-in dimension.",
			fm("Project$acme-prod", "acme-prod", 70), fm("Project$acme-research", "acme-research", 30)),
		cf("team", "By Team / Tag", "GroupBy:[{Type:TAG,Key:Team}]", "Cost Explorer GetCostAndUsage", "Groups[].Keys", "The AWS-prescribed Bedrock cost-attribution mechanism: activated cost-allocation tags / Cost Categories.",
			fm("Team$payments", "payments", 55), fm("Team$research", "research", 30), fm("CostCenter$ml-platform", "ml-platform", 15)),
		cf("sku", "By SKU / Usage Type", "GroupBy:[{Type:DIMENSION,Key:USAGE_TYPE}]", "Cost Explorer GetCostAndUsage", "Groups[].Keys", "USAGE_TYPE is the AWS-native SKU and most granular breakdown.",
			fm("USE1-InputTokenCount", "us-east-1 input", 50), fm("USE1-OutputTokenCount", "us-east-1 output", 30), fm("USE1-CacheReadInputTokenCount", "us-east-1 cache-read", 20)),
		cf("region", "By Region", "GroupBy:[{Type:DIMENSION,Key:REGION}]", "Cost Explorer GetCostAndUsage", "Groups[].Keys", "Works via console/CLI; REGION is undocumented in the GetCostAndUsage GroupBy list.",
			fm("us-east-1", "us-east-1", 60), fm("us-west-2", "us-west-2", 25), fm("eu-west-1", "eu-west-1", 15)),
		cf("deployment", "By Inference Profile", "GroupBy:[{Type:TAG,Key:InferenceProfile}]", "Cost Explorer GetCostAndUsage", "Groups[].Keys", "Closest analog to a deployment is a tagged application inference profile.",
			fm("InferenceProfile$acme-prod-claude", "acme-prod-claude", 70), fm("InferenceProfile$acme-staging-claude", "acme-staging-claude", 30)),
		uf("user", "By User", "Neither Cost Explorer nor CloudWatch has a per-end-user dimension; identity.arn lives only in invocation logs."),
		uf("api_key", "By API Key", "Cost Explorer has no api_key dimension and CloudWatch metrics aren't keyed by API key."),
		uf("service_tier", "By Service Tier", "PURCHASE_TYPE values are reservation options, not Bedrock tiers; can't separate on-demand/provisioned/batch."),
		uf("repo", "By Repo", "AWS billing has no source-repository concept."),
	},
	"azure_openai": {
		cf("model", "By Model", "grouping=[{type:Dimension,name:Meter}] (cost) / metric dim ModelName (tokens)", "Microsoft.CostManagement/query", "rows[] @ Meter", "Model is derived from the billed Meter for cost; clean token splits are native via the ModelName metric.",
			fm("gpt-4o", "gpt-4o", 55), fm("gpt-4o-mini", "gpt-4o-mini", 30), fm("o3-mini", "o3-mini", 15)),
		cf("deployment", "By Deployment", "metric dim ModelDeploymentName", "microsoft.insights/metrics", "ModelDeploymentName", "Per-deployment token usage/quota is first-class via Azure Monitor; USD is billed at the parent account.",
			fm("gpt-4o", "gpt-4o", 55), fm("o3-mini", "o3-mini", 25), fm("gpt-4o-mini", "gpt-4o-mini", 20)),
		cf("sku", "By SKU", "grouping=[{type:Dimension,name:Meter}]", "Microsoft.CostManagement/query", "rows[] @ Meter", "The billed SKU/product is a native Cost Management facet (Meter/Product); overlaps service_tier.",
			fm("ProvisionedManaged", "ProvisionedManaged", 50), fm("Standard", "Standard", 30), fm("GlobalStandard", "GlobalStandard", 20)),
		cf("service_tier", "By Service Tier", "grouping=[{type:Dimension,name:MeterSubcategory}]", "Microsoft.CostManagement/query", "rows[] @ MeterSubcategory", "Azure's tier is the deployment SKU (Standard/GlobalStandard/ProvisionedManaged).",
			fm("ProvisionedManaged", "ProvisionedManaged", 50), fm("GlobalStandard", "GlobalStandard", 30), fm("Standard", "Standard", 20)),
		cf("region", "By Region", "grouping=[{type:Dimension,name:ResourceLocation}]", "Microsoft.CostManagement/query", "rows[] @ ResourceLocation", "ResourceLocation is a standard Cost Management dimension and Region a standard metric dimension.",
			fm("eastus", "eastus", 60), fm("swedencentral", "swedencentral", 25), fm("westeurope", "westeurope", 15)),
		cf("team", "By Team / Tag", "grouping=[{type:TagKey,name:team}]", "Microsoft.CostManagement/query", "TagKey/TagValue columns", "Native TagKey grouping is the canonical Azure cost-attribution mechanism; untagged spend lands in a null bucket.",
			fm("payments", "payments", 50), fm("acme-ml", "acme-ml", 30), fm("prod", "prod", 20)),
		uf("user", "By User", "Azure OpenAI passes no end-user identity to billing or Azure Monitor metrics."),
		uf("api_key", "By API Key", "Resource keys / Entra tokens are not a billing or metric facet."),
		uf("project", "By Project", "No native Project group-by; a Foundry project is just an Azure resource."),
		uf("repo", "By Repo", "Azure has no repository concept for Azure OpenAI cost or metrics."),
	},
	"vertex": {
		cf("model", "By Model", "groupByFields=resource.label.model_user_id", "monitoring.googleapis.com/v3/.../timeSeries", "resource.labels.model_user_id", "Native token splits via Monitoring; cost-by-model is BigQuery-SKU only.",
			fm("gemini-2.5-pro", "gemini-2.5-pro", 60), fm("gemini-2.5-flash", "gemini-2.5-flash", 40)),
		cf("project", "By Project", "groupByFields=resource.label.resource_container", "monitoring.googleapis.com/v3/.../timeSeries", "resource.labels.resource_container", "Primary GCP tenancy unit; the real Monitoring label is resource_container.",
			fm("acme-prod", "acme-prod", 70), fm("acme-lab", "acme-lab", 30)),
		cf("region", "By Region", "groupByFields=resource.label.location", "monitoring.googleapis.com/v3/.../timeSeries", "resource.labels.location", "Supported on both Monitoring and BigQuery billing export.",
			fm("us-central1", "us-central1", 60), fm("us-east4", "us-east4", 25), fm("europe-west4", "europe-west4", 15)),
		cf("deployment", "By Endpoint", "groupByFields=resource.label.endpoint_id", "monitoring.googleapis.com/v3/.../timeSeries", "resource.labels.endpoint_id", "Only for models on a dedicated Endpoint; serverless Gemini publisher calls have no endpoint_id.",
			fm("7300123456789", "acme-prod-endpoint", 60), fm("5512098761234", "acme-lab-endpoint", 25), fm("deployed-gemini-2.5-pro-prod", "gemini-2.5-pro prod", 15)),
		cf("sku", "By SKU / Line Item", "BigQuery GROUP BY sku.description", "BigQuery gcp_billing_export_resource_v1_*", "sku.description", "Canonical dollar-cost dimension, BigQuery-export-only.",
			fm("Gemini 2.5 Pro Input", "Gemini 2.5 Pro Input", 40), fm("Gemini 2.5 Pro Output", "Gemini 2.5 Pro Output", 30), fm("Gemini 2.5 Flash Input", "Gemini 2.5 Flash Input", 20), fm("Vertex AI Provisioned Throughput GSU", "Provisioned Throughput GSU", 10)),
		cf("team", "By Team / Tag", "BigQuery UNNEST(labels) WHERE key='team'", "BigQuery gcp_billing_export_resource_v1_*", "labels", "First-class GCP label/tag cost attribution, BigQuery-export-only.",
			fm("team:payments", "payments", 50), fm("cost-center:ml-platform", "ml-platform", 30), fm("env:production", "production", 20)),
		uf("user", "By User", "No end-user dimension; the only 'who' is the IAM principal in audit logs, not aggregated cost/tokens."),
		uf("workspace", "By Workspace", "GCP's hierarchy is project→folder→org; there is no workspace label."),
		uf("api_key", "By API Key", "Auth is IAM/OAuth; no api_key dimension on Vertex cost/tokens."),
		uf("repo", "By Repo", "No repository concept anywhere in GCP billing or Cloud Monitoring."),
	},
	"github_copilot": {
		cf("repo", "By Repo", "aggregate usageItems[] by repositoryName", "/enterprises/{e}/settings/billing/usage", "repositoryName", "The distinctive GitHub dimension; only repo-attributable usage is populated.",
			fm("acme/payments", "acme/payments", 50), fm("acme/web", "acme/web", 30), fm("acme/infra", "acme/infra", 20)),
		cf("user", "By User", "seats[] is one row per assignee", "/enterprises/{e}/copilot/billing/seats", "seats[].assignee.login", "Seat-licensed, so per-user cost is the flat seat; premium-request overage isn't per-user in REST.",
			fm("dev-one", "dev-one", 55), fm("ada-admin", "ada-admin", 45)),
		cf("sku", "By SKU", "aggregate usageItems[] by sku", "/enterprises/{e}/settings/billing/usage", "sku", "The strongest native cost dimension; each line carries quantity (AI Credits) and netAmount (USD).",
			fm("copilot_premium_request", "Premium Request", 60), fm("Copilot Enterprise", "Copilot Enterprise", 25), fm("Copilot Business", "Copilot Business", 15)),
		cf("model", "By Model", "Metrics nests models[] under editors[]", "/enterprises/{e}/copilot/metrics", "editors[].models[].name", "Engagement usage only (not dollars); models nest under editors[].",
			fm("gpt-4o", "gpt-4o", 40), fm("claude-sonnet-4-6", "claude-sonnet-4-6", 30), fm("o3-mini", "o3-mini", 20), fm("gemini-2.5-pro", "gemini-2.5-pro", 10)),
		cf("language", "By Language / Editor", "Metrics nests languages[] under code-completions", "/enterprises/{e}/copilot/metrics", "languages[].name", "Engagement counts per language and editor, not cost.",
			fm("go", "go", 40), fm("python", "python", 30), fm("typescript", "typescript", 20), fm("vscode", "VS Code", 10)),
		cf("team", "By Team / Cost Center", "team_slug path segment / cost_center_id filter", "/enterprises/{e}/team/{team_slug}/copilot/metrics", "team_slug", "Teams give a usage breakdown; cost centers a cost-allocation filter (enterprise-only).",
			fm("platform", "platform", 45), fm("payments-squad", "payments-squad", 30), fm("Data-Science", "Data-Science", 25)),
		uf("project", "By Project", "No project billing unit; the nearest analog is the cost center under team."),
		uf("api_key", "By API Key", "Copilot is seat-licensed, not API-key metered."),
		uf("region", "By Region", "Neither billing/usage nor metrics breaks down by region."),
		uf("service_tier", "By Service Tier", "No inference service tier; the only tiering is the plan type, surfaced as a SKU."),
	},
	"mistral": {
		cf("workspace", "By Workspace", "workspace_id filter (no server-side group_by)", "/v1/admin/usage", "workspace_id", "Workspace is a filter, not a group_by; billing itself aggregates by fixed service category.",
			fm("ws_prod", "acme-prod", 65), fm("ws_research", "acme-research", 35)),
		uf("model", "By Model", "Billing breaks down by service category (chat/completion/ocr/…), not model; no group_by=model exists."),
		uf("user", "By User", "Analytics/billing attribute usage only to workspace; there is no user_id group-by."),
		uf("api_key", "By API Key", "Usage/spend rolls up to the workspace, not the individual key."),
		uf("project", "By Project", "The hierarchy is org→workspace; the workspace is the project-equivalent."),
		uf("region", "By Region", "EU-hosted; data residency is an account attribute, not a usage group-by."),
		uf("repo", "By Repo", "Mistral has no source-control concept."),
	},
	"together": {
		cf("model", "By Model", "per-response model field (client-side aggregate; no server group_by)", "/v1/chat/completions", "model", "model is a real per-response field but only client-side-aggregable; no server-side usage/cost group_by.",
			fm("meta-llama/Llama-3.3-70B-Instruct-Turbo", "Llama-3.3-70B", 45), fm("Qwen/Qwen2.5-72B-Instruct-Turbo", "Qwen2.5-72B", 25), fm("deepseek-ai/DeepSeek-V3", "DeepSeek-V3", 20), fm("mistralai/Mixtral-8x7B-Instruct-v0.1", "Mixtral-8x7B", 10)),
		uf("api_key", "By API Key", "The live response has no api_key_id field and no usage/cost API groups by key."),
		uf("user", "By User", "Cost is attributed to the bearer key, never to an org user identity."),
		uf("project", "By Project", "Keys scope to projects for access control only; the cost record returns no project_id."),
		uf("workspace", "By Workspace", "No workspace entity in Together's billing/usage model."),
		uf("repo", "By Repo", "Raw inference API with no repository concept."),
		uf("team", "By Team / Tag", "Requests carry no user-supplied tag/metadata/cost-center field."),
	},
	"m365_copilot": {
		cf("user", "By User", "report is one row per user", "/v1.0/reports/getMicrosoft365CopilotUsageUserDetail", "userPrincipalName", "Activity/adoption granularity (last-activity dates, engaged flags), not tokens or per-user dollars.",
			fm("admin@acme.com", "Ada Admin", 40), fm("dev-one@acme.com", "Dev One", 35), fm("sec@acme.com", "Sec Eng", 25)),
		cf("sku", "By SKU / License", "per-SKU rows", "/v1.0/subscribedSkus", "skuPartNumber", "Truest cost grouping for a license-priced product, though it returns license counts (consumedUnits), not dollars.",
			fm("Microsoft_365_Copilot", "M365 Copilot", 70), fm("Microsoft_365_Copilot_Chat", "M365 Copilot Chat", 30)),
		cf("language", "By App", "per-app *CopilotLastActivityDate columns", "/v1.0/reports/getMicrosoft365CopilotUsageUserDetail", "wordCopilotLastActivityDate", "Per-app activity (Word/Excel/Teams), encoded as columns not rows, activity-only.",
			fm("Word", "Word", 30), fm("Excel", "Excel", 25), fm("Teams", "Teams", 20), fm("Outlook", "Outlook", 15), fm("PowerPoint", "PowerPoint", 10)),
		uf("model", "By Model", "M365 Copilot abstracts the underlying foundation model; admins get no per-model usage."),
		uf("repo", "By Repo", "No repository concept in a Word/Excel/Teams productivity assistant."),
		uf("project", "By Project", "No project dimension in M365 Copilot usage/billing."),
		uf("team", "By Team / Tag", "Group-based license assignment exists, but the usage API doesn't emit cost grouped by team."),
	},
	"databricks": {
		cf("workspace", "By Workspace", "GROUP BY workspace_id", "system.billing.usage", "workspace_id", "First-class, always-populated column; numeric IDs map to the workspaces.",
			fm("1444828305810485", "acme-prod", 65), fm("2548219034857123", "acme-research", 35)),
		cf("model", "By Model", "GROUP BY served_entities.entity_name", "system.serving.served_entities ⋈ system.billing.usage", "served_entity_name", "Indirect — usage has no model column; resolved via the served_entities join.",
			fm("databricks-claude-opus-4-6", "claude-opus-4-6", 45), fm("databricks-meta-llama-3-3-70b-instruct", "llama-3.3-70b", 35), fm("gpt-4o", "gpt-4o", 20)),
		cf("user", "By User", "GROUP BY identity_metadata.run_as", "system.billing.usage", "run_as", "Attributes serverless spend to the user/service principal; classic compute attributes to the cluster.",
			fm("admin@acme.com", "Ada Admin", 40), fm("dev@acme.com", "Dev One", 35), fm("svc-agent@acme-prod", "svc-agent", 25)),
		cf("team", "By Team / Tag", "GROUP BY custom_tags['team']", "system.billing.usage", "custom_tags", "THE canonical Databricks cost-attribution mechanism (a MAP of user-defined tags).",
			fm("payments", "payments", 40), fm("ml-platform", "ml-platform", 30), fm("research", "research", 20), fm("data-eng", "data-eng", 10)),
		cf("sku", "By SKU", "GROUP BY sku_name", "system.billing.usage", "sku_name", "The primary billing dimension, present on every row.",
			fm("PREMIUM_FOUNDATION_MODEL_SERVING", "Foundation Model Serving", 45), fm("PREMIUM_ALL_PURPOSE_COMPUTE", "All-Purpose Compute", 30), fm("PREMIUM_JOBS_SERVERLESS_COMPUTE", "Jobs Serverless", 25)),
		cf("deployment", "By Endpoint", "GROUP BY usage_metadata.endpoint_name", "system.billing.usage", "endpoint_name", "A Databricks AI deployment is a Model Serving endpoint.",
			fm("acme-claude-chat", "acme-claude-chat", 50), fm("acme-rag-llama", "acme-rag-llama", 30), fm("databricks-claude-opus-4-6", "claude-opus-4-6", 20)),
		cf("service_tier", "By Serving Type", "GROUP BY product_features.serving_type", "system.billing.usage", "serving_type", "Derived field (PAY_PER_TOKEN vs PROVISIONED_THROUGHPUT).",
			fm("PAY_PER_TOKEN", "Pay Per Token", 55), fm("PROVISIONED_THROUGHPUT", "Provisioned Throughput", 45)),
		uf("repo", "By Repo", "DBUs are never attributed per git repository in system.billing.usage."),
		uf("api_key", "By API Key", "Auth is PAT/OAuth; there is no api_key column in system.billing.usage."),
		uf("region", "By Region", "No geographic-region column; the only native locality is cloud (AWS/AZURE/GCP)."),
	},
	"amazon_q": {
		cf("user", "By User", "per-user subscription principal (IAM Identity Center)", "Amazon Q ListSubscriptions", "subscriptions[].principal", "Seat/subscription granularity; Cost Explorer can't group token cost by end user.",
			fm("user-42", "user-42", 40), fm("user-7", "user-7", 35), fm("admin@acme.com", "Ada Admin", 25)),
		cf("team", "By Team / Tag", "GroupBy Type=TAG Key=team", "Cost Explorer GetCostAndUsage", "Groups[].Keys[0]", "Cost-allocation tags / Cost Categories are the idiomatic AWS way to attribute Q spend.",
			fm("team$payments", "payments", 50), fm("team$platform", "platform", 30), fm("cost-center$eng", "eng", 20)),
		cf("service_tier", "By Subscription", "subscription type via ListSubscriptions", "Amazon Q ListSubscriptions", "subscriptions[].type", "Subscription tier is Q's primary pricing axis (Free vs Pro / Lite vs Pro).",
			fm("Q_DEVELOPER_PRO", "Q Developer Pro", 55), fm("Q_BUSINESS_PRO", "Q Business Pro", 25), fm("Q_DEVELOPER_FREE", "Q Developer Free", 20)),
		cf("sku", "By SKU / Usage Type", "GroupBy Type=DIMENSION Key=USAGE_TYPE", "Cost Explorer GetCostAndUsage", "Groups[].Keys[0]", "USAGE_TYPE separates seat subscription vs index capacity vs agentic-request overage.",
			fm("USE1-Q-Developer-Pro", "Q Developer Pro", 50), fm("Q-Business-Index-Capacity-Unit", "Q Business Index Capacity", 30), fm("USE1-Q-Developer-AgenticRequest", "Agentic Request", 20)),
		cf("region", "By Region", "GroupBy Type=DIMENSION Key=REGION", "Cost Explorer GetCostAndUsage", "Groups[].Keys[0]", "Standard Cost Explorer dimension; Q spend groups cleanly by region.",
			fm("us-east-1", "us-east-1", 60), fm("us-west-2", "us-west-2", 25), fm("eu-west-1", "eu-west-1", 15)),
		uf("model", "By Model", "A managed assistant that abstracts the foundation model; customers aren't billed per model."),
		uf("repo", "By Repo", "Cost and activity are never attributed to a repo."),
		uf("api_key", "By API Key", "Authenticates via IAM / Identity Center, not API keys."),
	},
	"watsonx": {
		cf("team", "By Resource Group", "resource_group_id path segment", "billing.cloud.ibm.com/v4/.../resource_groups/{id}/usage", "resource_group_name", "Resource groups are IBM Cloud's cost-center/team boundary with a dedicated endpoint.",
			fm("acme-prod", "acme-prod", 45), fm("acme-research", "acme-research", 30), fm("data-science", "data-science", 25)),
		cf("sku", "By SKU / Plan", "plan_id / resource_id itemization", "billing.cloud.ibm.com/v4/.../usage", "plan_name", "IBM Cloud billing is organized by resource_id + plan (the SKU).",
			fm("watsonx.ai Runtime Standard", "Runtime Standard", 50), fm("watsonx.ai Studio", "Studio", 30), fm("watsonx.governance", "Governance", 20)),
		cf("region", "By Region", "region (per-resource field)", "billing.cloud.ibm.com/v4/.../usage", "region", "Each billing resource carries its region and is itemized per region.",
			fm("us-south", "us-south", 50), fm("eu-de", "eu-de", 30), fm("jp-tok", "jp-tok", 20)),
		cf("deployment", "By Service Instance", "resource_instance_id CRN path segment", "billing.cloud.ibm.com/v4/.../resource_instances/{id}/usage", "resource_instance_name", "Each provisioned watsonx.ai service instance is an IBM Cloud resource_instance.",
			fm("watsonxai-prod", "watsonxai-prod", 60), fm("watsonxai-dev", "watsonxai-dev", 40)),
		uf("user", "By User", "Billing attributes to account/group/instance/plan/region, never to the IAM user."),
		uf("model", "By Model", "No aggregate endpoint groups cost or tokens by foundation model."),
		uf("api_key", "By API Key", "Billing doesn't attribute cost or tokens by API key or service ID."),
		uf("repo", "By Repo", "watsonx has no source-code-repository concept."),
	},
	// Vendors with NO real server-side cost/usage group-by — honest empty drill-down.
	"perplexity": {
		uf("user", "By User", "Seat-based; no usage/cost-reporting API groups spend by user."),
		uf("model", "By Model", "model is echoed per-response but there is no usage/cost-reporting endpoint or group_by."),
		uf("api_key", "By API Key", "No usage/cost-by-key reporting API; the key id isn't echoed in responses."),
		uf("project", "By Project", "No project construct and no project-scoped cost/usage endpoint."),
		uf("sku", "By SKU / Line Item", "The inline usage.cost split is per-response metadata, not an aggregated group_by."),
	},
	"cohere": {
		uf("user", "By User", "No usage/cost API exists; usage is org-aggregate dashboard-only."),
		uf("model", "By Model", "Inline meta.billed_units is per-request, not a groupable surface."),
		uf("api_key", "By API Key", "Explicit negative: keys are only typed Trial vs Production with no per-key breakdown."),
		uf("sku", "By SKU / Line Item", "Billing units are distinguished inline but no programmatic API groups by SKU."),
	},
	"groq": {
		uf("user", "By User", "Usage is attributed to API keys (console-only), with no programmatic per-user breakdown."),
		uf("model", "By Model", "model is echoed per-response and charted in the console, but there is no group_by=model endpoint."),
		uf("api_key", "By API Key", "Per-key usage is dashboard-only; no public endpoint returns usage grouped by key."),
		uf("service_tier", "By Service Tier", "service_tier exists on the request path only, not as a reporting dimension."),
	},
	"xai": {
		uf("model", "By Model", "The console can break down by model, but no usage/cost endpoint accepts group_by=model."),
		uf("team", "By Team", "Teams are the real billing unit but there is no programmatic cost endpoint or group_by=team_id."),
		uf("api_key", "By API Key", "GET /v1/api-key returns key metadata but no spend/usage."),
		uf("user", "By User", "Consumption is attributed to API keys and teams, never to individual members."),
	},
}

// CostFacetsFor returns the assessed cost drill-down facets for a vendor (nil if none).
func CostFacetsFor(id string) []CostFacet { return costFacetsByID[id] }
