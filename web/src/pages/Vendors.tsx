import { useEffect, useState, type MouseEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, qk, type Adapter, type Mode } from '../lib/api.ts'
import { useClock } from '../lib/useClock.ts'
import PageHeader from '../components/PageHeader.tsx'
import ApiStateBanner from '../components/ApiStateBanner.tsx'
import VendorGlyph from '../components/VendorGlyph.tsx'
import DispositionChip from '../components/DispositionChip.tsx'
import Modal from '../components/Modal.tsx'
import { relativeTime, titleCase } from '../lib/format.ts'
import { validateEndpoint, expectedFor, knownPrefixes } from '../lib/endpoints.ts'
import { schemaFor, validateField, type ConfigField } from '../lib/authSchemas.ts'

const MODES: Mode[] = ['synthetic', 'proxy', 'disabled']
const SCENARIOS = ['healthy', '401', '403', '429-retry-after', '500', '503', 'timeout', 'invalid-json', 'empty']

function Switch({ on, onClick, title }: { on: boolean; onClick: (e: MouseEvent) => void; title?: string }) {
  return (
    <button
      onClick={onClick}
      role="switch"
      aria-checked={on}
      title={title}
      className="relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition"
      style={{ background: on ? 'var(--accent2)' : 'var(--line)' }}
    >
      <span className="inline-block h-4 w-4 rounded-full bg-white shadow transition-transform" style={{ transform: on ? 'translateX(18px)' : 'translateX(2px)' }} />
    </button>
  )
}

