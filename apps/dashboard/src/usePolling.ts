import { useEffect, useRef, useState } from 'react'

export interface PollResult<T> {
  data: T | null
  error: string | null
  loading: boolean
}

// Fetch `url` on mount and every `intervalMs`. Keeps the last good data when a
// poll fails (so a transient blip doesn't blank the panel).
export function usePolling<T>(url: string, intervalMs = 4000): PollResult<T> {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const alive = useRef(true)

  useEffect(() => {
    alive.current = true
    const tick = async () => {
      try {
        const res = await fetch(url)
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
  }, [url, intervalMs])

  return { data, error, loading }
}
