import { Fragment, useState } from 'react'
import type { ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, qk } from '../lib/api.ts'
import type { GatewayRedaction, HarnessRun, HarnessRunConfig, PatternProposal, SampleResult } from '../lib/api.ts'
import PageHeader from '../components/PageHeader.tsx'
import Sparkline from '../components/Sparkline.tsx'
import RedactionDiff from '../components/RedactionDiff.tsx'

const pct = (v: number) => `${(v * 100).toFixed(1)}%`

// Redaction offsets are byte offsets (Go); JS strings index UTF-16 units.
function byteToCharIndex(content: string): number[] {
  const enc = new TextEncoder()
  const map: number[] = []
  let byteOff = 0
  let charOff = 0
  for (const ch of content) {
    const bytes = enc.encode(ch).length
    for (let b = 0; b < bytes; b++) map[byteOff + b] = charOff
    byteOff += bytes
    charOff += ch.length
  }
  map[byteOff] = charOff
  return map
}

// Neutral span highlighting for ad-hoc samples: there is no ground truth, so
// detections are just "redacted" — never "false positive" (that judgment
// belongs to scored runs and RedactionDiff).
function SampleMarkedText({ content, redactions }: { content: string; redactions: GatewayRedaction[] }) {
  const b2c = byteToCharIndex(content)
  const reds = redactions
    .filter((r) => r.path.includes('content'))
    .map((r) => ({ ...r, start: b2c[r.start] ?? 0, end: b2c[r.end] ?? content.length }))
    .sort((a, b) => a.start - b.start || b.end - a.end)
  const flat: typeof reds = []
  for (const r of reds) {
    const last = flat[flat.length - 1]
    if (!last || r.start >= last.end) flat.push(r)
  }
  const parts: ReactNode[] = []
  let pos = 0
  flat.forEach((r, i) => {
    if (r.start > pos) parts.push(<span key={`p${i}`}>{content.slice(pos, r.start)}</span>)
    parts.push(
      <span
        key={`m${i}`}
        title={`${r.type} · ${r.detector} · ${(r.confidence * 100).toFixed(0)}%`}
        style={{ background: 'color-mix(in srgb, var(--accent) 18%, transparent)', borderBottom: '2px solid var(--accent)', borderRadius: 2 }}
      >
        {content.slice(r.start, r.end)}
      </span>,
    )
    pos = r.end
  })
  if (pos < content.length) parts.push(<span key="tail">{content.slice(pos)}</span>)
  return <>{parts}</>
}

function Chip({ ok, label }: { ok: boolean; label: string }) {
  return (
    <span className="inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px]" style={{ borderColor: 'var(--line)' }}>
      <span className="inline-block h-1.5 w-1.5 rounded-full" style={{ background: ok ? 'var(--green, #22c55e)' : 'var(--red, #ef4444)' }} />
      {label}
    </span>
  )
}

function ScoreCard({ label, value, tone }: { label: string; value: string; tone?: 'good' | 'bad' | 'plain' }) {
  const color = tone === 'good' ? 'var(--green, #22c55e)' : tone === 'bad' ? 'var(--red, #ef4444)' : 'var(--fg)'
  return (
    <div className="panel-2 px-3 py-2">
      <div className="text-xl font-semibold tabular-nums" style={{ color }}>{value}</div>
      <div className="text-[11px] text-muted">{label}</div>
    </div>
  )
}

