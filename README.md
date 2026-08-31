# air-traffic

**Enterprise AI Control Plane** — a unified control + observability *spine* across every
major AI vendor.

> **Read this first: no vendor is really contacted.** Air-Traffic talks to a **synthetic
> replica** of each vendor's admin/control surface that it serves itself, on its own port.
> Not one of the sixteen adapters holds a live credential or reaches
> `api.openai.com`, `api.anthropic.com`, or anything else on the internet. Every number, user
> list, guardrail and dollar figure you see is generated locally. What is real is the shape:
> the API surfaces, the error envelopes, the policy engine, the emitted signal contract — and
> the **inference gateway**, whose redaction runs on real request bytes and is measured
> behaviourally rather than asserted. Treat this as a working spec and a testbed for the
> integration, not as a production integration.

Air-Traffic's model is that it drives vendors' native admin APIs (`vendor_native`), pushes
managed config into dev/agent environments and reads it back for drift (`env_managed`), and
emits a normalized `ops-observation-batch/v1` signal across the developer-workflow, data-policy,
and budget control planes. It is **not** an inline inference proxy — the one thing that ever
sits on the request path is the **optional inference gateway**, a separate data-plane binary
(`cmd/air-traffic-gateway`, Phase 3 below) that redacts PII/PHI inline and is what makes the
`proxy_enforced` disposition true.

## Phase 1 (this) — synthetic byte-identical backend

Go (stdlib only, zero deps). Serves a synthetic replica of each vendor's admin/control surface
plus the control-plane API and the background emitter.

**What "byte-identical" means here, and what it doesn't.** Seven vendors have a hand-written
fixture shaped to that vendor's documented response — same envelope, same field names, same
pagination keys — so a client written against the real API parses it unchanged. It is *not*
verified against a captured live response; nothing in this repo diffs the two, because that
would need credentials the project deliberately does not hold. The other nine answer from a
generic envelope that is correctly shaped and labelled as such (see the table below).

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

Tier is the *research* depth of the capability manifest; **fidelity** is what the synthetic
replica actually returns, and the two are not the same thing. Seven vendors have a dedicated
byte-identical fixture; the rest answer from a generic envelope
(`internal/synthetic/errors.go` `genericFixture`) that is correctly shaped and honestly labelled
rather than vendor-exact. That gap is stated here rather than rounded up — the manifests, error
envelopes, cost facets and emitted signal are real for all sixteen.

| Tier | Manifest depth | Replica fidelity | Vendors |
|---|---|---|---|
| 1 | deep, multi-endpoint | **dedicated fixture** | OpenAI · Anthropic · AWS Bedrock · Azure OpenAI · Google Vertex · GitHub Copilot |
| 2 | core surfaces | **dedicated fixture** | Mistral |
| 2 | core surfaces | generic envelope | M365 Copilot · Databricks · Perplexity · Cohere · Together |
| 3 | manifest + emit | generic envelope | Groq · xAI · Amazon Q · IBM watsonx |

Per-vendor error envelopes are real for **all sixteen** regardless of fixture depth — that is the
part `/_harness/scenario/*` exercises, and it is the part an integration actually breaks on.

**Six adapters are enabled at boot** — the Tier-1 row (`defaultRoster`, `internal/store/store.go`).
The other ten, Mistral and its dedicated fixture included, ship **disabled** until their
per-vendor auth config exists (`docs/plans/TODO-vendor-auth.md`), and answer every synthetic call
with a vendor-shaped `503`. Turn one on with
`PATCH /api/adapters/{id}  {"enabled": true}`, or from the Vendors screen.

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
| `ANY /synthetic/{vendor}/{native-path}` | synthetic vendor surface (dedicated fixture for 7, generic envelope for 9) + `/_harness/*` control. The two mutating control paths — `_harness/scenario` and `_harness/reset` — write the adapter record and carry the operator key |

### Synthetic surface example

