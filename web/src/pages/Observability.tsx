import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, qk, type ObservationRecord } from '../lib/api.ts'
import PageHeader from '../components/PageHeader.tsx'
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

      <div className="panel overflow-hidden">
        <div className="grid grid-cols-[1.4fr_0.8fr_0.8fr_1fr_0.4fr] gap-2 border-b border-line px-5 py-2 text-[10px] font-semibold uppercase tracking-wider text-faint">
          <span>Vendor</span>
          <span>Obs</span>
          <span>Errors</span>
          <span>Collected</span>
          <span />
        </div>
        {obs.data?.map((r) => (
          <Row key={r.id} r={r} now={now} open={open === r.id} onToggle={() => setOpen(open === r.id ? null : r.id)} />
        ))}
        {obs.isLoading && <div className="h-40 animate-pulse" />}
        {obs.data && !obs.data.length && <div className="px-5 py-6 text-center text-sm text-faint">No batches yet.</div>}
      </div>
    </div>
  )
}

function Row({ r, now, open, onToggle }: { r: ObservationRecord; now: number; open: boolean; onToggle: () => void }) {
  return (
    <div className="border-b border-line last:border-0">
      <button onClick={onToggle} className="grid w-full grid-cols-[1.4fr_0.8fr_0.8fr_1fr_0.4fr] items-center gap-2 px-5 py-2.5 text-left transition hover:bg-panel2">
        <span className="flex items-center gap-2">
          <VendorGlyph id={r.connector_instance} size={22} />
          <span className="text-sm">{r.connector_instance}</span>
        </span>
        <span className="text-sm tabular-nums">{r.observation_count}</span>
        <span className="text-sm tabular-nums" style={{ color: r.error_count ? 'var(--amber)' : 'var(--muted)' }}>{r.error_count}</span>
        <span className="text-xs text-muted">{relativeTime(r.received_at, now)}</span>
        <span className="text-right text-xs text-faint">{open ? '▼' : '▶'}</span>
      </button>
      {open && (
        <pre className="max-h-80 overflow-auto bg-panel2 px-5 py-3 font-mono text-[11px] leading-relaxed text-muted">
          {JSON.stringify(r.body, null, 2)}
        </pre>
      )}
    </div>
  )
}
