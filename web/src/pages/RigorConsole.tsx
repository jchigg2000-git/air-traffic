import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, qk, type Adapter, type Baseline, type CoverageReport } from '../lib/api.ts'
import PageHeader from '../components/PageHeader.tsx'
import { titleCase } from '../lib/format.ts'

// Hand-written narrative for each preconfigured rigor profile. `posture` maps to the
// three surface cards above it (Data Policy · Developer Workflow · Budget). Profiles
// without an entry fall back to a field-derived narrative (see deriveNarrative).
type Narrative = {
  headline: string
  body: string
  posture: { data_policy: string; developer_workflow: string; budget: string }
}

const NARRATIVE: Record<string, Narrative> = {
  general_saas: {
    headline: 'Balanced defaults for everyday engineering velocity',
    body: 'General SaaS keeps the guardrails light so teams move fast — any approved model is fair game, retention sits at a standard 30 days, and spend ceilings are generous. It is the right floor for internal tools and customer-facing SaaS where the data is not regulated. Tighten to Fintech or Healthcare the moment PII, PHI, or financial flows enter the picture.',
    posture: {
      data_policy: 'Training opt-out on, but ZDR and PII redaction stay off — standard 30-day retention.',
      developer_workflow: 'Open model access; an agentic reviewer gates customer-facing code, not every change.',
      budget: 'Roomy caps ($10K org · $200/user) — monitor-first, not hard-stop.',
    },
  },
  fintech: {
    headline: 'Elevated controls for regulated financial workloads',
    body: 'Fintech assumes money movement and audited data paths. Model access narrows to production-tier only, PII redaction switches on, and retention drops to 7 days. A human signs off on financial flows, and per-user spend is capped tighter so a runaway agent loop never becomes a runaway invoice.',
    posture: {
      data_policy: 'PII redaction on, ZDR where the vendor supports it natively, 7-day retention, elevated content safety.',
      developer_workflow: 'Restricted to prod-tier models; human-required review on financial flows.',
      budget: 'Firm $50K org cap with a $500/user ceiling.',
    },
  },
  healthcare: {
    headline: 'Maximum rigor — BAA-only, zero retention',
    body: 'Healthcare is the strictest data posture: only BAA-signed vendors make the allowlist, retention is zero, and PII handling extends to PHI. Every change is human-reviewed and content safety is maxed. Budgets are board-approved rather than self-served, reflecting that compliance — not cost — is the binding constraint.',
    posture: {
      data_policy: 'ZDR enforced, PHI-aware redaction, zero retention, maximum content safety.',
      developer_workflow: 'BAA-only model allowlist; human-required review on everything.',
      budget: 'Board-approved org budget; $100/user — compliance gates spend, not the reverse.',
    },
  },
  gov_infra: {
    headline: 'FedRAMP / on-prem only, audit-controlled',
    body: 'Gov / Infra mirrors Healthcare’s rigor and adds a sovereignty constraint: models must run FedRAMP-authorized or on-prem, never on uncontrolled infrastructure. Retention is zero and audit-controlled, every action is human-reviewed, and spend is fully human-gated — there are no self-service caps because every dollar maps to an authorized program.',
    posture: {
      data_policy: 'ZDR enforced, on-prem / FedRAMP residency, zero audit-controlled retention.',
      developer_workflow: 'FedRAMP / on-prem models only; human-required review on all changes.',
      budget: 'Fully human-gated — no self-service spend.',
    },
  },
}

// Fallback for any preset shipped without hand-written copy (keeps the block honest
// instead of blank). TODO: add curated NARRATIVE copy if new baselines are introduced.
function deriveNarrative(b: Baseline): Narrative {
  return {
    headline: b.description,
    body: `The ${b.name} profile applies ${b.rigor} rigor across all three control surfaces. The cards above show every resolved control; the posture notes below summarize how this profile leans on each plane.`,
    posture: {
      data_policy: `Training opt-out ${b.training_opt_out ? 'on' : 'off'}, ZDR ${titleCase(b.zdr)}, PII redaction ${titleCase(b.pii_redaction)}, ${b.retention_days === 0 ? 'zero retention' : `${b.retention_days}-day retention`}.`,
      developer_workflow: `Model access ${titleCase(b.model_access)}; ${b.code_review}.`,
      budget: `${b.org_cap_usd ? `$${b.org_cap_usd.toLocaleString()} org cap` : 'Board-approved org budget'}, ${b.user_cap_usd ? `$${b.user_cap_usd}/user` : 'human-required per-user spend'}.`,
    },
  }
}

