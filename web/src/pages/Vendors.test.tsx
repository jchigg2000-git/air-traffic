import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, screen } from '@testing-library/react'
import { renderPage, ADAPTERS } from '../test/renderPage.tsx'

// Disabling a vendor drops it from both drift loops (internal/policy/drift.go),
// so the toggle must say that detection is paused rather than flip silently.

const patchAdapter = vi.fn()
let roster = ADAPTERS

vi.mock('../lib/api.ts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/api.ts')>()
  return {
    ...actual,
    api: {
      adapters: () => Promise.resolve(roster),
      calls: () => Promise.resolve([]),
      patchAdapter: (...args: unknown[]) => patchAdapter(...args),
      testAdapter: vi.fn(),
    },
  }
})

const { default: Vendors } = await import('./Vendors.tsx')

beforeEach(() => {
  patchAdapter.mockReset().mockResolvedValue(undefined)
  roster = ADAPTERS
})
afterEach(cleanup)

describe('the Enabled toggle and drift detection', () => {
  it('says drift detection is paused on a disabled vendor, and only there', async () => {
    roster = [{ ...ADAPTERS[0], enabled: false }, ADAPTERS[1]]
    renderPage(<Vendors />)
    await screen.findAllByText('OpenAI')
    // Row note plus the detail pane (the disabled vendor is selected first).
    expect(screen.getAllByText(/drift detection paused/i).length).toBeGreaterThanOrEqual(2)
    // The row switch and the detail-pane switch both carry the warning.
    const off = screen.getAllByRole('switch', { name: /Disabled — drift detection is paused/ })
    expect(off).toHaveLength(2)
    expect(screen.getByRole('switch', { name: /Enabled — click to disable \(pauses drift detection/ })).toHaveAttribute('aria-checked', 'true')

    // Patch behaviour is unchanged: the switch still just flips enabled.
    fireEvent.click(off[0])
    expect(patchAdapter).toHaveBeenCalledTimes(1)
    expect(patchAdapter).toHaveBeenCalledWith('openai', { enabled: true })
  })

  it('shows no paused notice when every vendor is enabled', async () => {
    renderPage(<Vendors />)
    await screen.findAllByText('OpenAI')
    expect(screen.queryByText(/drift detection paused/i)).not.toBeInTheDocument()
  })
})
