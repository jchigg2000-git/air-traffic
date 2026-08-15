/**
 * The one place a page admits it is showing data it can no longer vouch for.
 *
 * react-query retains the last successful response on a failed refetch, so without this a page
 * keeps rendering a dead feed as if it were live. Renders nothing when there is no error.
 */
export default function ApiStateBanner({ error, className = '' }: { error: unknown; className?: string }) {
  if (!error) return null
  return (
    <div className={`panel border-red/40 px-4 py-3 text-sm text-red ${className}`} role="alert">
      Cannot reach the Air-Traffic API (is the server running on :8122?) — showing the last data received.{' '}
      <span className="text-xs text-muted">{String(error)}</span>
    </div>
  )
}
