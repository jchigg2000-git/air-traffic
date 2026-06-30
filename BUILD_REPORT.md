# Build Report — air-traffic
_Generated 2026-06-29 from Claude Code session logs + git history._

**At a glance:** built in ~7.1 active hours across 5 sessions, ~$155, roughly ~100× faster than a solo human estimate. Data is complete — live transcripts and git history both present; no archive logs and no repo rename detected.

## Build sequence
| When (local, −05:00) | Phase | Trigger (commit / subject) |
|---|---|---|
| 2026-06-29 11:52 | Scaffold | `e251528` Initial commit |
| 2026-06-29 17:20 | Spec/assets | `30caaf2` docs: Air-Traffic specs, executive PDFs, brand assets (69 files) |
| 2026-06-29 17:37 | Plan | `d7934ef` docs: Phase 1 & Phase 2 build plans |
| 2026-06-29 17:54 | Backend | `3bd5bda` feat: Phase 1 — synthetic byte-identical vendor control-surface backend |
| 2026-06-29 18:09 | Frontend | `f404481` feat: Phase 2 — control & observability SPA (Flight Deck + config screens) |
| 2026-06-29 18:35 | Polish | `2d8678a` feat(web): real logo-pack brand mark + favicon, settings gear |
| 2026-06-29 18:55 | Feature | `ece88f2` feat(web): vendor pane fills height, per-row toggle, endpoint modal |
| 2026-06-29 18:59 | Feature | `5a4dda9` feat(web): saving an endpoint flips the adapter to proxy mode |
| 2026-06-29 19:20 | Feature | `1e480ae` feat: per-vendor proxy auth (top 6) + default roster (6 on/10 off) |
| 2026-06-29 21:05 | Feature | `396e6ee` feat: cost & usage drill-down + Rigor Console preset narrative |

Build-sequence rows are anchored to git commits; session `first_prompt` records were mostly slash-commands (`/model`, `/clear`, `cd ..`) and added no usable trigger text beyond the commits.

## Build time
| Metric | Value |
|---|---|
| Active wall-clock | 7h 3m (idle gaps >15 min excluded) |
| Sessions | 5 |
| Calendar span | 2026-06-29 16:54 → 2026-06-30 02:27 (UTC) |

## Estimated cost (USD)
| Token class | Tokens | Rate ($/MTok, Opus / Sonnet) | Cost |
|---|---|---|---|
| Input | 86,547 | 5.00 / 3.00 | $0.43 |
| Output | 977,327 | 25.00 / 15.00 | $23.68 |
| Cache write (5m) | 2,391,405 | 6.25 / 3.75 | $14.61 |
| Cache read | 234,065,947 | 0.50 / 0.30 | $116.57 |
| **Total** | | | **$155.29** |

Models: `claude-opus-4-8` (≈$152.95, 98% of spend) + `claude-sonnet-4-6` (≈$2.33). Tokens deduped by `message.id`. Cost summed per class per model, then totaled. Rates source: `claude-api` skill (current). Cache-write = 1.25× input, cache-read = 0.1× input per the skill's caching economics. The dominant line item by far is cache reads (234M tokens) — expected for a long agentic build that re-reads a large cached prefix every turn.

## Human-effort equivalent
| Metric | Value |
|---|---|
| Output | 10 commits, 148 tracked files, ~13,567 net lines (13,717 ins / 150 del) |
| Mid-level dev estimate | ~90 dev-days (~720 h) |
| Actual active build time | 7h 3m |
| Speed-up multiplier | ~100× |
Assumptions: ~150 net committed lines/day including testing & debugging. **Caveat — treat the multiplier as an upper bound:** a large share of the 13.6K net lines is *not* hand-authored logic — `30caaf2` alone is 69 files of specs/PDFs/brand assets, and Phase 1 is explicitly "synthetic byte-identical" vendor fixtures. A scope-based estimate of the genuinely hand-written Go backend + React SPA would land lower, so the true speed-up is meaningfully under 100×; the number is reported transparently, not defended.

## Steering
| Metric | Count |
|---|---|
| User prompts | 51 |
| Course-corrections | 4 |
| Interrupts | 5 |
| Off-task reins (est.) | Low — corrections were scope refinements, not rescues |
The flagged corrections were content/scope tweaks ("use the claude sunburst instead", "condense it for executive [audience]", "dial that back just a hair", "not limited to just those two vendors — full coverage"), i.e. steering the executive-doc narrative and vendor coverage rather than fixing broken code. 5 interrupts against 51 prompts is a clean run.

## Retrospective
This was a one-day sprint that went the distance: from an empty repo at 11:52 to a running zero-dependency Go backend, a React control SPA, and a 16-vendor adapter matrix by 21:05 — about seven active hours of work, almost entirely on Opus 4.8. The shape of the spend tells the story honestly: 98% of the dollars and 234M of the 240M tokens are cache reads, the signature of a long agentic loop steadily re-reading a big cached context as it builds feature on feature. Output tokens (~977K) are the actual writing; everything else is the model holding the whole project in its head turn after turn.

The wheel got yanked only gently. The four course-corrections were all about *presentation* — swap the logo to the Claude sunburst, broaden the executive doc past two vendors, condense for executives, soften the "you must be a CISO to get this" framing — not "you broke the build." That maps to a build where the engineering ran clean and the judgment calls were about how to pitch it. Five interrupts across 51 prompts is a low rein rate.

The one number to distrust is the speed-up. Thirteen thousand net lines sounds heroic until you notice a 69-file commit of PDFs and brand assets and a Phase-1 backend that is, by its own commit message, *synthetic byte-identical* fixtures — generated surface area, not hand-reasoned logic. The ~100× is arithmetic from a stated lines-per-day assumption, and it almost certainly overstates the human-equivalent effort. The defensible claims are the small, traceable ones: 7h 3m active, 5 sessions, 10 commits, ~$155, one calendar day. Those hold up.