export default function GatewayHarness() {
  const qc = useQueryClient()
  const gateway = useQuery({ queryKey: qk.gatewayStatus, queryFn: api.gatewayStatus })
  const runs = useQuery({
    queryKey: qk.harnessRuns,
    queryFn: api.harnessRuns,
    refetchInterval: (q) => (q.state.data?.some((r: HarnessRun) => r.status === 'running') ? 2000 : 5000),
  })
  const activeRun = runs.data?.[0]
  const results = useQuery({
    queryKey: qk.harnessResults(activeRun?.id ?? 'none'),
    queryFn: () => api.harnessResults(activeRun!.id, 300),
    enabled: !!activeRun && activeRun.status === 'done',
  })
  const ratchet = useQuery({ queryKey: qk.harnessRatchet, queryFn: api.harnessRatchet })
  const proposals = useQuery({ queryKey: qk.harnessProposals, queryFn: api.harnessProposals })
  const [corpusOpen, setCorpusOpen] = useState(false)
  const corpus = useQuery({ queryKey: qk.harnessCorpus, queryFn: api.harnessCorpus, enabled: corpusOpen })

  const [cfg, setCfg] = useState<HarnessRunConfig>({
    count: 200, concurrency: 4, include_traps: true, include_presidio_only: true,
    include_straddle: true, replay_percent: 20,
  })
  const [seedText, setSeedText] = useState('')
  const [starting, setStarting] = useState(false)
  const [startError, setStartError] = useState('')
  const [openRow, setOpenRow] = useState<string | null>(null)
  const [onlyMisses, setOnlyMisses] = useState(false)

  const [sampleText, setSampleText] = useState('')
  const [sample, setSample] = useState<SampleResult | null>(null)
  const [sampleSent, setSampleSent] = useState('') // the content the current result belongs to
  const [sampling, setSampling] = useState(false)
  const [sampleError, setSampleError] = useState('')

  async function sendSample() {
    if (sampling || sampleText.trim() === '') return
    setSampling(true)
    setSampleError('')
    try {
      const s = await api.runSample(sampleText)
      setSample(s)
      setSampleSent(sampleText)
    } catch (e) {
      setSampleError(e instanceof Error ? e.message : String(e))
    } finally {
      setSampling(false)
    }
  }

  const freshGateway = gateway.data?.gateways.find((g) => g.fresh)
  const running = activeRun?.status === 'running'

  async function startRun() {
    setStarting(true)
    setStartError('')
    try {
      const seed = seedText.trim() === '' ? undefined : Number(seedText)
      await api.startHarnessRun({ ...cfg, ...(seed !== undefined && !Number.isNaN(seed) ? { seed } : {}) })
      qc.invalidateQueries({ queryKey: qk.harnessRuns })
    } catch (e) {
      setStartError(e instanceof Error ? e.message : String(e))
    } finally {
      setStarting(false)
    }
  }

  async function decide(p: PatternProposal, verb: 'approve' | 'reject') {
    await (verb === 'approve' ? api.approveProposal(p.id) : api.rejectProposal(p.id))
    qc.invalidateQueries({ queryKey: qk.harnessProposals })
    qc.invalidateQueries({ queryKey: qk.gatewayStatus })
    qc.invalidateQueries({ queryKey: qk.audit })
  }

  const score = activeRun?.score
  const shownResults = (results.data ?? []).filter((r) => !onlyMisses || (r.missed_types?.length ?? 0) > 0)
  const ratchetValues = (ratchet.data ?? []).map((p) => p.recall_behavioral * 100)

  return (
    <div>
      <PageHeader
        title="Gateway Harness"
        subtitle="Prove redaction behaviorally: synthetic PII through the gateway, scored against ground truth, feeding the recall-ratchet flywheel."
        actions={
          <button
            onClick={startRun}
            disabled={starting || running || !freshGateway}
            className="rounded-lg px-4 py-2 text-sm font-medium text-bg disabled:opacity-50"
            style={{ background: 'var(--accent)' }}
          >
            {running ? 'Running…' : starting ? 'Starting…' : 'Run Traffic'}
          </button>
        }
      />

      {/* status strip */}
      <div className="mb-4 flex flex-wrap items-center gap-2">
        <Chip ok={!!freshGateway} label={freshGateway ? `gateway ${freshGateway.gateway_id} · ${freshGateway.action}` : 'no fresh gateway heartbeat'} />
        {freshGateway && <Chip ok label={`detectors: ${(gateway.data?.gateways[0]?.vendors ? activeRun?.detector_chain || 'regex' : 'regex')}`} />}
        <Chip ok label={`pattern pack v${gateway.data?.pattern_pack_version ?? 0}`} />
        <span className="text-[11px] text-muted">
          All values on this screen are synthetic by construction — displaying them is safe. Retune in v0 = pattern packs, not model training.
        </span>
      </div>
      {!freshGateway && (
        <div className="mb-4 panel p-3 text-xs text-muted">
          Start the data plane first: <code className="font-mono">go run ./cmd/air-traffic-gateway</code> (see docs/plans/phase-3-inference-gateway.md for env).
        </div>
      )}
      {startError && <div className="mb-4 panel p-3 text-xs" style={{ color: 'var(--red, #ef4444)' }}>{startError}</div>}

      {/* try a prompt */}
      <div className="mb-5 panel p-4">
        <div className="mb-2 flex items-center justify-between gap-3">
          <h3 className="text-sm font-semibold">Try a prompt</h3>
          <span className="text-[11px] text-muted">
            one request through the gateway — never scored, never fed to the flywheel. Test values only: samples land in the local capture ring like any traffic.
          </span>
        </div>
        <textarea
          value={sampleText}
          onChange={(e) => setSampleText(e.target.value)}
          onKeyDown={(e) => { if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') sendSample() }}
          rows={3}
          placeholder="e.g. Draft a reply to Jane Doe, SSN 123-45-6789, callback 555.123.4567"
          className="w-full rounded-lg border bg-transparent px-3 py-2 font-mono text-xs text-fg"
          style={{ borderColor: 'var(--line)' }}
        />
        <div className="mt-2 flex items-center gap-3">
          <button
            onClick={sendSample}
            disabled={sampling || !freshGateway || sampleText.trim() === ''}
            className="rounded-lg px-4 py-1.5 text-xs font-medium text-bg disabled:opacity-50"
            style={{ background: 'var(--accent)' }}
          >
            {sampling ? 'Sending…' : 'Send through gateway'}
          </button>
          <span className="text-[11px] text-faint">⌘⏎</span>
          {sampleError && <span className="text-xs" style={{ color: 'var(--red, #ef4444)' }}>{sampleError}</span>}
        </div>
        {sample && (
          <div className="mt-3 space-y-3">
            <div className="flex flex-wrap items-center gap-2">
              <Chip ok={sample.action !== ''} label={`action: ${sample.action || 'unknown'}`} />
              <Chip
                ok={sample.report_joined}
                label={sample.report_joined
                  ? `${(sample.redactions ?? []).length} redaction${(sample.redactions ?? []).length === 1 ? '' : 's'}`
                  : 'gateway report not joined'}
              />
              <span className="text-[11px] tabular-nums text-muted">
                HTTP {sample.http_status} · {sample.latency_ms}ms · pack v{sample.pack_version} · {sample.detector_chain || 'regex'}
              </span>
            </div>
            <div className="grid gap-3 md:grid-cols-2">
              <div className="panel-2 p-3">
                <div className="mb-1.5 text-[10px] font-semibold uppercase tracking-wide text-faint">Sent to gateway</div>
                <p className="whitespace-pre-wrap font-mono text-xs leading-5">
                  <SampleMarkedText content={sampleSent} redactions={sample.redactions ?? []} />
                </p>
              </div>
              <div className="panel-2 p-3">
                <div className="mb-1.5 text-[10px] font-semibold uppercase tracking-wide text-faint">Received by upstream</div>
                {sample.upstream_text ? (
                  <p className="whitespace-pre-wrap font-mono text-xs leading-5">{sample.upstream_text}</p>
                ) : (
                  <p className="text-xs italic text-muted">No upstream capture — blocked, errored, or a non-synthetic upstream.</p>
                )}
              </div>
            </div>
            {sample.response_text && (
              <div className="panel-2 p-3">
                <div className="mb-1.5 text-[10px] font-semibold uppercase tracking-wide text-faint">Model reply</div>
                <p className="whitespace-pre-wrap font-mono text-xs leading-5">{sample.response_text}</p>
              </div>
            )}
          </div>
        )}
      </div>

      {/* run config */}
      <div className="mb-5 panel p-4">
        <h3 className="mb-3 text-sm font-semibold">Run configuration</h3>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
          <label className="flex flex-col gap-1 text-[11px] text-muted">
            Requests (≤2000)
            <input type="number" min={1} max={2000} value={cfg.count}
              onChange={(e) => setCfg({ ...cfg, count: Number(e.target.value) })}
              className="rounded-lg border bg-transparent px-2 py-1.5 text-sm text-fg" style={{ borderColor: 'var(--line)' }} />
          </label>
          <label className="flex flex-col gap-1 text-[11px] text-muted">
            Concurrency (≤8)
            <input type="number" min={1} max={8} value={cfg.concurrency}
              onChange={(e) => setCfg({ ...cfg, concurrency: Number(e.target.value) })}
              className="rounded-lg border bg-transparent px-2 py-1.5 text-sm text-fg" style={{ borderColor: 'var(--line)' }} />
          </label>
          <label className="flex flex-col gap-1 text-[11px] text-muted">
            Corpus replay %
            <input type="number" min={0} max={90} value={cfg.replay_percent}
              onChange={(e) => setCfg({ ...cfg, replay_percent: Number(e.target.value) })}
              className="rounded-lg border bg-transparent px-2 py-1.5 text-sm text-fg" style={{ borderColor: 'var(--line)' }} />
          </label>
          <label className="flex flex-col gap-1 text-[11px] text-muted">
            Seed (blank = random)
            <input value={seedText} placeholder="reproducible run"
              onChange={(e) => setSeedText(e.target.value)}
              className="rounded-lg border bg-transparent px-2 py-1.5 text-sm text-fg" style={{ borderColor: 'var(--line)' }} />
          </label>
          <div className="col-span-2 flex flex-wrap items-end gap-4 text-xs">
            {([
              ['include_traps', 'FP traps'],
              ['include_presidio_only', 'Names/addresses (Presidio)'],
              ['include_straddle', 'SSE straddle'],
            ] as const).map(([key, label]) => (
              <label key={key} className="flex items-center gap-1.5">
                <input type="checkbox" checked={cfg[key]} onChange={(e) => setCfg({ ...cfg, [key]: e.target.checked })} />
                {label}
              </label>
            ))}
          </div>
        </div>
      </div>

      {/* live progress / summary */}
      {activeRun && (
        <div className="mb-5 panel p-4">
          <div className="mb-3 flex items-center justify-between">
            <h3 className="text-sm font-semibold">
              {running ? 'Run in progress' : 'Last run'} · pack v{activeRun.pack_version} · {activeRun.detector_chain || 'regex'} · seed {activeRun.config.seed}
            </h3>
            <span className="text-xs tabular-nums text-muted">{activeRun.completed}/{activeRun.total}</span>
          </div>
          <div className="mb-3 h-1.5 overflow-hidden rounded-full" style={{ background: 'var(--line)' }}>
            <div className="h-full rounded-full transition-all" style={{ width: `${(activeRun.completed / Math.max(1, activeRun.total)) * 100}%`, background: 'var(--accent)' }} />
          </div>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-6">
            <ScoreCard label="masked" value={String(activeRun.masked)} />
            <ScoreCard label="blocked" value={String(activeRun.blocked)} />
            <ScoreCard label="detect-only" value={String(activeRun.detect_only)} />
            <ScoreCard label="errors" value={String(activeRun.errors)} tone={activeRun.errors > 0 ? 'bad' : 'plain'} />
            <ScoreCard label="promoted to corpus" value={String(activeRun.promoted_count)} />
            <ScoreCard label="response leaks (info)" value={String(score?.response_leaks ?? 0)} />
          </div>
        </div>
      )}

      {/* score */}
      {score && (
        <div className="mb-5 panel p-4">
          <h3 className="mb-3 text-sm font-semibold">Score</h3>
          <div className="mb-4 grid grid-cols-2 gap-2 sm:grid-cols-5">
            <ScoreCard label="recall (behavioral)" value={pct(score.recall_behavioral)} tone={score.recall_behavioral >= 0.95 ? 'good' : 'bad'} />
            <ScoreCard label="recall (reported)" value={pct(score.recall_reported)} />
            <ScoreCard label="precision" value={pct(score.precision)} tone={score.precision >= 0.97 ? 'good' : 'bad'} />
            <ScoreCard label="trap FPs" value={String(score.trap_fps)} tone={score.trap_fps === 0 ? 'good' : 'bad'} />
            <ScoreCard label="join coverage" value={`${score.joined_reports}/${score.joined_reports + score.orphan_requests}`} />
          </div>
          <table className="w-full text-left text-xs">
            <thead className="text-[10px] uppercase tracking-wide text-faint">
              <tr><th className="pb-1">type</th><th>tp</th><th>fn</th><th>fp</th><th>behavioral misses</th></tr>
            </thead>
            <tbody className="tabular-nums">
              {Object.entries(score.by_type).map(([typ, ts]) => (
                <tr key={typ} className="border-t" style={{ borderColor: 'var(--line)' }}>
                  <td className="py-1 font-mono">{typ}</td>
                  <td>{ts.tp}</td>
                  <td style={{ color: ts.fn > 0 ? 'var(--red, #ef4444)' : undefined }}>{ts.fn}</td>
                  <td style={{ color: ts.fp > 0 ? 'var(--amber, #f59e0b)' : undefined }}>{ts.fp}</td>
                  <td style={{ color: ts.behavioral_misses > 0 ? 'var(--red, #ef4444)' : undefined }}>{ts.behavioral_misses}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <div className="mt-2 text-[11px] text-muted">
            engines: {Object.entries(score.by_engine).map(([e, n]) => `${e} ×${n}`).join(' · ') || '—'}
          </div>
        </div>
      )}

      {/* ratchet trend */}
      <div className="mb-5 panel p-4">
        <div className="mb-2 flex items-center justify-between">
          <h3 className="text-sm font-semibold">Recall ratchet</h3>
          <span className="text-[11px] text-muted">behavioral recall per run — the number this gateway publishes</span>
        </div>
        {ratchetValues.length >= 2 ? (
          <Sparkline values={ratchetValues} width={420} height={56} />
        ) : (
          <p className="text-xs italic text-muted">Run twice (approve a pattern in between) to draw the trend.</p>
        )}
        <div className="mt-2 space-y-1">
          {(ratchet.data ?? []).slice(-8).reverse().map((p) => (
            <div key={p.run_id} className="flex flex-wrap gap-3 text-[11px] tabular-nums text-muted">
              <span>{new Date(p.at).toLocaleTimeString()}</span>
              <span>recall {pct(p.recall_behavioral)}</span>
              <span>precision {pct(p.precision)}</span>
              <span>pack v{p.pack_version}</span>
              <span>{p.detector_chain || 'regex'}</span>
              <span>{p.request_count} req · seed {p.seed}</span>
            </div>
          ))}
        </div>
      </div>

      {/* flywheel proposals */}
      <div className="mb-5 panel p-4">
        <div className="mb-2 flex items-center justify-between">
          <h3 className="text-sm font-semibold">Flywheel · pattern proposals</h3>
          <span className="text-[11px] text-muted">human-approved only — nothing auto-applies</span>
        </div>
        {(proposals.data ?? []).length === 0 && <p className="text-xs italic text-muted">No proposals yet — run traffic and misses will surface here.</p>}
        <div className="space-y-2">
          {(proposals.data ?? []).map((p) => (
            <div key={p.id} className="panel-2 flex flex-wrap items-center gap-3 px-3 py-2">
              <span className="font-mono text-xs">{p.id}</span>
              <span className="rounded-full border px-2 py-0.5 text-[10px]" style={{ borderColor: 'var(--line)' }}>{p.type}</span>
              <span
                className="rounded-full border px-2 py-0.5 text-[10px] uppercase tracking-wide"
                style={{
                  // allow_list is the one kind that REMOVES enforcement, so it
                  // is coloured apart from the additive kinds rather than
                  // reading as one more rule in the same list.
                  borderColor: p.kind === 'allow_list' ? 'var(--amber)' : 'var(--line)',
                  color: p.kind === 'allow_list' ? 'var(--amber)' : 'var(--faint)',
                }}
              >
                {p.kind === 'deny_list'
                  ? 'deny list'
                  : p.kind === 'allow_list'
                    ? 'allow list · suppresses'
                    : p.kind === 'threshold'
                      ? 'score gate'
                      : p.kind === 'regex' || p.regex
                        ? 'regex'
                        : 'no artifact'}
              </span>
              <span className="text-[11px] text-muted">{p.rationale}</span>
              {p.regex && <code className="max-w-[280px] truncate font-mono text-[10px] text-faint">{p.regex}</code>}
              {p.kind === 'deny_list' && (p.deny_list?.length ?? 0) > 0 && (
                <code className="max-w-[280px] truncate font-mono text-[10px] text-faint" title={p.deny_list!.join(', ')}>
                  {p.deny_list!.slice(0, 4).join(' · ')}{p.deny_list!.length > 4 ? ` +${p.deny_list!.length - 4}` : ''}
                </code>
              )}
              {p.kind === 'allow_list' && (p.allow_list?.length ?? 0) > 0 && (
                <code className="max-w-[280px] truncate font-mono text-[10px]" style={{ color: 'var(--amber)' }} title={p.allow_list!.join(', ')}>
                  {p.allow_list!.slice(0, 4).join(' · ')}{p.allow_list!.length > 4 ? ` +${p.allow_list!.length - 4}` : ''}
                </code>
              )}
              {p.kind === 'threshold' && (
                <code className="font-mono text-[10px] text-faint">gate → {p.threshold?.toFixed(2)}</code>
              )}
              <span className="text-[11px] tabular-nums text-muted">{p.sample_misses} miss{p.sample_misses === 1 ? '' : 'es'}</span>
              <span className="ml-auto flex items-center gap-2">
                {p.status === 'proposed' ? (
                  <>
                    <button onClick={() => decide(p, 'approve')} className="rounded-lg px-3 py-1 text-xs font-medium text-bg" style={{ background: 'var(--accent)' }}>Approve</button>
                    <button onClick={() => decide(p, 'reject')} className="rounded-lg border px-3 py-1 text-xs" style={{ borderColor: 'var(--line)' }}>Reject</button>
                  </>
                ) : (
                  <span className="text-[10px] uppercase tracking-wide text-faint">{p.status}</span>
                )}
              </span>
            </div>
          ))}
        </div>
      </div>

      {/* results */}
      {results.data && (
        <div className="mb-5 panel p-4">
          <div className="mb-2 flex items-center justify-between">
            <h3 className="text-sm font-semibold">Per-request results</h3>
            <label className="flex items-center gap-1.5 text-[11px] text-muted">
              <input type="checkbox" checked={onlyMisses} onChange={(e) => setOnlyMisses(e.target.checked)} />
              only misses
            </label>
          </div>
          <table className="w-full text-left text-xs">
            <thead className="text-[10px] uppercase tracking-wide text-faint">
              <tr><th className="pb-1">template</th><th>action</th><th>seeded</th><th>missed</th><th>latency</th><th></th></tr>
            </thead>
            <tbody>
              {shownResults.slice(0, 100).map((r) => (
                <Fragment key={r.request_id}>
                  <tr className="border-t" style={{ borderColor: 'var(--line)' }}>
                    <td className="py-1.5 font-mono">{r.template}{r.straddle ? ' ⚡sse' : ''}</td>
                    <td>{r.action || (r.error ? 'error' : '—')}</td>
                    <td className="text-muted">{r.truth.map((t) => t.type).join(', ') || '—'}</td>
                    <td style={{ color: (r.missed_types?.length ?? 0) > 0 ? 'var(--red, #ef4444)' : undefined }}>
                      {r.missed_types?.join(', ') || '—'}
                    </td>
                    <td className="tabular-nums text-muted">{r.latency_ms}ms</td>
                    <td className="text-right">
                      <button onClick={() => setOpenRow(openRow === r.request_id ? null : r.request_id)} className="text-[11px] text-accent">
                        {openRow === r.request_id ? 'hide' : 'diff'}
                      </button>
                    </td>
                  </tr>
                  {openRow === r.request_id && (
                    <tr>
                      <td colSpan={6} className="py-2">
                        <RedactionDiff content={r.content} truth={r.truth} traps={r.traps} redactions={r.redactions} upstreamText={r.upstream_text} />
                      </td>
                    </tr>
                  )}
                </Fragment>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* corpus browser */}
      <div className="mb-5 panel p-4">
        <button onClick={() => setCorpusOpen(!corpusOpen)} className="flex w-full items-center justify-between text-sm font-semibold">
          <span>Promoted corpus {corpusOpen ? '▾' : '▸'}</span>
          <span className="text-[11px] font-normal text-muted">misses accumulate here as the regression set</span>
        </button>
        {corpusOpen && (
          <div className="mt-3 space-y-2">
            {(corpus.data ?? []).length === 0 && <p className="text-xs italic text-muted">Empty — no misses promoted yet.</p>}
            {(corpus.data ?? []).slice(0, 50).map((e) => (
              <div key={e.id} className="panel-2 px-3 py-2">
                <div className="mb-1 flex flex-wrap gap-3 text-[10px] text-faint">
                  <span className="font-mono">{e.id}</span>
                  <span>missed: {e.miss_types.join(', ')}</span>
                  <span>{new Date(e.added_at).toLocaleString()}</span>
                </div>
                <p className="font-mono text-xs text-muted">{e.text}</p>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
