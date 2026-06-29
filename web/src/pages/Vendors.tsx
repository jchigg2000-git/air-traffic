import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, qk, type Adapter, type Mode } from '../lib/api.ts'
import PageHeader from '../components/PageHeader.tsx'
import VendorGlyph from '../components/VendorGlyph.tsx'
import DispositionChip from '../components/DispositionChip.tsx'
import { relativeTime, titleCase } from '../lib/format.ts'

const MODES: Mode[] = ['synthetic', 'proxy', 'disabled']
const SCENARIOS = ['healthy', '401', '403', '429-retry-after', '500', '503', 'timeout', 'invalid-json', 'empty']

export default function Vendors() {
  const qc = useQueryClient()
  const adapters = useQuery({ queryKey: qk.adapters, queryFn: api.adapters })
  const [selId, setSelId] = useState<string | null>(null)

  const list = adapters.data ?? []
  const sel = list.find((a) => a.id === selId) ?? list[0]
  const calls = useQuery({ queryKey: qk.calls(sel?.id ?? ''), queryFn: () => api.calls(sel!.id), enabled: !!sel })

  async function patch(id: string, p: Partial<Pick<Adapter, 'mode' | 'scenario' | 'enabled' | 'emit'>>) {
    await api.patchAdapter(id, p)
    qc.invalidateQueries({ queryKey: qk.adapters })
  }
  async function test(id: string) {
    await api.testAdapter(id)
    qc.invalidateQueries({ queryKey: qk.adapters })
  }

  return (
    <div>
      <PageHeader title="Vendors & Surfaces" subtitle="Drive each synthetic control surface — mode, fault scenario, emitter — and inspect its manifest and recorded calls." />

      <div className="grid gap-4 lg:grid-cols-[1fr_1.4fr]">
        {/* list */}
        <div className="panel max-h-[78vh] overflow-auto">
          {list.map((a) => (
            <button
              key={a.id}
              onClick={() => setSelId(a.id)}
              className={`flex w-full items-center gap-3 border-b border-line px-4 py-2.5 text-left transition hover:bg-panel2 ${sel?.id === a.id ? 'bg-panel2' : ''}`}
            >
              <VendorGlyph id={a.id} />
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm font-medium">{a.display_name}</span>
                <span className="block text-[11px] text-faint">Tier {a.tier} · {a.capabilities.length} controls</span>
              </span>
              <span className="rounded px-2 py-0.5 text-[10px]" style={{ background: 'color-mix(in srgb, var(--accent2) 16%, transparent)', color: 'var(--accent)' }}>
                {a.mode}
              </span>
            </button>
          ))}
        </div>

        {/* detail */}
        {sel && (
          <div className="flex flex-col gap-4">
            <div className="panel p-5">
              <div className="mb-4 flex items-center gap-3">
                <VendorGlyph id={sel.id} size={36} />
                <div>
                  <div className="text-lg font-semibold">{sel.display_name}</div>
                  <div className="text-xs text-muted">{titleCase(sel.family)} · API {sel.api_version} · {sel.base_path}</div>
                </div>
                <button onClick={() => test(sel.id)} className="ml-auto rounded-lg border border-line bg-panel2 px-3 py-1.5 text-sm transition hover:border-accent">
                  Test
                </button>
              </div>

              <div className="grid gap-3 sm:grid-cols-3">
                <label className="flex flex-col gap-1 text-xs text-muted">
                  Mode
                  <select value={sel.mode} onChange={(e) => patch(sel.id, { mode: e.target.value as Mode })} className="rounded-lg border border-line bg-panel2 px-2 py-1.5 text-sm text-fg">
                    {MODES.map((m) => <option key={m} value={m}>{m}</option>)}
                  </select>
                </label>
                <label className="flex flex-col gap-1 text-xs text-muted">
                  Fault scenario
                  <select value={sel.scenario} onChange={(e) => patch(sel.id, { scenario: e.target.value })} className="rounded-lg border border-line bg-panel2 px-2 py-1.5 text-sm text-fg">
                    {SCENARIOS.map((s) => <option key={s} value={s}>{s}</option>)}
                  </select>
                </label>
                <label className="flex flex-col gap-1 text-xs text-muted">
                  Emitter
                  <button
                    onClick={() => patch(sel.id, { emit: !sel.emit })}
                    className="rounded-lg border border-line px-2 py-1.5 text-sm"
                    style={{ background: sel.emit ? 'color-mix(in srgb, var(--green) 16%, transparent)' : 'var(--panel2)', color: sel.emit ? 'var(--green)' : 'var(--muted)' }}
                  >
                    {sel.emit ? 'Emitting ●' : 'Paused ○'}
                  </button>
                </label>
              </div>
              <div className="mt-3 flex items-center gap-2 text-xs">
                <span className="text-muted">Status:</span>
                <span style={{ color: sel.status.state === 'healthy' ? 'var(--green)' : sel.status.state === 'degraded' ? 'var(--amber)' : 'var(--muted)' }}>
                  {sel.status.state} — {sel.status.message}
                </span>
              </div>
            </div>

            {/* manifest */}
            <div className="panel p-5">
              <h3 className="mb-3 text-sm font-semibold">Capability Manifest</h3>
              <div className="flex flex-col gap-1.5">
                {sel.capabilities.map((c) => (
                  <div key={c.key} className="grid grid-cols-[1.3fr_auto_1.6fr] items-center gap-3 border-b border-line py-1.5 last:border-0">
                    <span className="text-sm">{c.name}</span>
                    <DispositionChip disposition={c.disposition} enforcement={c.enforcement} size="xs" />
                    <span className="truncate text-[11px] text-faint">{c.endpoint || c.mechanism}</span>
                  </div>
                ))}
              </div>
            </div>

            {/* recorded calls */}
            <div className="panel p-5">
              <h3 className="mb-3 text-sm font-semibold">Recorded Calls <span className="text-xs font-normal text-faint">(redacted)</span></h3>
              <div className="max-h-56 overflow-auto font-mono text-xs">
                {(calls.data ?? []).map((c) => (
                  <div key={c.id} className="flex items-center gap-3 border-b border-line py-1 last:border-0">
                    <span className="w-10 shrink-0" style={{ color: c.status_code < 400 ? 'var(--green)' : 'var(--amber)' }}>{c.status_code}</span>
                    <span className="w-12 shrink-0 text-muted">{c.method}</span>
                    <span className="flex-1 truncate">{c.path}</span>
                    <span className="shrink-0 text-faint">{c.duration_ms}ms · {relativeTime(c.recorded_at)}</span>
                  </div>
                ))}
                {!calls.data?.length && <div className="text-faint">No calls yet — hit a /synthetic/{sel.id}/… endpoint.</div>}
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
