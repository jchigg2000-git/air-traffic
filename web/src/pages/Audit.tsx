import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, qk } from '../lib/api.ts'
import PageHeader from '../components/PageHeader.tsx'
import ApiStateBanner from '../components/ApiStateBanner.tsx'
import VendorGlyph from '../components/VendorGlyph.tsx'
import DispositionChip from '../components/DispositionChip.tsx'
import { relativeTime, titleCase } from '../lib/format.ts'
import { useClock } from '../lib/useClock.ts'

export default function Audit() {
  const now = useClock(1000)
  const audit = useQuery({ queryKey: qk.audit, queryFn: api.audit })
  const [filter, setFilter] = useState('')

  const rows = (audit.data ?? []).filter((e) =>
    !filter || (e.action + e.vendor + e.actor + e.resource).toLowerCase().includes(filter.toLowerCase()),
  )

  async function exportSiem() {
    const records = await api.siem()
    const blob = new Blob([JSON.stringify(records, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'air-traffic-audit-siem.json'
    a.click()
    URL.revokeObjectURL(url)
  }

  return (
    <div>
      <PageHeader
        title="Audit Stream"
        subtitle="One normalized cross-vendor audit stream (OTel GenAI semconv shaped). SIEM-ready."
        actions={
          <>
            <input
              type="search"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder="Filter…"
              aria-label="Filter audit events by actor, action, vendor or resource"
              className="rounded-lg border border-line bg-panel2 px-3 py-1.5 text-sm text-fg placeholder:text-faint"
            />
            <button
              onClick={exportSiem}
              aria-label="Export the audit stream in SIEM format"
              className="rounded-lg px-3 py-1.5 text-sm font-medium text-bg"
              style={{ background: 'var(--accent)' }}
            >
              Export SIEM
            </button>
          </>
        }
      />

      <ApiStateBanner error={audit.error} className="mb-4" />
      {/* A CSS grid, not a <table>, so the table semantics are declared
          explicitly — otherwise a screen reader reads 5 unrelated spans per
          event and the column meanings are lost entirely. */}
      <div className="panel overflow-hidden" role="table" aria-label="Normalized cross-vendor audit stream" aria-busy={audit.isLoading}>

        <div role="row" className="grid grid-cols-[1fr_1.4fr_1.2fr_1fr_0.9fr] gap-2 border-b border-line px-5 py-2 text-[10px] font-semibold uppercase tracking-wider text-faint">
          <span role="columnheader">Actor</span>
          <span role="columnheader">Action</span>
          <span role="columnheader">Resource</span>
          <span role="columnheader">Surface</span>
          <span role="columnheader">When</span>
        </div>
        <div className="max-h-[72vh] overflow-auto">
          {rows.map((e) => (
            <div key={e.id} role="row" className="grid grid-cols-[1fr_1.4fr_1.2fr_1fr_0.9fr] items-center gap-2 border-b border-line px-5 py-2 text-sm last:border-0">
              <span role="cell" className="truncate text-muted" title={e.actor}>{e.actor}</span>
              <span role="cell" className="truncate font-mono text-xs">{e.action}</span>
              <span role="cell" className="flex min-w-0 items-center gap-1.5">
                {e.vendor && e.vendor !== 'all' && <VendorGlyph id={e.vendor} size={18} />}
                <span className="truncate text-xs text-muted">{e.resource}</span>
              </span>
              <span role="cell"><DispositionChip disposition={e.control_surface} size="xs" /></span>
              <span role="cell" className="text-xs text-faint" title={`${titleCase(e.plane)} · ${e.request_id}`}>{relativeTime(e.timestamp, now)}</span>
            </div>
          ))}
          {audit.isLoading && <div role="status" aria-label="Loading audit events" className="h-40 animate-pulse" />}
          {audit.data && !rows.length && <div role="status" className="px-5 py-6 text-center text-sm text-faint">No matching audit events.</div>}
        </div>
      </div>
    </div>
  )
}
