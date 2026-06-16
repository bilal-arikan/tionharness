import { useEffect, useRef, useState } from 'react'
import { api } from '../api'

export interface NewWorkspaceData {
  name: string
  path?: string
  icon?: string
}

interface Props {
  onCreate: (data: NewWorkspaceData) => void
  onClose: () => void
}

// A small palette of starter emojis for the workspace identity.
const ICONS = ['⬡', '🚀', '🧪', '📦', '🛠', '🎯', '🌙', '🔬', '🏗', '🧠']

// WorkspaceCreateModal is the popup dialog for creating a new workspace: name,
// an optional data folder (native picker or manual path), and an emoji identity.
export function WorkspaceCreateModal({ onCreate, onClose }: Props) {
  const [name, setName] = useState('')
  const [path, setPath] = useState('')
  const [icon, setIcon] = useState('⬡')
  const [picking, setPicking] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const nameRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    nameRef.current?.focus()
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose])

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
    onCreate({ name: name.trim(), path: path.trim() || undefined, icon })
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      onMouseDown={onClose}
    >
      <div
        className="w-full max-w-md rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-5 shadow-2xl"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <h2 className="mb-4 text-base font-semibold">Yeni Workspace</h2>

        {/* Name */}
        <label className="mb-1 block text-xs text-[var(--color-text-dim)]">Ad</label>
        <input
          ref={nameRef}
          value={name}
          onChange={(e) => setName(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && submit()}
          placeholder="ör. Müşteri Projesi"
          className="mb-4 w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm outline-none focus:border-[var(--color-accent)]"
        />

        {/* Icon */}
        <label className="mb-1 block text-xs text-[var(--color-text-dim)]">Simge</label>
        <div className="mb-4 flex flex-wrap gap-1">
          {ICONS.map((ic) => (
            <button
              key={ic}
              onClick={() => setIcon(ic)}
              className={`flex h-8 w-8 items-center justify-center rounded-lg border text-base transition ${
                icon === ic
                  ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)]'
                  : 'border-[var(--color-border)] hover:bg-[var(--color-surface-2)]'
              }`}
            >
              {ic}
            </button>
          ))}
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

        {error && <p className="mb-3 text-xs text-red-400">{error}</p>}

        <div className="flex justify-end gap-2">
          <button
            onClick={onClose}
            className="rounded-lg px-3 py-2 text-sm text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]"
          >
            İptal
          </button>
          <button
            onClick={submit}
            className="rounded-lg bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-white hover:opacity-90"
          >
            Oluştur
          </button>
        </div>
      </div>
    </div>
  )
}
