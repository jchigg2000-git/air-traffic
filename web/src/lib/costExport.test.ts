import { describe, expect, it } from 'vitest'
import { buildCostSnapshot, snapshotToCSV, snapshotToJSON, exportFilename, SNAPSHOT_NOTE } from './costExport.ts'
import type { Fleet, VendorRollup } from './fleet.ts'
import type { Adapter, VendorCostFacets } from './api.ts'

function adapter(id: string, name: string): Adapter {
  return {
    id,
    display_name: name,
    tier: 1,
    mode: 'synthetic',
    capabilities: [],
  } as unknown as Adapter
}

function vendor(id: string, name: string, cost: number, facets: VendorRollup['facets'] = {}): VendorRollup {
  return {
    adapter: adapter(id, name),
    emitting: true,
    freshnessSec: 1,
    series: {},
    planeRag: {},
    worstRag: 'green',
    drift: [],
    costLatest: cost,
    tokensInLatest: cost * 10,
    tokensOutLatest: cost * 4,
    capUtil: 50,
    proxyGaps: 0,
    envCount: 0,
    facets,
  }
}

const FLEET: Fleet = {
  vendors: [
    vendor('openai', 'OpenAI', 300, {
      user: [
        { member: 'alice@corp', label: 'Alice', cost: 200, tokensIn: 2000, requests: 12, share: 66.6, series: [] },
        { member: 'bob, jr', label: 'Bob, Jr', cost: 100, tokensIn: 1000, requests: 6, share: 33.3, series: [] },
      ],
      model: [{ member: 'gpt-4o', label: 'gpt-4o', cost: 300, tokensIn: 3000, requests: 18, share: 100, series: [] }],
    }),
    vendor('anthropic', 'Anthropic', 150),
    vendor('idle', 'Idle Vendor', 0),
  ],
  totalSpend: 450,
  healthy: 3,
  emittingCount: 3,
  unmatchedEmitters: [],
  driftCount: 0,
  obsPerMin: 5,
  tokensInTotal: 4500,
}

const META: VendorCostFacets = {
  vendor: 'openai',
  supported: [
    { dimension: 'model', label: 'By Model' },
    { dimension: 'user', label: 'By User' },
  ],
  unsupported: [],
}

const NOW = Date.parse('2026-07-16T12:34:56.000Z')

describe('buildCostSnapshot', () => {
  it('rolls up only vendors with spend, ranked high→low', () => {
    const snap = buildCostSnapshot(FLEET, 50000, FLEET.vendors[0], META, NOW)
    expect(snap.vendors.map((v) => v.vendor)).toEqual(['openai', 'anthropic'])
    expect(snap.vendors[0].cost_usd).toBe(300)
    expect(snap.total_spend_usd).toBe(450)
    expect(snap.cap_util_pct).toBeCloseTo(0.9, 5)
    expect(snap.note).toBe(SNAPSHOT_NOTE)
    expect(snap.generated_at).toBe('2026-07-16T12:34:56.000Z')
  })

  it('orders drill-down dimensions by real-API support first', () => {
    const snap = buildCostSnapshot(FLEET, 50000, FLEET.vendors[0], META, NOW)
    expect(snap.drilldown?.vendor).toBe('openai')
    // META lists model before user, so drill-down keys follow that order.
    expect(Object.keys(snap.drilldown!.dimensions)).toEqual(['model', 'user'])
    expect(snap.drilldown!.dimensions.user[0].member).toBe('alice@corp')
  })

  it('emits null drill-down when the selected vendor has no facets', () => {
    const snap = buildCostSnapshot(FLEET, 50000, FLEET.vendors[1], undefined, NOW)
    expect(snap.drilldown).toBeNull()
  })
})

describe('snapshotToJSON', () => {
  it('round-trips to the same structured object', () => {
    const snap = buildCostSnapshot(FLEET, 50000, FLEET.vendors[0], META, NOW)
    expect(JSON.parse(snapshotToJSON(snap))).toEqual(snap)
  })
})

describe('snapshotToCSV', () => {
  it('emits a provenance preamble, a header, fleet rows, and drill-down rows', () => {
    const csv = snapshotToCSV(buildCostSnapshot(FLEET, 50000, FLEET.vendors[0], META, NOW))
    const lines = csv.trimEnd().split('\n')
    expect(lines[0]).toBe('# ' + SNAPSHOT_NOTE)
    expect(lines[1]).toContain('total_spend_usd=450')
    expect(lines[2].split(',')[0]).toBe('scope')
    expect(csv).toContain('fleet,openai,OpenAI')
    expect(csv).toContain('drilldown,openai,OpenAI,model,gpt-4o')
  })

  it('quotes cells containing commas', () => {
    const csv = snapshotToCSV(buildCostSnapshot(FLEET, 50000, FLEET.vendors[0], META, NOW))
    // "Bob, Jr" / "bob, jr" contain commas and must be quoted, not split into columns.
    expect(csv).toContain('"bob, jr","Bob, Jr"')
  })
})

describe('exportFilename', () => {
  it('is timestamped and extension-correct', () => {
    expect(exportFilename('csv', NOW)).toBe('air-traffic-cost-explorer-2026-07-16-12-34-56.csv')
    expect(exportFilename('json', NOW)).toBe('air-traffic-cost-explorer-2026-07-16-12-34-56.json')
  })
})
