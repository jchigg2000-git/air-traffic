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
| `GET /api/gateway/requests` | per-request proxy traffic, newest first — backs the **Gateway Traffic** screen. Metadata only (types, field paths, offsets, counts, tokens); never a value, never a dollar figure |
| `GET/POST /api/harness/runs` · `GET /runs/{id}` · `POST /sample` · `GET /ratchet` · `/corpus` · `GET/POST /proposals[/{id}/approve\|reject]` | flywheel harness — **503s without the harness engine** (`requireHarness`). `POST /proposals` authors an owner proposal (the flywheel infers additions from harness misses; it has no ground truth for real traffic, so retiring a false positive is a human call) |
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
internal/gateway         proxy · adapter_anthropic · adapter_openai · stream · spine_emit
                         · spine_pull · metrics · audit
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

A separate, stateless Go binary that proxies inference traffic in two client dialects —
`POST /v1/messages` (Anthropic Messages) and `POST /v1/chat/completions` (OpenAI-compatible,
which is what the Hugging Face router and most "OpenAI-compatible" endpoints speak) — detects
PII/PHI inline (RE2 regex floor + self-hosted [Presidio](https://microsoft.github.io/presidio/)
sidecar), applies `mask`/`block`/detect-only per the pulled policy baseline, and reports back
up the spine (observations, leak metadata, enforcement heartbeats → `proxy_enforced` flips
truthfully, staleness raises drift). Both routes run the same pipeline behind a `dialect`
descriptor, so a policy or detector change lands on both at once. Request-side only: responses
are relayed byte-faithfully (response-side enforcement is G4, deferred).
Design: `docs/inference-gateway-design.md`; sequencing:
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
both client routes, which the control plane's harness presents as
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
NER, no cloud inference or compute.

Rules come in four kinds. Three of them **add** enforcement (`regex`, `deny_list`) or gate it by
score (`threshold`); `allow_list` is the only one that **removes** a detection, and it exists
because a score gate cannot tell a false positive from a real hit — Presidio's spaCy layer returns
0.85 for every PERSON and LOCATION it emits either way. Suppression is applied in the detector
chain beside the type guards, so it overrules whichever engine made the claim, and it suppresses a
*term*, never a region: an allow-listed word sitting next to real PII does not shield it. The
flywheel cannot propose one itself — it has no ground truth for real traffic, so retiring a false
positive is an owner judgement, authored via `POST /api/harness/proposals` and then approved like
any other.

The **Gateway Traffic** tab (`/settings/traffic`) is the other half: not seeded harness runs but
whatever real traffic actually went through the proxy, newest first — route, model, action,
redaction types, tokens, upstream status, and how much latency the gateway itself added. Tokens
are what the vendor reported; there is no cost column, deliberately.

**Pointing real clients at it.** Claude Code speaks the Anthropic route as-is:

```bash
GATEWAY_UPSTREAM_BASE_URL=https://api.anthropic.com ANTHROPIC_UPSTREAM_KEY=<real key> \
  docker compose up -d --build gateway
ANTHROPIC_BASE_URL=http://127.0.0.1:8125 ANTHROPIC_AUTH_TOKEN=<gateway key> \
CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1 claude
```

`POST /v1/messages/count_tokens` and `HEAD /api/hello` are unrouted and will 404 — both are
documented as optional, and Claude Code falls back cleanly. Anything speaking the OpenAI wire
format (the `openai` SDK, `@huggingface/inference` with `endpointUrl`, LangChain) uses the other
route: set the client's base URL to `http://127.0.0.1:8125/v1`, pass a `GATEWAY_CLIENT_KEYS` entry
as the bearer token, and set `HF_UPSTREAM_TOKEN` (or point `OPENAI_UPSTREAM_BASE_URL` elsewhere and
supply that vendor's key). That upstream credential has **no compose fallback on purpose** — unset,
the route returns 502 locally rather than sending a placeholder to a real vendor.
