import VendorGlyph from '../../components/VendorGlyph.tsx'

// Bespoke section: the 16-vendor coverage wall. The roster and its tiering live
// inline here (co-located with the markup that renders them) rather than in a
// shared catalog — this is the landing page's own honest snapshot of coverage.
// Glyphs reuse the existing VendorGlyph so the monograms match the Flight Deck.

const TIER1 = [
  { id: 'openai', name: 'OpenAI' },
  { id: 'anthropic', name: 'Anthropic' },
  { id: 'bedrock', name: 'AWS Bedrock' },
  { id: 'azure_openai', name: 'Azure OpenAI' },
  { id: 'vertex', name: 'Google Vertex' },
  { id: 'github_copilot', name: 'GitHub Copilot' },
]

const TIER2 = [
  { id: 'm365_copilot', name: 'M365 Copilot' },
  { id: 'mistral', name: 'Mistral' },
  { id: 'databricks', name: 'Databricks' },
  { id: 'perplexity', name: 'Perplexity' },
  { id: 'cohere', name: 'Cohere' },
  { id: 'together', name: 'Together' },
]

const TIER3 = [
  { id: 'groq', name: 'Groq' },
  { id: 'xai', name: 'xAI' },
  { id: 'amazon_q', name: 'Amazon Q' },
  { id: 'watsonx', name: 'IBM watsonx' },
]

function Tier({
  n,
  label,
  desc,
  vendors,
}: {
  n: number
  label: string
  desc: string
  vendors: { id: string; name: string }[]
}) {
  return (
    <div>
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <h3 className="text-sm font-semibold text-fg">
          <span className="text-accent">Tier {n}</span> · {label}
        </h3>
        <span className="text-xs text-faint">{desc}</span>
      </div>
      <div className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
        {vendors.map((v) => (
          <div key={v.id} className="panel-2 flex items-center gap-2.5 px-3 py-2.5">
            <VendorGlyph id={v.id} size={26} />
            <span className="min-w-0 truncate text-sm font-medium text-fg">{v.name}</span>
          </div>
        ))}
      </div>
    </div>
  )
}

export default function VendorWall() {
  return (
    <section className="mx-auto max-w-5xl px-5 py-16 md:py-20">
      <div className="max-w-2xl">
        <p className="text-xs font-semibold uppercase tracking-[0.2em] text-accent">Coverage</p>
        <h2 className="mt-3 text-3xl font-semibold tracking-tight text-fg sm:text-4xl">Sixteen vendors, three depths of reach.</h2>
        <p className="mt-5 text-base leading-relaxed text-muted">
          Depth is a function of what each vendor's API actually exposes. Tier 1 reaches deep across many endpoints; Tier 2
          covers the core surfaces; Tier 3 manifests the vendor and emits a normalized budget signal.
        </p>
      </div>

      <div className="mt-10 flex flex-col gap-10">
        <Tier n={1} label="Deep, multi-endpoint" desc="admin + policy + usage across many endpoints" vendors={TIER1} />
        <Tier n={2} label="Core surfaces" desc="the primary governance + spend surfaces" vendors={TIER2} />
        <Tier n={3} label="Manifest + emit" desc="declared and reporting a normalized signal" vendors={TIER3} />
      </div>
    </section>
  )
}
