import { useState } from 'react'
import { X } from 'lucide-react'
import type { Agent } from '@/types'
import { useOutsideClick } from '@/shared/hooks/useOutsideClick'
import { AgentIdentity } from './AgentIdentity'
import { archivedAgentLabel, pickableAgents } from './pickableAgents'
import { useTranslation } from 'react-i18next'

interface Props {
  agents: Agent[]
  value: string
  onChange: (id: string) => void
  placeholder?: string
  // When true, a selected agent can be cleared back to "" — via a ✕ affordance
  // in the trigger and a "clear" option at the top of the list. Use it wherever
  // the assignment is optional (e.g. board tasks, flow agent nodes).
  clearable?: boolean
}

// AgentPicker is a custom dropdown that, unlike a native <select>, renders each
// agent's circular avatar (custom emoji or derived initials) next to its name —
// both in the trigger and the option list. Closes on outside click. Archived
// agents are never offered as a new choice; a currently-selected archived agent
// stays visible, marked "(arşivli)", so editing a record does not drop it.
export function AgentPicker({ agents, value, onChange, placeholder, clearable = false }: Props) {
  const { t } = useTranslation('sharedUi')
  const resolvedPlaceholder = placeholder ?? t('agents.select')
  const [open, setOpen] = useState(false)
  const rootRef = useOutsideClick<HTMLDivElement>(() => setOpen(false), open)
  const options = pickableAgents(agents, value)
  const selected = options.find((a) => a.id === value)
  const archivedMark = (a: Agent) =>
    a.archived ? (
      <span data-testid="agent-picker-archived" className="ml-1 text-[var(--color-text-dim)]">
        {archivedAgentLabel()}
      </span>
    ) : undefined

  const clear = () => {
    onChange('')
    setOpen(false)
  }

  return (
    <div ref={rootRef} className="relative">
      <button
        type="button"
        data-testid="agent-picker-trigger"
        onClick={() => setOpen((v) => !v)}
        className="flex min-w-40 items-center gap-2 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none hover:border-[var(--color-accent)]"
      >
        {selected ? (
          <AgentIdentity
            agent={selected}
            size="sm"
            subtitle="model"
            nameSuffix={archivedMark(selected)}
          />
        ) : (
          <span className="text-[var(--color-text-dim)]">{resolvedPlaceholder}</span>
        )}
        {clearable && selected ? (
          <span
            role="button"
            tabIndex={0}
            data-testid="agent-picker-clear"
            title={t('selection.remove')}
            aria-label={t('selection.remove')}
            onClick={(e) => {
              e.stopPropagation()
              clear()
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                e.stopPropagation()
                clear()
              }
            }}
            className="ml-auto flex h-5 w-5 items-center justify-center rounded text-[var(--color-text-dim)] transition hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
          >
            <X size={13} />
          </span>
        ) : (
          <span className="ml-auto text-[var(--color-text-dim)]">{'▾'}</span>
        )}
      </button>
      {open && (
        <div className="absolute z-20 mt-1 max-h-60 w-full min-w-44 overflow-y-auto rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] py-1 shadow-lg">
          {clearable && selected && (
            <button
              type="button"
              data-testid="agent-picker-clear-option"
              onClick={clear}
              className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm text-[var(--color-text-dim)] hover:bg-[var(--color-bg)]"
            >
              <X size={14} />
              {t('selection.remove')}
            </button>
          )}
          {options.length === 0 && (
            <div className="px-3 py-2 text-sm text-[var(--color-text-dim)]">
              {t('agents.empty')}
            </div>
          )}
          {options.map((a) => (
            <button
              key={a.id}
              type="button"
              data-testid="agent-picker-option"
              data-agent-id={a.id}
              onClick={() => {
                onChange(a.id)
                setOpen(false)
              }}
              className={`flex w-full items-center px-3 py-1.5 text-left text-sm hover:bg-[var(--color-bg)] ${
                a.id === value ? 'bg-[var(--color-accent-soft)]' : ''
              }`}
            >
              <AgentIdentity agent={a} size="sm" subtitle="model" nameSuffix={archivedMark(a)} />
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
