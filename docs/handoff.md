# Handoff — 2026-07-02 (evening) — G6 config-knob slice

## What shipped
- `c00c975` — feat: G6 config-knob proposals + harness try-a-prompt box — 23 files, +1507/−185, ff-merged to `main`, on `origin/main`; branch `gateway-g6-config-knobs` deleted local+remote.

The G6-full pull-forward: **config-shaped proposals** give the free-text miss classes an Approve button. Verified end-to-end (go test green, e2e 9/9 compose, live flywheel produced `deny-PERSON_NAME` from real misses).

Three artifact kinds now flow propose→approve→hot-reload (`model.KindRegex/KindDenyList/KindThreshold`; empty kind = regex for persisted-state back-compat):
- **`deny_list`** — exact terms from missed truth values; free-text types only (`PERSON_NAME`, `ADDRESS`); distributed as Presidio `ad_hoc_recognizers` `deny_list` (fires at score 1.0 — verified against live 2.2.359).
- **`threshold`** — per-type score gate below the 0.40 default, proposed ONLY with probe evidence: the flywheel re-analyzes missed content at threshold 0 (`internal/harness/probe.go`) and proposes a gate one 0.05-step under the lowest seen score. No evidence → no proposal.
- **`context`** — words on regex-kind rules (curated SSN/MRN candidates carry them); Presidio's context enhancer boosts the pattern score.

Touched: `internal/model/{gateway,harness,presidio(new)}.go`, `internal/gateway/detect/{presidio,regex,detector}.go`, `internal/gateway/spine_pull.go`, `internal/harness/{flywheel,probe(new),sample(new),runner}.go`, `internal/server/{server,routes_harness}.go`, `cmd/air-traffic-server/main.go`, `docker-compose.yml` (control-plane gets `AIRTRAFFIC_PRESIDIO_URL`), `web/src/{lib/api.ts,pages/GatewayHarness.tsx}`, tests (`flywheel_test.go`, `pack_kinds_test.go` new), docs (ledger + build-plan status).

**Also built (same batch): try-a-prompt box on the Gateway Harness tab.** `POST /api/harness/sample` (`internal/harness/sample.go`) fires one ad-hoc prompt through the freshest gateway exactly like run traffic (same key + request-id join), polls ≤8s for the gateway report, and returns verdict/redactions/upstream-capture/model-reply — the browser still never talks to the gateway port. Deliberately outside the run lifecycle: no scoring, no corpus promotion, no proposals (user-typed content has no ground truth), usable mid-run. UI panel renders redactions neutrally (never "false positive" — that judgment belongs to scored runs), converts Go byte offsets → UTF-16 for highlighting, and the copy warns samples aren't synthetic-by-construction (they land in the local capture ring). Live-verified: mask verdict, 4 redactions across both engines, masked upstream text, 44ms.

## Also fixed en route (would have blocked anything else)
- **Two live trap FPs** surfaced by the e2e run (latent holes, NOT caused by this diff — payload/gate semantics were byte-identical for the live pack): `US_ITIN` (unmapped Presidio built-in, bypassed guards) claimed the tail of an `ORD-` trap; `LOCATION→ADDRESS` claimed "ORD-970" as an airport code. Fixes: `US_ITIN→SSN` in the shared map (inherits hyphen guard), unmapped built-ins are now DROPPED unless the pack declares the type, `ADDRESS` joined `typeGuards` with `notHyphenAdjacent`. Regression test: `TestPresidioTrapShapesStaySilent`.
- **Stale-pointer upsert bug** in the old `upsertProposals`: map of pointers into `r.proposals` + append realloc = lost updates. Rewritten index-based; guarded by `TestUpsertSurvivesSliceGrowth`.
- **Full-loop test was silently lying**: it seeded a heartbeat override pointing at the httptest gateway, but the REAL gateway heartbeats every 15s advertising unbound-default `127.0.0.1:8125` — freshest wins in `freshGateway`, so run 2 (and anything late in the test) targeted whatever dev stack sat on 8125 (the live compose gateway → 401s), and 401s score as **perfect recall** (no captures = nothing leaked). Fixed: bind the listener first, `GATEWAY_LISTEN_ADDR` = its real addr, heartbeat now truthful, override deleted. Test went 27s → ~3s (was burning the 25s join timeout). The false-green pattern to watch for: recall 1.0 + joined_reports 0.

