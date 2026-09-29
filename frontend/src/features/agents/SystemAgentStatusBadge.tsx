import { Lock } from 'lucide-react'
import type { Agent } from '@/types'
import { useTranslation } from 'react-i18next'

interface Props {
  agent: Agent
}

// SystemAgentStatusBadge explains a system agent's place in the role
// resolution at a glance:
//  - a LOCKED built-in gets a lock glyph (values fixed in the app);
//  - an enabled customisation (System && !Locked) is the row that serves the
//    role right now;
//  - a disabled customisation is dormant — the built-in serves the role.
export function SystemAgentStatusBadge({ agent }: Props) {
  const { t } = useTranslation('agents')
  if (!agent.system) return null
  if (agent.locked) {
    return (
      <span
        data-testid="system-agent-locked-badge"
        className="ml-1.5 inline-flex shrink-0 items-center gap-0.5 rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--color-text-dim)]"
        title={t('status.builtinTitle')}
      >
        <Lock size={9} /> {t('status.builtin')}
      </span>
    )
  }
  if (agent.disabled) {
    return (
      <span
        data-testid="system-agent-fallback-badge"
        className="ml-1.5 shrink-0 rounded bg-[color-mix(in_srgb,var(--color-warning)_18%,transparent)] px-1.5 py-0.5 text-[10px] text-[var(--color-warning)]"
        title={t('status.fallbackTitle')}
      >
        {t('status.fallback')}
      </span>
    )
  }
  return (
    <span
      data-testid="system-agent-customization-badge"
      className="ml-1.5 shrink-0 rounded bg-[color-mix(in_srgb,var(--color-accent)_18%,transparent)] px-1.5 py-0.5 text-[10px] text-[var(--color-accent)]"
      title={t('status.servingTitle', { role: agent.systemKey })}
    >
      {t('status.serving')}
    </span>
  )
}
