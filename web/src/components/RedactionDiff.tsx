import type { ReactNode } from 'react'
import type { GatewayRedaction, TruthSpan } from '../lib/api.ts'

// Span-highlighted before/after view for one harness request. Left: the
// generated content with truth spans (green = detected, red = missed) and
// false-positive detections (amber). Right: what the mock upstream received.
// Every value on screen is synthetic by construction.

type Mark = { start: number; end: number; kind: 'detected' | 'missed' | 'fp' | 'trap' }

const MARK_STYLE: Record<Mark['kind'], { background: string; borderBottom: string; title: string }> = {
  detected: { background: 'color-mix(in srgb, var(--green, #22c55e) 18%, transparent)', borderBottom: '2px solid var(--green, #22c55e)', title: 'seeded PII — detected' },
  missed: { background: 'color-mix(in srgb, var(--red, #ef4444) 24%, transparent)', borderBottom: '2px solid var(--red, #ef4444)', title: 'seeded PII — MISSED' },
  fp: { background: 'color-mix(in srgb, var(--amber, #f59e0b) 24%, transparent)', borderBottom: '2px dashed var(--amber, #f59e0b)', title: 'detected but not seeded (false positive)' },
  trap: { background: 'transparent', borderBottom: '1px dotted var(--muted)', title: 'trap — must not be detected' },
}

function overlap(aStart: number, aEnd: number, bStart: number, bEnd: number) {
  return aStart < bEnd && bStart < aEnd
}

function buildMarks(truth: TruthSpan[], traps: TruthSpan[], redactions: GatewayRedaction[]): Mark[] {
  const contentReds = redactions.filter((r) => r.path.includes('content'))
  const marks: Mark[] = []
  for (const t of truth) {
    const hit = contentReds.some((r) => r.type === t.type && overlap(r.start, r.end, t.start, t.end))
    marks.push({ start: t.start, end: t.end, kind: hit ? 'detected' : 'missed' })
  }
  for (const t of traps) {
    const hit = contentReds.some((r) => overlap(r.start, r.end, t.start, t.end))
    marks.push({ start: t.start, end: t.end, kind: hit ? 'fp' : 'trap' })
  }
  for (const r of contentReds) {
    const covered = [...truth, ...traps].some((t) => overlap(r.start, r.end, t.start, t.end))
    if (!covered) marks.push({ start: r.start, end: r.end, kind: 'fp' })
  }
  marks.sort((a, b) => a.start - b.start || b.end - a.end)
  // drop overlapping later marks so segmentation stays simple
  const flat: Mark[] = []
  for (const m of marks) {
    const last = flat[flat.length - 1]
    if (!last || m.start >= last.end) flat.push(m)
  }
  return flat
}

function MarkedText({ content, marks }: { content: string; marks: Mark[] }) {
  const parts: ReactNode[] = []
  let pos = 0
  marks.forEach((m, i) => {
    if (m.start > pos) parts.push(<span key={`p${i}`}>{content.slice(pos, m.start)}</span>)
    const s = MARK_STYLE[m.kind]
    parts.push(
      <span key={`m${i}`} title={s.title} style={{ background: s.background, borderBottom: s.borderBottom, borderRadius: 2 }}>
        {content.slice(m.start, m.end)}
      </span>,
    )
    pos = m.end
  })
  if (pos < content.length) parts.push(<span key="tail">{content.slice(pos)}</span>)
  return <>{parts}</>
}

export default function RedactionDiff({
  content,
  truth,
  traps = [],
  redactions = [],
  upstreamText,
}: {
  content: string
  truth: TruthSpan[]
  traps?: TruthSpan[]
  redactions?: GatewayRedaction[]
  upstreamText?: string
}) {
  const marks = buildMarks(truth, traps, redactions)
  return (
    <div className="grid gap-3 md:grid-cols-2">
      <div className="panel-2 p-3">
        <div className="mb-1.5 text-[10px] font-semibold uppercase tracking-wide text-faint">Sent to gateway (ground truth)</div>
        <p className="whitespace-pre-wrap font-mono text-xs leading-5">
          <MarkedText content={content} marks={marks} />
        </p>
      </div>
      <div className="panel-2 p-3">
        <div className="mb-1.5 text-[10px] font-semibold uppercase tracking-wide text-faint">Received by upstream</div>
        {upstreamText ? (
          <p className="whitespace-pre-wrap font-mono text-xs leading-5">{upstreamText}</p>
        ) : (
          <p className="text-xs italic text-muted">Nothing reached upstream (blocked or errored).</p>
        )}
      </div>
      <div className="md:col-span-2 flex flex-wrap gap-4 text-[10px] text-muted">
        <span><span style={{ borderBottom: MARK_STYLE.detected.borderBottom }}>detected</span></span>
        <span><span style={{ borderBottom: MARK_STYLE.missed.borderBottom }}>missed</span></span>
        <span><span style={{ borderBottom: MARK_STYLE.fp.borderBottom }}>false positive</span></span>
        <span><span style={{ borderBottom: MARK_STYLE.trap.borderBottom }}>trap (must stay silent)</span></span>
      </div>
    </div>
  )
}
