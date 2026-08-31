import { afterEach, describe, expect, it, vi } from 'vitest'
import { safeStorage } from './safeStorage.ts'

afterEach(() => {
  vi.unstubAllGlobals()
  localStorage.clear()
})

/** A browser with site data blocked: every accessor throws, including the read. */
function denyStorage() {
  vi.stubGlobal('localStorage', {
    getItem() {
      throw new Error('denied')
    },
    setItem() {
      throw new Error('denied')
    },
    removeItem() {
      throw new Error('denied')
    },
  })
}

describe('safeStorage', () => {
  it('round-trips a value when storage works', () => {
    safeStorage.set('at-theme', 'light')
    expect(safeStorage.get('at-theme')).toBe('light')
    safeStorage.remove('at-theme')
    expect(safeStorage.get('at-theme')).toBeNull()
  })

  it('survives storage being unavailable', () => {
    denyStorage()
    expect(() => safeStorage.set('at-theme', 'light')).not.toThrow()
    expect(() => safeStorage.remove('at-theme')).not.toThrow()
    expect(safeStorage.get('at-theme')).toBeNull()
  })

  it('survives storage being absent entirely', () => {
    // Not every host defines the global at all; a bare reference would be a
    // ReferenceError, which is the same blank page as a SecurityError.
    vi.stubGlobal('localStorage', undefined)
    expect(safeStorage.get('at-theme')).toBeNull()
    expect(() => safeStorage.set('at-theme', 'light')).not.toThrow()
  })
})
