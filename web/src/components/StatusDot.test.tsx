import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import StatusDot from './StatusDot.tsx'

afterEach(cleanup)

describe('StatusDot', () => {
  it('states its meaning when nothing beside it does', () => {
    render(<StatusDot rag="amber" standalone />)
    expect(screen.getByRole('img', { name: 'Attention' })).toBeInTheDocument()
  })

  it('stays silent beside text that already says it', () => {
    render(<StatusDot rag="red" />)
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })

  it('distinguishes amber from red without relying on hue', () => {
    const { container: amber } = render(<StatusDot rag="amber" standalone />)
    const { container: red } = render(<StatusDot rag="red" standalone />)
    const shape = (c: HTMLElement) => {
      const el = c.firstElementChild as HTMLElement
      return `${el.style.borderRadius}|${el.style.transform}`
    }
    expect(shape(amber)).not.toBe(shape(red))
  })
})
