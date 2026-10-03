import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vitest/config'

// Unit tests for the pure-logic modules under src/ (rule engines, registries,
// formatters). Deliberately separate from vite.config.ts: the test run needs
// neither the React plugin, Tailwind, the dev proxy, nor the embed-into-Go build
// output, and pulling those in only slows the run down and couples it to the
// bundle config.
//
// Pure logic uses the node environment; component tests opt into jsdom with
// their file-level environment directive.
export default defineConfig({
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  test: {
    environment: 'node',
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
  },
})
