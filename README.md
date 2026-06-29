# air-traffic

**Enterprise AI Control Plane** — a unified control + observability *spine* across every
major AI vendor. Air-Traffic drives vendors' native admin APIs (`vendor_native`), pushes
managed config into dev/agent environments and reads it back for drift (`env_managed`), and
emits a normalized `ops-observation-batch/v1` signal across the developer-workflow, data-policy,
and budget control planes. It is **not** an inline inference proxy (an optional gateway is the
only thing that ever sits on the request path, and it is out of scope here).

Sibling app: [`it-scorecard`](../it-scorecard) — Air-Traffic mirrors its Go + React,
zero-external-deps, synthetic/proxy/disabled pattern.

## Phase 1 (this) — synthetic byte-identical backend

Go (stdlib only, zero deps). Serves a **byte-identical synthetic replica** of each vendor's
admin/control surface plus the control-plane API and the background emitter.

```bash
# run the API + synthetic surfaces + emitter (port 8122)
go run ./cmd/air-traffic-server

# tests
go test ./...
```

Env: `AIRTRAFFIC_ADDR` (default `127.0.0.1:8122`), `AIRTRAFFIC_EMIT` (`on`),
`AIRTRAFFIC_EMIT_INTERVAL_SECONDS` (`5`).

### Vendor coverage (16 adapters)

| Tier | Fidelity | Vendors |
|---|---|---|
| 1 | deep, multi-endpoint | OpenAI · Anthropic · AWS Bedrock · Azure OpenAI · Google Vertex · GitHub Copilot |
| 2 | core surfaces | M365 Copilot · Mistral · Databricks · Perplexity · Cohere · Together |
| 3 | manifest + emit | Groq · xAI · Amazon Q · IBM watsonx |

Each capability carries one of five **dispositions** — `vendor_native` · `env_managed` ·
`proxy_enforced` · `monitor_only` · `unverified` — and every `env_managed` capability also
carries an enforcement tier (`server_side` / `mdm_locked` / `seed_only`). The UI never renders
a seed-only control as "enforced."

### HTTP API

| Path | Purpose |
|---|---|
| `GET /api/health` | liveness + adapter count |
| `GET /api/adapters`, `GET/PATCH /api/adapters/{id}` | list / read / set mode·scenario·emit |
| `GET /api/adapters/{id}/manifest` · `/calls` · `POST /test` | capability manifest, recorded calls, connectivity test |
| `GET /api/baselines` | the 4 rigor profiles (General SaaS / Fintech / Healthcare / Gov) |
| `GET/PUT /api/policies` | read / apply policy-as-code (PUT returns a coverage report) |
| `GET/POST /api/observations` | latest `ops-observation-batch/v1` batches / ingest |
| `GET /api/audit` (`?format=siem`) | normalized cross-vendor audit stream |
| `GET /api/drift` | intent-vs-actual divergence |
| `GET /api/envconfig` | rendered managed artifacts + read-back env state |
| `POST /api/credentials` | write credential by `secret_ref` (plaintext rejected) |
| `ANY /synthetic/{vendor}/{native-path}` | **byte-identical** vendor surface + `/_harness/*` control |

### Synthetic surface example

```bash
curl :8122/synthetic/openai/admin/organization/users      # OpenAI {"object":"list","data":[…]}
curl :8122/synthetic/anthropic/v1/organizations/workspaces # Anthropic {"data":[…],"has_more":…}
curl :8122/synthetic/bedrock/guardrails                    # AWS {"guardrails":[…],"nextToken":…}
# inject a scenario; each vendor returns its OWN error envelope
curl -X PUT :8122/synthetic/openai/_harness/scenario/429-retry-after
```

## Architecture

```
cmd/air-traffic-server   boot + lifecycle
internal/catalog         the surface collection — per-vendor capabilities + metric defs
internal/synthetic       byte-identical /synthetic/{vendor}/… surfaces + vendor error envelopes
internal/emitter         background ops-observation-batch/v1 generator
internal/policy          baselines · reconcile/coverage · drift
internal/envconfig       render managed-settings.json / branch protection / VS Code / Cursor
internal/audit           normalized stream + SIEM export
internal/store           in-memory state (RWMutex, FIFO ring buffers)
internal/server          HTTP API + SPA fallback (Phase 2)
internal/model           domain types + ops-observation-batch/v1 contract
```

Plans: [`docs/plans/phase-1-surface-collection.md`](docs/plans/phase-1-surface-collection.md) ·
[`docs/plans/phase-2-frontend.md`](docs/plans/phase-2-frontend.md).

## Phase 2 (next) — control & observability SPA

Vite + React + TS + Tailwind on port 5202, served same-origin from `web/dist`. Flight Deck
observability landing + Rigor Console + Policy Editor + Cost Explorer. See the Phase 2 plan.
