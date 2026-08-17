# Phase 2 — Frontend: Control & Observability SPA

> **Goal:** Build the React SPA that **consumes the signals** Air-Traffic's synthetic
> surfaces emit and **controls** them. Landing page is observability-first (the **Flight
> Deck**); configuration (Rigor Console, Policy Editor, Cost Explorer) is treated with equal
> rigor. Served same-origin by the Go binary from `web/dist`. Modifies **only** `air-traffic`.

Status: **BUILT** — this document is a build-history record of the plan as written, not open work.
Every screen below shipped; current status lives in [`../../ROADMAP.md`](../../ROADMAP.md) §2, which
wins on any conflict. · Web dev port `5202` · API `8122`
Stack mirrors it-scorecard: **Vite + React + TypeScript + Tailwind v4 + TanStack Query + React Router**.
Brand: navy `#0B1220` + cyan `#0891B2`/`#38D5EA`; assets in `air-traffic-option-4-logo-pack/`,
vendor glyphs in `air-traffic-option-4-logo-pack/vendor/` (inline path data — no counterfeit logos).

---

## 0. Experience thesis

Air-Traffic is an air-traffic-control tower for enterprise AI. The operator lands on a
**Flight Deck** that shows, at a glance, the live health of every vendor surface — connectivity,
policy sync, observation freshness, and drift from the declared baseline. From there they drill
into configuration screens that are as polished and rigorous as the dashboard: setting a rigor
baseline (Rigor Console), fine-tuning per-vendor controls (Policy Editor), and watching spend
(Cost Explorer). Every control truthfully shows its **disposition** (native / env-managed /
proxy-opt / monitor-only) and, for env-managed, its **enforcement tier** — the UI never overstates
enforcement (Tier-C is never "enforced").

Design bar: this is a portfolio-grade UI. Strong visual hierarchy, motion that communicates
(live-updating tiles, status pulses), dark-mode-first with a light theme, fully responsive,
keyboard-accessible, with real loading / empty / error / success states everywhere.

---

## 1. Routes & information architecture

```
/                      Flight Deck            — observability landing (default)
/settings              Layout (sidebar)
  /settings            Overview               — harness stats, emitter health, recent activity
  /settings/rigor      Rigor Console          — baseline + per-control rigor sliders + coverage marks
  /settings/policy     Policy Editor          — per-vendor expandable control cards (advanced)
  /settings/cost       Cost & Usage Explorer  — spend by vendor/team, velocity, cap alerts
  /settings/vendors    Vendors / Adapters     — mode, scenario, manifest, recorded calls, test
  /settings/observability  Observability      — raw ops-observation-batch/v1 timeline
  /settings/audit      Audit                  — normalized cross-vendor audit stream + SIEM export
*                      → redirect to /
```

`/` is intentionally the observability surface (per the brief). Configuration lives under
`/settings/*` behind the control-plane `Layout`.

---

## 2. Screen specs

### 2.1 Flight Deck (`/`) — observability landing ⭐
Per analysis §8.4 — the marquee screen.

- **Header band:** Air-Traffic logo + tagline; global health pill (green/amber/red rollup);
  live clock + "emitter: ●5s"; theme toggle; quick actions `[Run Policy Sync] [Export Audit] [+ Add Vendor]`.
- **Fleet KPI strip:** total vendors, % healthy, open drift count, total spend vs org cap,
  observations/min — animated counters fed from `/api/observations` + `/api/adapters`.
- **Vendor status board** (the core): one row per adapter —
  `VENDOR` (inline brand glyph + name) · `STATUS` (●green/amber/red) · `POLICY`
  (✓ synced / ⚠ N gaps / ◆ env-mgd / ✗ error) · `OBS FRESH` (relative "14s ago", reddening
  as it ages) · `DRIFT` (none / "seat policy drift" / "ZDR unconfirmed" / "connection failed") ·
  a per-vendor **sparkline** of recent token/cost signal. Row expands to a detail drawer
  (capabilities by plane with disposition chips, last batch, recorded calls).
- **Live plane bands:** three compact panels — Developer-Workflow, Data-Policy, Budget — each
  rolling up its observations into a RAG mini-scorecard (reuse the it-scorecard tile/sparkline
  components in `scorecard/`).
- **Legend:** ● Native ◆ EnvManaged ✚ Proxy(opt) ⚠ Monitor ✗ ConnError.
- **Behavior:** TanStack Query polls `/api/observations` + `/api/adapters` every 5 s; tiles
  update in place with a subtle pulse; freshness counts up live between polls.

