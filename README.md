# air-traffic

**Enterprise AI Control Plane** — a unified control + observability *spine* across every
major AI vendor. Air-Traffic drives vendors' native admin APIs (`vendor_native`), pushes
managed config into dev/agent environments and reads it back for drift (`env_managed`), and
emits a normalized `ops-observation-batch/v1` signal across the developer-workflow, data-policy,
and budget control planes. It is **not** an inline inference proxy — the one thing that ever
sits on the request path is the **optional inference gateway**, a separate data-plane binary
(`cmd/air-traffic-gateway`, Phase 3 below) that redacts PII/PHI inline and is what makes the
`proxy_enforced` disposition true.

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

CI (`.github/workflows/ci.yml`) runs on every push and PR: Go gofmt + vet + build +
`test -race`, a stdlib-only guard (the build fails if a `go.sum` ever appears), the
redaction log-leak guard as its own job, the web typecheck/build/vitest suite, and both
Docker image targets.

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
| `GET /api/activity` | recent cross-plane activity feed |
| `GET /api/drift` | intent-vs-actual divergence |
| `GET /api/envconfig` | rendered managed artifacts + read-back env state |
| `POST /api/credentials` | write credential by `secret_ref` (plaintext rejected) |
| `GET /api/cost/facets` | per-vendor cost drill-down facets — backs the **Cost & Usage Explorer** screen |
| `POST /api/gateway/leaks` · `/enforcement` | gateway pushes per-request redaction metadata + enforcement heartbeats up the spine |
| `GET /api/gateway/patterns` · `/status` | active pattern pack (the gateway pulls it) · gateway liveness for the UI |
| `GET/POST /api/harness/runs` · `GET /runs/{id}` · `POST /sample` · `GET /ratchet` · `/corpus` · `GET/POST /proposals[/{id}/approve\|reject]` | flywheel harness — **503s without the harness engine** (`requireHarness`) |
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
  └ cost_facets.go       per-vendor cost drill-down facet defs (backs the Cost & Usage Explorer)
internal/synthetic       byte-identical /synthetic/{vendor}/… surfaces + vendor error envelopes
internal/emitter         background ops-observation-batch/v1 generator
internal/policy          baselines · reconcile/coverage · drift
internal/envconfig       render managed-settings.json / branch protection / VS Code / Cursor
internal/audit           normalized stream + SIEM export
internal/store           in-memory state (RWMutex, FIFO ring buffers)
internal/server          HTTP API + SPA fallback (Phase 2)
internal/model           domain types + ops-observation-batch/v1 contract

# Phase 3 — inference gateway (data plane) + flywheel harness
cmd/air-traffic-gateway  the inline redaction-proxy binary (port 8125)
internal/gateway         proxy · adapter_anthropic · spine_emit · spine_pull · metrics · audit
                         + config · credbroker · detect · redact
