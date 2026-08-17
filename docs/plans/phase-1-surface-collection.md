# Phase 1 — Surface Collection & Synthetic Byte-Identical Backend

> **Goal:** Stand up the Air-Traffic Go backend that deeply *collects* every vendor
> control surface defined in the design docs and serves a **byte-identical synthetic**
> replica of each one — plus the control-plane API, the `ops-observation-batch/v1`
> emitter, policy/baseline/drift engine, and env-config rendering. No frontend in
> this phase (Phase 2). Modifies **only** the `air-traffic` repo.

Status: **BUILT** — this document is a build-history record of the plan as written, not open work.
The backend below shipped; current status lives in [`../../ROADMAP.md`](../../ROADMAP.md) §1, which
wins on any conflict. (Its §13 acceptance checkboxes were never formally re-run line-by-line —
tracked as `OWED-3` in the roadmap.) · Owner: Justin · Ports: API `8122` (`AIRTRAFFIC_ADDR`), web dev `5202` (Phase 2)
Sibling reference (read-only): `../it-scorecard`

---

## 0. Design thesis

Air-Traffic is an **enterprise AI control plane** — a *spine*, not an inference proxy.
The spine has three cooperating parts (per system-design §1–2):

1. **Control plane** — drives each vendor's *native admin API* (`vendor_native`).
2. **Config distributor** (`internal/envconfig`) — renders managed config into dev/agent
   environments and reads it back for drift (`env_managed`). **No request interception.**
3. **Observability** — a background emitter producing `ops-observation-batch/v1` batches,
   a normalized cross-vendor audit stream, and drift detection.

The optional inference gateway (system-design §11) is **out of Phase 1 scope** — off by
default, nothing on the spine imports it.

### What "byte-identical synthetic surface" means here

For every vendor admin/control surface in the matrix below we serve a synthetic HTTP
endpoint at `/synthetic/{vendor}/{native-path}` whose **response body shape, field names,
status codes, headers, and pagination envelope match the real vendor API**. This mirrors
it-scorecard's `/synthetic/{id}/{native-path}` handler (a `switch c.Type` that fabricates
vendor-native payloads), extended from observability connectors to **AI-vendor admin APIs**.
A consumer pointed at our synthetic surface cannot tell the shape apart from the real one.

The typed `VendorAdapter` interface sits *on top* of these surfaces: in `synthetic` mode its
methods read/echo the synthetic fixtures; in `proxy` mode (future) they drive the real vendor
API. Phase 1 ships `synthetic` (+ `disabled`) fully; `proxy` is stubbed with the it-scorecard
"reached upstream but no normalizer" behavior.

---

## 1. Five dispositions (truthfulness layer)

Every capability a vendor exposes carries exactly one disposition. This is the spine's
honesty contract — the UI must never overstate enforcement.

| Disposition | Const | Color | Mechanism | On request path? |
|---|---|---|---|---|
| **VendorNative** | `vendor_native` | green `#16A34A` | Drive vendor admin API | No |
| **EnvManaged** | `env_managed` | teal `#0891B2` | Push managed config + read back for drift | No |
| **ProxyEnforced** | `proxy_enforced` | purple `#7C3AED` | Optional inline gateway (off by default) | Yes (optional) |
| **MonitorOnly** | `monitor_only` | slate `#64748B` | Observe / verify / gate / alert — no *set* | No |
| **Unverified** | `unverified` | amber `#D97706` | Not confirmed from live docs | No |
| **Unsupported** | `unsupported` | red `#DC2626` | Vendor explicitly cannot | No |

**Rule:** a `proxy_enforced` control with the gateway off renders as `monitor_only`.

### `env_managed` enforcement tiers

Each `env_managed` capability also carries `enforcement ∈ {server_side, mdm_locked, seed_only}`:

- **server_side** (Tier A, non-bypassable): GitHub/GitLab branch-protection & rulesets,
  Copilot MCP registry-only, GitLab Duo cascading lock, Gemini/Cursor-Enterprise/Amazon-Q admin gating.
- **mdm_locked** (Tier B, enforced only on MDM-enrolled devices): Claude Code
  `managed-settings.json`, VS Code enterprise policies, Windsurf. Gated behind a verified-MDM check.
- **seed_only** (Tier C, user can override): Cursor local MCP/dotfiles, JetBrains, unmanaged device.
  Distribute + drift-detect only. **UI never renders Tier-C as "enforced."**

---

## 2. Repo / package layout (mirrors it-scorecard, extends per system-design §15)