### 2.2 Rigor Console (`/settings/rigor`) — simple config
Per analysis §8.1. Primary user: CISO / IT admin / DPO.

- Profile selector chip row: `General SaaS 🔒` · `Fintech 🔒🔒` · `Healthcare 🔒🔒🔒` · `Gov 🔒🔒🔒`
  + vendor scope toggles (OpenAI / Anthropic / Azure / Bedrock / Vertex / ALL).
- Three sections (**Data Policy**, **Developer Workflow**, **Budget**), each control a row with:
  a **rigor slider** (🔒 / 🔒🔒 / 🔒🔒🔒), **coverage marks** (■ Native ◆ EnvManaged ✚ Proxy-opt
  per vendor), and an implementation note. ✚ rows show a confirmation that they need the
  optional gateway (off → behave as monitor-only).
- Footer `[Apply Profile] [Save]` → `PUT /api/policies`; show the returned **coverage report**
  inline (how many controls land native vs env-managed vs proxy-needed vs unverified).

### 2.3 Policy Editor (`/settings/policy`) — advanced config
Per analysis §8.2. Primary user: platform/security engineer.

- Collapsible plane sections (▼ DATA POLICY / ▼ DEVELOPER WORKFLOW / ▼ BUDGET).
- Each control → expandable card: name + rigor label; a **per-vendor sub-grid** (vendor ·
  state toggle · mechanism chip ■/◆/✚/⚠ · note); entity selectors where relevant (e.g. PII:
  ☑ SSN ☑ Email ☑ Phone) and an action selector (● Redact ○ Block ○ Passthrough+Audit);
  a coverage stamp. Edits compose into the policy-as-code doc and `PUT /api/policies`.
- Live "policy diff" preview vs current actual (from `/api/drift`).

### 2.4 Cost & Usage Explorer (`/settings/cost`)
Per analysis §8.3. Primary user: FinOps / eng leadership.

- Header: `TOTAL SPEND $X / $cap` progress bar + MoM delta.
- **By Vendor** bar chart and **By Team** bars with per-team cap indicators (⚠ at >90%).
- **Spend velocity** line chart (28-day, $K/day) built from emitted budget observations.
- **Alerts** list with `[Increase] [Notify] [Block]` actions (Block flagged as proxy-required).
- All charts are hand-rolled SVG (reuse `scorecard/parts.tsx` patterns) — no chart dep.

### 2.5 Vendors / Adapters (`/settings/vendors`)
Operational control of the synthetic surfaces (mirrors it-scorecard Connectors page):
mode selector (disabled/synthetic/proxy), scenario picker (healthy/401/429/timeout/empty/…),
emitter toggle, **manifest viewer** (capabilities grouped by plane with disposition + enforcement
chips), **recorded calls** viewer, and a `[Test]` button. Drives `PATCH /api/adapters/{id}`,
`/api/adapters/{id}/manifest`, `/calls`, `/test`.

### 2.6 Observability (`/settings/observability`) & Audit (`/settings/audit`)
- Observability: batch timeline from `/api/observations` — expandable raw `ops-observation-batch/v1`
  JSON, per-batch observation/error counts, freshness, schema badge.
- Audit: normalized `AuditEvent` table from `/api/audit` (actor/action/resource/plane/vendor/
  control_surface/before/after), filterable; `[Export SIEM]` → `?format=siem`.

---

## 3. Component & lib structure

```
web/
├── package.json            # react, react-dom, react-router-dom, @tanstack/react-query, vite, @tailwindcss/vite, typescript
├── vite.config.ts          # port 5202 strictPort; proxy /api + /synthetic → 127.0.0.1:8122; build → dist/
├── tsconfig.json
├── index.html
└── src/
    ├── main.tsx            # QueryClient + BrowserRouter
    ├── App.tsx             # route tree (above)
    ├── index.css           # Tailwind + CSS vars (dark-first navy/cyan, light theme)
    ├── components/
    │   ├── Layout.tsx          # /settings sidebar + header
    │   ├── TopBar.tsx          # global health pill, clock, theme toggle, quick actions
    │   ├── ThemeToggle.tsx
    │   ├── DispositionChip.tsx # ■◆✚⚠?✗ + color per disposition (+ enforcement tier badge)
    │   ├── VendorGlyph.tsx     # inline vendor SVG path data from logo pack
    │   ├── StatusDot.tsx
    │   └── ui/ { Badge, Card, StatCard, Slider, Toggle, Drawer, Modal, Table }.tsx
    ├── pages/
    │   ├── FlightDeck.tsx
    │   ├── Overview.tsx
    │   ├── RigorConsole.tsx
    │   ├── PolicyEditor.tsx
    │   ├── CostExplorer.tsx
    │   ├── Vendors.tsx
    │   ├── Observability.tsx
    │   ├── Audit.tsx
    │   └── NotFound.tsx
    ├── lib/
    │   ├── api.ts          # typed fetch wrappers + TS interfaces mirroring Go JSON + query keys (qk)
    │   ├── brandIcons.tsx  # vendorID → inline SVG path (from logo pack vendor/ glyphs)
    │   ├── dispositions.ts # disposition → {label,color,glyph}; enforcement → badge
    │   ├── format.ts       # fmt currency/tokens/relative-time; RAG status helpers
    │   └── cn.ts
    └── scorecard/          # reuse it-scorecard tile/sparkline engine
        ├── Tile.tsx
        ├── parts.tsx       # SVG sparkline + bars
        ├── compute.ts      # green/amber/red from value + thresholds + polarity
        └── useLiveData.ts  # React Query hook + live freshness ticker
```

