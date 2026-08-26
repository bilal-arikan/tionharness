// @ts-check
import { defineConfig } from 'astro/config'
import tailwindcss from '@tailwindcss/vite'

// Static-only promo site. No adapter, no SSR: `astro build` emits plain files
// into dist/ so any static host (Cloudflare Pages, Caddy, GitHub Pages) works.
export default defineConfig({
  output: 'static',
  // TODO(placeholder): set once a public domain exists, so canonical/og URLs resolve.
  // site: 'https://tionharness.dev',
  vite: {
    plugins: [tailwindcss()],
  },
})
