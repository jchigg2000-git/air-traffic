// Branded monogram chips — deliberately NOT counterfeit vendor logos; the chip carries
// the brand colour and the initials stay honest about not being the real marks.
//
// The initials are NOT drawn in the raw brand colour. Measured over the hero gradient,
// the worst backdrop, only two of the sixteen clear 4.5:1 at 11px against their own 18%
// tint in either theme (over the flatter panel backdrop dark gets a third, anthropic, at
// 4.50:1 — which is why the numbers below are quoted against the hero). #FF9900 is 1.9:1 in light,
// #1A1A1A is 1.0:1 in dark — so the text is mixed toward the theme foreground, which
// darkens it in light and lightens it in dark off one number. The ceiling is 47% (worst
// entry 4.53:1); 48% drops it to 4.41:1 and fails. The mix is 45% — two points of
// deliberate headroom, worst case 4.76:1 (bedrock and amazon_q, light, over the hero
// gradient). Per-vendor `fg` overrides were the previous attempt: each one fixed one
// theme and broke the other.

const BRAND: Record<string, { c: string; mark: string }> = {
  openai: { c: '#10A37F', mark: 'ai' },
  anthropic: { c: '#D97757', mark: 'An' },
  bedrock: { c: '#FF9900', mark: 'aws' },
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
  xai: { c: '#1A1A1A', mark: 'xA' },
  amazon_q: { c: '#FF9900', mark: 'Q' },
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
        color: `color-mix(in srgb, ${b.c} 45%, var(--fg))`,
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
