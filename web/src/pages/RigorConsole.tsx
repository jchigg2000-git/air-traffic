import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, qk, type Adapter, type Baseline, type CoverageReport } from '../lib/api.ts'
import PageHeader from '../components/PageHeader.tsx'
import Modal from '../components/Modal.tsx'
import { dispMeta } from '../lib/dispositions.ts'
import ApiStateBanner from '../components/ApiStateBanner.tsx'
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

// What each derived gateway action does to live traffic, in the words the rest
// of the app already uses. These three strings are the only thing Apply
// actually changes on the request path, and until 2026-08-16 none of them
// appeared anywhere on this page.
export const ACTION_META: Record<string, { label: string; color: string; effect: string }> = {
  detect: {
    label: 'detect',
    color: 'var(--monitor)',
    effect: 'Traffic flows unchanged. Detections are logged only — this is monitoring, not enforcement.',
  },
  mask: {
    label: 'mask',
    color: 'var(--accent)',
    effect: 'Detected values are rewritten before the request reaches the vendor. Callers still get responses.',
  },
  block: {
    label: 'block',
    color: 'var(--red)',
    effect:
      'Every request carrying a detected value is REFUSED with a 400. Callers see errors, not answers.',
  },
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
  // Nothing is armed on arrival. The primary CTA used to default to 'fintech',
  // so the accent-coloured button was always one click from applying a posture
  // nobody had chosen.
  const [selected, setSelected] = useState<string | null>(null)
  const [coverage, setCoverage] = useState<CoverageReport | null>(null)
  const [applying, setApplying] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const [attest, setAttest] = useState(false)
  const [applyError, setApplyError] = useState<unknown>(null)

  const list = baselines.data?.baselines
  const zdrAttested = baselines.data?.zdr_attested ?? false
  const baseline = selected ? list?.find((b) => b.id === selected) : undefined
  // The page still renders a profile's control surfaces before one is picked;
  // that is a description, not an action, so the first is fine as a preview.
  const shown = baseline ?? list?.[0]

  const planeCoverage = useMemo(() => coverageByPlane(adapters.data ?? []), [adapters.data])

  // How many distinct apps are currently reaching the gateway — the blast
  // radius of a `block`, stated as a count rather than left to the imagination.
  const traffic = useQuery({ queryKey: qk.gatewayRequests, queryFn: () => api.gatewayRequests(200) })
  const affectedApps = useMemo(() => {
    const ids = new Set((traffic.data ?? []).map((r) => r.app_id || 'env'))
    return ids.size
  }, [traffic.data])

  // Both actions come from the server (model.GatewayAction), so this is a
  // lookup rather than a second implementation of the enforcement rule.
  const actionOf = (b: Baseline, attested: boolean) =>
    attested ? b.gateway_action_attested : b.gateway_action

  // What Apply would produce given what the operator has ticked right now.
  const pendingAction = baseline ? actionOf(baseline, attest) : null

  async function confirmApply() {
    if (!baseline) return
    setApplying(true)
    setApplyError(null)
    try {
      // The attestation is the whole reason this dialog exists: it is the sole
      // input that turns an unattested `on+phi` baseline from block into mask,
      // and before 2026-08-16 no control in this SPA could set it.
      const overrides = attest ? { vendors: { anthropic: { zdr_attested: true } } } : {}
      const cov = await api.applyPolicy(baseline.id, overrides)
      setCoverage(cov)
      setConfirming(false)
      qc.invalidateQueries({ queryKey: qk.baselines })
      qc.invalidateQueries({ queryKey: qk.drift })
      qc.invalidateQueries({ queryKey: qk.audit })
    } catch (err) {
      setApplyError(err)
    } finally {
      setApplying(false)
    }
  }

  function openConfirm() {
    if (!baseline) return
    // Seed from the attestation currently in force, not from false. An
    // operator re-applying a baseline that is already attested would otherwise
    // silently revoke it — the dialog would preview `mask` while sending an
    // empty override map, and the gateway would drop to `block`. Which is the
    // original defect wearing a confirmation dialog.
    setAttest(zdrAttested)
    setApplyError(null)
    setConfirming(true)
  }

  return (
    <div>
      <PageHeader
        title="Rigor Console"
        subtitle="Set an industry-rigor baseline across every control surface in one glance."
        actions={
          <button
            onClick={openConfirm}
            disabled={!baseline || applying}
            title={baseline ? `Review and apply ${baseline.name}` : 'Select a profile first'}
            className="rounded-lg px-4 py-2 text-sm font-medium text-bg disabled:opacity-50"
            style={{ background: 'var(--accent)' }}
          >
            {applying ? 'Applying…' : 'Review & Apply…'}
          </button>
        }
      />

      <ApiStateBanner error={baselines.error || adapters.error} className="mb-4" />

      {/* profile selector */}
      {(baselines.isLoading || adapters.isLoading) && (
        <p role="status" className="mb-5 text-xs italic text-muted">
          loading…
        </p>
      )}
      <div role="radiogroup" aria-label="Rigor profile" className="mb-5 flex flex-wrap gap-2">
        {list?.map((b) => {
          // The chip shows what applying THIS profile would do given the
          // attestation currently in force — the same basis the dialog opens
          // with, so the two never disagree.
          const action = actionOf(b, zdrAttested)
          const meta = ACTION_META[action] ?? { label: action, color: 'var(--muted)' }
          return (
            <button
              key={b.id}
              role="radio"
              aria-checked={selected === b.id}
              onClick={() => setSelected(b.id)}
              className={`rounded-xl border px-4 py-2.5 text-left transition ${selected === b.id ? 'bg-panel2' : 'hover:bg-panel2'}`}
              style={{ borderColor: selected === b.id ? 'var(--accent)' : 'var(--line)' }}
            >
              <div className="flex items-center gap-2">
                <span className="text-sm font-semibold">{b.name}</span>
                <span>{b.rigor}</span>
              </div>
              <div className="mt-0.5 max-w-[220px] text-[11px] text-muted">{b.description}</div>
              {/* The lock ramp describes intent; this chip describes what the
                  gateway does. They disagree — gov_infra reads as joint-strictest
                  and masks, while healthcare unattested blocks everything — and
                  the chip is the half that is load-bearing. */}
              <div className="mt-2 flex items-center gap-1.5">
                <span className="text-[10px] uppercase tracking-wider text-faint">gateway</span>
                <span
                  className="rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide"
                  style={{ color: meta.color, border: `1px solid ${meta.color}` }}
                >
                  {meta.label}
                </span>
                {b.requires_zdr_attestation && action === 'block' && (
                  <span className="text-[10px] text-muted">until ZDR attested</span>
                )}
                {b.requires_zdr_attestation && action !== 'block' && (
                  <span className="text-[10px] text-muted">on your ZDR attestation</span>
                )}
              </div>
            </button>
          )
        })}
      </div>

      {baseline && (
        <ApplyConfirmation
          open={confirming}
          baseline={baseline}
          action={pendingAction}
          attest={attest}
          onAttestChange={setAttest}
          applying={applying}
          affectedApps={affectedApps}
          error={applyError}
          onCancel={() => setConfirming(false)}
          onConfirm={confirmApply}
        />
      )}

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
      {shown && (
        <>
        <div className="grid gap-4 lg:grid-cols-3">
          {PLANES.map((p) => {
            const cov = planeCoverage[p.key] ?? {}
            const total = Object.values(cov).reduce((a, b) => a + b, 0) || 1
            return (
              <div key={p.key} className="panel p-4">
                <h3 className="mb-3 text-sm font-semibold">{p.title}</h3>
                <div className="flex flex-col gap-3">
                  {p.controls(shown).map((c) => (
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
                      <span key={d} style={{ width: `${((cov[d] ?? 0) / total) * 100}%`, background: dispMeta(d).color }} title={`${titleCase(d)}: ${cov[d] ?? 0}`} />
                    ))}
                  </div>
                </div>
              </div>
            )
          })}
        </div>

        <NarrativeBlock baseline={shown} />
        </>
      )}
    </div>
  )
}

