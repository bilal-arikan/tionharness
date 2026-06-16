import type { Agent, AgentPatch } from '../../types'
import { AgentSettingsForm } from './AgentSettingsForm'

interface Props {
  agent: Agent
  onClose: () => void
  onSave: (patch: AgentPatch) => Promise<void>
}

// AgentSettingsModal wraps the shared AgentSettingsForm in a centered dialog.
// Used by the roster gear button (sidebar in memory/tools views).
export function AgentSettingsModal({ agent, onClose, onSave }: Props) {
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      onClick={onClose}
    >
      <div
        className="flex max-h-[88vh] w-full max-w-lg flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <AgentSettingsForm
          key={agent.id}
          agent={agent}
          onSave={onSave}
          onSaved={onClose}
          onCancel={onClose}
        />
      </div>
    </div>
  )
}
