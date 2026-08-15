import type { Rag } from '../lib/format.ts'

/** 'off' is not a severity — it means the vendor is not emitting, so it has no health to report. */
export type DotState = Rag | 'off'

const COLOR: Record<Rag, string> = { green: 'var(--green)', amber: 'var(--amber)', red: 'var(--red)' }

export default function StatusDot({ rag, pulse = false, size = 9 }: { rag: DotState; pulse?: boolean; size?: number }) {
  if (rag === 'off') {
    return (
      <span
        className="inline-block rounded-full"
        style={{ width: size, height: size, border: '1px solid var(--faint)', background: 'transparent' }}
        aria-label="off"
      />
    )
  }
  return (
    <span
      className={pulse ? 'pulse-dot inline-block rounded-full' : 'inline-block rounded-full'}
      style={{
        width: size,
        height: size,
        background: COLOR[rag],
        // Glow only where motion already says "look here" — a resting green dot that blooms
        // makes the glow meaningless everywhere else.
        boxShadow: pulse ? `0 0 ${size}px color-mix(in srgb, ${COLOR[rag]} 70%, transparent)` : undefined,
      }}
      aria-label={rag}
    />
  )
}