```
air-traffic/
├── go.mod                                  # module air-traffic, go 1.26, zero external deps
├── cmd/
│   └── air-traffic-server/
│       └── main.go                         # config + lifecycle (mirrors cmd/harness-server)
├── internal/
│   ├── model/
│   │   ├── model.go                        # Adapter, Disposition, Capability, ManagedArtifact, EnvState, ObservationRecord, Policy, Baseline, AuditEvent
│   │   └── contract.go                     # ObservationContract const + batch builder
│   ├── store/
│   │   ├── store.go                        # in-memory store (RWMutex, FIFO ring buffers max 5000)
│   │   └── seed.go                         # seed the vendor adapter catalog + capabilities + baselines
│   ├── adapter/
│   │   ├── adapter.go                       # VendorAdapter interface + Signal + registry + base helpers
│   │   ├── openai.go                        # Tier-1 deep
│   │   ├── anthropic.go                     # Tier-1 deep
│   │   ├── bedrock.go                       # Tier-1 deep
│   │   ├── azure_openai.go                  # Tier-1 deep
│   │   ├── vertex.go                        # Tier-1 deep
│   │   ├── github_copilot.go                # Tier-1 deep (control + env half)
│   │   ├── m365_copilot.go                  # Tier-2
│   │   ├── mistral.go                       # Tier-2
│   │   ├── databricks.go                    # Tier-2
│   │   ├── perplexity.go                    # Tier-2
│   │   ├── cohere.go                        # Tier-2
│   │   ├── together.go                      # Tier-2
│   │   ├── groq.go                          # Tier-3 (manifest + emit)
│   │   ├── xai.go                           # Tier-3
│   │   ├── amazon_q.go                      # Tier-3
│   │   └── watsonx.go                       # Tier-3
│   ├── synthetic/
│   │   ├── synthetic.go                     # /synthetic/{vendor}/... router + harness control paths
│   │   ├── fixtures_openai.go               # byte-identical response generators per vendor
│   │   ├── fixtures_anthropic.go
│   │   ├── fixtures_bedrock.go
│   │   ├── fixtures_azure.go
│   │   ├── fixtures_vertex.go
│   │   ├── fixtures_github.go
│   │   └── fixtures_misc.go                 # tier-2/3 vendors
│   ├── emitter/
│   │   ├── emitter.go                       # background loop, Seed() backfill, per-adapter Emit
│   │   └── catalog.go                       # per-vendor per-plane metric defs (baseline/min/max/step/thresholds)
│   ├── envconfig/
│   │   ├── render.go                        # policy → ManagedArtifact (claude-code managed-settings.json, GH branch protection, VS Code, Cursor)
│   │   ├── distribute.go                    # synthetic "push" (records intent; no real channel)
│   │   └── readback.go                      # synthetic effective-state read for drift
│   ├── policy/
│   │   ├── policy.go                        # Policy struct + load/merge (baseline ⊕ overrides)
│   │   ├── baselines.go                     # General SaaS / Fintech / Healthcare / Gov profiles
│   │   ├── reconcile.go                     # apply intent across vendor_native + env_managed; coverage report
│   │   └── drift.go                         # compare declared intent vs actual; emit drift observations
│   ├── audit/
│   │   └── audit.go                         # normalized AuditEvent stream (OTel GenAI semconv) + SIEM-shaped export
│   ├── server/
│   │   ├── server.go                        # New(), Routes(), recover middleware, SPA fallback (Phase 2)
│   │   ├── routes.go                        # /api/* handlers
│   │   └── http.go                          # JSON encode/decode helpers, error→status mapping, 2MB body limit
│   └── redact/
│       └── redact.go                        # redact secret headers/query/body; HasPlaintextSecretKey (reused pattern)
├── schemas/
│   ├── ops-observation-batch-v1.schema.json # reused verbatim from it-scorecard
│   └── air-traffic-capability.schema.json   # capability/disposition manifest schema (new)
├── web/                                     # Phase 2 (placeholder dir only in Phase 1)
└── docs/
    ├── plans/phase-1-surface-collection.md  # this file
    └── plans/phase-2-frontend.md
```

**Conventions mirrored from it-scorecard:** stdlib-only HTTP (`net/http` + `ServeMux`),
`slog` logging, snake_case JSON tags, `json.NewEncoder`/`Decoder` with `DisallowUnknownFields`,
`io.LimitReader` 2 MB cap, single recover middleware, graceful shutdown (10 s), `sync.RWMutex`
store, mean-reverting random-walk synthetic metrics, FIFO ring buffers, `secret_ref`-only
credentials (plaintext rejected).

