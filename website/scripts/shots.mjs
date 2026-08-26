// Captures product screenshots for the promo site.
//
// Point it at a running TionHarness instance (dev server or the single binary)
// and it walks the hash routes, shooting each view at a fixed viewport into
// website/public/shots/. Screenshot.astro picks the files up automatically the
// next time the site builds; until then it renders placeholder frames.
//
// Usage (via scripts\shots.ps1, or directly):
//   node website/scripts/shots.mjs --base http://127.0.0.1:5173 [--workspace WS1]

import { mkdir } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'
import { chromium } from 'playwright'

const HERE = dirname(fileURLToPath(import.meta.url))
const OUT_DIR = join(HERE, '..', 'public', 'shots')

const VIEWPORT = { width: 2560, height: 1440 }
/** Wait after navigation so SSE-driven panels have painted. */
const SETTLE_MS = 2500

// name -> app view slug (see frontend/src/app/url.ts). The names must match the
// `shot` paths in website/src/content/deepdives.ts.
const TARGETS = [
  { name: 'coordinator', view: 'chat' },
  { name: 'flows', view: 'flows' },
  { name: 'board', view: 'board' },
  { name: 'agents', view: 'agents' },
  { name: 'insights', view: 'insights' },
]

function parseArgs(argv) {
  const args = { base: 'http://127.0.0.1:5173', workspace: null }
  for (let i = 0; i < argv.length; i += 2) {
    const key = argv[i]?.replace(/^--/, '')
    const value = argv[i + 1]
    if (key && value && key in args) args[key] = value
  }
  return args
}

// The hash route is workspace-scoped, so a shot of the wrong workspace is a shot
// of an empty app. Resolve the first one from the API unless told otherwise.
async function resolveWorkspace(base, explicit) {
  if (explicit) return explicit
  const res = await fetch(`${base}/api/workspaces`)
  if (!res.ok) throw new Error(`GET /api/workspaces failed: ${res.status}`)
  const body = await res.json()
  const list = Array.isArray(body) ? body : (body.workspaces ?? [])
  const first = list[0]
  if (!first?.id) throw new Error('no workspace returned by /api/workspaces')
  return first.id
}

async function main() {
  const { base, workspace: explicitWorkspace } = parseArgs(process.argv.slice(2))
  const workspace = await resolveWorkspace(base, explicitWorkspace)
  console.log(`base=${base} workspace=${workspace}`)

  await mkdir(OUT_DIR, { recursive: true })

  const browser = await chromium.launch()
  const page = await browser.newPage({
    viewport: VIEWPORT,
    colorScheme: 'dark',
    deviceScaleFactor: 1,
  })

  try {
    for (const target of TARGETS) {
      const url = `${base}/#/w/${workspace}/${target.view}`
      await page.goto(url, { waitUntil: 'networkidle' })
      await page.waitForTimeout(SETTLE_MS)

      const file = join(OUT_DIR, `${target.name}.png`)
      await page.screenshot({ path: file })
      console.log(`shot ${target.name} -> ${file}`)
    }
  } finally {
    await browser.close()
  }
}

await main()
