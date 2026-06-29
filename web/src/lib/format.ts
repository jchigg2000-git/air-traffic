export function fmtUSD(n: number, opts: { compact?: boolean } = {}): string {
  if (opts.compact && Math.abs(n) >= 1000) {
    return '$' + (n / 1000).toFixed(1) + 'K'
  }
  return n.toLocaleString('en-US', { style: 'currency', currency: 'USD', maximumFractionDigits: 0 })
}

export function fmtNum(n: number): string {
  if (Math.abs(n) >= 1_000_000) return (n / 1_000_000).toFixed(2) + 'M'
  if (Math.abs(n) >= 1000) return (n / 1000).toFixed(1) + 'K'
  return Math.round(n).toLocaleString('en-US')
}

export function relativeTime(iso: string | undefined, now = Date.now()): string {
  if (!iso) return '—'
  const t = new Date(iso).getTime()
  if (Number.isNaN(t)) return '—'
  const secs = Math.max(0, Math.round((now - t) / 1000))
  if (secs < 60) return secs + 's ago'
  const mins = Math.round(secs / 60)
  if (mins < 60) return mins + 'm ago'
  const hrs = Math.round(mins / 60)
  if (hrs < 24) return hrs + 'h ago'
  return Math.round(hrs / 24) + 'd ago'
}

/** Age in seconds, for freshness coloring. */
export function ageSeconds(iso: string | undefined, now = Date.now()): number {
  if (!iso) return Infinity
  const t = new Date(iso).getTime()
  if (Number.isNaN(t)) return Infinity
  return Math.max(0, (now - t) / 1000)
}

export type Rag = 'green' | 'amber' | 'red'

export function freshnessRag(ageSec: number): Rag {
  if (ageSec <= 20) return 'green'
  if (ageSec <= 90) return 'amber'
  return 'red'
}

export function titleCase(s: string): string {
  return s.replace(/[_-]/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())
}
