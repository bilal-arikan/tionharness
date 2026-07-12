import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { MessageCircleQuestion, Paperclip } from 'lucide-react'
import type { Agent, Artifact, Attachment, SlashCommand } from '@/types'
import { AttachmentChip } from './AttachmentChip'
import { WorkDirBadge } from './WorkDirBadge'
import { api } from '@/api'
import { PASTE_AS_FILE_THRESHOLD } from '@/shared/lib/attachments'
import { BTN_ICON } from './composer/buttonStyles'
import { AgentSelect } from './composer/AgentSelect'
import { ComposerPicker } from './composer/ComposerPicker'
import { THINKING_OPTIONS, PERMISSION_OPTIONS } from './composer/pickerOptions'
import { AutocompleteMenu } from './composer/AutocompleteMenu'
import { BtwPanel } from './composer/BtwPanel'
import { SendActions } from './composer/SendActions'
import { detectTrigger, buildMenuItems, type Trigger } from './composer/trigger'
import { useSessionDraft } from './useSessionDraft'

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
  // Id of a freshly-opened chat that should auto-focus the input. The composer
  // focuses only when sessionId === focusSessionId — i.e. right after the user
  // opens a NEW chat. Switching to an existing session (sessionId changes but
  // does not match) leaves the input un-focused so selection never steals focus.
  focusSessionId?: string | null
  // streaming: a turn is currently in flight. Changes the action buttons:
  // empty input → "Durdur"; filled input → Queue / Interrupt / Steer.
  streaming?: boolean
  // waiting: the turn ended into a pending self-wake (schedule_wake) — no turn is
  // in flight, but the conversation will auto-resume. The composer adopts the same
  // "busy" look as streaming and, when empty, offers a Durdur that cancels the wake.
  waiting?: boolean
  // onCancelWait disarms the pending self-wake (the waiting-state Durdur button).
  onCancelWait?: () => void
  onSend: (text: string, attachments: Attachment[]) => void
  onStop?: () => void
  onInterrupt?: (text: string) => void
  onQueue?: (text: string) => void
  onSteer?: (text: string) => void
  // Fired on keystrokes to broadcast a cross-window "user is typing" signal.
  onTyping?: () => void
  // Per-turn reasoning level ('' = agent default). Picked from a small menu in
  // the composer and applied to the next message.
  thinkingLevel?: string
  onThinkingLevelChange?: (v: string) => void
  // Per-turn permission-mode override ('' = agent default). Picked from a small
  // menu in the composer or cycled with Shift+Tab; applied to the next message.
  permissionMode?: string
  onPermissionModeChange?: (v: string) => void
  agents: Agent[]
  // The agent that receives the message (mandatory). Chosen from the composer
  // dropdown — the "@mention" routing was removed. '' = none selected (send is
  // blocked until an agent is picked).
  agentId: string
  onAgentChange: (id: string) => void
  commands: SlashCommand[]
  // Artifacts in the active session, offered by the "#" picker to include their
  // content in the next turn.
  artifacts?: Artifact[]
}

