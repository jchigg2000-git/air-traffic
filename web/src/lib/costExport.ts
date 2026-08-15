import type { Fleet, VendorRollup } from './fleet.ts'
import type { VendorCostFacets } from './api.ts'

// costExport turns the CostExplorer's client-derived view into downloadable CSV / JSON.
//
// Honesty posture (see CostExplorer): every cost row is derived client-side from live
// observations via computeFleet, so an export is a point-in-time snapshot of the current
// query window — NOT a canonical server billing report. That caveat is carried in the
// snapshot `note` and CSV header comment so a spreadsheet never loses the provenance.
export const SNAPSHOT_NOTE =
  'Client-side snapshot derived from the current query window via computeFleet — a point-in-time view of live observations, not a canonical server billing report.'

export interface CostSnapshotVendor {
  vendor: string
  vendor_name: string
  cost_usd: number
  tokens_in: number
  tokens_out: number
  cap_util_pct: number
  status: string
}

export interface CostSnapshotDrillMember {
  member: string
  label: string
  cost_usd: number
  tokens_in: number
  requests: number
  share_pct: number
}

export interface CostSnapshot {
  generated_at: string
  note: string
  org_cap_usd: number
  total_spend_usd: number
  cap_util_pct: number
  tokens_in_total: number
  vendors: CostSnapshotVendor[]
  drilldown: {
    vendor: string
    vendor_name: string
    dimensions: Record<string, CostSnapshotDrillMember[]>
  } | null
}

// buildCostSnapshot assembles the structured export payload from the same values the
// CostExplorer page renders: the fleet rollup + the selected vendor's facet drill-down.
export function buildCostSnapshot(
  fleet: Fleet,
  orgCap: number,
  selected?: VendorRollup,
  meta?: VendorCostFacets,
  now: number = Date.now(),
): CostSnapshot {
  const vendors: CostSnapshotVendor[] = fleet.vendors
    .filter((v) => v.costLatest > 0)
    .sort((a, b) => b.costLatest - a.costLatest)
    .map((v) => ({
      vendor: v.adapter.id,
      vendor_name: v.adapter.display_name,
      cost_usd: v.costLatest,
      tokens_in: v.tokensInLatest,
      tokens_out: v.tokensOutLatest,
      cap_util_pct: v.capUtil,
      // An off vendor exports as 'off', never as its (meaningless) rollup colour — otherwise the
      // CSV persists the same green-when-silent lie the board used to show.
      status: v.emitting ? v.worstRag : 'off',
    }))

  let drilldown: CostSnapshot['drilldown'] = null
  if (selected && Object.keys(selected.facets).length > 0) {
    // dimension order mirrors the UI: real-API-supported dimensions first, then extras.
    const present = Object.keys(selected.facets)
    const ordered = (meta?.supported ?? []).map((s) => s.dimension).filter((d) => present.includes(d))
    const dims = [...ordered, ...present.filter((d) => !ordered.includes(d))]
    const dimensions: Record<string, CostSnapshotDrillMember[]> = {}
    for (const d of dims) {
      dimensions[d] = (selected.facets[d] ?? []).map((m) => ({
        member: m.member,
        label: m.label,
        cost_usd: m.cost,
        tokens_in: m.tokensIn,
        requests: m.requests,
        share_pct: m.share,
      }))
    }
    drilldown = { vendor: selected.adapter.id, vendor_name: selected.adapter.display_name, dimensions }
  }

  return {
    generated_at: new Date(now).toISOString(),
    note: SNAPSHOT_NOTE,
    org_cap_usd: orgCap,
    total_spend_usd: fleet.totalSpend,
    cap_util_pct: orgCap > 0 ? (fleet.totalSpend / orgCap) * 100 : 0,
    tokens_in_total: fleet.tokensInTotal,
    vendors,
    drilldown,
  }
}

export function snapshotToJSON(snap: CostSnapshot): string {
  return JSON.stringify(snap, null, 2)
}

// snapshotToCSV emits one long-format table so both the fleet rollup and the selected
// vendor's drill-down land in a single spreadsheet, distinguished by the `scope` column.
export function snapshotToCSV(snap: CostSnapshot): string {
  const header = [
    'scope',
    'vendor',
    'vendor_name',
    'dimension',
    'member',
    'member_label',
    'cost_usd',
    'tokens_in',
    'tokens_out',
    'requests',
    'share_pct',
    'cap_util_pct',
  ]
  const rows: string[][] = [header]

  for (const v of snap.vendors) {
    rows.push([
      'fleet',
      v.vendor,
      v.vendor_name,
      '',
      '',
      '',
      num(v.cost_usd),
      num(v.tokens_in),
      num(v.tokens_out),
      '',
      '',
      num(v.cap_util_pct),
    ])
  }

  if (snap.drilldown) {
    for (const [dim, members] of Object.entries(snap.drilldown.dimensions)) {
      for (const m of members) {
        rows.push([
          'drilldown',
          snap.drilldown.vendor,
          snap.drilldown.vendor_name,
          dim,
          m.member,
          m.label,
          num(m.cost_usd),
          num(m.tokens_in),
          '',
          num(m.requests),
          num(m.share_pct),
          '',
        ])
      }
    }
  }

  // A leading `#` comment line carries the provenance caveat; spreadsheets ignore it and
  // FinOps tools that don't can be told to skip 1 header line.
  const preamble = `# ${snap.note}\n# generated_at=${snap.generated_at} total_spend_usd=${num(snap.total_spend_usd)} org_cap_usd=${num(snap.org_cap_usd)}\n`
  return preamble + rows.map((r) => r.map(csvCell).join(',')).join('\n') + '\n'
}

// num renders a numeric cell plainly (no currency symbol / grouping) and trims float noise
// to at most 4 decimals so exports stay spreadsheet-clean and diff-stable.
function num(n: number): string {
  if (!Number.isFinite(n)) return ''
  return String(Math.round(n * 1e4) / 1e4)
}

function csvCell(s: string): string {
  return /[",\n]/.test(s) ? '"' + s.replace(/"/g, '""') + '"' : s
}

// downloadBlob mirrors the Audit page's client-side export pattern (Blob → object URL →
// synthetic anchor click) so the two export surfaces behave identically.
export function downloadBlob(content: string, filename: string, mime: string): void {
  const blob = new Blob([content], { type: mime })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

// exportFilename builds a stable, timestamped, spreadsheet-friendly filename.
export function exportFilename(ext: 'csv' | 'json', now: number = Date.now()): string {
  const stamp = new Date(now).toISOString().slice(0, 19).replace(/[:T]/g, '-')
  return `air-traffic-cost-explorer-${stamp}.${ext}`
}
