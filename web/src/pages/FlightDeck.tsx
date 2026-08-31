import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api, qk } from '../lib/api.ts'
import { computeFleet, type VendorRollup } from '../lib/fleet.ts'
import { relativeTime, freshnessRag, fmtUSD, fmtNum, titleCase, type Rag } from '../lib/format.ts'
import { useClock } from '../lib/useClock.ts'
import Brand from '../components/Brand.tsx'
import VendorGlyph, { vendorAccent } from '../components/VendorGlyph.tsx'
import DispositionChip from '../components/DispositionChip.tsx'
import StatusDot, { RAG_MEANING } from '../components/StatusDot.tsx'
import Sparkline from '../components/Sparkline.tsx'
import ApiStateBanner from '../components/ApiStateBanner.tsx'

// Demo constant, NOT policy. No baseline is applied at boot, and the real
// per-baseline caps differ ($10K general_saas, $50K fintech, none for
// healthcare/gov_infra — internal/policy/baselines.go). Labelled "notional" on
// screen so neither page asserts fintech's number as this org's cap.
const NOTIONAL_ORG_CAP = 50000
// A last-successful-poll older than this means the board is a frame we can no longer vouch for.
const FEED_STALE_MS = 15000

const RAG_ORDER: Record<Rag, number> = { red: 0, amber: 1, green: 2 }

