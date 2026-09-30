import type { CSSProperties } from 'react'

// Minimal ANSI SGR → styled segments, enough for `tmux capture-pane -e` output
// (16/256/truecolor, bold, dim, italic, underline, inverse). Other escape
// sequences are dropped.

export interface Segment {
  text: string
  style: CSSProperties
}

interface Pen {
  fg?: string
  bg?: string
  bold?: boolean
  dim?: boolean
  italic?: boolean
  underline?: boolean
  inverse?: boolean
}

// Base 16 colours, tuned a little warmer to sit on the dashboard's dark glass.
const BASE16 = [
  '#1c1410', '#e5604d', '#67c98a', '#e0a800', '#6c9fd8', '#c585c7', '#6fc0b8', '#d8cbb5',
  '#6b5d4f', '#ff7a66', '#8ee6a8', '#f5c84c', '#8fb9f0', '#e0a3e2', '#95e0d8', '#fff4e2',
]

function color256(n: number): string {
  if (n < 16) return BASE16[n]
  if (n < 232) {
    const i = n - 16
    const v = (c: number) => (c === 0 ? 0 : 55 + c * 40)
    return `rgb(${v(Math.floor(i / 36))},${v(Math.floor(i / 6) % 6)},${v(i % 6)})`
  }
  const g = 8 + (n - 232) * 10
  return `rgb(${g},${g},${g})`
}

// Parses an extended colour starting at groups[i] (38/48), returning the colour
// and how many extra `;`-separated groups it consumed.
function extColor(groups: string[][], i: number): [string | undefined, number] {
  const g = groups[i]
  if (g.length > 1) {
    // Colon form: 38:5:n or 38:2:[colorspace]:r:g:b
    if (g[1] === '5') return [color256(Number(g[2])), 0]
    if (g[1] === '2') {
      const [r, gg, b] = g.slice(-3).map(Number)
      return [`rgb(${r},${gg},${b})`, 0]
    }
    return [undefined, 0]
  }
  const mode = groups[i + 1]?.[0]
  if (mode === '5') return [color256(Number(groups[i + 2]?.[0])), 2]
  if (mode === '2') {
    const [r, gg, b] = [2, 3, 4].map((k) => Number(groups[i + k]?.[0]))
    return [`rgb(${r},${gg},${b})`, 4]
  }
  return [undefined, 0]
}

function applySgr(pen: Pen, params: string): Pen {
  const groups = (params === '' ? '0' : params).split(';').map((p) => p.split(':'))
  const next = { ...pen }
  for (let i = 0; i < groups.length; i++) {
    const code = Number(groups[i][0] || 0)
    if (code === 0) Object.keys(next).forEach((k) => delete next[k as keyof Pen])
    else if (code === 1) next.bold = true
    else if (code === 2) next.dim = true
    else if (code === 3) next.italic = true
    else if (code === 4) next.underline = groups[i][1] !== '0'
    else if (code === 7) next.inverse = true
    else if (code === 22) next.bold = next.dim = false
    else if (code === 23) next.italic = false
    else if (code === 24) next.underline = false
    else if (code === 27) next.inverse = false
    else if (code >= 30 && code <= 37) next.fg = BASE16[code - 30]
    else if (code >= 90 && code <= 97) next.fg = BASE16[code - 90 + 8]
    else if (code >= 40 && code <= 47) next.bg = BASE16[code - 40]
    else if (code >= 100 && code <= 107) next.bg = BASE16[code - 100 + 8]
    else if (code === 39) next.fg = undefined
    else if (code === 49) next.bg = undefined
    else if (code === 38 || code === 48) {
      const [c, used] = extColor(groups, i)
      if (code === 38) next.fg = c
      else next.bg = c
      i += used
    } else if (code === 58) i += extColor(groups, i)[1] // underline colour: skip
  }
  return next
}

function toStyle(p: Pen): CSSProperties {
  const s: CSSProperties = {}
  let fg = p.fg
  let bg = p.bg
  if (p.inverse) {
    ;[fg, bg] = [bg ?? 'var(--term-bg)', fg ?? 'var(--term-fg)']
  }
  if (fg) s.color = fg
  if (bg) s.backgroundColor = bg
  if (p.bold) s.fontWeight = 700
  if (p.dim) s.opacity = 0.6
  if (p.italic) s.fontStyle = 'italic'
  if (p.underline) s.textDecoration = 'underline'
  return s
}

// OSC (e.g. hyperlinks) and any non-SGR CSI sequences.
const STRIP = /\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b\[[0-9;:?]*[A-Za-ln-z]/g
const SGR = /\x1b\[([0-9;:]*)m/g

export function parseAnsi(input: string): Segment[] {
  const text = input.replace(STRIP, '')
  const out: Segment[] = []
  let pen: Pen = {}
  let last = 0
  for (const m of text.matchAll(SGR)) {
    if (m.index > last) out.push({ text: text.slice(last, m.index), style: toStyle(pen) })
    pen = applySgr(pen, m[1])
    last = m.index + m[0].length
  }
  if (last < text.length) out.push({ text: text.slice(last), style: toStyle(pen) })
  return out
}
