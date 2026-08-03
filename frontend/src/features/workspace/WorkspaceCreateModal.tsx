import { useEffect, useRef, useState } from 'react'
import { api } from '@/api'
import type { GitInfo, WorkspaceTemplate } from '@/types'
import { EmojiField } from '@/shared/components/EmojiField'
import { Button, ModalOverlay } from '@/shared/components'

export interface NewWorkspaceData {
  name: string
  // Optional project directory (session working dir). The workspace data dir
  // always uses the application default location — it is not user-selectable.
  projectDir?: string
  icon?: string
  template?: string
  // Run `git init` in projectDir right after creation (see the checkbox below).
  gitInit?: boolean
}

interface Props {
  onCreate: (data: NewWorkspaceData) => void | Promise<unknown>
  onClose: () => void
}

// WorkspaceCreateModal is the popup dialog for creating a new workspace: name,
// an optional project directory (native picker or manual path — the session cwd)
// and an emoji identity. The data dir always uses the app default location.
export function WorkspaceCreateModal({ onCreate, onClose }: Props) {
  const [name, setName] = useState('')
  const [projectDir, setProjectDir] = useState('')
  const [icon, setIcon] = useState('⬡')
  const [picking, setPicking] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [templates, setTemplates] = useState<WorkspaceTemplate[]>([])
  // Empty until templates load; the load effect auto-selects the blank default so
  // a valid market template id is always submitted.
  const [templateId, setTemplateId] = useState('')
  // Optional `git init` in the project dir, plus the probed git state of that dir
  // (null while unknown/unprobed) that decides whether the option is offerable.
  const [gitInit, setGitInit] = useState(false)
  const [git, setGit] = useState<GitInfo | null>(null)
  const nameRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    nameRef.current?.focus()
  }, [])

  // Probe the typed/picked project dir: is git installed on this machine, does the
  // folder exist, is it already a repo? Debounced because it runs on every
  // keystroke of a manually typed path.
  useEffect(() => {
    const dir = projectDir.trim()
    if (!dir) {
      setGit(null)
      return
    }
    let alive = true
    const t = setTimeout(() => {
      api
        .gitInfo(dir)
        .then((g) => alive && setGit(g))
        .catch(() => alive && setGit(null)) // probe failure just hides the hint
    }, 350)
    return () => {
      alive = false
      clearTimeout(t)
    }
  }, [projectDir])

  // git init is only meaningful with a path, with git installed, and when the
  // folder is not already versioned.
  const gitAvailable = !!projectDir.trim() && git?.gitInstalled === true && !git.isGitRepo

  // Load the available templates (market workspace-kind packs) for the picker.
  useEffect(() => {
    api
      .listWorkspaceTemplates()
      .then((ts) => {
        setTemplates(ts)
        // Default-select the blank template (or the first) so a valid market id
        // is always submitted and the picker shows an initial selection.
        const def = ts.find((t) => t.id.endsWith('blank')) ?? ts[0]
        if (def) setTemplateId((prev) => prev || def.id)
      })
      .catch(() => {}) // picker just stays empty / blank-only on failure
  }, [])

  // Selecting a template adopts its icon (unless the user already picked one).
  const selectTemplate = (t: WorkspaceTemplate) => {
    setTemplateId(t.id)
    if (t.icon) setIcon(t.icon)
  }

  const browse = async () => {
    setPicking(true)
    setError(null)
    try {
      const { path: chosen, canceled } = await api.pickFolder()
      if (!canceled && chosen) setProjectDir(chosen)
    } catch (e) {
      setError('Klasör seçici açılamadı — yolu elle yazabilirsin. (' + (e as Error).message + ')')
    } finally {
      setPicking(false)
    }
  }

  const submit = async () => {
    if (busy) return
    if (!name.trim()) {
      setError('Workspace adı gerekli')
      nameRef.current?.focus()
      return
    }
    setBusy(true)
    setError(null)
    try {
      // Awaits the whole provision so the button stays in its "Oluşturuluyor…"
      // state until the parent dismisses the modal on success. On failure the
      // creation path surfaces the error itself; we release busy so the user can
      // retry without a stuck spinner.
      await onCreate({
        name: name.trim(),
        projectDir: projectDir.trim() || undefined,
        icon,
        template: templateId,
        // Only send it when the option is actually offerable, so a stale checkbox
        // (ticked, then the path changed to an existing repo) cannot leak through.
        gitInit: gitInit && gitAvailable,
      })
    } catch (e) {
      setError('Workspace oluşturulamadı: ' + (e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <ModalOverlay onClose={onClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Yeni workspace"
        data-testid="workspace-create-modal"
        className="w-full max-w-lg rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-5 shadow-2xl"
      >
        <h2 className="mb-4 text-base font-semibold">Yeni Workspace</h2>

        {/* Template */}
        {templates.length > 0 && (
          <>
            <label className="mb-1 block text-xs text-[var(--color-text-dim)]">Şablon</label>
            <div className="mb-4 grid max-h-60 grid-cols-1 gap-1.5 overflow-y-auto">
              {templates.map((t) => (
                <button
                  key={t.id}
                  onClick={() => selectTemplate(t)}
                  className={`flex items-start gap-2.5 rounded-lg border p-2.5 text-left transition ${
                    templateId === t.id
                      ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)]'
                      : 'border-[var(--color-border)] hover:bg-[var(--color-surface-2)]'
                  }`}
                >
                  <span className="mt-0.5 text-lg leading-none">{t.icon}</span>
                  <span className="min-w-0 flex-1">
                    <span className="flex items-center gap-2 text-sm font-medium">
                      {t.name}
                      <span className="text-[10px] font-normal text-[var(--color-text-dim)]">
                        {t.agentCount} ajan{t.hasFlow ? ' · akış' : ''}
                      </span>
                    </span>
                    <span className="mt-0.5 block text-xs text-[var(--color-text-dim)]">
                      {t.description}
                    </span>
                  </span>
                </button>
              ))}
            </div>
          </>
        )}

        {/* Icon + Name on one row: the icon is a compact square to the left of the
            name input (same inline pattern as the agent editor). */}
        <label className="mb-1 block text-xs text-[var(--color-text-dim)]">Simge ve ad</label>
        <div className="mb-4 flex items-center gap-2">
          <EmojiField value={icon} onChange={setIcon} clearLabel="⬡" compact />
          <input
            ref={nameRef}
            value={name}
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && submit()}
            placeholder="ör. Müşteri Projesi"
            className="flex-1 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm outline-none focus:border-[var(--color-accent)]"
          />
        </div>

        {/* Project directory (session cwd) — optional. The data dir always uses the
            app default location and is no longer user-selectable. */}
        <label className="mb-1 block text-xs text-[var(--color-text-dim)]">
          Proje dizini (path){' '}
          <span className="opacity-60">(opsiyonel — oturumların çalışma dizini)</span>
        </label>
        <div className="mb-2 flex gap-1">
          <input
            value={projectDir}
            onChange={(e) => setProjectDir(e.target.value)}
            placeholder="C:\Users\...\Desktop\Projects\my-app"
            className="flex-1 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm outline-none focus:border-[var(--color-accent)]"
          />
          <button
            onClick={browse}
            disabled={picking}
            className="shrink-0 rounded-lg border border-[var(--color-border)] px-3 text-sm hover:bg-[var(--color-surface-2)] disabled:opacity-50"
          >
            {picking ? '…' : 'Gözat'}
          </button>
        </div>

        {/* git init — offered next to the project dir so a brand-new project folder
            is under version control from the start. Disabled (with the reason) when
            there is no path, no git on the machine, or the folder is already a repo. */}
        <div className="mb-4">
          <label
            className={`flex items-center gap-2 text-xs ${
              gitAvailable ? 'cursor-pointer' : 'cursor-not-allowed opacity-60'
            }`}
          >
            <input
              type="checkbox"
              data-testid="workspace-create-git-init"
              checked={gitInit && gitAvailable}
              disabled={!gitAvailable}
              onChange={(e) => setGitInit(e.target.checked)}
              className="accent-[var(--color-accent)]"
            />
            Git deposu başlat (<code>git init</code>, dal: main, <code>.gitignore</code> ile)
          </label>
          {projectDir.trim() && git && (
            <p className="mt-1 text-[11px] text-[var(--color-text-dim)]">
              {!git.gitInstalled
                ? 'Bu bilgisayarda git bulunamadı — kurup uygulamayı yeniden başlat.'
                : git.isGitRepo
                  ? `Bu klasör zaten bir git deposu${git.branch ? ` (${git.branch})` : ''}.`
                  : git.exists
                    ? 'Klasör mevcut, henüz versiyonlanmamış.'
                    : 'Klasör yok — oluşturma sırasında açılacak.'}
            </p>
          )}
        </div>

        {error && <p className="mb-3 text-xs text-[var(--color-danger)]">{error}</p>}

        <div className="flex justify-end gap-2">
          <button
            onClick={onClose}
            disabled={busy}
            className="rounded-lg px-3 py-2 text-sm text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] disabled:opacity-50"
          >
            İptal
          </button>
          <Button onClick={submit} size="lg" disabled={busy}>
            {busy ? 'Oluşturuluyor…' : 'Oluştur'}
          </Button>
        </div>
      </div>
    </ModalOverlay>
  )
}
