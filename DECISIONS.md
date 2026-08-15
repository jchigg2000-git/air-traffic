# air-traffic — Decisions

Append-only. One entry per ship or per decision that changes direction. **Supersede, never
silently rewrite** — a decision that is later reversed gets a new entry that names and supersedes
the old one; the old entry stays. Roadmap items cite entries by date + title.

## 2026-06-29 — Cost facets stay data-driven, not per-vendor Go objects

Asked directly of the builder while shipping cost drill-down: keep the **data-driven** model
(`internal/catalog/cost_facets.go` as the single `costFacetsByID` config object, read by both the
emitter and the synthetic fixture layer) rather than per-vendor emitter/config Go objects, to stay
faithful to the `it-scorecard` sibling repo's pattern (no `Connector` interface per vendor). The
emitter-vs-config split is realized as one emitter + one catalog map, not N per-vendor objects.
Source: `docs/plans/TODO-cost-drilldown.md` (folded into `ROADMAP.md` §4, 2026-08-07).

## 2026-07-02 — Honesty model extended, not relaxed, for the flywheel's manual→superseded flip

`manual` audit rows only flip to `superseded` when a config artifact (deny-list, threshold,
context-word rule) provably covers the missed PII type. Probe-sees-it-above-gate misses (e.g. SSE
straddle splits) stay `manual` — no config artifact would actually fix them. A detector chain
without Presidio in it never supersedes anything Presidio would have caught — approving a
Presidio-only config fix a non-Presidio chain ignores would be a lie the UI tells the operator.
Source: `docs/handoff.md` (folded into `ROADMAP.md` §0 HISTORY, 2026-08-07).

## 2026-07-02 — Shared Presidio vocabulary moved to `internal/model/presidio.go`

To preserve the G0 dependency-isolation rule (harness must not import `internal/gateway/detect`,
enforced by a ratchet test), the shared Presidio entity map, 0.40 default gate, and rune→byte
offset helpers moved out of `internal/gateway/detect/presidio.go` into `internal/model/presidio.go`
so both the gateway's detector and the harness's probe can read the same vocabulary without a
package-layering violation. Anything citing the old location (`detect/presidio.go:40`) is stale.
Source: `docs/handoff.md` (folded into `ROADMAP.md` §0 HISTORY, 2026-08-07).

## 2026-07-02 — Deny terms are values, and must never reach audit/reports

Deny-list terms (curated free-text PII values used to build Presidio `ad_hoc_recognizers`) are
themselves sensitive data, synthetic-only in this repo but treated as production-shaped. They may
live in proposals, the pattern pack, and `/api/gateway/patterns`, but must never appear in audit
events or gateway reports — the leak-guard tests check both surfaces specifically for this. Source:
`docs/handoff.md` (folded into `ROADMAP.md` §0 HISTORY, 2026-08-07).

## 2026-08-07 — ROADMAP.md + DECISIONS.md installed; TODO-gateway-deferred.md and TODO-vendor-auth.md deliberately NOT folded

Installed the roadmap SSOT triad via `/roadmap --no-delete` as part of a repo-wide doc-consolidation
sweep. Two rival status docs (`docs/handoff.md`, `docs/plans/TODO-cost-drilldown.md`) were folded
into `ROADMAP.md` and later staged out of the working tree (see `ROADMAP.md` Appendix). Two other
candidate docs — `docs/plans/TODO-gateway-deferred.md` and `docs/plans/TODO-vendor-auth.md` — were
**deliberately left standalone and unfolded**, because live Go and TypeScript source cite them by
path (`internal/gateway/credbroker/credbroker.go:35`, `internal/store/store.go:61`,
`web/src/lib/authSchemas.ts:3`) and `web/src/pages/Vendors.tsx:307` renders a citation to
`TODO-vendor-auth.md` directly in the running UI. Folding or staging either would have stranded
those citations. `ROADMAP.md` §3 and §5 point at them instead of duplicating their content.
`docs/plans/phase-1-surface-collection.md`, `phase-2-frontend.md`, and `phase-3-inference-gateway.md`
were similarly left standalone — all three are linked from `README.md`'s "Plans:" line as
build-history records, even though phase-1 and phase-2's own `Status:` headers are stale
(`PLANNED`, though the code has since shipped). This is agent-originated triage
(`CLAUDE-ORIGIN`), not an owner instruction — re-derive it before citing it as binding in a later
session.

## 2026-08-15 — Second client dialect shares one pipeline behind a `dialect` descriptor

