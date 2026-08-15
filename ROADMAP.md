# air-traffic — ROADMAP

> ⭐ **SINGLE SOURCE OF TRUTH.** On any handoff or fresh session, **read this first and follow
> only this** for what's left, what's next, phases, acceptance criteria, and decisions. There are
> **no other `*_PLAN` / handoff docs** — the ones that existed were consolidated into this file
> (2026-08-07). If another doc's status ever conflicts with this one, **this wins.**
>
> - **Strategy / the "why"** (a different layer, not an execution plan): `docs/air-traffic-analysis.md`,
>   `docs/air-traffic-system-design.md`, `docs/air-traffic-executive-overview.md`,
>   `docs/air-traffic-claude-code-github-copilot.md` (portfolio briefs — RECORDS, point-in-time by design).
> - **Reference / spec** (opened on demand, never as "the plan"): `docs/inference-gateway-design.md`
>   (design), `docs/inference-gateway-build-plan.md` (sequencing), `docs/inference-gateway-eli5.md`
>   (referenced by `scripts/capture-harness-screenshot.js`). **Live deferred-work ledgers, kept
>   separate deliberately** because code and UI cite them by path: `docs/plans/TODO-gateway-deferred.md`
>   (cited from `internal/gateway/credbroker/credbroker.go:35`, `internal/store/store.go:61`, and README)
>   and `docs/plans/TODO-vendor-auth.md` (cited from `internal/store/store.go:61`,
>   `web/src/lib/authSchemas.ts:3`, and rendered live in `web/src/pages/Vendors.tsx:307`). Also kept
>   separate as build-history records (linked from README's "Plans:" line):
>   `docs/plans/phase-1-surface-collection.md`, `docs/plans/phase-2-frontend.md`,
>   `docs/plans/phase-3-inference-gateway.md`.
> - **Decisions**: `DECISIONS.md` (append-only log; roadmap items cite it by date + title)

**Legend:** ✅ done · 🔶 shipped but UNVERIFIED · ⏳ in progress · ⬜ not started · 🔬 verification
owed · 🔁 superseded (kept for its evidence, not as work) · ⛔ **BLOCKS** — the only marker that
gates anything

**Backlog items are not blockers.** No item under a `BACKLOG` / `PARKED` status may be cited as
gating, blocking, or holding up any other work unless it carries a `⛔ BLOCKS:` line with the
owner's verbatim instruction. Absent that line, treat it as non-blocking. An agent that reports a
parked item as a blocker is misreading this file.

**Contents:** §0 Do next · §1 Phase 1 — control plane backend · §2 Phase 2 — frontend SPA ·
§3 Phase 3 — inference gateway + harness + flywheel · §4 Cost & usage drill-down · §5 Per-vendor
auth schemas · §6 Open-decisions index · Appendix

---

## §0 Do next

> ### ▶ RESUME HERE — session handoff 2026-08-15 (GATEWAY-5: OpenAI dialect + traffic feed)
>
> **State:** working tree has **uncommitted** GATEWAY-5 work (see §3). Toolchain present this pass
> (go 1.26.5, node 24.14.1, docker 29.5.3), so unlike the 2026-08-07 pass everything below was
> actually built and run: `go test ./...` green, `npm run build` + 12 vitest green,
> `E2E_COMPOSE=1 ./scripts/e2e-gateway.sh` **9/9** (recall_behavioral 1.000, precision 0.991,
> trap FPs 0), compose stack rebuilt and healthy on :8122 / :8125 / :8126.
>
> **Why this work happened:** the goal was to route the owner's own agentic apps
> (`~/Projects/meaning-to-making`, `~/Projects/hf-sandbox`) through the gateway. Both call
> Hugging Face's router via `@huggingface/inference` — **OpenAI wire format** — and the gateway
> spoke only Anthropic `/v1/messages`, so neither app could reach it at all. That gap, not a
> roadmap item, drove the slice.
>
> **▶ NEXT ACTION: this is live and proven end-to-end. Remaining work is unchanged:** §3's other
> deferred G-blocks, §4's vendor cost facets, §5's 10 vendor auth schemas — none block each other.
>
> #### ⚠️ meaning-to-making is NOT a client of this gateway — reverted 2026-08-15
> It was used as the initial proving ground and every change to that repo has since been reverted
> at the owner's instruction (`git status` there is clean; its `.env` no longer carries
> `HF_ENDPOINT_URL` / `AIRTRAFFIC_CLIENT_KEY`). Air-Traffic is an enterprise app and a private
> single-family creative tool is the wrong thing to demonstrate it against. **hf-sandbox is the
> live integration.** The meaning-to-making findings below are kept because they are the evidence
> that produced GATEWAY-6, not because that integration still exists — do not restore it.
>
> #### The integration, as proven through the gateway (2026-08-15)
> Live config: `.env` holds minted `GATEWAY_CLIENT_KEYS` (`gwk-…`) + `AIRTRAFFIC_SPINE_KEY`
> (`spk-…`) plus `HF_UPSTREAM_TOKEN` (copied from the two apps, which shared one HF token). The
> `openai` upstream points at `https://router.huggingface.co/v1`. Applied baseline is
> **`general_saas`** → `pii_redaction: off` → gateway action **`detect`** (monitor-only).
> - **meaning-to-making** *(historical — reverted, see the warning above)* — routed via
>   `HF_ENDPOINT_URL` + `AIRTRAFFIC_CLIENT_KEY`. One real turn produced exactly the predicted 3
>   chat calls (control-read 499/8, stage1 1364/99, stage2 558/25) while `render` and `tts` went
>   direct. This is what the token reconciliation below was measured against.
> - **hf-sandbox** *(live)* — `HF_ROUTER_URL=http://127.0.0.1:8125/v1` (base already carries `/v1`) +
>   `AIRTRAFFIC_CLIENT_KEY`. Streamed and non-streamed both round-trip; the terminal usage chunk
>   survives the proxy hop, so the UI's running cost total still works. `/api/models` still goes
>   to the real router, by construction.
> - **Reconciliation PASSED.** Gateway tokens vs meaning-to-making's *independent* SQLite store
>   (`server/data/observability.sqlite`): identical multisets, **2421 in / 132 out** across all 3
>   chat calls. Two separately-instrumented measurements agreeing is the proof that the OpenAI
>   usage scanner is right.
>
> #### Findings worth keeping
> - **Detect-first was the correct call, and there is now evidence.** On a turn about *"a big red
>   boat on the water"*, Presidio flagged 4 × `PERSON_NAME` and 1 × `ADDRESS` — all inside
>   `messages[0].content` at offsets 1512-1546 and 5574-5578, i.e. **in meaning-to-making's own
>   system prompt**, not in the child's utterance. Under `mask` those would have been rewritten to
>   `[PERSON_NAME]` inside the app's instructions, and the damage would have presented as model
>   misbehaviour. Raise the baseline for this app only after tuning; `general_saas` → `fintech`
>   flips it to `mask` live via policy pull, no restart.
> - Gateway-added latency scales with prompt size: 12–23 ms on hf-sandbox's small prompts,
>   216–217 ms on meaning-to-making's ~1.4k-token ones.
> - A transient HF `service_unavailable` was observed once mid-test and relayed byte-faithfully;
>   isolated to the vendor by comparing a direct call against a proxied one, not a gateway defect.
>
> #### Unverified / owed from this pass
> - The Gateway Traffic page (`/settings/traffic`) is served (route 200, component present in the
>   built bundle) and its API is verified against real data, but **nobody has looked at it in a
>   browser** — same gap OWED-2 records for the Harness tab.
> - The **Anthropic** route is still pointed at the synthetic replica; only the OpenAI route has
>   run against a real vendor. Claude Code through `/v1/messages` remains untried.
> - The control plane's store is in-memory: recreating the container wipes observations, reports,
>   and the applied policy. The `general_saas` baseline above must be re-applied after a restart.
>
> ### Session handoff 2026-08-07 (doc-consolidation pass) — HISTORY, superseded by the block above.
>
> **State:** `main` clean at `e4604d4` (`chore(ci): bump actions to v7, drop the unusable Go
> module cache`). No `go`/`npm` toolchain available in the sandbox this pass ran in, so nothing
> below was rebuilt or re-tested — treat "shipped" claims as **citations against README/commit
> history**, not as freshly-verified. Prior session evidence (README, `docs/plans/phase-3-*.md`):
> `go test ./...` green, `scripts/e2e-gateway.sh` 9/9 as of 2026-07-02; CI (`.github/workflows/ci.yml`)
> covers Go + web + both Docker images as of `e4604d4`.
>
> *(That pass's next-action, kept for the record: pick up any of §3's deferred G-blocks, §4's
> remaining vendor cost facets, or §5's remaining 10 vendor auth schemas — none block each other.
> Still true; see the current block above.)*
>
> #### What shipped (this pass)
> - Installed this ROADMAP.md + DECISIONS.md + CLAUDE.md triad (repo had no `ROADMAP.md` and no
>   `CLAUDE.md` before this pass).
> - Folded `docs/handoff.md` (2026-07-02 G6 session handoff) into the HISTORY block below.
> - Folded `docs/plans/TODO-cost-drilldown.md`'s open TODO items into §4.
>
> #### What I found by reading that nobody reported
> - `docs/plans/phase-1-surface-collection.md` and `docs/plans/phase-2-frontend.md` both still say
>   `Status: PLANNED` in their own headers, but the code (`internal/*`, `web/src/*`) and README both
>   show Phase 1, 2, and 3 fully shipped. Their `PLANNED` status lines are stale relative to the repo
>   but the files themselves stay in place (README links them as "Plans:" build-history records) —
>   this roadmap is the corrected status; the plan docs are not being edited.
> - Three docs carry a paired `.pdf` and read as generated pitch/brief decks (`docs/air-traffic-executive-overview.md`,
>   `docs/air-traffic-claude-code-github-copilot.md`, plus `docs/air-traffic-analysis.md` and
>   `docs/air-traffic-system-design.md` which also have `.pdf` twins but are additionally
>   code-referenced). None show signs of being superseded — classified as portfolio RECORDS, left
>   untouched per the design-doc-archive convention (archive only if superseded).
>
> #### What I deliberately did NOT do, and why
> - Did **not** touch `docs/plans/TODO-gateway-deferred.md` or `docs/plans/TODO-vendor-auth.md` —
>   both are cited by path from live Go/TS source and one is rendered directly in the Vendors UI.
>   Folding or staging either would silently break those in-repo citations. They stay the
>   authoritative, continuously-updated ledgers for their subject matter; this roadmap points at
>   them (§3, §5) rather than duplicating their content.
> - Did **not** re-run `go test`/`npm run build` — no Go/Node toolchain in this session's sandbox.
>   Nothing here should be read as "verified working today."
>
> #### Questions
> - None load-bearing enough to block — see §6 for the two 🔬 OWED items carried over from the
>   folded docs.

