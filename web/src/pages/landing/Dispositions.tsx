import { DISPOSITIONS, type Disposition } from '../../lib/dispositions.ts'

// Bespoke section: the five-disposition legend. Labels, glyphs, colors and blurbs
// are reused from the shared dispositions lib (the same source the Flight Deck uses)
// so the landing page can never drift from the product's own truthfulness token.
// The set of rows and the framing copy are owned here, inline.

const ROWS: Disposition[] = ['vendor_native', 'env_managed', 'proxy_enforced', 'monitor_only', 'unverified']

export default function Dispositions() {
  return (
    <section className="mx-auto max-w-5xl px-5 py-16 md:py-20">
      <div className="max-w-2xl">
        <p className="text-xs font-semibold uppercase tracking-[0.2em] text-accent">The truthfulness token</p>
        <h2 className="mt-3 text-3xl font-semibold tracking-tight text-fg sm:text-4xl">Five dispositions. No overstatement.</h2>
        <p className="mt-5 text-base leading-relaxed text-muted">
          Every control Air-Traffic reports carries a disposition that says exactly <span className="text-fg">how</span> it
          is actually enforced. It is the signature of the product: the board never claims more control than the vendor
          truly grants.
        </p>
      </div>

      <ul className="mt-10 grid gap-3 sm:grid-cols-2">
        {ROWS.map((key) => {
          const m = DISPOSITIONS[key]
          return (
            <li key={key} className="panel flex items-start gap-3.5 p-4">
              <span
                aria-hidden
                className="mt-0.5 grid h-8 w-8 shrink-0 place-items-center rounded-lg text-lg"
                style={{
                  color: m.color,
                  background: `color-mix(in srgb, ${m.color} 14%, transparent)`,
                  border: `1px solid color-mix(in srgb, ${m.color} 40%, transparent)`,
                }}
              >
                {m.glyph}
              </span>
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-semibold" style={{ color: m.color }}>
                    {m.label}
                  </span>
                  <span className="font-mono text-[10px] text-faint">{key}</span>
                </div>
                <p className="mt-1 text-sm leading-snug text-muted">{m.blurb}</p>
              </div>
            </li>
          )
        })}
      </ul>

      {/* Truthfulness callout */}
      <div
        className="mt-4 flex items-start gap-3 rounded-xl border p-4"
        style={{
          borderColor: 'color-mix(in srgb, var(--unverified) 40%, transparent)',
          background: 'color-mix(in srgb, var(--unverified) 9%, transparent)',
        }}
      >
        <span aria-hidden className="mt-0.5 text-lg text-unverified">
          ⚠
        </span>
        <p className="text-sm leading-relaxed text-muted">
          <span className="font-semibold text-fg">Honesty rule.</span> A seed-only <span className="text-env">Env-Managed</span>{' '}
          control is drift-detected but is <span className="font-semibold text-fg">never presented as enforced</span>. And{' '}
          <span className="text-proxy">Proxy (opt)</span> stays monitor-only until you turn the optional gateway on — it is
          off by default.
        </p>
      </div>
    </section>
  )
}