// The pre-commit gate. Apply used to be a bare click that changed enforcement
// for every caller of the gateway with no dialog anywhere in the codebase; the
// words detect/mask/block — the only thing it actually changes — appeared
// nowhere on the page. This says the consequence, in the operator's words,
// before anything commits.
function ApplyConfirmation({
  open,
  baseline,
  action,
  attest,
  onAttestChange,
  applying,
  affectedApps,
  error,
  onCancel,
  onConfirm,
}: {
  open: boolean
  baseline: Baseline
  action: string | null
  attest: boolean
  onAttestChange: (v: boolean) => void
  applying: boolean
  affectedApps: number
  error: unknown
  onCancel: () => void
  onConfirm: () => void
}) {
  const meta = (action && ACTION_META[action]) || null
  const blocking = action === 'block'
  return (
    <Modal
      open={open}
      onClose={onCancel}
      title={`Apply ${baseline.name}?`}
      footer={
        <>
          <button
            onClick={onCancel}
            className="rounded-lg border border-line px-4 py-2 text-sm text-muted transition hover:text-fg"
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            disabled={applying}
            className="rounded-lg px-4 py-2 text-sm font-medium text-bg disabled:opacity-50"
            style={{ background: blocking ? 'var(--red)' : 'var(--accent)' }}
          >
            {applying ? 'Applying…' : blocking ? 'Apply and block traffic' : `Apply and ${action} traffic`}
          </button>
        </>
      }
    >
      <div className="flex flex-col gap-3 text-sm">
        <div className="panel-2 p-3">
          <div className="mb-1 text-[10px] uppercase tracking-wider text-faint">Resulting gateway action</div>
          <div className="flex items-center gap-2">
            <span
              className="rounded px-2 py-0.5 text-xs font-semibold uppercase tracking-wide"
              style={{ color: meta?.color ?? 'var(--muted)', border: `1px solid ${meta?.color ?? 'var(--line)'}` }}
            >
              {action ?? 'unknown'}
            </span>
            <span className="text-xs text-muted">
              {affectedApps > 0
                ? `applies to all ${affectedApps} gateway caller${affectedApps === 1 ? '' : 's'} seen recently`
                : 'applies to every gateway caller'}
              , within one policy-pull interval
            </span>
          </div>
          <p className="mt-2 text-xs leading-relaxed text-muted">{meta?.effect}</p>
        </div>

        {baseline.requires_zdr_attestation && (
          <label className="flex cursor-pointer items-start gap-2.5 rounded-lg border border-line p-3">
            <input
              type="checkbox"
              checked={attest}
              onChange={(e) => onAttestChange(e.target.checked)}
              className="mt-0.5"
            />
            <span className="text-xs leading-relaxed text-muted">
              <span className="font-semibold text-fg">I attest that ZDR coverage is in place</span> for the
              vendor contracts in scope. This baseline enforces PHI-aware redaction and is a{' '}
              <span className="font-semibold">total block until that is attested</span> — the software cannot
              verify a contract, so this is your assertion, recorded with the policy.
            </span>
          </label>
        )}

        {blocking && (
          <p className="rounded-lg border p-3 text-xs leading-relaxed" style={{ borderColor: 'var(--red)' }}>
            Applying now will refuse every gateway request that carries a detected value. Callers receive a
            400, not an answer. This has previously presented as an application returning HTTP 200 while
            100% of its model calls were dropped.
          </p>
        )}

        <ApiStateBanner error={error} />
      </div>
    </Modal>
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

