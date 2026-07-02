# Handoff — 2026-06-30 11:17 CDT

> **Superseded 2026-07-02:** the gateway is no longer design-only. The MVP
> slice (G0/G1/G2/partial-G6/G7 + harness tab + flywheel v0) is built —
> see `plans/phase-3-inference-gateway.md` and `plans/TODO-gateway-deferred.md`.
> The "no code yet" statements below are historical.

## What shipped
- `6241d7a` — docs: add inference gateway design doc and pending deliverable pages — `docs/` (+ `BUILD_REPORT.md` at root). 5 files, +3770 lines, all additive/untracked-before. Now on `origin/main`.
  - The session's real work: `docs/inference-gateway-design.md` (272 lines) — a vendor-neutral PII/PHI-filtering inference gateway design.
  - Bundled along (pre-existing untracked from prior sessions, not authored this session): `BUILD_REPORT.md`, `docs/air-traffic-control-plane-spine.html`, `docs/air-traffic-enforcement-honesty.html`, `docs/air-traffic-nine-hour-build.html`. They rode the `git add -A` in `/shipit`.

## In-flight
None. Working tree clean (`git status --short` empty); branch `docs-inference-gateway-design` was merged ff-only to `main` and deleted local + remote.

## Decisions (this session)
All captured **inside the doc** (`docs/inference-gateway-design.md`) — no separate memory file written. The non-obvious ones the next session should know:
- **Gateway is design-only, off-spine, build-on-demand.** No code exists (`internal/gateway` absent, by design). Verdict in §15: build *when a real in-request-enforcement requirement lands*, not by default. Consistent with the standing "optional gateway" decision in auto-memory `project-air-traffic.md`.
- **Tee, don't gate the response** (§5, §11): response returns to caller directly; a split copy is teed to an async monitor. Detokenization is the one on-path exception (`tokenize` mode).
- **Tokenization oracle + capture buffer** (§11): vault doubles as a zero-false-positive leak oracle for previously-tokenized values; a short in-memory FIFO buffer (TTL ≥ monitor p99) harvests novel misses; **surrogate-on-promotion** keeps the durable training corpus synthetic.
- **Tokens are scoped+salted deterministic** (§7), e.g. `HMAC(conversation_salt, value)` — stable per entity within a conversation/tenant, never global (cross-session correlation risk).
- **OpenClaw is out** — earlier drafts discussed Anthropic's OpenClaw enforcement; user had it removed entirely as not applicable. Don't re-add. The supported routing point (API-key auth, `ANTHROPIC_BASE_URL`/`ANTHROPIC_AUTH_TOKEN`, BAA+ZDR) stays in §12.

## Don't break
- **The doc must stay self-consistent across edits.** It was audited once for drift; the load-bearing couplings: §7 token binding ↔ §10 "token *stability*" test (not "uniqueness"); §11 "monitor scans egress *and* responses" ↔ §4 diagram's *two* tees (`fwd -. tee cleaned request`, `vendor -. tee raw response`) ↔ §5 flow; §8 config keys ↔ the prose that introduces each. Change one, reconcile the others.
- **Mermaid diagrams** (two, in §4 and §11): validate via the Mermaid Chart MCP tool before committing — the render result is huge, so read only `{valid}` via `jq` from the saved tool-result file, don't inline it.
- **Vendor-neutral / no air-traffic branding** is a hard constraint on this doc (user-set). Keep it portable; the air-traffic-specific mapping (if ever wanted) goes in a *separate* companion doc.

## Next session: start here
Nothing is mid-edit — the gateway work is a complete design doc, shipped. Two clean continuations, neither urgent:
- **If a real requirement lands** (e.g. the pre-ZDR gate for a regulated health-plan org discussed this session — technically enforce "no PHI until ZDR"): build **M1** from §8 — a Go pass-through proxy (`net/http` + `httputil.ReverseProxy`), authenticate gateway key, swap upstream credential, forward, return, prove round-trip + streaming. New package `internal/gateway`, mounted only behind an `AIRTRAFFIC_GATEWAY` flag, nothing on the spine importing it.
- **Else** pick from the roadmap (`docs/plans/TODO-cost-drilldown.md`, `docs/plans/TODO-vendor-auth.md`) — both pre-date this session.
- First action either way: read `docs/inference-gateway-design.md` §8 (build plan) and §15 (should-we verdict).

## Deferred / open
- **M1–M6 gateway build** — design only; no code written this session. §8 sequences it; M6 (flywheel) is marked demand-driven.
- **Optional synchronous response guard** for model-generated/RAG PII — flagged in §5/§11 as a threat-model choice, intentionally not specified.
- **Cost drill-down byte-identical replicas** for Azure/Bedrock/Vertex/Tier-2/3 — pre-existing follow-up in `docs/plans/TODO-cost-drilldown.md` (untouched this session).

## How to verify
- `git log --oneline -3` → top is `6241d7a docs: add inference gateway design doc...`
- `git status --short` → empty (clean)
- `git ls-files docs/inference-gateway-design.md` → tracked; `grep -c '^## ' docs/inference-gateway-design.md` → 15 sections
- `ls internal/gateway 2>/dev/null || echo none` → `none` (design-only; confirms no code yet)