### Session handoff 2026-07-02 (evening) — HISTORY, superseded by the block above.
G6 config-knob slice shipped (`c00c975`): deny-list / threshold / context-word proposal kinds
flow propose→approve→hot-reload; try-a-prompt box added to the Gateway Harness tab
(`POST /api/harness/sample`). Also fixed en route: two live trap FPs (`US_ITIN`→`SSN` mapping,
unmapped-Presidio-built-ins now dropped, `ADDRESS` hyphen guard), a stale-pointer upsert bug
(`TestUpsertSurvivesSliceGrowth`), and a full-loop test that was silently lying about recall
(heartbeat pointed at the wrong gateway instance — see `DECISIONS.md` 2026-07-02 entries).
Verification owed at the time, uncertain if since closed — see §6 OWED-1 and OWED-2.
Full original text recoverable: `git show e4604d4:docs/handoff.md` (folded 2026-08-07, file
kept on disk, not deleted — see Appendix).

---

## §1 Phase 1 — Control plane backend

- ✅ **PHASE1-1** Go stdlib-only backend (`cmd/air-traffic-server`) serving byte-identical
  synthetic replicas of 16 vendor admin/control surfaces (6 Tier-1 deep, 6 Tier-2 core, 4 Tier-3
  manifest-only), the control-plane API, the `ops-observation-batch/v1` emitter, policy/baseline/
  drift engine, and env-config rendering. Full spec + acceptance criteria:
  `docs/plans/phase-1-surface-collection.md` (kept as build-history record, not re-transcribed
  here — see roadmap-callout Appendix note on fidelity).