// Composer is the chat input. Typing "@" opens an agent picker; typing "/" at
// the start opens the slash-command palette. Arrow keys navigate, Enter/Tab
// select, Esc closes. Send-row controls, pickers and the autocomplete menu live
// in ./composer/*; this file owns the input state and wires them together.
export function Composer({
  disabled,
  sessionId,
  focusSessionId,
  streaming = false,
  waiting = false,
  onCancelWait,
  onSend,
  onStop,
  onInterrupt,
  onQueue,
  onSteer,
  onTyping,
  thinkingLevel = '',
  onThinkingLevelChange,
  permissionMode = '',
  onPermissionModeChange,
  agents,
  agentId,
  onAgentChange,
  commands,
  artifacts = [],
}: Props) {
  // Per-session draft: unsent text is persisted in localStorage keyed by session,
  // so it survives switching sessions and reloads. setText persists every edit;
  // sending/clearing the composer sets it to '' which removes the stored draft.
  const [text, setText] = useSessionDraft(sessionId)
  const [trigger, setTrigger] = useState<Trigger>(null)
  const [sel, setSel] = useState(0)
  const [pending, setPending] = useState<PendingAttachment[]>([])
  const [dragOver, setDragOver] = useState(false)
  // Btw side chat: an off-transcript, tool-less question answered against the
  // session's context. Deliberately usable WHILE a turn streams (that is the
  // point: "ask without interrupting the main task"), so it is not gated on
  // `streaming` / `disabled` — only on having a session and a target agent.
  const [btwOpen, setBtwOpen] = useState(false)
  const taRef = useRef<HTMLTextAreaElement>(null)
  const fileRef = useRef<HTMLInputElement>(null)
  // Monotonic id for pending attachments (avoids Date.now collisions on bursts).
  const seq = useRef(0)

  // The "Oto" (default '') option resolves to the selected agent's own setting.
  // Surface that resolved value on the option label, e.g. "Oto(Yüksek)" /
  // "Oto(Sor)", so the user can see what auto currently means without opening the
  // agent. When the agent has no explicit value the plain "Oto" label is kept.
  const selectedAgent = useMemo(
    () => agents.find((a) => a.id === agentId),
    [agents, agentId],
  )
  const thinkingOptions = useMemo(() => {
    const lvl = selectedAgent?.thinkingLevel
    const resolved = lvl ? THINKING_OPTIONS.find((o) => o.value === lvl)?.label : undefined
    return THINKING_OPTIONS.map((o) =>
      o.value === '' ? { ...o, label: resolved ? `Oto(${resolved})` : o.label } : o,
    )
  }, [selectedAgent])
  const permissionOptions = useMemo(() => {
    const mode = selectedAgent?.permissionMode
    const resolved = mode ? PERMISSION_OPTIONS.find((o) => o.value === mode)?.label : undefined
    return PERMISSION_OPTIONS.map((o) =>
      o.value === '' ? { ...o, label: resolved ? `Oto(${resolved})` : o.label } : o,
    )
  }, [selectedAgent])

  // Auto-grow the textarea with its content: reset to a single row, then expand to
  // fit the text. A CSS max-height (max-h-[12rem] ≈ 7 lines) caps the growth and
  // turns on the internal scrollbar beyond that, so the composer never pushes the
  // toolbar around. Runs on every text change (typing, draft restore, clear).
  useLayoutEffect(() => {
    const el = taRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${el.scrollHeight}px`
  }, [text])

  // Focus the input ONLY when the active session is a freshly-opened new chat
  // (sessionId matches the parent's focusSessionId). Switching to an existing
  // session changes sessionId but does not match, so plain selection never steals
  // focus. Skipped while disabled (no session / read-only state).
  useEffect(() => {
    if (disabled) return
    if (!sessionId || sessionId !== focusSessionId) return
    taRef.current?.focus()
  }, [sessionId, disabled, focusSessionId])

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
  const items = useMemo(
    () => buildMenuItems(trigger, agents, commands, artifacts),
    [trigger, agents, commands, artifacts],
  )

  const updateTrigger = (value: string, caret: number) => {
    setTrigger(detectTrigger(value, caret))
    setSel(0)
  }

  const onChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    setText(e.target.value)
    updateTrigger(e.target.value, e.target.selectionStart ?? e.target.value.length)
    // Cross-window "is typing" signal (throttled inside onTyping).
    if (e.target.value) onTyping?.()
  }

  const closeMenu = () => setTrigger(null)

  const choose = (index: number) => {
    const item = items[index]
    if (!item) return
    if (item.agent) {
      // Insert a "@Name " NAME REFERENCE at the trigger position. This is plain
      // text the recipient agent reads — it does NOT route the turn to the
      // mentioned agent (the recipient stays the composer's dropdown agent).
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
  const hasText = text.trim().length > 0
  // active = the turn is visually "live": either streaming now, or paused on a
  // pending self-wake that will auto-resume. Both give the composer the busy look.
  const active = streaming || waiting

  const clearComposer = () => {
    pending.forEach((p) => p.previewURL && URL.revokeObjectURL(p.previewURL))
    setText('')
    setPending([])
    closeMenu()
  }

  const send = () => {
    const t = text.trim()
    // Block while uploads are still in flight so attachments are never dropped,
    // and require a target agent (selection is mandatory; no "@mention").
    if (disabled || anyUploading || !agentId || (!t && readyAttachments.length === 0)) return
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

  // Streaming-turn actions (only when a turn is in flight). Each consumes the input.
  const act = (fn?: (t: string) => void) => {
    const t = text.trim()
    if (!t || !fn) return
    fn(t)
    setText('')
    closeMenu()
  }

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
        if (hasText) act(onQueue)
      } else {
        send()
      }
    }
  }

  return (
    <div
      className={`relative bg-gradient-to-t from-[var(--color-bg)] via-[color-mix(in_srgb,var(--color-bg)_85%,transparent)] to-transparent px-3 pt-3 pb-1.5 transition-shadow md:px-6 md:pt-4 md:pb-2 ${
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
      {trigger && items.length > 0 && (
        <AutocompleteMenu
          mode={trigger.mode}
          items={items}
          sel={sel}
          onHover={setSel}
          onChoose={choose}
        />
      )}

      {/* Btw side chat. Rendered only with a session + agent (both are required to
          answer), and stays open across a streaming turn on purpose. */}
      {btwOpen && sessionId && agentId && (
        <BtwPanel sessionId={sessionId} agentId={agentId} onClose={() => setBtwOpen(false)} />
      )}

      {/* Attachment tray: chips for files/pasted text staged for the next turn. */}
      {pending.length > 0 && (
        <div className="mb-2 flex flex-wrap gap-2.5">
          {pending.map((p) => (
            <AttachmentChip
              key={p.localId}
              attachment={
                p.attachment ?? { id: p.localId, name: p.name, mime: '', kind: 'file', size: 0 }
              }
              previewURL={p.previewURL}
              uploading={p.attachment ? undefined : p.uploading}
              onRemove={() => removePending(p.localId)}
            />
          ))}
        </div>
      )}

      {/* Input card: the textarea grows (up to ~3 lines) on its own full-width row;
          the controls live on a fixed toolbar row beneath it, so they never stretch
          or shift as the text area expands. The card carries the border/focus ring. */}
      <div
        className={`flex flex-col gap-2 rounded-2xl border bg-[var(--color-surface)] px-3 pb-2 pt-2.5 shadow-lg transition-colors focus-within:border-[var(--color-accent)] ${
          active
            ? 'border-[color-mix(in_srgb,var(--color-accent)_55%,var(--color-border))]'
            : 'border-[var(--color-border)]'
        }`}
      >
        <textarea
          ref={taRef}
          value={text}
          onChange={onChange}
          onKeyDown={onKeyDown}
          onPaste={onPaste}
          data-testid="composer-input"
          aria-label="Mesaj yaz"
          rows={1}
          placeholder={
            waiting
              ? 'Otomatik devam bekleniyor — yazarsan konuşmayı devralırsın'
              : 'Mesaj yaz — @ ajan adı, # artifact, / komut, 📎 dosya'
          }
          className="max-h-[12rem] w-full resize-none overflow-y-auto bg-transparent px-1 py-0.5 text-sm leading-5 outline-none placeholder:text-[var(--color-text-dim)]"
        />

        {/* Toolbar row: left = target agent + per-turn pickers + workdir + attach;
            right = send/streaming actions. Wraps on narrow (mobile) widths so the
            send cluster drops to its own line instead of overflowing the viewport. */}
        <div className="flex flex-wrap items-center gap-1.5">
          <AgentSelect
            agents={agents}
            value={agentId}
            onChange={onAgentChange}
            disabled={!sessionId}
          />
          <ComposerPicker
            value={thinkingLevel}
            onChange={onThinkingLevelChange}
            options={thinkingOptions}
            header="Düşünme seviyesi"
            title={(c) => `Düşünme seviyesi: ${c.label} — ${c.hint}`}
          />
          <ComposerPicker
            value={permissionMode}
            onChange={onPermissionModeChange}
            options={permissionOptions}
            header="İzin modu (Shift+Tab)"
            title={(c) => `İzin modu: ${c.label} — ${c.hint} (Shift+Tab ile değiştir)`}
            menuWidthClass="w-60"
            iconOnly
          />
          <WorkDirBadge sessionId={sessionId} />
          {/* Attach button + hidden multi-file input. */}
          <input ref={fileRef} type="file" multiple className="hidden" onChange={onPickFiles} />
          <button
            type="button"
            onClick={() => fileRef.current?.click()}
            disabled={!sessionId}
            title="Dosya ekle"
            aria-label="Dosya ekle"
            data-testid="composer-attach"
            className={BTN_ICON}
          >
            <Paperclip size={18} />
          </button>

          {/* Btw: a side question answered from the conversation's context but never
              written into it. Enabled even while a turn is streaming — asking one is
              exactly what this button is for. Needs a session + a target agent. */}
          <button
            type="button"
            onClick={() => setBtwOpen((o) => !o)}
            disabled={!sessionId || !agentId}
            title="Btw — yan soru sor (geçmişe yazılmaz, ana görevi kesmez)"
            aria-label="Btw yan soru"
            aria-expanded={btwOpen}
            data-testid="composer-btw"
            className={`${BTN_ICON} ${btwOpen ? 'border-[var(--color-accent)] text-[var(--color-accent)]' : ''}`}
          >
            <MessageCircleQuestion size={18} />
          </button>

          {/* Spacer pushes the send cluster to the right edge. */}
          <div className="flex-1" />

          <SendActions
            streaming={streaming}
            waiting={waiting}
            hasText={hasText}
            hasContent={hasContent}
            anyUploading={anyUploading}
            disabled={disabled || !agentId}
            onSend={send}
            onStop={onStop}
            onCancelWait={onCancelWait}
            onQueue={() => act(onQueue)}
            onInterrupt={() => act(onInterrupt)}
            onSteer={() => act(onSteer)}
          />
        </div>
      </div>
    </div>
  )
}
