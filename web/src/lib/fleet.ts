import type { Adapter, ObservationRecord, DriftRecord } from './api.ts'
import { ageSeconds, type Rag } from './format.ts'

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

    const series: Record<string, number[]> = {}
    for (const r of recs) {
      for (const ob of r.body?.observations ?? []) {
        const v = ob.signal?.value
        if (typeof v === 'number') {
          ;(series[ob.signal.name] ??= []).push(v)
        }
      }
    }
    for (const k of Object.keys(series)) series[k] = series[k].slice(-24)

    const planeRag: Record<string, Rag> = {}
    let worstRag: Rag = adapter.mode === 'disabled' ? 'amber' : 'green'
    if (latest?.error_count) worstRag = 'amber'
    for (const ob of latest?.body?.observations ?? []) {
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
