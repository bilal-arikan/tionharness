import type { FlowNodeType } from '@/types'
import { i18next } from '@/i18n'

// Short, user-facing explanation of every flow node type, shown by the (ⓘ)
// button next to each palette entry. Wording mirrors the engine semantics in
// internal/orchestration/model.go — keep the two in sync when node behaviour
// changes.
export const NODE_TYPE_HELP = new Proxy({} as Record<FlowNodeType, string>, {
  get: (_target, type: string) => i18next.t(`nodeHelp.${type}`, { ns: 'flows' }),
})

export function nodeTypeHelp(type: FlowNodeType): string {
  return i18next.t(`nodeHelp.${type}`, { ns: 'flows' })
}