export default function FlightDeck() {
  const now = useClock(1000)
  const adapters = useQuery({ queryKey: qk.adapters, queryFn: api.adapters })
  const observations = useQuery({ queryKey: qk.observations, queryFn: () => api.observations(600) })
  const drift = useQuery({ queryKey: qk.drift, queryFn: api.drift })

  const loading = adapters.isLoading || observations.isLoading
  const error = adapters.error || observations.error

  const fleet =
    adapters.data && observations.data
      ? computeFleet(adapters.data, observations.data, drift.data ?? [], now)
      : null

  // Only emitting vendors vote on global status. A vendor that is off is not green — it is absent.
  const globalRag: Rag | null = fleet
    ? fleet.vendors.some((v) => v.emitting && v.worstRag === 'red')
      ? 'red'
      : fleet.vendors.some((v) => v.emitting && v.worstRag === 'amber')
        ? 'amber'
        : 'green'
    : null

  // The age of the last SUCCESSFUL poll — the only thing that can disprove "this board is live".
  const lastOk = observations.dataUpdatedAt
  const feedStale = lastOk > 0 && now - lastOk > FEED_STALE_MS
  const degraded = Boolean(error) || feedStale

  // Worst-first: emitting vendors by severity, then off vendors carrying drift, then the rest
  // in catalog order (Array#sort is stable, so ties keep their original ordering).
  const rows = fleet
    ? [...fleet.vendors].sort((a, b) => {
        if (a.emitting !== b.emitting) return a.emitting ? -1 : 1
        if (a.emitting) return RAG_ORDER[a.worstRag] - RAG_ORDER[b.worstRag]
        return (a.drift.length ? 0 : 1) - (b.drift.length ? 0 : 1)
      })
    : []

  return (
    <div className="relative mx-auto min-h-screen max-w-[1500px] px-5 pb-16 pt-5">
      <div className="grid-fade pointer-events-none absolute inset-x-0 top-0 h-64" />

      {/* Header */}
      <header className="relative flex flex-wrap items-center justify-between gap-4">
        <div className="flex items-center gap-4">
          <Brand size={30} />
          <span className="hidden text-sm text-muted sm:inline">Enterprise AI Control Plane · Flight Deck</span>
        </div>
        <div className="flex flex-wrap items-center gap-2.5">
          <FeedPill fleet={Boolean(fleet)} rag={globalRag} degraded={degraded} error={Boolean(error)} lastOk={lastOk} now={now} />
          <Link to="/settings/rigor" className="rounded-lg border border-line bg-panel2 px-3 py-1.5 text-sm text-muted transition hover:text-fg">
            Rigor Console
          </Link>
          <Link
            to="/settings"
            aria-label="Settings"
            title="Settings"
            className="grid h-8 w-8 place-items-center rounded-lg border border-line bg-panel2 text-muted transition hover:border-accent hover:text-fg"
          >
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
              <circle cx="12" cy="12" r="3" />
              <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
            </svg>
          </Link>
        </div>
      </header>

      <ApiStateBanner error={error} hasData={!!fleet} className="mt-6" />

      {loading && !error && (
        <div className="mt-8 grid animate-pulse gap-3">
          <div className="h-24 panel" />
          <div className="h-96 panel" />
        </div>
      )}

      {fleet && (
        // Stale content stays readable but must not read as current.
        <div style={{ opacity: degraded ? 0.6 : 1 }}>
          {/* KPI strip */}
          <section className="mt-6 grid grid-cols-2 gap-3 md:grid-cols-4">
            <Kpi
              label="Healthy"
              value={fleet.emittingCount ? `${Math.round((fleet.healthy / fleet.emittingCount) * 100)}%` : '—'}
              sub={`${fleet.healthy}/${fleet.emittingCount} emitting green`}
              rag={fleet.emittingCount === 0 ? undefined : fleet.healthy === fleet.emittingCount ? 'green' : 'amber'}
            />
            <Kpi label="Open Drift" value={String(fleet.driftCount)} sub="intent vs actual" rag={fleet.driftCount ? 'amber' : 'green'} />
            <Kpi
              label="Total Spend"
              value={fmtUSD(fleet.totalSpend, { compact: true })}
              sub={`of ${fmtUSD(NOTIONAL_ORG_CAP, { compact: true })} notional cap`}
              rag={fleet.totalSpend > NOTIONAL_ORG_CAP * 0.9 ? 'red' : fleet.totalSpend > NOTIONAL_ORG_CAP * 0.7 ? 'amber' : 'green'}
            />
            <Kpi label="Throughput" value={String(fleet.obsPerMin)} sub="obs/min · last 60s" />
          </section>

          {/* Vendor status board */}
          <section className="mt-6 panel overflow-hidden">
            <div className="flex items-center justify-between border-b border-line px-5 py-3">
              <h2 className="text-sm font-semibold tracking-wide text-muted">
                VENDOR STATUS BOARD
                <span className="ml-2 font-normal text-faint">
                  {fleet.emittingCount} emitting · {fleet.vendors.length - fleet.emittingCount} off
                </span>
              </h2>
              <FeedAge error={Boolean(error)} lastOk={lastOk} now={now} />
            </div>
            <div className="grid grid-cols-[1.6fr_0.7fr_1.1fr_0.9fr_1.1fr_0.9fr] items-center gap-2 px-5 py-2 text-[10px] font-semibold uppercase tracking-wider text-faint">
              <span>Vendor</span>
              <span>Status</span>
              <span>Policy</span>
              <span>Obs Fresh</span>
              <span>Drift</span>
              <span className="text-right">Signal</span>
            </div>
            <div>
              {rows.map((v) => (
                <BoardRow key={v.adapter.id} v={v} now={now} />
              ))}
            </div>
            {fleet.unmatchedEmitters.length > 0 && (
              <div className="border-t border-line px-5 py-2 text-[11px] text-faint">
                unlisted emitter: <span className="font-mono">{fleet.unmatchedEmitters.join(', ')}</span> — reporting without an adapter row
              </div>
            )}
          </section>

          {/* Plane bands */}
          <section className="mt-6 grid gap-3 lg:grid-cols-3">
            <PlaneBand fleet={fleet.vendors} plane="developer_workflow" title="Developer Workflow" />
            <PlaneBand fleet={fleet.vendors} plane="data_policy" title="Data Policy" />
            <PlaneBand fleet={fleet.vendors} plane="budget" title="Budget" />
          </section>
        </div>
      )}
    </div>
  )
}

/**
 * The header's liveness claim. Three states, each disprovable by data:
 * connecting (nothing fetched yet) · degraded (error or stale poll) · nominal/attention (live).
 */
function FeedPill({
  fleet,
  rag,
  degraded,
  error,
  lastOk,
  now,
}: {
  fleet: boolean
  rag: Rag | null
  degraded: boolean
  error: boolean
  lastOk: number
  now: number
}) {
  if (!fleet || rag === null) {
    return <span className="rounded-full px-3 py-1.5 text-sm text-faint">connecting…</span>
  }
  if (degraded) {
    return (
      <span className="flex items-center gap-2 rounded-full border border-line px-3 py-1.5 text-sm font-medium text-muted">
        <StatusDot rag="off" />
        {error ? 'API unreachable' : `stale · ${relativeTime(new Date(lastOk).toISOString(), now)}`}
      </span>
    )
  }
  return (
    <span
      className="flex items-center gap-2 rounded-full border px-3 py-1.5 text-sm font-medium"
      style={{ borderColor: `color-mix(in srgb, var(--${rag}) 45%, transparent)` }}
    >
      <StatusDot rag={rag} pulse={rag !== 'green'} />
      {rag === 'green' ? 'All systems nominal' : RAG_MEANING[rag]}
    </span>
  )
}

