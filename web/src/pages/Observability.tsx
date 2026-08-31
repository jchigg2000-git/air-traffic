import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, qk, type ObservationRecord } from '../lib/api.ts'
import PageHeader from '../components/PageHeader.tsx'
import ApiStateBanner from '../components/ApiStateBanner.tsx'
import VendorGlyph from '../components/VendorGlyph.tsx'
import { relativeTime } from '../lib/format.ts'
import { useClock } from '../lib/useClock.ts'

export default function Observability() {
  const now = useClock(1000)
  const obs = useQuery({ queryKey: qk.observations, queryFn: () => api.observations(200) })
  const [open, setOpen] = useState<number | null>(null)

  return (
    <div>
      <PageHeader
        title="Observability"
        subtitle="Raw ops-observation-batch/v1 batches as emitted — the contract every screen consumes."
        actions={<span className="rounded-md border border-line bg-panel2 px-2 py-1 font-mono text-[11px] text-accent">ops-observation-batch/v1</span>}
      />

      <ApiStateBanner error={obs.error} hasData={!!obs.data} className="mb-4" />
      {/* CSS grid, not a <table> — the table semantics are declared so a
          screen reader keeps the column meanings. */}
      <div className="panel overflow-hidden" role="table" aria-label="Observation batches, newest first" aria-busy={obs.isLoading}>

        <div role="row" className="grid grid-cols-[1.4fr_0.8fr_0.8fr_1fr_0.4fr] gap-2 border-b border-line px-5 py-2 text-[10px] font-semibold uppercase tracking-wider text-faint">
          <span role="columnheader">Vendor</span>
          <span role="columnheader">Obs</span>
          <span role="columnheader">Errors</span>
          <span role="columnheader">Collected</span>
          <span role="columnheader" aria-label="Expand batch" />
        </div>
        {obs.data?.map((r) => (
          <Row key={r.id} r={r} now={now} open={open === r.id} onToggle={() => setOpen(open === r.id ? null : r.id)} />
        ))}
        {obs.isLoading && <div role="status" aria-label="Loading observation batches" className="h-40 animate-pulse" />}
        {obs.data && !obs.data.length && <div role="status" className="px-5 py-6 text-center text-sm text-faint">No batches yet.</div>}
      </div>
    </div>
  )
}

function Row({ r, now, open, onToggle }: { r: ObservationRecord; now: number; open: boolean; onToggle: () => void }) {
  return (
    <div className="border-b border-line last:border-0">
      <button
        onClick={onToggle}
        aria-expanded={open}
        aria-label={`${open ? 'Hide' : 'Show'} the raw batch from ${r.connector_instance}`}
        className="grid w-full grid-cols-[1.4fr_0.8fr_0.8fr_1fr_0.4fr] items-center gap-2 px-5 py-2.5 text-left transition hover:bg-panel2"
      >
        <span className="flex items-center gap-2">
          <VendorGlyph id={r.connector_instance} size={22} />
          <span className="text-sm">{r.connector_instance}</span>
        </span>
        <span className="text-sm tabular-nums">{r.observation_count}</span>
        <span className="text-sm tabular-nums" style={{ color: r.error_count ? 'var(--amber)' : 'var(--muted)' }}>{r.error_count}</span>
        <span className="text-xs text-muted">{relativeTime(r.received_at, now)}</span>
        <span aria-hidden className="text-right text-xs text-faint">{open ? '▼' : '▶'}</span>
      </button>
      {open && (
        <pre className="max-h-80 overflow-auto bg-panel2 px-5 py-3 font-mono text-[11px] leading-relaxed text-muted">
          {JSON.stringify(r.body, null, 2)}
        </pre>
      )}
    </div>
  )
}