const OUTCOME_META: Record<string, { label: string; color: string }> = {
  applied_native: { label: 'Native', color: 'var(--native)' },
  applied_env: { label: 'Env-Managed', color: 'var(--env)' },
  proxy_needed: { label: 'Proxy-Needed', color: 'var(--proxy)' },
  monitored: { label: 'Monitored', color: 'var(--monitor)' },
  unverified: { label: 'Unverified', color: 'var(--unverified)' },
  unsupported: { label: 'Unsupported', color: 'var(--unsupported)' },
  excluded_no_baa: { label: 'Excluded (no BAA)', color: 'var(--red)' },
}

const PLANES: { key: string; title: string; controls: (b: Baseline) => { name: string; value: string }[] }[] = [
  {
    key: 'data_policy',
    title: 'Data Policy',
    controls: (b) => [
      { name: 'Data Retention', value: b.retention_days === 0 ? 'Zero retention' : `${b.retention_days}-day` },
      { name: 'Training Opt-Out', value: b.training_opt_out ? 'On' : 'Off' },
      { name: 'Zero Data Retention', value: titleCase(b.zdr) },
      { name: 'PII Redaction', value: titleCase(b.pii_redaction) },
      { name: 'Content Safety', value: titleCase(b.content_safety) },
    ],
  },
  {
    key: 'developer_workflow',
    title: 'Developer Workflow',
    controls: (b) => [
      { name: 'Model Access', value: titleCase(b.model_access) },
      { name: 'Code Review Level', value: b.code_review },
      { name: 'BAA-Only Vendors', value: b.baa_only ? 'Required' : 'Not required' },
    ],
  },
  {
    key: 'budget',
    title: 'Budget',
    controls: (b) => [
      { name: 'Org Spend Cap', value: b.org_cap_usd ? `$${b.org_cap_usd.toLocaleString()}` : 'Board-approved' },
      { name: 'Per-User Spend Cap', value: b.user_cap_usd ? `$${b.user_cap_usd}/user` : 'Human-required' },
    ],
  },
]

