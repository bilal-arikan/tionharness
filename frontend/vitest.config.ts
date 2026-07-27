import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vitest/config'

// Unit tests for the pure-logic modules under src/ (rule engines, registries,
// formatters). Deliberately separate from vite.config.ts: the test run needs
// neither the React plugin, Tailwind, the dev proxy, nor the embed-into-Go build
// output, and pulling those in only slows the run down and couples it to the
// bundle config.
//
// Scope: `node` environment, no DOM. Component tests would need jsdom +
// @testing-library; add that (and switch `environment`) when the first one lands.
export default defineConfig({
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  test: {
    environment: 'node',
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
  },
})
