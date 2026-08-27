import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { fileURLToPath, URL } from 'node:url'
import { defineConfig, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// internal/web/dist/.gitkeep is a tracked file (.gitignore ignores the dist
// contents but whitelists this one) because //go:embed all:dist in
// internal/web/embed.go fails to compile when the directory does not exist —
// on a fresh clone .gitkeep is what creates it. emptyOutDir wipes the whole
// directory on every build, so restore the placeholder once the bundle is out.
function keepDistPlaceholder(): Plugin {
  const gitkeep = fileURLToPath(new URL('../internal/web/dist/.gitkeep', import.meta.url))
  // Snapshot the tracked bytes now, while the file still exists — the config is
  // evaluated before Vite empties outDir. Restoring the exact content (BOM and
  // all) is what keeps the build out of `git status`. A previous build may have
  // wiped it already, in which case fall back to the canonical text.
  let content: Buffer | string
  try {
    content = readFileSync(gitkeep)
  } catch {
    content =
      '﻿Frontend build placeholder. Real assets are emitted here by ' +
      "'npm run build' (vite outDir) and embedded via go:embed. Do not delete this file."
  }
  return {
    name: 'tionharness-keep-dist-gitkeep',
    apply: 'build',
    closeBundle() {
      mkdirSync(fileURLToPath(new URL('../internal/web/dist', import.meta.url)), {
        recursive: true,
      })
      writeFileSync(gitkeep, content)
    },
  }
}

// During dev, proxy API calls to the Go backend on :8090 (TIONHARNESS_ADDR default
// in the run docs — :8080 collides with unity-mcp's HTTP backend).
export default defineConfig({
  plugins: [react(), tailwindcss(), keepDistPlaceholder()],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
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
    // The only chunk above the default 500 kB limit is relationGraph
    // (vis-network) — a monolithic vendor lib that is already lazy-loaded only
    // when a graph view opens, so it never weighs on the initial bundle.
    chunkSizeWarningLimit: 600,
    rollupOptions: {
      output: {
        // Split heavy vendor libraries out of the main app bundle so no single
        // chunk trips Vite's 500 kB warning. The graph libs (vis-network,
        // @xyflow) are already lazy-loaded as their own route chunks.
        manualChunks(id: string) {
          if (!id.includes('node_modules')) return
          if (id.includes('highlight.js')) return 'vendor-highlight'
          // NOTE: do NOT hand-group mermaid + its deps (d3, dagre, cytoscape, …)
          // into a named chunk. MermaidDiagram.tsx already `await import('mermaid')`s
          // it, so the bundler splits it out on its own. Forcing a manual group made
          // the shared dynamic-import preload helper land inside that 3 MB chunk,
          // which turned it into a static dependency of the entry — index.html then
          // modulepreloaded all of mermaid on every cold start, defeating the lazy
          // load entirely. Leave the dynamic import to do the splitting.
          if (
            id.includes('react-markdown') ||
            id.includes('remark') ||
            id.includes('rehype') ||
            id.includes('micromark') ||
            id.includes('mdast') ||
            id.includes('hast') ||
            id.includes('unist') ||
            id.includes('unified') ||
            id.includes('property-information') ||
            id.includes('hastscript') ||
            id.includes('vfile')
          )
            return 'vendor-markdown'
          if (id.includes('react-dom') || id.includes('/react/') || id.includes('scheduler'))
            return 'vendor-react'
        },
      },
    },
  },
})
