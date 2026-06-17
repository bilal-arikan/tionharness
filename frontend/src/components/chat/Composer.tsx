import { useEffect, useMemo, useRef, useState } from 'react'
import { Paperclip, Hash, Brain } from 'lucide-react'
import type { Agent, Artifact, Attachment, SlashCommand } from '../../types'
import { AgentAvatar } from '../agents/AgentAvatar'
import { AttachmentChip } from './AttachmentChip'
import { api } from '../../api'
import { PASTE_AS_FILE_THRESHOLD } from '../../lib/attachments'

// Cap how much of a referenced artifact is inlined into the turn (the artifact
// itself stays addressable; very large ones are truncated with a note).
const ARTIFACT_INLINE_CAP = 64 << 10

// PendingAttachment tracks one attachment while composing: its local preview and
// upload state, plus the server descriptor once the upload resolves.
interface PendingAttachment {
  localId: string
  name: string
  previewURL?: string // object URL for image previews
  uploading: boolean
  error?: string
  attachment?: Attachment // populated when the upload succeeds
}

interface Props {
  disabled: boolean
  // sessionId scopes uploads; attachments require an active session.
  sessionId?: string
  // streaming: a turn is currently in flight. Changes the action buttons:
  // empty input → "Durdur"; filled input → Queue / Interrupt / Steer.
  streaming?: boolean
  onSend: (text: string, attachments: Attachment[]) => void
  onStop?: () => void
  onInterrupt?: (text: string) => void
  onQueue?: (text: string) => void
  onSteer?: (text: string) => void
  // Per-turn reasoning level ('' = agent default). Picked from a small menu in
  // the composer and applied to the next message.
  thinkingLevel?: string
  onThinkingLevelChange?: (v: string) => void
  // Per-turn permission-mode override ('' = agent default). Picked from a small
  // menu in the composer or cycled with Shift+Tab; applied to the next message.
  permissionMode?: string
  onPermissionModeChange?: (v: string) => void
  agents: Agent[]
  commands: SlashCommand[]
  // Artifacts in the active session, offered by the "#" picker to include their
  // content in the next turn.
  artifacts?: Artifact[]
}

// Reasoning levels offered in the composer picker. '' defers to the agent's own
// ThinkingLevel; the rest override it for the turn (see chatReq.ThinkingLevel).
const THINKING_OPTIONS: { value: string; label: string; hint: string }[] = [
  { value: '', label: 'Oto', hint: 'Ajanın kendi ayarı' },
  { value: 'off', label: 'Kapalı', hint: 'Düşünme yok' },
  { value: 'low', label: 'Düşük', hint: 'Kısa akıl yürütme' },
  { value: 'medium', label: 'Orta', hint: 'Dengeli' },
  { value: 'high', label: 'Yüksek', hint: 'Derin akıl yürütme' },
]

// Permission modes offered in the composer picker / Shift+Tab cycle. '' defers
// to the agent's own PermissionMode; the rest override it for the turn (see
// chatReq.PermissionMode). Order is the Shift+Tab cycle order.
const PERMISSION_OPTIONS: { value: string; label: string; hint: string; icon: string }[] = [
  { value: '', label: 'Oto (ajan)', hint: 'Ajanın kendi izin ayarı', icon: '🛡' },
  { value: 'read-only', label: 'Salt-okunur', hint: 'Yazma/komut engellenir', icon: '🔒' },
  { value: 'ask', label: 'Sor', hint: 'Yazma/komut için onay iste', icon: '✋' },
  { value: 'auto', label: 'Otomatik', hint: 'Tüm araçlar onaysız', icon: '⚡' },
]

// Trigger detection: what (if any) autocomplete menu the caret is currently in.
type Trigger =
  | { mode: 'agent'; query: string; from: number } // "@..." token start index
  | { mode: 'artifact'; query: string; from: number } // "#..." token start index
  | { mode: 'command'; query: string }
  | null

