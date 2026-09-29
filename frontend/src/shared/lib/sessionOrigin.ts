// sessionOrigin — human wording for "who started this session and from where"
// (SessionOrigin, R1). One place so the session inspector, the sidebar and the
// Rota tooltips agree. Returns null for a plain user-started session: there is
// nothing to announce.
import type { SessionOrigin } from '@/types/session'
import { sharedText } from './sharedI18n'

export interface OriginLabel {
  // Short chip text, e.g. "Otomasyon AUT4 tetikledi".
  text: string
  // Longer explanation for a tooltip.
  title: string
  // Session the chip can jump to (the trigger), if any.
  sessionId?: string
  // Entity the origin names (automation / schedule / flow id), if any.
  entityId?: string
}

export function originLabel(o?: SessionOrigin | null): OriginLabel | null {
  if (!o || o.kind === 'user') return null
  const trigger = o.triggerSessionId || undefined
  const via = trigger ? sharedText('sessionOrigin.triggeredBy', { id: trigger }) : ''
  switch (o.kind) {
    case 'automation':
      return {
        text: sharedText('sessionOrigin.automation.text', { id: o.entityId || '' }).replace(
          '  ',
          ' ',
        ),
        title: sharedText('sessionOrigin.automation.title', {
          entity: o.entityId ? ` (${o.entityId})` : '',
          via,
        }),
        sessionId: trigger,
        entityId: o.entityId,
      }
    case 'schedule':
      return {
        text: sharedText('sessionOrigin.schedule.text', { id: o.entityId || '' }).replace(
          '  ',
          ' ',
        ),
        title: sharedText('sessionOrigin.schedule.title', {
          entity: o.entityId ? ` (${o.entityId})` : '',
        }),
        entityId: o.entityId,
      }
    case 'flow': {
      const bits = [
        o.runId ? sharedText('sessionOrigin.flow.run', { id: o.runId }) : '',
        o.nodeId ? sharedText('sessionOrigin.flow.node', { id: o.nodeId }) : '',
      ].filter(Boolean)
      return {
        text: sharedText('sessionOrigin.flow.text', {
          id: o.entityId || '',
          details: bits.length ? ` · ${bits.join(' · ')}` : '',
        }).replace('  ', ' '),
        title: sharedText('sessionOrigin.flow.title', {
          entity: o.entityId ? ` (${o.entityId})` : '',
          details: bits.length ? ` · ${bits.join(', ')}` : '',
          via,
        }),
        sessionId: trigger,
        entityId: o.entityId,
      }
    }
    case 'coordinator':
      return {
        text: sharedText('sessionOrigin.coordinator.text', { id: trigger || '' }).replace(
          '  ',
          ' ',
        ),
        title: sharedText('sessionOrigin.coordinator.title', {
          root: o.rootSessionId
            ? sharedText('sessionOrigin.coordinator.root', { id: o.rootSessionId })
            : '',
        }),
        sessionId: trigger,
      }
    case 'subagent':
      return {
        text: sharedText('sessionOrigin.subagent.text', {
          source: trigger || sharedText('sessionOrigin.subagent.parentTurn'),
        }),
        title: sharedText('sessionOrigin.subagent.title'),
        sessionId: trigger,
      }
    case 'handoff':
      return {
        text: sharedText('sessionOrigin.handoff.text', { target: trigger ? ` · ${trigger}` : '' }),
        title: sharedText('sessionOrigin.handoff.title'),
        sessionId: trigger,
      }
    case 'spawn':
      return {
        text: sharedText('sessionOrigin.spawn.text', { id: trigger || '' }).replace('  ', ' '),
        title: sharedText('sessionOrigin.spawn.title'),
        sessionId: trigger,
      }
    case 'insight':
      return {
        text: sharedText('sessionOrigin.insight.text', { run: o.runId ? ` ${o.runId}` : '' }),
        title: sharedText('sessionOrigin.insight.title'),
        sessionId: trigger,
      }
    default:
      return {
        text: String(o.kind),
        title: sharedText('sessionOrigin.unknown', { kind: o.kind }),
        sessionId: trigger,
      }
  }
}
