import type { Rag } from '../lib/format.ts'

const COLOR: Record<Rag, string> = { green: 'var(--green)', amber: 'var(--amber)', red: 'var(--red)' }

export default function StatusDot({ rag, pulse = false, size = 9 }: { rag: Rag; pulse?: boolean; size?: number }) {
  return (
    <span
      className={pulse ? 'pulse-dot inline-block rounded-full' : 'inline-block rounded-full'}
      style={{
        width: size,
        height: size,
        background: COLOR[rag],
        boxShadow: `0 0 ${size}px color-mix(in srgb, ${COLOR[rag]} 70%, transparent)`,
      }}
      aria-label={rag}
    />
  )
}
