// Validation of a configured upstream endpoint against each vendor's REAL endpoint
// data contract: the official API host + the admin-path prefixes the adapter's
// manifest actually exposes (adapter.capabilities[].endpoint).

import type { Adapter } from './api.ts'

interface Expected {
  hostSuffix: string
  example: string
}

// Real vendor admin-API hosts (suffix-matched so region/workspace subdomains pass).
const EXPECTED: Record<string, Expected> = {
  openai: { hostSuffix: 'api.openai.com', example: 'https://api.openai.com/v1' },
  anthropic: { hostSuffix: 'api.anthropic.com', example: 'https://api.anthropic.com' },
  bedrock: { hostSuffix: '.amazonaws.com', example: 'https://bedrock.us-east-1.amazonaws.com' },
  azure_openai: { hostSuffix: '.azure.com', example: 'https://acme.openai.azure.com' },
  vertex: { hostSuffix: '.googleapis.com', example: 'https://aiplatform.googleapis.com' },
  github_copilot: { hostSuffix: 'api.github.com', example: 'https://api.github.com' },
  m365_copilot: { hostSuffix: 'graph.microsoft.com', example: 'https://graph.microsoft.com' },
  mistral: { hostSuffix: 'api.mistral.ai', example: 'https://api.mistral.ai' },
  databricks: { hostSuffix: '.databricks.com', example: 'https://acme.cloud.databricks.com' },
  perplexity: { hostSuffix: 'api.perplexity.ai', example: 'https://api.perplexity.ai' },
  cohere: { hostSuffix: 'api.cohere.com', example: 'https://api.cohere.com' },
  together: { hostSuffix: 'api.together.xyz', example: 'https://api.together.xyz' },
  groq: { hostSuffix: 'api.groq.com', example: 'https://api.groq.com' },
  xai: { hostSuffix: 'api.x.ai', example: 'https://api.x.ai' },
  amazon_q: { hostSuffix: '.amazonaws.com', example: 'https://q.us-east-1.amazonaws.com' },
  watsonx: { hostSuffix: '.ibm.com', example: 'https://us-south.ml.cloud.ibm.com' },
}

export function expectedFor(id: string): Expected | null {
  return EXPECTED[id] ?? null
}

// Distinct admin-path prefixes drawn from the adapter's real capability endpoints.
export function knownPrefixes(a: Adapter): string[] {
  const set = new Set<string>()
  for (const c of a.capabilities) {
    if (!c.endpoint || !c.endpoint.startsWith('/')) continue
    const seg = c.endpoint.split('/').filter(Boolean)
    if (!seg.length) continue
    let prefix = '/' + seg[0]
    if (seg[1] && !seg[1].startsWith('{')) prefix += '/' + seg[1]
    set.add(prefix)
  }
  return [...set]
}

export interface Validation {
  ok: boolean
  errors: string[]
  warnings: string[]
}

export function validateEndpoint(a: Adapter, raw: string): Validation {
  const errors: string[] = []
  const warnings: string[] = []
  const value = raw.trim()
  if (!value) return { ok: false, errors: ['Endpoint URL is required.'], warnings: [] }

  let url: URL
  try {
    url = new URL(value)
  } catch {
    return { ok: false, errors: ['Not a valid URL (include the https:// scheme).'], warnings: [] }
  }

  if (url.protocol !== 'https:') errors.push('Must use https://.')

  const exp = expectedFor(a.id)
  if (exp && !url.host.endsWith(exp.hostSuffix)) {
    errors.push(`Host must match ${a.vendor} — expected …${exp.hostSuffix}.`)
  }

  const prefixes = knownPrefixes(a)
  const path = url.pathname.replace(/\/+$/, '')
  if (path && prefixes.length && !prefixes.some((p) => path.startsWith(p))) {
    warnings.push(`Path doesn't match a known admin path (${prefixes.slice(0, 3).join(', ')}).`)
  }

  return { ok: errors.length === 0, errors, warnings }
}
