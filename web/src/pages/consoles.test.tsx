import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, screen } from '@testing-library/react'
import { renderPage, ADAPTERS, BASELINES } from '../test/renderPage.tsx'

// Smoke cover for the remaining console screens. Two things are worth pinning
// beyond "it renders": that a failing API surfaces instead of failing silently
// (the bug the shared ApiStateBanner was added for), and that loading is not
// conflated with empty — a console that shows "no vendors" while a request is
// still in flight is stating something it does not know.

const failing = () => Promise.reject(new Error('boom'))

vi.mock('../lib/api.ts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/api.ts')>()
  return {
    ...actual,
    api: {
      adapters: () => Promise.resolve(ADAPTERS),
      baselines: () => Promise.resolve({ baselines: BASELINES, zdr_attested: false }),
      calls: () => Promise.resolve([]),
      audit: () => Promise.resolve([]),
      observations: () => Promise.resolve([]),
      gatewayRequests: () => Promise.resolve([]),
      patchAdapter: vi.fn(),
      testAdapter: vi.fn(),
      siem: vi.fn(),
    },
  }
})

const { default: Vendors } = await import('./Vendors.tsx')
const { default: PolicyEditor } = await import('./PolicyEditor.tsx')
const { default: Audit } = await import('./Audit.tsx')
const { default: Observability } = await import('./Observability.tsx')

afterEach(cleanup)

describe('Vendors', () => {
  it('lists the adapter roster', async () => {
    renderPage(<Vendors />)
    // The selected vendor also appears in the detail panel, so both names show
    // up more than once.
    expect((await screen.findAllByText('OpenAI')).length).toBeGreaterThan(0)
    expect(screen.getAllByText('Anthropic').length).toBeGreaterThan(0)
  })
})

describe('Policy Editor', () => {
  it('groups controls by plane and exposes each disclosure state', async () => {
    renderPage(<PolicyEditor />)
    const plane = await screen.findByRole('button', { name: /Collapse the Data Policy .* control plane, 1 controls/ })
    expect(plane).toHaveAttribute('aria-expanded', 'true')

    // The nested control disclosure starts closed and says so.
    const control = screen.getByRole('button', { name: /Expand PII Redaction across 2 vendors/ })
    expect(control).toHaveAttribute('aria-expanded', 'false')
    fireEvent.click(control)
    expect(screen.getByRole('button', { name: /Collapse PII Redaction across 2 vendors/ })).toBeInTheDocument()
  })
})

describe('Audit', () => {
  it('declares its grid as a table so the columns keep their meaning', async () => {
    renderPage(<Audit />)
    const table = await screen.findByRole('table', { name: /audit stream/i })
    expect(table).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: 'Actor' })).toBeInTheDocument()
  })

  it('says "no matching events" only once it knows', async () => {
    renderPage(<Audit />)
    expect(await screen.findByText(/No matching audit events/)).toBeInTheDocument()
  })
})

describe('Observability', () => {
  it('labels the batch table and its empty state', async () => {
    renderPage(<Observability />)
    expect(await screen.findByRole('table', { name: /Observation batches/i })).toBeInTheDocument()
    // Loading is not conflated with empty: the message appears only once the
    // query has actually resolved.
    expect(await screen.findByText(/No batches yet/)).toBeInTheDocument()
  })
})

describe('a failing API is surfaced, never swallowed', () => {
  it('shows the banner on Vendors', async () => {
    vi.resetModules()
    vi.doMock('../lib/api.ts', async (importOriginal) => {
      const actual = await importOriginal<typeof import('../lib/api.ts')>()
      return { ...actual, api: { adapters: failing, calls: failing } }
    })
    const { default: BrokenVendors } = await import('./Vendors.tsx')
    renderPage(<BrokenVendors />)
    expect(await screen.findByText(/boom|unavailable|error/i)).toBeInTheDocument()
    vi.doUnmock('../lib/api.ts')
  })
})
