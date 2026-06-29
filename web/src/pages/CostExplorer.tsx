import { useQuery } from '@tanstack/react-query'
import { api, qk } from '../lib/api.ts'
import { computeFleet, type VendorRollup } from '../lib/fleet.ts'
import { useClock } from '../lib/useClock.ts'
import { fmtUSD, fmtNum } from '../lib/format.ts'
import PageHeader from '../components/PageHeader.tsx'
import VendorGlyph, { vendorAccent } from '../components/VendorGlyph.tsx'
import Sparkline from '../components/Sparkline.tsx'

const ORG_CAP = 50000

export default function CostExplorer() {
  const now = useClock(1000)
  const adapters = useQuery({ queryKey: qk.adapters, queryFn: api.adapters })
  const observations = useQuery({ queryKey: qk.observations, queryFn: () => api.observations(600) })

  const fleet = adapters.data && observations.data ? computeFleet(adapters.data, observations.data, [], now) : null

  const byVendor = (fleet?.vendors ?? []).filter((v) => v.costLatest > 0).sort((a, b) => b.costLatest - a.costLatest)
  const maxCost = byVendor.length ? byVendor[0].costLatest : 1
  const velocity = velocitySeries(fleet?.vendors ?? [])
  const alerts = (fleet?.vendors ?? []).filter((v) => v.capUtil >= 80).sort((a, b) => b.capUtil - a.capUtil)
  const totalSpend = fleet?.totalSpend ?? 0
  const pct = Math.min(100, (totalSpend / ORG_CAP) * 100)

  return (
    <div>
      <PageHeader title="Cost & Usage Explorer" subtitle="Spend velocity, per-vendor attribution, and cap utilization across every vendor." />

      {!fleet && <div className="h-64 animate-pulse panel" />}

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
            {/* by vendor */}
            <div className="panel p-5">
              <h3 className="mb-4 text-sm font-semibold">Spend by Vendor</h3>
              <div className="flex flex-col gap-2.5">
                {byVendor.map((v) => (
                  <div key={v.adapter.id} className="flex items-center gap-3">
                    <span className="flex w-40 shrink-0 items-center gap-2">
                      <VendorGlyph id={v.adapter.id} size={22} />
                      <span className="truncate text-sm">{v.adapter.display_name}</span>
                    </span>
                    <span className="h-5 flex-1 overflow-hidden rounded-md bg-panel2">
                      <span className="block h-full rounded-md" style={{ width: `${(v.costLatest / maxCost) * 100}%`, background: vendorAccent(v.adapter.id), opacity: 0.85 }} />
                    </span>
                    <span className="w-20 shrink-0 text-right text-sm font-medium tabular-nums">{fmtUSD(v.costLatest)}</span>
                  </div>
                ))}
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
                    <div key={v.adapter.id} className="flex items-center gap-2 rounded-lg border px-3 py-2" style={{ borderColor: v.capUtil > 92 ? 'var(--red)' : 'var(--amber)' }}>
                      <span style={{ color: v.capUtil > 92 ? 'var(--red)' : 'var(--amber)' }}>⚠</span>
                      <VendorGlyph id={v.adapter.id} size={20} />
                      <span className="flex-1 text-sm">{v.adapter.display_name}</span>
                      <span className="text-sm font-semibold tabular-nums" style={{ color: v.capUtil > 92 ? 'var(--red)' : 'var(--amber)' }}>
                        {v.capUtil.toFixed(0)}%
                      </span>
                    </div>
                  ))}
                  {!alerts.length && <div className="flex items-center gap-2 text-sm text-muted"><span style={{ color: 'var(--green)' }}>✓</span> All vendors under 80% of cap</div>}
                </div>
              </div>
            </div>
          </div>
        </>
      )}
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