/** Replaces the old hardcoded "live · updates every 5s" with the real age of the last good poll. */
function FeedAge({ error, lastOk, now }: { error: boolean; lastOk: number; now: number }) {
  if (error) return <span className="text-xs text-red">disconnected</span>
  if (!lastOk) return <span className="text-xs text-faint">—</span>
  const age = (now - lastOk) / 1000
  const rag = freshnessRag(age)
  return (
    <span className="text-xs" style={{ color: `var(--${rag})` }}>
      updated {relativeTime(new Date(lastOk).toISOString(), now)}
    </span>
  )
}

function Kpi({ label, value, sub, rag }: { label: string; value: string; sub: string; rag?: Rag }) {
  const color = rag === 'red' ? 'var(--red)' : rag === 'amber' ? 'var(--amber)' : rag === 'green' ? 'var(--green)' : 'var(--fg)'
  return (
    <div className="panel px-4 py-3">
      <div className="text-[11px] uppercase tracking-wider text-faint">{label}</div>
      <div className="mt-1 text-2xl font-semibold tabular-nums" style={{ color }}>
        {value}
      </div>
      <div className="text-xs text-muted">{sub}</div>
    </div>
  )
}

function BoardRow({ v, now }: { v: VendorRollup; now: number }) {
  const [open, setOpen] = useState(false)
  const a = v.adapter
  const fr = freshnessRag(v.freshnessSec)
  const policy = policyCell(v)
  const series = v.series['cost_usd'] ?? v.series['tokens_in'] ?? []

  return (
    <div className="border-t border-line">
      <button
        onClick={() => setOpen((o) => !o)}
        className="grid w-full grid-cols-[1.6fr_0.7fr_1.1fr_0.9fr_1.1fr_0.9fr] items-center gap-2 px-5 py-3 text-left transition hover:bg-panel2"
        style={{ opacity: v.emitting ? 1 : 0.45 }}
      >
        <span className="flex min-w-0 items-center gap-2.5">
          <VendorGlyph id={a.id} />
          <span className="min-w-0">
            <span className="block truncate text-sm font-medium">{a.display_name}</span>
            <span className="block truncate text-[11px] text-faint">
              {titleCase(a.family)} · Tier {a.tier} · {a.mode}
            </span>
          </span>
        </span>
        <span className="flex items-center gap-2">
          {v.emitting ? (
            // The only dot in the product with no word beside it — the column is too narrow
            // for one — so it has to say what it means itself.
            <StatusDot rag={v.worstRag} pulse={v.worstRag !== 'green'} standalone />
          ) : (
            <>
              <StatusDot rag="off" />
              <span className="text-[11px] text-faint">off</span>
            </>
          )}
        </span>
        {/* An off vendor has no live policy/freshness/signal state to report — say so, don't imply one. */}
        <span className="text-xs" style={{ color: v.emitting ? policy.color : 'var(--faint)' }}>
          {v.emitting ? policy.label : '—'}
        </span>
        <span
          className="text-xs tabular-nums"
          style={{ color: !v.emitting || v.freshnessSec === Infinity ? 'var(--faint)' : `var(--${fr})` }}
        >
          {v.emitting && v.latest ? relativeTime(v.latest.received_at, now) : '—'}
        </span>
        {/* Drift is config-level, so it stays legible even on an off vendor — and it is the most
            actionable cell on the board, so it is no longer the quietest. */}
        <span className="truncate text-xs" style={{ color: v.drift.length ? 'var(--amber)' : 'var(--faint)' }}>
          {v.drift.length ? `⚠ ${v.drift[0].capability.replace(/_/g, ' ')}` : 'none'}
        </span>
        <span className="flex justify-end">
          {v.emitting ? <Sparkline values={series} color={vendorAccent(a.id)} /> : <span className="text-xs text-faint">—</span>}
        </span>
      </button>

      {open && (
        <div className="border-t border-line bg-panel2 px-5 py-4">
          <div className="mb-3 flex flex-wrap items-center gap-x-6 gap-y-1 text-xs text-muted">
            <span>API <span className="font-mono text-fg">{a.api_version}</span></span>
            <span>Mode <span className="text-fg">{a.mode}</span></span>
            <span>Emitting <span className="text-fg">{v.emitting ? 'yes' : 'no'}</span></span>
            <span>Scenario <span className="text-fg">{a.scenario}</span></span>
            <span>BAA <span className="text-fg">{a.baa_signed ? 'signed' : '—'}</span></span>
            <span>Last batch <span className="text-fg">{v.latest?.observation_count ?? 0} obs · {v.latest?.error_count ?? 0} err</span></span>
            <Link to="/settings/vendors" className="ml-auto text-accent hover:underline">
              Manage →
            </Link>
          </div>
          <div className="flex flex-wrap gap-1.5">
            {a.capabilities.slice(0, 18).map((c) => (
              <DispositionChip key={c.key} disposition={c.disposition} enforcement={c.enforcement} size="xs" />
            ))}
            {a.capabilities.length > 18 && <span className="self-center text-xs text-faint">+{a.capabilities.length - 18} more</span>}
          </div>
        </div>
      )}
    </div>
  )
}

