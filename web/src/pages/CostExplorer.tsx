import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, qk, type VendorCostFacets } from '../lib/api.ts'
import { computeFleet, type VendorRollup } from '../lib/fleet.ts'
import { useClock } from '../lib/useClock.ts'
import { fmtUSD, fmtNum, titleCase } from '../lib/format.ts'
import { buildCostSnapshot, snapshotToCSV, snapshotToJSON, downloadBlob, exportFilename } from '../lib/costExport.ts'
import PageHeader from '../components/PageHeader.tsx'
import ApiStateBanner from '../components/ApiStateBanner.tsx'
import VendorGlyph, { vendorAccent } from '../components/VendorGlyph.tsx'
import Sparkline from '../components/Sparkline.tsx'

const ORG_CAP = 50000

export default function CostExplorer() {
  const now = useClock(1000)
  const adapters = useQuery({ queryKey: qk.adapters, queryFn: api.adapters })
  const observations = useQuery({ queryKey: qk.observations, queryFn: () => api.observations(600) })
  const costFacets = useQuery({ queryKey: qk.costFacets, queryFn: api.costFacets })
  const [selectedId, setSelectedId] = useState<string | null>(null)

  // costFacets was silently excluded — a facet-fetch failure rendered as "no drill-down data".
  const error = adapters.error || observations.error || costFacets.error

  const fleet = adapters.data && observations.data ? computeFleet(adapters.data, observations.data, [], now) : null

  const byVendor = (fleet?.vendors ?? []).filter((v) => v.costLatest > 0).sort((a, b) => b.costLatest - a.costLatest)
  const maxCost = byVendor.length ? byVendor[0].costLatest : 1
  const velocity = velocitySeries(fleet?.vendors ?? [])
  const alerts = (fleet?.vendors ?? []).filter((v) => v.capUtil >= 80).sort((a, b) => b.capUtil - a.capUtil)
  const totalSpend = fleet?.totalSpend ?? 0
  const pct = Math.min(100, (totalSpend / ORG_CAP) * 100)

  // selected vendor for drill-down — defaults to the top spender that actually has facets.
  const selected =
    byVendor.find((v) => v.adapter.id === selectedId) ??
    byVendor.find((v) => Object.keys(v.facets).length > 0) ??
    byVendor[0]

  function exportSnapshot(format: 'csv' | 'json') {
    if (!fleet) return
    const snap = buildCostSnapshot(fleet, ORG_CAP, selected, selected ? costFacets.data?.[selected.adapter.id] : undefined, Date.now())
    if (format === 'csv') {
      downloadBlob(snapshotToCSV(snap), exportFilename('csv'), 'text/csv;charset=utf-8')
    } else {
      downloadBlob(snapshotToJSON(snap), exportFilename('json'), 'application/json')
    }
  }

  return (
    <div>
      <PageHeader
        title="Cost & Usage Explorer"
        subtitle="Spend velocity, per-vendor attribution, and cap utilization — then drill into any vendor by user, model, repo, project, and more."
        actions={
          <>
            <button
              onClick={() => exportSnapshot('csv')}
              disabled={!fleet}
              className="rounded-lg border border-line bg-panel2 px-3 py-1.5 text-sm font-medium text-fg transition hover:bg-panel disabled:opacity-40"
              title="Download the current fleet rollup + selected drill-down as CSV (point-in-time snapshot)"
            >
              Export CSV
            </button>
            <button
              onClick={() => exportSnapshot('json')}
              disabled={!fleet}
              className="rounded-lg px-3 py-1.5 text-sm font-medium text-bg transition disabled:opacity-40"
              style={{ background: 'var(--accent)' }}
              title="Download the current fleet rollup + selected drill-down as JSON (point-in-time snapshot)"
            >
              Export JSON
            </button>
          </>
        }
      />

      <ApiStateBanner error={error} className="mb-4" />

      {!fleet && !error && <div className="h-64 animate-pulse panel" />}

      {fleet && (
        <>
          {/* total spend header */}
          <div className="mb-4 panel p-5">
            <div className="flex flex-wrap items-end justify-between gap-2">
              <div>
                <div className="text-[11px] uppercase tracking-wider text-faint">Total spend (rolling)</div>
                <div className="text-3xl font-semibold tabular-nums">
                  {fmtUSD(totalSpend)} <span className="text-base font-normal text-muted">/ {fmtUSD(ORG_CAP)}</span>
                </div>
              </div>
              <div className="text-right">
                <div className="text-2xl font-semibold tabular-nums" style={{ color: pct > 90 ? 'var(--red)' : pct > 70 ? 'var(--amber)' : 'var(--green)' }}>
                  {pct.toFixed(1)}%
                </div>
                <div className="text-xs text-muted">{fmtNum(fleet.tokensInTotal)} input tokens/window</div>
              </div>
            </div>
            <div className="mt-3 flex h-3 overflow-hidden rounded-full bg-panel2">
              <span style={{ width: `${pct}%`, background: pct > 90 ? 'var(--red)' : pct > 70 ? 'var(--amber)' : 'var(--accent)' }} />
            </div>
          </div>

          <div className="grid gap-4 lg:grid-cols-[1.4fr_1fr]">
            {/* by vendor — click a row to drill in */}
            <div className="panel p-5">
              <div className="mb-4 flex items-center justify-between">
                <h3 className="text-sm font-semibold">Spend by Vendor</h3>
                <span className="text-[11px] text-faint">select to drill down ↓</span>
              </div>
              <div className="flex flex-col gap-1">
                {byVendor.map((v) => {
                  const isSel = selected?.adapter.id === v.adapter.id
                  const drillable = Object.keys(v.facets).length > 0
                  return (
                    <button
                      key={v.adapter.id}
                      onClick={() => setSelectedId(v.adapter.id)}
                      className="flex items-center gap-3 rounded-lg border px-2 py-1.5 text-left transition hover:bg-panel2"
                      style={{ borderColor: isSel ? 'var(--accent)' : 'transparent' }}
                    >
                      <span className="flex w-40 shrink-0 items-center gap-2">
                        <VendorGlyph id={v.adapter.id} size={22} />
                        <span className="truncate text-sm">{v.adapter.display_name}</span>
                      </span>
                      <span className="h-5 flex-1 overflow-hidden rounded-md bg-panel2">
                        <span className="block h-full rounded-md" style={{ width: `${(v.costLatest / maxCost) * 100}%`, background: vendorAccent(v.adapter.id), opacity: 0.85 }} />
                      </span>
                      <span className="w-20 shrink-0 text-right text-sm font-medium tabular-nums">{fmtUSD(v.costLatest)}</span>
                      <span className="w-4 shrink-0 text-center text-xs" style={{ color: isSel ? 'var(--accent)' : 'var(--faint)' }} title={drillable ? 'Drill-down available' : 'No drill-down dimensions'}>
                        {drillable ? '▸' : '·'}
                      </span>
                    </button>
                  )
                })}
                {!byVendor.length && <span className="text-xs text-faint">no spend signal yet</span>}
              </div>
            </div>

            {/* velocity + alerts */}
            <div className="flex flex-col gap-4">
              <div className="panel p-5">
                <h3 className="mb-2 text-sm font-semibold">Spend Velocity</h3>
                <Sparkline values={velocity} width={360} height={80} color="var(--accent)" />
                <div className="mt-1 text-xs text-muted">aggregate $/window across all vendors</div>
              </div>

              <div className="panel p-5">
                <h3 className="mb-3 text-sm font-semibold">Cap Alerts</h3>
                <div className="flex flex-col gap-2">
                  {alerts.map((v) => (
                    <div key={v.adapter.id} className="flex items-center gap-2 rounded-lg border px-3 py-2" style={{ borderColor: v.capUtil > 90 ? 'var(--red)' : 'var(--amber)' }}>
                      <span style={{ color: v.capUtil > 90 ? 'var(--red)' : 'var(--amber)' }}>⚠</span>
                      <VendorGlyph id={v.adapter.id} size={20} />
                      <span className="flex-1 text-sm">{v.adapter.display_name}</span>
                      <span className="text-sm font-semibold tabular-nums" style={{ color: v.capUtil > 90 ? 'var(--red)' : 'var(--amber)' }}>
                        {v.capUtil.toFixed(0)}%
                      </span>
                    </div>
                  ))}
                  {!alerts.length && <div className="flex items-center gap-2 text-sm text-muted"><span style={{ color: 'var(--green)' }}>✓</span> All vendors under 80% of cap</div>}
                </div>
              </div>
            </div>
          </div>

          {selected && <VendorDrilldown key={selected.adapter.id} v={selected} meta={costFacets.data?.[selected.adapter.id]} />}
        </>
      )}
    </div>
  )
}

