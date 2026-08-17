import { afterEach, describe, expect, it, vi } from 'vitest'
import { ADMIN_KEY_HEADER, adminHeaders, getAdminKey, setAdminKey } from './adminKey.ts'
import { ApiError, send } from './api.ts'

afterEach(() => {
  setAdminKey('')
  vi.unstubAllGlobals()
})

describe('operator key storage', () => {
  it('round-trips through localStorage', () => {
    setAdminKey('adm-abc123')
    expect(getAdminKey()).toBe('adm-abc123')
    expect(adminHeaders()).toEqual({ [ADMIN_KEY_HEADER]: 'adm-abc123' })
  })

  it('sends no header at all when unset', () => {
    // The server's default posture is open; an empty header would read as a
    // failed credential rather than as no credential.
    expect(adminHeaders()).toEqual({})
  })

  it('clears rather than storing an empty string', () => {
    setAdminKey('adm-abc123')
    setAdminKey('')
    expect(getAdminKey()).toBe('')
    expect(localStorage.getItem('airtraffic.adminKey')).toBeNull()
  })

  it('survives storage being unavailable', () => {
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
    expect(() => setAdminKey('adm-x')).not.toThrow()
    expect(getAdminKey()).toBe('')
  })
})

describe('mutating requests carry the key', () => {
  function stubFetch(status: number) {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: status < 400,
      status,
      json: () => Promise.resolve({ ok: true }),
    })
    vi.stubGlobal('fetch', fetchMock)
    return fetchMock
  }

  it('attaches the header in exactly one place', async () => {
    const fetchMock = stubFetch(200)
    setAdminKey('adm-abc123')
    await send('PUT', '/api/policies', { baseline: 'fintech' })

    const [, init] = fetchMock.mock.calls[0]
    expect(init.headers[ADMIN_KEY_HEADER]).toBe('adm-abc123')
    expect(init.method).toBe('PUT')
  })

  it('omits it entirely when no key is held', async () => {
    const fetchMock = stubFetch(200)
    await send('PUT', '/api/policies', { baseline: 'fintech' })
    expect(fetchMock.mock.calls[0][1].headers).not.toHaveProperty(ADMIN_KEY_HEADER)
  })

  it('turns a 401 into an actionable message rather than a bare status', async () => {
    stubFetch(401)
    await expect(send('PUT', '/api/policies', {})).rejects.toMatchObject({ status: 401 })
    await expect(send('PUT', '/api/policies', {})).rejects.toThrow(/Operator key/i)
  })

  it('leaves other failures as plain ApiErrors', async () => {
    stubFetch(500)
    const err = (await send('PUT', '/api/policies', {}).catch((e) => e)) as ApiError
    expect(err).toBeInstanceOf(ApiError)
    expect(err.status).toBe(500)
    expect(err.message).not.toMatch(/Operator key/i)
  })
})
