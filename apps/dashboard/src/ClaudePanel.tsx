import { useEffect, useLayoutEffect, useMemo, useRef, useState, type CSSProperties } from 'react'
import { createPortal } from 'react-dom'
import { Card } from './Card'
import { parseAnsi } from './ansi'
import { ago, uptime } from './format'
import { usePolling } from './usePolling'
import { useToken } from './token'
import type { ClaudeConversation, ClaudeConversationDetail, ClaudeScreen, ClaudeStatus } from './types'

function stateOf(s: ClaudeStatus): { label: string; dot: string } {
  if (!s.available) return { label: 'tmux down', dot: 'dot dot--bad' }
  if (s.running) return { label: 'Running', dot: 'dot dot--ok' }
  if (s.exists) return { label: 'Shell idle', dot: 'dot dot--warn' }
  return { label: 'Off', dot: 'dot' }
}

// One panel per tmux session the API is configured with. The token is shared,
// so it's held here rather than in each panel.
export function ClaudePanels() {
  const { data, error, refresh } = usePolling<ClaudeStatus[]>('/api/claude/status')
  const tokenState = useToken()
  if (!data) {
    return (
      <Card title="Claude" error={error}>
        <p className="muted">syncing…</p>
      </Card>
    )
  }
  return (
    <>
      {data.map((s) => (
        <ClaudePanel key={s.session} data={s} error={error} refresh={refresh} tokenState={tokenState} />
      ))}
    </>
  )
}

