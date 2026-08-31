import { useCallback, useEffect, useMemo, useState } from 'react'
import type { Dispatch, SetStateAction } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import type { Artifact, ArtifactKind } from '@/types'
import { toast } from '@/shared/components'
import { copyToClipboard } from '@/shared/lib/clipboard'
import { useRegisterDirty } from '@/shared/lib/dirtySignals'
import { draftFromArtifact, type Draft } from './artifactGrouping'

export interface ArtifactDetail {
  active: Artifact | null
  setActive: Dispatch<SetStateAction<Artifact | null>>
  // On-disk path of the active artifact (its source file, or its store JSON).
  activePath: string
  draft: Draft | null
  setDraft: Dispatch<SetStateAction<Draft | null>>
  saving: boolean
  // True when the open draft differs from the persisted artifact.
  dirty: boolean
  // Ask before dropping an unsaved draft; false = the caller must bail out.
  confirmDiscard: () => boolean
  selectArtifact: (id: string) => void
  createNew: () => Promise<void>
  startEdit: () => void
  save: () => Promise<void>
  copy: () => void
  remove: (id: string) => Promise<void>
  setArchived: (id: string, archived: boolean) => Promise<void>
  onImageUpdated: (updated: Artifact) => void
}

export interface UseArtifactDetailOptions {
  activeId: string | null
  setActiveId: Dispatch<SetStateAction<string | null>>
  setList: Dispatch<SetStateAction<Artifact[]>>
  onError: (msg: string) => void
}

