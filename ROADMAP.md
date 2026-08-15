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
auth schemas · §6 Open-decisions index · §7 Non-expert pivot (honesty, blast radius, detection,
budget) · Appendix

---

## §0 Do next

> ### ▶ RESUME HERE — session handoff 2026-08-15 (GATEWAY-7: the gateway keystore)
>
> **State:** working tree has **uncommitted** GATEWAY-7 work on top of the uncommitted GATEWAY-5/6
> work below. Everything below was built and verified this pass: `go test -race ./...` green,
> `npm run build` + 18 vitest green, `E2E_COMPOSE=1 ./scripts/e2e-gateway.sh` **9/9**
> (recall_behavioral 1.000, precision 0.991, trap FPs 0), compose stack rebuilt and healthy.
>
> **What landed.** A keystore: apps, many keys per app tagged by `subject` (user id / agent
> instance / CI job), route scoping, expiry, revocation — and, the actual payoff, **per-app
> redaction policy**. `GATEWAY_CLIENT_KEYS` already authenticated; what it could not do was say
> who called or serve two callers different postures. Proven live: `hf-sandbox` scoped to
> `fintech` resolved to **mask** while a legacy `env` caller on the global `general_saas` resolved
> to **detect**, same gateway, same instant. See §3 GATEWAY-7 and `DECISIONS.md` 2026-08-15
> "Gateway keystore".
>
> **▶ NEXT ACTION — superseded 2026-08-15 by the non-expert-pivot pass; see §7.** The strongest
> candidate is now **§7.1 PIVOT-1**, a confirmed one-click org-wide outage (the Rigor Console's
> "Healthcare" card resolves to `block` for every caller, with no UI path to the attestation that
> would prevent it), followed by §7.2's four cheap instrumentation fixes. §3's deferred G-blocks,
> §4's vendor cost facets and §5's 10 vendor auth schemas remain open and still block nothing.
> **GATEWAY-7a is no longer an owner call** — the `AIRTRAFFIC_ADMIN_KEY` tier was ratified
> 2026-08-15 as this repo's auth answer (`DECISIONS.md`, "The control plane stays single-operator").
>
> **Owed / unverified from the keystore pass:**
> - The Gateway Traffic page's new **App column is build-verified only** — the same
>   nobody-opened-a-browser gap OWED-2 records. Its API and attribution are verified against real
>   traffic.
> - There is **no keystore UI**, by construction (see GATEWAY-7a). Administration is
>   `scripts/keystore.sh`.
> - Live stack state left behind: an app `hf-sandbox` is registered with **no baseline** (inherits
>   the global policy) and **two revoked keys**. Inert, but it is real state in the `harness-data`
>   volume, not a fixture.
>
> ### Session handoff 2026-08-15 (GATEWAY-5: OpenAI dialect + traffic feed) — HISTORY
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
- ✅ **GATEWAY-7** (2026-08-15) **Gateway keystore — apps, issued keys, per-app policy.** Before
  this the gateway's whole notion of "who is calling" was `GATEWAY_CLIENT_KEYS`: one env var
  resolved at boot into a `map[string]struct{}`, checked by a bare map lookup. It authenticated
  and nothing more — no principal on any report, and one redaction posture for every caller.
  - **What shipped.** `model.App` / `model.APIKey` / `model.KeySnapshot`
    (`internal/model/keystore.go`); write-through persistence to `keys.json` under
    `AIRTRAFFIC_DATA_DIR` (`internal/store/keystore*.go`); a loopback-gated admin API
    (`internal/server/routes_keystore.go`: `/api/apps`, `/api/apps/{id}/keys`, `/api/keys/{kid}`)
    plus `GET /api/gateway/keys` on the spine; edge verification against a pulled snapshot
    (`internal/gateway/keystore.go`, `pullKeys` in `spine_pull.go`); `actionFor(principal)`
    replacing `currentAction()`; `app_id`/`key_id`/`subject`/`baseline` on
    `GatewayRequestReport`; an App column on the Gateway Traffic page; `scripts/keystore.sh`.
  - **Proven live on the compose stack**, not merely built: app `hf-sandbox` scoped to `fintech`
    resolved to **mask** at the same instant a legacy `env`-key caller on the global
    `general_saas` resolved to **detect** — two postures, one gateway. Attribution
    (`app_id`/`key_id`/`subject`) landed on the traffic feed; an `openai`-scoped key reached the
    real Hugging Face router on `/v1/chat/completions` and was refused 401 on `/v1/messages`;
    revocation took ~14 s (inside the 15 s pull window); the keystore survived a
    `docker compose restart control-plane` that wipes everything else in that store.
  - **Compatibility bar held:** `E2E_COMPOSE=1 ./scripts/e2e-gateway.sh` **9/9**
    (recall_behavioral 1.000, precision 0.991, trap FPs 0) — it authenticates with env keys, so
    green there is the proof. `go test -race ./...`, `npm run build`, 18 vitest all green.
  - **Rationale, tradeoffs, and what was deliberately not built:** `DECISIONS.md` 2026-08-15
    "Gateway keystore: apps own the policy, keys carry the identity".
  - ⬜ **GATEWAY-7a** (follow-up, non-blocking) — **DECIDED 2026-08-15, no longer an owner call.**
    No keystore UI, because issuance is loopback-only and compose publishes the control plane
    behind a port — a browser request arrives from the Docker bridge, not loopback. Opening it to
    the UI means adding an `AIRTRAFFIC_ADMIN_KEY` tier to `requireLocalAdmin`, the same two-tier
    ladder `requireSpineKey` already implements. ~30 lines, no data-model change. The owner
    ratified this as the repo's auth answer (`DECISIONS.md` 2026-08-15, "The control plane stays
    single-operator; auth is the admin-key tier, not a user model") — the alternative, an
    authenticated per-human principal at the `Routes()` seam, was considered and rejected. Build it
    when convenient; nothing gates it.
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
- ✅ **OWED-4 — ANSWERED 2026-08-15.** The dependency question G9 was blocked on
  (Redis vs hand-rolled RESP client vs neither) was put to the owner as part of the storage fork
  and answered **no third-party dependency; stdlib-only holds** (`DECISIONS.md` 2026-08-15,
  "Policy persists; the stdlib-only constraint holds; no durable time series"). G9's cross-replica
  budget counter therefore stays deferred **by ruling** rather than remaining an open fork. Read
  the consequence precisely: this does not make G9 buildable, `policy.json` is not a step toward
  it, and per-user vendor budget tuning (§7.5) does not need it and must not be bundled with it.

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
> to adjust parameters"; "auto budget tuning in per user (github copilot is our harness at work so
> def start with it), we have claude as well…but only few of us"; consider "administrative lift,
> and the risk of a user not understanding and opening the wrong thing"; and "different modes.
> Expert tuning versus guided."
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