function ClaudePanel({
  data,
  error,
  refresh,
  tokenState,
}: {
  data: ClaudeStatus
  error: string | null
  refresh: () => void
  tokenState: ReturnType<typeof useToken>
}) {
  const [token, ensureToken, forgetToken] = tokenState
  const api = `/api/claude/${encodeURIComponent(data.session)}`
  const [busy, setBusy] = useState(false)
  const [actionErr, setActionErr] = useState<string | null>(null)
  const [viewing, setViewing] = useState(false)
  const [conversation, setConversation] = useState<string | null>(null)
  // What Claude is doing lives in its transcripts, not the tmux pane (which for
  // a remote-control session only shows the bridge's status line).
  const convs = usePolling<ClaudeConversation[]>(
    token && data.controlEnabled ? `${api}/conversations` : null,
    5000,
    token,
  )
  const badToken = convs.error === 'HTTP 401'
  useEffect(() => {
    if (badToken) forgetToken()
    // forgetToken is a new function every render; only the 401 matters.
  }, [badToken])

  const toggle = async () => {
    if (busy) return
    const stopping = data.running
    if (stopping && !window.confirm(`Stop Claude? This kills the tmux session "${data.session}".`)) return
    const t = ensureToken()
    if (!t) return
    setBusy(true)
    setActionErr(null)
    try {
      const res = await fetch(`${api}/${stopping ? 'stop' : 'start'}`, {
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
    if (data.exists && ensureToken()) setViewing(true)
  }

  const state = stateOf(data)
  const canControl = data.available && data.controlEnabled

  return (
    <Card title="Claude" error={error} badge={`tmux · ${data.session}`}>
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

        {data.controlEnabled &&
          (token ? (
            <ConversationList items={convs.data} onOpen={setConversation} />
          ) : (
            <button type="button" className="claude__unlock" onClick={ensureToken}>
              Show activity ›
            </button>
          ))}

        {(actionErr || data.error || !data.controlEnabled) && (
          <p className="claude__err">
            {actionErr ?? data.error ?? 'Controls disabled: CLAUDE_CONTROL_TOKEN not set'}
          </p>
        )}
      </div>
      {viewing && (
        <SessionViewer
          session={data.session}
          url={`${api}/screen`}
          token={token}
          onClose={() => setViewing(false)}
          onBadToken={forgetToken}
        />
      )}
      {conversation && (
        <ConversationViewer
          url={`${api}/conversations/${conversation}`}
          token={token}
          onClose={() => setConversation(null)}
          onBadToken={forgetToken}
        />
      )}
    </Card>
  )
}

function ConversationList({
  items,
  onOpen,
}: {
  items: ClaudeConversation[] | null
  onOpen: (id: string) => void
}) {
  if (!items) return <p className="muted claude__empty">loading activity…</p>
  if (items.length === 0) return <p className="muted claude__empty">No conversations yet</p>
  return (
    <ul className="convs">
      {items.map((c) => (
        <li key={c.id}>
          <button type="button" className="conv" onClick={() => onOpen(c.id)}>
            <span className="conv__head">
              <span className={c.busy ? 'dot dot--ok dot--pulse' : 'dot'} />
              <span className="conv__title">{c.title || 'Untitled'}</span>
              <span className="conv__when">{c.busy ? 'working' : ago(c.updatedAt)}</span>
            </span>
            {c.last && <span className="conv__last">{c.last}</span>}
          </button>
        </li>
      ))}
    </ul>
  )
}

// Shared by both viewers: close on Escape or a bad token, and keep a scrolling
// element pinned to its newest output unless the user scrolled up to read.
function useViewer<T extends HTMLElement>(error: string | null, onClose: () => void, onBadToken: () => void, dep: unknown) {
  const ref = useRef<T>(null)
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
    const el = ref.current
    if (el && stick.current) el.scrollTop = el.scrollHeight
  }, [dep])

  const onScroll = () => {
    const el = ref.current
    if (el) stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40
  }
  return { ref, onScroll }
}

function clock(iso: string): string {
  const d = new Date(iso)
  return isNaN(d.getTime()) ? '' : d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function ConversationViewer({
  url,
  token,
  onClose,
  onBadToken,
}: {
  url: string
  token: string
  onClose: () => void
  onBadToken: () => void
}) {
  const { data, error } = usePolling<ClaudeConversationDetail>(url, 2000, token)
  const { ref, onScroll } = useViewer<HTMLDivElement>(error, onClose, onBadToken, data)

  return createPortal(
    <div className="modal" onClick={onClose}>
      <div className="modal__box" onClick={(e) => e.stopPropagation()} role="dialog" aria-label="Claude conversation">
        <div className="card__head">
          <h2 className="card__title feed__title">{data?.title || 'Conversation'}</h2>
          <span className={error ? 'card__err' : 'card__badge'}>
            {error ? `◆ ${error}` : !data ? 'loading' : data.busy ? 'working' : `idle · ${ago(data.updatedAt)}`}
          </span>
          <button type="button" className="modal__close" onClick={onClose} aria-label="Close">
            ✕
          </button>
        </div>
        <div ref={ref} className="feed" onScroll={onScroll}>
          {!data ? (
            <p className="muted">connecting…</p>
          ) : (
            data.events.map((e, i) => (
              <div key={i} className={`feed__item feed__item--${e.kind}${e.failed ? ' feed__item--failed' : ''}`}>
                <span className="feed__time">{clock(e.at)}</span>
                {e.kind === 'tool' ? (
                  <span className="feed__body">
                    <span className="feed__tool">{e.tool}</span> {e.text}
                    {e.failed && <span className="feed__fail"> · failed</span>}
                  </span>
                ) : (
                  <span className="feed__body">{e.text}</span>
                )}
              </div>
            ))
          )}
          {data?.busy && <p className="feed__working">working…</p>}
        </div>
      </div>
    </div>,
    document.body,
  )
}

function SessionViewer({
  session,
  url,
  token,
  onClose,
  onBadToken,
}: {
  session: string
  url: string
  token: string
  onClose: () => void
  onBadToken: () => void
}) {
  const { data, error } = usePolling<ClaudeScreen>(token ? url : null, 1500, token)
  const segments = useMemo(() => (data ? parseAnsi(data.text) : []), [data])
  const { ref: termRef, onScroll } = useViewer<HTMLPreElement>(error, onClose, onBadToken, segments)

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
