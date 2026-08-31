import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, fireEvent, screen } from '@testing-library/react'
// Type-only: a value import here would evaluate the hoisted vi.mock factory
// before the consts it closes over exist.
import type { HarnessRun, RunScore } from '../lib/api.ts'
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

const STATUS = {
  gateways: [{ gateway_id: 'gw@test', base_url: 'http://127.0.0.1:8125', action: 'mask', vendors: {}, last_seen: '', fresh: true }],
  pattern_pack_version: 2,
  spine_auth: 'loopback_only',
  spine_key_unrotated: false,
  admin_auth: 'open',
  keystore_version: 0,
  keystore_error: '',
  policy_error: '',
}

// Both are flipped mid-file: `statusFails` reproduces a control-plane blip over
// data already on screen, `RUNS` supplies a scored run.
let statusFails = false
let RUNS: HarnessRun[] = []

vi.mock('../lib/api.ts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/api.ts')>()
  return {
    ...actual,
    api: {
      gatewayStatus: () => (statusFails ? Promise.reject(new Error('control plane blip')) : Promise.resolve(STATUS)),
      harnessRuns: () => Promise.resolve(RUNS),
      harnessResults: () => Promise.resolve([]),
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

/** A finished run carrying the score fields under test; the rest is filler. */
function doneRun(score: Partial<RunScore>): HarnessRun {
  return {
    id: 'r-scored',
    config: { count: 200, concurrency: 4, seed: 7, include_traps: true, include_presidio_only: true, include_straddle: true, replay_percent: 20 },
    status: 'done',
    total: 200,
    completed: 200,
    masked: 200,
    blocked: 0,
    detect_only: 0,
    passed: 0,
    errors: 0,
    pack_version: 2,
    detector_chain: 'regex,presidio',
    promoted_count: 0,
    started_at: '2026-08-30T00:00:00Z',
    score: {
      precision: 1,
      recall_reported: 1,
      recall_behavioral: 0,
      trap_fps: 0,
      response_leaks: 0,
      by_type: {},
      by_engine: {},
      joined_reports: 200,
      orphan_requests: 0,
      capture_orphans: 0,
      ...score,
    },
  }
}

const { default: GatewayHarness } = await import('./GatewayHarness.tsx')

beforeEach(() => {
  statusFails = false
  RUNS = []
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

describe('a dead control plane is not a dead gateway', () => {
  it('reports the outage instead of telling the operator to start the data plane', async () => {
    vi.resetModules()
    const failing = () => Promise.reject(new Error('boom'))
    vi.doMock('../lib/api.ts', async (importOriginal) => {
      const actual = await importOriginal<typeof import('../lib/api.ts')>()
      return {
        ...actual,
        api: {
          gatewayStatus: failing,
          harnessRuns: failing,
          harnessRatchet: failing,
          harnessProposals: failing,
          harnessCorpus: failing,
          approveProposal: vi.fn(),
          rejectProposal: vi.fn(),
          startHarnessRun: vi.fn(),
          runSample: vi.fn(),
        },
      }
    })
    const { default: BrokenHarness } = await import('./GatewayHarness.tsx')
    renderPage(<BrokenHarness />)

    expect(await screen.findByRole('alert')).toHaveTextContent(/no data has loaded yet/)
    // The gateway may well be fine; nothing here knows either way.
    expect(screen.queryByText(/Start the data plane first/)).not.toBeInTheDocument()
    expect(screen.queryByText(/pattern pack v0/)).not.toBeInTheDocument()
    expect(screen.getByText(/pattern pack unknown/)).toBeInTheDocument()
    vi.doUnmock('../lib/api.ts')
  })
})

// A failed refetch leaves react-query holding the last good status alongside an
// error. Reading the chip's label off the error while its dot read off the data
// put a GREEN dot next to the words "gateway status unknown".
describe('a status refetch that fails over data already on screen', () => {
  it('keeps the dot and the label saying the same thing', async () => {
    const { qk } = await import('../lib/api.ts')
    const { qc } = renderPage(<GatewayHarness />)
    const chip = await screen.findByText(/gateway gw@test/)
    expect(chip.firstElementChild?.getAttribute('style')).toContain('var(--green)')

    statusFails = true
    await act(() => qc.refetchQueries({ queryKey: qk.gatewayStatus }))

    expect(await screen.findByRole('alert')).toHaveTextContent(/showing the last data received/)
    // Still green, because the data behind it is still the last thing the
    // control plane said — so it may not claim to know nothing.
    const after = screen.getByText(/gateway gw@test/)
    expect(after.firstElementChild?.getAttribute('style')).toContain('var(--green)')
    expect(screen.queryByText(/gateway status unknown/)).not.toBeInTheDocument()
  })
})

// Orphaned values are held out of the behavioral denominator server-side
// (model.RunScore.CaptureOrphans), so a run where no capture joined scores 0.0.
// Printing that as a red 0.0% beside a full join-coverage figure is exactly the
// misreading the field exists to prevent.
describe('a run whose captures never joined', () => {
  it('reports behavioral recall as unverified rather than as a number', async () => {
    RUNS = [doneRun({ recall_behavioral: 0, capture_orphans: 200 })]
    renderPage(<GatewayHarness />)
    expect(await screen.findByText('unverified')).toBeInTheDocument()
    expect(screen.queryByText('0.0%')).not.toBeInTheDocument()
    expect(screen.getByText(/held out of behavioral recall/)).toBeInTheDocument()
    // Join coverage is a different join and still reports what it knows — the
    // pairing that made the red 0.0% read as a catastrophe.
    expect(screen.getByText('join coverage').previousElementSibling).toHaveTextContent('200/200')
  })

  it('prints the figure when every seeded value was verified', async () => {
    RUNS = [doneRun({ recall_behavioral: 0.97, capture_orphans: 0 })]
    renderPage(<GatewayHarness />)
    expect(await screen.findByText('97.0%')).toBeInTheDocument()
    expect(screen.queryByText('unverified')).not.toBeInTheDocument()
    expect(screen.queryByText(/held out of behavioral recall/)).not.toBeInTheDocument()
  })
})