---

## 3. Core data model (`internal/model`)

```go
const ObservationContract = "ops-observation-batch/v1"

type Mode string // "disabled" | "synthetic" | "proxy"
type Disposition string // vendor_native | env_managed | proxy_enforced | monitor_only | unverified | unsupported
type Plane string // developer_workflow | data_policy | budget | observability
type Enforcement string // server_side | mdm_locked | seed_only  (env_managed only)

type Adapter struct {
    ID          string `json:"id"`          // "openai"
    Vendor      string `json:"vendor"`      // "OpenAI"
    DisplayName string `json:"display_name"`
    Family      string `json:"family"`      // "api-platform" | "hyperscaler" | "coding-assistant" | "productivity"
    APIVersion  string `json:"api_version"` // e.g. "2026-03-10"
    Tier        int    `json:"tier"`        // 1 | 2 | 3 fidelity
    Mode        Mode   `json:"mode"`        // default synthetic
    Enabled     bool   `json:"enabled"`
    Emit        bool   `json:"emit"`
    BasePath    string `json:"base_path"`   // "/synthetic/openai"
    UpstreamURL string `json:"upstream_url"`// proxy mode target (empty in synthetic)
    Scenario    string `json:"scenario"`    // healthy|401|403|429-retry-after|500|503|timeout|empty
    BAASigned   bool   `json:"baa_signed"`  // healthcare baseline gate
    Capabilities []Capability `json:"capabilities"`
    Status      Status `json:"status"`
    UpdatedAt   time.Time `json:"updated_at"`
}

type Capability struct {
    Key             string      `json:"key"`             // "model_access" | "spend_alerts" | "mcp_allow_deny" ...
    Name            string      `json:"name"`
    Plane           Plane       `json:"plane"`
    Disposition     Disposition `json:"disposition"`
    Enforcement     Enforcement `json:"enforcement,omitempty"` // env_managed only
    Mechanism       string      `json:"mechanism"`       // human note: "managed-settings.json", "POST /admin/.../spend_alerts"
    Endpoint        string      `json:"endpoint,omitempty"` // real vendor path the synthetic surface mirrors
    DocumentationURL string     `json:"documentation_url,omitempty"`
    Retryable       bool        `json:"retryable"`
}

type Signal struct { // returned alongside every adapter call
    Disposition Disposition `json:"disposition"`
    Code        string      `json:"code"`    // success|not_supported|credential_missing|rate_limit|drift|...
    Message     string      `json:"message"`
    Retryable   bool        `json:"retryable"`
}

type ManagedArtifact struct {
    Platform        string         `json:"platform"`     // claude_code | github | vs_code | cursor
    Identifier      string         `json:"identifier"`
    Artifact        map[string]any `json:"artifact"`     // platform-specific JSON (e.g. managed-settings.json contents)
    Enforcement     Enforcement    `json:"enforcement"`
    DistributionURL string         `json:"distribution_url"`
}

type EnvState struct {
    Platform       string         `json:"platform"`
    Identifier     string         `json:"identifier"`
    ActualSettings map[string]any `json:"actual_settings"`
    Source         string         `json:"source"` // locked | user_override | default
    DriftDetected  bool           `json:"drift_detected"`
    DriftMessage   string         `json:"drift_message,omitempty"`
}

type ObservationRecord struct { // it-scorecard compatible
    ID                int64          `json:"id"`
    ReceivedAt        time.Time      `json:"received_at"`
    Contract          string         `json:"contract"`
    ConnectorType     string         `json:"connector_type"`     // "ai-vendor"
    ConnectorInstance string         `json:"connector_instance"` // "openai"
    Complete          bool           `json:"complete"`
    ObservationCount  int            `json:"observation_count"`
    ErrorCount        int            `json:"error_count"`
    Body              map[string]any `json:"body"`               // full ops-observation-batch/v1 batch
}

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
```

### `ops-observation-batch/v1` batch shape (per analysis §7.1) — emitted per adapter

