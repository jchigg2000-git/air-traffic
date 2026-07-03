import type { ReactNode } from 'react'

// Bespoke section: how the spine actually works — three mechanisms plus the one
// optional inline component. Steps and copy are written out inline here.

function StepCard({
  step,
  token,
  tokenColor,
  title,
  children,
}: {
  step: string
  token: string
  tokenColor: string
  title: string
  children: ReactNode
}) {
  return (
    <article className="panel relative flex flex-col gap-3 p-6">
      <div className="flex items-center justify-between">
        <span className="font-mono text-xs text-faint">{step}</span>
        <span
          className="rounded-full border px-2 py-0.5 font-mono text-[10px] font-medium"
          style={{
            color: tokenColor,
            borderColor: `color-mix(in srgb, ${tokenColor} 40%, transparent)`,
            background: `color-mix(in srgb, ${tokenColor} 12%, transparent)`,
          }}
        >
          {token}
        </span>
      </div>
      <h3 className="text-base font-semibold text-fg">{title}</h3>
      <p className="text-sm leading-relaxed text-muted">{children}</p>
    </article>
  )
}

export default function HowItWorks() {
  return (
    <section className="mx-auto max-w-5xl px-5 py-16 md:py-20">
      <div className="max-w-2xl">
        <p className="text-xs font-semibold uppercase tracking-[0.2em] text-accent">How it works</p>
        <h2 className="mt-3 text-3xl font-semibold tracking-tight text-fg sm:text-4xl">
          Drive, manage, emit — off the request path.
        </h2>
        <p className="mt-5 text-base leading-relaxed text-muted">
          Three mechanisms produce one normalized signal. Only one component ever touches an inference request, and it is
          opt-in.
        </p>
      </div>

      <div className="mt-10 grid gap-4 md:grid-cols-3">
        <StepCard step="01" token="vendor_native" tokenColor="var(--native)" title="Drive native admin APIs">
          Air-Traffic calls each vendor's own admin, policy, and usage APIs directly. When a control is set this way it is
          genuinely enforced by the vendor — that is what earns the <span className="text-native">Native</span> disposition.
        </StepCard>

        <StepCard step="02" token="env_managed" tokenColor="var(--env)" title="Push managed config, read it back">
          Where there is no server-side API, it pushes managed config into the dev and agent environment, then reads it back
          to detect drift. Seed-only config is <span className="text-fg">watched, not enforced</span>.
        </StepCard>

        <StepCard step="03" token="ops-observation-batch/v1" tokenColor="var(--accent)" title="Emit one normalized batch">
          Everything collapses into a single <span className="font-mono text-fg">ops-observation-batch/v1</span> stream that
          fans out across the developer-workflow, data-policy, and budget planes.
        </StepCard>
      </div>

      {/* Optional inline gateway note */}
      <div className="panel-2 mt-4 flex flex-col gap-3 p-6 sm:flex-row sm:items-center sm:gap-6">
        <div className="flex items-center gap-3">
          <span
            className="grid h-11 w-11 shrink-0 place-items-center rounded-xl text-xl"
            style={{
              color: 'var(--proxy)',
              background: 'color-mix(in srgb, var(--proxy) 14%, transparent)',
              border: '1px solid color-mix(in srgb, var(--proxy) 40%, transparent)',
            }}
            aria-hidden
          >
            ✚
          </span>
          <div className="sm:hidden">
            <h3 className="text-base font-semibold text-fg">The optional inference gateway</h3>
          </div>
        </div>
        <div>
          <h3 className="hidden text-base font-semibold text-fg sm:block">The optional inference gateway</h3>
          <p className="mt-1 text-sm leading-relaxed text-muted">
            One separate data-plane binary is the only thing that ever sits on the request path. When enabled, it redacts
            PII/PHI inline and flips those controls to <span className="text-proxy">Proxy (opt)</span> — truly enforced. It
            is <span className="font-semibold text-fg">off by default</span>; nothing intercepts traffic unless you switch it on.
          </p>
        </div>
      </div>
    </section>
  )
}
