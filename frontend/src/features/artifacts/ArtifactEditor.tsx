import type { Dispatch, SetStateAction } from 'react'
import type { Artifact, ArtifactKind } from '@/types'
import { ArtifactView } from './ArtifactView'
import { KINDS, KIND_LABEL, isMediaKind } from './artifactMeta'
import type { Draft } from './artifactGrouping'

interface Props {
  // The persisted artifact behind the draft — media bodies still render from it.
  active: Artifact
  draft: Draft
  setDraft: Dispatch<SetStateAction<Draft | null>>
  // Existing group names, offered as autocomplete suggestions.
  groupNames: string[]
}

// ArtifactEditor is the edit form for one artifact: title/kind/language/group
// fields plus the content textarea (media kinds keep their read-only preview,
// since their body lives on disk). Presentational — saving is the caller's job.
export function ArtifactEditor({ active, draft, setDraft, groupNames }: Props) {
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-5">
      <div className="flex flex-wrap items-center gap-2">
        <input
          data-testid="artifact-edit-title-input"
          value={draft.title}
          onChange={(e) => setDraft({ ...draft, title: e.target.value })}
          placeholder="Başlık"
          className="min-w-48 flex-1 rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-1.5 text-sm"
        />
        {!isMediaKind(draft.kind) && (
          <select
            data-testid="artifact-edit-kind-select"
            value={draft.kind}
            onChange={(e) => setDraft({ ...draft, kind: e.target.value as ArtifactKind })}
            className="rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1.5 text-sm"
          >
            {KINDS.map((k) => (
              <option key={k} value={k}>
                {KIND_LABEL[k]}
              </option>
            ))}
          </select>
        )}
        {draft.kind === 'code' && (
          <input
            value={draft.language}
            onChange={(e) => setDraft({ ...draft, language: e.target.value })}
            placeholder="dil (ör. go)"
            className="w-32 rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-1.5 text-sm"
          />
        )}
        {/* Per-artifact group: edit this single artifact's organisation
            bucket directly, without entering multi-select or dragging.
            Autocompletes to existing group names; blank = ungrouped. */}
        <input
          data-testid="artifact-edit-group-input"
          list="artifacts-group-names"
          value={draft.group}
          onChange={(e) => setDraft({ ...draft, group: e.target.value })}
          placeholder="Grup (opsiyonel)"
          title="Bu artifact'in grubu — boş bırakırsan grupsuz olur"
          className="w-40 rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-1.5 text-sm"
        />
        <datalist id="artifacts-group-names">
          {groupNames.map((n) => (
            <option key={n} value={n} />
          ))}
        </datalist>
      </div>
      {isMediaKind(draft.kind) ? (
        <>
          <div className="flex-1 overflow-y-auto">
            <ArtifactView
              kind={active.kind}
              language={active.language}
              content={active.content}
              sourcePath={active.sourcePath}
            />
          </div>
          <p className="text-[11px] text-[var(--color-text-dim)]">
            Medya dosyasının içeriği düzenlenemez — yalnız başlığı değiştirebilirsin.
          </p>
        </>
      ) : (
        <>
          <textarea
            data-testid="artifact-edit-content-textarea"
            value={draft.content}
            onChange={(e) => setDraft({ ...draft, content: e.target.value })}
            placeholder="İçerik…"
            spellCheck={false}
            className="min-h-[40vh] flex-1 resize-none rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] p-3 font-mono text-xs leading-relaxed"
          />
          <p className="text-[11px] text-[var(--color-text-dim)]">
            Kaydedince içerik yerinde güncellenir.
          </p>
        </>
      )}
    </div>
  )
}
