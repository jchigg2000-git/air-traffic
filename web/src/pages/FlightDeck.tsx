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
import StatusDot from '../components/StatusDot.tsx'
import Sparkline from '../components/Sparkline.tsx'
import { dispMeta } from '../lib/dispositions.ts'

const NOTIONAL_ORG_CAP = 50000

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

  const globalRag: Rag = fleet
    ? fleet.vendors.some((v) => v.worstRag === 'red')
      ? 'red'
      : fleet.vendors.some((v) => v.worstRag === 'amber')
        ? 'amber'
        : 'green'
    : 'amber'

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
          <span
            className="flex items-center gap-2 rounded-full border px-3 py-1.5 text-sm font-medium"
            style={{ borderColor: `color-mix(in srgb, var(--${globalRag === 'green' ? 'green' : globalRag === 'amber' ? 'amber' : 'red'}) 45%, transparent)` }}
          >
            <StatusDot rag={globalRag} pulse />
            {globalRag === 'green' ? 'All systems nominal' : globalRag === 'amber' ? 'Attention' : 'Action required'}
          </span>
          <span className="hidden items-center gap-1.5 rounded-full border border-line bg-panel2 px-3 py-1.5 text-xs text-muted md:flex">
            <span className="pulse-dot inline-block h-2 w-2 rounded-full" style={{ background: 'var(--accent)' }} />
            emitter · 5s
          </span>
          <Link to="/settings/rigor" className="rounded-lg border border-line bg-panel2 px-3 py-1.5 text-sm text-fg transition hover:border-accent">
            Run Policy Sync
          </Link>
          <Link to="/settings/audit" className="hidden rounded-lg border border-line bg-panel2 px-3 py-1.5 text-sm text-muted transition hover:text-fg sm:block">
            Export Audit
          </Link>
          <Link to="/settings/vendors" className="rounded-lg px-3 py-1.5 text-sm font-medium text-bg" style={{ background: 'var(--accent)' }}>
            + Add Vendor
          </Link>
        </div>
      </header>

      {error && (
        <div className="mt-8 panel p-6 text-center text-red">
          Cannot reach the Air-Traffic API. Is the server running on :8122? <br />
          <span className="text-xs text-muted">{String(error)}</span>
        </div>
      )}

      {loading && !error && (
        <div className="mt-8 grid animate-pulse gap-3">
          <div className="h-24 panel" />
          <div className="h-96 panel" />
        </div>
      )}

      {fleet && (
        <>
          {/* KPI strip */}
          <section className="mt-6 grid grid-cols-2 gap-3 md:grid-cols-5">
            <Kpi label="Vendors" value={String(fleet.vendors.length)} sub="surfaces governed" />
            <Kpi
              label="Healthy"
              value={`${Math.round((fleet.healthy / fleet.vendors.length) * 100)}%`}
              sub={`${fleet.healthy}/${fleet.vendors.length} green`}
              rag={fleet.healthy === fleet.vendors.length ? 'green' : 'amber'}
            />
            <Kpi label="Open Drift" value={String(fleet.driftCount)} sub="intent vs actual" rag={fleet.driftCount ? 'amber' : 'green'} />
            <Kpi
              label="Total Spend"
              value={fmtUSD(fleet.totalSpend, { compact: true })}
              sub={`of ${fmtUSD(NOTIONAL_ORG_CAP, { compact: true })} cap`}
              rag={fleet.totalSpend > NOTIONAL_ORG_CAP * 0.9 ? 'red' : fleet.totalSpend > NOTIONAL_ORG_CAP * 0.7 ? 'amber' : 'green'}
            />
            <Kpi label="Throughput" value={String(fleet.obsPerMin)} sub="observations / min" />
          </section>

          {/* Vendor status board */}
          <section className="mt-6 panel overflow-hidden">
            <div className="flex items-center justify-between border-b border-line px-5 py-3">
              <h2 className="text-sm font-semibold tracking-wide text-muted">VENDOR STATUS BOARD</h2>
              <span className="text-xs text-faint">live · updates every 5s</span>
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
              {fleet.vendors.map((v) => (
                <BoardRow key={v.adapter.id} v={v} now={now} />
              ))}
            </div>
          </section>

          {/* Plane bands */}
          <section className="mt-6 grid gap-3 lg:grid-cols-3">
            <PlaneBand fleet={fleet.vendors} plane="developer_workflow" title="Developer Workflow" />
            <PlaneBand fleet={fleet.vendors} plane="data_policy" title="Data Policy" />
            <PlaneBand fleet={fleet.vendors} plane="budget" title="Budget" />
          </section>

          <Legend />
        </>
      )}
    </div>
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
          <StatusDot rag={v.worstRag} pulse={v.worstRag !== 'green'} />
        </span>
        <span className="text-xs" style={{ color: policy.color }}>
          {policy.label}
        </span>
        <span className="text-xs tabular-nums" style={{ color: v.freshnessSec === Infinity ? 'var(--faint)' : `var(--${fr === 'green' ? 'green' : fr === 'amber' ? 'amber' : 'red'})` }}>
          {v.latest ? relativeTime(v.latest.received_at, now) : '—'}
        </span>
        <span className="truncate text-xs text-muted">
          {v.drift.length ? `⚠ ${v.drift[0].capability.replace(/_/g, ' ')}` : 'none'}
        </span>
        <span className="flex justify-end">
          <Sparkline values={series} color={vendorAccent(a.id)} />
        </span>
      </button>

      {open && (
        <div className="border-t border-line bg-panel2 px-5 py-4">
          <div className="mb-3 flex flex-wrap items-center gap-x-6 gap-y-1 text-xs text-muted">
            <span>API <span className="font-mono text-fg">{a.api_version}</span></span>
            <span>Mode <span className="text-fg">{a.mode}</span></span>
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
  const total = green + amber + red || 1
  const top = tiles.sort((a, b) => (a.status === b.status ? 0 : a.status === 'red' ? -1 : b.status === 'red' ? 1 : a.status === 'amber' ? -1 : 1)).slice(0, 3)

  return (
    <div className="panel p-4">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-semibold">{title}</h3>
        <span className="text-[11px] text-faint">{total} signals</span>
      </div>
      <div className="mb-3 flex h-2 overflow-hidden rounded-full bg-panel2">
        <span style={{ width: `${(green / total) * 100}%`, background: 'var(--green)' }} />
        <span style={{ width: `${(amber / total) * 100}%`, background: 'var(--amber)' }} />
        <span style={{ width: `${(red / total) * 100}%`, background: 'var(--red)' }} />
      </div>
      <div className="flex flex-col gap-2">
        {top.map((t, i) => (
          <div key={i} className="flex items-center gap-2.5">
            <VendorGlyph id={t.vendor} size={22} />
            <span className="min-w-0 flex-1">
              <span className="block truncate text-xs text-muted">{titleCase(t.name)}</span>
              <span className="text-sm font-medium tabular-nums">
                {t.unit === 'usd' ? fmtUSD(t.value) : t.unit === 'tokens' ? fmtNum(t.value) : `${fmtNum(t.value)}${t.unit === '%' ? '%' : ''}`}
              </span>
            </span>
            <Sparkline values={t.series} width={64} height={22} color={`var(--${t.status === 'green' ? 'green' : t.status === 'amber' ? 'amber' : 'red'})`} fill={false} />
          </div>
        ))}
        {!top.length && <span className="text-xs text-faint">no metrics yet</span>}
      </div>
    </div>
  )
}

function Legend() {
  const keys = ['vendor_native', 'env_managed', 'proxy_enforced', 'monitor_only', 'unverified', 'unsupported']
  return (
    <div className="mt-6 flex flex-wrap items-center gap-x-5 gap-y-2 panel px-5 py-3 text-xs text-muted">
      <span className="font-semibold text-faint">DISPOSITIONS</span>
      {keys.map((k) => {
        const m = dispMeta(k)
        return (
          <span key={k} className="flex items-center gap-1.5">
            <span style={{ color: m.color }}>{m.glyph}</span>
            {m.label}
          </span>
        )
      })}
      <span className="ml-auto text-faint">Seed-only env config is drift-detected, never shown as “enforced”.</span>
    </div>
  )
}
