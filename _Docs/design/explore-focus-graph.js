// OpenPencil source for TSK487 Explore focus graph mockup.
const solid = (color) => [{ type: 'solid', color }]
const stroke = (color, thickness = 1) => ({ thickness, fill: solid(color) })

const screen = I(null, {
  type: 'frame',
  name: 'Explore — Focus graph',
  x: 0,
  y: 0,
  width: 1440,
  height: 900,
  fill: solid('#0B1020'),
  layout: 'vertical',
})

const header = I(screen, {
  type: 'frame', name: 'Header', width: 'fill_container', height: 64,
  fill: solid('#10182B'), stroke: stroke('#24314A'),
  layout: 'horizontal', alignItems: 'center', gap: 18, padding: [0, 24],
})
I(header, { type: 'text', content: '⌁  Harita', fontSize: 17, fontWeight: 700, fill: solid('#EAF0FF') })
I(header, { type: 'text', content: 'Odak çevresinde ara…', fontSize: 12, fill: solid('#7E8BA7') })
I(header, { type: 'frame', width: 'fill_container', height: 1 })
I(header, { type: 'text', content: 'Yenile', fontSize: 12, fontWeight: 600, fill: solid('#9CB8FF') })

const body = I(screen, { type: 'frame', name: 'Body', width: 'fill_container', height: 'fill_container', layout: 'horizontal' })
const canvas = I(body, { type: 'frame', name: 'Focus canvas', width: 1060, height: 'fill_container', fill: solid('#0B1020'), layout: 'none' })
const panel = I(body, { type: 'frame', name: 'Existing detail panel', width: 380, height: 'fill_container', fill: solid('#10182B'), stroke: stroke('#24314A'), layout: 'vertical', padding: 24, gap: 16 })

const label = (content, x) => I(canvas, { type: 'text', content, x, y: 36, fontSize: 11, fontWeight: 700, fill: solid('#66738E') })
label('ÜST BAĞLANTILAR', 80); label('ODAK', 484); label('ALT BAĞLANTILAR', 790)

function card({ name, meta, x, y, focus = false, selected = false, color = '#6D8DFF' }) {
  const frame = I(canvas, {
    type: 'frame', name, x, y, width: focus ? 252 : 218, height: focus ? 94 : 72,
    fill: solid(focus ? '#18284B' : '#121B30'),
    stroke: stroke(selected ? '#F6B94A' : focus ? '#7DA2FF' : '#2A3853', focus ? 2 : 1),
    cornerRadius: 12, layout: 'vertical', padding: [12, 14], gap: 8,
  })
  I(frame, { type: 'text', content: `${focus ? '◎' : '◇'}  ${name}`, fontSize: focus ? 15 : 13, fontWeight: 700, fill: solid('#F1F5FF') })
  I(frame, { type: 'text', content: meta, fontSize: 10, fill: solid(color) })
  return frame
}

card({ name: 'Ana Workspace', meta: 'workspace', x: 64, y: 194 })
card({ name: 'Planlama', meta: 'category', x: 64, y: 302, selected: true, color: '#F6B94A' })
card({ name: '+7 üst bağlantı', meta: 'Tümünü listele', x: 64, y: 410, color: '#9CB8FF' })
card({ name: 'TSK487', meta: 'ODAK · task', x: 404, y: 280, focus: true })
card({ name: 'Explorer Graph', meta: 'artifact', x: 774, y: 154, color: '#5DD6A8' })
card({ name: 'Coder oturumu', meta: 'session', x: 774, y: 262, color: '#A78BFA' })
card({ name: 'TSK487', meta: 'cycle · iki yönlü', x: 774, y: 370, color: '#F6B94A' })
card({ name: '+12 alt bağlantı', meta: 'Tümünü listele', x: 774, y: 478, color: '#9CB8FF' })

const edge = (x, y, w, dashed = false, caption = '') => {
  I(canvas, { type: 'line', x, y, x2: w, y2: 0, stroke: { thickness: 2, fill: solid(dashed ? '#F6B94A' : '#526887'), dashPattern: dashed ? [7, 6] : undefined } })
  if (caption) I(canvas, { type: 'text', content: caption, x: x + w / 2 - 35, y: y - 20, fontSize: 10, fontWeight: 700, fill: solid('#F6B94A') })
}
edge(282, 230, 122); edge(282, 338, 122); edge(656, 190, 118); edge(656, 298, 118); edge(656, 406, 118, true, '↔ cycle')

I(panel, { type: 'text', content: 'Seçili öğe', fontSize: 11, fontWeight: 700, fill: solid('#7E8BA7') })
I(panel, { type: 'text', content: 'Planlama', fontSize: 20, fontWeight: 700, fill: solid('#F2F5FF') })
I(panel, { type: 'text', content: 'category:planning', fontSize: 11, fill: solid('#9CB8FF') })
I(panel, { type: 'text', content: 'Tek tık seçimi bu panelde gösterir.\nÇift tık yeni odağı ortalar.', fontSize: 12, fill: solid('#A9B4CA') })
I(panel, { type: 'frame', width: 'fill_container', height: 1, fill: solid('#24314A') })
I(panel, { type: 'text', content: 'Özet\n\nİlişkiler ve mevcut get_view içeriği\nburada korunur.', fontSize: 13, fill: solid('#D5DCEC') })
