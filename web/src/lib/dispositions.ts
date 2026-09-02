// The five (six) dispositions are a first-class visual token. Truthfulness layer:
// the UI must never overstate enforcement — seed_only is never "enforced".

export type Disposition =
  | 'vendor_native'
  | 'env_managed'
  | 'proxy_enforced'
  | 'monitor_only'
  | 'unverified'
  | 'unsupported'

interface DispMeta {
  label: string
  glyph: string
  color: string // CSS var token name (without var())
  blurb: string
}

export const DISPOSITIONS: Record<Disposition, DispMeta> = {
  vendor_native: { label: 'Native', glyph: '■', color: 'var(--native)', blurb: 'Driven via the vendor admin API.' },
  env_managed: { label: 'Env-Managed', glyph: '◆', color: 'var(--env)', blurb: 'Managed config pushed into the environment; read back for drift.' },
  proxy_enforced: { label: 'Proxy (opt)', glyph: '✚', color: 'var(--proxy)', blurb: 'Needs the optional gateway (off by default) — behaves as monitor-only until enabled.' },
  monitor_only: { label: 'Monitor', glyph: '◉', color: 'var(--monitor)', blurb: 'Verified + alerting; no set mechanism.' },
  unverified: { label: 'Unverified', glyph: '?', color: 'var(--unverified)', blurb: 'Not confirmed from live vendor docs.' },
  unsupported: { label: 'Unsupported', glyph: '✗', color: 'var(--unsupported)', blurb: 'Vendor explicitly cannot.' },
}

export function dispMeta(d: string): DispMeta {
  return DISPOSITIONS[d as Disposition] ?? DISPOSITIONS.unverified
}

const ENFORCEMENT_LABEL: Record<string, { label: string; enforced: boolean }> = {
  server_side: { label: 'Server-side', enforced: true },
  mdm_locked: { label: 'MDM-locked', enforced: true },
  seed_only: { label: 'Seed-only', enforced: false }, // NEVER shown as "enforced"
}

export function enforcementMeta(e: string): { label: string; enforced: boolean } | null {
  return ENFORCEMENT_LABEL[e] ?? null
}
