import { useMemo, useRef, useState } from 'react'
import type { Agent, SlashCommand } from '../types'
import { AgentAvatar } from './AgentAvatar'

interface Props {
  disabled: boolean
  onSend: (text: string) => void
  agents: Agent[]
  commands: SlashCommand[]
}

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
export function Composer({ disabled, onSend, agents, commands }: Props) {
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
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      send()
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
        <textarea
          ref={taRef}
          value={text}
          onChange={onChange}
          onKeyDown={onKeyDown}
          rows={1}
          placeholder="Mesaj yaz — @ ile ajan, / ile komut"
          className="max-h-40 flex-1 resize-none rounded-xl border border-[var(--color-border)] bg-[var(--color-bg)] px-4 py-3 text-sm outline-none focus:border-[var(--color-accent)]"
        />
        <button
          onClick={send}
          disabled={disabled || !text.trim()}
          className="rounded-xl bg-[var(--color-accent)] px-5 py-3 text-sm font-medium text-white transition hover:opacity-90 disabled:opacity-30"
        >
          Gönder
        </button>
      </div>
    </div>
  )
}
