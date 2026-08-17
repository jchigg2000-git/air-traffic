import type { ReactElement } from 'react'
import { render } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import type { Baseline } from '../lib/api.ts'

// Console pages need a router and a query client. Retries off so a rejected
// query surfaces its error state immediately instead of after backoff.
export function renderPage(ui: ReactElement) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>{ui}</MemoryRouter>
    </QueryClientProvider>,
  )
}

/** The four shipped baselines, carrying the server-derived gateway actions. */
export const BASELINES: Baseline[] = [
  base('general_saas', 'General SaaS', '🔒', 'off', 'off', 'detect', 'detect', false),
  base('fintech', 'Fintech', '🔒🔒', 'on', 'where_native', 'mask', 'mask', false),
  // The one that blocks everything until attested — the PIVOT-1 case.
  base('healthcare', 'Healthcare', '🔒🔒🔒', 'on+phi', 'enforced', 'block', 'mask', true),
  // Renders as joint-strictest and enforces less. Deliberately in the fixture.
  base('gov_infra', 'Gov / Infra', '🔒🔒🔒', 'on', 'enforced', 'mask', 'mask', false),
]

function base(
  id: string,
  name: string,
  rigor: string,
  pii: string,
  zdr: string,
  action: 'detect' | 'mask' | 'block',
  attestedAction: 'detect' | 'mask' | 'block',
  gated: boolean,
): Baseline {
  return {
    id,
    name,
    rigor,
    description: `${name} profile`,
    model_access: 'open',
    training_opt_out: true,
    zdr,
    pii_redaction: pii,
    content_safety: 'standard',
    retention_days: 30,
    org_cap_usd: 10000,
    user_cap_usd: 200,
    code_review: 'agentic-reviewer',
    baa_only: false,
    gateway_action: action,
    gateway_action_attested: attestedAction,
    requires_zdr_attestation: gated,
  }
}

export const ADAPTERS = [
  adapter('openai', 'OpenAI'),
  adapter('anthropic', 'Anthropic'),
]

function adapter(id: string, vendor: string) {
  return {
    id,
    vendor,
    display_name: vendor,
    family: 'api-platform',
    api_version: 'v1',
    tier: 1,
    mode: 'synthetic' as const,
    enabled: true,
    emit: true,
    base_path: `/synthetic/${id}`,
    upstream_url: '',
    scenario: 'healthy',
    baa_signed: true,
    capabilities: [
      {
        key: 'pii_redaction',
        name: 'PII Redaction',
        plane: 'data_policy',
        disposition: 'proxy_enforced',
        mechanism: 'gateway',
        retryable: false,
      },
    ],
    status: { state: 'ok', message: 'fine', checked_at: '2026-08-16T00:00:00Z' },
    updated_at: '2026-08-16T00:00:00Z',
  }
}
