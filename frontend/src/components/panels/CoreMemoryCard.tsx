// CoreMemoryCard shows and edits an agent's MemGPT-style core working memory —
// the persistent text the agent maintains itself (via core_memory_replace/append)
// and that is injected into every prompt. It is split into two sections: persona
// (about the agent) and human (about the user). Collapsible, starts collapsed.
import { useEffect, useState } from 'react'
import { Brain, ChevronDown, ChevronRight, Pencil, Check, X, User, Bot } from 'lucide-react'
import { api } from '../../api'

interface Props {
  agentId: string
  onError: (msg: string) => void
}

type Section = 'persona' | 'human'

export function CoreMemoryCard({ agentId, onError }: Props) {
  const [persona, setPersona] = useState('')
  const [human, setHuman] = useState('')
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<Section | null>(null)
  const [draft, setDraft] = useState('')
  const [saving, setSaving] = useState(false)

  const load = (id: string) =>
    api
      .getCore(id)
      .then((r) => {
        setPersona(r.persona || '')
        setHuman(r.human || '')
      })
      .catch((e) => onError((e as Error).message))

  useEffect(() => {
    load(agentId)
    setEditing(null)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agentId])

  const startEdit = (section: Section) => {
    setDraft(section === 'persona' ? persona : human)
    setEditing(section)
    setOpen(true)
  }

  const save = async () => {
    if (!editing) return
    setSaving(true)
    try {
      const r = await api.writeCore(agentId, { [editing]: draft })
      setPersona(r.persona || '')
      setHuman(r.human || '')
      setEditing(null)
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const totalLen = persona.trim().length + human.trim().length

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
          {totalLen === 0 ? 'boş' : `${totalLen} karakter`}
        </span>
      </button>

      {open && (
        <div className="flex flex-col gap-2 border-t border-[var(--color-border)] px-3 py-2">
          <CoreSection
            section="persona"
            icon={<Bot size={13} className="text-[var(--color-accent)]" />}
            label="Persona — ajan kendini nasıl tanımlıyor"
            value={persona}
            editing={editing === 'persona'}
            draft={draft}
            saving={saving}
            onDraft={setDraft}
            onStart={() => startEdit('persona')}
            onCancel={() => setEditing(null)}
            onSave={save}
            placeholder="Ajanın kendi hakkındaki kalıcı notları (kimlik, davranış, ton)…"
          />
          <CoreSection
            section="human"
            icon={<User size={13} className="text-[var(--color-accent)]" />}
            label="Human — ajanın kullanıcı hakkında bildikleri"
            value={human}
            editing={editing === 'human'}
            draft={draft}
            saving={saving}
            onDraft={setDraft}
            onStart={() => startEdit('human')}
            onCancel={() => setEditing(null)}
            onSave={save}
            placeholder="Kullanıcı hakkında kalıcı bilgiler (ad, tercihler, bağlam)…"
          />
        </div>
      )}
    </div>
  )
}

function CoreSection({
  icon,
  label,
  value,
  editing,
  draft,
  saving,
  onDraft,
  onStart,
  onCancel,
  onSave,
  placeholder,
}: {
  section: Section
  icon: React.ReactNode
  label: string
  value: string
  editing: boolean
  draft: string
  saving: boolean
  onDraft: (v: string) => void
  onStart: () => void
  onCancel: () => void
  onSave: () => void
  placeholder: string
}) {
  const empty = value.trim() === ''
  return (
    <div className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] p-2">
      <div className="mb-1 flex items-center gap-1.5">
        {icon}
        <span className="flex-1 text-xs font-medium text-[var(--color-text)]">{label}</span>
        {!editing && (
          <button
            onClick={onStart}
            className="flex items-center gap-1 rounded px-1.5 py-0.5 text-xs text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
          >
            <Pencil size={11} /> Düzenle
          </button>
        )}
      </div>
      {editing ? (
        <div className="flex flex-col gap-2">
          <textarea
            value={draft}
            onChange={(e) => onDraft(e.target.value)}
            rows={5}
            placeholder={placeholder}
            className="w-full resize-y rounded border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
          />
          <div className="flex items-center gap-2">
            <button
              onClick={onSave}
              disabled={saving}
              className="flex items-center gap-1 rounded bg-[var(--color-accent)] px-2.5 py-1 text-xs font-medium text-white transition disabled:opacity-40"
            >
              <Check size={12} /> {saving ? 'Kaydediliyor…' : 'Kaydet'}
            </button>
            <button
              onClick={onCancel}
              className="flex items-center gap-1 rounded border border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-text-dim)] transition hover:text-[var(--color-text)]"
            >
              <X size={12} /> İptal
            </button>
          </div>
        </div>
      ) : empty ? (
        <p className="text-xs text-[var(--color-text-dim)]">Boş — ajan kendisi yazar ya da "Düzenle" ile ekleyebilirsin.</p>
      ) : (
        <pre className="whitespace-pre-wrap break-words font-mono text-xs leading-relaxed text-[var(--color-text)]">{value}</pre>
      )}
    </div>
  )
}