// MenuItem is one row in the autocomplete menu: an agent mention, a slash command
// or an artifact reference. The optional fields let a single array type cover all
// menu modes.
type MenuItem = {
  key: string
  label: string
  sub?: string
  agent?: Agent
  cmd?: SlashCommand
  artifact?: Artifact
}

function detectTrigger(value: string, caret: number): Trigger {
  const before = value.slice(0, caret)
  // "/" command palette — only when the whole input is a single "/word".
  if (before.startsWith('/') && !before.includes(' ')) {
    return { mode: 'command', query: before.slice(1) }
  }
  // "@" agent mention — last token at the caret starting with "@".
  const m = before.match(/(?:^|\s)@([^\s@]*)$/)
  if (m) {
    return { mode: 'agent', query: m[1], from: caret - m[1].length - 1 }
  }
  // "#" artifact reference — last token at the caret starting with "#".
  const a = before.match(/(?:^|\s)#([^\s#]*)$/)
  if (a) {
    return { mode: 'artifact', query: a[1], from: caret - a[1].length - 1 }
  }
  return null
}

// Composer is the chat input. Typing "@" opens an agent picker; typing "/" at
// the start opens the slash-command palette. Arrow keys navigate, Enter/Tab
// select, Esc closes.
export function Composer({
  disabled,
  sessionId,
  streaming = false,
  onSend,
  onStop,
  onInterrupt,
  onQueue,
  onSteer,
  thinkingLevel = '',
  onThinkingLevelChange,
  permissionMode = '',
  onPermissionModeChange,
  agents,
  commands,
  artifacts = [],
}: Props) {
  const [text, setText] = useState('')
  const [trigger, setTrigger] = useState<Trigger>(null)
  const [sel, setSel] = useState(0)
  const [pending, setPending] = useState<PendingAttachment[]>([])
  const [dragOver, setDragOver] = useState(false)
  const taRef = useRef<HTMLTextAreaElement>(null)
  const fileRef = useRef<HTMLInputElement>(null)
  // Monotonic id for pending attachments (avoids Date.now collisions on bursts).
  const seq = useRef(0)

  // uploadFiles uploads each file, tracking per-file progress in `pending`. Image
  // files get a local object-URL preview shown immediately. Requires a session.
  const uploadFiles = (files: File[]) => {
    if (!sessionId || files.length === 0) return
    for (const file of files) {
      const localId = `att-${seq.current++}`
      const isImage = file.type.startsWith('image/')
      const previewURL = isImage ? URL.createObjectURL(file) : undefined
      setPending((p) => [...p, { localId, name: file.name, previewURL, uploading: true }])
      api
        .uploadFile(sessionId, file)
        .then((attachment) => {
          setPending((p) =>
            p.map((x) => (x.localId === localId ? { ...x, uploading: false, attachment } : x)),
          )
        })
        .catch((e: unknown) => {
          setPending((p) =>
            p.map((x) =>
              x.localId === localId ? { ...x, uploading: false, error: (e as Error).message } : x,
            ),
          )
        })
    }
  }

  const removePending = (localId: string) => {
    const hit = pending.find((x) => x.localId === localId)
    if (hit?.previewURL) URL.revokeObjectURL(hit.previewURL)
    // Delete the already-uploaded file so a cancelled attachment is not orphaned
    // on disk (best-effort; the file only exists once its upload resolved).
    if (hit?.attachment?.relPath) api.deleteFile(hit.attachment.relPath).catch(() => {})
    setPending((p) => p.filter((x) => x.localId !== localId))
  }

  const onPickFiles = (e: React.ChangeEvent<HTMLInputElement>) => {
    uploadFiles(Array.from(e.target.files ?? []))
    e.target.value = '' // allow re-selecting the same file
  }

  // addArtifact stages an existing session artifact as a text attachment so its
  // content is included in the next turn. No upload: the content is inlined
  // directly (capped). De-dupes on the artifact id.
  const addArtifact = (a: Artifact) => {
    const localId = `art-${a.id}`
    if (pending.some((p) => p.localId === localId)) return
    let content = a.content ?? ''
    if (content.length > ARTIFACT_INLINE_CAP) {
      content = content.slice(0, ARTIFACT_INLINE_CAP) + '\n…(truncated)'
    }
    const attachment: Attachment = {
      id: localId,
      name: a.title || 'artifact',
      mime: 'text/plain',
      kind: a.kind === 'code' ? 'code' : 'text',
      size: content.length,
      textContent: content,
      source: 'artifact',
    }
    setPending((p) => [...p, { localId, name: attachment.name, uploading: false, attachment }])
  }

  // onPaste: large clipboard text becomes a .txt attachment (Claude.ai-style),
  // and pasted image data (e.g. a screenshot) is uploaded as an image. Plain
  // short text falls through to the textarea's normal paste.
  const onPaste = (e: React.ClipboardEvent) => {
    if (!sessionId) return
    const files = Array.from(e.clipboardData.files ?? [])
    if (files.length > 0) {
      e.preventDefault()
      uploadFiles(files)
      return
    }
    const txt = e.clipboardData.getData('text')
    if (txt && txt.length > PASTE_AS_FILE_THRESHOLD) {
      e.preventDefault()
      const file = new File([txt], 'pasted-text.txt', { type: 'text/plain' })
      uploadFiles([file])
    }
  }

  // Items currently shown in the open menu (filtered by the trigger query).
  const items = useMemo<MenuItem[]>(() => {
    if (!trigger) return []
    const q = trigger.query.toLowerCase()
    if (trigger.mode === 'agent') {
      return agents
        .filter((a) => a.name.toLowerCase().includes(q))
        .map((a): MenuItem => ({ key: a.id, label: a.name, sub: a.provider, agent: a }))
    }
    if (trigger.mode === 'artifact') {
      return artifacts
        .filter((a) => a.title.toLowerCase().includes(q))
        .map((a): MenuItem => ({ key: a.id, label: a.title || 'İsimsiz', sub: a.kind, artifact: a }))
    }
    return commands
      .filter((c) => c.name.toLowerCase().includes(q) || c.description.toLowerCase().includes(q))
      .map((c): MenuItem => ({ key: c.name, label: '/' + c.name, sub: c.description, cmd: c }))
  }, [trigger, agents, commands, artifacts])

  const updateTrigger = (value: string, caret: number) => {
    const t = detectTrigger(value, caret)
    setTrigger(t)
    setSel(0)
  }

  const onChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    setText(e.target.value)
    updateTrigger(e.target.value, e.target.selectionStart ?? e.target.value.length)
  }

  const closeMenu = () => setTrigger(null)

  const choose = (index: number) => {
    const item = items[index]
    if (!item) return
    if (item.agent) {
      // Insert a "@Name " mention at the trigger position — this turn routes to
      // the mentioned agent(s) (parsed on send).
      if (trigger?.mode === 'agent') {
        const caret = taRef.current?.selectionStart ?? text.length
        const mention = '@' + item.agent.name.replace(/\s+/g, '') + ' '
        const next = text.slice(0, trigger.from) + mention + text.slice(caret)
        setText(next)
      }
    } else if (item.artifact) {
      // Drop the "#query" token and stage the artifact as a content attachment.
      if (trigger?.mode === 'artifact') {
        const caret = taRef.current?.selectionStart ?? text.length
        setText(text.slice(0, trigger.from) + text.slice(caret))
      }
      addArtifact(item.artifact)
    } else if (item.cmd) {
      if (item.cmd.takesInput) {
        // Insert "/name " and wait for the user to type an argument + Enter
        // (handled in send()), instead of running immediately.
        setText('/' + item.cmd.name + ' ')
      } else {
        item.cmd.run()
        setText('') // a command consumes the input
      }
    }
    closeMenu()
    taRef.current?.focus()
  }

  // Attachments ready to send (upload finished, no error).
  const readyAttachments = pending.filter((p) => p.attachment && !p.error).map((p) => p.attachment!)
  const anyUploading = pending.some((p) => p.uploading)
  const hasContent = text.trim().length > 0 || readyAttachments.length > 0

  const clearComposer = () => {
    pending.forEach((p) => p.previewURL && URL.revokeObjectURL(p.previewURL))
    setText('')
    setPending([])
    closeMenu()
  }

  const send = () => {
    const t = text.trim()
    // Block while uploads are still in flight so attachments are never dropped.
    if (disabled || anyUploading || (!t && readyAttachments.length === 0)) return
    // "/name args…" → run the matching slash command with the trailing text as
    // its input, instead of sending a literal message. Unknown "/foo" falls
    // through and is sent as plain text.
    if (t.startsWith('/')) {
      const sp = t.indexOf(' ')
      const name = sp === -1 ? t.slice(1) : t.slice(1, sp)
      const rest = sp === -1 ? '' : t.slice(sp + 1)
      const cmd = commands.find((c) => c.name === name)
      if (cmd) {
        cmd.run(rest, readyAttachments)
        clearComposer()
        return
      }
    }
    onSend(t, readyAttachments)
    clearComposer()
  }

  // Streaming-turn actions (only when a turn is in flight). Each consumes the
  // input.
  const act = (fn?: (t: string) => void) => {
    const t = text.trim()
    if (!t || !fn) return
    fn(t)
    setText('')
    closeMenu()
  }
  const doQueue = () => act(onQueue)
  const doInterrupt = () => act(onInterrupt)
  const doSteer = () => act(onSteer)
  const hasText = text.trim().length > 0

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (trigger && items.length > 0) {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        setSel((s) => (s + 1) % items.length)
        return
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault()
        setSel((s) => (s - 1 + items.length) % items.length)
        return
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        e.preventDefault()
        choose(sel)
        return
      }
      if (e.key === 'Escape') {
        e.preventDefault()
        closeMenu()
        return
      }
    }
    // Shift+Tab cycles the per-turn permission mode (external-agent-oss signature),
    // but only when no autocomplete menu is open (Tab there selects an item).
    if (e.key === 'Tab' && e.shiftKey && onPermissionModeChange) {
      e.preventDefault()
      const i = PERMISSION_OPTIONS.findIndex((o) => o.value === permissionMode)
      const next = PERMISSION_OPTIONS[(i + 1) % PERMISSION_OPTIONS.length]
      onPermissionModeChange(next.value)
      return
    }
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      // While streaming, Enter queues the typed message (safest default) rather
      // than interrupting the in-flight turn.
      if (streaming) {
        if (hasText) doQueue()
      } else {
        send()
      }
    }
  }

  return (
    <div
      className={`relative border-t border-[var(--color-border)] bg-[var(--color-surface)] px-6 py-4 ${
        dragOver ? 'ring-2 ring-inset ring-[var(--color-accent)]' : ''
      }`}
      onDragOver={(e) => {
        if (!sessionId) return
        e.preventDefault()
        setDragOver(true)
      }}
      onDragLeave={() => setDragOver(false)}
      onDrop={(e) => {
        if (!sessionId) return
        e.preventDefault()
        setDragOver(false)
        uploadFiles(Array.from(e.dataTransfer.files ?? []))
      }}
    >
      {/* Autocomplete menu, anchored above the input. */}
      {trigger && items.length > 0 && (
        <div className="absolute bottom-full left-6 mb-2 max-h-64 w-80 overflow-y-auto rounded-xl border border-[var(--color-border)] bg-[var(--color-surface-2)] p-1 shadow-xl">
          <div className="px-2 py-1 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
            {trigger.mode === 'agent' ? 'Ajanlar' : trigger.mode === 'artifact' ? 'Artifactlar' : 'Komutlar'}
          </div>
          {items.map((it, i) => (
            <button
              key={it.key}
              onMouseEnter={() => setSel(i)}
              onClick={() => choose(i)}
              className={`flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm ${
                i === sel ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)]' : 'text-[var(--color-text-dim)]'
              }`}
            >
              {it.agent ? (
                <AgentAvatar agent={it.agent} size={22} />
              ) : it.artifact ? (
                <span className="flex h-[22px] w-[22px] shrink-0 items-center justify-center text-[var(--color-accent)]">
                  <Hash size={15} />
                </span>
              ) : (
                <span className="flex h-[22px] w-[22px] shrink-0 items-center justify-center">
                  {it.cmd?.icon ?? '⚡'}
                </span>
              )}
              <span className="flex min-w-0 flex-col">
                <span className="truncate font-medium text-[var(--color-text)]">{it.label}</span>
                {it.sub && <span className="truncate text-xs opacity-70">{it.sub}</span>}
              </span>
            </button>
          ))}
        </div>
      )}

      {/* Attachment tray: chips for files/pasted text staged for the next turn. */}
      {pending.length > 0 && (
        <div className="mb-2 flex flex-wrap gap-2.5">
          {pending.map((p) =>
            p.attachment ? (
              <AttachmentChip
                key={p.localId}
                attachment={p.attachment}
                previewURL={p.previewURL}
                onRemove={() => removePending(p.localId)}
              />
            ) : (
              <AttachmentChip
                key={p.localId}
                attachment={{ id: p.localId, name: p.name, mime: '', kind: 'file', size: 0 }}
                previewURL={p.previewURL}
                uploading={p.uploading}
                onRemove={() => removePending(p.localId)}
              />
            ),
          )}
        </div>
      )}

      <div className="flex w-full items-end gap-2">
        <ThinkingPicker value={thinkingLevel} onChange={onThinkingLevelChange} />
        <PermissionPicker value={permissionMode} onChange={onPermissionModeChange} />
        {/* Attach button + hidden multi-file input. */}
        <input ref={fileRef} type="file" multiple className="hidden" onChange={onPickFiles} />
        <button
          type="button"
          onClick={() => fileRef.current?.click()}
          disabled={!sessionId}
          title="Dosya ekle"
          className="rounded-xl border border-[var(--color-border)] px-2.5 py-3 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)] disabled:opacity-30"
        >
          <Paperclip size={18} />
        </button>
        <textarea
          ref={taRef}
          value={text}
          onChange={onChange}
          onKeyDown={onKeyDown}
          onPaste={onPaste}
          rows={1}
          placeholder="Mesaj yaz — @ ajan, # artifact, / komut, 📎 dosya"
          className="max-h-40 flex-1 resize-none rounded-xl border border-[var(--color-border)] bg-[var(--color-bg)] px-4 py-3 text-sm outline-none focus:border-[var(--color-accent)]"
        />
        {!streaming ? (
          <button
            onClick={send}
            disabled={disabled || !hasContent || anyUploading}
            className="rounded-xl bg-[var(--color-accent)] px-5 py-3 text-sm font-medium text-white transition hover:opacity-90 disabled:opacity-30"
          >
            Gönder
          </button>
        ) : hasText ? (
          // Input filled while streaming → queue / interrupt / steer.
          <div className="flex items-end gap-1.5">
            <button
              onClick={doQueue}
              title="Bu tur bitince gönder"
              className="rounded-xl border border-[var(--color-border)] px-3 py-3 text-sm font-medium text-[var(--color-text)] transition hover:border-[var(--color-accent)]"
            >
              Sıraya
            </button>
            <button
              onClick={doInterrupt}
              title="Turu kes ve hemen gönder"
              className="rounded-xl bg-[var(--color-warning)] px-3 py-3 text-sm font-medium text-white transition hover:opacity-90"
            >
              Kes
            </button>
            <button
              onClick={doSteer}
              title="Çalışan turu canlı yönlendir (araç döngüsünde etkili)"
              className="rounded-xl bg-[var(--color-accent)] px-3 py-3 text-sm font-medium text-white transition hover:opacity-90"
            >
              Yönlendir
            </button>
          </div>
        ) : (
          // Streaming, empty input → stop.
          <button
            onClick={onStop}
            title="Üretimi durdur"
            className="rounded-xl bg-[var(--color-danger)] px-5 py-3 text-sm font-medium text-white transition hover:opacity-90"
          >
            Durdur
          </button>
        )}
      </div>
    </div>
  )
}

