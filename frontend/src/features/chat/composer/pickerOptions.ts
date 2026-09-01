// Option lists for the composer's per-turn pickers (see ComposerPicker).

export interface PickerOption {
  value: string
  label: string
  hint: string
  icon?: string
  // When true the entry is shown greyed and non-selectable in the menu (e.g. a
  // reasoning tier the current model can't honour); `hint` carries the reason.
  disabled?: boolean
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
  {
    value: 'xhigh',
    label: 'Çok yüksek',
    hint: 'effort xhigh — kodlama/ajan işleri (güncel modeller)',
    icon: '◉',
  },
  { value: 'max', label: 'Maks', hint: 'effort max — en zor işler (güncel modeller)', icon: '✦' },
  { value: 'ultra', label: 'Ultra', hint: 'effort ultra — Codex GPT-5 ailesi', icon: '✹' },
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
