# Cost & Usage drill-down — status & follow-ups

Shipped: per-vendor cost drill-down (cost by user / model / repo / project / workspace /
api_key / sku / service_tier / region / deployment / team / language) across the whole app.

## Architecture (single source of truth)

`internal/catalog/cost_facets.go` — `costFacetsByID` declares, per vendor, every assessed
drill-down dimension: supported ones carry the real group-by param + endpoint + the exact
JSON `response_field` + canonical members; unsupported ones carry an honest `Reason`. This is
the **config object** (extends `catalog.Definition.CostFacets`). Both surfaces below read it,
so they can never disagree:

- **Emitter** (`internal/emitter/emitter.go` `costBreakdowns`): emits one
  `kind:"cost_breakdown"` observation per supported facet member into the
  `ops-observation-batch/v1` stream (shares normalized to the aggregate `cost_usd`). Drives
  the Cost Explorer drill-in for **all 16 vendors**.
- **Synthetic** (`internal/synthetic/fixtures_cost.go` + `cost_grouping.go`): byte-identical
  grouped vendor-API responses.

Decision (asked of me): kept the **data-driven** model — NOT per-vendor emitter/config Go
objects — to stay faithful to the it-scorecard sibling (no Connector interface). The
emitter-vs-config split is realized as one emitter + one catalog map, not N objects.

## Byte-identical synthetic grouping — DONE

- **OpenAI** — `/usage` (group_by project_id/model/user_id/api_key_id/service_tier) and
  `/costs` (group_by project_id/api_key_id/line_item). Param-driven.
- **Anthropic** — `usage_report` (model/workspace_id/api_key_id/service_tier) and
  `cost_report` (workspace_id/description). Param-driven.
- **GitHub Copilot** — inherent multi-row: `billing/usage` (one usageItems[] per repo/sku),
  `metrics` (languages[] + editors[].models[]), seats (one per assignee).

Tests: `internal/synthetic/fixtures_cost_test.go` (envelope preserved, rows fan out, grouped
field populated, non-grouped keys null, ungrouped path unchanged).

## Byte-identical synthetic grouping — TODO (emitter drill-in already works for these)

The drill-down already works in the Cost Explorer for every vendor below via the emitter; what
remains is serving the *byte-identical vendor-API replica* of the grouped cost endpoint:

- **Azure OpenAI** — add a `Microsoft.CostManagement/query` case
  (`{properties:{columns:[],rows:[]}}`) for cost dims + token grouping via the metrics
  endpoint (ModelName/ModelDeploymentName/Region dimensions).
- **AWS Bedrock** — add Cost Explorer `GetCostAndUsage`
  (`{ResultsByTime:[{Groups:[{Keys,Metrics}]}]}`) + CloudWatch `GetMetricData`
  (`{MetricDataResults:[{Label,Values}]}`, ModelId).
- **Vertex** — extend `timeSeries` to multiple series by label
  (`resource.labels.model_user_id` / `resource_container` / `location` / `endpoint_id`); add a
  BigQuery-billing-export rows envelope for the SKU + team (label) cost dims.
- **Mistral** — workspace is a *filter*, not a group_by; align the fixture to the real
  `/v1/admin/usage` service-category envelope (currently object:list/data wrapper).
- **Tier-2/3** (m365_copilot, databricks, amazon_q, watsonx) — register fixtures mirroring
  their real billing shapes (Graph `value[]`, `system.billing.usage` rows, Cost Explorer,
  IBM Cloud Usage Reports). perplexity/cohere/groq/xai expose no real server-side group-by
  (empty supported[] — honest).

Research backing all of the above: the 24-agent study digested in the session that built this
(real group-by params, endpoints, response fields, verify-corrected). Config-surface stubs for
these are intentionally deferred (per the original request).