- ✅ **PHASE1-2** Five-disposition honesty model (`vendor_native` / `env_managed` /
  `proxy_enforced` / `monitor_only` / `unverified` / `unsupported`) plus `env_managed` enforcement
  tiers (`server_side` / `mdm_locked` / `seed_only`) — README confirms this is live and documented
  in the HTTP API surface.
- 🔬 **OWED-3** The phase-1 doc's acceptance checklist (`docs/plans/phase-1-surface-collection.md`
  §13) uses `- [ ]` checkboxes never flipped to `[x]` even though the code shipped — nobody ever
  re-ran `go test ./... ` against that specific checklist to close it out formally. Low priority
  (code + README are strong secondary evidence) but never confirmed line-by-line.

---

## §2 Phase 2 — Frontend SPA

- ✅ **PHASE2-1** Vite + React + TS + Tailwind v4 + TanStack Query + React Router SPA, served
  same-origin from `web/dist`. Flight Deck (`/`) observability landing, `/settings/*` consoles
  (Rigor Console, Policy Editor, Cost & Usage Explorer, Vendors, Observability, Audit), plus a
  public `/welcome` marketing page. Full spec: `docs/plans/phase-2-frontend.md` (build-history
  record, kept in place).
- ✅ **PHASE2-2** (`3f14442`+ series) CSV + JSON export added to Cost Explorer; API error surfacing
  in CostExplorer (`9e212b2`); Vitest harness added (`d2cd9cb`).
