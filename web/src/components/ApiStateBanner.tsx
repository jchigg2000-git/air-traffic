/**
 * The one place a page admits it is showing data it can no longer vouch for.
 *
 * react-query retains the last successful response on a failed refetch, so without this a page
 * keeps rendering a dead feed as if it were live. Renders nothing when there is no error.
 *
 * `hasData` separates the two failures. A refetch that failed over real data leaves stale content
 * on screen; a cold start leaves nothing at all, and claiming "the last data received" above an
 * empty page is the same lie in the other direction.
 */
export default function ApiStateBanner({
  error,
  hasData,
  className = '',
}: {
  error: unknown
  hasData: boolean
  className?: string
}) {
  if (!error) return null
  return (
    <div className={`panel border-red/40 px-4 py-3 text-sm text-red ${className}`} role="alert">
      Cannot reach the Air-Traffic API (is the server running on :8122?){' '}
      {hasData ? '— showing the last data received.' : '— no data has loaded yet.'}{' '}
      <span className="text-xs text-muted">{String(error)}</span>
    </div>
  )
}
