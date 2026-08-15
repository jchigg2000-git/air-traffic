# air-traffic — repo notes

**`ROADMAP.md` is the SINGLE SOURCE OF TRUTH for execution** — what's left, what's next, and
every phase / acceptance criterion / decision, across all workstreams. On any handoff, **read it
first and follow only it as the plan.** There are deliberately **no other `*_PLAN` or handoff
docs** — the ones that existed (`docs/handoff.md`, `docs/plans/TODO-cost-drilldown.md`) were
consolidated into it (2026-08-07); never recreate them. Put new plan or status content in
`ROADMAP.md`. If any doc's status conflicts with ROADMAP, ROADMAP wins.

**Exception, by design:** `docs/plans/TODO-gateway-deferred.md` and `docs/plans/TODO-vendor-auth.md`
stay standalone rather than folding into `ROADMAP.md` — they are cited by path from live Go/TS
source (`internal/gateway/credbroker/credbroker.go`, `internal/store/store.go`,
`web/src/lib/authSchemas.ts`) and one is rendered directly in the Vendors UI
(`web/src/pages/Vendors.tsx`). `ROADMAP.md` §3 and §5 point at them rather than duplicating their
content; keep them current in place, don't refold them into the roadmap.

**Backlog items are not blockers.** No item under a `BACKLOG` / `PARKED` status gates any other
work unless it carries a `⛔ BLOCKS:` line quoting the owner's instruction from when it was
parked. Absent that line, it is non-blocking. Do not infer blocking from urgency or dependency
order.

`DECISIONS.md` is append-only; supersede rather than rewrite.

Read order for a fresh session: this file → `docs/air-traffic-analysis.md` /
`docs/air-traffic-system-design.md` (strategy/spec, on demand) → `ROADMAP.md`, then
`docs/inference-gateway-design.md` / `docs/inference-gateway-build-plan.md` /
`docs/plans/TODO-gateway-deferred.md` / `docs/plans/TODO-vendor-auth.md` on demand.
