// Placeholder monitoring shell for the iPad dashboard.
// Data source (Docker / k3s / metrics) is intentionally not wired up yet —
// these panels are static stand-ins for what this server will report.

type Panel = {
  title: string
  hint: string
}

const PANELS: Panel[] = [
  { title: 'Host', hint: 'CPU · memory · disk · uptime' },
  { title: 'Docker', hint: 'running containers · status · image' },
  { title: 'k3s', hint: 'pods · namespaces · restarts' },
]

function App() {
  return (
    <main className="app">
      <header className="app__header">
        <h1>Server Dashboard</h1>
        <span className="app__host">server .72</span>
      </header>

      <section className="grid">
        {PANELS.map((p) => (
          <article key={p.title} className="card">
            <h2 className="card__title">{p.title}</h2>
            <p className="card__hint">{p.hint}</p>
            <p className="card__status">no data source connected yet</p>
          </article>
        ))}
      </section>
    </main>
  )
}

export default App
