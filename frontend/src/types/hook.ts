// PreToolUse / PostToolUse hooks (Phase P4) — mirrors db.Hook. A hook runs an
// external command around a native tool call: it may rewrite the input/output,
// auto-approve, or block the call. The command speaks the Claude Code hook
// contract (JSON on stdin, JSON on stdout, exit 2 = block).

export type HookEvent = 'PreToolUse' | 'PostToolUse'

export interface Hook {
  id: string
  event: HookEvent
  matcher: string // tool-name glob, "" = all tools
  type: string // "command"
  command: string
  timeoutSec: number
  enabled: boolean
  createdBy?: string
  createdAt: number
}

// BuiltinHook is one of SwarmGo's automatic, non-editable tool behaviours
// (freshness guard, CLI native-tool bridging, hook passthrough, ...) surfaced
// read-only in the Hooks screen. Mirrors api.builtinHook.
export interface BuiltinHook {
  name: string
  scope: 'native' | 'cli' | 'both'
  event: string // "PreToolUse" | "PostToolUse" | "System" | combinations
  description: string
  enabled: boolean
  setting?: string // settings.json key that toggles it (absent = always on)
}
