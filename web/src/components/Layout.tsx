import { NavLink, Outlet, Link } from 'react-router-dom'
import Brand from './Brand.tsx'
import ThemeToggle from './ThemeToggle.tsx'

const NAV = [
  { to: '/settings/rigor', label: 'Rigor Console', icon: '🎚', desc: 'Baseline rigor' },
  { to: '/settings/policy', label: 'Policy Editor', icon: '⚙', desc: 'Per-vendor controls' },
  { to: '/settings/cost', label: 'Cost & Usage', icon: '◷', desc: 'Spend & caps' },
  { to: '/settings/vendors', label: 'Vendors', icon: '⛁', desc: 'Surfaces & modes' },
  { to: '/settings/observability', label: 'Observability', icon: '∿', desc: 'Raw batches' },
  { to: '/settings/audit', label: 'Audit', icon: '❏', desc: 'Audit stream' },
]

export default function Layout() {
  return (
    <div className="mx-auto flex min-h-screen max-w-[1500px] gap-6 px-5 py-5">
      <aside className="sticky top-5 hidden h-[calc(100vh-2.5rem)] w-64 shrink-0 flex-col panel p-4 md:flex">
        <Link to="/" className="mb-1 flex items-center px-1">
          <Brand size={24} />
        </Link>
        <Link to="/" className="mb-4 px-1 text-xs text-muted transition hover:text-accent">
          ← Back to Flight Deck
        </Link>
        <nav className="flex flex-col gap-1">
          {NAV.map((n) => (
            <NavLink
              key={n.to}
              to={n.to}
              className={({ isActive }) =>
                `flex items-center gap-3 rounded-lg px-3 py-2 text-sm transition ${
                  isActive ? 'bg-panel2 text-fg' : 'text-muted hover:bg-panel2 hover:text-fg'
                }`
              }
            >
              <span className="w-4 text-center text-base text-accent">{n.icon}</span>
              <span className="flex flex-col leading-tight">
                <span>{n.label}</span>
                <span className="text-[10px] text-faint">{n.desc}</span>
              </span>
            </NavLink>
          ))}
        </nav>
        <div className="mt-auto flex items-center justify-between px-1 pt-4">
          <span className="text-[10px] text-faint">Phase 2 · control plane</span>
          <ThemeToggle />
        </div>
      </aside>

      <main className="min-w-0 flex-1">
        <Outlet />
      </main>
    </div>
  )
}