The gateway now serves two client wire formats: `POST /v1/messages` (Anthropic Messages) and
`POST /v1/chat/completions` (OpenAI-compatible). Rejected the obvious alternative — a second
handler that copies the read → detect → redact/block → forward sequence — because that sequence is
where policy actually lives: fail-mode, the per-policy action derivation, the block/mask branch,
the byte-faithful relay. Two copies would drift, and the drift would be silent (one route quietly
not enforcing a policy the UI claims is enforced). Instead `proxyRequest` is shared and a `dialect`
struct carries only what genuinely differs: field walk, JSON usage extractor, SSE usage scanner,
error envelope, upstream path suffix, and default auth header. Adding a third dialect means adding
a walker and a scanner, not another pipeline.

Driver, recorded because it is not derivable from this repo: the owner's own agentic apps
(`~/Projects/meaning-to-making`, `~/Projects/hf-sandbox`) both reach Hugging Face's router through
`@huggingface/inference`, which speaks OpenAI wire format. With only the Anthropic route, neither
app could talk to the gateway at all — "prove it on my own apps" was blocked on a dialect, not on
policy or detection.

## 2026-08-15 — Upstream credential presentation is config, not hardcoded per route

`GATEWAY_UPSTREAMS[route]` gained `auth` (`x-api-key` | `bearer`). Anthropic wants a bare
`x-api-key`; OpenAI-compatible routers want `Authorization: Bearer`. Making this a config field
rather than a property of the dialect means one gateway can front `api.anthropic.com`,
`router.huggingface.co`, and `api.openai.com` simultaneously, and an endpoint that deviates from
its ecosystem's norm is a config change rather than a code change. Empty falls back to the
dialect's default, so every config written before this keeps working untouched.

Related: the version segment sits on different sides of the base-URL seam by ecosystem convention
(Anthropic `base_url` is the bare host + `/v1/messages`; OpenAI-compatible `base_url` already ends
in `/v1`, + `/chat/completions`). Followed each convention rather than normalizing to a third one,
so a URL copied from either vendor's docs works as-is.

## 2026-08-15 — `HF_UPSTREAM_TOKEN` has no compose default, on purpose

Every other credential in `docker-compose.yml` has a throwaway fallback so the stack comes up in
one command. The OpenAI route's does not. A placeholder there would mean sending a junk credential
to a **real** third-party endpoint (the route defaults to `router.huggingface.co`), whereas an
unset value fails credential resolution locally — `credbroker` errors on an empty `env:` ref — and
returns 502 without any outbound call. Verified: `POST /v1/chat/completions` on the running stack
returns `{"error":{"message":"upstream credential unavailable",...}}` and the vendor is never
contacted. The Anthropic route keeps its fallback because that fallback points at the in-repo
synthetic replica, not at a real vendor.

## 2026-08-15 — The per-request traffic feed reports tokens, still never dollars

`GET /api/gateway/requests` and the Gateway Traffic page exist because the per-request report ring
had no reader outside the harness scorer, so real application traffic was invisible in the UI.
`GatewayRequestReport` gained `model`, `tokens_in`, `tokens_out` — the last two attached inside
`record()` rather than at each call site, so the ring the spine drains and the metrics window
cannot disagree about a request.

`cost_usd` stays absent, extending the 2026-07-02 honesty position rather than revisiting it: the
gateway reports the tokens the vendor billed and stops. Absent usage stays zero rather than being
estimated — notably, an OpenAI-dialect stream reports no counts at all unless the caller sent
`stream_options.include_usage`, and reporting zero there is correct. The page says this in prose so
a blank column reads as "not reported" rather than "free".

## 2026-08-15 — The flywheel gets a suppression primitive; scores cannot separate a false positive from a real one

Added `allow_list` as a pattern-rule kind. Every prior kind either adds a detection (`regex`,
`deny_list`) or gates one by score (`threshold`); none could express "this exact term is not PII."

The alternative considered first — a per-type score gate — was rejected on measurement, not taste.
Presidio's spaCy layer returns **0.85 for every PERSON and LOCATION it emits**, whether the hit is
real or not: `Alice Whitfield`, `Margaret Chen`, `Springfield` and `Pennsylvania Avenue NW` score
identically to `Portugal`, `Spider-Man` and `JSON`. A gate that removes one removes the other. The
score is not a confidence in the colloquial sense and must not be tuned as if it were.

