import type { ReactNode } from 'react'

export function Card({
  title,
  badge,
  error,
  children,
}: {
  title: string
  badge?: string
  error: string | null
  children: ReactNode
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