```json
{
  "contract": "ops-observation-batch/v1",
  "batch_id": "uuid",
  "connector": { "type": "ai-vendor", "instance": "openai", "api_version": "2026-03-10" },
  "collected_at": "RFC3339",
  "window": { "from": "RFC3339", "to": "RFC3339" },
  "cursor": { "input": null, "output": null },
  "complete": true,
  "observations": [
    {
      "kind": "metric",                       // metric | event | state
      "signal": { "name": "tokens_in", "value": 142500, "unit": "tokens", "status": "green", "severity": "info" },
      "dimensions": { "plane": "budget", "vendor": "openai", "team": "platform-engineering", "model": "gpt-4o", "control_surface": "vendor_native" },
      "provenance": { "fixture": "vendor_native", "source_url": "https://api.openai.com/v1/organization/usage" }
    }
  ],
  "errors": []
}
```

`kind`: `metric` (usage/cost/cap util) · `event` (policy change/drift) · `state` (env_managed config state).

---

## 4. VendorAdapter interface (`internal/adapter`) — per analysis §5.2

```go
type VendorAdapter interface {
    // identity & manifest
    VendorID() string
    APIVersion() string
    Manifest() model.AdapterManifest          // capability catalog with dispositions

    // developer-workflow plane
    ListModels(ctx, ListModelsRequest)   (ListModelsResponse, Signal, error)
    SetModelAccess(ctx, ModelAccessRequest) (Signal, error)
    GetAgentPolicies(ctx)                (AgentPolicies, Signal, error)
    SetAgentPolicies(ctx, AgentPolicies) (Signal, error)

    // data-policy plane
    GetDataPolicy(ctx)            (DataPolicy, Signal, error)
    SetDataPolicy(ctx, DataPolicy)(Signal, error)
    GetRetentionPolicy(ctx)       (RetentionPolicy, Signal, error)
    SetRetentionPolicy(ctx, RetentionPolicy)(Signal, error)
    GetContentSafetyPolicy(ctx)   (ContentSafetyPolicy, Signal, error)
    SetContentSafetyPolicy(ctx, ContentSafetyPolicy)(Signal, error)

    // budget plane
    GetSpendLimits(ctx)            (SpendLimits, Signal, error)
    SetSpendLimits(ctx, SpendLimits)(Signal, error)
    GetUsage(ctx, UsageRequest)    (UsageReport, Signal, error)

    // env-managed half
    RenderManagedConfig(ctx, AgentPolicies)(model.ManagedArtifact, Signal, error)
    ReadEnvState(ctx)             (model.EnvState, Signal, error)

    // observability
    GetAuditLogs(ctx, AuditLogRequest)  (AuditLogBatch, Signal, error)
    GetUsageMetrics(ctx, MetricsRequest)(MetricsBatch, Signal, error)

    // emission (it-scorecard compatible)
    Emit(ctx, ts time.Time)(model.ObservationRecord, error)
}
// NO Forward() — request forwarding is the optional gateway's job only.
```

**Implementation strategy.** A `baseAdapter` struct provides shared synthetic behavior
(walk state, manifest storage, `Signal` helpers, default `monitor_only`/`unsupported`
returns for capabilities a vendor lacks). Each vendor file embeds `baseAdapter` and overrides
only the methods where its surface is real, wiring them to the synthetic fixtures. A package
registry (`map[string]func() VendorAdapter`) registers all adapters; `store.seed()` instantiates them.

---

## 5. Per-vendor synthetic control-surface inventory

Each row → a synthetic endpoint at `/synthetic/{vendor}/{path}` returning a byte-identical
body, **and** a `Capability` in the adapter manifest with its disposition. Tier-1 vendors get
the full endpoint set; Tier-2 get the highlighted core; Tier-3 get manifest + emit only.

### 5.1 OpenAI (Tier 1) — `api_version` `2026-03-10`
- `GET /admin/organization/users`, `POST /admin/organization/invites`, `PATCH/DELETE /admin/organization/users/{id}` → user/invite objects — **vendor_native**
- `POST /admin/organization/projects`, `PATCH /admin/organization/projects/{id}` → project objects — **vendor_native**
- `PATCH /admin/organization/projects/{id}/model_permissions` `{mode: allow_list|deny_list, models:[...]}` — **vendor_native**
- `POST /admin/organization/projects/{id}/spend_alerts` `{thresholds (cents), interval: monthly|custom}` — **vendor_native**
- `GET/POST /admin/organization/projects/{id}/rate_limits` — **vendor_native**
- `GET /admin/organization/audit_logs` (51 event types, cursor-paginated) — **vendor_native**
- `GET /v1/organization/usage`, `GET /v1/organization/costs` (daily granularity, USD) — **vendor_native**
- Hard spend cap — **proxy_enforced** (confirmed: org budget no longer hard-stops 2026) → renders monitor_only
- ZDR, abuse-monitoring opt-out, request/response content logs — **monitor_only**
- MCP allow/deny, hooks, code-review — **env_managed**