// useArtifactDetail owns the right-hand pane: the loaded artifact, its on-disk
// path, the edit draft and every single-artifact mutation (create, save, copy,
// archive, delete). The unsaved-draft guard lives here too, since it is the
// draft's own state that makes a selection change destructive.
export function useArtifactDetail({
  activeId,
  setActiveId,
  setList,
  onError,
}: UseArtifactDetailOptions): ArtifactDetail {
  const { t } = useTranslation('common')
  const [active, setActive] = useState<Artifact | null>(null)
  const [activePath, setActivePath] = useState<string>('')
  // Edit/create state. When `draft` is set the viewer becomes an editor.
  const [draft, setDraft] = useState<Draft | null>(null)
  const [saving, setSaving] = useState(false)

  // Load the full artifact whenever the selection changes.
  useEffect(() => {
    if (!activeId) {
      setActive(null)
      return
    }
    setDraft(null)
    // Same cancellation guard as the path effect below: a slow response for a
    // previous selection must not overwrite the artifact now on screen.
    let cancelled = false
    api
      .getArtifact(activeId)
      .then((a) => {
        if (!cancelled) setActive(a)
      })
      .catch((e) => {
        if (!cancelled) onError((e as Error).message)
      })
    return () => {
      cancelled = true
    }
  }, [activeId, onError])

  // Resolve the active artifact's on-disk path for the copy/open-folder actions.
  useEffect(() => {
    if (!activeId) {
      setActivePath('')
      return
    }
    let cancelled = false
    api
      .artifactPath(activeId)
      .then((r) => {
        if (!cancelled) setActivePath(r.path)
      })
      .catch(() => {
        if (!cancelled) setActivePath('')
      })
    return () => {
      cancelled = true
    }
  }, [activeId])

  const remove = useCallback(
    async (id: string) => {
      if (!confirm('Bu artifact kalıcı olarak silinsin mi?')) return
      try {
        await api.deleteArtifact(id)
        setList((prev) => prev.filter((a) => a.id !== id))
        setActiveId((cur) => (cur === id ? null : cur))
        toast.success('Artifact silindi')
      } catch (e) {
        onError((e as Error).message)
      }
    },
    [onError, setList, setActiveId],
  )

  // Single-artifact archive / un-archive from the detail toolbar. Keeps the list
  // and the active artifact in sync after the flip.
  const setArchived = useCallback(
    async (id: string, archived: boolean) => {
      try {
        const updated = await api.setArtifactArchived(id, archived)
        setList((prev) => prev.map((a) => (a.id === id ? updated : a)))
        setActive((cur) => (cur && cur.id === id ? updated : cur))
        // An archived artifact leaves the default view (and vice-versa); drop the
        // selection so the viewer clears rather than showing a now-hidden card.
        setActiveId((cur) => (cur === id ? null : cur))
      } catch (e) {
        onError((e as Error).message)
      }
    },
    [onError, setList, setActiveId],
  )

  // Unsaved-edits flag: an open draft whose fields differ from the persisted
  // artifact. Surfaces on the nav "Artifactlar" item + workspace label.
  const dirty = useMemo(
    () =>
      !!draft &&
      !!active &&
      (draft.title !== active.title ||
        draft.kind !== active.kind ||
        draft.language !== active.language ||
        draft.content !== active.content ||
        draft.group.trim() !== (active.group ?? '')),
    [draft, active],
  )
  useRegisterDirty('artifacts', dirty)

  // Every action that replaces the selection also drops the open draft (the load
  // effect clears it), so an unsaved edit must not disappear silently — ask
  // first and let the caller bail out when the user declines.
  const confirmDiscard = useCallback(
    () => !dirty || confirm('Kaydedilmemiş değişiklikler var, atılsın mı?'),
    [dirty],
  )

  const selectArtifact = useCallback(
    (id: string) => {
      if (id === activeId) return
      if (!confirmDiscard()) return
      setActiveId(id)
    },
    [activeId, confirmDiscard, setActiveId],
  )

  // Create a blank artifact and drop straight into edit mode. Asks before
  // creating, since the new artifact takes over the selection and the editor.
  const createNew = useCallback(async () => {
    if (!confirmDiscard()) return
    try {
      const a = await api.createArtifact({ title: 'Yeni artifact', kind: 'markdown', content: '' })
      setList((prev) => [a, ...prev])
      setActiveId(a.id)
      setActive(a)
      setDraft(draftFromArtifact(a))
    } catch (e) {
      onError((e as Error).message)
    }
  }, [confirmDiscard, onError, setList, setActiveId])

  // Enter edit mode for the current artifact.
  const startEdit = useCallback(() => {
    if (!active) return
    setDraft(draftFromArtifact(active))
  }, [active])

  // An annotated image was written back over the SAME artifact (new source file,
  // same id). Swap the persisted row into the viewer and the list so the new
  // sourcePath — and therefore the image URL — is picked up immediately.
  const onImageUpdated = useCallback(
    (updated: Artifact) => {
      setActive(updated)
      setList((prev) => prev.map((a) => (a.id === updated.id ? updated : a)))
      toast.success(t('artifactAnnotation.updated'))
    },
    [t, setList],
  )

  // Save the draft: send only changed fields (a content change overwrites in
  // place). The `group` field is not part of the content/meta patch — the
  // backend `updateArtifact` handler ignores it — so a group change is persisted
  // through the dedicated `setArtifactGroup` endpoint (same one bulk + DnD use).
  const save = useCallback(async () => {
    if (!active || !draft) return
    const patch: { title?: string; kind?: ArtifactKind; language?: string; content?: string } = {}
    if (draft.title.trim() && draft.title !== active.title) patch.title = draft.title.trim()
    if (draft.kind !== active.kind) patch.kind = draft.kind
    if (draft.language !== active.language) patch.language = draft.language
    if (draft.content !== active.content) patch.content = draft.content
    const nextGroup = draft.group.trim()
    const groupChanged = nextGroup !== (active.group ?? '')
    if (Object.keys(patch).length === 0 && !groupChanged) {
      setDraft(null)
      return
    }
    setSaving(true)
    try {
      // Apply the content/meta patch first (if any), then the group change; the
      // last response is the authoritative post-save artifact.
      let updated = active
      if (Object.keys(patch).length > 0) {
        updated = await api.updateArtifact(active.id, patch)
      }
      if (groupChanged) {
        updated = await api.setArtifactGroup(active.id, nextGroup)
      }
      setActive(updated)
      setDraft(null)
      setList((prev) => prev.map((a) => (a.id === updated.id ? updated : a)))
      toast.success('Kaydedildi')
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }, [active, draft, onError, setList])

  const copy = useCallback(() => {
    if (!active) return
    copyToClipboard(active.content).then((ok) => {
      if (ok) toast.info('Panoya kopyalandı')
    })
  }, [active])

  return {
    active,
    setActive,
    activePath,
    draft,
    setDraft,
    saving,
    dirty,
    confirmDiscard,
    selectArtifact,
    createNew,
    startEdit,
    save,
    copy,
    remove,
    setArchived,
    onImageUpdated,
  }
}
