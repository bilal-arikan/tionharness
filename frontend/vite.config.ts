import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// During dev, proxy API calls to the Go backend on :8090 (SWARMGO_ADDR default
// in the run docs — :8080 collides with unity-mcp's HTTP backend).
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      // 127.0.0.1 (not "localhost") so the proxy hits the IPv4 address the Go
      // backend binds — on Windows "localhost" resolves to ::1 first and 502s.
      '/api': 'http://127.0.0.1:8090',
      '/health': 'http://127.0.0.1:8090',
    },
  },
  build: {
    // Emit straight into the Go package that embeds it (//go:embed all:dist in
    // internal/web), so `npm run build` + `go build` yields one binary that
    // serves the UI. emptyOutDir is required because the target is outside root.
    outDir: '../internal/web/dist',
    emptyOutDir: true,
  },
})