## Decisions (this session)
- **Honesty model extended, not relaxed**: `manual` rows flip to `superseded` only when a config artifact provably covers the type. Probe-sees-it-above-gate misses (e.g. SSE straddle splits) stay manual — no artifact would fix them. Chain without `presidio` → everything stays manual (approving Presidio config a non-Presidio chain ignores would be a lie).
- **G0 dependency isolation preserved**: harness does NOT import `internal/gateway/detect` (depisolation test enforces). Shared Presidio vocabulary (entity map, 0.40 gate, rune→byte offsets) moved to **`internal/model/presidio.go`** — update anything that says the map lives in `detect/presidio.go:40`.
- **Rejected deny terms stay rejected** (tracked across all deny rows for the type); new terms after a settled row open `deny-<TYPE>-2` etc.
- Live volume observation: pack is **v0** — ship-day `ssn-bare-context` approval never landed in the compose volume (it re-proposed today, correctly, alongside `deny-PERSON_NAME`). The user owns all approvals; nothing was approved this session.

## Don't break
- All items from the previous handoff still stand (`gatewayStaleAfter` ×2, heartbeat honesty, mock-upstream branch order, byte-faithful pass-through, ratchet `corpus_version`, compose images bake source).
- **Shared vocabulary moved**: new engines/PII types join `model.PresidioEntityMap` (`internal/model/presidio.go`) + `typeGuards` (`internal/gateway/detect/detector.go:41`). Unmapped Presidio built-ins are silently dropped now — a new built-in type you WANT must be mapped explicitly.
- **Probe and adapter must stay in step**: `internal/harness/probe.go` deliberately mirrors the detect adapter via the shared model vocabulary; if the adapter grows type-shaping logic beyond that map, mirror it or threshold evidence lies.
- **`Detect`'s request threshold = min(all gates)** then re-filters per type — sending the default gate would starve lowered types of candidates.
- **Deny terms are values by design** (synthetic here) — they live in proposals + pack + `/api/gateway/patterns`, but must NEVER enter audit events or gateway reports (leak-guard tests check audit/reports only).

## Next session: start here
1. The demo moment: the user clicks Approve on `deny-PERSON_NAME` and re-runs to watch the ratchet recover from the replay-amplified name misses.
2. Browser click-through of the Gateway Harness tab (STILL never visually verified — kind chips, deny-term/threshold rendering, and the try-a-prompt panel are build/API-verified only).
3. Or any deferred-ledger item: `docs/plans/TODO-gateway-deferred.md` (G3 vault, G4 async monitor, managed DLP, per-route engine selection, YAML mount, auth on `/api/gateway/*` — now includes the pattern GET, since it distributes deny terms).

## How to verify
- `git log --oneline -1` → `c00c975 feat: G6 config-knob proposals…`; `git status --short` → empty; `go test ./...` all ok (~15s)
- `curl -X POST 127.0.0.1:8122/api/harness/sample -d '{"content":"SSN 123-45-6789"}' -H 'Content-Type: application/json'` → mask verdict + redactions + masked upstream text
- `docker compose ps` → 3 healthy; stack serves this working tree (rebuilt twice this session)
- `E2E_COMPOSE=1 ./scripts/e2e-gateway.sh` → 9/9, trap_fps=0 (NB mutates runtime state)
- `curl -s 127.0.0.1:8122/api/harness/proposals` → `deny-PERSON_NAME` proposed (3 Diego-terms), `manual-PERSON_NAME` superseded, `manual-PHONE` still manual, `ssn-bare-context` proposed with context words
