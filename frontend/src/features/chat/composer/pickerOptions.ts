// Option lists for the composer's per-turn pickers (see ComposerPicker).
import { i18next } from '@/i18n'

const tx = (key: string) => i18next.t(key, { ns: 'chatControls' })

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
  {
    value: '',
    get label() {
      return tx('picker.thinking.auto.label')
    },
    get hint() {
      return tx('picker.thinking.auto.hint')
    },
    icon: '◌',
  },
  {
    value: 'off',
    get label() {
      return tx('picker.thinking.off.label')
    },
    get hint() {
      return tx('picker.thinking.off.hint')
    },
    icon: '○',
  },
  {
    value: 'low',
    get label() {
      return tx('picker.thinking.low.label')
    },
    get hint() {
      return tx('picker.thinking.low.hint')
    },
    icon: '◔',
  },
  {
    value: 'medium',
    get label() {
      return tx('picker.thinking.medium.label')
    },
    get hint() {
      return tx('picker.thinking.medium.hint')
    },
    icon: '◑',
  },
  {
    value: 'high',
    get label() {
      return tx('picker.thinking.high.label')
    },
    get hint() {
      return tx('picker.thinking.high.hint')
    },
    icon: '●',
  },
  {
    value: 'xhigh',
    get label() {
      return tx('picker.thinking.xhigh.label')
    },
    get hint() {
      return tx('picker.thinking.xhigh.hint')
    },
    icon: '◉',
  },
  {
    value: 'max',
    get label() {
      return tx('picker.thinking.max.label')
    },
    get hint() {
      return tx('picker.thinking.max.hint')
    },
    icon: '✦',
  },
  {
    value: 'ultra',
    get label() {
      return tx('picker.thinking.ultra.label')
    },
    get hint() {
      return tx('picker.thinking.ultra.hint')
    },
    icon: '✹',
  },
]

// Permission modes offered in the composer picker / Shift+Tab cycle. '' defers
// to the agent's own PermissionMode; the rest override it for the turn (see
// chatReq.PermissionMode). Order is the Shift+Tab cycle order.
export const PERMISSION_OPTIONS: PickerOption[] = [
  {
    value: '',
    get label() {
      return tx('picker.permission.default.label')
    },
    get hint() {
      return tx('picker.permission.default.hint')
    },
    icon: '🛡',
  },
  {
    value: 'read-only',
    get label() {
      return tx('picker.permission.readOnly.label')
    },
    get hint() {
      return tx('picker.permission.readOnly.hint')
    },
    icon: '🔒',
  },
  {
    value: 'ask',
    get label() {
      return tx('picker.permission.ask.label')
    },
    get hint() {
      return tx('picker.permission.ask.hint')
    },
    icon: '✋',
  },
  {
    value: 'auto',
    get label() {
      return tx('picker.permission.auto.label')
    },
    get hint() {
      return tx('picker.permission.auto.hint')
    },
    icon: '⚡',
  },
]
