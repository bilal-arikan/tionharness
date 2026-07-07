import type { Agent, AgentPatch } from '@/types'
import { AgentSettingsForm } from './AgentSettingsForm'
import { ModalOverlay } from '@/shared/components'

interface Props {
  agent: Agent
  onClose: () => void
  onSave: (patch: AgentPatch) => Promise<void>
}

// AgentSettingsModal wraps the shared AgentSettingsForm in a centered dialog.
// Used by the roster gear button (sidebar in tools views).
export function AgentSettingsModal({ agent, onClose, onSave }: Props) {
  return (
    <ModalOverlay onClose={onClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Ajan ayarları"
        data-testid="agent-settings-modal"
        className="flex max-h-[88vh] w-full max-w-lg flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-xl"
      >
        <AgentSettingsForm
          key={agent.id}
          agent={agent}
          onSave={onSave}
          onSaved={onClose}
          onCancel={onClose}
        />
      </div>
    </ModalOverlay>
  )
}
