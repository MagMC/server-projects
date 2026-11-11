import { useEffect, useState } from 'react'
import { Card } from './Card'
import { ago } from './format'
import { usePolling } from './usePolling'
import { useToken } from './token'
import type { VpnContainer, VpnJobStep, VpnStatus } from './types'

function liveDot(c: VpnContainer): string {
  if (c.health === 'healthy') return 'dot dot--ok'
  if (c.health === 'unhealthy') return 'dot dot--bad'
  if (c.health === 'starting') return 'dot dot--warn'
  if (c.state === 'running') return 'dot dot--ok'
  if (c.state === 'exited' || c.state === 'dead') return 'dot dot--bad'
  return 'dot dot--warn'
}

function liveLabel(c: VpnContainer): string {
  return c.state === 'running' && c.health ? c.health : c.state
}

function stepDot(s: VpnJobStep): string {
  if (s.state === 'ok') return 'dot dot--ok'
  if (s.state === 'failed') return 'dot dot--bad'
  if (s.state === 'restarting' || s.state === 'waiting') return 'dot dot--warn dot--pulse'
  return 'dot'
}

// gluetun plus every container sharing its network. The button restarts
// gluetun, waits for it to be healthy, then restarts the rest so they rejoin
// its new network namespace (they lose their network otherwise).
export function VpnPanel() {
  const [fast, setFast] = useState(false)
  const { data, error, refresh } = usePolling<VpnStatus>('/api/vpn', fast ? 2000 : 5000)
  const [token, ensureToken, forgetToken] = useToken()
  const [busy, setBusy] = useState(false)
  const [actionErr, setActionErr] = useState<string | null>(null)

  const job = data?.job
  const running = job?.state === 'running'
  useEffect(() => setFast(running), [running])

  const restart = async () => {
    if (busy || running || !data) return
    const names = data.dependents.map((d) => d.name).join(', ') || 'none'
    if (!window.confirm(`Restart gluetun, then once it's healthy: ${names}?`)) return
    const t = token || ensureToken()
    if (!t) return
    setBusy(true)
    setActionErr(null)
    try {
      const res = await fetch('/api/vpn/restart', {
        method: 'POST',
        headers: { Authorization: `Bearer ${t}` },
      })
      if (res.status === 401) forgetToken()
      if (!res.ok) {
        const body = (await res.json().catch(() => null)) as { error?: string } | null
        throw new Error(body?.error ?? `HTTP ${res.status}`)
      }
      setFast(true)
    } catch (e) {
      setActionErr(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
      refresh()
    }
  }

  if (!data) {
    return (
      <Card title="VPN" error={error}>
        <p className="muted">syncing…</p>
      </Card>
    )
  }

  const g = data.gluetun
  const containers = g ? [g, ...data.dependents] : data.dependents
  // While a restart runs, each row shows where it is in the restart instead.
  const steps = new Map<string, VpnJobStep>(running && job ? job.steps.map((s) => [s.name, s] as const) : [])
  const failedSteps = job?.state === 'failed' ? job.steps.filter((s) => s.state === 'failed' && s.note) : []

  return (
    <Card title="VPN" error={error} badge={g ? `gluetun · ${liveLabel(g)}` : 'gluetun missing'}>
      <div className="vpn">
        <div className="rows">
          {containers.map((c) => {
            const s = steps.get(c.name)
            return (
              <div className="row" key={c.name}>
                <span className={s ? stepDot(s) : liveDot(c)} />
                <span className="row__name">
                  {c.name}
                  {c === g && <span className="vpn__tag">tunnel</span>}
                </span>
                <span className="vpn__state">{s ? s.state : liveLabel(c)}</span>
              </div>
            )
          })}
        </div>

        <button
          type="button"
          className="vpn__btn"
          onClick={restart}
          disabled={!g || !data.controlEnabled || busy || running}
        >
          {running ? 'Restarting…' : busy ? 'Starting…' : 'Restart VPN stack'}
        </button>

        {job && job.state !== 'idle' && (
          <p className={`vpn__job vpn__job--${job.state}`}>
            {running
              ? job.step
              : `Last restart ${job.state === 'done' ? 'OK' : 'failed'}${job.finishedAt ? ` · ${ago(job.finishedAt)}` : ''}${job.state === 'failed' ? ` — ${job.step}` : ''}`}
          </p>
        )}
        {failedSteps.map((s) => (
          <p className="claude__err" key={s.name}>
            {s.name}: {s.note}
          </p>
        ))}
        {(actionErr || data.error || !data.controlEnabled) && (
          <p className="claude__err">
            {actionErr ?? data.error ?? 'Controls disabled: CLAUDE_CONTROL_TOKEN not set'}
          </p>
        )}
      </div>
    </Card>
  )
}
