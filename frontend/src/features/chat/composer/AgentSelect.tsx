import { useState } from 'react'
import type { Agent } from '@/types'
import { useOutsideClick } from '@/shared/hooks/useOutsideClick'
import { AgentIdentity } from '@/shared/components/agents/AgentIdentity'
import { useTranslation } from 'react-i18next'

interface Props {
  agents: Agent[]
  value: string
  onChange: (id: string) => void
  // Disabled until a session exists (a message always targets a session's agent).
  disabled?: boolean
}

// AgentSelect is the composer's mandatory agent picker: the message is always sent
// to the chosen agent (the "@mention" routing was removed). An icon-only trigger
// (the agent's avatar; name in the tooltip) opens an upward menu listing every
// agent with avatar + name, matching the other composer pickers.
export function AgentSelect({ agents, value, onChange, disabled }: Props) {
  const { t } = useTranslation('chatControls')
  const [open, setOpen] = useState(false)
  const rootRef = useOutsideClick<HTMLDivElement>(() => setOpen(false), open)
  const selected = agents.find((a) => a.id === value)

  return (
    <div ref={rootRef} className="relative shrink-0">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        disabled={disabled}
        data-testid="agent-select"
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-label={
          selected
            ? t('agentSelect.targetAgent', { name: selected.name })
            : t('agentSelect.selectAgent')
        }
        title={
          selected
            ? t('agentSelect.messageTarget', { name: selected.name })
            : t('agentSelect.selectAgent')
        }
        className={`flex items-center gap-1.5 rounded-xl border px-2.5 py-3 text-sm transition disabled:opacity-40 ${
          selected
            ? 'border-[var(--color-accent)] text-[var(--color-text)]'
            : 'border-[var(--color-danger)] text-[var(--color-danger)]'
        }`}
      >
        {selected ? (
          <AgentIdentity
            agent={selected}
            size="sm"
            subtitle="model"
            showId
            mobileIconOnly
            className="md:max-w-[220px]"
          />
        ) : (
          <span>{t('agentSelect.selectAgent')}</span>
        )}
      </button>

      {open && (
        <div
          role="listbox"
          aria-label={t('agentSelect.sendMessage')}
          data-testid="agent-select-menu"
          className="absolute bottom-full left-0 mb-2 max-h-64 w-56 overflow-y-auto rounded-xl border border-[var(--color-border)] bg-[var(--color-surface-2)] p-1 shadow-xl"
        >
          <div className="px-2 py-1 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
            {t('agentSelect.sendMessage')}
          </div>
          {agents.length === 0 && (
            <div className="px-2 py-2 text-sm text-[var(--color-text-dim)]">
              {t('agentSelect.noAgents')}
            </div>
          )}
          {agents.map((a) => (
            <button
              key={a.id}
              role="option"
              aria-selected={a.id === value}
              data-testid="agent-option"
              data-agent-id={a.id}
              onClick={() => {
                onChange(a.id)
                setOpen(false)
              }}
              className={`flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm ${
                a.id === value
                  ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)]'
                  : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface)]'
              }`}
            >
              <AgentIdentity agent={a} size="sm" subtitle="model" showId />
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
