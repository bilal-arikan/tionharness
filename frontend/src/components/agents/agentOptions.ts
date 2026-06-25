import type { PillOption } from '../common/OptionPills'

// Option lists for the agent profile's planning / thinking / permission pickers.
// These describe the agent's OWN stored setting (no "agent default" entry — the
// agent IS the default), but reuse the same icon language as the composer's
// per-turn pickers (see composer/pickerOptions.ts) for visual consistency.

// Planning mode: how much the agent plans before acting.
export const PLANNING_OPTIONS: PillOption[] = [
  { value: 'standard', label: 'Standart', hint: 'Doğrudan yürütme', icon: '📋' },
  { value: 'deep', label: 'Derin', hint: 'Önce ayrıntılı planlama', icon: '🧭' },
]

// Thinking (extended reasoning) level. Empty string = off (matches storage).
// Icons form an intensity ramp: ○ off · ◔ low · ◑ medium · ● high.
export const THINKING_OPTIONS: PillOption[] = [
  { value: '', label: 'Kapalı', hint: 'Düşünme yok', icon: '○' },
  { value: 'low', label: 'Düşük', hint: '~2K token', icon: '◔' },
  { value: 'medium', label: 'Orta', hint: '~8K token', icon: '◑' },
  { value: 'high', label: 'Yüksek', hint: '~16K token', icon: '●' },
]

// Tool-use permission mode (the agent's own default).
export const PERMISSION_OPTIONS: PillOption[] = [
  { value: 'auto', label: 'Otomatik', hint: 'Tüm araçlar onaysız çalışır', icon: '⚡' },
  { value: 'ask', label: 'Sor', hint: 'Yazma/komut için onay iste', icon: '✋' },
  { value: 'read-only', label: 'Salt-okunur', hint: 'Yazma/komut engellenir', icon: '🔒' },
]
