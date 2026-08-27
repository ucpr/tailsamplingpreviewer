import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// The Preview Server has no CORS support (same-origin only, spec.md ss34),
// so the dev server proxies REST and WebSocket traffic to the real
// backend started separately via `go run ./cmd/tailpreview`.
const backend = 'http://127.0.0.1:17778'

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': backend,
      '/v1': {
        target: backend,
        ws: true,
      },
    },
  },
})
