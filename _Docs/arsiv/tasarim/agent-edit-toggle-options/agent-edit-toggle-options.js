const colors = {
  canvas: '#0D1117',
  panel: '#161B22',
  surface: '#21262D',
  border: '#30363D',
  accent: '#2F81F7',
  accentSoft: '#1F3A5F',
  text: '#F0F6FC',
  dim: '#8B949E',
}

const solid = (color) => [{ type: 'solid', color }]

const text = (content, fontSize, color = colors.text, fontWeight = 400) => ({
  type: 'text',
  content,
  fontFamily: 'Inter',
  fontSize,
  fontWeight,
  fill: solid(color),
})

const pill = (icon, label, active = false) => ({
  type: 'frame',
  role: 'button',
  layout: 'horizontal',
  alignItems: 'center',
  gap: 7,
  padding: [8, 12],
  width: 'fit_content',
  height: 'fit_content',
  cornerRadius: 8,
  fill: solid(active ? colors.accentSoft : colors.panel),
  stroke: {
    thickness: 1,
    fill: solid(active ? colors.accent : colors.border),
  },
  children: [
    text(icon, 15, active ? colors.text : colors.dim, 500),
    text(label, 14, active ? colors.text : colors.dim, 600),
  ],
})

const optionGroup = (label, hint, options) => ({
  type: 'frame',
  layout: 'vertical',
  width: 'fill_container',
  height: 'fit_content',
  gap: 9,
  children: [
    text(label, 12, colors.dim, 600),
    {
      type: 'frame',
      layout: 'horizontal',
      width: 'fill_container',
      height: 'fit_content',
      gap: 7,
      children: options,
    },
    text(hint, 12, colors.dim, 400),
  ],
})

I(null, {
  type: 'frame',
  name: 'Agent edit — option pills',
  x: 0,
  y: 0,
  width: 900,
  height: 620,
  layout: 'vertical',
  gap: 24,
  padding: 32,
  fill: solid(colors.canvas),
  children: [
    {
      type: 'frame',
      layout: 'vertical',
      width: 'fill_container',
      height: 'fit_content',
      gap: 6,
      children: [
        text('Ajan ayarları — seçim kalıpları', 24, colors.text, 700),
        text(
          'Boolean ayarlar da izin modu gibi her zaman görünür, iki seçenekli pill kontrolü kullanır.',
          13,
          colors.dim,
        ),
      ],
    },
    {
      type: 'frame',
      name: 'Settings panel',
      layout: 'vertical',
      width: 'fill_container',
      height: 'fill_container',
      gap: 22,
      padding: 24,
      cornerRadius: 12,
      fill: solid(colors.panel),
      stroke: { thickness: 1, fill: solid(colors.border) },
      children: [
        optionGroup('İzin modu (araç kullanımı)', 'Tüm araçlar onaysız çalışır.', [
          pill('⚡', 'Otomatik', true),
          pill('✋', 'Sor'),
          pill('🔒', 'Salt-okunur'),
        ]),
        optionGroup(
          'Sağlayıcının kendi web araması',
          'CLI sağlayıcısının yerleşik web aramasını açar veya kapatır.',
          [pill('●', 'Açık', true), pill('○', 'Kapalı')],
        ),
        optionGroup(
          'Koordinatör',
          'Ajanın açtığı yeni oturumların koordinatör olarak başlayıp başlamayacağını seçer.',
          [pill('●', 'Açık', true), pill('○', 'Kapalı')],
        ),
      ],
    },
  ],
})
