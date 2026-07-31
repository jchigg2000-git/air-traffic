# Air-Traffic — System Design Plan

> **Scope:** Build-ready system design for **Air-Traffic**, an enterprise AI **control plane
> and observability layer**. Air-Traffic governs AI vendors and agentic developer platforms
> two ways: it **drives each vendor's native admin API** to set and read controls, and it
> **distributes managed configuration into developer/agent environments** (MCP allow/deny,
> shared hooks, code-review gates, cron) and reads that state back. It then **monitors** the
> whole estate through one normalized signal. It is the engineering companion to the
> research/product spec in [`air-traffic-analysis.md`](./air-traffic-analysis.md).
>
> **What Air-Traffic is NOT (by design):** it is **not** an inline inference proxy on the
> critical path of every model call. Routing all developer/agent traffic through a gateway is
> a separate, heavier concern with real operational cost (key reissuance, streaming, latency,
> per-vendor request shapes, SPOF). That capability exists here only as an **optional, clearly
> bounded module** (§11) for the handful of controls that genuinely cannot be done any other
> way. The spine is control + config + observability.
>
> **Architectural anchor:** Air-Traffic is a sibling of the `it-scorecard` repo and reuses its
> proven shape — a Go HTTP service exposing a **control plane** + a **background emitter** that
> produces a normalized `ops-observation-batch/v1` signal, with a React SPA on top. it-scorecard
> configures connectors and emits signal; Air-Traffic does the same, and adds a **config
> distributor** for agentic environments. (`/Users/justinhiggins/Projects/it-scorecard`.)

---

## Table of Contents

