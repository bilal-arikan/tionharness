import type { PillOption } from '@/shared/components/OptionPills'
import { i18next } from '@/i18n'

const tx = (key: string, options?: Record<string, unknown>) =>
  i18next.t(key, { ns: 'agents', ...options })

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
  {
    value: 'off',
    get label() {
      return tx('options.thinking.off.label')
    },
    get hint() {
      return tx('options.thinking.off.hint')
    },
    icon: '○',
  },
  {
    value: 'low',
    get label() {
      return tx('options.thinking.low.label')
    },
    get hint() {
      return tx('options.thinking.low.hint')
    },
    icon: '◔',
  },
  {
    value: 'medium',
    get label() {
      return tx('options.thinking.medium.label')
    },
    get hint() {
      return tx('options.thinking.medium.hint')
    },
    icon: '◑',
  },
  {
    value: 'high',
    get label() {
      return tx('options.thinking.high.label')
    },
    get hint() {
      return tx('options.thinking.high.hint')
    },
    icon: '●',
  },
  {
    value: 'xhigh',
    get label() {
      return tx('options.thinking.xhigh.label')
    },
    get hint() {
      return tx('options.thinking.xhigh.hint')
    },
    icon: '◉',
  },
  {
    value: 'max',
    get label() {
      return tx('options.thinking.max.label')
    },
    get hint() {
      return tx('options.thinking.max.hint')
    },
    icon: '✦',
  },
  {
    value: 'ultra',
    get label() {
      return tx('options.thinking.ultra.label')
    },
    get hint() {
      return tx('options.thinking.ultra.hint')
    },
    icon: '✹',
  },
]

// Tool-use permission mode (the agent's own default).
export const PERMISSION_OPTIONS: PillOption[] = [
  {
    value: 'auto',
    get label() {
      return tx('options.permission.auto.label')
    },
    get hint() {
      return tx('options.permission.auto.hint')
    },
    icon: '⚡',
  },
  {
    value: 'ask',
    get label() {
      return tx('options.permission.ask.label')
    },
    get hint() {
      return tx('options.permission.ask.hint')
    },
    icon: '✋',
  },
  {
    value: 'read-only',
    get label() {
      return tx('options.permission.readOnly.label')
    },
    get hint() {
      return tx('options.permission.readOnly.hint')
    },
    icon: '🔒',
  },
]

// Two-state agent settings use the same always-visible selection language as
// permission mode. Keep string tokens at the visual-control boundary; persisted
// agent fields remain booleans.
export const BOOLEAN_OPTIONS: PillOption[] = [
  {
    value: 'on',
    get label() {
      return tx('options.boolean.on.label')
    },
    get hint() {
      return tx('options.boolean.on.hint')
    },
    icon: '●',
  },
  {
    value: 'off',
    get label() {
      return tx('options.boolean.off.label')
    },
    get hint() {
      return tx('options.boolean.off.hint')
    },
    icon: '○',
  },
]

export function booleanFromOption(value: string): boolean {
  if (value === 'on') return true
  if (value === 'off') return false
  throw new Error(tx('options.unknownBoolean', { value }))
}