Applied in `Chain.Run` beside `typeGuards`, not inside the Presidio engine, because suppression has
to overrule whichever engine made the claim — the same argument that put the SSN/Luhn/IBAN guards
there. Scoped by type, matched on the span's own text case-insensitively. It suppresses a *term*,
never a region, so an allow-listed word sitting next to real PII does not shield it.

Approval is the trust boundary, so this kind gets the strictest validation: bounded count, no
empty or whitespace-padded terms, no line spans, and a shorter maximum length than a deny term
(48 vs 80 bytes) — the longer the string, the more likely it is an attempt to disable a type
wholesale rather than name a thing.

## 2026-08-15 — A false positive in real traffic is an owner judgement, so proposals became authorable

Added `POST /api/harness/proposals`. The flywheel infers proposals from harness misses, where
ground truth is exact by construction. It has no ground truth for real application traffic and
therefore **cannot infer that a detection was wrong** — only a human can say that `Spider-Man` in a
system prompt is not a person.

Rather than let that judgement become a hardcoded exception in the detector, it enters the same
propose → approve → hot-reload path as everything else, with the same audit event and the same
pack version bump. Authoring and approving stay separate acts, and a proposal without a rationale
is rejected at authoring time: an unexplained suppression is unreviewable.

Driver: under the `healthcare` baseline, meaning-to-making's stage-1 call was blocked on 100% of
turns by five false positives inside its own static system prompt — its `CHILD_INTERESTS` list
(`Spider-Man`, `Dogman`, `Pokemon`, `Mario`) read as PERSON, and the literal token `JSON` from
"Output ONLY the JSON object" read as LOCATION → ADDRESS. The app returned HTTP 200 the whole time
because it catches the failure and degrades, so nothing outside the gateway's own traffic feed
showed an outage.

## 2026-08-15 — meaning-to-making unrouted; hf-sandbox is the reference client

Owner call: Air-Traffic is an enterprise app, and a private single-family creative tool is the
wrong thing to demonstrate it against. Every change made to `~/Projects/meaning-to-making` was
reverted (source, tests, README, and its `.env` routing vars); that repo is no longer a client of
this gateway and the integration should not be restored.

The evidence it produced is kept rather than erased, because it is *why* the `allow_list` primitive
exists — five false positives inside a static system prompt, invisible behind an HTTP 200, blocking
every turn. That finding stands on its own regardless of which app surfaced it. `hf-sandbox` remains
the live OpenAI-dialect client.

Consequence recorded, not fixed: the pattern pack is append-only, so the allow-list rule authored
for that app (`manual-person-m2m-interests`) cannot be retired and stays in the pack. See
`docs/plans/TODO-gateway-deferred.md`, "the pattern pack has no retraction path".

## 2026-08-15 — Flight Deck: an element may not claim liveness it cannot disprove

Driver: the owner's read that the Flight Deck was "too busy" and that "many of the things stay
active as if they're emitting regardless." Probing the live stack confirmed the second half as a
correctness bug, not a styling complaint: the Go emitter gates emission on
`Enabled && Emit && Mode != disabled` (`internal/emitter/emitter.go:62`), but the client rollup
gated only on `mode === 'disabled'` (`web/src/lib/fleet.ts`). All 16 adapters carry
`mode: "synthetic"`, so the 13 with `enabled: false` fell through to green, rendered as healthy
rows, and inflated the "Healthy %" KPI. Three vendors were emitting; thirteen were painted alive.

**Principle adopted:** no element may claim liveness, health, or freshness unless it is derived
from data that could currently disprove it — and when the data disproves it, the element must show
that instead. Applied as: `VendorRollup.emitting` mirrors the backend gate; an emitting vendor's
`worstRag` now decays with feed age; off vendors render dimmed with an `off` marker and `—` in the
live columns; the header pill and board header report the age of the last *successful* poll; the
unconditional "emitter · 5s" chip and the static "live · updates every 5s" string are gone.

