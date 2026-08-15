import { dispMeta, enforcementMeta } from '../lib/dispositions.ts'

export default function DispositionChip({
  disposition,
  enforcement,
  size = 'sm',
}: {
  disposition: string
  enforcement?: string
  size?: 'sm' | 'xs'
}) {
  const m = dispMeta(disposition)
  const enf = enforcement ? enforcementMeta(enforcement) : null
  const pad = size === 'xs' ? 'px-1.5 py-0.5 text-[10px]' : 'px-2 py-0.5 text-xs'
  // Border + glyph + text carry the disposition; the background fill was a third stacked colour
  // treatment that turned an 18-chip row into a wall of tint.
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-full border font-medium ${pad}`}
      style={{
        color: m.color,
        borderColor: `color-mix(in srgb, ${m.color} 45%, transparent)`,
      }}
      title={m.blurb}
    >
      <span aria-hidden style={{ lineHeight: 1 }}>{m.glyph}</span>
      {m.label}
      {enf && (
        <span
          className="ml-0.5 rounded-sm px-1 text-[9px] uppercase tracking-wide"
          style={{
            color: enf.enforced ? 'var(--fg)' : 'var(--amber)',
            background: enf.enforced ? 'color-mix(in srgb, var(--fg) 12%, transparent)' : 'color-mix(in srgb, var(--amber) 16%, transparent)',
          }}
          title={enf.enforced ? 'Enforced' : 'Seed-only — drift-detected, NOT enforced'}
        >
          {enf.label}
        </span>
      )}
    </span>
  )
}
