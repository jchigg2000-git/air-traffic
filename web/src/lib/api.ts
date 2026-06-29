// Typed client for the Air-Traffic control-plane API (Phase 1 backend).

export type Mode = 'disabled' | 'synthetic' | 'proxy'

export interface Capability {
  key: string
  name: string
  plane: string
  disposition: string
  enforcement?: string
  mechanism: string
  endpoint?: string
  note?: string
  retryable: boolean
}

export interface Status {
  state: string
  message: string
  checked_at: string
}

export interface Adapter {
  id: string
  vendor: string
  display_name: string
  family: string
  api_version: string
  tier: number
  mode: Mode
  enabled: boolean
  emit: boolean
  base_path: string
  upstream_url: string
  scenario: string
  baa_signed: boolean
  capabilities: Capability[]
  status: Status
  updated_at: string
}

export interface Baseline {
  id: string
  name: string
  rigor: string
  description: string
  model_access: string
  training_opt_out: boolean
  zdr: string
  pii_redaction: string
  content_safety: string
  retention_days: number
  org_cap_usd: number
  user_cap_usd: number
  code_review: string
  baa_only: boolean
}

export interface SignalEntry {
  name: string
  value: number | boolean
  unit: string
  status: 'green' | 'amber' | 'red'
  severity: string
}

export interface ObsEntry {
  kind: string
  signal: SignalEntry
  dimensions: Record<string, unknown> & { plane?: string; vendor?: string; control_surface?: string; team?: string; model?: string }
  provenance: { fixture?: string; source_url?: string }
}

export interface BatchBody {
  contract: string
  batch_id: string
  connector: { type: string; instance: string; api_version: string }
  collected_at: string
  window: { from: string; to: string }
  complete: boolean
  observations: ObsEntry[]
  errors: unknown[]
}

export interface ObservationRecord {
  id: number
  received_at: string
  contract: string
  connector_type: string
  connector_instance: string
  complete: boolean
  observation_count: number
  error_count: number
  body: BatchBody
}

export interface AuditEvent {
  id: string
  timestamp: string
  actor: string
  action: string
  resource: string
  plane: string
  vendor: string
  control_surface: string
  before?: Record<string, unknown>
  after?: Record<string, unknown>
  request_id: string
}

export interface DriftRecord {
  id: number
  vendor: string
  capability: string
  plane: string
  control_surface: string
  declared: string
  actual: string
  severity: string
  message: string
  detected_at: string
}

export interface CoverageRow {
  vendor: string
  capability: string
  plane: string
  disposition: string
  enforcement?: string
  outcome: string
  signal: { disposition: string; code: string; message: string }
}

export interface CoverageReport {
  baseline: string
  applied_at: string
  rows: CoverageRow[]
  summary: Record<string, number>
}

export interface CallRecord {
  id: number
  adapter_id: string
  method: string
  path: string
  query: Record<string, string[]>
  headers: Record<string, string[]>
  scenario: string
  status_code: number
  duration_ms: number
  recorded_at: string
}

export interface ManagedArtifact {
  platform: string
  identifier: string
  artifact: Record<string, unknown>
  enforcement: string
  distribution_url: string
  rendered_at: string
}

export interface EnvState {
  platform: string
  identifier: string
  actual_settings: Record<string, unknown>
  source: string
  drift_detected: boolean
  drift_message?: string
}

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message)
  }
}

async function getJSON<T>(path: string): Promise<T> {
  const res = await fetch(path, { headers: { accept: 'application/json' } })
  if (!res.ok) throw new ApiError(res.status, `GET ${path} → ${res.status}`)
  return (await res.json()) as T
}

export async function send<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: { accept: 'application/json', 'content-type': 'application/json' },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) throw new ApiError(res.status, `${method} ${path} → ${res.status}`)
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

export const api = {
  health: () => getJSON<{ ok: boolean; adapter_count: number; ts: string }>('/api/health'),
  adapters: () => getJSON<{ adapters: Adapter[] }>('/api/adapters').then((d) => d.adapters),
  adapter: (id: string) => getJSON<{ adapter: Adapter }>(`/api/adapters/${id}`).then((d) => d.adapter),
  manifest: (id: string) => getJSON<{ capabilities: Capability[]; env_platforms: unknown[] }>(`/api/adapters/${id}/manifest`),
  calls: (id: string) => getJSON<{ calls: CallRecord[] }>(`/api/adapters/${id}/calls`).then((d) => d.calls),
  baselines: () => getJSON<{ baselines: Baseline[] }>('/api/baselines').then((d) => d.baselines),
  policy: () => getJSON<{ policy: unknown }>('/api/policies').then((d) => d.policy),
  observations: (limit = 400) => getJSON<{ observations: ObservationRecord[] }>(`/api/observations?limit=${limit}`).then((d) => d.observations),
  audit: () => getJSON<{ audit: AuditEvent[] }>('/api/audit').then((d) => d.audit),
  siem: () => getJSON<{ records: Record<string, unknown>[] }>('/api/audit?format=siem').then((d) => d.records),
  drift: () => getJSON<{ drift: DriftRecord[] }>('/api/drift').then((d) => d.drift),
  envconfig: () => getJSON<{ artifacts: ManagedArtifact[]; states: EnvState[] }>('/api/envconfig'),

  patchAdapter: (id: string, patch: Partial<Pick<Adapter, 'mode' | 'scenario' | 'enabled' | 'emit' | 'upstream_url'>>) =>
    send<{ adapter: Adapter }>('PATCH', `/api/adapters/${id}`, patch).then((d) => d.adapter),
  testAdapter: (id: string) => send<{ adapter: Adapter; status: Status }>('POST', `/api/adapters/${id}/test`),
  applyPolicy: (baseline: string, overrides: Record<string, unknown> = {}) =>
    send<{ coverage: CoverageReport }>('PUT', '/api/policies', { baseline, ...overrides }).then((d) => d.coverage),
}

export const qk = {
  health: ['health'] as const,
  adapters: ['adapters'] as const,
  adapter: (id: string) => ['adapter', id] as const,
  calls: (id: string) => ['calls', id] as const,
  baselines: ['baselines'] as const,
  observations: ['observations'] as const,
  audit: ['audit'] as const,
  siem: ['siem'] as const,
  drift: ['drift'] as const,
  envconfig: ['envconfig'] as const,
}