### 5.2 Anthropic (Tier 1) — Admin + Usage/Cost + Compliance + Rate-Limits APIs
- `POST /v1/organizations/invites`, `GET/PATCH/DELETE /v1/organizations/users/{id}` — **vendor_native**
- `POST /v1/organizations/workspaces` (max 100; archive irreversible), members CRUD — **vendor_native**
- `GET /v1/organizations/workspaces/{id}/rate_limits` (read-only) — **vendor_native**
- `inference_geo` on `/v1/messages` (`us` 1.1×) + workspace `allowed_inference_geos`/`default_inference_geo` — **vendor_native**
- `GET /v1/organizations/usage_report/messages` (filters: api_key, workspace, model, service_tier, inference_geo; 1m/1h/1d) — **vendor_native**
- `GET /v1/organizations/cost_report` (USD, daily, by workspace) — **vendor_native**
- `GET /v1/compliance/activities` (30+ event types, 180-day), `/v1/compliance/chats|files|projects` (actual content) — **vendor_native (Enterprise)**
- Model access per workspace — **env_managed / proxy_enforced** (no workspace model-permission API — biggest gap)
- Workspace spend limits (Console-only), ZDR, 7-day retention — **monitor_only**
- Spend alerting — **unverified**

### 5.3 AWS Bedrock (Tier 1)
- IAM/SCP model access (model-access page retired 2026; default-available) — **vendor_native**
- `CreateGuardrail` / content filters (hate, violence, sexual, misconduct, prompt-attack; PII REDACT/ANONYMIZE/BLOCK) — **vendor_native**
- Cross-account `BEDROCK_POLICY` via Organizations; `DescribeEffectivePolicy` (layered, versioned) — **vendor_native**
- AgentCore Policy (default-deny tool use, compiled to Cedar) + Guardrails-in-policy (GA Jun 17 2026) — **vendor_native**
- `PutModelInvocationLoggingConfiguration` (S3 + CloudWatch; full payload ≤100KB inline; `identity.arn`) — **vendor_native**
- CloudTrail audit; CloudWatch `aws/bedrock` (InputTokenCount, OutputTokenCount, InvocationLatency); Cost Explorer/CUR 2.0 — **vendor_native**
- AWS Budgets (tag-filtered SNS/email) — **vendor_native** (alert only)
- Hard spend cap — **proxy_enforced** (no native cap; AWS prescribes API-GW/Lambda+DynamoDB proxy) → monitor_only
- MCP/hooks/cron — **env_managed**

### 5.4 Azure OpenAI (Tier 1) — `api-version` `2024-10-01`
- RBAC roles (OpenAI User/Contributor, Cognitive Services Contributor, Usages Reader) via ARM — **vendor_native**
- Deployment CRUD + PTU (`ProvisionedManaged` SKU) via ARM — **vendor_native**
- Content Filters API (per-deployment: hate/violence/sexual/self-harm × low/med/high/block) — **vendor_native**
- `GET .../locations/{loc}/usages?api-version=2024-10-01` (`currentValue`/`limit`), Model Capacities API — **vendor_native**
- Diagnostic `RequestResponseLog` → Log Analytics (`requestPayload_s`/`responsePayload_s`; off by default) — **vendor_native**
- Azure Monitor metrics (`TokensProcessed`, `GeneratedTokens`, `ProcessedPromptTokens`); `x-ratelimit-*` headers — **vendor_native**
- Azure Cost Management Budgets + action groups — **vendor_native** (no native OpenAI-level cap)
- Per-user model routing / per-user rate sub-limits, PII redaction — **proxy_enforced**
- Agentic-action logs — **unverified**

