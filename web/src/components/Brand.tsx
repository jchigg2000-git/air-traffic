// Air-Traffic routed-arrow mark (navy/cyan), inline SVG so it themes + scales.
export default function Brand({ size = 26, withText = true }: { size?: number; withText?: boolean }) {
  return (
    <span className="inline-flex items-center gap-2.5">
      <svg width={size} height={size} viewBox="0 0 32 32" fill="none" aria-hidden>
        <rect x="1" y="1" width="30" height="30" rx="8" fill="#0B1220" stroke="#0891B2" strokeOpacity="0.5" />
        <circle cx="16" cy="16" r="9" stroke="#38D5EA" strokeOpacity="0.35" strokeWidth="1.2" />
        <path d="M7 22 L20 11 M20 11 L20 16.5 M20 11 L14.5 11" stroke="#38D5EA" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
        <circle cx="16" cy="16" r="1.7" fill="#38D5EA" />
      </svg>
      {withText && (
        <span className="font-semibold tracking-tight" style={{ fontSize: size * 0.62 }}>
          Air-Traffic
        </span>
      )}
    </span>
  )
}
