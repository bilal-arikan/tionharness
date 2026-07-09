import { useCallback, useMemo, useRef, useState } from 'react'
import { Paperclip, Plus, UploadCloud } from 'lucide-react'
import { api } from '@/api'
import type { Artifact } from '@/types'
import { KIND_ICON, KIND_LABEL, artifactKindForUpload } from '@/features/artifacts/artifactMeta'

interface Props {
  // Referenced artifact ids (order preserved).
  value: string[]
  onChange: (ids: string[]) => void
  // All workspace artifacts, used to resolve referenced ids to titles/kinds and
  // to populate the "link existing" picker.
  artifacts: Artifact[]
  // Called after new artifacts are created from dropped files, so the parent can
  // reload its artifact list (the newly created ones need to resolve to chips).
  onArtifactsChanged?: (created: Artifact[]) => void
  // Upload path bucket (a safe single segment): the task id when editing, else a
  // shared bucket. Only affects where the uploaded file lands on disk.
  bucket: string
  onError: (msg: string) => void
}

// TaskArtifactRefs manages a card's artifact references: it lists the linked
// artifacts as removable chips, lets the user link an existing artifact from a
// searchable picker, and accepts dropped files — each dropped file is uploaded
// into the workspace, saved as an artifact, and its id appended to the card.
export function TaskArtifactRefs({ value, onChange, artifacts, onArtifactsChanged, bucket, onError }: Props) {
  const [pickerOpen, setPickerOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [dragging, setDragging] = useState(false)
  const [importing, setImporting] = useState(false)
  const dragDepth = useRef(0)

  const byId = useMemo(() => new Map(artifacts.map((a) => [a.id, a])), [artifacts])
  // Resolve refs to artifacts, dropping ids that no longer exist (stale refs are
  // silently skipped — the artifact was deleted elsewhere).
  const linked = value.map((id) => byId.get(id)).filter((a): a is Artifact => !!a)

  const add = (id: string) => {
    if (!value.includes(id)) onChange([...value, id])
    setQuery('')
    setPickerOpen(false)
  }
  const remove = (id: string) => onChange(value.filter((x) => x !== id))

  // Candidates for the picker: not already linked, matching the query.
  const candidates = useMemo(() => {
    const q = query.trim().toLowerCase()
    return artifacts
      .filter((a) => !value.includes(a.id))
      .filter((a) => !q || a.title.toLowerCase().includes(q) || a.kind.toLowerCase().includes(q))
      .slice(0, 20)
  }, [artifacts, value, query])

  // Upload each dropped file into the workspace, create an artifact pointing at
  // it, and append the new ids to the card's references.
  const importFiles = useCallback(
    async (files: File[]) => {
      if (files.length === 0) return
      setImporting(true)
      const createdIds: string[] = []
      const created: Artifact[] = []
      try {
        for (const file of files) {
          try {
            const att = await api.uploadFile(bucket || 'board', file)
            const a = await api.createArtifact({
              title: att.name,
              kind: artifactKindForUpload(att),
              content: att.textContent ?? '',
              sourcePath: att.relPath,
              origin: 'manual',
            })
            createdIds.push(a.id)
            created.push(a)
          } catch (e) {
            onError(`"${file.name}" eklenemedi: ${(e as Error).message}`)
          }
        }
        if (createdIds.length) {
          onChange([...value, ...createdIds])
          onArtifactsChanged?.(created)
        }
      } finally {
        setImporting(false)
      }
    },
    [bucket, value, onChange, onArtifactsChanged, onError],
  )

  const onDragEnter = (e: React.DragEvent) => {
    if (!Array.from(e.dataTransfer.types).includes('Files')) return
    e.preventDefault()
    dragDepth.current += 1
    setDragging(true)
  }
  const onDragOver = (e: React.DragEvent) => {
    if (Array.from(e.dataTransfer.types).includes('Files')) e.preventDefault()
  }
  const onDragLeave = (e: React.DragEvent) => {
    if (!Array.from(e.dataTransfer.types).includes('Files')) return
    dragDepth.current = Math.max(0, dragDepth.current - 1)
    if (dragDepth.current === 0) setDragging(false)
  }
  const onDrop = (e: React.DragEvent) => {
    e.preventDefault()
    dragDepth.current = 0
    setDragging(false)
    const files = Array.from(e.dataTransfer.files)
    if (files.length) void importFiles(files)
  }

  return (
    <div
      onDragEnter={onDragEnter}
      onDragOver={onDragOver}
      onDragLeave={onDragLeave}
      onDrop={onDrop}
      className={`flex flex-col gap-2 rounded-lg border border-dashed p-2 transition ${
        dragging
          ? 'border-[var(--color-accent)] bg-[color-mix(in_srgb,var(--color-accent)_10%,transparent)]'
          : 'border-[var(--color-border)]'
      }`}
    >
      {/* Linked artifact chips */}
      {linked.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {linked.map((a) => {
            const Icon = KIND_ICON[a.kind] ?? Paperclip
            return (
              <span
                key={a.id}
                title={`${KIND_LABEL[a.kind] ?? a.kind} — ${a.title}`}
                className="inline-flex max-w-[220px] items-center gap-1 rounded-full bg-[var(--color-surface-2)] px-2 py-0.5 text-xs text-[var(--color-text)]"
              >
                <Icon size={12} className="shrink-0 text-[var(--color-text-dim)]" />
                <span className="truncate">{a.title}</span>
                <button
                  onClick={() => remove(a.id)}
                  className="shrink-0 opacity-60 hover:opacity-100"
                  title="Referansı kaldır"
                >
                  ×
                </button>
              </span>
            )
          })}
        </div>
      )}

      {/* Actions row: link existing + drop hint */}
      <div className="relative flex items-center gap-2">
        <button
          type="button"
          onClick={() => setPickerOpen((v) => !v)}
          className="inline-flex items-center gap-1 rounded border border-[var(--color-border)] px-2 py-1 text-xs text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
        >
          <Plus size={12} /> Var olan
        </button>
        <span className="inline-flex items-center gap-1 text-[11px] text-[var(--color-text-dim)]">
          {importing ? (
            <>
              <UploadCloud size={12} className="animate-pulse" /> Yükleniyor…
            </>
          ) : (
            <>
              <UploadCloud size={12} /> ya da dosya sürükle-bırak
            </>
          )}
        </span>

        {/* Existing-artifact picker dropdown */}
        {pickerOpen && (
          <div className="absolute left-0 top-9 z-20 w-72 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-2 shadow-lg">
            <input
              autoFocus
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Artifact ara…"
              className="mb-1.5 w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-xs outline-none focus:border-[var(--color-accent)]"
            />
            <div className="max-h-48 overflow-y-auto">
              {candidates.length === 0 ? (
                <div className="px-2 py-3 text-center text-xs text-[var(--color-text-dim)]">
                  {artifacts.length === 0 ? 'Henüz artifact yok' : 'Eşleşme yok'}
                </div>
              ) : (
                candidates.map((a) => {
                  const Icon = KIND_ICON[a.kind] ?? Paperclip
                  return (
                    <button
                      key={a.id}
                      onClick={() => add(a.id)}
                      className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs hover:bg-[var(--color-surface-2)]"
                    >
                      <Icon size={13} className="shrink-0 text-[var(--color-text-dim)]" />
                      <span className="truncate">{a.title}</span>
                      <span className="ml-auto shrink-0 text-[10px] text-[var(--color-text-dim)]">
                        {KIND_LABEL[a.kind] ?? a.kind}
                      </span>
                    </button>
                  )
                })
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
