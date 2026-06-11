import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    // Reachable from the iPad on the LAN, not just localhost.
    host: true,
    // In dev, forward /api to the backend (run it locally on :3001) so the
    // single-origin fetch('/api/...') calls work the same as behind nginx.
    proxy: {
      '/api': {
        target: 'http://localhost:3001',
        changeOrigin: true,
        rewrite: (p) => p.replace(/^\/api/, ''),
      },
    },
  },
})
