// Bespoke section: the problem framing ("the spine, not an inline proxy") plus
// the three control planes. Copy + the plane cards are co-located here — no shared
// data array feeds this; the three cards are written out explicitly below.

function SpineIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M12 3v18" />
      <path d="M12 7h6M12 12h6M12 17h6M12 7H6M12 12H6M12 17H6" />
      <circle cx="12" cy="3" r="1.4" fill="currentColor" />
    </svg>
  )
}

function DevIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="m8 8-4 4 4 4M16 8l4 4-4 4M14 5l-4 14" />
    </svg>
  )
}

function ShieldIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M12 3 4 6v6c0 5 3.5 7.5 8 9 4.5-1.5 8-4 8-9V6l-8-3Z" />
      <path d="m9 12 2 2 4-4" />
    </svg>
  )
}

function BudgetIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M3 3v18h18" />
      <path d="m7 15 3-4 3 3 4-6" />
    </svg>
  )
}

export default function Planes() {
  return (
    <section className="mx-auto max-w-5xl px-5 py-16 md:py-20">
      {/* The problem / what it is */}
      <div className="max-w-2xl">
        <p className="text-xs font-semibold uppercase tracking-[0.2em] text-accent">The spine, not a proxy</p>
        <h2 className="mt-3 text-3xl font-semibold tracking-tight text-fg sm:text-4xl">
          Every enterprise now runs a dozen AI vendors. None of them share a governance surface.
        </h2>
        <p className="mt-5 text-base leading-relaxed text-muted">
          Spend, policy, and audit each live in a different vendor console — if they exist at all. Air-Traffic sits
          <span className="text-fg"> alongside</span> those vendors as a single spine: it drives their native admin APIs,
          pushes managed config into your dev and agent environments, and normalizes everything into one signal. It is
          <span className="text-fg"> not</span> an inline inference proxy — the request path stays yours.
        </p>
      </div>

      {/* Three control planes — written out explicitly, one card each */}
      <div className="mt-12 grid gap-4 md:grid-cols-3">
        <article className="panel flex flex-col gap-3 p-6">
          <div className="flex items-center gap-2.5">
            <span className="grid h-9 w-9 place-items-center rounded-lg border border-line bg-panel2 text-accent">
              <DevIcon />
            </span>
            <h3 className="text-base font-semibold text-fg">Developer-workflow</h3>
          </div>
          <p className="text-sm leading-relaxed text-muted">
            Governs the coding assistants and agent runtimes your engineers touch daily — model access, allowed tools,
            and workspace policy pushed into the environment and read back for drift.
          </p>
        </article>

        <article className="panel flex flex-col gap-3 p-6">
          <div className="flex items-center gap-2.5">
            <span className="grid h-9 w-9 place-items-center rounded-lg border border-line bg-panel2 text-env">
              <ShieldIcon />
            </span>
            <h3 className="text-base font-semibold text-fg">Data-policy</h3>
          </div>
          <p className="text-sm leading-relaxed text-muted">
            Tracks retention, training opt-out, BAA status, and PII/PHI handling per vendor. When the optional gateway is
            enabled, redaction becomes enforceable inline — until then it is monitored and alerted.
          </p>
        </article>

        <article className="panel flex flex-col gap-3 p-6">
          <div className="flex items-center gap-2.5">
            <span className="grid h-9 w-9 place-items-center rounded-lg border border-line bg-panel2 text-accent2">
              <BudgetIcon />
            </span>
            <h3 className="text-base font-semibold text-fg">Budget</h3>
          </div>
          <p className="text-sm leading-relaxed text-muted">
            Rolls token and dollar spend up across every vendor against org and team caps, with live throughput so a
            runaway agent shows up on the board before it shows up on the invoice.
          </p>
        </article>
      </div>

      <p className="mt-6 flex items-center gap-2 text-xs text-faint">
        <span className="text-accent">
          <SpineIcon />
        </span>
        One normalized <span className="font-mono text-muted">ops-observation-batch/v1</span> feeds all three planes.
      </p>
    </section>
  )
}