- ✅ **PHASE2-3** (2026-08-15) **Honest-liveness pass.** Fixed the rollup bug that painted 13
  non-emitting vendors green (`web/src/lib/fleet.ts` now mirrors the emitter's own gate via
  `VendorRollup.emitting`; stale feeds decay; `healthy` counts emitting vendors only;
  `unmatchedEmitters` surfaces emitters with no adapter row). Flight Deck: off rows dimmed with an
  `off` marker, worst-first sort, unconditional pulses and the hardcoded "emitter · 5s" /
  "live · updates every 5s" claims replaced with the real age of the last successful poll, KPI
  strip 5→4, Legend and dead `.tile-flash` CSS removed. App-wide: shared
  `components/ApiStateBanner.tsx` mounted on the 7 pages that previously failed silently;
  loading-vs-empty conflation fixed in Vendors + Gateway Harness + Rigor Console. Principle and
  scope rulings: `DECISIONS.md` 2026-08-15 "an element may not claim liveness it cannot disprove".
  - ⬜ **PHASE2-3a** Next `/ratchet-up landing-page` run must fix `web/src/pages/landing/Hero.tsx`
    (~`:30-40`): the "spine online" and "emitter · ops-observation-batch/v1 · 5s" badges pulse
    unconditionally with zero data binding and cannot go false — a violation of that page's own
    Correctness axis. Left untouched this pass because `/welcome` is a ratcheted champion; it must
    be changed through the ledger, and the run must still beat the reigning score.
  - ⬜ **PHASE2-3b** Residual, non-blocking: `web/src/components/VendorGlyph.tsx` brand hues for
    bedrock/mistral/m365/groq (`#FF9900`, `#FA520F`, `#D83B01`, `#F55036`) sit in the same band as
    `--amber`/`--unverified`, so a decorative glyph can camouflage a real status dot in the same
    row. Mitigated (off-row dimming + glow now conditional), not solved; a full palette rework was
    deliberately not taken because `VendorGlyph` is shared with the ratcheted landing page.

---

## §3 Phase 3 — Inference gateway + harness + flywheel

- ✅ **GATEWAY-1** MVP inference gateway (`cmd/air-traffic-gateway`, port 8125): pass-through
  proxy, regex + self-hosted Presidio PII/PHI detection, mask/block/detect-only, spine
  integration (observations, leak findings, enforcement heartbeats, policy pull), Gateway Harness
  UI tab + recall-ratchet flywheel v0. Built and verified 2026-07-02 per
  `docs/plans/phase-3-inference-gateway.md` (`go test ./...` green, `e2e-gateway.sh` 9/9,
  recall_behavioral 0.997 / precision 0.975 / trap FPs 0 on a 120-request seeded run —
  **not re-verified this pass**, see §0 state note).
- ✅ **GATEWAY-2** (`8b4319f`, closed 2026-07-30 per `docs/plans/TODO-gateway-deferred.md`) Spine
  auth on `/api/gateway/*` (`AIRTRAFFIC_SPINE_KEY`), key rotation via `scripts/dev-env.sh`,
  deeper multimodal/tool-call field walk (tool_result, tool_use.input, document sources),
  SSE usage extraction, CI hardening.
- 🔶 **GATEWAY-5** (2026-08-15, **uncommitted**) OpenAI-compatible dialect + per-request traffic
  feed. `POST /v1/chat/completions` ships beside `/v1/messages`, both running the *same* pipeline
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
- 🔶 **GATEWAY-6** (2026-08-15, **uncommitted**) Flywheel suppression: `allow_list` pattern-rule
  kind + `POST /api/harness/proposals` for owner-authored proposals. The flywheel could only ever
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