function VendorDrilldown({ v, meta }: { v: VendorRollup; meta?: VendorCostFacets }) {
  // dimension order: supported-metadata order first (only those with emitted data), then any extras.
  const present = Object.keys(v.facets)
  const ordered = (meta?.supported ?? []).map((s) => s.dimension).filter((d) => present.includes(d))
  const dims = [...ordered, ...present.filter((d) => !ordered.includes(d))]

  const [dim, setDim] = useState<string | null>(null)
  const active = dim && dims.includes(dim) ? dim : (dims[0] ?? '')
  const members = v.facets[active] ?? []
  const max = members.length ? Math.max(...members.map((m) => m.cost)) : 1
  const activeMeta = meta?.supported.find((s) => s.dimension === active)
  const labelOf = (d: string) =>
    meta?.supported.find((s) => s.dimension === d)?.label ?? `By ${titleCase(d)}`

  return (
    <div className="mt-4 panel p-5">
      <div className="mb-3 flex flex-wrap items-center gap-3">
        <VendorGlyph id={v.adapter.id} size={30} />
        <div className="min-w-0">
          <div className="text-sm font-semibold">{v.adapter.display_name} · cost drill-down</div>
          <div className="text-xs text-muted">
            {fmtUSD(v.costLatest)} rolling · attributed across {dims.length} dimension{dims.length === 1 ? '' : 's'} its real billing API exposes
          </div>
        </div>
      </div>

      {!dims.length && (
        <div className="rounded-lg border border-line bg-panel2 p-3 text-sm text-muted">
          {v.adapter.display_name}’s real billing/usage API exposes no cost-attribution dimensions — spend is reported only at the org level.
          {meta?.unsupported?.length ? <UnsupportedNote items={meta.unsupported} /> : null}
        </div>
      )}

      {dims.length > 0 && (
        <>
          {/* dimension tabs */}
          <div className="mb-3 flex flex-wrap gap-1.5">
            {dims.map((d) => (
              <button
                key={d}
                onClick={() => setDim(d)}
                className="rounded-lg border px-3 py-1.5 text-xs font-medium transition"
                style={{
                  borderColor: d === active ? 'var(--accent)' : 'var(--line)',
                  background: d === active ? 'color-mix(in srgb, var(--accent2) 14%, transparent)' : 'transparent',
                  color: d === active ? 'var(--fg)' : 'var(--muted)',
                }}
              >
                {labelOf(d)}
                <span className="ml-1.5 text-faint">{(v.facets[d] ?? []).length}</span>
              </button>
            ))}
          </div>

          {/* real-API provenance for the active dimension */}
          {activeMeta && (
            <div className="mb-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-faint">
              {activeMeta.real_param && <span>Real API · <span className="font-mono text-muted">{activeMeta.real_param}</span></span>}
              {activeMeta.endpoint && <span className="font-mono text-muted">{activeMeta.endpoint}</span>}
              {activeMeta.reason && <span className="text-muted">· {activeMeta.reason}</span>}
            </div>
          )}

          {/* breakdown rows */}
          <div className="flex flex-col gap-2">
            {members.map((m) => (
              <div key={m.member} className="flex items-center gap-3">
                <span className="w-44 shrink-0 truncate text-sm" title={m.member}>{m.label}</span>
                <span className="h-5 flex-1 overflow-hidden rounded-md bg-panel2">
                  <span className="block h-full rounded-md" style={{ width: `${(m.cost / max) * 100}%`, background: vendorAccent(v.adapter.id), opacity: 0.85 }} />
                </span>
                <span className="w-16 shrink-0 text-right text-[11px] tabular-nums text-faint">{fmtNum(m.tokensIn)} tok</span>
                <span className="w-12 shrink-0 text-right text-[11px] tabular-nums text-muted">{m.share.toFixed(0)}%</span>
                <span className="w-20 shrink-0 text-right text-sm font-medium tabular-nums">{fmtUSD(m.cost)}</span>
                <span className="hidden shrink-0 sm:block"><Sparkline values={m.series} width={64} height={22} color={vendorAccent(v.adapter.id)} fill={false} /></span>
              </div>
            ))}
            {!members.length && <span className="text-xs text-faint">no breakdown signal yet for {labelOf(active)}</span>}
          </div>

          {meta?.unsupported?.length ? <UnsupportedNote items={meta.unsupported} /> : null}
        </>
      )}
    </div>
  )
}

function UnsupportedNote({ items }: { items: VendorCostFacets['unsupported'] }) {
  return (
    <div className="mt-4 border-t border-line pt-3 text-[11px] leading-relaxed text-faint">
      <span className="font-semibold uppercase tracking-wider">No native breakdown</span>
      <span className="ml-2">
        {items.map((u, i) => (
          <span key={u.dimension}>
            {i > 0 && ' · '}
            <span className="text-muted">{u.label}</span>
            {u.reason ? ` — ${u.reason}` : ''}
          </span>
        ))}
      </span>
    </div>
  )
}

function velocitySeries(vendors: VendorRollup[]): number[] {
  const seriesList = vendors.map((v) => v.series['cost_usd']).filter((s): s is number[] => !!s && s.length >= 2)
  if (!seriesList.length) return []
  const len = Math.min(...seriesList.map((s) => s.length))
  const out: number[] = []
  for (let i = 0; i < len; i++) {
    let sum = 0
    for (const s of seriesList) sum += s[s.length - len + i]
    out.push(sum)
  }
  return out
}