// ThinkingPicker is the composer's reasoning-level selector: a compact button
// (🧠 + current label) that opens a small menu above it. The choice applies to
// the next message; the menu closes on select or outside click.
function ThinkingPicker({
  value,
  onChange,
}: {
  value: string
  onChange?: (v: string) => void
}) {
  const [open, setOpen] = useState(false)
  const rootRef = useRef<HTMLDivElement>(null)
  const current = THINKING_OPTIONS.find((o) => o.value === value) ?? THINKING_OPTIONS[0]

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onDown)
    return () => document.removeEventListener('mousedown', onDown)
  }, [open])

  return (
    <div ref={rootRef} className="relative shrink-0">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        title={`Düşünme seviyesi: ${current.label} — ${current.hint}`}
        className={`flex items-center gap-1 rounded-xl border px-2.5 py-3 text-sm transition ${
          value
            ? 'border-[var(--color-accent)] text-[var(--color-accent)]'
            : 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:text-[var(--color-accent)]'
        }`}
      >
        <Brain size={15} />
        <span className="hidden sm:inline">{current.label}</span>
      </button>

      {open && (
        <div className="absolute bottom-full left-0 mb-2 w-56 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface-2)] p-1 shadow-xl">
          <div className="px-2 py-1 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
            Düşünme seviyesi
          </div>
          {THINKING_OPTIONS.map((o) => (
            <button
              key={o.value || 'auto'}
              onClick={() => {
                onChange?.(o.value)
                setOpen(false)
              }}
              className={`flex w-full items-center justify-between gap-2 rounded-lg px-2 py-1.5 text-left text-sm ${
                o.value === value
                  ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)]'
                  : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface)]'
              }`}
            >
              <span className="font-medium text-[var(--color-text)]">{o.label}</span>
              <span className="truncate text-xs opacity-60">{o.hint}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

// PermissionPicker is the composer's per-turn permission-mode selector: a compact
// button (current icon + label) opening a menu above it. Shift+Tab cycles the
// same options without opening the menu. The choice applies to the next message.
function PermissionPicker({
  value,
  onChange,
}: {
  value: string
  onChange?: (v: string) => void
}) {
  const [open, setOpen] = useState(false)
  const rootRef = useRef<HTMLDivElement>(null)
  const current = PERMISSION_OPTIONS.find((o) => o.value === value) ?? PERMISSION_OPTIONS[0]

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onDown)
    return () => document.removeEventListener('mousedown', onDown)
  }, [open])

  return (
    <div ref={rootRef} className="relative shrink-0">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        title={`İzin modu: ${current.label} — ${current.hint} (Shift+Tab ile değiştir)`}
        className={`flex items-center gap-1 rounded-xl border px-2.5 py-3 text-sm transition ${
          value
            ? 'border-[var(--color-accent)] text-[var(--color-accent)]'
            : 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:text-[var(--color-accent)]'
        }`}
      >
        <span>{current.icon}</span>
        <span className="hidden sm:inline">{current.label}</span>
      </button>

      {open && (
        <div className="absolute bottom-full left-0 mb-2 w-60 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface-2)] p-1 shadow-xl">
          <div className="px-2 py-1 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
            İzin modu (Shift+Tab)
          </div>
          {PERMISSION_OPTIONS.map((o) => (
            <button
              key={o.value || 'auto-default'}
              onClick={() => {
                onChange?.(o.value)
                setOpen(false)
              }}
              className={`flex w-full items-center justify-between gap-2 rounded-lg px-2 py-1.5 text-left text-sm ${
                o.value === value
                  ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)]'
                  : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface)]'
              }`}
            >
              <span className="flex items-center gap-1.5 font-medium text-[var(--color-text)]">
                <span>{o.icon}</span>
                {o.label}
              </span>
              <span className="truncate text-xs opacity-60">{o.hint}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
