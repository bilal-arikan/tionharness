import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { MessageCircleQuestion, Paperclip, SlidersHorizontal, Wrench } from 'lucide-react'
import type { Agent, Artifact, Attachment, SlashCommand } from '@/types'
import { AttachmentChip } from './AttachmentChip'
import { WorkDirBadge } from './WorkDirBadge'
import { api } from '@/api'
import { PASTE_AS_FILE_THRESHOLD } from '@/shared/lib/attachments'
import { useCatalog, thinkingInfoForModel, thinkingTierDisabledReason } from '@/shared/lib/catalog'
import { BTN_ICON } from './composer/buttonStyles'
import { AgentSelect } from './composer/AgentSelect'
import { ComposerPicker } from './composer/ComposerPicker'
import { THINKING_OPTIONS, PERMISSION_OPTIONS } from './composer/pickerOptions'
import { AutocompleteMenu } from './composer/AutocompleteMenu'
import { BtwPanel } from './composer/BtwPanel'
import { MicButton } from './composer/MicButton'
import { ToolAccessPanel } from './composer/ToolAccessPanel'
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
  // Submitting a message is asynchronous: the composer awaits this and locks its
  // input until it settles. Resolving to false means the send failed — the
  // composer then keeps the text so the user can retry without retyping.
  onSend: (text: string, attachments: Attachment[]) => void | Promise<boolean | void>
  onStop?: () => void
  // Same contract as onSend: resolving to false keeps the draft in the composer.
  onInterrupt?: (text: string, attachments: Attachment[]) => void | Promise<boolean | void>
  onQueue?: (text: string, attachments: Attachment[]) => void | Promise<boolean | void>
  onSteer?: (text: string) => void | Promise<boolean | void>
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
  // A submit is in flight (the enqueue round-trip). The input is locked until it
  // settles so the same draft cannot be submitted twice; on failure the text is
  // kept (see `send`) and the lock is released for a retry.
  const [sending, setSending] = useState(false)
  // Btw side chat: an off-transcript, tool-less question answered against the
  // session's context. Deliberately usable WHILE a turn streams (that is the
  // point: "ask without interrupting the main task"), so it is not gated on
  // `streaming` / `disabled` — only on having a session and a target agent.
  const [btwOpen, setBtwOpen] = useState(false)
  // Tool inspector: a read-only view of what the selected agent can use right now
  // (eager vs on-demand tools) and what the MCP gateway has open. Informational
  // only — it never changes configuration, so it stays usable while streaming.
  const [toolsOpen, setToolsOpen] = useState(false)
  // On narrow/portrait widths the per-turn pickers (thinking, permission, workdir)
  // are collapsed behind a toggle to keep the toolbar from wrapping; they are
  // always shown from `md:` up. Preference persists across sessions/reloads.
  const [showControls, setShowControls] = useState(
    () => localStorage.getItem('tionharness.composerControlsOpen') === '1',
  )
  const toggleControls = () =>
    setShowControls((v) => {
      const next = !v
      localStorage.setItem('tionharness.composerControlsOpen', next ? '1' : '0')
      return next
    })
  const taRef = useRef<HTMLTextAreaElement>(null)
  const fileRef = useRef<HTMLInputElement>(null)
  // Mirrors the latest text so voice-dictation callbacks append to the current
  // draft (the draft setter takes a plain string, not a functional updater, and
  // the callback would otherwise close over a stale value across rapid chunks).
  const textRef = useRef(text)
  // Monotonic id for pending attachments (avoids Date.now collisions on bursts).
  const seq = useRef(0)

  // The "Oto" (default '') option resolves to the selected agent's own setting.
  // Surface that resolved value on the option label, e.g. "Oto(Yüksek)" /
  // "Oto(Sor)", so the user can see what auto currently means without opening the
  // agent. When the agent has no explicit value the plain "Oto" label is kept.
  const selectedAgent = useMemo(() => agents.find((a) => a.id === agentId), [agents, agentId])
  // Reasoning tiers + class for the selected agent's model. tiers null =
  // unknown/custom → every tier stays enabled (the provider clamps anything the
  // concrete model can't honour).
  const catalog = useCatalog()
  const thinking = useMemo(
    () =>
      selectedAgent
        ? thinkingInfoForModel(catalog, selectedAgent.provider, selectedAgent.model)
        : { tiers: null, cls: '' },
    [catalog, selectedAgent],
  )
  const thinkingOptions = useMemo(() => {
    const lvl = selectedAgent?.thinkingLevel
    const resolved = lvl ? THINKING_OPTIONS.find((o) => o.value === lvl)?.label : undefined
    const { tiers, cls } = thinking
    return THINKING_OPTIONS.map((o) => {
      // "Oto" defers to the agent and whatever this turn is already set to stays
      // selectable; every other unsupported tier is shown greyed with a reason.
      const supported =
        o.value === '' || o.value === thinkingLevel || tiers == null || tiers.includes(o.value)
      const withLabel =
        o.value === '' ? { ...o, label: resolved ? `Oto(${resolved})` : o.label } : o
      return supported
        ? withLabel
        : { ...withLabel, disabled: true, hint: thinkingTierDisabledReason(cls, o.value) }
    })
  }, [selectedAgent, thinking, thinkingLevel])
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

  // Keep textRef in sync so voice-dictation callbacks read the current draft.
  useEffect(() => {
    textRef.current = text
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

  // appendTranscript inserts a finalized voice-dictation chunk at the end of the
  // draft, separated by a space, then re-runs trigger detection and the typing
  // signal exactly as a keystroke would.
  const appendTranscript = (chunk: string) => {
    const prev = textRef.current
    const sep = prev && !/\s$/.test(prev) ? ' ' : ''
    const next = prev + sep + chunk
    setText(next)
    updateTrigger(next, next.length)
    onTyping?.()
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
        // A command consumes the input — but only once it succeeded. Lock the
        // composer while it runs so the menu selection cannot be fired twice.
        const cmd = item.cmd
        setSending(true)
        void (async () => {
          try {
            const ok = await cmd.run()
            if (ok !== false) setText('')
          } finally {
            setSending(false)
            taRef.current?.focus()
          }
        })()
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

  const send = async () => {
    const t = text.trim()
    // Block while uploads are still in flight so attachments are never dropped,
    // and require a target agent (selection is mandatory; no "@mention").
    if (disabled || sending || anyUploading || !agentId || (!t && readyAttachments.length === 0))
      return
    // "/name args…" → run the matching slash command with the trailing text as
    // its input, instead of sending a literal message. Unknown "/foo" falls
    // through and is sent as plain text.
    if (t.startsWith('/')) {
      const sp = t.indexOf(' ')
      const name = sp === -1 ? t.slice(1) : t.slice(1, sp)
      const rest = sp === -1 ? '' : t.slice(sp + 1)
      const cmd = commands.find((c) => c.name === name)
      if (cmd) {
        setSending(true)
        try {
          const ok = await cmd.run(rest, readyAttachments)
          if (ok !== false) clearComposer()
        } finally {
          setSending(false)
          taRef.current?.focus()
        }
        return
      }
    }
    // Async submit: lock the input until the send settles. Clear it only when the
    // message was accepted — a failed send keeps the draft (and its attachments)
    // in place so the user can retry without retyping.
    setSending(true)
    try {
      const ok = await onSend(t, readyAttachments)
      if (ok !== false) clearComposer()
    } finally {
      setSending(false)
      // Disabling the textarea drops focus; give it back so typing continues.
      taRef.current?.focus()
    }
  }

  // Streaming-turn actions that carry the composer's payload (Sıraya / Kes): they
  // run as a full turn later, so the attachments must travel with the text —
  // clearing only the text would strand the uploaded files in the composer and
  // silently send an attachment-less turn.
  // Like `send`, these are awaited and the composer stays locked-and-filled until
  // the action is accepted: a failed queue/interrupt must not eat the draft.
  const actWithAttachments = async (
    fn?: (t: string, a: Attachment[]) => void | Promise<boolean | void>,
  ) => {
    if (!fn || sending || anyUploading) return
    const t = text.trim()
    if (!t && readyAttachments.length === 0) return
    setSending(true)
    try {
      const ok = await fn(t, readyAttachments)
      if (ok !== false) clearComposer()
    } finally {
      setSending(false)
      taRef.current?.focus()
    }
  }

  // Text-only streaming action (Yönlendir): live guidance is injected into the
  // running turn and cannot carry files, so pending attachments stay put.
  const act = async (fn?: (t: string) => void | Promise<boolean | void>) => {
    const t = text.trim()
    if (!t || !fn || sending) return
    setSending(true)
    try {
      const ok = await fn(t)
      if (ok !== false) {
        setText('')
        closeMenu()
      }
    } finally {
      setSending(false)
      taRef.current?.focus()
    }
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
        if (hasContent) void actWithAttachments(onQueue)
      } else {
        void send()
      }
    }
  }

  return (
    <div
      className={`relative bg-gradient-to-t from-[var(--color-bg)] via-[color-mix(in_srgb,var(--color-bg)_85%,transparent)] to-transparent px-1 pt-3 pb-1.5 transition-shadow md:px-6 md:pt-4 md:pb-2 ${
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

      {/* Tool inspector. Needs a target agent (tool access is per-agent); no
          session required, so it also answers "what could this agent do?". */}
      {toolsOpen && agentId && (
        <ToolAccessPanel key={agentId} agentId={agentId} onClose={() => setToolsOpen(false)} />
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
        className={`flex flex-col gap-2 rounded-2xl border bg-[var(--color-surface)] px-3 pb-2 pt-2.5 shadow-lg transition-colors focus-within:ring-1 focus-within:ring-inset focus-within:ring-[var(--color-accent)] ${
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
          disabled={sending}
          data-testid="composer-input"
          aria-label="Mesaj yaz"
          rows={1}
          placeholder={
            waiting
              ? 'Otomatik devam bekleniyor — yazarsan konuşmayı devralırsın'
              : 'Mesaj yaz — @ ajan adı, # artifact, / komut'
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
          {/* Narrow-screen toggle: reveals/hides the per-turn pickers below. Hidden
              from `md:` up, where the pickers are always shown. */}
          <button
            type="button"
            onClick={toggleControls}
            title="Tur ayarları (düşünme · izin · çalışma dizini · araçlar)"
            aria-label="Tur ayarlarını göster/gizle"
            aria-expanded={showControls}
            data-testid="composer-controls-toggle"
            className={`${BTN_ICON} md:hidden ${showControls ? 'border-[var(--color-accent)] text-[var(--color-accent)]' : ''}`}
          >
            <SlidersHorizontal size={18} />
          </button>
          {/* Per-turn pickers. `contents` keeps them as direct flex children (so the
              toolbar gap is unaffected); collapsed on narrow widths unless toggled,
              always shown from `md:` up. */}
          <div className={`${showControls ? 'contents' : 'hidden'} md:contents`}>
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
            {/* Tool inspector: what this agent can use right now (active vs
                on-demand) and what the MCP gateway has open. Read-only, so it is
                never disabled by a streaming turn — only by having no agent to
                inspect. Folded into this group so the narrow-width toolbar hides
                it with the rest of the per-turn controls; collapsing the group
                also closes an open panel, since the ⚙ toggle is not tagged
                data-tool-access-toggle and so counts as an outside click. */}
            <button
              type="button"
              onClick={() => setToolsOpen((o) => !o)}
              disabled={!agentId}
              title="Araçlar — bu ajanın kullanabildiği araçlar ve MCP durumu (salt bilgi)"
              aria-label="Araç bilgisi"
              aria-expanded={toolsOpen}
              data-testid="composer-tools"
              data-tool-access-toggle=""
              className={`${BTN_ICON} ${toolsOpen ? 'border-[var(--color-accent)] text-[var(--color-accent)]' : ''}`}
            >
              <Wrench size={18} />
            </button>
          </div>
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

          {/* Voice dictation: language picker + mic toggle. Speaking appends
              recognized text to the draft. Renders nothing when the browser lacks
              Web Speech recognition. Needs a session (nothing to dictate into). */}
          <MicButton disabled={!sessionId} onTranscript={appendTranscript} />

          {/* Spacer pushes the send cluster to the right edge. */}
          <div className="flex-1" />

          <SendActions
            streaming={streaming}
            waiting={waiting}
            hasText={hasText}
            hasContent={hasContent}
            anyUploading={anyUploading}
            disabled={disabled || sending || !agentId}
            onSend={() => void send()}
            onStop={onStop}
            onCancelWait={onCancelWait}
            onQueue={() => void actWithAttachments(onQueue)}
            onInterrupt={() => void actWithAttachments(onInterrupt)}
            onSteer={() => void act(onSteer)}
          />
        </div>
      </div>
    </div>
  )
}
