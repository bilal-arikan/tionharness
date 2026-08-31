import { useCallback, useRef, useState } from 'react'
import { api } from '@/api'
import { artifactKindForUpload } from './artifactMeta'

export interface ArtifactImport {
  // True while a file drag hovers the panel, and while an import is running —
  // both render the drop overlay.
  dragging: boolean
  importing: boolean
  // Spread onto the panel root to arm the drop zone.
  dropZoneProps: {
    onDragEnter: (e: React.DragEvent) => void
    onDragOver: (e: React.DragEvent) => void
    onDragLeave: (e: React.DragEvent) => void
    onDrop: (e: React.DragEvent) => void
  }
}

export interface UseArtifactImportOptions {
  onError: (msg: string) => void
  reload: () => void
  // Jump to the first imported artifact. Guarded by the unsaved-draft prompt.
  selectArtifact: (id: string) => void
}

// useArtifactImport owns the drag-and-drop file upload: the depth-counted drag
// overlay state and the per-file upload → createArtifact loop.
export function useArtifactImport({
  onError,
  reload,
  selectArtifact,
}: UseArtifactImportOptions): ArtifactImport {
  // Drag-and-drop file import state. dragDepth tracks nested dragenter/leave so
  // the overlay does not flicker when dragging over child elements.
  const [dragging, setDragging] = useState(false)
  const [importing, setImporting] = useState(false)
  const dragDepth = useRef(0)

  // Import dropped files: upload each into the workspace, then create an artifact
  // pointing at it (media renders inline; small text/code keeps its content).
  const importFiles = useCallback(
    async (files: File[]) => {
      if (files.length === 0) return
      setImporting(true)
      let firstId: string | null = null
      try {
        for (const file of files) {
          try {
            // Sessionless manual uploads share the "_shared" bucket under artifacts/.
            const att = await api.uploadFile('_shared', file)
            const kind = artifactKindForUpload(att)
            const created = await api.createArtifact({
              title: att.name,
              kind,
              content: att.textContent ?? '',
              // Every kind now references its uploaded file on disk; text/code also
              // keep their content inline for the editor.
              sourcePath: att.relPath,
              origin: 'manual',
            })
            if (!firstId) firstId = created.id
          } catch (e) {
            onError(`"${file.name}" eklenemedi: ${(e as Error).message}`)
          }
        }
        reload()
        // The import itself is what the user asked for, so it always happens;
        // only the jump to the first imported artifact is guarded, and declining
        // keeps the open draft on screen.
        if (firstId) selectArtifact(firstId)
      } finally {
        setImporting(false)
      }
    },
    [onError, reload, selectArtifact],
  )

  // Drag-and-drop handlers (depth-counted so nested elements don't flicker).
  const onDragEnter = useCallback((e: React.DragEvent) => {
    if (!Array.from(e.dataTransfer.types).includes('Files')) return
    e.preventDefault()
    dragDepth.current += 1
    setDragging(true)
  }, [])
  const onDragOver = useCallback((e: React.DragEvent) => {
    if (Array.from(e.dataTransfer.types).includes('Files')) e.preventDefault()
  }, [])
  const onDragLeave = useCallback((e: React.DragEvent) => {
    if (!Array.from(e.dataTransfer.types).includes('Files')) return
    dragDepth.current = Math.max(0, dragDepth.current - 1)
    if (dragDepth.current === 0) setDragging(false)
  }, [])
  const onDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault()
      dragDepth.current = 0
      setDragging(false)
      const files = Array.from(e.dataTransfer.files)
      if (files.length) void importFiles(files)
    },
    [importFiles],
  )

  return {
    dragging,
    importing,
    dropZoneProps: { onDragEnter, onDragOver, onDragLeave, onDrop },
  }
}
