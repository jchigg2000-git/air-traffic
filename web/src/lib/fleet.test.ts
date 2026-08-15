import { describe, expect, it } from 'vitest'
import { computeFleet } from './fleet.ts'
import type { Adapter, ObservationRecord } from './api.ts'

// Pins ONLY the honest-liveness semantics: a vendor may not read as healthy unless it is
// actually emitting, and an emitting vendor's health decays as its feed goes stale.

const NOW = Date.parse('2026-08-15T12:00:00.000Z')

function adapter(id: string, over: Partial<Adapter> = {}): Adapter {
  return {
    id,
    display_name: id,
    mode: 'synthetic',
    enabled: true,
    emit: true,
    tier: 1,
    capabilities: [],
    ...over,
  } as unknown as Adapter
}

function obs(vendor: string, agoSec: number): ObservationRecord {
  return {
    connector_instance: vendor,
    received_at: new Date(NOW - agoSec * 1000).toISOString(),
    observation_count: 1,
    error_count: 0,
    body: { observations: [] },
  } as unknown as ObservationRecord
}

describe('computeFleet — emitting gate', () => {
  it('marks a disabled adapter as not emitting and excludes it from healthy', () => {
    const fleet = computeFleet([adapter('off_vendor', { enabled: false })], [], [], NOW)
    expect(fleet.vendors[0].emitting).toBe(false)
    expect(fleet.healthy).toBe(0)
    expect(fleet.emittingCount).toBe(0)
  })

  it('treats emit:false and mode:disabled as not emitting', () => {
    const fleet = computeFleet([adapter('a', { emit: false }), adapter('b', { mode: 'disabled' as Adapter['mode'] })], [], [], NOW)
    expect(fleet.vendors.map((v) => v.emitting)).toEqual([false, false])
    expect(fleet.emittingCount).toBe(0)
  })

  it('counts an enabled, fresh, green vendor as healthy', () => {
    const fleet = computeFleet([adapter('live')], [obs('live', 2)], [], NOW)
    expect(fleet.vendors[0].emitting).toBe(true)
    expect(fleet.vendors[0].worstRag).toBe('green')
    expect(fleet.healthy).toBe(1)
    expect(fleet.emittingCount).toBe(1)
  })

  it('decays an emitting vendor whose feed goes stale', () => {
    expect(computeFleet([adapter('live')], [obs('live', 45)], [], NOW).vendors[0].worstRag).toBe('amber')
    expect(computeFleet([adapter('live')], [obs('live', 300)], [], NOW).vendors[0].worstRag).toBe('red')
  })

  it('does not decay a non-emitting vendor — it is off, not stale', () => {
    const fleet = computeFleet([adapter('off_vendor', { enabled: false })], [], [], NOW)
    expect(fleet.vendors[0].worstRag).toBe('green')
    expect(fleet.vendors[0].emitting).toBe(false)
  })

  it('surfaces an emitter that has no adapter row', () => {
    const fleet = computeFleet([adapter('live')], [obs('live', 2), obs('gw@0.0.0.0:8125', 2)], [], NOW)
    expect(fleet.unmatchedEmitters).toEqual(['gw@0.0.0.0:8125'])
  })
})
