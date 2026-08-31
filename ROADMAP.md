# air-traffic — ROADMAP

> ⭐ **SINGLE SOURCE OF TRUTH.** On any handoff or fresh session, **read this first and follow
> only this** for what's left, what's next, phases, acceptance criteria, and decisions. There are
> **no other `*_PLAN` / handoff docs** — the ones that existed were consolidated into this file
> (2026-08-07). If another doc's status ever conflicts with this one, **this wins.**
>
> - **Strategy / the "why"** (a different layer, not an execution plan): `docs/air-traffic-analysis.md`,
>   `docs/air-traffic-system-design.md`, `docs/air-traffic-executive-overview.html`,
>   `docs/air-traffic-claude-code-github-copilot.html` (portfolio briefs — RECORDS, point-in-time by design).
> - **Reference / spec** (opened on demand, never as "the plan"): `docs/inference-gateway-design.md`
>   (design), `docs/inference-gateway-build-plan.md` (sequencing), `docs/inference-gateway-eli5.md`
>   (referenced by `scripts/capture-harness-screenshot.js`). **Live deferred-work ledgers, kept
>   separate deliberately** because code and UI cite them by path: `docs/plans/TODO-gateway-deferred.md`
>   (cited from `internal/gateway/credbroker/credbroker.go:35`, `internal/server/routes_gateway.go:126`,
>   `internal/model/gateway.go:47`, and README) and `docs/plans/TODO-vendor-auth.md` (cited from
>   `internal/store/store.go:78`, `web/src/lib/authSchemas.ts:3`, and rendered live in
>   `web/src/pages/Vendors.tsx:311`). Also kept separate as build-history records (linked from
>   README's "Plans:" line): `docs/plans/phase-1-surface-collection.md`,
>   `docs/plans/phase-2-frontend.md`, `docs/plans/phase-3-inference-gateway.md`.
> - **Decisions**: `DECISIONS.md` (append-only log; roadmap items cite it by date + title)

> **Closed work is not in this file.** An item is deleted at the edit that closes it — there is no
> ✅ status. What shipped is recorded by the closing commit. To resurrect or
> cite a deleted item: `git log -S'<ID>' -- ROADMAP.md`, then `git show <sha>:ROADMAP.md`.

**Legend:** ⏳ in progress · ⬜ not started · 🔶 shipped but UNVERIFIED · 🔬 verification owed ·
⛔ **BLOCKS** — the only marker that gates anything. Superseded/rejected work survives only as a
one-line struck entry in the open-decisions index.

**Backlog items are not blockers.** No item under a `BACKLOG` / `PARKED` status may be cited as
gating, blocking, or holding up any other work unless it carries a `⛔ BLOCKS:` line with the
owner's verbatim instruction. Absent that line, treat it as non-blocking. An agent that reports a
parked item as a blocker is misreading this file.

**Contents:** §0 Do next · §1 Phase 1 — control plane backend · §2 Phase 2 — frontend SPA ·
§3 Phase 3 — inference gateway + harness + flywheel · §4 Cost & usage drill-down · §5 Per-vendor
auth schemas · §6 Open-decisions index · §7 Non-expert pivot (honesty, blast radius, detection,
budget) · Appendix

---

## §0 Do next

> ### ▶ RESUME HERE — session handoff 2026-08-16 (gap-closure pass: PIVOT-1, auth, persistence)
>
> **State:** everything below was **built and verified this pass** — full toolchain present
> (go 1.26.5 then; `go.mod` has since pinned toolchain go1.26.7 to clear reachable stdlib CVEs,
> 6c2d936 — node 24.14.1, docker 29.5.3). `gofmt` clean, `go vet` clean, `go test -race ./...`
> green, `npm run build` + the full **vitest suite** green (61 tests across 10 files at last count,
> 2026-08-31; was 18 across 3), `E2E_COMPOSE=1 ./scripts/e2e-gateway.sh` **9/9** (recall_behavioral
> 1.000, precision 0.991, trap FPs 0), compose stack rebuilt and healthy. The Rigor Console was
> also **opened in a real browser** (headless Chromium) and its screenshots are in `docs/images/` —
> that is the OWED-2 class of verification, done for this change at least.
>
> **▶ NEXT ACTION:** §7.2's four cheap instrumentation fixes (PIVOT-2 → PIVOT-5) are now the
> strongest candidates — PIVOT-2 especially, since every rate in §7.4 is computed over a
> denominator missing an entire failure class until it lands. §3's deferred G-blocks, §4's vendor
> cost facets and §5's 10 vendor auth schemas remain open and still block nothing.
>
> **Owed / explicitly NOT done this pass — read this before claiming any of it is closed:**
> - **G4 response-side enforcement is still not built** and was not attempted. It is deferred by
>   design (`docs/plans/TODO-gateway-deferred.md`, README): responses relay byte-faithfully and a
>   streamed leak is scored informationally, not blocked. A gap scan will keep finding it; it is a
>   feature, not a defect.
> - **PIVOT-13 is NOT closed.** `pullPolicy` now *logs* when the control plane reports no policy
>   while this gateway is still enforcing. Persistence removes the common cause; the drift record
>   that would make the disagreement first-class is still open.
> - **Five Tier-2 fixtures are still missing** (M365 Copilot, Databricks, Perplexity, Cohere,
>   Together) — see §1 PHASE1-3. The README was corrected to stop claiming otherwise; the code was
>   not changed. Building them needs real vendor API documentation, not recall.
> - **A used stack no longer boots clean:** the applied policy write-throughs to `policy.json` in
>   the `harness-data` volume, so once any policy has been applied the stack comes back enforcing
>   it. Durable persistence is the point; the consequence is that there is no longer a "no policy
>   applied" starting state short of dropping the volume.
> - The **Gateway Traffic** page and the Gateway Harness `allow_list` chip remain browser-unverified
>   (OWED-2). Only the Rigor Console was opened this pass.
> - **Observations/reports store is still in-memory** (carried from the GATEWAY-5 pass,
>   2026-08-15) — `policy.json`, the keystore (`keys.json`) and the harness flywheel state
>   (`ratchet.jsonl`, `corpus/*.json`, `patterns.json`) are what persist across restarts.

---

## §1 Phase 1 — Control plane backend

- ⬜ **PHASE1-3** **Five Tier-2 vendors still answer from the generic fixture.**
  `internal/synthetic/fixtures_t1.go:10-18` registers dedicated byte-identical fixtures for the six
  Tier-1 vendors plus `mistral`; **M365 Copilot, Databricks, Perplexity, Cohere and Together** fall
  through to `genericFixture` (`internal/synthetic/errors.go:97`). The README claimed all six
  Tier-2 vendors were at "core surfaces" fidelity; it was corrected 2026-08-16 to separate
  *manifest depth* from *replica fidelity* rather than write five envelopes from memory
  (`DECISIONS.md` 2026-08-16). Build each against real vendor documentation — Graph `value[]` for
  M365, `/api/2.0/serving-endpoints` + `system.billing.usage` for Databricks, etc. Per-vendor error
  envelopes and the emitted signal are already real for all sixteen; only the success-path bodies
  are generic. Overlaps §4 COST-3, which needs the same vendors' billing envelopes.
- 🔬 **OWED-3** The phase-1 doc's acceptance checklist (`docs/plans/phase-1-surface-collection.md`
  §13) uses `- [ ]` checkboxes never flipped to `[x]` even though the code shipped — nobody ever
  re-ran `go test ./... ` against that specific checklist to close it out formally. Low priority
  (code + README are strong secondary evidence) but never confirmed line-by-line.

