// Air-Traffic holding-pattern mark — the real logo-pack icon
// (air-traffic-option-4-logo-pack/air-traffic-icon-dark.svg), inlined so it scales.
export default function Brand({ size = 26, withText = true }: { size?: number; withText?: boolean }) {
  return (
    <span className="inline-flex items-center gap-2.5">
      <svg width={size} height={size} viewBox="0 0 512 512" role="img" aria-label="Air-Traffic">
        <rect width="512" height="512" rx="112" fill="#07111F" />
        <g fill="none" strokeLinecap="round" strokeLinejoin="round">
          <path d="M184 138 H328 C400 138 400 256 328 256 H184 C112 256 112 374 184 374 H328" stroke="#F8FAFC" strokeWidth="38" />
          <path d="M292 320 L344 374 L292 426" stroke="#38D5EA" strokeWidth="36" />
        </g>
        <circle cx="184" cy="138" r="27" fill="#38D5EA" />
      </svg>
      {withText && (
        <span className="font-semibold tracking-tight" style={{ fontSize: size * 0.62 }}>
          Air-Traffic
        </span>
      )}
    </span>
  )
}