```bash
curl 127.0.0.1:8122/synthetic/openai/admin/organization/users       # OpenAI {"object":"list","data":[…]}
curl 127.0.0.1:8122/synthetic/anthropic/v1/organizations/workspaces # Anthropic {"data":[…],"has_more":…}
curl 127.0.0.1:8122/synthetic/bedrock/guardrails                    # AWS {"guardrails":[…],"nextToken":…}
# inject a scenario; each vendor returns its OWN error envelope
curl -X PUT 127.0.0.1:8122/synthetic/openai/_harness/scenario/429-retry-after \
  -H "X-Air-Traffic-Admin-Key: $AIRTRAFFIC_ADMIN_KEY"   # only needed once a key is set
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
  └ *_persist.go         the two exceptions, written through to disk: keystore + applied policy
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

**Applying a rigor baseline states its consequence first.** The Rigor Console's profiles map to a
gateway action — `detect` (monitor), `mask`, or `block` — and Healthcare is a *total block until ZDR
coverage is attested*, by design (design §15's pre-coverage gate). Apply therefore previews the
derived action, names the blast radius, and carries the attestation itself; the derivation is
computed once in `internal/model/gateway_action.go` and shared with the gateway, so the preview and
the enforcement cannot disagree.

![The Rigor Console's Apply confirmation: resulting gateway action BLOCK, the ZDR attestation
checkbox, and a warning that every request carrying a detected value will be
refused](docs/images/rigor-console-apply-preview.png)

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

**Operator key (`AIRTRAFFIC_ADMIN_KEY`).** The control plane is single-operator by decision — no
user model, no login, no per-human principal, so an audit row can name the system but never a
person (`DECISIONS.md` 2026-08-15). What it does have is one key gating every **state-changing**
route: adapter patch, policy PUT, credential POST, harness run/sample, proposal approve/reject,
and the two mutating synthetic-harness control paths (`_harness/scenario`, `_harness/reset` —
they write the same adapter record). Reads are never gated — the observability surfaces are the
product.

```bash
./scripts/dev-env.sh              # mints AIRTRAFFIC_ADMIN_KEY (adm-…) into .env
grep AIRTRAFFIC_ADMIN_KEY .env    # paste it into the SPA sidebar's "Operator key" field
curl -X PUT 127.0.0.1:8122/api/policies -H "X-Air-Traffic-Admin-Key: $KEY" -d '{"baseline":"fintech"}'
```

**Unset, the writes are open** — that is the posture this repo shipped with, and it is what the
one-command compose demo runs in, because the SPA is served from the control-plane container
behind a published port and a browser arrives over the Docker bridge rather than loopback.
Rather than imply otherwise, the boot logs warn on every start and `GET /api/health` +
`/api/gateway/status` report `"admin_auth": "open"` until a key is set. The key is also an
alternative to loopback for the keystore admin API below.

**Keystore (issued keys, per-app policy).** `GATEWAY_CLIENT_KEYS` authenticates but says nothing
about *who* is calling, and it gives every caller one posture. The keystore adds apps and keys
issued against them, so a request carries a principal and an app can run its own baseline:

```bash
./scripts/keystore.sh add-app hf-sandbox            # optional 2nd arg: a baseline id
./scripts/keystore.sh issue   hf-sandbox user-42    # optional: <route> <expires-in-days>
#   → prints the key ONCE; only a SHA-256 digest is stored
./scripts/keystore.sh set-baseline hf-sandbox fintech   # "" to go back to the global policy
./scripts/keystore.sh revoke  <key-id>
```

One app may hold many keys, tagged by `subject` (a user id, agent instance, CI job) and optionally
scoped to one route and an expiry. Requests then show up on the Gateway Traffic tab attributed to
their app, and an app carrying a baseline is served under *that* posture while everyone else stays
on the applied policy — the gateway's heartbeat stops claiming enforcement if any app is scoped to
monitor-only, so per-app scoping cannot quietly overstate coverage.

Two things to know about the trust boundary. The admin API takes **a loopback caller or
`AIRTRAFFIC_ADMIN_KEY`, and deliberately not the spine key** — the gateway holds that key, and a
gateway that can mint its own credentials makes the keystore pointless. With no admin key set the
posture is loopback-only, and since compose publishes the control plane behind a port, a browser
(or a plain host `curl`) reaches it over the Docker bridge and gets a 401; that is why
`scripts/keystore.sh` routes the call through the container's netns, and why there is still no
keystore UI. And revocation is
**eventual**: gateways verify against a pulled snapshot, so a revoked key stops working within one
`GATEWAY_POLICY_PULL_INTERVAL` (15s by default), not instantly. `GATEWAY_CLIENT_KEYS` keeps working
throughout and is still required at boot; keystore-issued keys are additive, and legacy callers
report as app `env`.

**What survives a restart, and what deliberately doesn't.** Three things persist to
`AIRTRAFFIC_DATA_DIR` (the `harness-data` volume): the keystore (`keys.json` — issued credentials
are not reconstructible), the **applied policy** (`policy.json` — the gateway is already enforcing
it, so forgetting it here would leave the two halves disagreeing while traffic flows), and the
**harness flywheel state** (`ratchet.jsonl`, `corpus/*.json`, `patterns.json` — a ratchet that
resets is not a ratchet, and the promoted corpus is the accumulated regression set). The first two
are whole-file atomic JSON writes. Everything else — observations, gateway request reports, drift,
audit — is in-memory ring buffers by decision, because a durable time series is what the rejected
third-party-dependency fork would have bought (`DECISIONS.md` 2026-08-15). A policy write that
fails is reported on `/api/gateway/status` as `policy_error` rather than swallowed; a corrupt
`policy.json` warns and boots with none applied instead of refusing to start.

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

![The Gateway Harness readout: try-a-prompt redaction diff, run configuration, behavioral score
card, recall ratchet series, flywheel pattern proposals, per-request misses and the promoted
corpus](docs/images/gateway-harness-readout.png)

*One 200-request run: **100.0% behavioral recall** (99.0% *reported* recall — the gap between the
two is the honesty check), 97.7% precision, 0 trap false positives. Read
it top to bottom — a live prompt masked mid-flight (`Jane Doe` → `[PERSON_NAME]`, SSN → `[SSN]`)
with what the upstream actually received beside what was sent; the per-type TP/FN/FP table; the
ratchet's run-over-run history; and the flywheel's pattern proposals, which are* human-approved
only — nothing auto-applies. *An annotated walkthrough of the same screen is at
[`docs/images/gateway-harness-readout-annotated.png`](docs/images/gateway-harness-readout-annotated.png),
and the concepts behind it are explained without jargon in
[`docs/inference-gateway-eli5.md`](docs/inference-gateway-eli5.md).*

**The 60-second path to seeing redaction work** (no Docker, no third-party image). The Presidio
sidecar is the NER tier, not the whole detector — the regex floor runs standalone and is the
default:

```bash
# terminal 1 — control plane (also serves the SPA if web/dist exists)
go run ./cmd/air-traffic-server

# terminal 2 — gateway, regex only, pointed at the synthetic Anthropic replica
GATEWAY_UPSTREAMS='{"anthropic":{"base_url":"http://127.0.0.1:8122/synthetic/anthropic","credential_ref":"env:ANTHROPIC_UPSTREAM_KEY"}}' \
ANTHROPIC_UPSTREAM_KEY=sk-ant-synthetic-dev GATEWAY_CLIENT_KEYS=gwk-demo \
GATEWAY_DETECTORS=regex GATEWAY_REDACT_ACTION=mask \
go run ./cmd/air-traffic-gateway

# terminal 3 — send a prompt carrying two planted values
curl -s 127.0.0.1:8125/v1/messages -H 'x-api-key: gwk-demo' -H 'content-type: application/json' \
  -d '{"model":"claude-3-5-sonnet","max_tokens":64,"messages":[{"role":"user","content":"Wire it to SSN 123-45-6789, callback 555-123-4567"}]}'

# ...then read what the upstream ACTUALLY received. This is the line that proves it:
curl -s 127.0.0.1:8122/synthetic/anthropic/_harness/inference
```

The first call returns the mock upstream's reply and tells you nothing — the redaction happened
on the way out, so the response looks the same either way. The second is the evidence:
`"body"` is the request bytes as the upstream saw them, and it reads
`Wire it to SSN [SSN], callback [PHONE]`. Proving redaction by inspecting what the far side
received, rather than trusting the proxy's own report of itself, is the same behavioural standard
the harness scores against.

What you give up by skipping Presidio, stated plainly: the regex tier recognizes exactly eight
**structured** types — `SSN`, `CREDIT_CARD`, `IBAN`, `EMAIL`, `PHONE`, `IP`, `DOB`, `MRN`
(`internal/gateway/detect/regex.go`). Two carry a real checksum before a hit counts (Luhn on
`CREDIT_CARD`, ISO 13616 mod-97 on `IBAN`); three more are validated past the raw match (octet
range on `IP`, a hyphen-adjacency guard on `SSN` and `PHONE`) — and
recognizes **nothing** unstructured. `PERSON_NAME` and `ADDRESS` are the NER tier's job and go
undetected here. Expect recall well below the numbers above, which are measured with
`GATEWAY_DETECTORS=regex,presidio`. Use this path to see the mechanism; use compose to measure it.

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
whatever real traffic actually went through the proxy, newest first — app, route, model, action,
redaction types, tokens, upstream status, and how much latency the gateway itself added. The App
column comes from the keystore (hover it for the key id, subject and baseline); requests
authenticated by `GATEWAY_CLIENT_KEYS` show as `env`. Tokens are what the vendor reported; there is
no cost column, deliberately.

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

## Contributing, security, license

Pull requests are welcome — [`CONTRIBUTING.md`](CONTRIBUTING.md) has the check suite and the
handful of deliberate constraints that will otherwise look like bugs. Before running this
anywhere but your own machine, read [`SECURITY.md`](SECURITY.md): with `AIRTRAFFIC_ADMIN_KEY`
unset the writes are open, and compose binds `0.0.0.0`.

MIT — Copyright (c) 2026 Justin Higgins. See [LICENSE](LICENSE).
