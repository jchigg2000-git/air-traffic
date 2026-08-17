import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, screen, waitFor } from '@testing-library/react'
import { renderPage, BASELINES, ADAPTERS } from '../test/renderPage.tsx'

// Regression cover for PIVOT-1: applying "Healthcare" from this page used to be
// a one-click, permanent, organization-wide traffic block. The chain was
// api.applyPolicy(selected) with no overrides → Policy.Vendors empty →
// zdrAttested false → healthcare derives `block` for every caller, with no
// control anywhere in the UI able to set the attestation.

const applyPolicy = vi.fn()
// The attestation currently in force, as the server reports it.
let zdrAttested = false

vi.mock('../lib/api.ts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/api.ts')>()
  return {
    ...actual,
    api: {
      baselines: () => Promise.resolve({ baselines: BASELINES, zdr_attested: zdrAttested }),
      adapters: () => Promise.resolve(ADAPTERS),
      gatewayRequests: () =>
        Promise.resolve([
          { request_id: 'a', route: 'anthropic', action: 'mask', app_id: 'hf-sandbox', latency_ms: 1, added_latency_ms: 1, at: '' },
          { request_id: 'b', route: 'openai', action: 'mask', app_id: 'env', latency_ms: 1, added_latency_ms: 1, at: '' },
        ]),
      applyPolicy,
    },
  }
})

const { default: RigorConsole } = await import('./RigorConsole.tsx')

beforeEach(() => {
  zdrAttested = false
  applyPolicy.mockReset()
  applyPolicy.mockResolvedValue({ baseline: 'healthcare', applied_at: '', rows: [], summary: {} })
})
afterEach(cleanup)

async function open() {
  renderPage(<RigorConsole />)
  await screen.findByRole('radio', { name: /Healthcare/ })
}

describe('the Apply control cannot fire by accident', () => {
  it('arms nothing on arrival', async () => {
    await open()
    expect(screen.getByRole('button', { name: /Review & Apply/ })).toBeDisabled()
    // No profile is pre-selected; the old default was 'fintech'.
    for (const r of screen.getAllByRole('radio')) {
      expect(r).toHaveAttribute('aria-checked', 'false')
    }
  })

  it('never applies straight from the header button', async () => {
    await open()
    fireEvent.click(screen.getByRole('radio', { name: /Healthcare/ }))
    fireEvent.click(screen.getByRole('button', { name: /Review & Apply/ }))
    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    expect(applyPolicy).not.toHaveBeenCalled()
  })
})

describe('the derived gateway action is visible before commit', () => {
  it('labels each profile with what it does to traffic', async () => {
    await open()
    const healthcare = screen.getByRole('radio', { name: /Healthcare/ })
    expect(healthcare).toHaveTextContent('block')
    expect(healthcare).toHaveTextContent(/until ZDR attested/i)
    // The inverted lock ramp made this the trap: gov_infra renders 🔒🔒🔒 like
    // healthcare and enforces strictly less.
    expect(screen.getByRole('radio', { name: /Gov \/ Infra/ })).toHaveTextContent('mask')
    expect(screen.getByRole('radio', { name: /General SaaS/ })).toHaveTextContent('detect')
  })

  it('spells out the blast radius in the confirmation', async () => {
    await open()
    fireEvent.click(screen.getByRole('radio', { name: /Healthcare/ }))
    fireEvent.click(screen.getByRole('button', { name: /Review & Apply/ }))

    const dialog = await screen.findByRole('dialog')
    expect(dialog).toHaveTextContent(/REFUSED|refused/)
    // Two distinct app_ids came back from the traffic feed.
    expect(dialog).toHaveTextContent(/2 gateway callers/)
    expect(screen.getByRole('button', { name: /Apply and block traffic/ })).toBeInTheDocument()
  })
})

