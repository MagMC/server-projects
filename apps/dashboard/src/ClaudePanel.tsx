import { useEffect, useLayoutEffect, useMemo, useRef, useState, type CSSProperties } from 'react'
import { createPortal } from 'react-dom'
import { Card } from './Card'
import { parseAnsi } from './ansi'
import { uptime } from './format'
import { usePolling } from './usePolling'
import type { ClaudeScreen, ClaudeStatus } from './types'

// The control token lives only in this browser (the API checks it). Storage can
// be unavailable (private mode), so every access is guarded.
const TOKEN_KEY = 'claudeControlToken'

function loadToken(): string {
  try {
    return localStorage.getItem(TOKEN_KEY) ?? ''
  } catch {
    return ''
  }
}

function saveToken(t: string) {
  try {
    if (t) localStorage.setItem(TOKEN_KEY, t)
    else localStorage.removeItem(TOKEN_KEY)
  } catch {
    /* not persisted; still works for this page load */
  }
}

function useToken(): [string, () => string, () => void] {
  const [token, setToken] = useState(loadToken)
  // Returns the current token, asking for one if there isn't any yet.
  const ensure = () => {
    if (token) return token
    const t = window.prompt('Control token (CLAUDE_CONTROL_TOKEN in infra/docker/.env)')?.trim() ?? ''
    saveToken(t)
    setToken(t)
    return t
  }
  const forget = () => {
    saveToken('')
    setToken('')
  }
  return [token, ensure, forget]
}

function stateOf(s: ClaudeStatus): { label: string; dot: string } {
  if (!s.available) return { label: 'tmux down', dot: 'dot dot--bad' }
  if (s.running) return { label: 'Running', dot: 'dot dot--ok' }
  if (s.exists) return { label: 'Shell idle', dot: 'dot dot--warn' }
  return { label: 'Off', dot: 'dot' }
}

export function ClaudePanel() {
  const { data, error, refresh } = usePolling<ClaudeStatus>('/api/claude/status')
  const [token, ensureToken, forgetToken] = useToken()
  const [busy, setBusy] = useState(false)
  const [actionErr, setActionErr] = useState<string | null>(null)
  const [viewing, setViewing] = useState(false)

  const toggle = async () => {
    if (!data || busy) return
    const stopping = data.running
    if (stopping && !window.confirm(`Stop Claude? This kills the tmux session "${data.session}".`)) return
    const t = ensureToken()
    if (!t) return
    setBusy(true)
    setActionErr(null)
    try {
      const res = await fetch(`/api/claude/${stopping ? 'stop' : 'start'}`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${t}` },
      })
      if (res.status === 401) forgetToken()
      if (!res.ok) {
        const body = (await res.json().catch(() => null)) as { error?: string } | null
        throw new Error(body?.error ?? `HTTP ${res.status}`)
      }
    } catch (e) {
      setActionErr(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
      refresh()
    }
  }

  const openViewer = () => {
    if (data?.exists && ensureToken()) setViewing(true)
  }

  const state = data ? stateOf(data) : null
  const canControl = !!data?.available && data.controlEnabled

  return (
    <Card title="Claude" error={error} badge={data ? `tmux · ${data.session}` : undefined}>
      {!data || !state ? (
        <p className="muted">syncing…</p>
      ) : (
        <div className="claude">
          <button
            type="button"
            className="claude__status"
            onClick={openViewer}
            disabled={!data.exists || !data.controlEnabled}
            title={data.exists ? 'View session' : undefined}
          >
            <span className="claude__state">
              <span className={state.dot} /> {state.label}
              {data.exists && data.controlEnabled && <span className="claude__view">view ›</span>}
            </span>
            {data.exists && (
              <span className="claude__meta">
                <span>{data.path.replace(/^\/home\/[^/]+/, '~')}</span>
                <span>
                  up {uptime(Math.floor(Date.now() / 1000) - data.createdAt)} · {data.command}
                  {data.attached > 0 && ` · ${data.attached} attached`}
                </span>
              </span>
            )}
          </button>

          <div className="claude__actions">
            <button
              type="button"
              role="switch"
              aria-checked={data.running}
              className={`switch ${data.running ? 'switch--on' : ''}`}
              onClick={toggle}
              disabled={!canControl || busy}
            >
              <span className="switch__knob" />
            </button>
            <span className="muted claude__hint">
              {busy ? 'working…' : data.running ? 'On' : 'Off'}
            </span>
          </div>

          {(actionErr || data.error || !data.controlEnabled) && (
            <p className="claude__err">
              {actionErr ?? data.error ?? 'Controls disabled: CLAUDE_CONTROL_TOKEN not set'}
            </p>
          )}
        </div>
      )}
      {viewing && data && (
        <SessionViewer session={data.session} token={token} onClose={() => setViewing(false)} onBadToken={forgetToken} />
      )}
    </Card>
  )
}

function SessionViewer({
  session,
  token,
  onClose,
  onBadToken,
}: {
  session: string
  token: string
  onClose: () => void
  onBadToken: () => void
}) {
  const { data, error } = usePolling<ClaudeScreen>(token ? '/api/claude/screen' : null, 1500, token)
  const segments = useMemo(() => (data ? parseAnsi(data.text) : []), [data])
  const termRef = useRef<HTMLPreElement>(null)
  // Follow new output unless the user has scrolled up to read history.
  const stick = useRef(true)

  useEffect(() => {
    if (error === 'HTTP 401') {
      onBadToken()
      onClose()
    }
  }, [error, onBadToken, onClose])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  useLayoutEffect(() => {
    const el = termRef.current
    if (el && stick.current) el.scrollTop = el.scrollHeight
  }, [segments])

  const onScroll = () => {
    const el = termRef.current
    if (el) stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40
  }

  // Portal: the card's backdrop-filter would otherwise contain position:fixed.
  return createPortal(
    <div className="modal" onClick={onClose}>
      <div className="modal__box" onClick={(e) => e.stopPropagation()} role="dialog" aria-label="Claude session">
        <div className="card__head">
          <h2 className="card__title">Session · {session}</h2>
          <span className={error ? 'card__err' : 'card__badge'}>{error ? `◆ ${error}` : 'live · read-only'}</span>
          <button type="button" className="modal__close" onClick={onClose} aria-label="Close">
            ✕
          </button>
        </div>
        <pre
          ref={termRef}
          className="term"
          onScroll={onScroll}
          style={{ '--cols': data?.width || 80 } as CSSProperties}
        >
          {!data
            ? 'connecting…'
            : segments.map((s, i) => (
                <span key={i} style={s.style}>
                  {s.text}
                </span>
              ))}
        </pre>
      </div>
    </div>,
    document.body,
  )
}
