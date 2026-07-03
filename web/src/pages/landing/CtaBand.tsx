import { Link } from 'react-router-dom'

// Bespoke section: the closing call-to-action band plus the "everything is
// local / synthetic" honesty note. Copy is owned here inline.

export default function CtaBand() {
  return (
    <section className="mx-auto max-w-5xl px-5 py-16 md:py-20">
      <div className="panel relative overflow-hidden p-8 text-center md:p-14">
        <div className="grid-fade pointer-events-none absolute inset-0" />
        <div className="relative mx-auto max-w-2xl">
          <h2 className="text-3xl font-semibold tracking-tight text-fg sm:text-4xl">Take the tower.</h2>
          <p className="mt-4 text-base leading-relaxed text-muted">
            See all 16 vendors, three planes, and every disposition on one board — then prove inline redaction in the
            gateway harness.
          </p>

          <div className="mt-8 flex flex-wrap items-center justify-center gap-3">
            <Link
              to="/"
              className="inline-flex items-center gap-2 rounded-xl px-6 py-3 text-sm font-semibold text-bg transition hover:opacity-90"
              style={{ background: 'var(--accent)' }}
            >
              Enter the Control Tower
              <span aria-hidden>→</span>
            </Link>
            <Link
              to="/settings/harness"
              className="inline-flex items-center gap-2 rounded-xl border border-line bg-panel2 px-6 py-3 text-sm font-semibold text-fg transition hover:border-accent"
            >
              See redaction proof — Gateway Harness
            </Link>
          </div>

          <p className="mx-auto mt-8 flex max-w-xl items-center justify-center gap-2 text-xs leading-relaxed text-faint">
            <span
              className="inline-block h-1.5 w-1.5 shrink-0 rounded-full"
              style={{ background: 'var(--env)' }}
              aria-hidden
            />
            Everything here runs local and synthetic — synthetic vendor surfaces, self-hosted NER, no cloud inference. The
            controls are real; the traffic is not.
          </p>
        </div>
      </div>
    </section>
  )
}
