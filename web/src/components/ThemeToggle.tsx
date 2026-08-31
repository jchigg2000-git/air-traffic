import { useState } from 'react'
import { safeStorage } from '../lib/safeStorage.ts'

export default function ThemeToggle() {
  const [dark, setDark] = useState(() => document.documentElement.classList.contains('dark'))
  function toggle() {
    const next = !dark
    setDark(next)
    document.documentElement.classList.toggle('dark', next)
    safeStorage.set('at-theme', next ? 'dark' : 'light')
  }
  return (
    <button
      onClick={toggle}
      className="grid h-8 w-8 place-items-center rounded-lg border border-line bg-panel2 text-muted transition hover:text-fg"
      title={dark ? 'Switch to light' : 'Switch to dark'}
      aria-label="Toggle theme"
    >
      {dark ? '☾' : '☀'}
    </button>
  )
}
