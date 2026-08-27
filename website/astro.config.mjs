// @ts-check
import { defineConfig } from 'astro/config'
import tailwindcss from '@tailwindcss/vite'

// Static-only promo site. No adapter, no SSR: `astro build` emits plain files
// into dist/ so any static host (Cloudflare Pages, Caddy, GitHub Pages) works.
export default defineConfig({
  output: 'static',
  // Keep in sync with `site.url` in src/site.config.ts -- this file cannot import
  // the TypeScript config, so the canonical origin is written twice.
  site: 'https://tionharness.com',
  vite: {
    plugins: [tailwindcss()],
  },
})
