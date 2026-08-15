import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, qk, type GatewayRequest } from '../lib/api.ts'
import PageHeader from '../components/PageHeader.tsx'
import ApiStateBanner from '../components/ApiStateBanner.tsx'
import { relativeTime, fmtNum } from '../lib/format.ts'
import { useClock } from '../lib/useClock.ts'

const COLS = 'grid-cols-[0.7fr_0.9fr_0.6fr_1.2fr_0.7fr_1.3fr_0.8fr_0.6fr_0.8fr]'

// Action colours track what the gateway actually did to the request, not how
// worried to be: mask and block both mean the filter fired.
const ACTION_COLOR: Record<string, string> = {
  pass: 'var(--faint)',
  detect: 'var(--accent)',
  mask: 'var(--amber)',
  block: 'var(--red)',
}

function ActionChip({ action }: { action: string }) {
  const color = ACTION_COLOR[action] ?? 'var(--muted)'
  return (
    <span
      className="inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide"
      style={{ borderColor: 'var(--line)', color }}
    >
      <span className="inline-block h-1.5 w-1.5 rounded-full" style={{ background: color }} />
      {action}
    </span>
  )
}

// "SSN×2, EMAIL×1" — counts and types only, mirroring what the gateway logs.
// A value never leaves the data plane, so there is nothing here to reveal.
function redactionSummary(r: GatewayRequest): string {
  const counts = new Map<string, number>()
  for (const red of r.redactions ?? []) counts.set(red.type, (counts.get(red.type) ?? 0) + 1)
  return [...counts.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([t, n]) => `${t}×${n}`)
    .join(', ')
}

// Which app sent the request, and — when the app carries its own baseline —
// which posture it was served under. Requests authenticated by the legacy
// GATEWAY_CLIENT_KEYS env var report "env"; anything older than the keystore
// carries no attribution at all.
function appLabel(r: GatewayRequest): string {
  return r.app_id || '—'
}

function attributionTitle(r: GatewayRequest): string {
  const parts = [`key ${r.key_id || 'unknown'}`]
  if (r.subject) parts.push(`subject ${r.subject}`)
  if (r.baseline) parts.push(`baseline ${r.baseline}`)
  return parts.join(' · ')
}

function statusColor(status: number | undefined): string {
  if (!status) return 'var(--faint)'
  if (status >= 500) return 'var(--red)'
  if (status >= 400) return 'var(--amber)'
  return 'var(--green)'
}

