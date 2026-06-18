import { useState } from 'react'
import type { BoardColumnDef } from '../../types'

// Palette of preset accent colors for quick selection.
const COLOR_PRESETS = [
  '#6b7280', // gray
  '#3b82f6', // blue
  '#8b5cf6', // violet
  '#ec4899', // pink
  '#f97316', // orange
  '#eab308', // yellow
  '#22c55e', // green
  '#14b8a6', // teal
  '#ef4444', // red
]

// Slugify a label into a safe key: lowercase, spaces/hyphens to underscores,
// strip everything else, collapse repeated underscores, trim.
function slugify(label: string): string {
  return label
    .toLowerCase()
    .replace(/[\s-]+/g, '_')
    .replace(/[^a-z0-9_]/g, '')
    .replace(/_+/g, '_')
    .replace(/^_|_$/g, '')
}

// Validate that a key is non-empty and contains only safe chars.
function isValidKey(key: string): boolean {
  return /^[a-z0-9_]+$/.test(key)
}

interface Props {
  columns: BoardColumnDef[]
  taskCountByColumn: Record<string, number>
  onSave: (columns: BoardColumnDef[]) => Promise<void>
  onClose: () => void
}

export function BoardColumnEditor({ columns, taskCountByColumn, onSave, onClose }: Props) {
  const [draft, setDraft] = useState<BoardColumnDef[]>(() =>
    columns.map((c) => ({ ...c })),
  )
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  // Index of the column whose color picker is open (-1 = none).
  const [colorPickerIdx, setColorPickerIdx] = useState(-1)
  // Index being dragged for reorder.
  const [dragIdx, setDragIdx] = useState<number | null>(null)

  const update = (idx: number, patch: Partial<BoardColumnDef>) => {
    setDraft((prev) =>
      prev.map((c, i) => (i === idx ? { ...c, ...patch } : c)),
    )
  }

  const addColumn = () => {
    const newCol: BoardColumnDef = { key: '', label: 'Yeni Sütun', color: '' }
    setDraft((prev) => [...prev, newCol])
  }

  const removeColumn = (idx: number) => {
    setDraft((prev) => prev.filter((_, i) => i !== idx))
  }

  const moveUp = (idx: number) => {
    if (idx === 0) return
    setDraft((prev) => {
      const next = [...prev]
      ;[next[idx - 1], next[idx]] = [next[idx], next[idx - 1]]
      return next
    })
  }

  const moveDown = (idx: number) => {
    setDraft((prev) => {
      if (idx >= prev.length - 1) return prev
      const next = [...prev]
      ;[next[idx], next[idx + 1]] = [next[idx + 1], next[idx]]
      return next
    })
  }

  const handleLabelChange = (idx: number, label: string) => {
    const col = draft[idx]
    // Auto-generate the key from the label only while the key looks like it
    // was auto-generated (matches the slug of the current label).
    const currentAuto = slugify(col.label)
    const autoKey = col.key === '' || col.key === currentAuto
    update(idx, {
      label,
      key: autoKey ? slugify(label) : col.key,
    })
  }

  const handleSave = async () => {
    setError('')
    const keys = new Set<string>()
    for (const col of draft) {
      if (!isValidKey(col.key)) {
        setError(`"${col.label}" için geçersiz anahtar: "${col.key}" (sadece küçük harf, rakam, alt çizgi)`)
        return
      }
      if (!col.label.trim()) {
        setError('Sütun etiketi boş bırakılamaz.')
        return
      }
      if (keys.has(col.key)) {
        setError(`Yinelenen sütun anahtarı: "${col.key}"`)
        return
      }
      keys.add(col.key)
    }
    if (draft.length === 0) {
      setError('En az bir sütun gereklidir.')
      return
    }
    setSaving(true)
    try {
      await onSave(draft)
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="flex h-full w-72 flex-shrink-0 flex-col border-r border-[var(--color-border)] bg-[var(--color-surface)]">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
        <span className="text-sm font-semibold">Sütun Düzenleyici</span>
        <button
          onClick={onClose}
          className="rounded p-1 text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
          title="Kapat"
        >
          ✕
        </button>
      </div>

      {/* Column list */}
      <div className="flex-1 overflow-y-auto p-3 space-y-2">
        {draft.map((col, idx) => {
          const taskCount = taskCountByColumn[col.key] ?? 0
          const canDelete = draft.length > 1 && taskCount === 0
          const isOpen = colorPickerIdx === idx
          return (
            <div
              key={idx}
              draggable
              onDragStart={() => setDragIdx(idx)}
              onDragOver={(e) => e.preventDefault()}
              onDrop={() => {
                if (dragIdx === null || dragIdx === idx) return
                setDraft((prev) => {
                  const next = [...prev]
                  const [moved] = next.splice(dragIdx, 1)
                  next.splice(idx, 0, moved)
                  return next
                })
                setDragIdx(null)
              }}
              className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] p-2"
            >
              {/* Row 1: drag handle + label input + color swatch + delete */}
              <div className="flex items-center gap-1.5">
                <span
                  className="cursor-grab select-none text-[var(--color-text-dim)] text-sm"
                  title="Sürükleyerek yeniden sırala"
                >
                  ⠿
                </span>
                <input
                  value={col.label}
                  onChange={(e) => handleLabelChange(idx, e.target.value)}
                  placeholder="Sütun adı"
                  className="min-w-0 flex-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-0.5 text-sm outline-none focus:border-[var(--color-accent)]"
                />
                {/* Color swatch button */}
                <div className="relative">
                  <button
                    onClick={() => setColorPickerIdx(isOpen ? -1 : idx)}
                    title="Renk seç"
                    className="h-6 w-6 flex-shrink-0 rounded border border-[var(--color-border)] hover:opacity-80"
                    style={{
                      backgroundColor: col.color || 'var(--color-surface)',
                    }}
                  />
                  {isOpen && (
                    <div className="absolute left-0 top-8 z-20 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-2 shadow-lg">
                      <div className="mb-2 grid grid-cols-3 gap-1.5">
                        {COLOR_PRESETS.map((preset) => (
                          <button
                            key={preset}
                            title={preset}
                            onClick={() => {
                              update(idx, { color: preset })
                              setColorPickerIdx(-1)
                            }}
                            className="h-6 w-6 rounded border border-[var(--color-border)] hover:scale-110 transition-transform"
                            style={{ backgroundColor: preset }}
                          />
                        ))}
                      </div>
                      {/* Custom hex input */}
                      <div className="flex items-center gap-1 mt-1">
                        <input
                          type="color"
                          value={col.color || '#6b7280'}
                          onChange={(e) => update(idx, { color: e.target.value })}
                          className="h-6 w-8 cursor-pointer rounded border-0 bg-transparent p-0"
                          title="Özel renk"
                        />
                        <input
                          value={col.color}
                          onChange={(e) => update(idx, { color: e.target.value })}
                          placeholder="#rrggbb"
                          className="w-20 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-1 py-0.5 text-xs outline-none"
                        />
                        {col.color && (
                          <button
                            onClick={() => update(idx, { color: '' })}
                            className="text-xs text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
                            title="Rengi kaldır"
                          >
                            ✕
                          </button>
                        )}
                      </div>
                    </div>
                  )}
                </div>
                {/* Delete */}
                <button
                  onClick={() => removeColumn(idx)}
                  disabled={!canDelete}
                  title={
                    taskCount > 0
                      ? `${taskCount} görev bu sütunda — önce taşıyın`
                      : draft.length <= 1
                      ? 'Son sütun silinemez'
                      : 'Sütunu sil'
                  }
                  className="flex-shrink-0 rounded p-1 text-xs text-[var(--color-text-dim)] hover:bg-red-500/10 hover:text-red-400 disabled:cursor-not-allowed disabled:opacity-30"
                >
                  🗑
                </button>
              </div>
              {/* Row 2: key input + reorder buttons */}
              <div className="mt-1.5 flex items-center gap-1">
                <span className="text-[10px] text-[var(--color-text-dim)]">anahtar:</span>
                <input
                  value={col.key}
                  onChange={(e) => update(idx, { key: e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, '') })}
                  placeholder="ornek_anahtar"
                  className={`w-32 rounded border px-1.5 py-0.5 font-mono text-[11px] outline-none ${
                    col.key && !isValidKey(col.key)
                      ? 'border-red-400 bg-red-400/10'
                      : 'border-[var(--color-border)] bg-[var(--color-bg)] focus:border-[var(--color-accent)]'
                  }`}
                />
                {taskCount > 0 && (
                  <span className="ml-auto rounded bg-[var(--color-surface)] px-1.5 py-0.5 text-[10px] text-[var(--color-text-dim)]">
                    {taskCount} görev
                  </span>
                )}
                <div className="ml-auto flex gap-0.5">
                  <button
                    onClick={() => moveUp(idx)}
                    disabled={idx === 0}
                    className="rounded p-0.5 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface)] disabled:opacity-20"
                    title="Yukarı taşı"
                  >
                    ↑
                  </button>
                  <button
                    onClick={() => moveDown(idx)}
                    disabled={idx === draft.length - 1}
                    className="rounded p-0.5 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface)] disabled:opacity-20"
                    title="Aşağı taşı"
                  >
                    ↓
                  </button>
                </div>
              </div>
            </div>
          )
        })}

        {/* Add column button */}
        <button
          onClick={addColumn}
          className="w-full rounded-lg border border-dashed border-[var(--color-border)] py-2 text-sm text-[var(--color-text-dim)] hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
        >
          + Sütun Ekle
        </button>
      </div>

      {/* Footer */}
      <div className="border-t border-[var(--color-border)] p-3 space-y-2">
        {error && (
          <div className="rounded bg-red-500/10 px-2 py-1 text-xs text-red-400">{error}</div>
        )}
        <button
          onClick={handleSave}
          disabled={saving}
          className="w-full rounded bg-[var(--color-accent)] py-1.5 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
        >
          {saving ? 'Kaydediliyor…' : 'Kaydet'}
        </button>
      </div>
    </div>
  )
}
