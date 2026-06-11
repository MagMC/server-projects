import { usePolling } from './usePolling'
import { bytes, milliCores, pct, uptime } from './format'
import type { ContainerInfo, HostStats, K3sData } from './types'
import './App.css'

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
  error,
  children,
}: {
  title: string
  error: string | null
  children: React.ReactNode
}) {
  return (
    <article className="card">
      <div className="card__head">
        <h2 className="card__title">{title}</h2>
        {error && <span className="card__err" title={error}>stale</span>}
      </div>
      {children}
    </article>
  )
}

function HostPanel() {
  const { data, error } = usePolling<HostStats>('/api/host')
  return (
    <Card title="Host" error={error}>
      {!data ? (
        <p className="muted">loading…</p>
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
          <div className="host__foot">
            <span>
              CPU temp <strong>{data.cpuTempC != null ? `${data.cpuTempC}°C` : '—'}</strong>
            </span>
            <span>
              up <strong>{uptime(data.uptimeSec)}</strong>
            </span>
          </div>
          <div className="chips">
            {data.thermalZones.map((z) => (
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
    <Card title={`Docker${data ? ` · ${data.length}` : ''}`} error={error}>
      {!data ? (
        <p className="muted">loading…</p>
      ) : (
        <div className="rows">
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
    <Card title={`k3s${data ? ` · ${data.pods.length} pods` : ''}`} error={error}>
      {!data ? (
        <p className="muted">loading…</p>
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
    <main className="app">
      <header className="app__header">
        <h1>Server Dashboard</h1>
        <span className="app__host">server .72</span>
      </header>
      <section className="grid">
        <HostPanel />
        <DockerPanel />
        <K3sPanel />
      </section>
    </main>
  )
}

export default App