- ✅ **COST-1** `internal/catalog/cost_facets.go` is the single source of truth for per-vendor
  cost drill-down dimensions (`costFacetsByID`); both the emitter (`internal/emitter/emitter.go`
  `costBreakdowns`, all 16 vendors) and the synthetic fixture layer
  (`internal/synthetic/fixtures_cost.go` + `cost_grouping.go`) read it, so they can't disagree.
  Decision on the data-driven-vs-per-vendor-object shape: `DECISIONS.md` 2026-06-29
  "Cost facets stay data-driven, not per-vendor Go objects".
- ✅ **COST-2** Byte-identical synthetic grouping shipped for OpenAI (`/usage`, `/costs`),
  Anthropic (`usage_report`, `cost_report`), GitHub Copilot (billing/usage, metrics, seats).
  Tests: `internal/synthetic/fixtures_cost_test.go`.
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

- ✅ **VENDOR-1** 6 Tier-1 vendors (OpenAI, Anthropic, AWS Bedrock, Azure OpenAI, Google Vertex,
  GitHub Copilot) have real proxy-config schemas in `web/src/lib/authSchemas.ts` and ship
  **enabled** by default.
- ⬜ **VENDOR-2** 10 remaining vendors (Mistral, Databricks, Perplexity, Cohere, Together, Groq,
  xAI, Amazon Q, IBM watsonx, M365 Copilot) ship **disabled** and fall back to URL-only config
  until their schema is built. Suggested shape per vendor + build steps: full detail in
  `docs/plans/TODO-vendor-auth.md` — not duplicated here since that file is the one the UI cites.
  **Reminder carried forward:** the enabled/disabled roster is the owner's to set — never
  auto-toggle (`docs/plans/TODO-vendor-auth.md` cites auto-memory `feedback-no-auto-toggle-vendors`).

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
- 🔬 **OWED-4** — see §3 GATEWAY-3 G9: Redis-vs-hand-rolled-RESP-client is an owner scope call,
  not yet made.

---

## Appendix — consolidation history

Fold pass run 2026-08-07 via `/roadmap --no-delete` as part of a repo-wide doc-consolidation
sweep. **Nothing was deleted from disk this pass** (`--no-delete`); a separate doc-consolidation
step staged two files to `~/Projects/purgatory/air-traffic/` afterward (see that tool's manifest
for exact destinations):

- `docs/handoff.md` (45 lines, folded into the §0 HISTORY block) —
  `git show e4604d4:docs/handoff.md`
- `docs/plans/TODO-cost-drilldown.md` (60 lines, folded into §4 in full — nothing summarized away) —
  `git show e4604d4:docs/plans/TODO-cost-drilldown.md`

**Deliberately not folded / not migrated — kept as separate, standalone files, because other
code or UI cites them by path** (folding would strand those citations):

- `docs/plans/TODO-gateway-deferred.md` — cited from `internal/gateway/credbroker/credbroker.go:35`,
  `internal/store/store.go:61`, README. Pointed at from §3.
- `docs/plans/TODO-vendor-auth.md` — cited from `internal/store/store.go:61`,
  `web/src/lib/authSchemas.ts:3`, rendered live in `web/src/pages/Vendors.tsx:307`. Pointed at
  from §5.
- `docs/plans/phase-1-surface-collection.md`, `docs/plans/phase-2-frontend.md`,
  `docs/plans/phase-3-inference-gateway.md` — all three linked directly from `README.md` as
  "Plans:" / "what shipped vs deferred" build-history records. Their own `Status:` headers are
  stale (phase-1/2 still say `PLANNED`); this roadmap's §1/§2/§3 are the corrected status. The
  files themselves were left untouched — editing a README-linked doc was out of scope for this
  pass.
- `docs/inference-gateway-design.md`, `docs/inference-gateway-build-plan.md` — reference/spec
  docs (README-linked as "Design:" / "sequencing:"); they reason about the gateway's architecture
  rather than sequence open work, so they stay separate per the roadmap-skill's reasoning-vs-
  sequencing test.

**Stays separate, not a plan doc at all:**

- `docs/air-traffic-analysis.md`, `docs/air-traffic-system-design.md` — strategy/spec docs,
  code-referenced (`internal/catalog/catalog.go`, `internal/catalog/cost_facets.go`).
- `docs/air-traffic-executive-overview.md`, `docs/air-traffic-claude-code-github-copilot.md` —
  portfolio brief decks (paired `.pdf` twins), RECORDS by design, not superseded — left alone
  per the design-doc-archive convention (archive only applies to superseded generated docs).
- `docs/inference-gateway-eli5.md` — code-referenced (`scripts/capture-harness-screenshot.js`).
- `BUILD_REPORT.md` — a different command's output, regenerated on its own cadence; never a fold
  target.
