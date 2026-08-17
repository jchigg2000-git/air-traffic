import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, qk } from '../lib/api.ts'
import { getAdminKey, setAdminKey } from '../lib/adminKey.ts'

// Where the operator pastes the key that state-changing routes require.
//
// It renders only when the server actually requires one (`admin_auth` on
// /api/health), because a credential field on an open control plane invites
// the belief that something is protecting it. When the server reports "open",
// this says so instead — the same honesty rule the rest of the app follows
// about claims it cannot substantiate.
export default function AdminKeyField() {
  const health = useQuery({ queryKey: qk.health, queryFn: api.health })
  const [key, setKey] = useState(getAdminKey())
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    if (!saved) return
    const t = setTimeout(() => setSaved(false), 1600)
    return () => clearTimeout(t)
  }, [saved])

  if (health.data && health.data.admin_auth !== 'admin_key') {
    return (
      <p className="px-1 text-[10px] leading-relaxed text-faint">
        Writes unauthenticated — set <code>AIRTRAFFIC_ADMIN_KEY</code> to require an operator key.
      </p>
    )
  }
  if (!health.data) return null

  return (
    <div className="px-1">
      <label htmlFor="admin-key" className="mb-1 block text-[10px] uppercase tracking-wider text-faint">
        Operator key
      </label>
      <div className="flex gap-1.5">
        <input
          id="admin-key"
          type="password"
          value={key}
          onChange={(e) => setKey(e.target.value)}
          placeholder="required for changes"
          aria-describedby="admin-key-help"
          className="min-w-0 flex-1 rounded-lg border border-line bg-panel2 px-2 py-1 text-xs outline-none focus:border-accent"
        />
        <button
          onClick={() => {
            setAdminKey(key)
            setSaved(true)
          }}
          className="rounded-lg border border-line px-2 py-1 text-xs text-muted transition hover:text-fg"
        >
          {saved ? '✓' : 'Save'}
        </button>
      </div>
      <p id="admin-key-help" className="mt-1 text-[10px] leading-relaxed text-faint">
        Single shared operator key, kept in this browser. It gates changes, not people — there are no
        user accounts, and audit rows name the system rather than a person.
      </p>
    </div>
  )
}
