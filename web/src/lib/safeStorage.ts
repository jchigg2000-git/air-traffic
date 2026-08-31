// localStorage is not always reachable. Private mode, "block all site data", and
// some embedded webviews throw SecurityError on the *read*, not just the write —
// and main.tsx reads a theme at module scope, before createRoot, where a throw is
// a blank white page rather than a lost preference.
//
// Everything this app keeps here is discardable: a theme, an operator key. So the
// contract is degrade, never throw, and it lives in one place so no call site has
// to remember it.

export const safeStorage = {
  /** null when the value is absent OR storage is unavailable — the caller cannot tell, and does not need to. */
  get(key: string): string | null {
    try {
      return localStorage.getItem(key)
    } catch {
      return null
    }
  },

  set(key: string, value: string): void {
    try {
      localStorage.setItem(key, value)
    } catch {
      /* unavailable, or over quota — the preference is lost, the page is not */
    }
  },

  remove(key: string): void {
    try {
      localStorage.removeItem(key)
    } catch {
      /* see set */
    }
  },
}
