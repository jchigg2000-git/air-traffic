// Branded monogram chips — deliberately NOT counterfeit vendor logos; brand-color
// initials read cleanly and stay honest about not being the real marks.

const BRAND: Record<string, { c: string; mark: string; fg?: string }> = {
  openai: { c: '#10A37F', mark: 'ai' },
  anthropic: { c: '#D97757', mark: 'An' },
  bedrock: { c: '#FF9900', mark: 'aws', fg: '#111' },
  azure_openai: { c: '#0078D4', mark: 'Az' },
  vertex: { c: '#4285F4', mark: 'Ve' },
  github_copilot: { c: '#6E7681', mark: 'GH' },
  m365_copilot: { c: '#D83B01', mark: 'M3' },
  mistral: { c: '#FA520F', mark: 'Mi' },
  databricks: { c: '#FF3621', mark: 'Dx' },
  perplexity: { c: '#20808D', mark: 'Px' },
  cohere: { c: '#39594D', mark: 'Co' },
  together: { c: '#0F6FFF', mark: 'Tg' },
  groq: { c: '#F55036', mark: 'Gq' },
  xai: { c: '#1A1A1A', mark: 'xA', fg: '#fff' },
  amazon_q: { c: '#FF9900', mark: 'Q', fg: '#111' },
  watsonx: { c: '#0F62FE', mark: 'Wx' },
}

export default function VendorGlyph({ id, size = 28 }: { id: string; size?: number }) {
  const b = BRAND[id] ?? { c: 'var(--accent2)', mark: id.slice(0, 2) }
  return (
    <span
      style={{
        width: size,
        height: size,
        background: `color-mix(in srgb, ${b.c} 18%, transparent)`,
        border: `1px solid color-mix(in srgb, ${b.c} 55%, transparent)`,
        color: b.fg ?? b.c,
        fontSize: size * 0.4,
      }}
      className="inline-flex shrink-0 items-center justify-center rounded-md font-mono font-semibold leading-none"
      aria-hidden
    >
      {b.mark}
    </span>
  )
}

export function vendorAccent(id: string): string {
  return (BRAND[id] ?? { c: 'var(--accent2)' }).c
}
