import { useState } from 'react'

// The control token (Claude and VPN controls) lives only in this browser; the
// API checks it. Storage can be unavailable (private mode), so every access is
// guarded.
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

export function useToken(): [string, () => string, () => void] {
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
