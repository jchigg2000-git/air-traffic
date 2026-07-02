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
  endpoint_config?: Record<string, string>
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

// Cost drill-down CONFIG (from /api/cost/facets): which dimensions each vendor's real
// billing/usage API can attribute cost by, and which it cannot. The VALUES live in the
// observation stream (kind "cost_breakdown"); this is the labelling + honest-gap layer.
export interface CostFacetMeta {
  dimension: string
  label: string
  real_param?: string
  endpoint?: string
  response_field?: string
  reason?: string
}

export interface VendorCostFacets {
  vendor: string
  supported: CostFacetMeta[]
  unsupported: CostFacetMeta[]
}

// ---- Gateway harness (Phase 3: inference gateway + flywheel) ----

export interface GatewayStatusEntry {
  gateway_id: string
  base_url: string
  action: string
  vendors: Record<string, string[]>
  last_seen: string
  fresh: boolean
}

export interface GatewayStatus {
  gateways: GatewayStatusEntry[]
  pattern_pack_version: number
}

export interface TruthSpan {
  start: number
  end: number
  type: string
  value: string
}

export interface GatewayRedaction {
  path: string
  type: string
  start: number
  end: number
  detector: string
  confidence: number
}

export interface HarnessRunConfig {
  count: number
  concurrency: number
  seed?: number
  include_traps: boolean
  include_presidio_only: boolean
  include_straddle: boolean
  replay_percent: number
}

export interface TypeScore {
  tp: number
  fn: number
  fp: number
  behavioral_misses: number
}

export interface RunScore {
  precision: number
  recall_reported: number
  recall_behavioral: number
  trap_fps: number
  response_leaks: number
  by_type: Record<string, TypeScore>
  by_engine: Record<string, number>
  joined_reports: number
  orphan_requests: number
}

export interface HarnessRun {
  id: string
  config: HarnessRunConfig
  status: 'running' | 'done' | 'failed'
  total: number
  completed: number
  masked: number
  blocked: number
  detect_only: number
  passed: number
  errors: number
  pack_version: number
  detector_chain: string
  score?: RunScore
  promoted_count: number
  error?: string
  started_at: string
  finished_at?: string
}

export interface HarnessResult {
  run_id: string
  request_id: string
  template: string
  content: string
  truth: TruthSpan[]
  traps?: TruthSpan[]
  straddle?: boolean
  action: string
  http_status: number
  redactions?: GatewayRedaction[]
  upstream_text?: string
  missed_types?: string[]
  response_leaks?: number
  latency_ms: number
  error?: string
}

export interface RatchetPoint {
  run_id: string
  at: string
  corpus_version: string
  pack_version: number
  detector_chain: string
  precision: number
  recall_reported: number
  recall_behavioral: number
  request_count: number
  seed: number
}

export interface CorpusEntry {
  id: string
  text: string
  truth: TruthSpan[]
  source_run: string
  miss_types: string[]
  added_at: string
}

export interface PatternProposal {
  id: string
  type: string
  kind?: 'regex' | 'deny_list' | 'threshold'
  regex?: string
  deny_list?: string[]
  threshold?: number
  context?: string[]
  confidence?: number
  rationale: string
  sample_misses: number
  status: 'proposed' | 'approved' | 'rejected' | 'manual' | 'superseded'
  source_run: string
  created_at: string
}

export interface SampleResult {
  request_id: string
  action: string
  http_status: number
  redactions?: GatewayRedaction[]
  report_joined: boolean
  upstream_text?: string
  response_text?: string
  latency_ms: number
  pack_version: number
  detector_chain: string
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
  costFacets: () => getJSON<{ facets: Record<string, VendorCostFacets> }>('/api/cost/facets').then((d) => d.facets),
  envconfig: () => getJSON<{ artifacts: ManagedArtifact[]; states: EnvState[] }>('/api/envconfig'),

  patchAdapter: (id: string, patch: Partial<Pick<Adapter, 'mode' | 'scenario' | 'enabled' | 'emit' | 'upstream_url' | 'endpoint_config'>>) =>
    send<{ adapter: Adapter }>('PATCH', `/api/adapters/${id}`, patch).then((d) => d.adapter),
  testAdapter: (id: string) => send<{ adapter: Adapter; status: Status }>('POST', `/api/adapters/${id}/test`),
  applyPolicy: (baseline: string, overrides: Record<string, unknown> = {}) =>
    send<{ coverage: CoverageReport }>('PUT', '/api/policies', { baseline, ...overrides }).then((d) => d.coverage),

  gatewayStatus: () => getJSON<GatewayStatus>('/api/gateway/status'),
  harnessRuns: () => getJSON<{ runs: HarnessRun[] }>('/api/harness/runs').then((d) => d.runs),
  harnessRun: (id: string) => getJSON<{ run: HarnessRun }>(`/api/harness/runs/${id}`).then((d) => d.run),
  harnessResults: (id: string, limit = 500) =>
    getJSON<{ results: HarnessResult[] }>(`/api/harness/runs/${id}/results?limit=${limit}`).then((d) => d.results),
  harnessRatchet: () => getJSON<{ ratchet: RatchetPoint[] }>('/api/harness/ratchet').then((d) => d.ratchet),
  harnessCorpus: () => getJSON<{ corpus: CorpusEntry[] }>('/api/harness/corpus').then((d) => d.corpus),
  harnessProposals: () => getJSON<{ proposals: PatternProposal[] }>('/api/harness/proposals').then((d) => d.proposals),
  startHarnessRun: (cfg: HarnessRunConfig) => send<{ run: HarnessRun }>('POST', '/api/harness/runs', cfg).then((d) => d.run),
  runSample: (content: string) => send<{ sample: SampleResult }>('POST', '/api/harness/sample', { content }).then((d) => d.sample),
  approveProposal: (id: string) =>
    send<{ proposals: PatternProposal[] }>('POST', `/api/harness/proposals/${id}/approve`).then((d) => d.proposals),
  rejectProposal: (id: string) =>
    send<{ proposals: PatternProposal[] }>('POST', `/api/harness/proposals/${id}/reject`).then((d) => d.proposals),
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
  costFacets: ['costFacets'] as const,
  envconfig: ['envconfig'] as const,
  gatewayStatus: ['gatewayStatus'] as const,
  harnessRuns: ['harnessRuns'] as const,
  harnessResults: (id: string) => ['harnessResults', id] as const,
  harnessRatchet: ['harnessRatchet'] as const,
  harnessCorpus: ['harnessCorpus'] as const,
  harnessProposals: ['harnessProposals'] as const,
}
