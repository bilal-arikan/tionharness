import { useState } from 'react'
import { Sparkles, FolderPlus } from 'lucide-react'
import { WorkspaceCreateModal, type NewWorkspaceData } from './WorkspaceCreateModal'
import { Button } from '../common'

interface Props {
  // onCreate provisions the first workspace. Once it resolves the app has an
  // active workspace and App stops rendering this screen.
  onCreate: (data: NewWorkspaceData) => void
}

// OnboardingScreen is the first-run experience shown when there are ZERO
// workspaces (a fresh install — the backend no longer seeds a default one). It
// opens the create-workspace popup immediately. If the user dismisses the popup
// without creating one, nothing is provisioned: they fall back to this welcome
// card with a button to open the popup again. No default workspace is ever
// created behind their back.
export function OnboardingScreen({ onCreate }: Props) {
  const [showModal, setShowModal] = useState(true)

  return (
    <div
      data-testid="onboarding-screen"
      className="flex h-full w-full items-center justify-center bg-[var(--color-bg)] p-6 text-[var(--color-text)]"
    >
      <div className="flex max-w-md flex-col items-center gap-5 text-center">
        <div className="flex h-16 w-16 items-center justify-center rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] text-3xl">
          ⬡
        </div>

        <div className="flex flex-col gap-2">
          <h1 className="flex items-center justify-center gap-2 text-xl font-semibold">
            <Sparkles size={18} className="text-[var(--color-accent)]" />
            SwarmGo'ya hoş geldin
          </h1>
          <p className="text-sm text-[var(--color-text-dim)]">
            Başlamak için bir workspace oluştur. Tüm ajanların, oturumların ve verilerin
            seçtiğin workspace içinde izole şekilde saklanır.
          </p>
        </div>

        <Button onClick={() => setShowModal(true)} size="lg">
          <FolderPlus size={16} className="mr-1.5 inline shrink-0" />
          Workspace Oluştur
        </Button>

        <p className="text-xs text-[var(--color-text-dim)]">
          Workspace oluşturmadan devam edemezsin — varsayılan bir kurulum yapılmaz.
        </p>
      </div>

      {showModal && (
        <WorkspaceCreateModal onCreate={onCreate} onClose={() => setShowModal(false)} />
      )}
    </div>
  )
}
