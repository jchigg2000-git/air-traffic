# Phase 3 — Inference Gateway MVP + Harness + Flywheel v0

**Status: built and verified 2026-07-02** — `go test ./...` green, `scripts/e2e-gateway.sh` 9/9 (recall_behavioral 0.997, precision 0.975, trap FPs 0 on the 120-request seeded run). Deferred items: [`TODO-gateway-deferred.md`](./TODO-gateway-deferred.md).

*This document records the phases as they were built. Post-ship changes (spine auth, key
rotation, the deeper multimodal/tool field walk, SSE usage extraction, CI) are logged in the
ledger's "Closed" section — read that for current behaviour, this for build history.*

*Implementation plan. Executes the MVP slice of [`../inference-gateway-build-plan.md`](../inference-gateway-build-plan.md) (G0 → G1 → G2 → partial-G6 → G7) plus a UI test harness tab and a flywheel v0 (recall ratchet). Deferred: G3 (tokenize/vault), G4 (async monitor), G5 (oracle), G8 (OTel depth), G9 (scale), G10-full — see `TODO-gateway-deferred.md`.*

## Context

The control plane ships `proxy_enforced` as a label with no enforcement behind it (stubs: `internal/synthetic/synthetic.go` 501 `proxy_not_normalized`, `internal/emitter` `emitProxyStub`, `internal/policy/reconcile.go` → `proxy_needed`). This phase builds the separate data-plane service that makes it true, a harness tab that proves redaction behaviorally, and the first turn of the recall-ratchet flywheel.

## Locked decisions

1. **Scope:** MVP slice + harness tab + flywheel v0.
2. **Runtime:** Go, second binary `cmd/air-traffic-gateway`; goroutine worker pools. Control-plane binary stays stdlib-only (enforced by a dep-isolation test). The MVP gateway itself also needs zero third-party deps.
3. **Detectors:** in-process RE2 regex + self-hosted Presidio analyzer sidecar (Docker) behind one `Detector` interface. All local — no cloud inference/compute.
4. **"RL loop" = the recall-ratchet flywheel** (supervised, synthetic-only), not reinforcement learning. Harness traffic is synthetic by construction so it feeds the golden corpus directly.
5. **Ports (claimed in the global registry):** gateway **127.0.0.1:8125**, Presidio **127.0.0.1:8126**. Control plane stays 8122, Vite 5202.

## Build phases

Each phase compiles, tests green, and is demoable on its own.

- **Phase 0 (G0)** — gateway skeleton: config loader (secret-refs only; refuses to start on inline secret via `internal/redact`), `/healthz` `/readyz`, graceful shutdown, slog JSON logging, dep-isolation ratchet test. *Deviation, recorded:* OTel trimmed to slog + spine metrics (OTel depth is G8).
- **Phase 1 (G1)** — pass-through proxy: `POST /v1/messages`, gateway-key auth, credential broker (`env:` refs), byte-faithful forward, SSE pass-through (`io.Pipe`+`http.Flusher`). Plus the **mock upstream** in `internal/synthetic` (`/synthetic/anthropic/v1/messages`, JSON+SSE, capture ring, `X-Harness-Echo` / `X-Harness-Straddle` controls).
- **Phase 2 (G2)** — `Detector` interface, regex recognizers (email, phone, SSN, CC+Luhn, IP, IBAN, MRN), mask + block actions, Anthropic field walk (`system`, `messages[].content` string/blocks), metadata-only audit, golden corpus, log-leak guard test.
- **Phase 3 (partial G6)** — Presidio analyzer client behind the interface; `deploy/presidio/docker-compose.yml` (pinned image, mem limit, healthcheck); ordered chain regex→presidio with overlapping-span merge; per-detector timeout honoring `GATEWAY_FAIL_MODE`.
- **Phase 4 (G7)** — spine integration: observations up (`ops-observation-batch/v1` → `POST /api/observations`, connector `{type:"gateway"}`), leak findings → `POST /api/gateway/leaks` (metadata-only, validated), enforcement heartbeat → `POST /api/gateway/enforcement`, policy pull (`GET /api/policies` → mask/block/detect-only from baseline + `zdr_attested`), pattern pull (`GET /api/gateway/patterns`), `proxy_enforced` honesty flip in `reconcile.go` (fresh heartbeat → `applied_proxy`), staleness drift in `drift.go`, new anthropic `pii_redaction` capability. The 501 admin-surface stubs stay (different concern).
- **Phase 5 (harness)** — `internal/harness` in the control plane (runner worker pool, seeded generator with truth spans + FP traps + straddle cases, behavioral + span scoring, stream reassembly scanner) + `/api/harness/*` routes + `web/src/pages/GatewayHarness.tsx` tab.
- **Phase 6 (flywheel v0)** — miss→corpus promotion (`data/harness/corpus/`), curated pattern-pack proposals with **human approval in the UI**, gateway hot-reload via pattern pull (`atomic.Pointer` swap; also injected to Presidio as `ad_hoc_recognizers`), ratchet series (`data/harness/ratchet.jsonl` + store ring + `detector_recall_ratchet` observation), Sparkline trend.

