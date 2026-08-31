// The operator key for state-changing control-plane routes.
//
// The control plane is single-operator by decision (DECISIONS.md 2026-08-15):
// there is no user model and no login, so this is one shared secret held in
// this browser, not a session. It is stored in localStorage deliberately — a
// key that has to be re-pasted every reload gets disabled instead of used.
//
// What this is NOT: a permission boundary between people. Anyone holding the
// key holds all of it, and audit rows can name the system but never a person.
// Say that plainly wherever it appears in the UI.

import { safeStorage } from './safeStorage.ts'

const STORAGE_KEY = 'airtraffic.adminKey'
export const ADMIN_KEY_HEADER = 'X-Air-Traffic-Admin-Key'

export function getAdminKey(): string {
  // Private-mode / disabled storage: no key is a working state (the server may
  // not require one), so safeStorage degrades rather than throwing.
  return safeStorage.get(STORAGE_KEY) ?? ''
}

export function setAdminKey(key: string): void {
  if (key) safeStorage.set(STORAGE_KEY, key)
  else safeStorage.remove(STORAGE_KEY)
}

/** Header bag for a mutating request; empty when no key is held. */
export function adminHeaders(): Record<string, string> {
  const key = getAdminKey()
  return key ? { [ADMIN_KEY_HEADER]: key } : {}
}
