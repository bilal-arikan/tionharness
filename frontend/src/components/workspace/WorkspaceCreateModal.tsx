import { useEffect, useRef, useState } from 'react'
import { api } from '../../api'
import type { WorkspaceTemplate } from '../../types'
import { EmojiField } from '../common/EmojiField'
import { Button, ModalOverlay } from '../common'

export interface NewWorkspaceData {
  name: string
  path?: string
  icon?: string
  template?: string
}

interface Props {
  onCreate: (data: NewWorkspaceData) => void
  onClose: () => void
}

// WorkspaceCreateModal is the popup dialog for creating a new workspace: name,
// an optional data folder (native picker or manual path), and an emoji identity.
export function WorkspaceCreateModal({ onCreate, onClose }: Props) {
  const [name, setName] = useState('')
  const [path, setPath] = useState('')
  const [icon, setIcon] = useState('⬡')
  const [picking, setPicking] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [templates, setTemplates] = useState<WorkspaceTemplate[]>([])
  // Empty until templates load; the load effect auto-selects the blank default so
  // a valid market template id is always submitted.
  const [templateId, setTemplateId] = useState('')
  const nameRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    nameRef.current?.focus()
  }, [])

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
      if (!canceled && chosen) setPath(chosen)
    } catch (e) {
      setError('Klasör seçici açılamadı — yolu elle yazabilirsin. (' + (e as Error).message + ')')
    } finally {
      setPicking(false)
    }
  }

  const submit = () => {
    if (!name.trim()) {
      setError('Workspace adı gerekli')
      nameRef.current?.focus()
      return
    }
    onCreate({ name: name.trim(), path: path.trim() || undefined, icon, template: templateId })
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

        {/* Folder */}
        <label className="mb-1 block text-xs text-[var(--color-text-dim)]">
          Veri klasörü <span className="opacity-60">(opsiyonel — boşsa varsayılan konum)</span>
        </label>
        <div className="mb-4 flex gap-1">
          <input
            value={path}
            onChange={(e) => setPath(e.target.value)}
            placeholder="C:\Users\...\workspaces"
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

        {error && <p className="mb-3 text-xs text-[var(--color-danger)]">{error}</p>}

        <div className="flex justify-end gap-2">
          <button
            onClick={onClose}
            className="rounded-lg px-3 py-2 text-sm text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]"
          >
            İptal
          </button>
          <Button onClick={submit} size="lg">
            Oluştur
          </Button>
        </div>
      </div>
    </ModalOverlay>
  )
}