## Key design resolutions

- **Mock upstream lives in `internal/synthetic`** — new `handleInference` branch inserted after the `_harness` branch, before mode/proxy checks, so adapter mode never breaks it. Captures `{ID, AdapterID, GatewayRequestID, Body, AuthFingerprint (never the credential), Stream, ReceivedAt}` to a store ring; retrieval at `GET /synthetic/{vendor}/_harness/inference`. Scenario overrides honored (free upstream-error testing).
- **Harness lives in the control plane** (`internal/harness` + `/api/harness/*`): UI reaches it through the existing Vite `/api` proxy; scoring joins captures + gateway audit in-process; the harness is a *client* of the gateway (traffic generator, not path component) — production inference never traverses the control plane.
- **Scoring:** recall behavioral = seeded values absent from upstream captures; precision/recall reported = gateway per-request audit spans (offsets/types only, never values; join on `X-Gateway-Request-Id`) vs truth spans, per type and per engine. FP traps (order numbers, semver, UUIDs, invalid-Luhn) must not fire.
- **Retune (honest v0):** pattern packs only — no learned regexes, no NER fine-tuning. Misses matched against a curated candidate library → proposals → human approve → pack version bump + audit event → gateway hot-reload → re-run → ratchet point. Misses with no candidate surface as "needs new recognizer — manual".
- **UI↔backend: polling only** (5s default, 2s while a run is active). Gateway liveness via `GET /api/gateway/status` (heartbeat freshness) — the browser never talks to :8125.
- **Config (env):** `GATEWAY_LISTEN_ADDR` (default `127.0.0.1:8125`), `GATEWAY_UPSTREAMS` (JSON route→`{base_url, credential_ref}`), `GATEWAY_CLIENT_KEYS_REF`, `GATEWAY_DETECTORS` (`regex` | `regex,presidio`), `GATEWAY_PRESIDIO_URL` (default `http://127.0.0.1:8126`), `GATEWAY_DETECTOR_TIMEOUT_MS` (800), `GATEWAY_REDACT_ACTION` (`mask|block|per_policy`), `GATEWAY_FAIL_MODE` (default **closed**), `GATEWAY_CONTROL_PLANE_URL`, `GATEWAY_OBS_PUSH_INTERVAL` (5s), `GATEWAY_POLICY_PULL_INTERVAL` (15s), `GATEWAY_MAX_BODY_BYTES` (10 MB).

## File surface (abridged)

