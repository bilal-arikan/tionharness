// Renders review stills of one flow composition at 40% scale into out/stills/<id>/.
//   node scripts/flow-stills.mjs FlowsOverview 60,150,300
import { bundle } from '@remotion/bundler'
import { renderStill, selectComposition } from '@remotion/renderer'
import { mkdirSync, rmSync } from 'node:fs'
import path from 'node:path'

const id = process.argv[2]
const frames = process.argv[3].split(',').map(Number)
const dir = `out/stills/${id}`
rmSync(dir, { recursive: true, force: true })
mkdirSync(dir, { recursive: true })
const serveUrl = await bundle({ entryPoint: path.resolve('src/index.ts') })
const composition = await selectComposition({ serveUrl, id })
for (const [i, frame] of frames.entries()) {
  await renderStill({ composition, serveUrl, frame, output: `${dir}/${String(i).padStart(2, '0')}.jpg`, imageFormat: 'jpeg', scale: 0.4 })
}
console.log(dir)