function policyCell(v: VendorRollup): { label: string; color: string } {
  if (v.drift.length) return { label: `⚠ ${v.drift.length} gap${v.drift.length > 1 ? 's' : ''}`, color: 'var(--amber)' }
  if (v.adapter.mode === 'disabled') return { label: '✗ disabled', color: 'var(--red)' }
  if (v.proxyGaps) return { label: `✚ ${v.proxyGaps} proxy`, color: 'var(--proxy)' }
  if (v.envCount) return { label: '◆ env-managed', color: 'var(--env)' }
  return { label: '✓ synced', color: 'var(--green)' }
}

interface Tile {
  vendor: string
  name: string
  value: number
  unit: string
  status: Rag
  series: number[]
}

function PlaneBand({ fleet, plane, title }: { fleet: VendorRollup[]; plane: string; title: string }) {
  let green = 0,
    amber = 0,
    red = 0
  const tiles: Tile[] = []
  for (const v of fleet) {
    for (const ob of v.latest?.body?.observations ?? []) {
      if (String(ob.dimensions?.plane) !== plane) continue
      const rag: Rag = ob.signal?.status === 'red' ? 'red' : ob.signal?.status === 'amber' ? 'amber' : 'green'
      if (rag === 'green') green++
      else if (rag === 'amber') amber++
      else red++
      if (ob.kind === 'metric' && typeof ob.signal.value === 'number') {
        tiles.push({ vendor: v.adapter.id, name: ob.signal.name, value: ob.signal.value, unit: ob.signal.unit, status: rag, series: v.series[ob.signal.name] ?? [] })
      }
    }
  }
  // Real total, before the divide-by-zero guard — so an empty plane can be told from a full one.
  const total = green + amber + red
  const denom = total || 1
  const top = tiles.sort((a, b) => (a.status === b.status ? 0 : a.status === 'red' ? -1 : b.status === 'red' ? 1 : a.status === 'amber' ? -1 : 1)).slice(0, 3)

  return (
    <div className="panel p-4">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-semibold">{title}</h3>
        <span className="text-[11px] text-faint">{total} signals</span>
      </div>
      {total > 0 && (
        <div className="mb-3 flex h-2 overflow-hidden rounded-full bg-panel2">
          <span style={{ width: `${(green / denom) * 100}%`, background: 'var(--green)' }} />
          <span style={{ width: `${(amber / denom) * 100}%`, background: 'var(--amber)' }} />
          <span style={{ width: `${(red / denom) * 100}%`, background: 'var(--red)' }} />
        </div>
      )}
      <div className="flex flex-col gap-2">
        {top.map((t, i) => (
          <div key={i} className="flex items-center gap-2.5">
            <VendorGlyph id={t.vendor} size={22} />
            <span className="min-w-0 flex-1">
              <span className="block truncate text-xs text-muted">{titleCase(t.name)}</span>
              <span className="text-sm font-medium tabular-nums" style={{ color: t.status === 'green' ? undefined : `var(--${t.status})` }}>
                {t.unit === 'usd' ? fmtUSD(t.value) : t.unit === 'tokens' ? fmtNum(t.value) : `${fmtNum(t.value)}${t.unit === '%' ? '%' : ''}`}
              </span>
            </span>
          </div>
        ))}
        {/* "no metrics yet" used to render next to hundreds of live state signals. Say which it is. */}
        {!top.length && (
          <span className="text-xs text-faint">{total > 0 ? `${total} state signals · no numeric metrics` : 'no signals'}</span>
        )}
      </div>
    </div>
  )
}
