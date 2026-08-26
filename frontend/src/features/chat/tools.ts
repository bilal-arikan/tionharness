// Display metadata for tool activity cards. Maps a (possibly namespaced) tool
// name to a short human label, a themed lucide icon and a one-line intent
// extracted from its JSON input — mirroring how the External Agent chat renders
// tool steps.
import type { LucideIcon } from 'lucide-react'
import { stripShellHost } from '@/shared/lib/commandProgram'
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

/** True for shell tools whose stdout/stderr is fed back into the context. */
export function isShellTool(name: string): boolean {
  const base = toolBase(name)
  return base === 'bash' || base === 'powershell'
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
  'options',
  'tags',
  'dependencies',
  'exclude',
  'args',
  'spawntags',
  'skills',
])

/** A field's value as a string, or '' when absent/non-string. */
function str(v: unknown): string {
  return typeof v === 'string' ? v : ''
}

/** "actor → payload" (or just whichever half is present). */
function directed(actor: unknown, payload: unknown): string | null {
  const a = str(actor)
  const p = str(payload)
  if (a && p) return `${a} → ${p}`
  return a || p || null
}

// Per-tool summary templates, tried before the generic extraction. Each maps a
// call's input to a richer one-liner that names both sides of the action —
// e.g. "@Ali → merhaba" for a message, "Fix login [in_progress]" for a task —
// so the collapsed card reads like a sentence, not a lone value.
const RICH_TEMPLATES: Record<string, (o: Record<string, unknown>) => string | null> = {
  send_message: (o) => directed(o.to, o.message ?? o.summary),
  send_to_worker: (o) => directed(o.worker, o.message),
  spawn_worker: (o) => directed(o.agent, o.task),
  spawn_session: (o) => directed(o.agent, o.prompt),
  create_task: (o) => taskLine(o),
  update_task: (o) => taskLine(o),
  read_logs: (o) => str(o.q) || str(o.level) || null,
  create_schedule: (o) => scheduleLine(o),
  update_schedule: (o) => scheduleLine(o),
  create_hook: (o) => directed(o.event, o.command),
  create_mcp_server: (o) => directed(o.name, o.command ?? o.url),
  apply_patch: (o) => patchSummary(o),
}

// "<path>" for a single-file patch, "<path> +N" for a multi-file one — parsed
// from the unified diff's own `+++ b/<path>` headers, since apply_patch's
// input carries only the raw patch text (no separate path field).
function patchSummary(o: Record<string, unknown>): string | null {
  const patch = typeof o.patch === 'string' ? o.patch : ''
  if (!patch) return null
  const paths = Array.from(patch.matchAll(/^\+\+\+ [ab]\/(.+?)(?:\t|$)/gm)).map((m) => m[1])
  if (paths.length === 0) return null
  return paths.length === 1 ? paths[0] : `${paths[0]} +${paths.length - 1}`
}

/** "title [column]" for board tasks; falls back to whichever part exists. */
function taskLine(o: Record<string, unknown>): string | null {
  const head = str(o.title) || str(o.prompt)
  const state = str(o.boardState)
  if (head && state) return `${head} [${state}]`
  if (state) return `→ ${state}`
  return head || null
}

/** "prompt · cron" for schedules; either part alone is fine. */
function scheduleLine(o: Record<string, unknown>): string | null {
  const p = str(o.prompt)
  const cron = str(o.cronExpr)
  if (p && cron) return `${p} · ${cron}`
  return p || cron || null
}

/** Derive a one-line summary from common tool input shapes. The goal is a
 *  content-bearing line (the actual command / path / message / items), never a
 *  raw key list — a bare "names" or "questions" next to the tool name is noise,
 *  so when nothing meaningful can be extracted we show nothing at all. A
 *  per-tool template (RICH_TEMPLATES) is preferred when one matches the base. */
function summarize(base: string, input: unknown): string {
  if (!input || typeof input !== 'object') {
    return typeof input === 'string' ? input : ''
  }
  if (Array.isArray(input)) return summarizeArray(input)
  const o = input as Record<string, unknown>
  const rich = RICH_TEMPLATES[base]?.(o)
  if (rich) return rich
  // Prefer a genuine content value over structural keys.
  const first =
    (typeof o.command === 'string' ? stripShellHost(o.command) : o.command) ??
    o.query ??
    o.url ??
    o.path ??
    o.file_path ??
    o.pattern ??
    o.q ??
    o.agent ??
    o.slug ??
    o.skill ??
    o.question ??
    o.title ??
    o.name ??
    o.message ??
    o.text ??
    o.prompt ??
    o.objective ??
    o.symbol ??
    o.template ??
    o.worker ??
    o.reason ??
    o.to ??
    o.recipient ??
    o.input
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
    summary: summarize(base, input),
    isDiff: DIFF_TOOLS.some((t) => base === t || base.includes(t)),
  }
}
