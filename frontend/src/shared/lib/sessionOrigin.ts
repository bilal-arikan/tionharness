// sessionOrigin — human wording for "who started this session and from where"
// (SessionOrigin, R1). One place so the session inspector, the sidebar and the
// Rota tooltips agree. Returns null for a plain user-started session: there is
// nothing to announce.
import type { SessionOrigin } from '@/types/session'

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
  const via = trigger ? ` · tetikleyen ${trigger}` : ''
  switch (o.kind) {
    case 'automation':
      return {
        text: `Otomasyon ${o.entityId || ''} tetikledi`.replace('  ', ' '),
        title: `Bu oturumu bir otomasyon açtı${o.entityId ? ` (${o.entityId})` : ''}${via}`,
        sessionId: trigger,
        entityId: o.entityId,
      }
    case 'schedule':
      return {
        text: `Zamanlayıcı ${o.entityId || ''} başlattı`.replace('  ', ' '),
        title: `Bu oturumu zamanlanmış bir çalıştırma açtı${o.entityId ? ` (${o.entityId})` : ''}`,
        entityId: o.entityId,
      }
    case 'flow': {
      const bits = [o.runId ? `koşu ${o.runId}` : '', o.nodeId ? `düğüm ${o.nodeId}` : ''].filter(
        Boolean,
      )
      return {
        text: `Akış ${o.entityId || ''}${bits.length ? ` · ${bits.join(' · ')}` : ''}`.replace(
          '  ',
          ' ',
        ),
        title: `Bu oturum bir akış koşusuna ait${o.entityId ? ` (${o.entityId})` : ''}${bits.length ? ` · ${bits.join(', ')}` : ''}${via}`,
        sessionId: trigger,
        entityId: o.entityId,
      }
    }
    case 'coordinator':
      return {
        text: `Koordinatör ${trigger || ''} açtı`.replace('  ', ' '),
        title: `Bu oturum bir koordinatörün worker'ı olarak açıldı${o.rootSessionId ? ` · kök ${o.rootSessionId}` : ''}`,
        sessionId: trigger,
      }
    case 'subagent':
      return {
        text: `Alt-ajan · ${trigger || 'üst tur'}`,
        title: 'Bu oturum bir run_subagent çağrısıyla açıldı',
        sessionId: trigger,
      }
    case 'handoff':
      return {
        text: `Devir · ${trigger || ''}`.replace(' · ', trigger ? ' · ' : ''),
        title: 'Bu oturum bir context-reset (handoff) ile önceki oturumdan devraldı',
        sessionId: trigger,
      }
    case 'spawn':
      return {
        text: `Oturum ${trigger || ''} başlattı`.replace('  ', ' '),
        title: 'Bu oturum spawn_session ile başka bir oturumdan başlatıldı',
        sessionId: trigger,
      }
    case 'insight':
      return {
        text: `İçgörü koşusu${o.runId ? ` ${o.runId}` : ''}`,
        title: 'Bu oturum bir içgörü (insight) koşusuna ait',
        sessionId: trigger,
      }
    default:
      return { text: String(o.kind), title: `Köken: ${o.kind}`, sessionId: trigger }
  }
}