### 5.5 Google Vertex / Gemini (Tier 1)
- IAM roles (`aiplatform.admin/user/endpoints.predict`), ABAC tags, custom roles — **vendor_native**
- Agent Engine deployment (VPC-SC), API-registry tool governance — **vendor_native**
- Model Armor (GA 2026: prompt/response safety + injection screening, org-enforceable) — **vendor_native**
- `store=false` per-request ZDR (self-serve); org ZDR via form — **vendor_native**
- Cloud Audit Logs (Admin always-on; **Data Access OFF by default** — flag the misconfig) — **vendor_native**
- Cloud Billing Budgets API (Pub/Sub/email), project-level hard caps (Next '26, pauses traffic) — **vendor_native**
- Cloud Monitoring AI metrics; BigQuery billing export (per-label) — **vendor_native**
- Per-user/team token caps (project-level only), per-request safety override lockdown — **proxy_enforced**
- MCP allow/deny, hooks, cron — **env_managed**

### 5.6 GitHub Copilot (Tier 1) — `api-version` `2026-03-10`
- `GET /enterprises/{e}/copilot/billing/seats`, `POST/DELETE /orgs/{org}/copilot/billing/selected_users` — **vendor_native**
- `GET/PUT /enterprises/{e}/copilot/content_exclusion` (+ org scope) — **vendor_native**
- `GET /enterprises/{e}/billing/usage` (1 AI Credit = $0.01; Enterprise $39/user incl. $39 credits) — **vendor_native**
- `GET /enterprises/{e}/copilot/metrics` (28-day rolling, 1-yr history; by team/language/editor) — **vendor_native**
- Audit Log API (+ agent control plane GA Feb 26 2026: `actor_is_agent`, `agent_session.task`; 180-day) — **vendor_native**
- Feature/model/MCP policy toggles — **vendor_native (portal-only; no REST)** → manifest notes portal-only
- Agentic code-review level / test-coverage gate → **env_managed** (`server_side` — GH branch protection + required status checks)
- Prompt/response content — **monitor_only**

### 5.7 Tier-2 core surfaces
- **M365 Copilot:** Graph license assignment, `GET /reports/microsoft365CopilotUsageUserDetail`, Purview Unified Audit Log (`AIAppInteraction`, `AgentId`/`AgentName`, 180-day), DLP/sensitivity-label — mostly **vendor_native**; budget — **monitor_only** (license-priced).
- **Mistral AI:** `/beta/admin/workspaces|users|user-groups|api-keys|billing|analytics`, per-workspace MCP Connectors (rare native MCP control) — **vendor_native**; audit export — **monitor_only** (panel-only); per-workspace model access — **proxy_enforced**.
- **Databricks Unity AI Gateway:** foundation-model access policies, Contextual Service Policies (Beta), MCP governance, hard spend caps w/ auto-stop, unified tracing — **vendor_native** (richest agentic).
- **Perplexity Enterprise:** real-time webhook audit log (SIEM-ready, 50+ seats), SSO/SCIM — **vendor_native**.
- **Cohere:** training opt-out, 30-day retention, monthly spend limit — **vendor_native**; per-key cap — **proxy_enforced**.
- **Together AI:** project-scoped keys, key expiry, ZDR self-serve toggle, cost-by-`api_key_id` — **vendor_native**; per-key spend cap — **proxy_enforced** (explicit negative).

### 5.8 Tier-3 (manifest + emit only)
- **Groq:** org monthly spend cap + alert thresholds (UI-only) — **vendor_native**; training opt-out — **unverified**.
- **xAI Grok:** team mgmt, SSO/SCIM, no-train default, custom rate limits — **vendor_native**; audit — **unverified**.
- **Amazon Q:** IAM Identity Center, guardrails, CMEK, CloudTrail — **vendor_native**; per-token cap — **monitor_only**.
- **IBM watsonx:** provider/model/policy config, agent catalog, multi-tenancy, governance GRC — **vendor_native**.

---

## 6. Synthetic surface mechanics (`internal/synthetic`)

Mirrors it-scorecard `synthetic.go`:

1. Router: `/synthetic/{vendorID}/...` → look up adapter; reject unknown with vendor-shaped 404.
2. Harness control sub-paths (not part of vendor API — Air-Traffic control):
   - `GET  /synthetic/{id}/_harness/manifest` → `{vendor, mode, supported_modes, supported_scenarios, capabilities}`
   - `PUT  /synthetic/{id}/_harness/scenario[/{name}]` → set scenario override
   - `POST /synthetic/{id}/_harness/reset` → clear recorded calls
   - `GET  /synthetic/{id}/_harness/calls` → recorded calls (redacted)
3. Every non-harness call recorded via `store.RecordCall` with redacted headers/query/body.
4. `disabled` mode → vendor-shaped 503. Scenario overrides (`401/403/429-retry-after/500/503/timeout/empty/invalid-json`) take precedence and emit the **vendor's own error envelope** (e.g. OpenAI `{"error":{"message","type","code"}}`, AWS `{"__type","message"}`, Azure `{"error":{"code","message"}}`).
5. Default `synthetic` → dispatch to the vendor fixture generator producing the byte-identical success body. Latency simulated with `time.Sleep`.

**Fidelity rule:** fixtures reproduce the real envelope exactly — OpenAI list objects use
`{"object":"list","data":[...],"first_id","last_id","has_more"}`; Anthropic uses `{"data":[...],"has_more","first_id","last_id"}`;
AWS uses its operation-specific JSON; GitHub uses its array/object shapes with the documented
field names. Pagination cursors, `*_at` epoch vs RFC3339 per vendor, and header casing all match.

---

## 7. Emitter (`internal/emitter`)

Mirrors it-scorecard's background emitter:
- `Seed()` backfills 12 ticks of history before `Run`.
- `Run(ctx)` ticks every `AIRTRAFFIC_EMIT_INTERVAL_SECONDS` (default 5).
- Per tick, per emitting adapter: build one `ops-observation-batch/v1` batch spanning the
  vendor's planes — budget metrics (`tokens_in`, `tokens_out`, `cost_usd`, `cap_utilization`),
  data-policy state (`training_opt_out`, `retention_days`, `content_safety`), developer-workflow
  state (`model_access`, `mcp_allow`), and an env_managed `state` observation per managed capability.
- Synthetic values via mean-reverting random walk (`delta + (baseline-cur)*0.06`, clamped).
- `disabled`/non-emit adapters skipped. `proxy` mode → "reached upstream, no normalizer" error batch (it-scorecard parity).
- Drift: when an adapter's effective state diverges from declared policy intent, emit a
  `kind=state, signal=drift, severity=warning` observation (amber) — feeds Flight Deck DRIFT column.

`catalog.go` holds per-vendor metric defs (`{Key,Name,Unit,Plane,Baseline,Min,Max,Step,Green,Amber,Polarity}`).

---

## 8. Policy / baselines / drift (`internal/policy`)

- **Baselines** (`baselines.go`): `General SaaS 🔒`, `Fintech 🔒🔒`, `Healthcare 🔒🔒🔒`, `Gov/Infra 🔒🔒🔒`
  with the exact settings from analysis §6 (model access, training opt-out, ZDR, PII, content
  safety, retention, org/user caps, code-review). Healthcare gates to `baa_signed` adapters.
- **Policy** (`policy.go`): the policy-as-code struct from analysis §8 (`baseline`, `vendor_defaults`,
  `vendors.{id}.{model_access,spend_alerts,content_safety}`, `agentic.{claude_code,github}`, `budget`).
  `Resolve()` = baseline ⊕ overrides.
- **Reconcile** (`reconcile.go`): for each `(target, control)` apply via `vendor_native` (call adapter
  `SetX`) or `env_managed` (render + distribute artifact); record a `Signal`; produce a **coverage
  report** (native vs env-managed vs proxy-needed vs unverified per control).
- **Drift** (`drift.go`): re-read actual from both surfaces; any divergence → drift observation + `/api/drift` row.

---

## 9. EnvConfig (`internal/envconfig`)

- `render.go`: policy intent → `ManagedArtifact` per platform. Concrete byte-faithful artifacts:
  - **Claude Code** `managed-settings.json`: `allowedMcpServers`, `deniedMcpServers`,
    `allowManagedMcpServersOnly`, `allowManagedHooksOnly`, `allowManagedPermissionRulesOnly`,
    `disableSkillShellExecution` (enforcement `mdm_locked`).
  - **GitHub** branch-protection ruleset: required reviewers + required status checks (enforcement `server_side`).
  - **VS Code** enterprise policy: `ChatMCP`, `ChatAgentMode`, `AllowedExtensions` (enforcement `mdm_locked`).
  - **Cursor** admin policy lock (enforcement `seed_only` unless MDM).
- `distribute.go` (synthetic): record the push intent + target channel; no real network.
- `readback.go` (synthetic): return effective `EnvState` with `source ∈ {locked,user_override,default}`;
  occasionally synthesize a Tier-C override to exercise drift.

---

## 10. HTTP API (`internal/server`) — consumed by Phase 2

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/health` | `{ok, service, adapter_count, ts}` |
| GET | `/api/adapters` | list adapters (+ capabilities, status, mode) |
| GET | `/api/adapters/{id}` | one adapter |
| PATCH | `/api/adapters/{id}` | set mode/scenario/enabled/emit |
| GET | `/api/adapters/{id}/manifest` | capability manifest (dispositions + enforcement) |
| POST | `/api/adapters/{id}/test` | connectivity test → status |
| GET | `/api/adapters/{id}/calls` | recorded synthetic calls (redacted) |
| GET | `/api/baselines` | the 4 baseline profiles |
| GET/PUT | `/api/policies` | read / apply policy-as-code (PUT runs reconcile, returns coverage report) |
| POST | `/api/credentials` | write credential by `secret_ref` (plaintext rejected) |
| GET | `/api/observations` | latest `ops-observation-batch/v1` batches (limit 200) |
| POST | `/api/observations` | ingest external batch (contract-checked) → 202 |
| GET | `/api/audit` | normalized audit stream (+ `?format=siem`) |
| GET | `/api/drift` | drift observations (intent vs actual) |
| GET/POST | `/api/envconfig` | rendered managed artifacts / read-back env state |
| ANY | `/synthetic/{id}/...` | byte-identical vendor surface + `_harness/*` control |
| — | `/`, `/settings/*` | SPA fallback (Phase 2; `web/dist` if present) |

Response envelopes match it-scorecard (`{adapters:[...]}`, `{observations:[...]}`, etc.).

---

## 11. Environment / config

| Var | Default | Effect |
|---|---|---|
| `AIRTRAFFIC_ADDR` | `127.0.0.1:8122` | listen address |
| `AIRTRAFFIC_EMIT` | `on` | background emitter on/off |
| `AIRTRAFFIC_EMIT_INTERVAL_SECONDS` | `5` | emit cadence |
| `AIRTRAFFIC_GATEWAY` | `off` | optional gateway (out of Phase 1 scope; mount no-op) |

`.devlauncher.json` registers `HARNESS`-style launch on `8122` (`AIRTRAFFIC_ADDR=127.0.0.1:{port} go run ./cmd/air-traffic-server`).

---

## 12. Testing strategy (must be green before /shipit)

1. `gofmt -l` clean · `go vet ./...` clean · `go build ./...` clean.
2. **Unit tests** (`*_test.go`, `httptest`):
   - `store_test.go` — seed counts, patch apply, ring-buffer eviction, plaintext-secret rejection.
   - `emitter_test.go` — one batch per emitting adapter; batch validates against `ops-observation-batch-v1.schema.json`; walk stays within `[min,max]`.
   - `adapter_test.go` — every adapter satisfies `VendorAdapter`; manifest dispositions are valid enum values; healthcare baseline excludes non-`baa_signed`.
   - `synthetic_test.go` — **fidelity tests**: each Tier-1 vendor success body has the exact top-level keys/envelope; each scenario emits that vendor's error shape; `_harness/manifest` shape.
   - `routes_test.go` — every `/api/*` route returns expected envelope + status; PUT `/api/policies` returns a coverage report; `/api/drift` reflects an injected override.
   - `policy_test.go` — baseline ⊕ override resolution; reconcile coverage classification.
3. **Live smoke** (scripted): boot server on an ephemeral port, `curl` `/api/health`, `/api/adapters`,
   a Tier-1 synthetic success + each error scenario, `/api/observations` (non-empty after one tick),
   `/api/audit`, `/api/drift`, PUT a baseline and re-read coverage. Capture outputs to the test log.

---

## 13. Acceptance criteria

- [ ] `go build ./...` and `go test ./...` green; `go vet` clean; zero external deps in `go.mod`.
- [ ] ≥16 vendor adapters registered and listed by `/api/adapters`, each with a capability manifest carrying valid dispositions (+ enforcement tier for every `env_managed` cap).
- [ ] All 6 Tier-1 vendors serve byte-identical synthetic success bodies and vendor-shaped error envelopes for all scenarios, with calls recorded + redacted.
- [ ] Emitter produces schema-valid `ops-observation-batch/v1` batches every tick; `/api/observations` non-empty; drift surfaces on injected override.
- [ ] 4 baselines apply via PUT `/api/policies` and return a coverage report; `/api/drift` and `/api/audit` populated.
- [ ] EnvConfig renders a real-shaped Claude Code `managed-settings.json` and a GitHub branch-protection ruleset with correct enforcement tiers.
- [ ] Live smoke script passes end-to-end.

## 14. Explicitly out of scope (Phase 1)

- Any frontend (Phase 2).
- The optional inference gateway (`internal/gateway`, system-design §11) — `proxy` mode stays stubbed.
- Real vendor network calls / real credentials / Postgres / real IdP-KMS (hardening phase).
- Changes to any repo other than `air-traffic`.
