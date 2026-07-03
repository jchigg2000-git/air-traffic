# Ratchet-Up Ledger — `landing-page`

**Feature:** The Air-Traffic public landing page — a front-door/product page introducing the
Enterprise AI Control Plane and routing newcomers into the app (Flight Deck / Gateway Harness).
Additive React route `/welcome` in the Vite + React 19 + TS + Tailwind v4 SPA under `web/`.

**Rubric:** v1
**Reigning champion:** Run 1 · Candidate B (bespoke section components / straight-line composition) — **44/50** 👑
**Next must beat:** **44** (strictly greater to dethrone; a tie is a loss)

---

## Rubric v1 (score each axis 1–10, Total /50)

| Axis | What it measures |
|---|---|
| Correctness | Builds & typechecks; route works; tests pass; content factually grounded (16 vendors, 5 dispositions, 3 planes, correct CTAs); truthfulness rule respected (seed-only never shown "enforced"). |
| Architecture fit | Fits the SPA's conventions (tokens, `Brand`/`VendorGlyph`, disposition source of truth, router idiom); skeleton coherent and the decomposition justified. |
| Test coverage | Meaningful tests for the page (renders, all sections, vendor/disposition completeness, CTA targets); runnable via the repo's runner. Prefer *non-circular* assertions (hardcoded ground truth, not data the page also renders). |
| Readability / maintainability | Clear structure; easy to add/edit/reorder a section; naming; no dead weight; copy stated once (no divergence risk). |
| Robustness / efficiency | Responsive; light+dark; reduced-motion respected; no console errors; no needless re-renders or bloat. |

Tie-break between a run's two candidates: axis order left→right (Correctness first).

---

## Scoreboard (sorted by Total desc)

| Run | Date | Candidate | Skeleton archetype | Corr | Arch | Test | Read | Robust | Total | Patch | Result |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 1 | 2026-07-03 | B | Bespoke section components / straight-line composition | 10 | 8 | 9 | 8 | 9 | **44** | `landing-page/run-1-b.patch` | 👑 **SHIPPED** (first run, auto-ship winner) |
| 1 | 2026-07-03 | A | Content-as-data / schema-driven renderer | 10 | 8 | 8 | 8 | 9 | 43 | `landing-page/run-1-a.patch` | lost by 1 (Test axis) |

## Skeletons already used (NO-REPEAT)

- **Bespoke section components / straight-line composition** — one self-contained component per section (`Hero`/`Planes`/`Dispositions`/`VendorWall`/`HowItWorks`/`CtaBand`), each owning its copy+data+markup inline; `Welcome.tsx` composes them literally top-to-bottom. No content registry, no generic renderer. _(Run 1, champion)_
- **Content-as-data / schema-driven renderer** — a typed discriminated-union `LANDING` data array (`content.ts`, zero JSX) fed through a single `SectionRenderer` that `switch`es on `section.kind`; `Welcome.tsx` maps the array. Content ⟂ presentation. _(Run 1)_

## Techniques already used (NO-REPEAT)

- Disposition legend sourced from the shared `web/src/lib/dispositions.ts` (`DISPOSITIONS`/`dispMeta`) — single source of truth, no hardcoded hex.
- Reuse of existing `Brand` + `VendorGlyph` components and the `.grid-fade`/`.pulse-dot`/`.panel` idiom from FlightDeck.
- Semantic Tailwind-v4 color tokens only (light/dark auto-flip); `prefers-reduced-motion` honored.
- vitest + @testing-library/react + jsdom test harness (added to the previously test-less `web/` workspace); `MemoryRouter`-wrapped render tests.
- **Non-circular black-box content tests** — assert canonical vendor names / disposition labels / plane names hardcoded in the test (would catch a content typo), rather than deriving them from the page's own data. _(what won Run 1)_
- Exhaustive `switch` closed with `assertNever` for compile-time section coverage. _(Candidate A; not shipped but recorded)_

---

## Run log (most recent first)

### Run 1 — 2026-07-03 — 👑 shipped Candidate B (44/50)
- **Spec attacked:** public landing page for Air-Traffic (the Enterprise AI Control Plane) at route `/welcome`; front door → CTAs into Flight Deck (`/`) and Gateway Harness (`/settings/harness`). `/` left as FlightDeck.
- **Skeletons tried:** A = content-as-data / schema-driven renderer; B = bespoke section components / straight-line composition. Layout-divergence gate PASSED (map-over-data+switch vs. straight-line composition of N bespoke components — distinguishable with identifiers stripped).
- **Both candidates:** typecheck clean, 5/5 vitest tests pass, production build succeeds; content grounded (16 vendors in exact tiers, 5 dispositions from the shared lib, 3 control planes, correct CTA targets); truthfulness rule honored (proxy = monitor-only until the off-by-default gateway is enabled; seed-only never "enforced"; "not an inline proxy").
- **Scores:** A 43 (Corr10/Arch8/Test8/Read8/Robust9) · B 44 (Corr10/Arch8/Test9/Read8/Robust9). Judged by an independent agent that ran both suites; not self-scored by either builder.
- **Why B won:** the only axis separating them was Test coverage — B's tests hardcode the 16 canonical vendor names / 5 labels / 3 planes as independent black-box DOM assertions (would catch a content typo), whereas A's derive from the same data the page renders (partially circular). All other axes tied.
- **Noted defects (non-blocking):** A — vendor test partially circular. B — `HowItWorks.tsx` hand-recodes the disposition→color/label mapping instead of reading `DISPOSITIONS` (small source-of-truth wobble), and repeats one product claim in three slightly different wordings.
- **Gate outcome:** first run for this feature ⇒ no champion ⇒ winner auto-ships. No strict-beat bar applied (none exists yet). Adversarial "beat-the-champion" subagent intentionally skipped (no champion to defend; judge already ran real tests + was adversarial).
- **Promotion:** B applied to `main` working tree (uncommitted) — 12 files under `web/` + `npm install` regenerated the lockfile; suite re-verified green in the real tree. Both worktrees removed, both branches deleted, both full patches archived.
- **Unverified:** page not screenshotted in a real browser this run (verification was jsdom full-render tests + typecheck + build). Thin 1-point margin was accepted on the judge's reasoning without a second independent judge.
- **Spent skeletons:** both "bespoke straight-line composition" and "content-as-data schema-driven" are now on the NO-REPEAT list — a future run must pick a *third* decomposition (e.g. section-registry-with-hooks, MDX/config-file-driven, or a layout-primitives/slot-composition approach).
