import { useState } from 'react'
import { Sparkles, FolderPlus, FolderOpen } from 'lucide-react'
import { WorkspaceCreateModal, type NewWorkspaceData } from './WorkspaceCreateModal'
import { api } from '@/api'
import { Button } from '@/shared/components'

interface Props {
  // onCreate provisions the first workspace. Once it resolves the app has an
  // active workspace and App stops rendering this screen.
  onCreate: (data: NewWorkspaceData) => void
  // onAttach adopts an EXISTING workspace data folder by absolute path. It
  // re-throws on failure (invalid folder / already attached) so we can surface
  // the message inline; on success the app switches to that workspace and this
  // screen unmounts.
  onAttach: (path: string) => Promise<unknown>
}

// OnboardingScreen is the first-run experience shown when there are ZERO
// workspaces (a fresh install — the backend no longer seeds a default one). It
// presents a choice of two paths: create a brand-new workspace (opens the create
// popup) OR select an existing workspace folder (e.g. copied from another machine
// or a prior install). Neither provisions anything until the user acts, and no
// default workspace is ever created behind their back.
export function OnboardingScreen({ onCreate, onAttach }: Props) {
  const [showModal, setShowModal] = useState(false)
  const [selecting, setSelecting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Open the native folder picker, then attach the chosen folder as a workspace.
  // Validation happens on the backend; an invalid/duplicate folder surfaces its
  // message here without leaving the onboarding screen.
  const selectExisting = async () => {
    setSelecting(true)
    setError(null)
    try {
      const { path, canceled } = await api.pickFolder()
      if (canceled || !path) return // user dismissed the dialog — no-op
      await onAttach(path)
      // Success: App re-renders into the shell; nothing more to do here.
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSelecting(false)
    }
  }

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
            TionSwarm'ya hoş geldin
          </h1>
          <p className="text-sm text-[var(--color-text-dim)]">
            Başlamak için bir workspace oluştur ya da daha önce kullandığın bir workspace klasörünü
            seç. Tüm ajanların, oturumların ve verilerin seçtiğin workspace içinde izole şekilde
            saklanır.
          </p>
        </div>

        <div className="flex flex-col items-stretch gap-2 sm:flex-row">
          <Button onClick={() => setShowModal(true)} size="lg">
            <FolderPlus size={16} className="mr-1.5 inline shrink-0" />
            Workspace Oluştur
          </Button>
          <Button
            onClick={selectExisting}
            disabled={selecting}
            size="lg"
            variant="secondary"
            data-testid="select-existing-workspace"
          >
            <FolderOpen size={16} className="mr-1.5 inline shrink-0" />
            {selecting ? 'Seçiliyor…' : 'Mevcut Workspace Seç'}
          </Button>
        </div>

        {error && (
          <p className="text-xs text-[var(--color-danger)]" data-testid="onboarding-error">
            {error}
          </p>
        )}

        <p className="text-xs text-[var(--color-text-dim)]">
          Workspace oluşturmadan ya da seçmeden devam edemezsin — varsayılan bir kurulum yapılmaz.
        </p>
      </div>

      {showModal && (
        <WorkspaceCreateModal onCreate={onCreate} onClose={() => setShowModal(false)} />
      )}
    </div>
  )
}
