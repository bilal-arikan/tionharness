import type { PillOption } from '@/shared/components/OptionPills'

// Option lists for the agent profile's thinking / permission pickers. These
// describe the agent's OWN stored setting (no "agent default" entry — the agent
// IS the default), but reuse the same icon language as the composer's per-turn
// pickers (see composer/pickerOptions.ts) for visual consistency.

// Thinking (extended reasoning) level. Empty string = off (matches storage).
// Icons form an intensity ramp: ○ off · ◔ low · ◑ medium · ● high · ◉ xhigh · ✦ max.
// xhigh/max map to the effort tiers of adaptive-class models (Opus 4.7/4.8,
// Sonnet 5, Fable 5); on older models they clamp down to high.
export const THINKING_OPTIONS: PillOption[] = [
  { value: '', label: 'Kapalı', hint: 'Düşünme yok', icon: '○' },
  { value: 'low', label: 'Düşük', hint: '~2K token / effort low', icon: '◔' },
  { value: 'medium', label: 'Orta', hint: '~8K token / effort medium', icon: '◑' },
  { value: 'high', label: 'Yüksek', hint: '~16K token / effort high', icon: '●' },
  { value: 'xhigh', label: 'Çok yüksek', hint: 'effort xhigh — kodlama/ajan işleri (güncel modeller)', icon: '◉' },
  { value: 'max', label: 'Maks', hint: 'effort max — en zor işler (güncel modeller)', icon: '✦' },
]

// Tool-use permission mode (the agent's own default).
export const PERMISSION_OPTIONS: PillOption[] = [
  { value: 'auto', label: 'Otomatik', hint: 'Tüm araçlar onaysız çalışır', icon: '⚡' },
  { value: 'ask', label: 'Sor', hint: 'Yazma/komut için onay iste', icon: '✋' },
  { value: 'read-only', label: 'Salt-okunur', hint: 'Yazma/komut engellenir', icon: '🔒' },
]
