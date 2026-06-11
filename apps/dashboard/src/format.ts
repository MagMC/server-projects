export function bytes(n: number | null | undefined): string {
  if (n == null) return '—'
  if (n === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  const i = Math.min(units.length - 1, Math.floor(Math.log(n) / Math.log(1024)))
  const v = n / Math.pow(1024, i)
  return `${v.toFixed(v >= 100 || i === 0 ? 0 : 1)} ${units[i]}`
}

export function pct(n: number | null | undefined): string {
  return n == null ? '—' : `${n.toFixed(n >= 10 ? 0 : 1)}%`
}

export function milliCores(m: number | null | undefined): string {
  return m == null ? '—' : `${(m / 1000).toFixed(2)} cores`
}

// Friendly names for Linux thermal-zone kernel types.
export function thermalLabel(type: string): string {
  const t = type.toLowerCase()
  if (t === 'x86_pkg_temp' || t.startsWith('coretemp')) return 'CPU'
  if (t === 'acpitz') return 'Board'
  if (t.startsWith('pch')) return 'Chipset'
  if (t.startsWith('iwlwifi') || t.includes('wifi') || t.includes('wlan')) return 'Wi-Fi'
  if (t.startsWith('nvme')) return 'SSD'
  if (t.includes('gpu') || t.startsWith('amdgpu')) return 'GPU'
  return type
}

export function uptime(sec: number): string {
  if (!sec) return '—'
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  if (d > 0) return `${d}d ${h}h`
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}
