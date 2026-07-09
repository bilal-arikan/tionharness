// Display metadata for tool activity cards. Maps a (possibly namespaced) tool
// name to a short human label, a themed lucide icon and a one-line intent
// extracted from its JSON input — mirroring how the External Agent chat renders
// tool steps.
import type { LucideIcon } from 'lucide-react'
import { toolIcon } from '@/shared/lib/toolIcons'

export interface ToolMeta {
  label: string
  /** Themed lucide icon component for the tool (inherits currentColor). */
  icon: LucideIcon
  /** Short summary of what the call does, derived from its input. */
  summary: string
  /** When true the card body should render output as a unified diff. */
  isDiff: boolean
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

/** Pull a short human-readable string out of one array item, if any. */
function itemLabel(item: unknown): string | null {
  if (typeof item === 'string') return item
  if (item && typeof item === 'object') {
    const o = item as Record<string, unknown>
    const v =
      o.question ?? o.title ?? o.name ?? o.text ?? o.label ?? o.content ?? o.prompt ?? o.header
    if (typeof v === 'string') return v
  }
  return null
}

/** Summarize an array by joining its items' labels, or counting them. */
function summarizeArray(arr: unknown[]): string {
  if (arr.length === 0) return ''
  const labels = arr.map(itemLabel).filter((x): x is string => !!x)
  if (labels.length === 0) return `${arr.length} öğe`
  const shown = labels.slice(0, 3).join(', ')
  return labels.length > 3 ? `${shown} +${labels.length - 3}` : shown
}

// Array fields that are never the primary payload — showing them next to the
// tool name is noise (e.g. ask_user.options is the answer choices, not the
// question; archive_sessions.exclude is a filter). Skip them in the fallback.
const SECONDARY_ARRAY_KEYS = new Set([
  'options', 'tags', 'dependencies', 'exclude', 'args', 'spawntags', 'skills',
])

/** Derive a one-line summary from common tool input shapes. The goal is a
 *  content-bearing line (the actual command / path / message / items), never a
 *  raw key list — a bare "names" or "questions" next to the tool name is noise,
 *  so when nothing meaningful can be extracted we show nothing at all. */
function summarize(input: unknown): string {
  if (!input || typeof input !== 'object') {
    return typeof input === 'string' ? input : ''
  }
  if (Array.isArray(input)) return summarizeArray(input)
  const o = input as Record<string, unknown>
  // Prefer a genuine content value over structural keys.
  const first =
    o.command ?? o.query ?? o.url ?? o.path ?? o.file_path ?? o.pattern ?? o.q ?? o.agent ??
    o.slug ?? o.skill ?? o.question ?? o.title ?? o.name ?? o.message ?? o.text ?? o.prompt ??
    o.objective ?? o.symbol ?? o.template ?? o.worker ?? o.reason ?? o.to ?? o.recipient ?? o.input
  if (typeof first === 'string') return first
  // transform_data: show the interpreter and destination path (its actual
  // values) instead of the raw key list, so the card reads e.g.
  // "python3 → /tmp/result.json" rather than "language, output_file, script".
  if (typeof o.output_file === 'string') {
    const lang = typeof o.language === 'string' ? o.language : ''
    return lang ? `${lang} → ${o.output_file}` : o.output_file
  }
  // A bare board move (move_task / update_task with only a target column) reads
  // best as its destination: "→ done".
  if (typeof o.boardState === 'string') return `→ ${o.boardState}`
  // Array-valued fields carry the real payload for many tools
  // (activate_tools.names, ask_user.questions, todo_write.todos, …) — surface
  // the first meaningful one, skipping secondary/filter arrays, instead of its key.
  for (const [k, v] of Object.entries(o)) {
    if (Array.isArray(v) && v.length && !SECONDARY_ARRAY_KEYS.has(k.toLowerCase())) {
      return summarizeArray(v)
    }
  }
  // Nothing content-bearing — leave the side info blank rather than dumping keys.
  return ''
}

const DIFF_TOOLS = ['edit', 'write', 'multiedit', 'apply_patch']

export function toolMeta(name: string, input: unknown): ToolMeta {
  const base = baseName(name).toLowerCase()
  return {
    label: pickLabel(name),
    icon: toolIcon(name),
    summary: summarize(input),
    isDiff: DIFF_TOOLS.some((t) => base === t || base.includes(t)),
  }
}
