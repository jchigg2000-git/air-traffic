import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, qk, type Adapter, type Capability } from '../lib/api.ts'
import PageHeader from '../components/PageHeader.tsx'
import ApiStateBanner from '../components/ApiStateBanner.tsx'
import VendorGlyph from '../components/VendorGlyph.tsx'
import DispositionChip from '../components/DispositionChip.tsx'
import { titleCase } from '../lib/format.ts'

interface ControlGroup {
  key: string
  name: string
  rows: { adapter: Adapter; cap: Capability }[]
}

const PLANE_TITLES: Record<string, string> = {
  data_policy: 'Data Policy Plane',
  developer_workflow: 'Developer Workflow Plane',
  budget: 'Budget Plane',
  observability: 'Observability Plane',
}

export default function PolicyEditor() {
  const adapters = useQuery({ queryKey: qk.adapters, queryFn: api.adapters })
  const [openPlanes, setOpenPlanes] = useState<Record<string, boolean>>({ data_policy: true, developer_workflow: true })
  const [openControl, setOpenControl] = useState<string | null>(null)

  const grouped = useMemo(() => groupControls(adapters.data ?? []), [adapters.data])

  return (
    <div>
      <PageHeader title="Policy Editor" subtitle="Expand each control into per-vendor mechanisms. Every chip states the real disposition — never overstated." />
      <ApiStateBanner error={adapters.error} hasData={!!adapters.data} className="mb-4" />

      {adapters.isLoading && (
        <div role="status" aria-label="Loading per-vendor controls" className="h-64 animate-pulse panel" />
      )}

      <div className="flex flex-col gap-3">
        {Object.entries(grouped).map(([plane, controls]) => {
          const open = openPlanes[plane] ?? false
          return (
            <div key={plane} className="panel overflow-hidden">
              <button
                onClick={() => setOpenPlanes((p) => ({ ...p, [plane]: !open }))}
                aria-expanded={open}
                aria-label={`${open ? 'Collapse' : 'Expand'} the ${PLANE_TITLES[plane] ?? titleCase(plane)} control plane, ${controls.length} controls`}
                className="flex w-full items-center justify-between px-5 py-3 text-left transition hover:bg-panel2"
              >
                <span className="flex items-center gap-2 text-sm font-semibold">
                  <span aria-hidden className="text-accent">{open ? '▼' : '▶'}</span>
                  {PLANE_TITLES[plane] ?? titleCase(plane)}
                </span>
                <span className="text-xs text-faint">{controls.length} controls</span>
              </button>

              {open && (
                <div className="border-t border-line">
                  {controls.map((g) => {
                    const cid = plane + '/' + g.key
                    const expanded = openControl === cid
                    const dispSet = uniqueDisp(g.rows)
                    return (
                      <div key={cid} className="border-b border-line last:border-0">
                        <button
                          onClick={() => setOpenControl(expanded ? null : cid)}
                          aria-expanded={expanded}
                          aria-label={`${expanded ? 'Collapse' : 'Expand'} ${g.name} across ${g.rows.length} vendors`}
                          className="flex w-full items-center justify-between gap-3 px-5 py-2.5 text-left transition hover:bg-panel2"
                        >
                          <span className="flex items-center gap-2.5">
                            <span aria-hidden className="text-xs text-faint">{expanded ? '–' : '+'}</span>
                            <span className="text-sm font-medium">{g.name}</span>
                            <span className="text-[11px] text-faint">{g.rows.length} vendors</span>
                          </span>
                          <span className="flex flex-wrap items-center gap-1">
                            {dispSet.map((d) => (
                              <DispositionChip key={d} disposition={d} size="xs" />
                            ))}
                          </span>
                        </button>
                        {expanded && (
                          <div className="bg-panel2 px-5 py-3">
                            <div className="grid gap-1.5">
                              {g.rows.map(({ adapter, cap }) => (
                                <div key={adapter.id} className="grid grid-cols-[1.2fr_1fr_2fr] items-center gap-3 border-b border-line py-1.5 last:border-0">
                                  <span className="flex items-center gap-2">
                                    <VendorGlyph id={adapter.id} size={22} />
                                    <span className="truncate text-sm">{adapter.display_name}</span>
                                  </span>
                                  <DispositionChip disposition={cap.disposition} enforcement={cap.enforcement} size="xs" />
                                  <span className="truncate text-xs text-muted" title={cap.note || cap.mechanism}>
                                    {cap.mechanism}
                                    {cap.note ? ` — ${cap.note}` : ''}
                                  </span>
                                </div>
                              ))}
                            </div>
                          </div>
                        )}
                      </div>
                    )
                  })}
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}

function groupControls(adapters: Adapter[]): Record<string, ControlGroup[]> {
  const planes: Record<string, Map<string, ControlGroup>> = {}
  for (const a of adapters) {
    for (const c of a.capabilities) {
      const m = (planes[c.plane] ??= new Map())
      const g = m.get(c.key) ?? { key: c.key, name: c.name, rows: [] }
      g.rows.push({ adapter: a, cap: c })
      m.set(c.key, g)
    }
  }
  const out: Record<string, ControlGroup[]> = {}
  for (const [plane, m] of Object.entries(planes)) {
    out[plane] = [...m.values()].sort((a, b) => b.rows.length - a.rows.length)
  }
  return out
}

function uniqueDisp(rows: { cap: Capability }[]): string[] {
  return [...new Set(rows.map((r) => r.cap.disposition))]
}
