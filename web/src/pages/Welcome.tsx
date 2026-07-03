import { Link } from 'react-router-dom'
import Brand from '../components/Brand.tsx'
import ThemeToggle from '../components/ThemeToggle.tsx'
import Hero from './landing/Hero.tsx'
import Planes from './landing/Planes.tsx'
import Dispositions from './landing/Dispositions.tsx'
import VendorWall from './landing/VendorWall.tsx'
import HowItWorks from './landing/HowItWorks.tsx'
import CtaBand from './landing/CtaBand.tsx'

// Welcome — the public landing page. Straight-line composition: each section is an
// independent, self-contained component owning its own copy + data, read top to
// bottom. No content registry, no generic section renderer, no data-array map.

export default function Welcome() {
  return (
    <main className="min-h-screen">
      <Hero />
      <Planes />
      <Dispositions />
      <VendorWall />
      <HowItWorks />
      <CtaBand />

      <footer className="border-t border-line">
        <div className="mx-auto flex max-w-5xl flex-col items-start justify-between gap-4 px-5 py-8 sm:flex-row sm:items-center">
          <div className="flex flex-col gap-1">
            <Brand size={22} />
            <span className="text-xs text-faint">Enterprise AI Control Plane · local / synthetic build</span>
          </div>
          <div className="flex items-center gap-4">
            <Link to="/" className="text-sm text-muted transition hover:text-accent">
              Control Tower
            </Link>
            <Link to="/settings/harness" className="text-sm text-muted transition hover:text-accent">
              Gateway Harness
            </Link>
            <Link to="/settings/policy" className="text-sm text-muted transition hover:text-accent">
              Policy
            </Link>
            <ThemeToggle />
          </div>
        </div>
      </footer>
    </main>
  )
}
