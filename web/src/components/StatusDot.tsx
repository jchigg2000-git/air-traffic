import type { Rag } from '../lib/format.ts'

/** 'off' is not a severity — it means the vendor is not emitting, so it has no health to report. */
export type DotState = Rag | 'off'

const COLOR: Record<Rag, string> = { green: 'var(--green)', amber: 'var(--amber)', red: 'var(--red)' }

/** One vocabulary for RAG, so a dot and the header pill that summarises it never disagree. */
export const RAG_MEANING: Record<Rag, string> = { green: 'Nominal', amber: 'Attention', red: 'Action required' }

// Shape carries the severity as well as hue. Amber and red both pulse and differ only in
// hue, which is no difference at all to a red-green colour-blind reader or a greyscale
// print: circle / diamond / square makes them three marks. The scaling is in `transform`,
// not in width, so every dot keeps the same layout box and columns of them stay aligned.
const SHAPE: Record<Rag, { radius: number | string; transform?: string }> = {
  green: { radius: '50%' },
  amber: { radius: 1, transform: 'rotate(45deg) scale(0.72)' },
  red: { radius: 2, transform: 'scale(0.86)' },
}

/**
 * `standalone` — this dot is the only thing saying it, so it carries the meaning as an
 * accessible name. Next to text that already says it, leave it off; otherwise the row is
 * read twice.
 */
export default function StatusDot({
  rag,
  pulse = false,
  size = 9,
  standalone = false,
}: {
  rag: DotState
  pulse?: boolean
  size?: number
  standalone?: boolean
}) {
  const a11y = standalone
    ? ({ role: 'img', 'aria-label': rag === 'off' ? 'Not emitting' : RAG_MEANING[rag] } as const)
    : ({ 'aria-hidden': true } as const)

  if (rag === 'off') {
    return (
      <span
        {...a11y}
        className="inline-block rounded-full"
        style={{ width: size, height: size, border: '1px solid var(--faint)', background: 'transparent' }}
      />
    )
  }
  const shape = SHAPE[rag]
  return (
    <span
      {...a11y}
      className={pulse ? 'pulse-dot inline-block' : 'inline-block'}
      style={{
        width: size,
        height: size,
        borderRadius: shape.radius,
        transform: shape.transform,
        background: COLOR[rag],
        // Glow only where motion already says "look here" — a resting green dot that blooms
        // makes the glow meaningless everywhere else.
        boxShadow: pulse ? `0 0 ${size}px color-mix(in srgb, ${COLOR[rag]} 70%, transparent)` : undefined,
      }}
    />
  )
}