---

## §2 Phase 2 — Frontend SPA

- ⬜ **PHASE2-3a** `web/src/pages/landing/Hero.tsx` (~`:30-40`): the "spine online" and
  "emitter · ops-observation-batch/v1 · 5s" badges pulse unconditionally with zero data binding
  and cannot go false — a violation of that page's own Correctness axis. Bind them to
  `/api/gateway/status` and the observation feed, or make them visibly static. The bar is that
  `/welcome` must not lose visual quality in the process. (This page was last shaped through a
  local `/ratchet-up` ledger under `.claude/ratchet-up/`, now gitignored and gone from the working
  tree but still in the published history, since it was tracked until the release-prep commit —
  see `DECISIONS.md` 2026-08-15 "Flight Deck: an element may not claim liveness it cannot
  disprove", scope ruling 2.)
- ⬜ **PHASE2-3b** Residual, non-blocking: `web/src/components/VendorGlyph.tsx` brand hues for
  bedrock/mistral/m365/groq (`#FF9900`, `#FA520F`, `#D83B01`, `#F55036`) sit in the same band as
  `--amber`/`--unverified`, so a decorative glyph can camouflage a real status dot in the same
  row. Mitigated (off-row dimming + glow now conditional), not solved; a full palette rework was
  deliberately not taken because `VendorGlyph` is shared with the ratcheted landing page.

---

## §3 Phase 3 — Inference gateway + harness + flywheel

- 🔶 **GATEWAY-5** (2026-08-15) OpenAI-compatible dialect + per-request traffic feed.
  `POST /v1/chat/completions` ships beside `/v1/messages`, both running the *same* pipeline
  behind a `dialect` descriptor (`internal/gateway/proxy.go`) so detector/policy changes can't
  drift between them; new `internal/gateway/adapter_openai.go` field walk, OpenAI SSE usage
  scanner, OpenAI error envelope, and a `GATEWAY_UPSTREAMS[route].auth` knob (`x-api-key` |
  `bearer`). Control plane gained `GET /api/gateway/requests` and a **Gateway Traffic** page
  (`/settings/traffic`); `GatewayRequestReport` gained `model` / `tokens_in` / `tokens_out`.
  Full detail + what stays deliberately unwalked: `docs/plans/TODO-gateway-deferred.md`
  "Closed 2026-08-15". Decisions: `DECISIONS.md` 2026-08-15.
  **Proven live 2026-08-15** against the real Hugging Face router with both sibling apps routed
  through it, token counts reconciling exactly against meaning-to-making's independent cost store
  (see §0). **Still 🔶 not ✅ because:** the Anthropic route has still only ever talked to the
  synthetic replica, and the Gateway Traffic page has not been opened in a browser.
- 🔶 **GATEWAY-6** (2026-08-15) Flywheel suppression: `allow_list` pattern-rule kind +
  `POST /api/harness/proposals` for owner-authored proposals. The flywheel could only ever
  raise recall; nothing could retire a false positive, and a score gate provably cannot (spaCy
  returns 0.85 for every PERSON/LOCATION, real or not — measured). Applied in `Chain.Run` beside
  `typeGuards` so it overrules any engine; scoped by type; suppresses a term, never a region.
  Full detail + the live before/after: `docs/plans/TODO-gateway-deferred.md` "Closed 2026-08-15
  (second pass)". Decisions: `DECISIONS.md` 2026-08-15 (two entries).
  **Marked 🔶 not ✅ because:** the new Gateway Harness chip rendering for `allow_list` is
  build-verified only, not browser-verified (same standing gap as OWED-2).
- ⬜ **GATEWAY-3** Deferred G-blocks — full detail lives in `docs/plans/TODO-gateway-deferred.md`
  (kept separate, code-referenced; not duplicated here). Summary pointer only:
  - G3 (reversible tokenize + Redis vault), G4 (async monitor + response-side enforcement),
    G5 (tokenization oracle, needs G3) — not started.
  - G6 remainder (managed DLP adapters: Comprehend Medical / Google DLP / Azure PII) — not started.
  - G8 remainder (OTel trace-span depth) — not started; log-leak CI guard already shipped.
  - **G9 (horizontal scale)** — `🔬 OWED-4`: **blocked by a scope decision, not effort.** A
    Redis-backed budget counter needs a Redis client, and this repo is stdlib-only with CI that
    fails the build if a `go.sum` appears. Accepting the first third-party dependency vs. hand-writing
    a RESP client is an owner call — see `docs/plans/TODO-gateway-deferred.md` "G9" for the full
    argument. **Not a blocker on anything else** (`Not a blocker.`) — no owner `⛔ BLOCKS:`
    instruction exists for it.
  - G10-full (LLM-judge labeling, Synthea generation, shadow mode) — not started; no-GPU local
    ceiling is a deliberate design constraint, not a gap.
- ⬜ **GATEWAY-4** Cosmetic: `web/src/lib/dispositions.ts`'s `proxy_enforced` blurb still says
  "off by default" while the Policy Editor's outcome chip already says `applied_proxy` truthfully
  — noted as a slice-local deferral in `docs/plans/TODO-gateway-deferred.md`, never picked up.

---

## §4 Cost & usage drill-down

Folded from `docs/plans/TODO-cost-drilldown.md` (staged to purgatory 2026-08-07 — content below
is the full remaining scope, nothing lost; original recoverable per Appendix).

- ⬜ **COST-3** Byte-identical synthetic grouping still owed for (emitter drill-in already works
  for all of these via the catalog — only the *replica* endpoint is missing):
  - **Azure OpenAI** — `Microsoft.CostManagement/query` case (`{properties:{columns:[],rows:[]}}`)
    for cost dims + token grouping (ModelName/ModelDeploymentName/Region).
  - **AWS Bedrock** — Cost Explorer `GetCostAndUsage` (`{ResultsByTime:[{Groups:[{Keys,Metrics}]}]}`)
    + CloudWatch `GetMetricData` (`{MetricDataResults:[{Label,Values}]}`, ModelId).
  - **Vertex** — multi-series `timeSeries` by label (`resource.labels.model_user_id` /
    `resource_container` / `location` / `endpoint_id`) + a BigQuery-billing-export rows envelope
    for SKU + team cost dims.
  - **Mistral** — workspace is a filter, not a group_by; align the fixture to the real
    `/v1/admin/usage` service-category envelope (currently a plain object:list/data wrapper).
  - **Tier-2/3** (m365_copilot, databricks, amazon_q, watsonx) — register fixtures mirroring
    their real billing shapes (Graph `value[]`, `system.billing.usage` rows, Cost Explorer, IBM
    Cloud Usage Reports). `perplexity`/`cohere`/`groq`/`xai` have no real server-side group-by —
    `supported[]` stays empty, honestly.
  - Backing research: a 24-agent study (real group-by params, endpoints, response fields,
    verify-corrected) — not re-derivable from git alone; the study output itself wasn't committed,
    only its conclusions (the facet table + this list).

---

## §5 Per-vendor auth schemas

Live ledger stays at `docs/plans/TODO-vendor-auth.md` (code + UI referenced — see roadmap
callout). Pointer only:

- ⬜ **VENDOR-2** 10 remaining vendors (Mistral, Databricks, Perplexity, Cohere, Together, Groq,
  xAI, Amazon Q, IBM watsonx, M365 Copilot) ship **disabled** and fall back to URL-only config
  until their schema is built. Suggested shape per vendor + build steps: full detail in
  `docs/plans/TODO-vendor-auth.md` — not duplicated here since that file is the one the UI cites.
  **Reminder carried forward:** the enabled/disabled roster is the owner's to set. An agent never
  enables or disables a vendor on its own — toggling is an explicit operator action
  (`PATCH /api/adapters/{id}`), which is how the code states it (`internal/store/store.go:76-79`).

---

## §6 Open-decisions index

- 🔬 **OWED-1** — Did the "demo moment" (owner clicks Approve on `deny-PERSON_NAME`, re-runs to
  watch the ratchet recover) ever happen? Asked in the 2026-07-02 handoff as next-session item 1;
  no later commit or doc confirms it. Not answered — do not treat as closed.
- 🔬 **OWED-2** — Was the Gateway Harness tab ever visually verified in a real browser (kind
  chips, deny-term/threshold rendering, try-a-prompt panel)? The 2026-07-02 handoff explicitly
  flagged this as "STILL never visually verified — build/API-verified only." No later doc or
  commit message confirms a browser click-through happened since. Not answered.
- 🔬 **OWED-3** — see §1: phase-1 acceptance checklist never formally re-run/checked off against
  current code.
- ~~**OWED-4** — G9 blocked on Redis-vs-hand-rolled-RESP-client dependency decision~~ **killed
  2026-08-15:** answered — no third-party dep; G9 stays deferred, must not bundle with §7.5.
- ~~**GATEWAY-7a** — no keystore UI; issuance stays loopback-only by design~~ **killed 2026-08-16:**
  superseded — admin-key tier built, gates every write route.
- ~~Persisting policy without fixing PIVOT-13's split-brain revert~~ **killed 2026-08-16:**
  reasoning inverted — persistence removes the disagreement, PIVOT-13 detector still open.

---

## §7 Non-expert pivot — honesty, blast radius, detection, budget

> **Provenance, read this before citing anything below as binding.** Two items are
> **user-ratified** (2026-08-15, answered directly): the single-operator/admin-key posture and
> policy-persistence-only. Both have `DECISIONS.md` entries. **Everything else in this section is
> `CLAUDE-ORIGIN`** — agent-authored from a six-lens analysis pass on 2026-08-15, not decided by
> the owner. Ranking, thresholds, and effort estimates are proposals. Re-derive before treating any
> of it as a requirement; do not cite one of these items back as settled because it appears here.
>
> **Driver (owner's words, 2026-08-15):** pivot the app "to be helpful [to] even those who don't
> understand these things"; identify bottlenecks in settings and "potentially auto heal, or alert
> to adjust parameters"; **per-user budget tuning, GitHub Copilot first** (paraphrased, not
> quoted); consider "administrative lift, and the risk of a user not understanding and opening the
> wrong thing"; and "different modes. Expert tuning versus guided."
>
> **Tiers are ordering constraints, not preferences.** A later tier shipped before an earlier one
> is dishonest, not merely early — each tier supplies the evidence the next one's claims rest on.

### §7.0 The finding that frames the rest

The gateway is real. **The control plane is a simulator.** `internal/policy/reconcile.go:67-85`
`classify()` is a `switch` on a compile-time constant — `applied_native` is asserted for all 16
vendors with no vendor call ever made. `internal/policy/drift.go:101-110` `overrideHeuristic` is
`hash(id) % 5 == 0`, so the `env_managed` half of drift is deterministic fiction while the gateway
half is real, and **both write `model.DriftRecord` with no field distinguishing them.** Every
dollar, token, latency and cap-utilization figure comes from `internal/emitter/emitter.go:94`
(random walk) fanned across static facet weights. The control plane has never made an outbound
vendor call.

Consequence for the pivot: an expert reads `mode: "synthetic"` and knows; a non-expert reads
"All vendors under 80% of cap." **The only surfaces safe to hand a non-expert today are the
Gateway Traffic page and the keystore.**

### §7.2 Tier 0 — instrumentation preconditions (cheap, no decision needed)

- ⬜ **PIVOT-2** **Record the nine silent exits on the proxy path.** Counted 2026-08-31,
  `s.record()` fires at 3 of the 12 ways a request leaves `internal/gateway/proxy.go`: the
  fail-closed detector abort, the policy block, and the success path. Auth failure (in
  `requireClientKey`, one frame up), no upstream for the route, oversized body, bad JSON,
  mask-rewrite failure, credential resolution failure, unusable upstream base URL, request-build
  failure and upstream unreachable all return with zero rows and zero metrics (`metrics.observe`
  is reachable only via `record`, `internal/gateway/audit.go:66-67`). Those counts are as-of, not
  live — grep `d.writeErr(` for the current set. The heartbeat keeps beating on its own timer
  claiming enforcement throughout. **A route failing 100% of requests is indistinguishable from
  an idle one** — and compose ships `HF_UPSTREAM_TOKEN` with no default by design
  (`DECISIONS.md` 2026-08-15), so that is the likeliest real failure. Lands FIRST: every rate in
  §7.4 is otherwise computed over a denominator missing an entire failure class.
- ⬜ **PIVOT-3** **Heartbeat carries effective action + pulled policy/pack/keystore versions + a
  `detector_ran` fact.** `model.EnforcementReport` (`internal/model/gateway.go:59-66`) carries no
  versions, so a gateway stuck on a stale snapshot is indistinguishable from a current one.
  Unblocks PIVOT-9, PIVOT-10 and the pack-version join in one change.
- ⬜ **PIVOT-4** **Fix hardcoded vendor attribution.** `internal/gateway/spine_emit.go:97` and
  `:133` hardcode `anthropic` regardless of route: every gateway *aggregate* is attributed to
  Anthropic, and the `openai` adapter can never reach `applied_proxy`
  (`internal/policy/reconcile.go:74`). The per-request feed is correct; the aggregate is not.
- ⬜ **PIVOT-5** **Collapse the duplicated staleness constants.** `gatewayStaleAfter = 45s` is
  hardcoded twice independently (`internal/policy/reconcile.go:15`,
  `internal/server/routes_gateway.go:17`) and `heartbeatInterval = 15s` a third time
  (`internal/gateway/spine_emit.go:19`), none derived from a shared constant or from
  `GATEWAY_POLICY_PULL_INTERVAL`. Three-way divergence risk.

### §7.3 Tier 1 — blast radius (the "user opens the wrong thing" ask)

- ⛔ **PIVOT-7** **Pattern-pack retraction path — BLOCKS all pack automation.** `ApproveProposal`
  only appends (`internal/harness/flywheel.go:670`); `allow_list` is the one kind that *removes*
  detection; approval is permanent. A stale suppression (`manual-person-m2m-interests`) already
  sits in the running pack for a client that no longer exists, unretirable without wiping the
  volume and destroying the ratchet history. Spec already written:
  `docs/plans/TODO-gateway-deferred.md:36-40`.
  **⛔ BLOCKS:** nothing automated may touch the pattern pack until this exists — *including
  auto-authoring*, because a flood of auto-authored proposals into a UI with a one-click Approve is
  an auto-approver with extra steps. (This marker is `CLAUDE-ORIGIN` reasoning, not an owner
  instruction — it gates only §7.4's PIVOT-9 remedy path, which is itself agent-proposed.)
- ⬜ **PIVOT-8** **Make an `allow_list` Approve look different from an additive Approve.** Today the
  irreversible button is byte-identical in size, colour and position to the reversible one
  (the `decide(p, 'approve')` button in `web/src/pages/GatewayHarness.tsx`, `:464` as of
  2026-08-31), under a header reading *"human-approved only — nothing auto-applies"* —
  reassurance where a warning belongs.
- ⬜ **PIVOT-8a** **Surface that the Vendors "Enabled" toggle blinds drift detection.**
  `internal/policy/drift.go:17` and `:56` exclude disabled adapters from *both* drift loops, so
  flipping one of sixteen bare switches silently stops divergence detection for that vendor with no
  indication anywhere.

### §7.4 Tier 2 — bottleneck detection

Ruling that shapes this whole tier: **cost is not a detectable axis, because no setting controls
cost.** `OrgCapUSD`, `UserCapUSD`, `RetentionDays`, `ModelAccess`, `ContentSafety`,
`TrainingOptOut` and `Policy.Budget` are declared (`internal/model/policy.go:11-29`) and never read
by any non-test Go code; only `PIIRedaction`, `ZDR` and `BAAOnly` are load-bearing. The honest axes
are **latency and success**.

- ⬜ **PIVOT-9** **Invariant redaction fingerprint** — the highest-value detector found.
  Fire on `Action=="block"` ∧ every redaction `Path` prefixed `system` or `tools[` ∧ N≥10
  consecutive blocks from one `AppID`. The path-provenance half is decisive:
  `walkAnthropicBody` emits `Path=="system"` / `system[N].text` / `tools[N].description`
  (`internal/gateway/adapter_anthropic.go:40,43,69,87`), so a redaction on those paths is *by
  construction* in developer-authored text, not user input.
  **Validated against the canonical incident:** existing drift misses (heartbeat fresh, action
  `block` = enforcing — enforcing *was* the failure). The harness misses **provably** — it scored
  9/9, recall 1.000, precision 0.991 *during* the outage, because it scores generated corpus and
  never the app's real prompt. The flywheel misses by design. This catches it in ~10 requests + one
  5s push, and can name the exact field path and span length **without ever having seen a value**.
  Remedy is PROPOSE-only and is gated by PIVOT-7. Hard limit: the report carries no values by
  design (`internal/model/gateway.go:5-6`), so an auto-authored proposal cannot populate
  `AllowList` — it can only state the location and ask a human to open that prompt.
- ⬜ **PIVOT-10** **Per-app block-rate saturation** — block rate ≥0.9 over N≥20 per `AppID` from
  `store.ListGatewayReports` (`internal/store/gateway.go:82-90`). **Must not ship with any
  auto-remedy before PIVOT-9 exists**: alone it cannot separate "misconfigured" from "correctly
  blocking an app that genuinely sends PII", so an auto-unblock fires precisely when an app is
  sending the most PII.
- ⬜ **PIVOT-11** **App-baseline substitution** — an app naming a baseline the gateway hasn't pulled
  falls through to the global action and only logs (`internal/gateway/proxy.go:123-126`), while the
  report's `Baseline` carries the *global* id. Exact string comparison against authoritative local
  keystore state; near-zero false-positive risk.
- ⬜ **PIVOT-12** **Fail-open unenforcement → the one justified auto-apply.** Under
  `GATEWAY_FAIL_MODE=open` a dead Presidio forwards traffic unfiltered while `pushHeartbeat` keeps
  claiming `pii_redaction` — it consults only `enforces(action)` and `allAppsEnforce()`
  (`internal/gateway/spine_emit.go:132-134`) and never asks whether the chain actually ran. Drift
  structurally cannot see this. What auto-applies is **honesty, not policy**: retract the
  enforcement claim. It changes no enforcement behaviour; it withdraws an assertion the system
  cannot substantiate. Sole condition satisfying all three safety properties — it is a *retraction*
  not a novel state, it moves fail-*closed*, and its signal is a direct fact rather than an
  inference over a distribution.
- ⏳ **PIVOT-13** **Split-brain policy detector — HALF DONE 2026-08-16, do not read as closed.**
  Two of the three legs changed: the applied policy now **persists** (`internal/store/policy_persist.go`,
  loaded at boot, verified across a real `docker compose restart control-plane`), so the common
  cause — the control plane forgetting on restart — is gone; and `pullPolicy` now **logs** the
  disagreement when the control plane reports no policy while this gateway is still enforcing.
  **Still open:** turning that into a first-class `model.DriftRecord` so it appears on `/api/drift`
  rather than only in a log line, and the gateway-restart case that falls to `actionMask`. The
  §7.7 ruling that persistence must not ship without the revert fix is satisfied in substance —
  persisting *is* what stops the disagreement being durable — but the detector itself is not built.
  Original finding, unchanged:
  but `pullPolicy` returns early on `Policy == nil` **without clearing `s.policyAction`**
  (`internal/gateway/spine_pull.go:110-112`) — the gateway keeps enforcing while the control plane
  believes nothing is applied, and nothing compares them. `gatewayDrift`'s `expected` flips false
  (`internal/policy/drift.go:49-52`), *silencing the one check that would have noticed*. When the
  gateway later restarts it falls to `actionMask` (`proxy.go:158`) — a third posture matching
  neither belief. Detection needs no persistence: policy nil + a gateway reporting a non-default
  enforcing action = one comparison, one drift record. Pairs with the policy-persistence work
  ratified in `DECISIONS.md` 2026-08-15 — **persistence and heartbeat version-reporting land
  together or not at all**, since persisting without fixing the revert semantics makes the
  disagreement durable instead of transient.

### §7.5 Tier 3 — per-user budget tuning (Copilot first)

- ⬜ **PIVOT-14** **Provenance enum on the observation contract + typographic rendering.** Gates any
  real number entering the UI. `web/src/lib/fleet.ts:150` **sums** `cost_usd` across vendors, so one
  real Copilot figure gets added to fifteen fabricated ones — **the lie is the total, not the row.**
  Colour is fully spent (severity / disposition / vendor identity), so the signal must be
  typographic: measured renders as today, simulated renders italic-muted. A mixed-provenance
  aggregate renders as **two numbers, never a sum**. Also: `web/src/lib/costExport.ts:10-11`'s
  snapshot note caveats *staleness*, not *fabrication* — the wrong caveat, and its rigorous tone
  makes a fabricated CSV read as more credible, not less. Provenance must be a **column per row**,
  not a header comment.
- ⬜ **PIVOT-15** **Copilot read-only budget posture view** — the smallest honest first slice, and
  the first real dollar figure in the app. Three real reads:
  `GET /orgs/{org}/copilot/billing/seats` (roster — **discovered** identity),
  `GET /organizations/{org}/settings/billing/ai_credit/usage?user=` (per-user spend),
  `GET /organizations/{org}/settings/billing/budgets` (current caps). No write path.
  - **Never join vendor identity to gateway `Subject`.** `APIKey.Subject`
    (`internal/model/keystore.go:41-49`) is owner-typed free text with no uniqueness constraint and
    is explicitly not verified identity; Copilot's user-scope budget *requires*
    `prevent_further_usage: true`, so a mistyped join hard-stops the wrong engineer with no error
    either way. Vendor identity must be **discovered** from `seats[].assignee.login` /
    `actor.email_address`, never typed — which makes the wrong-person hard stop unrepresentable
    rather than merely unlikely. A display-only link is acceptable in the read direction
    (vendor principal → subject) and must never be read when constructing a budget write.
  - **Key the money path on the billing endpoint**, which is *not* subject to the 5-active-user
    privacy floor — that floor applies to the metrics reports only, and is evaluated per org **and
    per team**, so small teams inside a large org silently drop days. Use metrics for soft context
    (IDE, model, language, acceptance) and render a withheld day as **"withheld", never 0** — a
    withheld day shown as zero is an idle user who isn't idle, which is the input to a seat reclaim.
- ⬜ **PIVOT-16** **Tuning-loop semantics, when it eventually writes.** Clock is the **billing
  period**, not the tick: trailing 3 *complete* periods, last 48h excluded as incomplete, at most
  one write per principal per period, dead-band ~25%. **Never write a cap below current
  period-to-date spend** — the single comparison that prevents the catastrophic case (an engineer
  burns 40% of a month on one agentic refactor on the 4th; a cap computed from quiet months lands
  below their MTD; `prevent_further_usage: true` removes Copilot for 26 days, instantly). Raises:
  auto-apply inside a pre-ratified envelope (**the envelope's numbers are an owner decision, not a
  design**). **Lowers and first-ever caps: PROPOSE-only, always** — on Copilot, lowering is not a
  budget adjustment, it is scheduling a named engineer's outage. Durable state is *decisions*, not
  history: an append-only `budget-decisions.jsonl` in `AIRTRAFFIC_DATA_DIR`, the same shape as
  `appendRatchet` (`internal/harness/persist.go:43-51`). Vendor history is read on demand, never
  mirrored — a second drifting copy of billing data is both a support burden and a wrong input to a
  hard stop.
- 🔬 **PIVOT-17** **Stale catalog entries, verified against live vendor docs 2026-08-15.**
  `internal/catalog/vendors.go` states Copilot has "no native cap on AI credit spend" and that
  Anthropic's workspace spend limit is "Console-only; no SET endpoint". Both were true when written
  and are **wrong as of June 2026**: GitHub moved Copilot to token-based AI Credits on 2026-06-01
  and shipped a user-scoped Budgets API (`budget_scope: "user"`, `prevent_further_usage: true`) on
  2026-06-04; Anthropic shipped a per-user Spend Limits API where `scope.type: "user"` is the only
  accepted scope (Enterprise plan + usage credits required — **verify with one call to
  `/v1/organizations/spend_limits/effective` before designing anything; a 403/404 answers it**, and
  if it fails, Claude is PROPOSE-only by plan rather than by design choice). Not edited — that pass
  was scoped analysis-only. Worth date-labelling catalog notes generally: one lens reached a wrong
  conclusion by trusting this comment as current.

### §7.6 Tier 4 — Expert vs Guided

- ⬜ **PIVOT-18** **Guided is a reading mode, not a simplified app.** Per `DECISIONS.md` 2026-08-15
  (single-operator ratified), it is a **view preference, not a permission model** — it protects
  against a slip, not against a user, and must be described that way in the UI and never as a
  safety boundary.
  Proposed rule: Guided-safe iff **bounded** (one named object) ∧ **previewable** (resulting state
  renderable pre-commit) ∧ **reversible in view**. Applied honestly that yields **one write control
  in the entire product** (the Vendors Enabled toggle) — the correct output of the rule, not a
  failure of it.
  - Guided pages: Gateway Traffic; Audit minus the disposition column (it imports a 6-term taxonomy
    into the only page that otherwise needs no vocabulary); Flight Deck reworked; Rigor Console
    **read-only** — its `NARRATIVE` block (`web/src/pages/RigorConsole.tsx:18-55`) is the best
    non-expert writing in the app, so keep the page and move the button.
  - Expert-only, do not attempt to simplify: Policy Editor (`proxy_enforced` is a *conditional*
    truth — `web/src/lib/dispositions.ts:24` — and any simplification rounds it up to "protected",
    the exact lie the taxonomy exists to prevent); Vendors (a test-fixture driver wearing an admin
    console's clothes — it offers `500` / `timeout` / `invalid-json`); Observability; Gateway
    Harness (eight prerequisite concepts, and it holds the only irreversible control).
  - **Guided must not:** show "Healthy %" without the emitting denominator in the headline
    (`web/src/pages/FlightDeck.tsx:104-106` — dropping sub-labels re-creates the exact bug
    `DECISIONS.md` 2026-08-15 was written to kill); show spend against a cap
    (`FlightDeck.tsx:15` and `web/src/pages/CostExplorer.tsx:13` each hardcode `50000`, which is
    *fintech's* number, rendered against every baseline — under healthcare/gov the real cap is `0`,
    i.e. none); drop the stale/degraded treatment; collapse dispositions into
    "protected / not protected"; or show a block count without the posture that produced it.

### §7.7 Explicitly not to be built (kept here so a later pass doesn't re-propose it)

- Any detector over emitter output — a "cost anomaly" alarm on random-walked numbers trains the
  operator to ignore alerts, and is precisely what the honesty model exists to prevent.
- Any alarm over `/api/drift` without filtering to `Surface == proxy_enforced` — it will fire on
  `hash % 5` rows. Drift is not automatically honest just because it is not the emitter.
- A per-user "low acceptance rate" flag — a productivity judgement about a named person, derived
  from an API whose 5-user floor exists specifically to prevent per-person surveillance below that
  threshold. Cohort statistic only, if at all. **Values call for the owner, not an engineering one.**
- Auto-reverting the pattern pack on a latency regression — considered and declined: reverted rules
  may include `allow_list` entries, so a revert can loosen *or* tighten depending on pack contents,
  and there is no store-side retraction to revert *to*.

### §7.8 Analysis provenance

Six-lens pass, 2026-08-15: two mechanical inventories (configurable-knob surface; telemetry/data
surface), four judgement lenses (solution architecture; per-user budget design; enterprise UX;
platform/SRE detection), plus one live-vendor-docs research pass. Nothing was executed — no compose
stack, no `go test`, no e2e run; every claim is read from source or from vendor documentation.
The PIVOT-1 chain and the inverted severity ramp were verified directly against the four files
involved. N-thresholds in PIVOT-9/PIVOT-10 are proposals sized against one documented incident, not
derived from retained traffic — the system retains none.

---

## Appendix — consolidation history

Fold pass run 2026-08-07 via `/roadmap --no-delete` as part of a repo-wide doc-consolidation
sweep. **Nothing was deleted from disk this pass** (`--no-delete`); a separate doc-consolidation
step staged two files to a local out-of-repo purgatory folder afterward (see that tool's manifest
for exact destinations):

- `docs/handoff.md` (45 lines, folded into the §0 HISTORY block) —
  `git show 165b8df:docs/handoff.md`
- `docs/plans/TODO-cost-drilldown.md` (60 lines, folded into §4 in full — nothing summarized away) —
  `git show 165b8df:docs/plans/TODO-cost-drilldown.md`

**Deliberately not folded / not migrated — kept as separate, standalone files, because other
code or UI cites them by path** (folding would strand those citations):

- `docs/plans/TODO-gateway-deferred.md` — cited from `internal/gateway/credbroker/credbroker.go:35`,
  `internal/server/routes_gateway.go:126`, `internal/model/gateway.go:47`, README. Pointed at from §3.
- `docs/plans/TODO-vendor-auth.md` — cited from `internal/store/store.go:78`,
  `web/src/lib/authSchemas.ts:3`, rendered live in `web/src/pages/Vendors.tsx:311`. Pointed at
  from §5.
- `docs/plans/phase-1-surface-collection.md`, `docs/plans/phase-2-frontend.md`,
  `docs/plans/phase-3-inference-gateway.md` — all three linked directly from `README.md` as
  "Plans:" / "what shipped vs deferred" build-history records. All three now declare their built
  status in the doc itself (`phase-1-surface-collection.md:9`, `phase-2-frontend.md:8`,
  `phase-3-inference-gateway.md:3`), corrected in 7a4e36e; this roadmap's §1/§2/§3 remain the live
  status and still win on any conflict, because they carry the open items those records don't track.
- `docs/inference-gateway-design.md`, `docs/inference-gateway-build-plan.md` — reference/spec
  docs (README-linked as "Design:" / "sequencing:"); they reason about the gateway's architecture
  rather than sequence open work, so they stay separate per the roadmap-skill's reasoning-vs-
  sequencing test.

**Stays separate, not a plan doc at all:**

- `docs/air-traffic-analysis.md`, `docs/air-traffic-system-design.md` — strategy/spec docs,
  code-referenced (`internal/catalog/catalog.go`, `internal/catalog/cost_facets.go`).
- `docs/air-traffic-executive-overview.html`, `docs/air-traffic-claude-code-github-copilot.html` —
  portfolio brief decks, RECORDS by design, not superseded — left alone per the
  design-doc-archive convention (archive only applies to superseded generated docs). Both were
  class-styled HTML carrying a `.md` extension, so GitHub rendered them as doubled unstyled
  text; renamed to `.html` 2026-08-30. The four `.pdf` twins were deleted in the same pass —
  they were June renderings that had drifted from the corrected Markdown.
- `docs/inference-gateway-eli5.md` — code-referenced (`scripts/capture-harness-screenshot.js`).
- `BUILD_REPORT.md` — a different command's output, regenerated on its own cadence; never a fold
  target.
- Trimmed to open set 2026-08-18 — 15 closed items, 4 history blocks deleted; full pre-trim file:
  git show 3acf8cf3ab706590b7fdca7d63b63ee9daba2549:ROADMAP.md
