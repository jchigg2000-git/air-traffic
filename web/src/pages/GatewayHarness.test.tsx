import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, screen } from '@testing-library/react'
import { renderPage } from '../test/renderPage.tsx'

// The harness holds the only irreversible control in the product: approving an
// `allow_list` proposal REMOVES a detection and there is no retraction path
// (ROADMAP.md PIVOT-7). These cover that the decision surface renders honestly
// and that a decision only happens on an explicit click.

const approveProposal = vi.fn()
const rejectProposal = vi.fn()
const startHarnessRun = vi.fn()

const PROPOSALS = [
  {
    id: 'p-add',
    kind: 'deny_list',
    type: 'PERSON_NAME',
    status: 'proposed',
    rationale: 'Presidio never saw these spans',
    sample_misses: 19,
    deny_list: ['Diego Novak'],
  },
  {
    id: 'p-suppress',
    kind: 'allow_list',
    type: 'PERSON_NAME',
    status: 'proposed',
    rationale: 'a false positive on a product name',
    sample_misses: 0,
    allow_list: ['acme'],
  },
  {
    id: 'p-done',
    kind: 'regex',
    type: 'SSN',
    status: 'approved',
    rationale: 'bare 9-digit SSNs after a context word',
    sample_misses: 3,
  },
]

vi.mock('../lib/api.ts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/api.ts')>()
  return {
    ...actual,
    api: {
      gatewayStatus: () =>
        Promise.resolve({
          gateways: [{ gateway_id: 'gw@test', base_url: 'http://127.0.0.1:8125', action: 'mask', vendors: {}, last_seen: '', fresh: true }],
          pattern_pack_version: 2,
          spine_auth: 'loopback_only',
          spine_key_unrotated: false,
          admin_auth: 'open',
          keystore_version: 0,
          keystore_error: '',
          policy_error: '',
        }),
      harnessRuns: () => Promise.resolve([]),
      harnessRatchet: () => Promise.resolve([]),
      harnessProposals: () => Promise.resolve(PROPOSALS),
      harnessCorpus: () => Promise.resolve([]),
      approveProposal,
      rejectProposal,
      startHarnessRun,
      runSample: vi.fn(),
    },
  }
})

const { default: GatewayHarness } = await import('./GatewayHarness.tsx')

beforeEach(() => {
  approveProposal.mockReset().mockResolvedValue(PROPOSALS)
  rejectProposal.mockReset().mockResolvedValue(PROPOSALS)
  startHarnessRun.mockReset().mockResolvedValue({ id: 'r1' })
})
afterEach(cleanup)

describe('the flywheel decision surface', () => {
  it('renders both proposal kinds with their rationale', async () => {
    renderPage(<GatewayHarness />)
    expect(await screen.findByText(/Presidio never saw these spans/)).toBeInTheDocument()
    expect(screen.getByText(/a false positive on a product name/)).toBeInTheDocument()
  })

  it('says plainly that nothing auto-applies', async () => {
    renderPage(<GatewayHarness />)
    expect(await screen.findByText(/human-approved only/i)).toBeInTheDocument()
  })

  it('offers a decision only on undecided proposals', async () => {
    renderPage(<GatewayHarness />)
    await screen.findByText(/Presidio never saw these spans/)
    // Two proposed, one already approved.
    expect(screen.getAllByRole('button', { name: 'Approve' })).toHaveLength(2)
    expect(screen.getAllByRole('button', { name: 'Reject' })).toHaveLength(2)
  })

  it('decides nothing until a decision is clicked', async () => {
    renderPage(<GatewayHarness />)
    await screen.findByText(/Presidio never saw these spans/)
    expect(approveProposal).not.toHaveBeenCalled()
    expect(rejectProposal).not.toHaveBeenCalled()
  })

  it('sends the approval for the proposal that was clicked', async () => {
    renderPage(<GatewayHarness />)
    await screen.findByText(/Presidio never saw these spans/)
    fireEvent.click(screen.getAllByRole('button', { name: 'Approve' })[0])
    expect(approveProposal).toHaveBeenCalledWith('p-add')
  })
})

describe('the run control', () => {
  it('does not start a run on render', async () => {
    renderPage(<GatewayHarness />)
    await screen.findByRole('button', { name: /Run Traffic/ })
    expect(startHarnessRun).not.toHaveBeenCalled()
  })
})
