import { readdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { gzipSync } from 'node:zlib'
import type { Plugin } from 'vite'

export function precompressAssets(): Plugin {
  let output = ''
  return {
    name: 'tionharness-precompress',
    apply: 'build',
    configResolved(config) {
      output = config.build.outDir
    },
    closeBundle() {
      const visit = (dir: string) => {
        for (const entry of readdirSync(dir, { withFileTypes: true })) {
          const file = join(dir, entry.name)
          if (entry.isDirectory()) visit(file)
          else if (/\.(js|css|html|svg|json)$/.test(entry.name)) {
            const original = readFileSync(file)
            const compressed = gzipSync(original, { level: 9 })
            if (compressed.length < original.length) writeFileSync(`${file}.gz`, compressed)
          }
        }
      }
      visit(output)
    },
  }
}
