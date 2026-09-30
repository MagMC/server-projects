import { useCallback, useEffect, useRef, useState } from 'react'

export interface PollResult<T> {
  data: T | null
  error: string | null
  loading: boolean
  refresh: () => void
}

// Fetch `url` on mount and every `intervalMs`. Keeps the last good data when a
// poll fails (so a transient blip doesn't blank the panel). A null url pauses
// polling; `token` is sent as a bearer token for protected endpoints.
export function usePolling<T>(url: string | null, intervalMs = 4000, token?: string): PollResult<T> {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [nonce, setNonce] = useState(0)
  const alive = useRef(true)

  useEffect(() => {
    if (!url) return
    alive.current = true
    const tick = async () => {
      try {
        const res = await fetch(url, token ? { headers: { Authorization: `Bearer ${token}` } } : undefined)
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        const json = (await res.json()) as T
        if (alive.current) {
          setData(json)
          setError(null)
        }
      } catch (e) {
        if (alive.current) setError(e instanceof Error ? e.message : String(e))
      } finally {
        if (alive.current) setLoading(false)
      }
    }
    tick()
    const id = setInterval(tick, intervalMs)
    return () => {
      alive.current = false
      clearInterval(id)
    }
  }, [url, intervalMs, token, nonce])

  const refresh = useCallback(() => setNonce((n) => n + 1), [])
  return { data, error, loading, refresh }
}