export default function GatewayTraffic() {
  const now = useClock(1000)
  const [live, setLive] = useState(true)
  const [filter, setFilter] = useState('')

  const traffic = useQuery({
    queryKey: qk.gatewayRequests,
    queryFn: () => api.gatewayRequests(200),
    refetchInterval: live ? 3000 : false,
  })

  const rows = useMemo(() => {
    const all = traffic.data ?? []
    if (!filter.trim()) return all
    const q = filter.toLowerCase()
    return all.filter((r) =>
      [r.route, r.model ?? '', r.action, r.request_id, r.app_id ?? '', r.subject ?? '', redactionSummary(r)]
        .join(' ')
        .toLowerCase()
        .includes(q),
    )
  }, [traffic.data, filter])

  const totals = useMemo(() => {
    let redactions = 0
    let blocked = 0
    let tokensIn = 0
    let tokensOut = 0
    const apps = new Set<string>()
    const added: number[] = []
    for (const r of rows) {
      redactions += r.redactions?.length ?? 0
      if (r.action === 'block') blocked++
      tokensIn += r.tokens_in ?? 0
      tokensOut += r.tokens_out ?? 0
      if (r.app_id) apps.add(r.app_id)
      added.push(r.added_latency_ms)
    }
    added.sort((a, b) => a - b)
    const p95 = added.length ? added[Math.min(added.length - 1, Math.floor(added.length * 0.95))] : 0
    return { redactions, blocked, tokensIn, tokensOut, p95, apps: apps.size }
  }, [rows])

  return (
    <div>
      <PageHeader
        title="Gateway Traffic"
        subtitle="Every request that actually went through the proxy. Metadata only — types, field paths, offsets, counts; never a redacted value."
        actions={
          <>
            <input
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder="Filter app, route, model, action…"
              className="rounded-lg border border-line bg-panel2 px-3 py-1.5 text-sm text-fg placeholder:text-faint"
            />
            <button
              onClick={() => setLive((v) => !v)}
              className="rounded-lg border border-line px-3 py-1.5 text-sm text-muted transition hover:text-fg"
            >
              {live ? '⏸ Pause' : '▶ Live'}
            </button>
            <button
              onClick={() => traffic.refetch()}
              className="rounded-lg px-3 py-1.5 text-sm font-medium text-bg"
              style={{ background: 'var(--accent)' }}
            >
              Refresh
            </button>
          </>
        }
      />

      <ApiStateBanner error={traffic.error} className="mb-4" />
      <div className="mb-4 grid grid-cols-2 gap-3 md:grid-cols-6">

        <Tile label="Requests" value={fmtNum(rows.length)} />
        <Tile label="Apps" value={fmtNum(totals.apps)} />
        <Tile label="Redactions" value={fmtNum(totals.redactions)} />
        <Tile label="Blocked" value={fmtNum(totals.blocked)} tone={totals.blocked ? 'var(--red)' : undefined} />
        <Tile label="Tokens in / out" value={`${fmtNum(totals.tokensIn)} / ${fmtNum(totals.tokensOut)}`} />
        <Tile label="Added latency p95" value={`${fmtNum(totals.p95)} ms`} />
      </div>

      <div className="panel overflow-hidden">
        <div
          className={`grid ${COLS} gap-2 border-b border-line px-5 py-2 text-[10px] font-semibold uppercase tracking-wider text-faint`}
        >
          <span>When</span>
          <span>App</span>
          <span>Route</span>
          <span>Model</span>
          <span>Action</span>
          <span>Redactions</span>
          <span>Tokens</span>
          <span>Status</span>
          <span>Latency</span>
        </div>
        <div className="max-h-[64vh] overflow-auto">
          {rows.map((r) => (
            <div
              key={r.request_id}
              className={`grid ${COLS} items-center gap-2 border-b border-line px-5 py-2 text-sm last:border-0`}
            >
              <span className="text-xs text-faint" title={r.request_id}>
                {relativeTime(r.at, now)}
              </span>
              <span className="truncate font-mono text-[11px] text-fg" title={attributionTitle(r)}>
                {appLabel(r)}
                {r.baseline && <span className="ml-1 text-[10px] text-faint">{r.baseline}</span>}
              </span>
              <span className="truncate text-xs text-muted">
                {r.route}
                {r.stream && <span className="ml-1 text-[10px] text-faint">≋</span>}
              </span>
              <span className="truncate font-mono text-[11px] text-muted" title={r.model}>
                {r.model || '—'}
              </span>
              <span>
                <ActionChip action={r.action} />
              </span>
              <span className="truncate font-mono text-[11px]" title={redactionSummary(r)}>
                {redactionSummary(r) || <span className="text-faint">—</span>}
              </span>
              <span className="font-mono text-[11px] text-muted">
                {r.tokens_in || r.tokens_out ? `${fmtNum(r.tokens_in ?? 0)}/${fmtNum(r.tokens_out ?? 0)}` : <span className="text-faint">—</span>}
              </span>
              <span className="font-mono text-[11px]" style={{ color: statusColor(r.upstream_status) }}>
                {r.upstream_status || '—'}
              </span>
              <span className="font-mono text-[11px] text-muted" title={`${r.added_latency_ms} ms added by the gateway`}>
                {fmtNum(r.latency_ms)}
                <span className="text-faint"> +{fmtNum(r.added_latency_ms)}</span>
              </span>
            </div>
          ))}
          {traffic.isLoading && <div className="h-40 animate-pulse" />}
          {traffic.data && !rows.length && (
            <div className="px-5 py-6 text-center text-sm text-faint">
              {filter
                ? 'No requests match that filter.'
                : 'No proxied requests yet. Point an app at the gateway and its traffic lands here.'}
            </div>
          )}
        </div>
      </div>

      <p className="mt-3 text-xs text-faint">
        Token counts are what the vendor reported for each request, not an estimate — a response without a usage block
        stays blank rather than being guessed at. Cost is deliberately absent: pricing belongs to whoever owns the
        vendor contract, not to the proxy.
      </p>
    </div>
  )
}

function Tile({ label, value, tone }: { label: string; value: string; tone?: string }) {
  return (
    <div className="panel px-4 py-3">
      <div className="text-[10px] font-semibold uppercase tracking-wider text-faint">{label}</div>
      <div className="mt-1 font-mono text-lg" style={tone ? { color: tone } : undefined}>
        {value}
      </div>
    </div>
  )
}