### §7.1 CONFIRMED DEFECT — Healthcare is a one-click org-wide outage

⬜ **PIVOT-1** Verified end-to-end against source 2026-08-15 (not executed; read from code):
`web/src/pages/RigorConsole.tsx:126` calls `api.applyPolicy(selected)` with one argument →
`web/src/lib/api.ts:410` defaults `overrides = {}` → `Policy.Vendors` is empty →
`internal/gateway/spine_pull.go:117-119` derives `zdrAttested = false` → healthcare is
`ZDR:"enforced"` + `PIIRedaction:"on+phi"` (`internal/policy/baselines.go:26-29`) →
`spine_pull.go:144-146` returns **`actionBlock`**.

The card labelled "Healthcare 🔒🔒🔒 — Maximum rigor" blocks 100% of gateway traffic org-wide
within one pull interval. **No control anywhere in the UI can set `zdr_attested`**, so from a
browser Healthcare is unconditionally block, permanently. This is the failure already in the record
(`docs/plans/TODO-gateway-deferred.md:30`) — an app returning HTTP 200 while every call was dropped.

Three aggravators, same page:
- `RigorConsole.tsx:116` is `useState<string>('fintech')` — the accent-coloured primary CTA is
  **armed on arrival** with a posture nobody chose.
- The severity ramp is **inverted**: buttons render 🔒→🔒🔒→🔒🔒🔒→🔒🔒🔒 while `deriveAction`
  yields `detect→mask→block→mask`. `gov_infra` sits last, looks strictest, and enforces *less* than
  healthcare because its `PIIRedaction` is `"on"` and falls to the default branch.
- No confirmation dialog exists anywhere in the codebase, and `PUT /api/policies` is unauthenticated.

Candidate fixes (not chosen): let the UI send `zdr_attested`; gate Healthcare behind an attestation
step; or — preferred by the analysis — make Apply **preview the derived gateway action before
commit**. The words `detect`/`mask`/`block` currently appear nowhere on the page and are the only
thing Apply changes.

### §7.2 Tier 0 — instrumentation preconditions (cheap, no decision needed)

- ⬜ **PIVOT-2** **Record the eight silent exits in `proxyRequest`.** `s.record()` fires at only 3
  of 11 exits (`internal/gateway/proxy.go:245`, `:256`, `:325`). Auth failure (`:89`), no upstream
  (`:172`), oversized body (`:178`), bad JSON (`:206`), mask failure (`:265`), credential
  resolution failure (`:279`), request-build failure (`:286`) and upstream unreachable (`:298`) all
  return with zero rows and zero metrics (`metrics.observe` is reachable only via `record`,
  `internal/gateway/audit.go:66-67`). The heartbeat keeps beating on its own timer claiming
  enforcement throughout. **A route failing 100% of requests is indistinguishable from an idle
  one** — and compose ships `HF_UPSTREAM_TOKEN` with no default by design
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

- ⬜ **PIVOT-6** Close PIVOT-1 (see §7.1) — preview-before-commit, drop the armed default, show the
  affected-app count (`totals.apps` already exists on the traffic page).
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
  (`web/src/pages/GatewayHarness.tsx:441`), under a header reading *"human-approved only — nothing
  auto-applies"* — reassurance where a warning belongs.
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
- ⬜ **PIVOT-13** **Split-brain policy detector.** On restart the control plane forgets the policy,
  but `pullPolicy` returns early on `Policy == nil` **without clearing `s.policyAction`**
  (`internal/gateway/spine_pull.go:110-112`) — the gateway keeps enforcing while the control plane
  believes nothing is applied, and nothing compares them. `gatewayDrift`'s `expected` flips false
  (`internal/policy/drift.go:49-52`), *silencing the one check that would have noticed*. When the
  gateway later restarts it falls to `actionMask` (`proxy.go:148`) — a third posture matching
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
- Persisting the policy without fixing the split-brain revert (PIVOT-13) — that makes the
  disagreement durable instead of transient.

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
