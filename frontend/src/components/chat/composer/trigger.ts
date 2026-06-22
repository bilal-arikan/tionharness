import type { Agent, Artifact, SlashCommand } from '../../../types'

// Trigger detection: what (if any) autocomplete menu the caret is currently in.
// "@" inserts a NAME REFERENCE to an agent into the message text — it does NOT
// route the turn to that agent (the recipient is the composer's dropdown agent);
// it only lets you point at who you mean. "#" references artifacts, "/" commands.
export type Trigger =
  | { mode: 'agent'; query: string; from: number } // "@..." token start index
  | { mode: 'artifact'; query: string; from: number } // "#..." token start index
  | { mode: 'command'; query: string }
  | null

// MenuItem is one row in the autocomplete menu: an agent name reference, a slash
// command or an artifact reference. The optional fields let a single array type
// cover all menu modes.
export type MenuItem = {
  key: string
  label: string
  sub?: string
  agent?: Agent
  cmd?: SlashCommand
  artifact?: Artifact
}

// detectTrigger inspects the text before the caret and reports which autocomplete
// menu (if any) should be open.
export function detectTrigger(value: string, caret: number): Trigger {
  const before = value.slice(0, caret)
  // "/" command palette — only when the whole input is a single "/word".
  if (before.startsWith('/') && !before.includes(' ')) {
    return { mode: 'command', query: before.slice(1) }
  }
  // "@" agent name reference — last token at the caret starting with "@".
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

// buildMenuItems filters the trigger's source list by its query into menu rows.
export function buildMenuItems(
  trigger: Trigger,
  agents: Agent[],
  commands: SlashCommand[],
  artifacts: Artifact[],
): MenuItem[] {
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
}
