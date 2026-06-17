// Display metadata for tool activity cards. Maps a (possibly namespaced) tool
// name to a short human label, an icon glyph and a one-line intent extracted
// from its JSON input — mirroring how the External Agent chat renders tool steps.

export interface ToolMeta {
  label: string
  icon: string
  /** Short summary of what the call does, derived from its input. */
  summary: string
  /** When true the card body should render output as a unified diff. */
  isDiff: boolean
}

// Per-tool icon glyphs (emoji keeps us dependency-free vs. an icon set).
const ICONS: Record<string, string> = {
  get_current_time: '🕐',
  http_get: '🌐',
  http_request: '🌐',
  memory_recall: '🧠',
  memory_remember: '🧠',
  read: '📄',
  write: '✏️',
  edit: '✏️',
  bash: '▶️',
  terminal: '▶️',
  grep: '🔎',
  glob: '🗂️',
  browser: '🧭',
  todo_write: '✅',
  ask_user: '💬',
  call_agent: '🤝',
}

/** Strip an MCP namespace prefix (`server__tool`) for display. */
function baseName(name: string): string {
  const i = name.lastIndexOf('__')
  return i >= 0 ? name.slice(i + 2) : name
}

/** Lower-cased base tool name with any MCP namespace prefix removed. */
export function toolBase(name: string): string {
  return baseName(name).toLowerCase()
}

/** True for read-only file readers whose output is the file content. */
export function isReadTool(name: string): boolean {
  const base = toolBase(name)
  return base === 'read' || base === 'read_file'
}

function pickIcon(name: string): string {
  const base = baseName(name).toLowerCase()
  for (const key of Object.keys(ICONS)) {
    if (base === key || base.startsWith(key) || base.includes(key)) return ICONS[key]
  }
  if (name.startsWith('mcp__') || name.includes('__')) return '🧩'
  return '🛠️'
}

/** A readable label: "Memory Recall", "server · tool" for MCP. */
function pickLabel(name: string): string {
  const i = name.lastIndexOf('__')
  if (i >= 0) {
    const server = name.slice(0, i).replace(/^mcp__/, '')
    const tool = name.slice(i + 2)
    return `${server} · ${humanize(tool)}`
  }
  return humanize(name)
}

function humanize(s: string): string {
  return s
    .replace(/[_-]+/g, ' ')
    .replace(/\b\w/g, (c) => c.toUpperCase())
    .trim()
}

/** Derive a one-line summary from common tool input shapes. */
function summarize(input: unknown): string {
  if (!input || typeof input !== 'object') {
    return typeof input === 'string' ? input : ''
  }
  const o = input as Record<string, unknown>
  const first =
    o.command ?? o.query ?? o.url ?? o.path ?? o.file_path ?? o.pattern ?? o.q ?? o.agent ?? o.input
  if (typeof first === 'string') return first
  // Fall back to a compact key list.
  const keys = Object.keys(o)
  return keys.length ? keys.join(', ') : ''
}

const DIFF_TOOLS = ['edit', 'write', 'multiedit', 'apply_patch']

export function toolMeta(name: string, input: unknown): ToolMeta {
  const base = baseName(name).toLowerCase()
  return {
    label: pickLabel(name),
    icon: pickIcon(name),
    summary: summarize(input),
    isDiff: DIFF_TOOLS.some((t) => base === t || base.includes(t)),
  }
}
