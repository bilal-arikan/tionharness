import {
  Archive,
  ArchiveRestore,
  Copy,
  ExternalLink,
  FileCode,
  Pencil,
  Save,
  Trash2,
  X,
} from 'lucide-react'
import type { Agent, Artifact } from '@/types'
import { PaneHeader } from '@/shared/components'
import { CopyPathButton } from '@/shared/components/CopyPathButton'
import { AgentAvatar } from '@/shared/components/agents/AgentAvatar'
import { ImageArtifactEditButton } from './ImageArtifactEditButton'
import { KIND_LABEL } from './artifactMeta'
import { OriginBadge } from './OriginBadge'

interface Props {
  listOpen: boolean
  onToggleList: () => void
  active: Artifact | null
  // True while the editor is open — swaps the action set for save/cancel.
  editing: boolean
  saving: boolean
  // On-disk path of the active artifact, for the copy-path action.
  activePath: string
  agents: Agent[]
  onOpenSession?: (sessionId: string) => void
  onError: (msg: string) => void
  onImageUpdated: (updated: Artifact) => void
  onStartEdit: () => void
  onCopy: () => void
  onSave: () => void
  onCancelEdit: () => void
  onSetArchived: (id: string, archived: boolean) => void
  onDelete: (id: string) => void
}

const iconBtn =
  'rounded-md border border-[var(--color-border)] p-1.5 text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]'

// ArtifactDetailHeader is the viewer/editor column's title bar: artifact
// identity chips on the left, content actions in the chip row, and the
// file-level / destructive actions on the right. Presentational.
export function ArtifactDetailHeader({
  listOpen,
  onToggleList,
  active,
  editing,
  saving,
  activePath,
  agents,
  onOpenSession,
  onError,
  onImageUpdated,
  onStartEdit,
  onCopy,
  onSave,
  onCancelEdit,
  onSetArchived,
  onDelete,
}: Props) {
  const creator = (a: Artifact) => agents.find((ag) => ag.id === a.agentId) ?? null

  return (
    <PaneHeader
      listOpen={listOpen}
      onToggleList={onToggleList}
      // Detail view: the top bar hosts the artifact identity + actions (no
      // redundant "Artifactlar" title / subtitle). Empty state keeps the title.
      // Chips + the İçerik (content-copy) button always live on their own second
      // row (every width), so the first row stays compact.
      secondaryAlwaysWrap
      title={active ? undefined : 'Artifactlar'}
      titleSlot={
        active ? (
          <div className="flex min-w-0 items-center gap-2">
            <FileCode size={16} className="shrink-0 text-[var(--color-accent)]" />
            <div className="truncate text-sm font-semibold">{active.title}</div>
          </div>
        ) : undefined
      }
      secondary={
        active ? (
          <>
            <OriginBadge origin={active.origin} />
            {active.archived && (
              <span className="flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide bg-[color-mix(in_srgb,var(--color-warning)_15%,transparent)] text-[var(--color-warning)]">
                <Archive size={11} /> Arşivlendi
              </span>
            )}
            {active.group && (
              <span className="rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide bg-[var(--color-surface-2)] text-[var(--color-text-dim)]">
                {active.group}
              </span>
            )}
            <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[11px] text-[var(--color-text-dim)]">
              {KIND_LABEL[active.kind] ?? active.kind}
              {active.language ? ` · ${active.language}` : ''}
            </span>
            {creator(active) && (
              <span className="flex items-center gap-1 text-[11px] text-[var(--color-text-dim)]">
                <AgentAvatar agent={creator(active)!} size={14} />
                {creator(active)!.name}
              </span>
            )}
            {/* Content actions sit together at the end of the chip row: edit
                lives right next to the content-copy button (both act on the
                artifact's body), while the right slot keeps the file-level
                and destructive actions. */}
            {!editing && (
              <div className="ml-auto flex shrink-0 items-center gap-1.5">
                {/* Image artifacts additionally get the annotator, which
                    overwrites this same artifact's source file in place. */}
                <ImageArtifactEditButton
                  artifact={active}
                  onUpdated={onImageUpdated}
                  onError={onError}
                  className="flex shrink-0 items-center gap-1.5 rounded-md border border-[var(--color-border)] px-2 py-1 text-xs text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] disabled:opacity-50"
                />
                <button
                  data-testid="artifact-detail-edit"
                  onClick={onStartEdit}
                  title="Düzenle"
                  className="flex shrink-0 items-center gap-1.5 rounded-md border border-[var(--color-border)] px-2 py-1 text-xs text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
                >
                  <Pencil size={14} />
                  <span>Düzenle</span>
                </button>
                <button
                  data-testid="artifact-detail-copy"
                  onClick={onCopy}
                  title="İçeriği kopyala"
                  className="flex shrink-0 items-center gap-1.5 rounded-md border border-[var(--color-border)] px-2 py-1 text-xs text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
                >
                  <Copy size={14} />
                  <span>İçerik</span>
                </button>
              </div>
            )}
          </>
        ) : undefined
      }
      right={
        active ? (
          <div className="flex flex-wrap items-center gap-1.5">
            {editing ? (
              <>
                <button
                  data-testid="artifact-edit-save"
                  onClick={onSave}
                  disabled={saving}
                  className="flex items-center gap-1 rounded-md bg-[var(--color-accent)] px-2.5 py-1.5 text-xs font-medium text-[var(--color-on-accent)] hover:brightness-110 disabled:opacity-50"
                >
                  <Save size={14} /> {saving ? 'Kaydediliyor…' : 'Kaydet'}
                </button>
                <button onClick={onCancelEdit} title="İptal" className={iconBtn}>
                  <X size={15} />
                </button>
              </>
            ) : (
              <>
                {/* Edit moved next to the content-copy button in the chip row. */}
                <CopyPathButton path={activePath} title="Yolu kopyala" />
                {active.sessionId && onOpenSession && (
                  <button
                    onClick={() => onOpenSession(active.sessionId)}
                    title="Kaynak sohbete git"
                    className={iconBtn}
                  >
                    <ExternalLink size={15} />
                  </button>
                )}
                <button
                  data-testid="artifact-detail-archive"
                  onClick={() => onSetArchived(active.id, !active.archived)}
                  title={active.archived ? 'Arşivden çıkar' : 'Arşivle'}
                  className={iconBtn}
                >
                  {active.archived ? <ArchiveRestore size={15} /> : <Archive size={15} />}
                </button>
                <button
                  data-testid="artifact-detail-delete"
                  onClick={() => onDelete(active.id)}
                  title="Sil"
                  className="flex items-center gap-1.5 rounded-md border border-[var(--color-border)] px-2.5 py-1.5 text-xs text-[var(--color-danger)] hover:bg-[color-mix(in_srgb,var(--color-danger)_10%,transparent)] disabled:opacity-50"
                >
                  <Trash2 size={14} /> Sil
                </button>
              </>
            )}
          </div>
        ) : undefined
      }
    />
  )
}
