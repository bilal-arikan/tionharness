// Renders a contact sheet of stills per format for quick visual review.
import { bundle } from '@remotion/bundler'
import { renderStill, selectComposition } from '@remotion/renderer'
import path from 'node:path'

const frames = (process.argv[3] ?? '60,100,170,250,350,440,530,620,700,800,870,930,990,1060').split(',').map(Number)
const ids = (process.argv[2] ?? 'Horizontal,Vertical,Square').split(',')
const serveUrl = await bundle({ entryPoint: path.resolve('src/index.ts') })
for (const id of ids) {
  const composition = await selectComposition({ serveUrl, id })
  for (const frame of frames) {
    await renderStill({ composition, serveUrl, frame, output: `out/stills/${id}-${String(frame).padStart(4, '0')}.jpg`, imageFormat: 'jpeg', scale: 0.4 })
  }
  console.log('done', id)
}
