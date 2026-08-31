// Per-vendor proxy config schemas. Only the six Tier-1 vendors are modeled today
// (a deliberate scope cut); the other ten fall back to URL-only and are tracked in
// docs/plans/TODO-vendor-auth.md. Secrets are entered BY REFERENCE only — raw
// tokens are rejected (mirrors the credentials store's no-plaintext posture).

export type FieldKind = 'url' | 'text' | 'secret_ref' | 'select'

export interface ConfigField {
  key: string
  label: string
  kind: FieldKind
  required?: boolean
  placeholder?: string
  options?: string[]
  help?: string
  default?: string
  requiredWhen?: { key: string; value: string }
}

export interface AuthSchema {
  authType: string
  note?: string
  fields: ConfigField[]
}

const REF_HELP = 'Reference only — e.g. vault://kv/… or env:NAME. Raw secrets are rejected.'

export const AUTH_SCHEMAS: Record<string, AuthSchema> = {
  openai: {
    authType: 'Bearer admin API key',
    fields: [
      { key: 'upstream_url', label: 'Base URL', kind: 'url', required: true, default: 'https://api.openai.com/v1' },
      { key: 'admin_key_ref', label: 'Admin API key', kind: 'secret_ref', required: true, help: REF_HELP },
      { key: 'org_id', label: 'Organization ID', kind: 'text', placeholder: 'org-…' },
      { key: 'project_id', label: 'Project ID', kind: 'text', placeholder: 'proj_…' },
    ],
  },
  anthropic: {
    authType: 'Bearer admin API key + version header',
    fields: [
      { key: 'upstream_url', label: 'Base URL', kind: 'url', required: true, default: 'https://api.anthropic.com' },
      { key: 'admin_key_ref', label: 'Admin API key', kind: 'secret_ref', required: true, help: REF_HELP },
      { key: 'anthropic_version', label: 'anthropic-version header', kind: 'text', required: true, default: '2023-06-01' },
    ],
  },
  bedrock: {
    authType: 'AWS SigV4',
    note: 'Bedrock has no bearer token — requests are SigV4-signed with IAM credentials.',
    fields: [
      { key: 'upstream_url', label: 'Base URL', kind: 'url', required: true, default: 'https://bedrock.us-east-1.amazonaws.com' },
      { key: 'region', label: 'AWS Region', kind: 'select', required: true, default: 'us-east-1', options: ['us-east-1', 'us-west-2', 'eu-central-1', 'eu-west-1', 'ap-southeast-2'] },
      { key: 'access_key_id_ref', label: 'Access key ID', kind: 'secret_ref', required: true, help: REF_HELP },
      { key: 'secret_access_key_ref', label: 'Secret access key', kind: 'secret_ref', required: true, help: REF_HELP },
      { key: 'session_token_ref', label: 'Session token (optional)', kind: 'secret_ref', help: REF_HELP },
      { key: 'role_arn', label: 'Assume-role ARN (optional)', kind: 'text', placeholder: 'arn:aws:iam::123456789012:role/…' },
    ],
  },
  azure_openai: {
    authType: 'API key or Entra (AAD)',
    fields: [
      { key: 'upstream_url', label: 'Resource URL', kind: 'url', required: true, default: 'https://acme.openai.azure.com' },
      { key: 'api_version', label: 'api-version', kind: 'text', required: true, default: '2024-10-01' },
      { key: 'auth_mode', label: 'Auth mode', kind: 'select', required: true, default: 'api_key', options: ['api_key', 'entra'] },
      { key: 'api_key_ref', label: 'API key', kind: 'secret_ref', requiredWhen: { key: 'auth_mode', value: 'api_key' }, help: REF_HELP },
      { key: 'tenant_id', label: 'Entra tenant ID', kind: 'text', requiredWhen: { key: 'auth_mode', value: 'entra' } },
      { key: 'client_id', label: 'Entra client ID', kind: 'text', requiredWhen: { key: 'auth_mode', value: 'entra' } },
      { key: 'client_secret_ref', label: 'Entra client secret', kind: 'secret_ref', requiredWhen: { key: 'auth_mode', value: 'entra' }, help: REF_HELP },
    ],
  },
  vertex: {
    authType: 'OAuth2 (service account)',
    note: 'Vertex mints short-lived OAuth tokens from a service account — there is no static key.',
    fields: [
      { key: 'upstream_url', label: 'Base URL', kind: 'url', required: true, default: 'https://aiplatform.googleapis.com' },
      { key: 'project', label: 'GCP Project ID', kind: 'text', required: true, placeholder: 'acme-prod' },
      { key: 'location', label: 'Location', kind: 'text', required: true, default: 'us-central1' },
      { key: 'service_account_ref', label: 'Service account key', kind: 'secret_ref', required: true, help: REF_HELP },
    ],
  },
  github_copilot: {
    authType: 'Bearer token (PAT / GitHub App)',
    fields: [
      { key: 'upstream_url', label: 'Base URL', kind: 'url', required: true, default: 'https://api.github.com' },
      { key: 'enterprise', label: 'Enterprise slug', kind: 'text', required: true, placeholder: 'acme' },
      { key: 'token_ref', label: 'PAT / App token', kind: 'secret_ref', required: true, help: REF_HELP },
      { key: 'api_version', label: 'X-GitHub-Api-Version', kind: 'text', default: '2026-03-10' },
    ],
  },
}

export function schemaFor(id: string): AuthSchema | null {
  return AUTH_SCHEMAS[id] ?? null
}

const REF_SCHEME = /^(env|vault|secret|aws|gcp|azurekv|ref|sm):/i
const RAW_SECRET = /^(sk-|ghp_|gho_|github_pat_|AKIA|xox[bpa]-|eyJ|ya29\.)/

export function looksLikeReference(v: string): boolean {
  return REF_SCHEME.test(v) || v.includes('://')
}
export function looksLikeRawSecret(v: string): boolean {
  if (RAW_SECRET.test(v)) return true
  // long opaque blob that isn't a reference reads as a pasted raw secret
  return v.length > 24 && !looksLikeReference(v) && !v.includes(' ')
}

function isRequired(f: ConfigField, values: Record<string, string>): boolean {
  if (f.required) return true
  if (f.requiredWhen) return values[f.requiredWhen.key] === f.requiredWhen.value
  return false
}

/** Returns an error string for a non-url field, or null if valid. */
export function validateField(f: ConfigField, value: string, values: Record<string, string>): string | null {
  const v = (value ?? '').trim()
  const req = isRequired(f, values)
  if (!v) return req ? `${f.label} is required.` : null
  if (f.kind === 'secret_ref') {
    if (looksLikeRawSecret(v)) return `Use a secret reference, not a raw value.`
    if (!looksLikeReference(v)) return `Must be a reference (e.g. vault://… or env:NAME).`
  }
  if (f.kind === 'select' && f.options && !f.options.includes(v)) return `Pick one of: ${f.options.join(', ')}.`
  return null
}
