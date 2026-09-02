import type { PillOption } from '@/shared/components/OptionPills'

// Option lists for the agent profile's thinking / permission pickers. These
// describe the agent's OWN stored setting (no "agent default" entry — the agent
// IS the default), but reuse the same icon language as the composer's per-turn
// pickers (see composer/pickerOptions.ts) for visual consistency.

// Thinking (extended reasoning) level. Every pill carries a real backend token —
// "off" included. The empty string is NOT one of them: the backend rejects a
// blank level on the agent write path, because it used to mean "no thinking" on
// the native API but "high effort" on the CLI path.
// Icons form an intensity ramp: ○ off · ◔ low · ◑ medium · ● high · ◉ xhigh · ✦ max · ✹ ultra.
// xhigh/max map to the effort tiers of adaptive-class models (Opus 4.7/4.8,
// Sonnet 5, Fable 5); on older models they clamp down to high.
export const THINKING_OPTIONS: PillOption[] = [
  { value: 'off', label: 'Kapalı', hint: 'Düşünme yok', icon: '○' },
  { value: 'low', label: 'Düşük', hint: '~2K token / effort low', icon: '◔' },
  { value: 'medium', label: 'Orta', hint: '~8K token / effort medium', icon: '◑' },
  { value: 'high', label: 'Yüksek', hint: '~16K token / effort high', icon: '●' },
  {
    value: 'xhigh',
    label: 'Çok yüksek',
    hint: 'effort xhigh — kodlama/ajan işleri (güncel modeller)',
    icon: '◉',
  },
  { value: 'max', label: 'Maks', hint: 'effort max — en zor işler (güncel modeller)', icon: '✦' },
  { value: 'ultra', label: 'Ultra', hint: 'effort ultra — Codex GPT-5 ailesi', icon: '✹' },
]

// Tool-use permission mode (the agent's own default).
export const PERMISSION_OPTIONS: PillOption[] = [
  { value: 'auto', label: 'Otomatik', hint: 'Tüm araçlar onaysız çalışır', icon: '⚡' },
  { value: 'ask', label: 'Sor', hint: 'Yazma/komut için onay iste', icon: '✋' },
  { value: 'read-only', label: 'Salt-okunur', hint: 'Yazma/komut engellenir', icon: '🔒' },
]

// Two-state agent settings use the same always-visible selection language as
// permission mode. Keep string tokens at the visual-control boundary; persisted
// agent fields remain booleans.
export const BOOLEAN_OPTIONS: PillOption[] = [
  { value: 'on', label: 'Açık', hint: 'Etkin', icon: '●' },
  { value: 'off', label: 'Kapalı', hint: 'Devre dışı', icon: '○' },
]

export function booleanFromOption(value: string): boolean {
  if (value === 'on') return true
  if (value === 'off') return false
  throw new Error(`Bilinmeyen boolean seçenek: ${value}`)
}