1. [Overview](#1-overview)
2. [Architecture](#2-architecture)
3. [Core Concepts](#3-core-concepts)
4. [Components](#4-components)
5. [The VendorAdapter Contract](#5-the-vendoradapter-contract)
6. [Enforcement Mechanisms](#6-enforcement-mechanisms)
7. [Data Flows](#7-data-flows)
8. [Policy-as-Code & Drift Detection](#8-policy-as-code--drift-detection)
9. [Observability & the Normalized Audit Stream](#9-observability--the-normalized-audit-stream)
10. [External Integrations](#10-external-integrations)
11. [Optional Module: the Inference Gateway](#11-optional-module-the-inference-gateway)
12. [Deployment](#12-deployment)
13. [Key Design Decisions & Risks](#13-key-design-decisions--risks)
14. [Build Sequence (Roadmap)](#14-build-sequence-roadmap)
15. [Appendix: Repository Layout](#15-appendix-repository-layout)

---

## 1. Overview

Air-Traffic gives an enterprise **one surface** to govern and observe AI usage across every
major vendor (OpenAI, Anthropic, Google Vertex, AWS Bedrock, Azure OpenAI, GitHub Copilot,
M365 Copilot, Cohere, Mistral, …) **and** the agentic developer platforms their engineers use
(Claude Code, GitHub Copilot, Cursor, CI). It does this through three cooperating parts inside
a single Go process:

1. **Control-plane API** (`/api/*`) — the admin surface. CRUD over vendor adapters, policies,
   industry baselines, and credentials. When a policy changes, the control plane **drives each
   vendor's native admin API** (OpenAI Admin API, Bedrock, Azure ARM, Google IAM/Model Armor,
   GitHub Copilot policies, …) to set the control, and reads it back to confirm.

2. **Config distributor** (`internal/envconfig`) — for controls that live in the *developer/agent
   environment* rather than a vendor API, Air-Traffic **renders managed configuration and
   distributes it** to the environment (e.g. Claude Code managed settings, GitHub branch
   protection, Cursor admin settings, CI), then **reads the effective state back** for drift.
   This is how MCP allow/deny, shared hooks, code-review gates, cron, and per-environment model
   assignment are governed — **no request interception required.**

3. **Background emitter + observability** — every tick, emit one `ops-observation-batch/v1`
   batch per adapter so the scorecard has live, moving signal. In **synthetic** mode it produces
   walk-based demo metrics (identical to it-scorecard's `emitSynthetic`); in **proxy** mode it
   pulls real usage/cost/audit from the vendor admin APIs and reads back environment config
   state.

A **policy engine** ties them together: it reconciles declared policy-as-code (intent) against
observed vendor + environment state (actual), and continuously emits **drift** observations
when they diverge.

> **Optional, not core:** an **inference gateway** (§11) can additionally sit in the request
> path to enforce the residual runtime-only controls — hard per-user *cross-vendor* spend caps
> and per-request PII redaction. It is a bounded, separately-deployed add-on (its own
> `air-traffic-gateway` binary), and nothing in the spine depends on it.

**At a glance**

- **Backend:** Go (stdlib-first, mirroring it-scorecard's zero-dependency posture).
- **Frontend:** Vite + React + TypeScript + Tailwind + TanStack Query (same stack as it-scorecard `web/`).
- **Contract:** `ops-observation-batch/v1` — reused verbatim from it-scorecard so AI-plane tiles render in the same scorecard infrastructure.
- **Three adapter modes:** `disabled` · `synthetic` · `proxy` (mirrors it-scorecard `internal/model/model.go` `Mode`).
- **Three control dispositions:** `vendor_native` · `env_managed` · `proxy_enforced` (the last is the optional gateway). Plus `unverified` / `unsupported`.

---

## 2. Architecture

### 2.1 Context diagram

An **admin** configures policy; the control plane fans out to each vendor's **admin API** and
pushes **managed config** into the agentic environments; a **viewer** watches the scorecard. The
optional inference path (developer/agent → gateway) is shown dashed because it is not part of
the spine.

```mermaid
graph TD
  admin["Admin (browser)<br/>Rigor Console / Policy Editor"] -->|"PATCH /api/policies, /api/adapters"| spa["React SPA<br/>(scorecard + control plane)"]
  viewer["Viewer (browser)<br/>Flight Deck / Cost Explorer"] -->|"GET /api/observations (poll)"| spa
  spa -->|"/api/*"| go

  subgraph go["Control plane — air-traffic-server"]
    api["Control-plane API<br/>/api/* (incl. /api/gateway/*, /api/harness/*)"]
    env["Config distributor<br/>internal/envconfig"]
    pol["Policy engine<br/>reconcile + drift"]
    emit["Background emitter<br/>ops-observation-batch/v1"]
    store["In-memory store"]
    api --> store
    env --> store
    pol --> store
    emit --> store
    api --> pol
    pol --> env
  end

  gwopt["Inference gateway<br/>air-traffic-gateway<br/>(OPTIONAL, separate binary)"]

  pol -. "drive native admin API (set + read)" .-> vadmin["Vendor Admin APIs<br/>OpenAI Admin, Bedrock, Azure ARM,<br/>Google IAM/Model Armor, GitHub policies"]
  env -. "push managed config + read state" .-> envt["Agentic Environments<br/>Claude Code managed settings,<br/>GitHub branch protection, Cursor, CI"]
  emit -. "pull usage/cost/audit" .-> vadmin
  dev["Developer / Agent"] -. "OPTIONAL inference path" .-> gwopt
  gwopt -. "forward (real credential)" .-> vinf["Vendor Inference APIs"]
  gwopt -. "spine push: leaks/enforcement/patterns" .-> api
  api -. "policy + pattern pack" .-> gwopt
```

### 2.2 Module diagram

The Go backend is the it-scorecard shape extended with `envconfig`, `policy`, `audit`, and a
first-class eval/ratchet `harness`. Vendor surfaces are exposed through `catalog` + `synthetic`
rather than a single `adapter` package. The **inference gateway shipped 2026-07-02 as a separate
binary** — `cmd/air-traffic-gateway` with its own `internal/gateway/*` closure — **not** a package
compiled into the control plane; the control plane learns of it only through spine push/pull (see
the [§2.4 note](#24-gateway-as-a-separate-data-plane-tier) and the
[Inference Gateway Build Plan](./inference-gateway-build-plan.md)). The diagram below shows the
control-plane process and the gateway as its own tier.

```mermaid
graph LR
  subgraph cp["Control plane — cmd/air-traffic-server (internal/)"]
    main["cmd/air-traffic-server<br/>config + lifecycle"]
    server["server<br/>HTTP mux, /api/*, /synthetic/*, static SPA"]
    catalog["catalog + synthetic<br/>vendor/platform surfaces + manifests"]
    envconfig["envconfig<br/>render + push managed config; read state"]
    policy["policy<br/>reconcile, drift, baselines"]
    emitter["emitter<br/>ops-observation-batch/v1 generator"]
    audit["audit<br/>normalized audit stream"]
    harness["harness<br/>eval/ratchet: gen · runner · flywheel · score"]
    store["store<br/>in-memory state"]
    model["model<br/>domain types + contract"]
    redact["redact<br/>secret/PII redaction (log path)"]

    main --> server
    server --> catalog
    server --> policy
    server --> harness
    policy --> envconfig
    emitter --> catalog
    emitter --> envconfig
    emitter --> store
    audit --> store
    harness --> store
    catalog --> model
    store --> model
  end

  subgraph dp["Data plane — cmd/air-traffic-gateway (SHIPPED, optional)"]
    gw["internal/gateway<br/>proxy · detect · redact · credbroker"]
    gwspine["spine_emit / spine_pull<br/>observations↑ · policy+patterns↓"]
    gw --> gwspine
  end

  gwspine -. "push leaks/enforcement/patterns" .-> server
  server -. "policy + pattern pack" .-> gwspine

  subgraph frontend["React SPA (web/src)"]
    rigor["Rigor Console"]
    editor["Policy Editor"]
    cost["Cost Explorer"]
    deck["Flight Deck"]
    scorecard["Scorecard (reused)"]
    apilib["lib/api.ts"]
    rigor --> apilib
    editor --> apilib
    cost --> apilib
    deck --> apilib
    scorecard --> apilib
  end

  apilib -.->|"HTTP /api/*"| server
```

### 2.3 Adapter mode state

Each adapter moves between three modes, set from the control plane — identical semantics to
it-scorecard's connector modes.

```mermaid
stateDiagram-v2
  [*] --> synthetic
  synthetic --> proxy: attach credential + enable
  proxy --> synthetic: detach / demo
  synthetic --> disabled: disable
  proxy --> disabled: disable
  disabled --> synthetic: re-enable
  note right of synthetic
    walk-based demo metrics;
    no real vendor or env calls
  end note
  note right of proxy
    control plane drives the vendor admin API
    and/or pushes managed env config;
    emitter pulls real usage/cost/audit + reads env state
  end note
```

### 2.4 Gateway as a separate data-plane tier

The gateway is **not** compiled into `air-traffic-server` and is **not** mounted at any in-process
route. It shipped 2026-07-02 as its **own stateless data-plane binary** (`cmd/air-traffic-gateway`)
with its own `config.Load()`, deploy unit, and — when scaled — L7 load balancer, in the **same
repo**, sharing `internal/model` contracts. This was the design from the start, for reasons that
also justify keeping it off the control-plane process (full decision table in the
[Build Plan §2](./inference-gateway-build-plan.md#2-architectural-decision-separate-data-plane-service-vs-in-process)):

- **Orthogonal scaling axes** — the control plane is low-QPS and read-mostly; the gateway scales
  with inference traffic. Co-locating forces the whole control plane to absorb inference spikes.
- **Statefulness mismatch** — the control plane is happily single-node in-memory; the gateway must
  be stateless with state externalized to Redis/KMS to scale out at all.
- **Failure domain** — the gateway is a SPOF on the request path; the control plane must not share
  its blast radius (the whole ethos is "off the spine").
- **Dependency purity** — keeping the gateway a separate binary leaves the control plane's
  dependency closure **stdlib-only**.

The two tiers stay one system via contracts that already exist: **policy flows down** (the control
plane is the policy authority; the gateway is an enforcement point) and **observations flow up**
(the gateway emits `ops-observation-batch/v1` + leak findings into the existing spine). See §11.

---

## 3. Core Concepts

### 3.1 The three modes (reused from it-scorecard)

| Mode | Control plane | Config distributor | Emitter |
|---|---|---|---|
| `disabled` | no-op | no-op | silent |
| `synthetic` | echoes config locally; no vendor call | renders config but does not push | walk-based metrics |
| `proxy` | drives the vendor's native admin API | pushes managed config + reads state | pulls real usage/cost/audit |

Synthetic mode makes Air-Traffic demoable from the first commit with zero credentials — exactly
how it-scorecard ships live tiles with no upstreams.

### 3.2 The control dispositions (the honesty layer)

Every capability an adapter exposes carries a disposition. This is the Air-Traffic analog of
it-scorecard's `proxy_not_normalized` signal — it never fabricates a green and is always explicit
about *which mechanism* fulfils a control.

| Disposition | Mechanism | Where it executes | Examples |
|---|---|---|---|
| `vendor_native` | call the vendor admin API | control plane → vendor admin API | model_permissions, Bedrock Guardrails, spend_alerts, audit export, content filters |
| `env_managed` | push managed config into the environment, read it back | config distributor → agentic platform | MCP allow/deny, shared hooks, code-review gate, cron, per-env model assignment |
| `proxy_enforced` | intercept the request (OPTIONAL gateway) | data plane → gateway (§11) | hard per-user *cross-vendor* $ cap at call time, per-request PII scrub |
| `monitor_only` | verify / gate / alert — no *set* mechanism | emitter + policy engine | contractual training opt-out, ZDR-via-DPA, vendor-side retention/residency, soft spend alerts, read-only audit gaps |
| `unverified` | could not confirm from vendor/platform docs | surfaced as amber, not asserted | — |
| `unsupported` | platform explicitly cannot do this | surfaced, excluded from baselines | — |

> The single most important correction over a naive design: **agentic primitives (MCP/hooks/
> code-review/cron) are `env_managed`, not `proxy_enforced`.** They are governed by distributing
> managed configuration into the environment and reading it back — not by intercepting traffic.
> This is why the spine needs no inference gateway.

### 3.3 How `env_managed` actually works — and its enforcement-confidence tiers

The config distributor renders a platform-specific managed-config artifact, distributes it, and
reads the effective state back. **Enforcement strength is not binary** — research across the
agentic platforms (verified June 2026) shows three tiers, and Air-Traffic must track an
**enforcement-confidence level per `(control × platform × device)`** rather than a flat "enforced."

**Tier A — Server-side enforced (strongest; non-bypassable, needs no client cooperation).** The
control is enforced on the vendor's servers, so a developer cannot override it locally:

| Control | Platform mechanism | Read-back |
|---|---|---|
| Code-review gate + **test-coverage gate** | **GitHub / GitLab branch protection & rulesets** (required reviewers, required status checks; enforced at merge) | branch-protection / rulesets REST API |
| MCP allow/deny | **GitHub Copilot** registry-only allowlist (server-side); **GitLab Duo** cascading lock | audit log; content-exclusion API |
| Feature/model gating | GitLab Duo (`lock_duo_features_enabled`), Gemini admin controls, Cursor Enterprise (Privacy Mode + model allowlist), Amazon Q (SCP gating) | settings / admin API; CloudTrail |

**Tier B — Locked client config via OS/MDM (admin-enforced *iff* the device is MDM-managed).**
A managed file at an OS path takes top precedence over user/project settings:

| Control | Platform mechanism | Read-back |
|---|---|---|
| MCP allow/deny, hooks, permissions, agent/skill exec | **Claude Code** `managed-settings.json` (macOS `/Library/Application Support/ClaudeCode/`, Linux `/etc/claude-code/`, Windows `C:\Program Files\ClaudeCode\`; `managed-settings.d/` drop-ins) — `allowedMcpServers`/`deniedMcpServers`/`allowManagedMcpServersOnly`, `allowManagedHooksOnly`, `allowManagedPermissionRulesOnly`, `disableSkillShellExecution` | `claude doctor` (effective setting + source) |
| MCP/extension access, agent mode, telemetry | **VS Code** enterprise policies (Windows ADMX → registry, Intune, macOS `.mobileconfig`) — `ChatMCP`, `ChatAgentMode`, `AllowedExtensions`, `Claude3PIntegration`/`Codex3PIntegration` | policy/registry state |
| ZDR, feature disable, RBAC | **Windsurf (Codeium)** admin console policy lock | audit logs |

> **Hard dependency:** Tier-B locks require **MDM enrollment** (Jamf / Intune / Kandji) to deploy
> the managed file. On an *unmanaged* device the same controls silently degrade to Tier C. So the
> product must verify MDM coverage before claiming Tier-B enforcement.

**Tier C — Seed + drift-detect only (user *can* override).** Local client-file settings outside
the locked set (Cursor local MCP/dotfiles, JetBrains fine-grained config, any unmanaged device).
Air-Traffic distributes the config and **detects drift**, but cannot prevent override.

**Design rule (`internal/envconfig`):** every `env_managed` capability is emitted with an
`enforcement` field ∈ `{server_side, mdm_locked, seed_only}`. The UI renders `server_side` and a
*verified-managed* `mdm_locked` device as **green (enforced)**; an `mdm_locked` control on an
**unverified-MDM** device and all `seed_only` controls render **amber (monitored, not enforced)** —
**a Tier-C control is never shown as "enforced."** Lead baselines with Tier-A controls for
guaranteed enforcement; gate Tier-B controls behind an MDM-coverage check.

---

## 4. Components

> File paths are the proposed Air-Traffic layout. "Mirrors X" points at the it-scorecard file the
> design is lifted from.

### `cmd/air-traffic-server` — control-plane entrypoint
- **Responsibility:** parse config, wire store + server + emitter + policy drift + the harness engine, run HTTP with graceful shutdown.
- **Config (env):** `AIRTRAFFIC_ADDR` (default `127.0.0.1:8122`), `AIRTRAFFIC_EMIT`, `AIRTRAFFIC_EMIT_INTERVAL_SECONDS`, `AIRTRAFFIC_DATA_DIR` (harness durable state), `AIRTRAFFIC_GATEWAY_KEY` (client key the harness uses to drive the gateway), `AIRTRAFFIC_PRESIDIO_URL`. The store is **in-memory only** — there is no store-backend switch and no in-process gateway toggle.
- **Mirrors:** it-scorecard's `cmd/harness-server/main.go`.

### `internal/server` — HTTP layer
- **Responsibility:** route registration for `/api/*` (control plane), the `/synthetic/*` vendor surfaces, static SPA serving, panic recovery. It **never** mounts the gateway in-process; instead it **always** serves `/api/gateway/{leaks,enforcement,patterns,status}` (`server.go:55–58`), which read data the separate gateway binary **pushes up the spine**.
- **Routes** (`server.go:43–65`): `/api/health`, `/api/adapters(+/)`, `/api/baselines`, `/api/policies`, `/api/credentials`, `/api/observations`, `/api/audit`, `/api/activity`, `/api/drift`, `/api/cost/facets`, `/api/envconfig`, `/api/gateway/{leaks,enforcement,patterns,status}`, `/api/harness/{runs,runs/,sample,ratchet,corpus,proposals,proposals/}` (503 via `requireHarness` when the engine is absent), `/synthetic/`, and `/`.
- **Mirrors:** it-scorecard's `internal/server/server.go`, `routes.go`.

### `internal/catalog` + `internal/synthetic` — vendor & platform surfaces ⭐
- **Responsibility:** `catalog` (`catalog.go`, `vendors.go`, `cost_facets.go`) holds the vendor/platform registry, capability manifests, and cost facets — the machine-readable form of the analysis doc's vendor tables, each `CapabilityEntry` carrying a disposition. `synthetic` serves per-vendor native-shaped surfaces under `/synthetic/{vendor}/…` so the control plane is demoable with zero credentials.
- **Two halves per capability:**
  - *Control half* — get/set policy by calling the vendor admin API (`vendor_native`).
  - *Env half* — render managed config + read effective state for `env_managed` controls (the agentic-platform surfaces lean on this half).

### `internal/envconfig` — config distributor ⭐
- **Responsibility:** render platform-specific managed-config artifacts (Claude Code managed settings, GitHub branch-protection rules, etc.), distribute them through the configured channel (MDM endpoint, platform API, git commit), and read effective state back for drift.
- **Key file:** `envconfig.go` (policy → artifact per platform, distribute, and effective-state read-back).
- **New concept (no it-scorecard equivalent):** this is the agentic-governance heart of the product.

### `internal/policy` — policy engine ⭐
- **Responsibility:** reconcile intent vs actual across **both** the vendor-API surface and the env-config surface, compute drift, and hold the **baseline profiles** library (General SaaS 🔒, Fintech 🔒🔒, Healthcare 🔒🔒🔒, Gov 🔒🔒🔒). Policy domain types live in `internal/model/policy.go`.
- **Key files:** `reconcile.go`, `drift.go`, `baselines.go`.

### `internal/emitter` — background signal
- **Responsibility:** per tick, emit one `ops-observation-batch/v1` per adapter. Synthetic = walk (reuse it-scorecard's `walk`/`metricDef`); proxy = pull vendor usage/cost/audit + read env-config state. Also emits drift observations from the policy engine.
- **Mirrors:** it-scorecard's `internal/emitter/emitter.go`; the proxy branch gains real vendor-API + env-state normalizers (the gap it-scorecard explicitly left open).

### `internal/audit` — normalized audit stream ⭐
- **Responsibility:** ingest vendor-native audit events (OpenAI Audit Logs + ChatGPT Enterprise Compliance Logs, Azure Activity Log, GitHub Audit Log incl. 2026 agentic events, CloudTrail, M365 Purview, Google Reports API) + config-distribution events, normalize into one `ops-observation-batch/v1`-shaped stream (OTel GenAI semconv) with `plane`/`vendor`/`control_surface` dimensions; expose `/api/audit` + SIEM export.

### `internal/harness` — eval / ratchet harness ⭐ (shipped, first-class)
- **Responsibility:** a first-class subsystem that drives synthetic traffic through the gateway, scores detector performance, and ratchets it upward. Files: `gen.go` (corpus generation), `runner.go` (run orchestration), `streamscan.go`, `persist.go` (durable state under `AIRTRAFFIC_DATA_DIR`), `sample.go` (try-a-prompt), `probe.go` (Presidio raw-score probe), `flywheel.go` (the improvement loop feeding G6 config-knob proposals), `score.go`.
- **Surfaces:** `/api/harness/{runs,sample,ratchet,corpus,proposals}` — try-a-prompt, the ratchet ledger, the corpus, and config-knob **proposals** — all guarded by `requireHarness` (503 when the engine is not attached).

### `internal/store` — state
- **Responsibility:** in-memory store of adapters, credentials (by reference), policies, baselines, rendered env artifacts + read-back state, observation batches, audit events, activity log. Mirrors `internal/store/store.go`.

### `internal/model` — domain + contract
- **Responsibility:** domain types + `ObservationContract` (reused), plus Air-Traffic additions: `Plane`, `Disposition`, `CapabilityEntry`, `AdapterManifest`, `Policy`, `ManagedArtifact`.

### `internal/redact` — safety
- **Responsibility:** secret/PII redaction before any storage (reused from it-scorecard). *(Request-path PII redaction lives in the optional gateway, §11.)*

> **Not yet built:** there is no `internal/secrets` or `internal/identity` package. Credentials are
> held **by reference** only; admin auth + KMS/IdP integration are a hardening-phase intent (§13).

### `internal/gateway` — inference-gateway data plane (SHIPPED, separate binary)
- See §11. The gateway's hot-path code lives here and ships as its **own binary**
  (`cmd/air-traffic-gateway`, shipped 2026-07-02) with its own `config.Load()` — a separate,
  independently-booting data-plane service, **never** mounted into `air-traffic-server`.
- **Files:** `proxy.go` (reverse proxy + streaming), `adapter_anthropic.go` (Anthropic Messages
  dialect), `server.go` (`POST /v1/messages`, `/healthz`, `/readyz`), `spine_emit.go`
  (observations + leak findings + heartbeat up), `spine_pull.go` (policy + pattern pack down),
  `metrics.go`, `audit.go`, `health.go`, `stream.go`.
- **Subpackages:** `config/`, `credbroker/` (credential broker), `detect/` (regex + Presidio
  detectors), `redact/` (mask / reversible tokenize).
- Nothing in the control plane imports it, so `air-traffic-server`'s dependency closure stays
  stdlib-only. Build sequence and package layout:
  [Inference Gateway Build Plan](./inference-gateway-build-plan.md).

### `web/` — React SPA
- **Responsibility:** the **Flight Deck** at `/`, a public **`/welcome`** landing page (`pages/landing/*`: Hero, Planes, VendorWall, Dispositions, HowItWorks, CtaBand; added 2026-07-03), and the `/settings/*` control surfaces (Rigor Console, Policy Editor, Cost Explorer, Vendors, Observability, Audit, Gateway Harness). Mirrors it-scorecard `web/`.
- **Tests:** a vitest suite (`package.json` → `"test": "vitest run"`; e.g. `pages/Welcome.test.tsx`), added 2026-07-03.

---

## 5. The VendorAdapter Contract

The full Go interface is in
[`air-traffic-analysis.md` §5](./air-traffic-analysis.md#5-air-traffic-api-vendoradapter-architecture).
Every adapter implements method groups across the planes, plus the two halves:

- **Control half (`vendor_native`):** `ListModels`, `SetModelAccess`, `GetDataPolicy/SetDataPolicy`, `GetContentSafetyPolicy/SetContentSafetyPolicy`, `GetSpendLimits/SetSpendLimits`, `GetUsage`, `GetAuditLogs`, `GetUsageMetrics` — each calling the vendor admin API.
- **Env half (`env_managed`):** `RenderManagedConfig(policy) → ManagedArtifact` and `ReadEnvState() → EnvState` — for the agentic-platform controls (MCP, hooks, code-review, cron).
- **Emission:** `Emit(ctx, ts) → model.ObservationRecord` (the it-scorecard contract).

Every method returns a `Signal{Disposition, Code, Message, Retryable}` so the caller always knows
whether the outcome came from the vendor API (`vendor_native`), an env-config push (`env_managed`),
or — only in the optional gateway — request interception (`proxy_enforced`). This is the
load-bearing honesty primitive, the same role `proxy_not_normalized` plays in it-scorecard.

> Note there is **no `Forward()` in the core interface.** Request forwarding belongs to the
> optional gateway adapter (§11), keeping the spine free of the inference path.

---

## 6. Enforcement Mechanisms

Air-Traffic enforces a control through whichever of three mechanisms fits — and says which:

```mermaid
flowchart TD
  ctrl["A control to enforce<br/>(from policy-as-code)"] --> q1{"vendor exposes<br/>a native admin API?"}
  q1 -->|yes| native["vendor_native<br/>call admin API; read back to confirm"]
  q1 -->|no| q2{"lives in the dev/agent<br/>environment?"}
  q2 -->|yes| env["env_managed<br/>render managed config → distribute → read back<br/>(MCP, hooks, code-review, cron)"]
  q2 -->|no, runtime-only| q3{"inference gateway<br/>deployed?"}
  q3 -->|yes| proxy["proxy_enforced (optional §11)<br/>intercept request"]
  q3 -->|no| obs["observe + alert only<br/>(soft): meter from vendor usage API,<br/>flag breaches, no hard stop"]
```

The decisive design point: **only the bottom-right leaf needs the inference gateway.** Everything
the user's core product targets — vendor controls and agentic primitives — resolves to
`vendor_native` or `env_managed`, neither of which touches the request path. Where a runtime-only
control (hard cross-vendor cap, per-request PII scrub) is in policy but the gateway is not deployed,
Air-Traffic degrades **honestly** to observe-and-alert rather than silently claiming enforcement.

---

## 7. Data Flows

### 7.1 Live emit → scorecard (reused from it-scorecard)

```mermaid
sequenceDiagram
  participant Emit as Emitter
  participant Store as Store
  participant API as /api/observations
  participant Deck as Scorecard / Flight Deck
  loop every AIRTRAFFIC_EMIT_INTERVAL_SECONDS
    Emit->>Emit: per adapter — synthetic walk OR proxy pull (vendor API + env state)
    Emit->>Store: AddObservation(ops-observation-batch/v1)
  end
  loop every 5s (TanStack Query)
    Deck->>API: GET /api/observations
    API->>Store: ListObservations
    Store-->>API: batches
    API-->>Deck: {observations}
    Deck->>Deck: compute RAG tiles + scores
  end
```

### 7.2 Admin applies a baseline → reconcile across both surfaces

```mermaid
sequenceDiagram
  participant Admin as Rigor Console
  participant API as /api/policies
  participant Pol as Policy engine
  participant Ad as VendorAdapters (control half)
  participant Env as Config distributor
  Admin->>API: PUT /api/policies (apply "Fintech 🔒🔒")
  API->>Pol: load intent
  Pol->>Ad: vendor_native controls → SetX() on vendor admin API
  Pol->>Env: env_managed controls → render + distribute managed config
  Ad-->>Pol: Signal{vendor_native}
  Env-->>Pol: Signal{env_managed}
  Pol->>Pol: recompute drift (intent vs actual) → 0
  Pol-->>Admin: coverage report (native vs env-managed vs proxy-needed per control)
```

### 7.3 Agentic config push + read-back (the core agentic flow)

```mermaid
sequenceDiagram
  participant Pol as Policy engine
  participant Env as Config distributor
  participant CC as Claude Code (managed settings)
  participant GH as GitHub (branch protection)
  Pol->>Env: render MCP allow/deny + hooks → Claude Code managed-settings.json
  Pol->>Env: render code-review gate → GitHub branch-protection rule
  Env->>CC: distribute managed-settings.json (MDM channel)
  Env->>GH: PUT branch protection (required reviewers + status checks)
  Env->>CC: read back effective managed settings
  Env->>GH: read back branch-protection state
  Env-->>Pol: EnvState (for drift comparison)
```

### 7.4 Drift detection loop

```mermaid
sequenceDiagram
  participant Emit as Emitter (tick)
  participant Ad as VendorAdapter
  participant Env as Config distributor
  participant Pol as Policy engine
  participant Store as Store
  Emit->>Ad: read vendor actual (GetDataPolicy/GetSpendLimits/…)
  Emit->>Env: read env actual (ReadEnvState)
  Ad-->>Emit: vendor state (+Signal)
  Env-->>Emit: env state (+Signal)
  Emit->>Pol: compare actual vs declared intent
  alt diverged
    Pol->>Store: AddObservation(kind=state, signal=drift, severity=warning)
    Note over Store: Flight Deck DRIFT column → amber
  else in sync
    Pol->>Store: AddObservation(kind=state, signal=synced, severity=info)
  end
```

---

## 8. Policy-as-Code & Drift Detection

A single declarative document is the source of truth for intent across all three planes and both
surfaces (vendor API + env config). The reconciler converges actual→intent; the drift detector
reports divergence.

```yaml
# air-traffic-policy.yaml
baseline: fintech-elevated          # one-click industry baseline, then override below
vendor_defaults:
  training_opt_out: true            # vendor_native where available
  content_safety: elevated          # vendor_native (Azure/Bedrock/Model Armor) else observe
  data_retention_days: 7
vendors:
  openai:
    model_access: { mode: allow_list, allowed_models: [gpt-4o, gpt-4o-mini, o3-mini] }  # vendor_native
    spend_alerts: { threshold_cents: 5000000 }                                          # vendor_native (soft)
  bedrock:
    content_safety: { guardrail_id: g-12345abc, apply_cross_account: true }             # vendor_native
agentic:                            # env_managed — pushed into dev/agent environments
  claude_code:
    mcp:
      allow: [filesystem, git, internal-db-readonly]
      deny: ["*"]                    # default-deny, allow-list above
    hooks:
      pre_commit: [run-tests, secret-scan]
    managed_settings_locked: true    # admin-controlled; user cannot override
  github:
    code_review:
      require_human_review: true     # branch protection: required reviewers
      required_status_checks: [tests, coverage-90]
budget:
  org_monthly_cap_usd: 50000         # vendor_native soft caps + cross-vendor MONITOR by default;
                                     # hard cross-vendor enforcement only if gateway enabled (§11)
  per_user_cap_usd: 500              # observe + alert by default; proxy_enforced if gateway on
```

**Reconcile algorithm (`internal/policy/reconcile.go`):**
1. Resolve effective intent = baseline ⊕ overrides.
2. For each `(target, control)`: read actual; if `vendor_native`, call `SetX()` on the vendor API;
   if `env_managed`, render + distribute the managed artifact; record the `Signal`.
3. Emit a **coverage report**: per control, native vs env-managed vs proxy-needed vs unverified.

**Drift (`internal/policy/drift.go`):** the emitter periodically re-reads actual state from both
surfaces. Any `(target, control)` whose actual ≠ intent emits a `drift` observation (amber). Drift
sources: a console side-change, a vendor default change, a developer overriding a *seeded* (not
locked) managed setting, credential rotation that silently disables a `vendor_native` control, or a
new environment added without policy coverage.

---

## 9. Observability & the Normalized Audit Stream

Air-Traffic mirrors the three control planes in observability and unifies everything into the
single `ops-observation-batch/v1` contract.

- **Budget plane:** tokens/cost per vendor/model/team from vendor usage APIs; cap utilization
  computed by Air-Traffic. (Per-user *enforcement* is optional/proxy, but per-user *attribution*
  comes from vendor logs like Bedrock `identity.arn` + Application Inference Profiles.)
- **Data-policy plane:** content-safety triggers (vendor-native where available), ZDR compliance,
  drift events.
- **Developer-workflow plane:** MCP allow/deny state, hook config, code-review gate state, and
  **agentic action logs from the platforms' own audit surfaces** (GitHub 2026 agentic audit events,
  Claude Code telemetry, ChatGPT Enterprise Compliance Logs) — read, normalized, not intercepted.

**Schema — OpenTelemetry GenAI semantic conventions.** The normalized stream maps its
`ops-observation-batch/v1` dimensions onto OTel GenAI semconv attributes (`gen_ai.system`,
`gen_ai.request.model`, `gen_ai.usage.input_tokens`/`output_tokens`, …) so it is ingestible by any
OTel/SIEM consumer (Datadog, Splunk, Elastic) and deep per-span tracing can be **offloaded to
Langfuse via its OTLP endpoint** rather than rebuilt.

**Normalized audit event** (one shape for vendor-native + env-config + (optional) gateway events):

```json
{
  "id": "...", "timestamp": "...", "actor": "user:jdoe", "action": "mcp.policy.set",
  "resource": "claude-code:internal-db-readonly", "plane": "developer_workflow",
  "vendor": "claude_code", "control_surface": "env_managed",
  "before": {"allow": ["filesystem"]}, "after": {"allow": ["filesystem","internal-db-readonly"]},
  "request_id": "req-..."
}
```

---

## 10. External Integrations

| Name | Protocol | Direction | Surface | Notes |
|---|---|---|---|---|
| Browser SPA | HTTP/JSON | inbound | `/`, `/settings/*`, `/api/*` | scorecard + 4 control surfaces |
| Vendor **admin** APIs | HTTPS | outbound (proxy mode) | per-adapter | drives `vendor_native` controls + pulls usage/cost/audit |
| **Agentic environments** | HTTPS / MDM / git | outbound (proxy mode) | Claude Code managed settings, GitHub branch-protection + policy API, Cursor, CI | `env_managed` push + read-back |
| SIEM (Splunk/Elastic/Datadog) | HTTP/webhook | outbound (opt-in) | audit export | normalized stream (OTel GenAI semconv) |
| Langfuse (deep tracing) | OTLP | outbound (opt-in) | `/api/public/otel` | offload per-span tracing |
| Secret manager / KMS | — | outbound (intended) | credential references (`secret_ref`) | resolves credential references — not yet built |
| IdP (SSO/SCIM) | — | outbound (intended) | admin auth (intended) | admin auth + distribution-channel auth — not yet built |
| Vendor **inference** APIs | HTTPS | outbound (**optional gateway only**) | gateway upstreams (`GATEWAY_UPSTREAMS`) | served by the separate `air-traffic-gateway` binary, not the control plane |

Synthetic mode keeps **all** outbound integrations opt-in — the service is fully functional and
demoable with zero credentials, exactly like it-scorecard.

---

## 11. Optional Module: the Inference Gateway

> **Status: shipped 2026-07-02 as a separate, optional binary (`cmd/air-traffic-gateway`). Off the
> spine.** Deploy it only if the residual runtime-only controls below are a hard requirement. Full vendor-neutral design:
> [`inference-gateway-design.md`](./inference-gateway-design.md). Sequenced, horizontally-scalable
> build: [`inference-gateway-build-plan.md`](./inference-gateway-build-plan.md). The gateway's
> headline residual control is **per-request PII/PHI redaction**; the hard cross-vendor spend cap
> rides the same data plane.

Some controls cannot be done by calling a vendor API or pushing environment config — they require
being *in the request path* at call time. There are essentially two:

1. **Hard, cross-vendor, per-user spend caps** — stop a user mid-request when their budget is gone,
   *even on a vendor with no native cap* (AWS Bedrock has none; AWS itself prescribes a proxy).
2. **Per-request PII/PHI redaction** of arbitrary outbound traffic (e.g. healthcare baseline).

Everything else — including *monitoring* spend, *setting* a vendor's native cap, *soft* alerts, and
all agentic governance — needs no gateway.

The gateway follows the standard AI-gateway pattern (validated against LiteLLM / Portkey / Kong):
a vendor-dialect ingress (Anthropic Messages `POST /v1/messages` first), **client keys** that map to
real vendor credentials and a policy scope via the credential broker, ordered guardrail hooks
(PII detection + redaction), and **threshold actions** (mask / block / per-policy). The shipped MVP
covers detect + redact + credential broker + spine push/pull; the **Redis cross-pod spend counter
with DB fail-closed** and cross-vendor hard caps remain Build-Plan milestones. Developers point
their SDK `base_url` at the gateway and authenticate with a client key; the gateway enforces, swaps
in the real credential, meters, and audits.

```mermaid
flowchart LR
  in["request (virtual key)"] --> vk{"resolve vkey<br/>→ identity + scope"}
  vk --> bud{"budget pre-check<br/>Redis counter, DB fail-closed"}
  bud -->|over hard cap| deny["block / throttle / downgrade"]
  bud -->|ok| pii["PII redaction + safety pre-filter"]
  pii --> fwd["forward to vendor (real credential)"]
  fwd --> meter["meter → increment counter → audit"]
  meter --> out["response"]
```

**Operational cost to be honest about:** this makes Air-Traffic latency-critical and a potential
SPOF, requires reissuing/rotating vendor keys so the gateway is the only path, and requires
per-vendor request-shape + streaming handling. That cost is exactly why it is **opt-in**, not the
foundation.

### 11.1 Topology, horizontal scale, and how it fits the spine

The gateway ships as a **separate, stateless data-plane service** (§2.4) that scales out behind an
L7 load balancer. The pieces that make scale-out *correct* (not just possible) — some in the
shipped MVP, some still Build-Plan milestones:

- **Stateless pods.** All cross-request state is externalized — the **token vault** and per-caller
  **budget counters** live in **Redis** (scoped + salted, short TTL, KMS envelope-encrypted), so a
  pod can be added or lost mid-conversation without breaking token stability or caps. Sticky routing
  is a cache optimization, never a correctness requirement.
- **Raw PHI stays in-pod.** The async leak monitor and its seconds-long capture buffer run *inside*
  each pod; only **findings** (`{request_id, type, confidence, in_redaction_map}`) or **synthetic
  surrogates** ever leave it. The heavy NER/DLP detector (self-hosted Presidio for PHI) is a
  separate, independently-autoscaled tier.
- **Uniform fail mode** (`FAIL_MODE=open|closed`) across all pods, so scaling never changes the
  security posture.

**Fit with the spine (no new UI).** The gateway is the concrete realization of the
**`proxy_enforced`** disposition the control plane already promises. It integrates through existing
contracts:

| Direction | Mechanism | Effect |
|---|---|---|
| Up | emits `ops-observation-batch/v1` → `POST /api/observations`; leak findings → audit stream | gateway activity lights up **Flight Deck / Observability / Cost Explorer / Audit** with zero new screens |
| Up | reports enforcement status | capabilities flip `monitor_only` → `proxy_enforced`, shown truthfully in the **Policy Editor** |
| Down | pulls policy-as-code from `GET /api/policies` | control plane = policy authority, gateway = enforcement point; changes propagate without redeploy |
| Both | intended-vs-actual enforcement | feeds the existing **drift** loop |

The published **recall ratchet** (`v3 catches 96.2% of held-out PHI, up from 91%`) becomes a
first-class metric in that observation stream — the differentiator no proxy-only incumbent ships.

---

## 12. Deployment

### Runtime topology
- **Control plane:** a **single Go process** (`air-traffic-server`) serving the control-plane API, config distributor, emitter, harness engine, and static SPA. Off the request path; happily single-node.
- **Gateway data plane (shipped 2026-07-02, optional):** a **separate, stateless binary** (`cmd/air-traffic-gateway`, default listen `127.0.0.1:8125`) with an optional Presidio detector sidecar. The shipped MVP runs a **single replica**. The scale-out design — `1 → N` replicas behind an L7 load balancer, shared **Redis** (token vault + budget counters, KMS-encrypted), and a separately-scaled **detector tier** — is the target once traffic demands it. Deployed and scaled independently of the control plane; integrates via the spine contracts (§11.1). Full topology + sequence: [Build Plan §3](./inference-gateway-build-plan.md#3-target-architecture-fit--horizontal-scale).
- **Frontend:** dev via Vite proxy; prod-style via `web/dist` served same-origin (mirrors it-scorecard).

### Configuration surfaces (env)
Control-plane process (`cmd/air-traffic-server`):

| Var | Default | Effect |
|---|---|---|
| `AIRTRAFFIC_ADDR` | `127.0.0.1:8122` | listen address |
| `AIRTRAFFIC_EMIT` | `on` | background emitter on/off |
| `AIRTRAFFIC_EMIT_INTERVAL_SECONDS` | `5` | emit tick cadence |
| `AIRTRAFFIC_DATA_DIR` | `data/harness` | harness durable state (ratchet, corpus, pattern pack) |
| `AIRTRAFFIC_GATEWAY_KEY` | `gwk-demo` | client key the harness uses to drive the gateway |
| `AIRTRAFFIC_SPINE_KEY` | *(unset)* | shared key required on `/api/gateway/{leaks,enforcement,patterns}`; unset ⇒ those routes accept **loopback callers only** |
| `AIRTRAFFIC_PRESIDIO_URL` | `http://127.0.0.1:8126` | Presidio sidecar for the harness raw-score probe |

**Spine auth.** The three routes the gateway drives are not UI routes: two ingest enforcement
evidence, and `GET /api/gateway/patterns` *distributes* the pattern pack — which since the G6
config-knob slice carries deny-list **terms**. So the read side is gated too. With
`AIRTRAFFIC_SPINE_KEY` set, callers present it as `Authorization: Bearer …` or
`X-Air-Traffic-Key` (constant-time compared); with it unset, only loopback callers are accepted
— a container-network peer is *not* loopback, which is why the compose stack sets the key on
both services. `GET /api/gateway/status` stays open: it is the browser's liveness view, carries
no terms, and reports the posture in effect (`spine_auth`, `spine_key_unrotated`). The
compose defaults (`gwk-demo` / `spine-dev-insecure`) are throwaway values; both binaries log a
warning while they are in use, and `scripts/dev-env.sh` mints random replacements into `.env`.

The store is **in-memory only** — there is no `AIRTRAFFIC_STORE`/Postgres switch and no in-process
gateway toggle. The control plane's gateway panels (`/api/gateway/*`) are always live and simply
render whatever the gateway binary has pushed up the spine.

> The gateway **binary** carries its own config surface, read by its own `config.Load()` and **not**
> driven by the `AIRTRAFFIC_*` env of the control-plane process: `GATEWAY_LISTEN_ADDR` (default
> `127.0.0.1:8125`), `GATEWAY_UPSTREAMS`, `GATEWAY_CLIENT_KEYS_REF`,
> `GATEWAY_CONTROL_PLANE_KEY_REF` (default `env:GATEWAY_CONTROL_PLANE_KEY`; the spine key,
> optional — absent means this gateway can only reach a control plane over loopback),
> `GATEWAY_DETECTORS`
> (`regex`,`presidio`), `GATEWAY_REDACT_ACTION` (`mask`/`block`/`per_policy`), `GATEWAY_FAIL_MODE`
> (`open`/`closed`), `GATEWAY_CONTROL_PLANE_URL`, `GATEWAY_PRESIDIO_URL`, … See
> [Build Plan §5](./inference-gateway-build-plan.md#5-build-milestones) for the per-milestone keys
> and [design doc §8](./inference-gateway-design.md) for the full list. Secrets are stored
> **by reference** (`env:NAME`), never inline, on both sides.

### Secrets handling
- Credentials stored **by reference** (`secret_ref`); plaintext rejected on write (reuse `internal/redact.HasPlaintextSecretKey`).
- Distribution-channel credentials (MDM, GitHub app, etc.) held **by reference** (`env:NAME` / KMS/Vault), never inlined; a dedicated secrets resolver is a hardening-phase intent.
- Recorded calls + observation bodies redacted before storage.

---

## 13. Key Design Decisions & Risks

- **The spine never intercepts inference.** Governance is achieved by driving vendor admin APIs and
  distributing managed environment config — both off the request path. This is the deliberate scope
  decision (the inline gateway is optional, §11). Consequence: a handful of runtime-only controls
  (hard cross-vendor caps, per-request PII scrub) are **observe-and-alert** unless the optional
  gateway is enabled. The UI must label those as monitored-not-enforced so the posture is never
  overstated.

- **`env_managed` enforcement strength is tiered (resolved — see §3.3).** Server-side controls
  (GitHub/GitLab rulesets, Copilot registry-only MCP, GitLab Duo, Gemini/Cursor/Amazon Q admin
  gating) are genuine, non-bypassable enforcement. OS/MDM-locked client config (Claude Code
  `managed-settings.json`, VS Code policies, Windsurf) is genuine **only on MDM-enrolled devices** —
  the single biggest caveat. Seed-only config is drift-detected, never "enforced." **Mitigation:**
  every `env_managed` capability carries an `enforcement` ∈ `{server_side, mdm_locked, seed_only}`;
  `mdm_locked` is gated behind a verified-MDM check; the UI renders Tier-C as monitored-not-enforced.

- **Read-back depends on each platform exposing effective config.** Drift detection needs to read
  the *actual* state. Where a platform offers no read API, drift on that control is `unverified`.

- **Proxy-mode normalizer is real per-adapter work.** it-scorecard left proxy normalization
  unimplemented; Air-Traffic must implement real vendor-API + env-state normalizers in each adapter.
  This is the bulk of per-adapter effort.

- **In-memory store loses state on restart.** Same posture as it-scorecard; Postgres for
  policies/audit at the hardening phase.

- **Vendor admin APIs drift and rate-limit.** Pulling usage/cost/audit every tick can hit vendor
  rate limits. **Mitigation:** per-vendor pull cadence + caching, decoupled from the 5s scorecard tick.

- **(Optional gateway only) durable, fail-closed budget counters.** If the gateway is enabled, its
  hard caps need Redis cross-pod counters with a DB fail-closed mode — a hard cap that silently
  fails open is worse than no cap because it reads as enforced.

- **Identity & secrets are not built yet.** There is no auth or KMS integration and no `internal/identity`/`internal/secrets` package; credentials are held by reference only. Production needs a real IdP + KMS at the hardening phase.

---

## 14. Build Sequence (Roadmap)

Each phase is independently demoable. Phases 0–1 reuse it-scorecard scaffolding almost verbatim;
the differentiated work is Phase 2 (config distributor) and Phase 3 (real vendor-API adapters).

| Phase | Goal | Key deliverables | Reuses from it-scorecard |
|---|---|---|---|
| **0 — Skeleton** | Service boots; scorecard renders AI tiles | `model`, `store`, `server`, synthetic `emitter`, scorecard SPA | ~80% verbatim |
| **1 — Adapters (synthetic)** | All vendors + platforms present as adapters with manifests | `adapter` iface + manifests; Rigor Console + Flight Deck read manifests | emitter/catalog pattern |
| **2 — Config distributor** ⭐ | Agentic primitives governed via env config | `envconfig` (render/distribute/readback) + per-capability `enforcement` tier; **start Tier-A server-side** (GitHub/GitLab rulesets, Copilot MCP registry), then Tier-B (Claude Code/VS Code, gated on MDM check) | — |
| **3 — Vendor-API adapters** ⭐ | Real vendor admin APIs driven + read | control half for Bedrock, Azure, OpenAI, Vertex, GitHub Copilot (most `vendor_native`) | proxy mode shape |
| **4 — Policy + drift** ⭐ | Policy-as-code, reconcile across both surfaces, drift, baselines | `policy` engine, baselines library, drift observations | — |
| **5 — Audit + observability** | One normalized audit stream (OTel GenAI) + SIEM export | `audit` package, audit normalizers per source, export | observation contract |
| **6 — Hardening** | Durable, authenticated | Postgres store, real `identity`/`secrets` | — |
| **7 — Gateway (MVP shipped 2026-07-02, optional)** | Runtime enforcement of residual controls (per-request PII/PHI redaction; hard cross-vendor caps still ahead) | separate `cmd/air-traffic-gateway` binary — **MVP shipped**: reverse proxy (`POST /v1/messages`, Anthropic dialect) + inline regex/Presidio detection + mask/tokenize redaction + credential broker + spine push/pull, driven by the `internal/harness` eval/ratchet loop. Remaining [Build Plan](./inference-gateway-build-plan.md) milestones (Redis token vault, cross-vendor hard caps, recall ratchet at scale, horizontal-scale hardening) are future | — |

**Critical path:** Phases 2–4 are the differentiated core. The config distributor (Phase 2) is the
highest-value milestone — it is what makes Air-Traffic an *agentic* control plane and not just a
vendor-API dashboard. The gateway (Phase 7) is optional and off the spine; its MVP slice — Build
Plan `G0→G1→G2→G7` — **shipped 2026-07-02** as a single-replica binary (proxy + detect/redact +
credential broker + spine integration, exercised by the harness), ahead of any of the recall/scale
work, which remains.

---

## 15. Appendix: Repository Layout

```
air-traffic/
├── cmd/
│   ├── air-traffic-server/
│   │   └── main.go                 # control plane: config + lifecycle + harness wiring
│   └── air-traffic-gateway/        # SHIPPED 2026-07-02 — separate stateless data-plane binary (§11.1)
│       └── main.go                 #   own config.Load(); control-plane binary never imports gateway
├── internal/
│   ├── catalog/                    # ⭐ vendor/platform registry, manifests, cost facets
│   │   ├── catalog.go
│   │   ├── vendors.go
│   │   └── cost_facets.go
│   ├── synthetic/                  # per-vendor native-shaped surfaces under /synthetic/{vendor}/…
│   ├── envconfig/                  # ⭐ render + distribute managed config; read state
│   │   └── envconfig.go
│   ├── policy/                     # ⭐ reconcile + drift + baselines (types live in model/policy.go)
│   │   ├── reconcile.go
│   │   ├── drift.go
│   │   └── baselines.go
│   ├── harness/                    # ⭐ SHIPPED eval/ratchet harness (drives the gateway)
│   │   ├── gen.go                  #   corpus generation
│   │   ├── runner.go               #   run orchestration
│   │   ├── streamscan.go
│   │   ├── persist.go              #   durable state under AIRTRAFFIC_DATA_DIR
│   │   ├── sample.go               #   try-a-prompt
│   │   ├── probe.go                #   Presidio raw-score probe
│   │   ├── flywheel.go             #   improvement loop → G6 config-knob proposals
│   │   └── score.go
│   ├── emitter/                    # ops-observation-batch/v1 generator (mirrors it-scorecard)
│   ├── audit/                      # ⭐ normalized audit stream (OTel GenAI semconv)
│   ├── store/                      # in-memory state only (store.go, gateway.go)
│   ├── model/                      # domain types + contract (contract.go, gateway.go, harness.go, policy.go, presidio.go, model.go)
│   ├── redact/                     # secret/PII redaction (log path)
│   ├── server/                    # HTTP mux: /api/*, /api/gateway/*, /api/harness/*, /synthetic/*
│   └── gateway/                    # SHIPPED inference-gateway data plane (§11) — its own binary
│       ├── proxy.go                #   reverse proxy + streaming
│       ├── adapter_anthropic.go    #   Anthropic Messages dialect
│       ├── server.go               #   POST /v1/messages, /healthz, /readyz
│       ├── spine_emit.go           #   observations + leak findings + heartbeat ↑
│       ├── spine_pull.go           #   policy + pattern pack ↓
│       ├── metrics.go              #   audit.go, health.go, stream.go alongside
│       ├── config/                 #   config.Load() + validation (GATEWAY_* env)
│       ├── credbroker/             #   credential broker (ref → real vendor key)
│       ├── detect/                 #   Detector iface + regex + Presidio adapter
│       └── redact/                 #   mask + reversible tokenize
├── schemas/
│   └── ops-observation-batch-v1.schema.json   # reused verbatim
├── web/                            # React SPA (Flight Deck + /welcome landing + /settings/* surfaces)
│   └── src/
│       ├── pages/{FlightDeck,RigorConsole,PolicyEditor,CostExplorer,Vendors,Observability,Audit,GatewayHarness,Welcome}.tsx
│       ├── pages/landing/          # Hero, Planes, VendorWall, Dispositions, HowItWorks, CtaBand
│       ├── pages/Welcome.test.tsx  # vitest ("test": "vitest run")
│       ├── components/  ·  lib/  ·  test/
└── docs/
    ├── air-traffic-analysis.md           # research + product spec
    ├── air-traffic-system-design.md      # this document
    ├── inference-gateway-design.md       # OPTIONAL gateway — vendor-neutral design & feasibility
    └── inference-gateway-build-plan.md   # OPTIONAL gateway — sequenced, horizontally-scalable build
```

---

*Companion to [`air-traffic-analysis.md`](./air-traffic-analysis.md). Spine = vendor-API control +
environment config distribution + observability; inline inference gateway is an optional module.
Architecture anchored on the `it-scorecard` connector/emitter pattern. Revised July 2026 (gateway MVP + harness shipped).*
