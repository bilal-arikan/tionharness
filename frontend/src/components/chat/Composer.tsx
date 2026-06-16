import { useEffect, useMemo, useRef, useState } from 'react'
import type { Agent, SlashCommand } from '../../types'
import { AgentAvatar } from '../agents/AgentAvatar'

interface Props {
  disabled: boolean
  // streaming: a turn is currently in flight. Changes the action buttons:
  // empty input → "Durdur"; filled input → Queue / Interrupt / Steer.
  streaming?: boolean
  onSend: (text: string) => void
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
  | { mode: 'command'; query: string }
  | null

// MenuItem is one row in the autocomplete menu: an agent mention or a slash
// command. Both `agent` and `cmd` are optional so a single array type covers
// both menu modes.
type MenuItem = { key: string; label: string; sub?: string; agent?: Agent; cmd?: SlashCommand }

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
  return null
}

// Composer is the chat input. Typing "@" opens an agent picker; typing "/" at
// the start opens the slash-command palette. Arrow keys navigate, Enter/Tab
// select, Esc closes.
export function Composer({
  disabled,
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
}: Props) {
  const [text, setText] = useState('')
  const [trigger, setTrigger] = useState<Trigger>(null)
  const [sel, setSel] = useState(0)
  const taRef = useRef<HTMLTextAreaElement>(null)

  // Items currently shown in the open menu (filtered by the trigger query).
  const items = useMemo<MenuItem[]>(() => {
    if (!trigger) return []
    const q = trigger.query.toLowerCase()
    if (trigger.mode === 'agent') {
      return agents
        .filter((a) => a.name.toLowerCase().includes(q))
        .map((a): MenuItem => ({ key: a.id, label: a.name, sub: a.provider, agent: a }))
    }
    return commands
      .filter((c) => c.name.toLowerCase().includes(q) || c.description.toLowerCase().includes(q))
      .map((c): MenuItem => ({ key: c.name, label: '/' + c.name, sub: c.description, cmd: c }))
  }, [trigger, agents, commands])

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
    } else if (item.cmd) {
      item.cmd.run()
      setText('') // a command consumes the input
    }
    closeMenu()
    taRef.current?.focus()
  }

  const send = () => {
    const t = text.trim()
    if (!t || disabled) return
    onSend(t)
    setText('')
    closeMenu()
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
    <div className="relative border-t border-[var(--color-border)] bg-[var(--color-surface)] px-6 py-4">
      {/* Autocomplete menu, anchored above the input. */}
      {trigger && items.length > 0 && (
        <div className="absolute bottom-full left-6 mb-2 max-h-64 w-80 overflow-y-auto rounded-xl border border-[var(--color-border)] bg-[var(--color-surface-2)] p-1 shadow-xl">
          <div className="px-2 py-1 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
            {trigger.mode === 'agent' ? 'Ajanlar' : 'Komutlar'}
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

      <div className="flex w-full items-end gap-2">
        <ThinkingPicker value={thinkingLevel} onChange={onThinkingLevelChange} />
        <PermissionPicker value={permissionMode} onChange={onPermissionModeChange} />
        <textarea
          ref={taRef}
          value={text}
          onChange={onChange}
          onKeyDown={onKeyDown}
          rows={1}
          placeholder="Mesaj yaz — @ ile ajan, / ile komut"
          className="max-h-40 flex-1 resize-none rounded-xl border border-[var(--color-border)] bg-[var(--color-bg)] px-4 py-3 text-sm outline-none focus:border-[var(--color-accent)]"
        />
        {!streaming ? (
          <button
            onClick={send}
            disabled={disabled || !hasText}
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
              className="rounded-xl bg-amber-500/90 px-3 py-3 text-sm font-medium text-white transition hover:opacity-90"
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
            className="rounded-xl bg-red-500/90 px-5 py-3 text-sm font-medium text-white transition hover:opacity-90"
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
        <span>🧠</span>
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
