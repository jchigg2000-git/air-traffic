import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import ApiStateBanner from './ApiStateBanner.tsx'

afterEach(cleanup)

describe('ApiStateBanner', () => {
  it('says nothing while the feed is healthy', () => {
    const { container } = render(<ApiStateBanner error={null} hasData />)
    expect(container).toBeEmptyDOMElement()
  })

  it('names stale data as stale when a refetch failed over real data', () => {
    render(<ApiStateBanner error={new Error('boom')} hasData />)
    expect(screen.getByRole('alert')).toHaveTextContent(/showing the last data received/)
  })

  it('does not claim data it never received on a cold start', () => {
    render(<ApiStateBanner error={new Error('boom')} hasData={false} />)
    const alert = screen.getByRole('alert')
    expect(alert).toHaveTextContent(/no data has loaded yet/)
    expect(alert).not.toHaveTextContent(/last data received/)
  })
})
