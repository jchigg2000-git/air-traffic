import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import Welcome from './Welcome.tsx'

function renderWelcome() {
  return render(
    <MemoryRouter initialEntries={['/welcome']}>
      <Welcome />
    </MemoryRouter>,
  )
}

afterEach(cleanup)

const VENDORS = [
  // Tier 1
  'OpenAI',
  'Anthropic',
  'AWS Bedrock',
  'Azure OpenAI',
  'Google Vertex',
  'GitHub Copilot',
  // Tier 2
  'M365 Copilot',
  'Mistral',
  'Databricks',
  'Perplexity',
  'Cohere',
  'Together',
  // Tier 3
  'Groq',
  'xAI',
  'Amazon Q',
  'IBM watsonx',
]

const DISPOSITION_LABELS = ['Native', 'Env-Managed', 'Proxy (opt)', 'Monitor', 'Unverified']

const PLANES = ['Developer-workflow', 'Data-policy', 'Budget']

describe('Welcome landing page', () => {
  it('renders the hero headline', () => {
    renderWelcome()
    expect(screen.getByRole('heading', { name: /one control tower for every ai vendor/i })).toBeInTheDocument()
  })

  it('shows all 16 vendor names', () => {
    renderWelcome()
    const body = document.body.textContent ?? ''
    for (const v of VENDORS) {
      expect(body, `expected vendor "${v}" to appear`).toContain(v)
    }
  })

  it('shows all 5 disposition labels', () => {
    renderWelcome()
    const body = document.body.textContent ?? ''
    for (const label of DISPOSITION_LABELS) {
      expect(body, `expected disposition "${label}" to appear`).toContain(label)
    }
  })

  it('shows the 3 control-plane names', () => {
    renderWelcome()
    const body = document.body.textContent ?? ''
    for (const plane of PLANES) {
      expect(body, `expected plane "${plane}" to appear`).toContain(plane)
    }
  })

  it('links the primary CTA to / and the secondary CTA to /settings/harness', () => {
    renderWelcome()

    const primary = screen.getAllByRole('link', { name: /enter the control tower/i })
    expect(primary.length).toBeGreaterThan(0)
    for (const link of primary) {
      expect(link).toHaveAttribute('href', '/')
    }

    const secondary = screen.getAllByRole('link', { name: /see redaction proof/i })
    expect(secondary.length).toBeGreaterThan(0)
    for (const link of secondary) {
      expect(link).toHaveAttribute('href', '/settings/harness')
    }
  })
})
