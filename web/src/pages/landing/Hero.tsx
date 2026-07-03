import { Link } from 'react-router-dom'
import Brand from '../../components/Brand.tsx'
import VendorGlyph from '../../components/VendorGlyph.tsx'

// Bespoke Hero section — owns its own copy, its teaser coverage strip, and the
// two primary CTAs. Nothing here is driven by a shared content registry.

const COVERAGE_TEASER = [
  'openai',
  'anthropic',
  'bedrock',
  'azure_openai',
  'vertex',
  'github_copilot',
  'm365_copilot',
  'mistral',
  'databricks',
  'perplexity',
]

export default function Hero() {
  return (
    <section className="relative overflow-hidden">
      <div className="grid-fade pointer-events-none absolute inset-x-0 top-0 h-[36rem]" />

      <div className="relative mx-auto max-w-5xl px-5 pb-14 pt-10 md:pt-16">
        {/* Brand + status chip row */}
        <div className="flex flex-wrap items-center justify-between gap-3">
          <Brand size={30} />
          <div className="flex flex-wrap items-center gap-2.5">
            <span className="flex items-center gap-2 rounded-full border border-line bg-panel px-3 py-1.5 text-xs font-medium text-muted">
              <span
                className="pulse-dot inline-block h-2 w-2 rounded-full"
                style={{ background: 'var(--green)', boxShadow: '0 0 8px color-mix(in srgb, var(--green) 70%, transparent)' }}
              />
              spine online
            </span>
            <span className="hidden items-center gap-1.5 rounded-full border border-line bg-panel2 px-3 py-1.5 text-xs text-muted sm:flex">
              <span className="pulse-dot inline-block h-2 w-2 rounded-full" style={{ background: 'var(--accent)' }} />
              emitter · ops-observation-batch/v1 · 5s
            </span>
          </div>
        </div>

        {/* Headline */}
        <div className="mt-12 md:mt-16">
          <p className="text-xs font-semibold uppercase tracking-[0.2em] text-accent">Enterprise AI Control Plane</p>
          <h1 className="mt-4 max-w-3xl text-4xl font-semibold leading-[1.08] tracking-tight text-fg sm:text-5xl md:text-6xl">
            One control tower for every AI vendor.
          </h1>
          <p className="mt-6 max-w-2xl text-lg leading-relaxed text-muted">
            Air-Traffic is the unified control and observability <span className="text-fg">spine</span> across all 16 major
            AI vendors — driving their native admin APIs, managing environment config and reading it back for drift, and
            emitting one normalized signal across developer-workflow, data-policy, and budget.
          </p>

          {/* CTAs */}
          <div className="mt-9 flex flex-wrap items-center gap-3">
            <Link
              to="/"
              className="inline-flex items-center gap-2 rounded-xl px-5 py-3 text-sm font-semibold text-bg transition hover:opacity-90"
              style={{ background: 'var(--accent)' }}
            >
              Enter the Control Tower
              <span aria-hidden>→</span>
            </Link>
            <Link
              to="/settings/harness"
              className="inline-flex items-center gap-2 rounded-xl border border-line bg-panel px-5 py-3 text-sm font-semibold text-fg transition hover:border-accent"
            >
              <span aria-hidden style={{ color: 'var(--proxy)' }}>
                ✚
              </span>
              See redaction proof — Gateway Harness
            </Link>
            <Link to="/settings/rigor" className="px-2 py-3 text-sm font-medium text-muted transition hover:text-accent">
              or tune baseline rigor →
            </Link>
          </div>

          {/* Stat + coverage teaser */}
          <div className="mt-12 flex flex-wrap items-center gap-x-8 gap-y-4">
            <dl className="flex flex-wrap gap-x-8 gap-y-2">
              <Stat n="16" label="vendor adapters" />
              <Stat n="3" label="control planes" />
              <Stat n="5" label="dispositions" />
            </dl>
            <div className="flex items-center gap-1.5">
              {COVERAGE_TEASER.map((id) => (
                <VendorGlyph key={id} id={id} size={26} />
              ))}
              <span className="ml-1 text-xs text-faint">+6</span>
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}

function Stat({ n, label }: { n: string; label: string }) {
  return (
    <div>
      <dt className="sr-only">{label}</dt>
      <dd className="flex items-baseline gap-2">
        <span className="text-2xl font-semibold tabular-nums text-fg">{n}</span>
        <span className="text-xs text-muted">{label}</span>
      </dd>
    </div>
  )
}
