import { useEffect, useState } from 'react'
import { usePolling } from './usePolling'
import { bytes, milliCores, pct, uptime } from './format'
import type { ContainerInfo, HostStats, K3sData } from './types'
import './App.css'

function useClock(): Date {
  const [now, setNow] = useState(() => new Date())
  useEffect(() => {
    const id = setInterval(() => setNow(new Date()), 1000)
    return () => clearInterval(id)
  }, [])
  return now
}

function greeting(h: number): string {
  if (h < 5) return 'Good night'
  if (h < 12) return 'Good morning'
  if (h < 18) return 'Good afternoon'
  return 'Good evening'
}

function Header() {
  const now = useClock()
  const hh = now.getHours().toString().padStart(2, '0')
  const mm = now.getMinutes().toString().padStart(2, '0')
  const date = now.toLocaleDateString(undefined, {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
  })
  return (
    <header className="hero">
      <div className="hero__clock">
        {hh}
        <span className="hero__colon">:</span>
        {mm}
      </div>
      <div className="hero__meta">
        <p className="hero__greet">{greeting(now.getHours())}</p>
        <p className="hero__date">{date}</p>
      </div>
    </header>
  )
}

function Bar({ value }: { value: number | null }) {
  const v = Math.max(0, Math.min(100, value ?? 0))
  const tone = v >= 90 ? 'bar--hot' : v >= 70 ? 'bar--warn' : ''
  return (
    <div className="bar">
      <div className={`bar__fill ${tone}`} style={{ width: `${v}%` }} />
    </div>
  )
}

function Card({
  title,
  badge,
  error,
  children,
}: {
  title: string
  badge?: string
  error: string | null
  children: React.ReactNode
}) {
  return (
    <article className="card">
      <div className="card__head">
        <h2 className="card__title">{title}</h2>
        {error ? (
          <span className="card__err" title={error}>
            ◆ stale
          </span>
        ) : (
          badge && <span className="card__badge">{badge}</span>
        )}
      </div>
      {children}
    </article>
  )
}

function HostPanel() {
  const { data, error } = usePolling<HostStats>('/api/host')
  return (
    <Card
      title="Host"
      error={error}
      badge={data ? `up ${uptime(data.uptimeSec)}` : undefined}
    >
      {!data ? (
        <p className="muted">syncing…</p>
      ) : (
        <div className="host">
          <div className="metric">
            <span className="metric__label">CPU</span>
            <Bar value={data.cpuPct} />
            <span className="metric__val">{pct(data.cpuPct)}</span>
          </div>
          <div className="metric">
            <span className="metric__label">Memory</span>
            <Bar value={data.mem.usedPct} />
            <span className="metric__val">
              {bytes(data.mem.usedBytes)} / {bytes(data.mem.totalBytes)}
            </span>
          </div>
          {data.disks.map((d) => (
            <div className="metric" key={d.mount}>
              <span className="metric__label">Disk {d.mount}</span>
              <Bar value={d.usedPct} />
              <span className="metric__val">
                {bytes(d.usedBytes)} / {bytes(d.totalBytes)}
              </span>
            </div>
          ))}
          <div className="chips">
            {data.cpuTempC != null && (
              <span className="chip chip--hot">CPU {data.cpuTempC}°C</span>
            )}
            {data.thermalZones
              .filter((z) => z.type !== 'x86_pkg_temp')
              .map((z) => (
                <span className="chip" key={z.type}>
                  {z.type} {z.tempC}°
                </span>
              ))}
          </div>
        </div>
      )}
    </Card>
  )
}

function stateClass(state: string): string {
  if (state === 'running') return 'dot dot--ok'
  if (state === 'exited' || state === 'dead') return 'dot dot--bad'
  return 'dot dot--warn'
}

function DockerPanel() {
  const { data, error } = usePolling<ContainerInfo[]>('/api/docker/containers')
  return (
    <Card
      title="Docker"
      error={error}
      badge={data ? `${data.length} running` : undefined}
    >
      {!data ? (
        <p className="muted">syncing…</p>
      ) : (
        <div className="rows rows--scroll">
          {data.map((c) => (
            <div className="row" key={c.id}>
              <span className={stateClass(c.state)} title={c.status} />
              <span className="row__name" title={c.image}>
                {c.name}
              </span>
              <span className="row__num">{pct(c.cpuPct)}</span>
              <span className="row__num">{bytes(c.memUsedBytes)}</span>
            </div>
          ))}
        </div>
      )}
    </Card>
  )
}

function phaseClass(phase: string): string {
  if (phase === 'Running' || phase === 'Succeeded') return 'dot dot--ok'
  if (phase === 'Failed') return 'dot dot--bad'
  return 'dot dot--warn'
}

function K3sPanel() {
  const { data, error } = usePolling<K3sData>('/api/k3s')
  return (
    <Card
      title="k3s"
      error={error}
      badge={data ? `${data.pods.length} pods` : undefined}
    >
      {!data ? (
        <p className="muted">syncing…</p>
      ) : (
        <>
          {data.nodes.map((n) => (
            <div className="metric" key={n.name}>
              <span className="metric__label">
                <span className={n.ready ? 'dot dot--ok' : 'dot dot--bad'} /> {n.name}
              </span>
              <span className="metric__val">
                {milliCores(n.cpuMilli)} · {bytes(n.memUsedBytes)}
              </span>
            </div>
          ))}
          <div className="rows rows--scroll">
            {data.pods.map((p) => (
              <div className="row" key={`${p.namespace}/${p.name}`}>
                <span className={phaseClass(p.phase)} title={p.phase} />
                <span className="row__ns">{p.namespace}</span>
                <span className="row__name">{p.name}</span>
                <span className="row__num">{p.ready}</span>
                <span className="row__num" title="restarts">
                  ↻{p.restarts}
                </span>
              </div>
            ))}
          </div>
        </>
      )}
    </Card>
  )
}

function App() {
  return (
    <div className="app">
      <div className="app__scrim" />
      <main className="app__inner">
        <Header />
        <section className="grid">
          <HostPanel />
          <DockerPanel />
          <K3sPanel />
        </section>
        <footer className="credit">
          server .72 — art by{' '}
          <a href="https://metronovon.com" target="_blank" rel="noreferrer">
            metronovon
          </a>
        </footer>
      </main>
    </div>
  )
}

export default App
