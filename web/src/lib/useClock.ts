import { useEffect, useState } from 'react'

/** Re-renders every `ms` so relative-time / freshness labels tick live. */
export function useClock(ms = 1000): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), ms)
    return () => clearInterval(id)
  }, [ms])
  return now
}