**Scope rulings.** (1) The `fleet.ts` change is classed as presentation-layer honesty, not a
behaviour change: it corrects a client-side view rollup, mutates no server state, and enables or
disables no adapter — the owner's standing rule that adapters are his to toggle is untouched.
(2) The same fake-live badges in `web/src/pages/landing/Hero.tsx` ("spine online", "emitter ·
…5s", both hardcoded and pulsing) were left in place: `/welcome` is a ratcheted champion governed
by `.claude/ratchet-up/landing-page-ledger.md`, and editing it outside that process would mutate
the champion. Filed as spec input for the next `/ratchet-up landing-page` run instead.

Not taken, deliberately: the vendor brand-hue palette rework (`VendorGlyph.tsx` oranges collide
with `--amber`), the `.panel-2` vs `bg-panel2` naming reconciliation, and propagating Gateway
Traffic's Live/Pause toggle app-wide — the last is behaviour addition, not tightening.

## 2026-08-15 — Gateway keystore: apps own the policy, keys carry the identity

Driver: the owner asked for "a keystore that we can issue keys based on user ids or apps… a
specific app should be allowed many keys." Reconnaissance reframed the problem before design
started. `GATEWAY_CLIENT_KEYS` already **authenticates** — one comma-separated env var resolved
once at boot into a `map[string]struct{}` (`internal/gateway/server.go:49`), checked by a bare map
lookup (`proxy.go:74`). What it cannot do is say *who* called, or serve two callers different
postures. So the keystore's justification is **attribution and scoping**, not authentication, and
the design is judged on those.

Both gaps were already costing something, in the repo's own record. `GatewayRequestReport` carried
no principal, so the Gateway Traffic page shipped earlier the same day could not tell one client
from another. And §0 of the roadmap records needing `general_saas`/`detect` for one app while a
stricter client would want `mask` — impossible with one global dial.

**Shape.** `App` is the parent and the unit policy attaches to; `APIKey` hangs off it with a
free-form `Subject` (user id, agent instance, CI job). One app, many keys. The `Subject` field is
also the deliberate OIDC on-ramp: a `sub` claim maps into it later, making federation a new
issuance path rather than a new data model. No OIDC/Entra machinery was built.

**Decisions worth naming, with their reasons:**

- **SHA-256, no pepper, no KDF.** The secret is 192 bits of `crypto/rand`, not a password — there
  is no dictionary to attack, so argon2/bcrypt would buy nothing while adding latency to a hot path
  that advertises 12–23 ms of added overhead. A pepper would have to reach every gateway,
  recreating the shared-secret problem it was meant to solve. (Go 1.26 has `crypto/pbkdf2` and
  `crypto/hkdf` in stdlib; the constraint here is judgement, not availability.)
- **Verification at the edge against a pulled snapshot**, on the same version-compare-and-swap
  contract as `PatternPack`. Rejected: a per-request `/verify` call to the control plane — it would
  add a network hop per request and break the property that the gateway keeps serving when the
  control plane is down.
- **Cache-and-continue, so revocation is eventual** (≤ `GATEWAY_POLICY_PULL_INTERVAL`, 15 s). A
  failed pull leaves the last snapshot in place. Failing closed would take every client down on a
  routine control-plane restart, and that store is in-memory so restarts are routine. Measured at
  ~14 s live. Instant revocation is a push, not a pull; not built.
- **The keystore persists; nothing else in the store does.** Observations and reports are
  reconstructible, issued credentials are not. `keys.json` under `AIRTRAFFIC_DATA_DIR`, 0600,
  temp-file + rename, digests only. A corrupt file is a **hard boot failure**: silently starting
  with an empty keystore would present to clients as an unexplained 401 storm.
- **Issuance is loopback-only, and explicitly NOT the spine key** (owner's choice when asked). The
  gateway holds the spine key; a gateway able to mint its own credentials makes the keystore
  pointless. Consequence, surfaced before the choice was ratified and confirmed live: compose
  publishes the control plane behind a port, so a browser or host `curl` arrives from the Docker
  bridge and gets 401. Hence **no keystore UI in this slice**, and `scripts/keystore.sh` routes
  admin calls through the container's netns. The upgrade path is the two-tier ladder
  `requireSpineKey` already implements, with an `AIRTRAFFIC_ADMIN_KEY`; not built.
- **`GATEWAY_CLIENT_KEYS` stays required at boot** and keeps authenticating, reporting as app
  `env`. The keystore is additive: README, compose, `dev-env.sh`, `e2e-gateway.sh` and the live
  hf-sandbox integration all depend on that path, and `E2E_COMPOSE=1 ./scripts/e2e-gateway.sh`
  staying 9/9 is the compatibility proof.
- **The heartbeat now accounts for scoped apps.** `pushHeartbeat` claims vendor enforcement only if
  the default action enforces *and* every app carrying its own baseline also enforces. Without
  this, one app scoped to monitor-only would have let the gateway overstate coverage — the same
  honesty rule as the 2026-08-15 Flight Deck entry, applied to a surface that scoping had just made
  able to lie.
- **`Subject` is the first field in `GatewayRequestReport` that can hold a human identifier.** It
  is admitted because the owner authors it at issuance — a deliberate label, not content extracted
  from traffic — and it is kept out of log lines even though it rides the report.

**Explicitly not in this slice:** per-app pattern-rule scoping (the `manual-person-m2m-interests`
retraction hole in `docs/plans/TODO-gateway-deferred.md` stays open), per-key quotas (same
stdlib-only blocker as G9), OIDC, and a keystore UI.

## 2026-08-15 — The control plane stays single-operator; auth is the admin-key tier, not a user model

Owner decision, asked directly during the non-expert-pivot analysis and answered
**"single-operator, harden in place."** The question on the table was whether the control plane
gets authenticated humans (principal + role at the `Routes()` seam, mirroring the gateway's own
`withPrincipal`) or stays a single-operator tool. It stays single-operator.

**What this ratifies:** `GATEWAY-7a` — the `AIRTRAFFIC_ADMIN_KEY` tier on `requireLocalAdmin`,
the same two-tier ladder `requireSpineKey` already implements — is the auth answer for this repo,
and it is now a decided item rather than an open owner call. It also unblocks a keystore UI, which
was absent only because loopback-only admin is unreachable from a browser over the Docker bridge.

**What this rules out, said plainly rather than left implicit:** there is no authenticated human
principal, so `model.AuditEvent.Actor` keeps its hardcoded literals (`"air-traffic:admin"` at
`routes.go:67`, `reconcile.go:60`, `routes_keystore.go:215`, `flywheel.go:678`) — an audit row can
name the system but never a person. Any future feature whose safety argument depends on
"we can see who did this" is not available under this decision.

**Consequence for Expert-vs-Guided modes:** Guided becomes **a read-only page set and a view
preference, not a permission model.** Without per-user authorization a mode that changes what
someone *may* do cannot exist; a mode that changes what they *see* is bypassable with one `curl`.
For a single operator that is honest and sufficient — the mode protects against a slip, not
against a user. It must be described that way in the UI and never as a safety boundary.

## 2026-08-15 — Policy persists; the stdlib-only constraint holds; no durable time series

Owner decision, same pass, on the storage fork: **policy persistence only.** The applied policy
gets write-through to a `policy.json` under `AIRTRAFFIC_DATA_DIR`, mirroring the keystore's
existing atomic temp-file + rename pattern (`internal/store/keystore_persist.go:27-40`). Stdlib
only; no `go.sum`; the CI guard stands.

Driver: `SetPolicy` writes `s.policy = &p` and nothing else (`internal/store/store.go:292-297`),
with no boot reload — so a control-plane restart silently discards the applied baseline while the
gateway keeps enforcing the old action (`pullPolicy` returns early on `Policy == nil` without
clearing `s.policyAction`, `internal/gateway/spine_pull.go:110-112`). The deployed stack runs
`GATEWAY_REDACT_ACTION=per_policy` (`docker-compose.yml:96`), so this is the live configuration,
not a latent one.

**Rejected, with the reason:** accepting a first third-party dependency to get a durable
`gwReports` time series. It would have unblocked zero-traffic detection, trailing-window
baselines, and cross-restart trend analysis in one decision, at the cost of the repo's most
distinctive property. Also rejected: hand-rolling a rolling-window store — `keys.json` proves the
JSON-file pattern works for a flat map, and a windowed rollup is where that stops being reasonable.

**What this permanently rules out, so no later pass re-proposes it as a gap:** detection of
"an app stopped sending traffic" (absence of traffic is indistinguishable from absence of the
metric — `pushObservations` returns early at zero requests, `spine_emit.go:53-55`), any threshold
computed against a trailing baseline that outlives a restart, and any cross-restart trend. All
bottleneck detection must therefore be computable from the **current** 5000-entry ring
(`internal/store/gateway.go:82-90`) within a single process lifetime.

**Relationship to G9, stated because it is easy to get wrong in both directions:** this answers the
*dependency* question G9 was blocked on (`ROADMAP.md` OWED-4) — the answer is no third-party
dependency — so G9's Redis-backed cross-replica budget counter stays deferred **by ruling** rather
than remaining an open fork. It does not make G9 buildable, and `policy.json` is not a step toward
it. Per-user vendor budget tuning does not need G9 and must not be bundled with it.