export default function Vendors() {
  const qc = useQueryClient()
  const now = useClock(1000)
  const adapters = useQuery({ queryKey: qk.adapters, queryFn: api.adapters })
  const [selId, setSelId] = useState<string | null>(null)
  const [epOpen, setEpOpen] = useState(false)
  const [epValues, setEpValues] = useState<Record<string, string>>({})

  const list = adapters.data ?? []
  const sel = list.find((a) => a.id === selId) ?? list[0]
  const calls = useQuery({ queryKey: qk.calls(sel?.id ?? ''), queryFn: () => api.calls(sel!.id), enabled: !!sel })

  async function patch(id: string, p: Partial<Pick<Adapter, 'mode' | 'scenario' | 'enabled' | 'emit' | 'upstream_url' | 'endpoint_config'>>) {
    await api.patchAdapter(id, p)
    qc.invalidateQueries({ queryKey: qk.adapters })
  }
  async function test(id: string) {
    await api.testAdapter(id)
    qc.invalidateQueries({ queryKey: qk.adapters })
  }

  function openEndpoint() {
    if (!sel) return
    const schema = schemaFor(sel.id)
    const init: Record<string, string> = {}
    if (schema) {
      for (const f of schema.fields) {
        init[f.key] = f.key === 'upstream_url' ? sel.upstream_url || f.default || '' : sel.endpoint_config?.[f.key] ?? f.default ?? ''
      }
    } else {
      init.upstream_url = sel.upstream_url || (expectedFor(sel.id)?.example ?? '')
    }
    setEpValues(init)
    setEpOpen(true)
  }

  async function saveEndpoint() {
    if (!sel) return
    const { upstream_url = '', ...rest } = epValues
    const config: Record<string, string> = {}
    for (const [k, v] of Object.entries(rest)) if (v.trim()) config[k] = v.trim()
    await patch(sel.id, { upstream_url: upstream_url.trim(), endpoint_config: config, mode: 'proxy' })
    setEpOpen(false)
  }

  const onCount = list.filter((a) => a.enabled).length

  return (
    <div>
      <PageHeader title="Vendors & Surfaces" subtitle="Drive each synthetic control surface — toggle, mode, fault scenario, emitter, endpoint — and inspect its manifest and recorded calls." />
      <ApiStateBanner error={adapters.error} hasData={!!adapters.data} className="mb-4" />

      <div className="grid items-start gap-4 lg:grid-cols-[1fr_1.4fr]">
        {/* list — fills the viewport height */}
        <div className="panel flex flex-col overflow-hidden lg:sticky lg:top-5 lg:h-[calc(100vh-7rem)]">
          <div className="flex items-center justify-between border-b border-line px-4 py-2.5 text-[11px] uppercase tracking-wider text-faint">
            <span>{adapters.isLoading ? '—' : `${list.length} vendors`}</span>
            <span>{adapters.isLoading ? '—' : `${onCount} on · ${list.length - onCount} off`}</span>
          </div>
          <div className="flex-1 overflow-y-auto">
            {list.map((a) => (
              <div
                key={a.id}
                className={`flex items-center gap-2 border-b border-line px-3 ${sel?.id === a.id ? 'bg-panel2' : ''}`}
              >
                <button
                  onClick={() => setSelId(a.id)}
                  className="flex min-w-0 flex-1 items-center gap-3 py-2.5 text-left transition hover:opacity-100"
                  style={{ opacity: a.enabled ? 1 : 0.45 }}
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
                <Switch
                  on={a.enabled}
                  title={a.enabled ? 'Enabled — click to disable' : 'Disabled — click to enable'}
                  onClick={(e) => {
                    e.stopPropagation()
                    patch(a.id, { enabled: !a.enabled })
                  }}
                />
              </div>
            ))}
          </div>
        </div>

        {/* detail */}
        {sel && (
          <div className="flex flex-col gap-4">
            <div className="panel p-5">
              <div className="mb-4 flex items-center gap-3">
                <VendorGlyph id={sel.id} size={36} />
                <div className="min-w-0">
                  <div className="text-lg font-semibold">{sel.display_name}</div>
                  <div className="truncate text-xs text-muted">{titleCase(sel.family)} · API {sel.api_version} · {sel.base_path}</div>
                </div>
                <div className="ml-auto flex items-center gap-2">
                  <button
                    onClick={openEndpoint}
                    aria-label="Configure endpoint"
                    title="Configure endpoint"
                    className="grid h-9 w-9 place-items-center rounded-lg border border-line bg-panel2 text-muted transition hover:border-accent hover:text-fg"
                  >
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
                      <circle cx="12" cy="12" r="3" />
                      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
                    </svg>
                  </button>
                  <button onClick={() => test(sel.id)} className="rounded-lg border border-line bg-panel2 px-3 py-1.5 text-sm transition hover:border-accent">
                    Test
                  </button>
                </div>
              </div>

              <div className="grid gap-3 sm:grid-cols-4">
                <label className="flex flex-col gap-1 text-xs text-muted">
                  Active
                  <button
                    onClick={() => patch(sel.id, { enabled: !sel.enabled })}
                    className="flex items-center justify-between rounded-lg border border-line px-2 py-1.5 text-sm"
                    style={{ background: sel.enabled ? 'color-mix(in srgb, var(--green) 16%, transparent)' : 'var(--panel2)', color: sel.enabled ? 'var(--green)' : 'var(--muted)' }}
                  >
                    {sel.enabled ? 'On' : 'Off'}
                    <Switch on={sel.enabled} onClick={(e) => { e.stopPropagation(); patch(sel.id, { enabled: !sel.enabled }) }} />
                  </button>
                </label>
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
              <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs">
                <span><span className="text-muted">Status:</span> <span style={{ color: sel.status.state === 'healthy' ? 'var(--green)' : sel.status.state === 'degraded' ? 'var(--amber)' : 'var(--muted)' }}>{sel.status.state} — {sel.status.message}</span></span>
                <span className="text-muted">Endpoint: <span className="font-mono text-fg">{sel.upstream_url || '— (synthetic)'}</span></span>
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
                    <span className="shrink-0 text-faint">{c.duration_ms}ms · {relativeTime(c.recorded_at, now)}</span>
                  </div>
                ))}
                {!calls.data?.length && <div className="text-faint">No calls yet — hit a /synthetic/{sel.id}/… endpoint.</div>}
              </div>
            </div>
          </div>
        )}
      </div>

      {sel && (
        <EndpointModal
          adapter={sel}
          open={epOpen}
          values={epValues}
          onField={(k, v) => setEpValues((s) => ({ ...s, [k]: v }))}
          onClose={() => setEpOpen(false)}
          onSave={saveEndpoint}
        />
      )}
    </div>
  )
}

const URL_FALLBACK: ConfigField = { key: 'upstream_url', label: 'Base URL', kind: 'url', required: true }

