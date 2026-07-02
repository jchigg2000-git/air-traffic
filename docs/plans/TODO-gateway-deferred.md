# Inference gateway — deferred work ledger

*Phase 3 (see [`phase-3-inference-gateway.md`](./phase-3-inference-gateway.md)) shipped the MVP slice: G0, G1, G2, partial-G6 (Presidio), G7, the Gateway Harness tab, and flywheel v0. Everything below is deliberately deferred; full specs live in the G-blocks of [`../inference-gateway-build-plan.md`](../inference-gateway-build-plan.md) §5.*

## Deferred G-blocks

- **G3 — Reversible tokenize + Redis vault + SSE boundary buffering.** Only `mask`/`block`/`detect` ship today; `tokenize` is rejected at config load. The credbroker recognizes `vault:`/`kms:` refs but errors on them (env: only).
- **G4 — Async monitor + in-pod capture buffer + dual tee.** No response-side enforcement: the harness's SSE straddle cases score the *reassembly scanner* (detect-only, informational `response_leaks`), not a response gate.
- **G5 — Tokenization oracle.** Needs the G3 vault.
- **G6 (second half) — Managed DLP adapters** (Comprehend Medical / Google DLP / Azure PII, BAA-gated). Presidio self-hosted tier shipped; policy-selected engine routing per route did not (chain is static from `GATEWAY_DETECTORS`).
- **G8 — OTel depth + log-leak CI wiring.** Gateway telemetry is slog JSON + spine metrics; no OTel SDK, no trace spans. The log-leak guard exists as Go tests (`internal/gateway/pipeline_test.go`, harness full-loop test) but isn't a separate CI job.
- **G9 — Horizontal-scale hardening.** Single replica; no Redis budget counters, no hard cross-vendor spend stop, no HPA/drain. The dependency isolation + stateless-pod shape is in place.
- **G10 (full) — LLM-judge labeling, Synthea generation, shadow mode.** Flywheel v0 is harness-ground-truth + curated candidate library + human approval only. No learned patterns, no NER fine-tuning (no-GPU local ceiling — deliberate).

## Slice-local deferrals (from this build)

- **SSE usage extraction:** token counts are parsed from non-streaming responses only (`relayWithUsage`); streaming `message_delta.usage` is ignored.
- **`cost_usd` pass-through:** gateway emits `tokens_in/out` but not dollars — fabricating a price table would violate the honesty model. Wire real pricing when a source of truth exists.
- **Auth on `/api/gateway/*` ingest routes:** the control plane trusts local pushes (loopback dev posture). Add a shared token before any non-local deploy.
- **Multimodal / tool-call field walk:** the Anthropic adapter walks `system` + `messages[].content` (string + text blocks). `tool_result` content, images, and documents are unwalked — the adapter-drift risk called out in build-plan §7.
- **OpenAI-compatible dialect:** Anthropic Messages only (`/v1/messages`); `adapter_openai.go` is additive behind the same walk seam.
- **Audit-ring pressure at volume:** per-request gateway reports live in a 5000-cap ring; only blocks/fail-trips hit the audit stream. Fine for harness scale; revisit with G8.
- **Presidio custom-recognizer YAML mount:** pattern-pack rules ride per-request `ad_hoc_recognizers`; a mounted recognizer config is the G6-full upgrade.
- **Harness gateway key:** control plane reads `AIRTRAFFIC_GATEWAY_KEY` (default `gwk-demo`) — dev convenience, rotate for any shared deploy.
- **`web/src/lib/dispositions.ts` copy:** the `proxy_enforced` blurb still says "off by default"; the Policy Editor outcome chip is already truthful (`applied_proxy`), so this is cosmetic.