export default function RigorConsole() {
  const qc = useQueryClient()
  const baselines = useQuery({ queryKey: qk.baselines, queryFn: api.baselines })
  const adapters = useQuery({ queryKey: qk.adapters, queryFn: api.adapters })
  const [selected, setSelected] = useState<string>('fintech')
  const [coverage, setCoverage] = useState<CoverageReport | null>(null)
  const [applying, setApplying] = useState(false)

  const baseline = baselines.data?.find((b) => b.id === selected) ?? baselines.data?.[0]

  const planeCoverage = useMemo(() => coverageByPlane(adapters.data ?? []), [adapters.data])

  async function apply() {
    setApplying(true)
    try {
      const cov = await api.applyPolicy(selected)
      setCoverage(cov)
      qc.invalidateQueries({ queryKey: qk.drift })
      qc.invalidateQueries({ queryKey: qk.audit })
    } finally {
      setApplying(false)
    }
  }

  return (
    <div>
      <PageHeader
        title="Rigor Console"
        subtitle="Set an industry-rigor baseline across every control surface in one glance."
        actions={
          <button
            onClick={apply}
            disabled={applying}
            className="rounded-lg px-4 py-2 text-sm font-medium text-bg disabled:opacity-50"
            style={{ background: 'var(--accent)' }}
          >
            {applying ? 'Applying…' : 'Apply Profile'}
          </button>
        }
      />

      {/* profile selector */}
      <div className="mb-5 flex flex-wrap gap-2">
        {baselines.data?.map((b) => (
          <button
            key={b.id}
            onClick={() => setSelected(b.id)}
            className={`rounded-xl border px-4 py-2.5 text-left transition ${selected === b.id ? 'bg-panel2' : 'hover:bg-panel2'}`}
            style={{ borderColor: selected === b.id ? 'var(--accent)' : 'var(--line)' }}
          >
            <div className="flex items-center gap-2">
              <span className="text-sm font-semibold">{b.name}</span>
              <span>{b.rigor}</span>
            </div>
            <div className="mt-0.5 max-w-[220px] text-[11px] text-muted">{b.description}</div>
          </button>
        ))}
      </div>

      {coverage && (
        <div className="mb-5 panel p-4">
          <div className="mb-3 flex items-center justify-between">
            <h3 className="text-sm font-semibold">Coverage report · {titleCase(coverage.baseline)}</h3>
            <span className="text-xs text-muted">{coverage.rows.length} controls reconciled across all vendors</span>
          </div>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-4 lg:grid-cols-7">
            {Object.entries(coverage.summary).map(([outcome, count]) => {
              const m = OUTCOME_META[outcome] ?? { label: outcome, color: 'var(--muted)' }
              return (
                <div key={outcome} className="panel-2 px-3 py-2">
                  <div className="text-xl font-semibold tabular-nums" style={{ color: m.color }}>
                    {count}
                  </div>
                  <div className="text-[11px] text-muted">{m.label}</div>
                </div>
              )
            })}
          </div>
        </div>
      )}

      {/* control sections */}
      {baseline && (
        <>
        <div className="grid gap-4 lg:grid-cols-3">
          {PLANES.map((p) => {
            const cov = planeCoverage[p.key] ?? {}
            const total = Object.values(cov).reduce((a, b) => a + b, 0) || 1
            return (
              <div key={p.key} className="panel p-4">
                <h3 className="mb-3 text-sm font-semibold">{p.title}</h3>
                <div className="flex flex-col gap-3">
                  {p.controls(baseline).map((c) => (
                    <div key={c.name} className="flex items-center justify-between gap-2 border-b border-line pb-2 last:border-0">
                      <span className="text-sm text-muted">{c.name}</span>
                      <span className="text-sm font-medium">{c.value}</span>
                    </div>
                  ))}
                </div>
                <div className="mt-3">
                  <div className="mb-1.5 text-[10px] uppercase tracking-wider text-faint">Coverage across vendors</div>
                  <div className="flex h-2 overflow-hidden rounded-full bg-panel2">
                    {['vendor_native', 'env_managed', 'proxy_enforced', 'monitor_only', 'unverified', 'unsupported'].map((d) => (
                      <span key={d} style={{ width: `${((cov[d] ?? 0) / total) * 100}%`, background: dispColor(d) }} title={`${titleCase(d)}: ${cov[d] ?? 0}`} />
                    ))}
                  </div>
                </div>
              </div>
            )
          })}
        </div>

        <NarrativeBlock baseline={baseline} />
        </>
      )}
    </div>
  )
}

function NarrativeBlock({ baseline }: { baseline: Baseline }) {
  const n = NARRATIVE[baseline.id] ?? deriveNarrative(baseline)
  const surfaces: { key: keyof Narrative['posture']; title: string }[] = [
    { key: 'data_policy', title: 'Data Policy' },
    { key: 'developer_workflow', title: 'Developer Workflow' },
    { key: 'budget', title: 'Budget' },
  ]
  return (
    <div className="mt-4 panel p-5">
      <div className="mb-2 flex items-baseline gap-2">
        <h3 className="text-sm font-semibold">About the {baseline.name} profile</h3>
        <span className="text-xs">{baseline.rigor}</span>
        <span className="text-xs text-faint">preconfigured rigor baseline</span>
      </div>
      <div className="text-sm font-medium" style={{ color: 'var(--accent)' }}>{n.headline}</div>
      <p className="mt-1.5 max-w-3xl text-sm leading-relaxed text-muted">{n.body}</p>
      <div className="mt-4 grid gap-3 sm:grid-cols-3">
        {surfaces.map((s) => (
          <div key={s.key} className="panel-2 p-3">
            <div className="mb-1 text-[10px] uppercase tracking-wider text-faint">{s.title}</div>
            <div className="text-xs leading-relaxed text-muted">{n.posture[s.key]}</div>
          </div>
        ))}
      </div>
    </div>
  )
}

function coverageByPlane(adapters: Adapter[]): Record<string, Record<string, number>> {
  const out: Record<string, Record<string, number>> = {}
  for (const a of adapters) {
    for (const c of a.capabilities) {
      ;(out[c.plane] ??= {})[c.disposition] = ((out[c.plane] ??= {})[c.disposition] ?? 0) + 1
    }
  }
  return out
}

function dispColor(d: string): string {
  return {
    vendor_native: 'var(--native)',
    env_managed: 'var(--env)',
    proxy_enforced: 'var(--proxy)',
    monitor_only: 'var(--monitor)',
    unverified: 'var(--unverified)',
    unsupported: 'var(--unsupported)',
  }[d] ?? 'var(--muted)'
}
