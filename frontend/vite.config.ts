import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// During dev, proxy API calls to the Go backend on :8080.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      '/api': 'http://localhost:8090',
      '/health': 'http://localhost:8090',
    },
  },
  build: {
    outDir: 'dist',
  },
})