internal/harness         gen · runner · streamscan · persist · sample · probe · flywheel · score
```

Plans: [`docs/plans/phase-1-surface-collection.md`](docs/plans/phase-1-surface-collection.md) ·
[`docs/plans/phase-2-frontend.md`](docs/plans/phase-2-frontend.md).

## Phase 2 — control & observability SPA

Vite + React + TS + Tailwind on port 5202, served same-origin from `web/dist`. Dark-first
control-tower UI; disposition is a first-class visual token (seed-only is never shown as
"enforced").

```bash
cd web
npm install
npm run dev          # Vite dev server on :5202, proxies /api + /synthetic → :8122
npm run build        # → web/dist (the Go binary auto-serves it when present)
npm test             # Vitest unit tests (vitest run — e.g. src/pages/Welcome.test.tsx)
```

Screens: **Flight Deck** (observability landing — live vendor status board, KPI strip, plane
bands, drift, freshness) · **Rigor Console** (baseline + coverage marks) · **Policy Editor**
(per-vendor control cards with truthful disposition/enforcement chips) · **Cost & Usage
Explorer** (spend by vendor, velocity, cap alerts, CSV/JSON export of the current fleet rollup +
drill-down) · **Vendors** (mode/scenario/emitter + manifest + recorded calls) ·
**Observability** (raw batches) · **Audit** (normalized stream + SIEM export).

**Routing** (`web/src/App.tsx`): `/` renders the **Flight Deck** (the app); `/welcome` is a
public marketing **Welcome** page assembled from `pages/landing/*` (Hero · Planes · VendorWall ·
Dispositions · HowItWorks · CtaBand); the consoles above live under `/settings/*`.

To serve the SPA from the Go binary: `cd web && npm run build`, then run the server from the
repo root and open <http://127.0.0.1:8122/>.

## Phase 3 — inference gateway (data plane) + harness + flywheel v0

A separate, stateless Go binary that proxies Anthropic Messages traffic, detects PII/PHI
inline (RE2 regex floor + self-hosted [Presidio](https://microsoft.github.io/presidio/)
sidecar), applies `mask`/`block`/detect-only per the pulled policy baseline, and reports back
up the spine (observations, leak metadata, enforcement heartbeats → `proxy_enforced` flips
truthfully, staleness raises drift). Design: `docs/inference-gateway-design.md`; sequencing:
`docs/inference-gateway-build-plan.md`; what shipped vs deferred:
`docs/plans/phase-3-inference-gateway.md` + `docs/plans/TODO-gateway-deferred.md`.

**Run the whole stack (dockerized):**

```bash
./scripts/dev-env.sh              # optional: mint real keys into .env (see Keys below)
docker compose up -d --build      # control plane :8122 (SPA baked in) + gateway :8125 + Presidio :8126
open http://127.0.0.1:8122/settings/harness
E2E_COMPOSE=1 ./scripts/e2e-gateway.sh   # assert the running stack end-to-end
```

**Keys.** Two shared secrets hold the stack together: `GATEWAY_CLIENT_KEYS` (the caller key for
the gateway's `/v1/messages`, which the control plane's harness presents as
`AIRTRAFFIC_GATEWAY_KEY`) and `AIRTRAFFIC_SPINE_KEY` (required on `/api/gateway/leaks`,
`/enforcement`, and `/patterns` — the pattern pack distributes deny-list *terms*, so the read
side is gated too). Compose falls back to throwaway values (`gwk-demo`, `spine-dev-insecure`)
so the demo comes up in one command; both binaries log a warning while those are live and
`/api/gateway/status` reports `spine_key_unrotated`. Run `./scripts/dev-env.sh` (add `--rotate`
to replace existing keys) before anything shared. With no spine key set at all, those three
routes accept **loopback callers only** — a container-network peer does not qualify.

Images bake built source — after a code change, `docker compose up -d --build <service>`
(a bare `restart` won't pick it up). Harness state (ratchet series, promoted corpus, pattern
pack) persists in the `harness-data` volume. The compose stack and the bare `go run` dev
flow use the same ports — run one or the other.

**Bare-process dev flow** (fast iteration, same behavior):

```bash
docker compose -f deploy/presidio/docker-compose.yml up -d   # NER tier only (port 8126)

GATEWAY_UPSTREAMS='{"anthropic":{"base_url":"http://127.0.0.1:8122/synthetic/anthropic","credential_ref":"env:ANTHROPIC_UPSTREAM_KEY"}}' \
ANTHROPIC_UPSTREAM_KEY=sk-ant-synthetic-dev GATEWAY_CLIENT_KEYS=gwk-demo \
GATEWAY_DETECTORS=regex,presidio GATEWAY_REDACT_ACTION=per_policy \
go run ./cmd/air-traffic-gateway                              # data plane (port 8125)

./scripts/e2e-gateway.sh                                      # boots its own processes
```

The **Gateway Harness** tab (`/settings/harness`) drives seeded synthetic PII through the
gateway, proves redaction *behaviorally* (did the raw value reach the upstream capture?),
scores precision/recall against exact ground truth, and feeds the recall-ratchet flywheel:
misses → promoted corpus → curated pattern proposals → human approval → hot-reload (no
restart) → re-run → the ratchet climbs. Everything is local — synthetic traffic, self-hosted
NER, no cloud inference or compute. Point real Claude Code at it with
`ANTHROPIC_BASE_URL=http://127.0.0.1:8125` + `ANTHROPIC_AUTH_TOKEN=<gateway key>`.