New: `cmd/air-traffic-gateway/main.go`; `internal/gateway/{server,health,proxy,adapter_anthropic,stream,audit,metrics}.go`, `internal/gateway/config/`, `internal/gateway/credbroker/`, `internal/gateway/detect/{detector,regex,presidio}.go`, `internal/gateway/redact/mask.go`, `internal/gateway/spine/{emit,policy_pull}.go`, `internal/gateway/detect/testdata/corpus/`; `deploy/presidio/{docker-compose.yml,README.md}`; control plane `internal/model/gateway.go`, `internal/store/gateway.go`, `internal/server/{routes_gateway,routes_harness}.go`, `internal/synthetic/inference.go`, `internal/harness/{runner,gen,score,streamscan,flywheel,persist}.go`; `web/src/pages/GatewayHarness.tsx`, `web/src/components/RedactionDiff.tsx`; `scripts/e2e-gateway.sh`; `docs/plans/TODO-gateway-deferred.md`.

Modified: `internal/server/server.go` (route registration), `internal/store/store.go` (rings/init), `internal/policy/{reconcile,drift}.go`, `internal/catalog/vendors.go` (anthropic `pii_redaction`), `internal/synthetic/synthetic.go` (inference branch), `cmd/air-traffic-server/main.go` (harness engine + `AIRTRAFFIC_DATA_DIR`), `web/src/App.tsx`, `web/src/components/Layout.tsx`, `web/src/lib/api.ts`, `.gitignore` (`data/`).

## Local ML/RL concerns (the requested honesty section)

- **Presidio on Docker/macOS:** image ≈ 1.5–2.5 GB (bundles spaCy `en_core_web_lg`); resident RAM ≈ 1–1.5 GB (compose `mem_limit: 2g`); cold start 10–45 s (healthcheck gates readiness); per-call latency ~20–150 ms CPU-only vs microseconds for regex — chain order regex-first, 800 ms timeout + `FAIL_MODE` tested.
- **No-GPU ceiling:** local "retune" = pattern packs, context words, thresholds — **yes**. spaCy NER fine-tuning — **no** (flywheel v0 does not claim it; UI copy says so, so the ratchet is never oversold).
- **Ollama LLM-judge: deferred with G10** — the judge only enters at quarantine labeling, which doesn't exist until G4 produces real-traffic misses; harness ground truth makes a judge redundant in this slice.
- **Run-size bounds on one machine:** ≤ 2,000 req/run, concurrency ≤ 8; a Presidio-chain 2,000-request run ≈ 1–3 min at ~10–50 req/s. Store rings cap at 5,000; durable results live in `data/harness/`.
- **Local ALIGNS with the design:** the heavy detector must be self-hosted so PHI stays in-boundary ("sending text to a third party to check whether it contains PII is the same leak you're guarding against"). Cloud DLP is the BAA-gated *exception* (deferred G6 half). All-local is the design's stated posture, not a downgrade.

## Verification

- **Per phase:** G0 boots stateless + refuses inline secrets + dep-isolation test green; G1 byte-faithful round-trip, credential swap proven at the mock, SSE framing intact; G2 corpus floors (regex types ≥ 0.95 recall / ≥ 0.97 precision), traps zero-fire, log-leak scan green; G6-partial: names/addresses masked only with Presidio in chain, fail-open/closed exercised by killing the container; G7: existing screens light up with zero new UI, policy flip propagates without restart, killing the gateway raises drift; harness: deterministic seeds, 100% join coverage, straddled-SSN caught by reassembly; flywheel: crippled pack → miss → proposal → approve → hot-reload → strictly higher recall → two ratchet points.
- **End-to-end (`scripts/e2e-gateway.sh`):** compose up Presidio → boot control plane (8122) → boot gateway (8125, upstream = synthetic mock) → `npm run dev` → `/settings/harness` → run 200 requests under the healthcare baseline (block), flip `zdr_attested` (mask), watch Flight Deck / Observability / Cost / Audit / Drift.
- **Optional Claude Code proof (design §12):** `ANTHROPIC_BASE_URL=http://127.0.0.1:8125` + `ANTHROPIC_AUTH_TOKEN=<gateway key>` against a real upstream credential.