describe('the ZDR attestation', () => {
  it('is offered only where it changes the outcome', async () => {
    await open()
    fireEvent.click(screen.getByRole('radio', { name: /Fintech/ }))
    fireEvent.click(screen.getByRole('button', { name: /Review & Apply/ }))
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
  })

  it('is never pre-ticked', async () => {
    await open()
    fireEvent.click(screen.getByRole('radio', { name: /Healthcare/ }))
    fireEvent.click(screen.getByRole('button', { name: /Review & Apply/ }))
    expect(await screen.findByRole('checkbox')).not.toBeChecked()
  })

  it('flips the previewed action from block to mask', async () => {
    await open()
    fireEvent.click(screen.getByRole('radio', { name: /Healthcare/ }))
    fireEvent.click(screen.getByRole('button', { name: /Review & Apply/ }))
    fireEvent.click(await screen.findByRole('checkbox'))
    expect(await screen.findByRole('button', { name: /Apply and mask traffic/ })).toBeInTheDocument()
  })

  it('reaches the API as a vendor override — the argument that was never sent', async () => {
    await open()
    fireEvent.click(screen.getByRole('radio', { name: /Healthcare/ }))
    fireEvent.click(screen.getByRole('button', { name: /Review & Apply/ }))
    fireEvent.click(await screen.findByRole('checkbox'))
    fireEvent.click(screen.getByRole('button', { name: /Apply and mask traffic/ }))

    await waitFor(() =>
      expect(applyPolicy).toHaveBeenCalledWith('healthcare', {
        vendors: { anthropic: { zdr_attested: true } },
      }),
    )
  })

  it('applies unattested only as a deliberate, previewed choice', async () => {
    await open()
    fireEvent.click(screen.getByRole('radio', { name: /Healthcare/ }))
    fireEvent.click(screen.getByRole('button', { name: /Review & Apply/ }))
    fireEvent.click(await screen.findByRole('button', { name: /Apply and block traffic/ }))
    await waitFor(() => expect(applyPolicy).toHaveBeenCalledWith('healthcare', {}))
  })
})

// Found by opening the page against the live stack: with an attestation already
// in force the dialog previewed `mask` while an unticked checkbox would have
// sent an empty override map — silently revoking it and flipping the gateway to
// `block`. The original defect wearing a confirmation dialog.
describe('an attestation already in force', () => {
  it('seeds the checkbox instead of quietly revoking it', async () => {
    zdrAttested = true
    await open()
    fireEvent.click(screen.getByRole('radio', { name: /Healthcare/ }))
    fireEvent.click(screen.getByRole('button', { name: /Review & Apply/ }))
    expect(await screen.findByRole('checkbox')).toBeChecked()
    expect(screen.getByRole('button', { name: /Apply and mask traffic/ })).toBeInTheDocument()
  })

  it('re-applying keeps the attestation on the wire', async () => {
    zdrAttested = true
    await open()
    fireEvent.click(screen.getByRole('radio', { name: /Healthcare/ }))
    fireEvent.click(screen.getByRole('button', { name: /Review & Apply/ }))
    fireEvent.click(await screen.findByRole('button', { name: /Apply and mask traffic/ }))
    await waitFor(() =>
      expect(applyPolicy).toHaveBeenCalledWith('healthcare', {
        vendors: { anthropic: { zdr_attested: true } },
      }),
    )
  })

  it('previews the block the moment the operator unticks it', async () => {
    zdrAttested = true
    await open()
    fireEvent.click(screen.getByRole('radio', { name: /Healthcare/ }))
    fireEvent.click(screen.getByRole('button', { name: /Review & Apply/ }))
    fireEvent.click(await screen.findByRole('checkbox'))
    expect(await screen.findByRole('button', { name: /Apply and block traffic/ })).toBeInTheDocument()
  })

  it('shows the chip as mask while it holds, not as a permanent block', async () => {
    zdrAttested = true
    await open()
    expect(screen.getByRole('radio', { name: /Healthcare/ })).toHaveTextContent('mask')
    expect(screen.getByRole('radio', { name: /Healthcare/ })).toHaveTextContent(/on your ZDR attestation/i)
  })
})
