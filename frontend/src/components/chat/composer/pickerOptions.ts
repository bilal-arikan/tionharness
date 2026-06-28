// Option lists for the composer's per-turn pickers (see ComposerPicker).

export interface PickerOption {
  value: string
  label: string
  hint: string
  icon?: string
}

// Reasoning levels offered in the composer picker. '' defers to the agent's own
// ThinkingLevel; the rest override it for the turn (see chatReq.ThinkingLevel).
// Icons form an intensity ramp so the current reasoning level is readable at a
// glance from the composer's icon-only trigger: ◌ auto · ○ off · ◔ low · ◑ medium · ● high.
export const THINKING_OPTIONS: PickerOption[] = [
  { value: '', label: 'Oto', hint: 'Ajanın kendi ayarı', icon: '◌' },
  { value: 'off', label: 'Kapalı', hint: 'Düşünme yok', icon: '○' },
  { value: 'low', label: 'Düşük', hint: 'Kısa akıl yürütme', icon: '◔' },
  { value: 'medium', label: 'Orta', hint: 'Dengeli', icon: '◑' },
  { value: 'high', label: 'Yüksek', hint: 'Derin akıl yürütme', icon: '●' },
]

// Permission modes offered in the composer picker / Shift+Tab cycle. '' defers
// to the agent's own PermissionMode; the rest override it for the turn (see
// chatReq.PermissionMode). Order is the Shift+Tab cycle order.
export const PERMISSION_OPTIONS: PickerOption[] = [
  { value: '', label: 'Oto', hint: 'Ajanın kendi izin ayarı', icon: '🛡' },
  { value: 'read-only', label: 'Salt-okunur', hint: 'Yazma/komut engellenir', icon: '🔒' },
  { value: 'ask', label: 'Sor', hint: 'Yazma/komut için onay iste', icon: '✋' },
  { value: 'auto', label: 'Otomatik', hint: 'Tüm araçlar onaysız', icon: '⚡' },
]