function EndpointModal({
  adapter,
  open,
  values,
  onField,
  onClose,
  onSave,
}: {
  adapter: Adapter
  open: boolean
  values: Record<string, string>
  onField: (key: string, value: string) => void
  onClose: () => void
  onSave: () => void
}) {
  const [touched, setTouched] = useState(false)
  useEffect(() => {
    if (open) setTouched(false)
  }, [open])

  const schema = schemaFor(adapter.id)
  const fields = schema?.fields ?? [URL_FALLBACK]
  const exp = expectedFor(adapter.id)
  const prefixes = knownPrefixes(adapter)

  // per-field errors/warnings
  const errors: Record<string, string> = {}
  let urlWarning = ''
  for (const f of fields) {
    const val = values[f.key] ?? ''
    if (f.kind === 'url') {
      const r = validateEndpoint(adapter, val)
      if (!r.ok) errors[f.key] = r.errors[0]
      else if (r.warnings.length) urlWarning = r.warnings[0]
    } else {
      const e = validateField(f, val, values)
      if (e) errors[f.key] = e
    }
  }
  const ok = Object.keys(errors).length === 0

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={`Configure ${adapter.display_name}`}
      footer={
        <>
          <button onClick={onClose} className="rounded-lg border border-line bg-panel2 px-3 py-1.5 text-sm text-muted transition hover:text-fg">
            Cancel
          </button>
          <button
            onClick={() => ok && onSave()}
            disabled={!ok}
            className="rounded-lg px-4 py-1.5 text-sm font-medium text-bg disabled:cursor-not-allowed disabled:opacity-40"
            style={{ background: 'var(--accent)' }}
          >
            Save & switch to proxy
          </button>
        </>
      }
    >
      <p className="mb-1 text-xs text-muted">
        {schema ? <><span className="text-fg">{schema.authType}</span>. </> : null}
        Saving switches this adapter to <span className="text-fg">proxy</span> mode. Validated against {adapter.vendor}'s real endpoint contract.
      </p>
      {schema?.note && <p className="mb-3 text-[11px] text-amber">{schema.note}</p>}
      {!schema && (
        <p className="mb-3 rounded-lg border border-line bg-panel2 p-2 text-[11px] text-muted">
          Full per-vendor config for {adapter.vendor} isn't modeled yet — tracked in <span className="font-mono">docs/plans/TODO-vendor-auth.md</span>. You can set the base URL for now.
        </p>
      )}

      <div className="flex max-h-[52vh] flex-col gap-3 overflow-y-auto pr-1">
        {fields.map((f, i) => {
          const val = values[f.key] ?? ''
          const err = touched ? errors[f.key] : undefined
          return (
            <label key={f.key} className="flex flex-col gap-1 text-xs text-muted">
              <span>
                {f.label}
                {(f.required || (f.requiredWhen && values[f.requiredWhen.key] === f.requiredWhen.value)) && <span className="text-red"> *</span>}
              </span>
              {f.kind === 'select' ? (
                <select
                  value={val}
                  onChange={(e) => { onField(f.key, e.target.value); setTouched(true) }}
                  className="rounded-lg border border-line bg-panel2 px-2 py-1.5 text-sm text-fg"
                >
                  {f.options?.map((o) => <option key={o} value={o}>{o}</option>)}
                </select>
              ) : (
                <input
                  autoFocus={i === 0}
                  value={val}
                  onChange={(e) => { onField(f.key, e.target.value); setTouched(true) }}
                  placeholder={f.placeholder ?? (f.kind === 'url' ? exp?.example : '')}
                  className="rounded-lg border bg-panel2 px-3 py-2 font-mono text-sm text-fg"
                  style={{ borderColor: err ? 'var(--red)' : 'var(--line)' }}
                  spellCheck={false}
                />
              )}
              {err ? (
                <span className="flex items-center gap-1 text-[11px]" style={{ color: 'var(--red)' }}><span>✕</span> {err}</span>
              ) : f.help ? (
                <span className="text-[10px] text-faint">{f.help}</span>
              ) : null}
            </label>
          )
        })}
      </div>

      {touched && ok && urlWarning && (
        <div className="mt-2 flex items-center gap-1.5 text-xs" style={{ color: 'var(--amber)' }}><span>⚠</span> {urlWarning}</div>
      )}
      {touched && ok && (
        <div className="mt-2 flex items-center gap-1.5 text-xs" style={{ color: 'var(--green)' }}><span>✓</span> Valid — matches the {adapter.vendor} contract.</div>
      )}

      <div className="mt-4 rounded-lg border border-line bg-panel2 p-3 text-[11px] text-muted">
        <div className="mb-1 font-semibold uppercase tracking-wider text-faint">Contract</div>
        <div>Host · <span className="font-mono text-fg">…{exp?.hostSuffix ?? 'n/a'}</span> · https only · secrets by reference</div>
        {prefixes.length > 0 && (
          <div className="mt-1">Known admin paths · <span className="font-mono text-fg">{prefixes.slice(0, 4).join('  ')}</span></div>
        )}
      </div>
    </Modal>
  )
}