---

## 4. Data flow

```
Go API (:8122)
  GET /api/observations ──┐
  GET /api/adapters     ──┤ TanStack Query (poll 5s, staleTime tuned)
  GET /api/drift        ──┤
  GET /api/audit        ──┘
        │
        ├─ FlightDeck: adapters × latest batch → status board + plane bands (compute.ts → RAG)
        ├─ CostExplorer: budget-plane observations → vendor/team/velocity charts
        ├─ Observability/Audit: raw batches / normalized events
        │
  PUT /api/policies      ← RigorConsole / PolicyEditor (apply baseline/overrides) → coverage report
  PATCH /api/adapters/{id} ← Vendors (mode/scenario/emit)
```

No CORS (same-origin in prod via `web/dist`; Vite proxy in dev).

---

## 5. Visual system

- **Theme:** dark-first. `--bg #0B1220`, `--panel #0F1729`, `--accent #38D5EA`, `--accent-2 #0891B2`,
  `--green #16A34A`, `--amber #D97706`, `--red #DC2626`, `--teal #0891B2`, `--purple #7C3AED`, `--slate #64748B`.
  Light theme via CSS vars + `localStorage` toggle.
- **Disposition is a first-class visual token** — consistent chip/glyph/color everywhere
  (`DispositionChip`), so an operator reads enforcement truthfully at a glance.
- **Motion:** live tiles pulse on update; freshness counters tick; status dots breathe; drawers
  slide. Respect `prefers-reduced-motion`.
- **Density:** Flight Deck is information-dense but scannable (control-tower aesthetic);
  config screens are calmer and more spacious.
- **Polish pass:** after the build, run `/uxrefine` (and `/frontendtailwind`) on a branch for
  spacing/typography/state-coverage/a11y — strictly visual, no behavior change.

---

## 6. Build & serve

- Dev: `cd web && npm install && npm run dev` (Vite on `5202`, proxy → `8122`); `go run ./cmd/air-traffic-server` for the API.
- Prod: `npm run build` → `web/dist`; Go `server.go` SPA-fallback serves `web/dist/index.html`
  for `/` and `/settings/*`, static assets otherwise (mirrors it-scorecard `spaFileServer`).
- `npm run typecheck` clean; `npm run build` clean.

---

## 7. Acceptance criteria

- [ ] `npm run build` + `npm run typecheck` green; SPA served same-origin by the Go binary from `web/dist`.
- [ ] Flight Deck landing shows live vendor status board + KPI strip + plane bands, polling every 5 s, with live freshness and drift.
- [ ] Rigor Console applies all 4 baselines via `PUT /api/policies` and renders the coverage report; coverage marks per vendor are correct.
- [ ] Policy Editor edits per-vendor controls with truthful disposition + enforcement chips; Tier-C never shown as "enforced".
- [ ] Cost Explorer renders spend-by-vendor/team + velocity + cap alerts from emitted budget observations.
- [ ] Vendors page drives mode/scenario/emit and shows manifest + recorded calls; Observability + Audit render live data with SIEM export.
- [ ] Loading / empty / error / success states present on every data surface; dark + light themes; responsive; keyboard-accessible.

## 8. Out of scope (Phase 2)

- Backend changes beyond what the SPA needs (Phase 1 is the contract; add a thin endpoint only if a screen demands it, noted in closeout).
- The optional inference gateway UI.
- Real auth/SSO (stubbed; hardening phase).
- Any repo other than `air-traffic`.
