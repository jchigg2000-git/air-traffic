import type { Adapter, ObservationRecord, DriftRecord } from './api.ts'
import { ageSeconds, type Rag } from './format.ts'

// FacetMemberRollup is one cost drill-down row (cost attributed to one user / model / repo
// / project / …), derived from the emitter's kind:"cost_breakdown" observations.
export interface FacetMemberRollup {
  member: string
  label: string
  cost: number
  tokensIn: number
  requests: number
  share: number // % of the vendor's drilled cost
  series: number[]
}

export interface VendorRollup {
  adapter: Adapter
  latest?: ObservationRecord
  freshnessSec: number
  series: Record<string, number[]>
  planeRag: Record<string, Rag>
  worstRag: Rag
  drift: DriftRecord[]
  costLatest: number
  tokensInLatest: number
  tokensOutLatest: number
  capUtil: number
  proxyGaps: number
  envCount: number
  // cost drill-down by dimension (user|model|repo|project|…) → ranked members.
  facets: Record<string, FacetMemberRollup[]>
}

const RANK: Record<Rag, number> = { green: 0, amber: 1, red: 2 }

function worse(a: Rag, b: Rag): Rag {
  return RANK[a] >= RANK[b] ? a : b
}

function asRag(status: string): Rag {
  return status === 'red' ? 'red' : status === 'amber' ? 'amber' : 'green'
}

export interface Fleet {
  vendors: VendorRollup[]
  totalSpend: number
  healthy: number
  driftCount: number
  obsPerMin: number
  tokensInTotal: number
}

export function computeFleet(
  adapters: Adapter[],
  observations: ObservationRecord[],
  drift: DriftRecord[],
  now: number,
): Fleet {
  const byVendor = new Map<string, ObservationRecord[]>()
  for (const o of observations) {
    const arr = byVendor.get(o.connector_instance) ?? []
    arr.push(o)
    byVendor.set(o.connector_instance, arr)
  }

  let recentBatches = 0
  for (const o of observations) if (ageSeconds(o.received_at, now) <= 60) recentBatches++

  const vendors: VendorRollup[] = adapters.map((adapter) => {
    const recs = (byVendor.get(adapter.id) ?? []).slice().sort((a, b) => a.received_at.localeCompare(b.received_at))
    const latest = recs[recs.length - 1]

    // aggregate metric series (cost_breakdown rows are drill-down attribution, not
    // fleet-level metrics — skip them so they can't pollute cost_usd / totals).
    const series: Record<string, number[]> = {}
    for (const r of recs) {
      for (const ob of r.body?.observations ?? []) {
        if (ob.kind === 'cost_breakdown') continue
        const v = ob.signal?.value
        if (typeof v === 'number') {
          ;(series[ob.signal.name] ??= []).push(v)
        }
      }
    }
    for (const k of Object.keys(series)) series[k] = series[k].slice(-24)

    const facets = buildFacets(recs)

    const planeRag: Record<string, Rag> = {}
    let worstRag: Rag = adapter.mode === 'disabled' ? 'amber' : 'green'
    if (latest?.error_count) worstRag = 'amber'
    for (const ob of latest?.body?.observations ?? []) {
      if (ob.kind === 'cost_breakdown') continue
      const plane = String(ob.dimensions?.plane ?? 'other')
      const rag = asRag(ob.signal?.status)
      planeRag[plane] = planeRag[plane] ? worse(planeRag[plane], rag) : rag
      worstRag = worse(worstRag, rag)
    }

    const vendorDrift = drift.filter((d) => d.vendor === adapter.id)
    if (vendorDrift.length) worstRag = worse(worstRag, 'amber')

    const last = (name: string) => {
      const s = series[name]
      return s && s.length ? s[s.length - 1] : 0
    }

    const proxyGaps = adapter.capabilities.filter(
      (c) => c.disposition === 'proxy_enforced' || c.disposition === 'unverified',
    ).length
    const envCount = adapter.capabilities.filter((c) => c.disposition === 'env_managed').length

    return {
      adapter,
      latest,
      freshnessSec: latest ? ageSeconds(latest.received_at, now) : Infinity,
      series,
      planeRag,
      worstRag,
      drift: vendorDrift,
      costLatest: last('cost_usd'),
      tokensInLatest: last('tokens_in'),
      tokensOutLatest: last('tokens_out'),
      capUtil: last('cap_utilization'),
      proxyGaps,
      envCount,
      facets,
    }
  })

  return {
    vendors,
    totalSpend: vendors.reduce((s, v) => s + v.costLatest, 0),
    healthy: vendors.filter((v) => v.worstRag === 'green').length,
    driftCount: drift.length,
    obsPerMin: recentBatches,
    tokensInTotal: vendors.reduce((s, v) => s + v.tokensInLatest, 0),
  }
}

// buildFacets turns a vendor's kind:"cost_breakdown" observations into ranked drill-down
// members per dimension. Each member's `cost` is its latest tick; `series` is its recent
// cost history (for the row sparkline); `share` is its % of the dimension's drilled total.
function buildFacets(recs: ObservationRecord[]): Record<string, FacetMemberRollup[]> {
  type Acc = { label: string; series: number[]; tokensIn: number; requests: number }
  const byDim: Record<string, Record<string, Acc>> = {}
  for (const r of recs) {
    for (const ob of r.body?.observations ?? []) {
      if (ob.kind !== 'cost_breakdown') continue
      const dim = String(ob.dimensions?.cost_facet ?? '')
      const member = String(ob.dimensions?.cost_facet_member ?? '')
      if (!dim || !member) continue
      const acc = (byDim[dim] ??= {})
      const entry = (acc[member] ??= { label: String(ob.dimensions?.cost_facet_label ?? member), series: [], tokensIn: 0, requests: 0 })
      const cost = typeof ob.signal?.value === 'number' ? ob.signal.value : 0
      entry.series.push(cost)
      entry.tokensIn = Number(ob.dimensions?.tokens_in ?? entry.tokensIn)
      entry.requests = Number(ob.dimensions?.requests ?? entry.requests)
    }
  }

  const out: Record<string, FacetMemberRollup[]> = {}
  for (const [dim, members] of Object.entries(byDim)) {
    const rows: FacetMemberRollup[] = Object.entries(members).map(([member, e]) => ({
      member,
      label: e.label,
      cost: e.series[e.series.length - 1] ?? 0,
      tokensIn: e.tokensIn,
      requests: e.requests,
      share: 0,
      series: e.series.slice(-24),
    }))
    const total = rows.reduce((s, m) => s + m.cost, 0) || 1
    for (const m of rows) m.share = (m.cost / total) * 100
    rows.sort((a, b) => b.cost - a.cost)
    out[dim] = rows
  }
  return out
}
