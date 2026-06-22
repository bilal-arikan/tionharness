// CoreMemoryCard shows and edits an agent's MemGPT-style core working memory —
// the persistent text the agent maintains itself (via core_memory_replace/append)
// and that is injected into every prompt. It is a set of named blocks: persona
// and human by default, plus any custom blocks the user/agent defines. Each block
// has a character limit shown as a usage bar. Collapsible, starts collapsed.
import { useEffect, useState } from 'react'
import { Brain, ChevronDown, ChevronRight, Pencil, Check, X, Plus, Trash2, Lock } from 'lucide-react'
import { api } from '../../api'
import type { CoreBlock } from '../../types'
import { Button } from '../common'

interface Props {
  agentId: string
  onError: (msg: string) => void
}

// Default blocks cannot be deleted (the backend re-seeds them); only custom
// blocks show a delete affordance.
const DEFAULT_LABELS = new Set(['persona', 'human'])

export function CoreMemoryCard({ agentId, onError }: Props) {
  const [blocks, setBlocks] = useState<CoreBlock[]>([])
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<string | null>(null) // label being edited
  const [draft, setDraft] = useState('')
  const [saving, setSaving] = useState(false)
  const [adding, setAdding] = useState(false)
  const [newLabel, setNewLabel] = useState('')
  const [newLimit, setNewLimit] = useState('')

  const load = (id: string) =>
    api
      .getCore(id)
      .then((r) => setBlocks(r.blocks || []))
      .catch((e) => onError((e as Error).message))

  useEffect(() => {
    load(agentId)
    setEditing(null)
    setAdding(false)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agentId])

  const startEdit = (b: CoreBlock) => {
    setDraft(b.content)
    setEditing(b.label)
    setOpen(true)
  }

  const save = async () => {
    if (!editing) return
    setSaving(true)
    try {
      const r = await api.writeCore(agentId, { [editing]: draft })
      setBlocks(r.blocks || [])
      setEditing(null)
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const addBlock = async () => {
    const label = newLabel.trim().toLowerCase()
    if (!label) return
    setSaving(true)
    try {
      const r = await api.defineCoreBlock(agentId, {
        label,
        charLimit: newLimit.trim() ? Number(newLimit) : undefined,
      })
      setBlocks(r.blocks || [])
      setNewLabel('')
      setNewLimit('')
      setAdding(false)
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const removeBlock = async (label: string) => {
    try {
      const r = await api.deleteCoreBlock(agentId, label)
      setBlocks(r.blocks || [])
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const totalLen = blocks.reduce((sum, b) => sum + b.content.trim().length, 0)

  return (
    <div className="mb-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)]">
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm font-medium text-[var(--color-text)]"
      >
        {open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        <Brain size={14} className="text-[var(--color-accent)]" />
        Çekirdek bellek
        <span className="rounded bg-[var(--color-bg)] px-1.5 py-0.5 text-xs font-normal text-[var(--color-text-dim)]">
          {blocks.length} blok · {totalLen === 0 ? 'boş' : `${totalLen} karakter`}
        </span>
      </button>

      {open && (
        <div className="flex flex-col gap-2 border-t border-[var(--color-border)] px-3 py-2">
          {blocks.map((b) => (
            <CoreBlockRow
              key={b.label}
              block={b}
              editing={editing === b.label}
              draft={draft}
              saving={saving}
              deletable={!DEFAULT_LABELS.has(b.label)}
              onDraft={setDraft}
              onStart={() => startEdit(b)}
              onCancel={() => setEditing(null)}
              onSave={save}
              onDelete={() => removeBlock(b.label)}
            />
          ))}

          {/* Add a custom block */}
          {adding ? (
            <div className="flex flex-col gap-2 rounded border border-dashed border-[var(--color-border)] bg-[var(--color-bg)] p-2">
              <div className="flex gap-2">
                <input
                  value={newLabel}
                  onChange={(e) => setNewLabel(e.target.value)}
                  placeholder="blok etiketi (örn. project)"
                  className="flex-1 rounded border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
                />
                <input
                  value={newLimit}
                  onChange={(e) => setNewLimit(e.target.value.replace(/[^0-9]/g, ''))}
                  placeholder="limit"
                  className="w-20 rounded border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
                />
              </div>
              <div className="flex items-center gap-2">
                <Button onClick={addBlock} disabled={saving || !newLabel.trim()} size="sm" className="flex items-center gap-1">
                  <Check size={12} /> Ekle
                </Button>
                <Button onClick={() => setAdding(false)} variant="secondary" size="sm" className="flex items-center gap-1">
                  <X size={12} /> İptal
                </Button>
              </div>
            </div>
          ) : (
            <button
              onClick={() => setAdding(true)}
              className="flex items-center justify-center gap-1 rounded border border-dashed border-[var(--color-border)] py-1.5 text-xs text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
            >
              <Plus size={12} /> Yeni blok
            </button>
          )}
        </div>
      )}
    </div>
  )
}

function CoreBlockRow({
  block,
  editing,
  draft,
  saving,
  deletable,
  onDraft,
  onStart,
  onCancel,
  onSave,
  onDelete,
}: {
  block: CoreBlock
  editing: boolean
  draft: string
  saving: boolean
  deletable: boolean
  onDraft: (v: string) => void
  onStart: () => void
  onCancel: () => void
  onSave: () => void
  onDelete: () => void
}) {
  const len = (editing ? draft : block.content).trim().length
  const pct = block.charLimit > 0 ? Math.min(100, (len / block.charLimit) * 100) : 0
  const over = len > block.charLimit
  const empty = block.content.trim() === ''

  return (
    <div className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] p-2">
      <div className="mb-1 flex items-center gap-1.5">
        <span className="flex items-center gap-1 text-xs font-medium text-[var(--color-text)]">
          {block.label}
          {block.readOnly && <Lock size={10} className="text-[var(--color-text-dim)]" />}
        </span>
        <span className={`text-[10px] ${over ? 'text-[var(--color-danger)]' : 'text-[var(--color-text-dim)]'}`}>
          {len}/{block.charLimit}
        </span>
        <div className="ml-auto flex items-center gap-1">
          {!editing && !block.readOnly && (
            <button
              onClick={onStart}
              className="flex items-center gap-1 rounded px-1.5 py-0.5 text-xs text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
            >
              <Pencil size={11} /> Düzenle
            </button>
          )}
          {deletable && !editing && (
            <button
              onClick={onDelete}
              className="rounded px-1 py-0.5 text-[var(--color-text-dim)] transition hover:text-[var(--color-danger)]"
              title="Bloğu sil"
            >
              <Trash2 size={11} />
            </button>
          )}
        </div>
      </div>

      {/* Usage bar */}
      <div className="mb-1.5 h-1 overflow-hidden rounded bg-[var(--color-surface-2)]">
        <div
          className={`h-full transition-all ${over ? 'bg-[var(--color-danger)]' : 'bg-[var(--color-accent)]'}`}
          style={{ width: `${pct}%` }}
        />
      </div>

      {block.description && !editing && (
        <p className="mb-1 text-[10px] italic text-[var(--color-text-dim)]">{block.description}</p>
      )}

      {editing ? (
        <div className="flex flex-col gap-2">
          <textarea
            value={draft}
            onChange={(e) => onDraft(e.target.value)}
            rows={5}
            className="w-full resize-y rounded border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
          />
          <div className="flex items-center gap-2">
            <Button onClick={onSave} disabled={saving} size="sm" className="flex items-center gap-1">
              <Check size={12} /> {saving ? 'Kaydediliyor…' : 'Kaydet'}
            </Button>
            <Button onClick={onCancel} variant="secondary" size="sm" className="flex items-center gap-1">
              <X size={12} /> İptal
            </Button>
          </div>
        </div>
      ) : empty ? (
        <p className="text-xs text-[var(--color-text-dim)]">Boş — ajan kendisi yazar ya da "Düzenle" ile ekleyebilirsin.</p>
      ) : (
        <pre className="whitespace-pre-wrap break-words font-mono text-xs leading-relaxed text-[var(--color-text)]">{block.content}</pre>
      )}
    </div>
  )
}
